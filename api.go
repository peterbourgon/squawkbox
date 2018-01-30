package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
)

func loggingMiddleware(logger log.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				begin = time.Now()
				iw    = &interceptingWriter{http.StatusOK, w}
			)
			next.ServeHTTP(iw, r)
			level.Info(logger).Log(
				"method", r.Method,
				"uri", r.RequestURI,
				"content_length", r.ContentLength,
				"status_code", iw.code,
				"took", time.Since(begin),
			)
		})
	}
}

func auditingMiddleware(log *auditLog) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				begin = time.Now()
				e     = newAuditEvent(r)
				ctx   = context.WithValue(r.Context(), auditEventKey, e)
				rr    = r.WithContext(ctx)
				iw    = &interceptingWriter{http.StatusOK, w}
			)
			next.ServeHTTP(iw, rr)
			e.finalize(time.Since(begin), iw.code)
			log.logEvent(e)
		})
	}
}

type eventLogger interface {
	logEvent(*auditEvent)
}

const auditEventKey = "audit_event"

func setAuditEvent(ctx context.Context, k auditEventKind) *auditEvent {
	e := ctx.Value(auditEventKey).(*auditEvent)
	e.setKind(k)
	return e
}

func registerDoorbellRoutes(
	router *mux.Router,
	forwardText string,
	forwardNumber string,
	noResponseText string,
	rm *recordingManager,
) {
	var (
		greeting  = handleGreeting()
		forward   = handleForward(forwardText, forwardNumber, noResponseText)
		recording = handleRecording(rm)
	)
	router.Methods("POST").Path("/v1/greeting").Handler(greeting)
	router.Methods("POST").Path("/v1/forward").Handler(forward)
	router.Methods("POST").Path("/v1/recordings").Handler(recording)
}

func handleGreeting() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAuditEvent(r.Context(), doorbellGreeting)

		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Response>
				<Redirect>/v1/forward</Redirect>
			</Response>
	`)
	})
}

func handleForward(forwardText, forwardNumber, noResponseText string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAuditEvent(r.Context(), doorbellForward)

		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
			<Response>
				<Say>%s</Say>
				<Dial record="record-from-ringing" recordingStatusCallback="/v1/recordings" recordingStatusCallbackMethod="POST">
					<Number>%s</Number>
				</Dial>
				<Say>%s</Say>
				<Hangup />
			</Response>
		`, forwardText, forwardNumber, noResponseText)
	})
}

func handleRecording(m *recordingManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := setAuditEvent(r.Context(), doorbellRecording)

		r.ParseForm()
		var (
			url = r.FormValue("RecordingUrl")
			sid = r.FormValue("RecordingSid")
			dur = r.FormValue("RecordingDuration")
		)

		if url == "" || sid == "" || dur == "" {
			e.eventLog("Recording request was missing data; not saved")
			http.NotFound(w, r)
			return
		}

		var (
			date = time.Now().Format("2006-01-02-15-04-05")
			name = date + "-" + dur + "sec" + "-" + sid + ".wav"
		)
		if err := m.saveRecording(name, url); err != nil {
			e.eventLogf("Recording save failed: %v", err)
			http.Error(w, errors.Wrap(err, "saving recording").Error(), http.StatusInternalServerError)
			return
		}

		e.eventLog("Recording saved successfully")
		fmt.Fprintf(w, "Saved %s OK\n", name)
	})
}

//
//
//

func authMiddleware(realm, user, pass string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requser, reqpass, _ := r.BasicAuth()
			if requser != user || reqpass != pass {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, http.StatusText(http.StatusUnauthorized))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func registerAdminRoutes(
	router *mux.Router,
	basicAuthRealm, basicAuthUser, basicAuthPass string,
	log *auditLog,
	rm *recordingManager,
) {
	auth := authMiddleware(basicAuthRealm, basicAuthUser, basicAuthPass)
	router.Methods("GET").Path("/").Handler(auth(handleIndex()))
	router.Methods("GET").Path("/events").Handler(auth(handleGetEvents(log)))
	router.Methods("GET").Path("/events/{id}").Handler(auth(handleGetEvent(log)))
	router.Methods("GET").Path("/recordings").Handler(auth(handleGetRecordings(rm)))
	router.Methods("GET").Path("/recordings/{id}").Handler(auth(handleGetRecording(rm)))
}

func handleIndex() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/events", http.StatusTemporaryRedirect)
	})
}

func handleGetEvents(log *auditLog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAuditEvent(r.Context(), adminGetEvents)

		r.ParseForm()
		var (
			from     = r.FormValue("from")
			countStr = r.FormValue("count")
			count, _ = strconv.Atoi(countStr)
		)
		if count == 0 {
			count = 100
		}

		events, err := log.getEvents(from, count)
		if err != nil {
			http.Error(w, errors.Wrap(err, "couldn't list events").Error(), http.StatusInternalServerError)
			return
		}

		type templateEvent struct {
			Color   string
			ULID    string
			Time    string
			Kind    string
			Details []string
		}

		templateEvents := make([]templateEvent, len(events))
		for i, event := range events {
			templateEvents[i] = templateEvent{
				Color:   string(event.Kind.Color),
				ULID:    event.ID,
				Time:    ulid2localtime(event.ID),
				Kind:    event.Kind.Name,
				Details: event.Details,
			}
		}

		var nextPage string
		if len(templateEvents) >= count {
			nextPage = templateEvents[len(templateEvents)-1].ULID
		}

		aggregate := headerTemplate + eventsTemplate + footerTemplate
		if err := template.Must(template.New("events").Parse(aggregate)).Execute(w, struct {
			Events   []templateEvent
			NextPage string
		}{
			Events:   templateEvents,
			NextPage: nextPage,
		}); err != nil {
			http.Error(w, errors.Wrap(err, "executing events template").Error(), http.StatusInternalServerError)
			return
		}
	})
}

func handleGetEvent(log *auditLog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAuditEvent(r.Context(), adminGetEvent)

		id := mux.Vars(r)["id"]
		if id == "" {
			http.Error(w, "no event ID provided; bad routing", http.StatusInternalServerError)
			return
		}

		e, err := log.getEvent(id)
		if err != nil {
			http.Error(w, errors.Wrap(err, "getting event").Error(), http.StatusNotFound)
			return
		}

		var httpDetails []string
		httpDetails = append(httpDetails, fmt.Sprintf("%s %s", e.Request.Method, e.Request.URI))
		for k, vs := range e.Request.Headers {
			httpDetails = append(httpDetails, fmt.Sprintf("%s: %s", k, strings.Join(vs, ", ")))
		}
		sort.Strings(httpDetails)

		aggregate := headerTemplate + eventTemplate + footerTemplate
		if err := template.Must(template.New("event").Parse(aggregate)).Execute(w, struct {
			Color   string
			ULID    string
			Time    string
			UTC     string
			Kind    string
			Details []string
			HTTP    []string
		}{
			Color:   string(e.Kind.Color),
			ULID:    e.ID,
			Time:    ulid2localtime(e.ID),
			UTC:     ulid2utctime(e.ID),
			Kind:    e.Kind.Name,
			Details: e.Details,
			HTTP:    httpDetails,
		}); err != nil {
			http.Error(w, errors.Wrap(err, "executing event template").Error(), http.StatusInternalServerError)
			return
		}
	})
}

func handleGetRecordings(rm *recordingManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAuditEvent(r.Context(), adminGetRecordings)

		recordings := rm.listRecordings()

		aggregate := headerTemplate + recordingsTemplate + footerTemplate
		if err := template.Must(template.New("recordings").Parse(aggregate)).Execute(w, struct {
			Recordings []string
		}{
			Recordings: recordings,
		}); err != nil {
			http.Error(w, errors.Wrap(err, "executing recordings template").Error(), http.StatusInternalServerError)
			return
		}
	})
}

func handleGetRecording(rm *recordingManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := setAuditEvent(r.Context(), adminGetRecording)

		id, ok := mux.Vars(r)["id"]
		if !ok {
			http.Error(w, "recording ID not provided", http.StatusBadRequest)
			return
		}

		rec, err := rm.getRecording(id)
		if err != nil {
			http.Error(w, errors.Wrap(err, "fetching recording").Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "audio/wav")
		n, err := io.Copy(w, rec)
		if err != nil {
			http.Error(w, errors.Wrap(err, "streaming recording to user").Error(), http.StatusInternalServerError)
			return
		}

		e.eventLogf("Streamed %dB to client", n)
	})
}

//
//
//

type interceptingWriter struct {
	code int
	http.ResponseWriter
}

func (iw *interceptingWriter) WriteHeader(code int) {
	iw.code = code
	iw.ResponseWriter.WriteHeader(code)
}

func isNumeric(s string) bool {
	for _, r := range s {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		default:
			return false
		}
	}
	return true
}

func ulid2localtime(id string) string {
	u, err := ulid.Parse(id)
	if err != nil {
		return "(couldn't parse time from ID)"
	}
	var (
		msec = u.Time()
		sec  = msec / 1e3
		nsec = (msec % 1e3) * 1e6
		t    = time.Unix(int64(sec), int64(nsec))
	)
	return t.Format(myDate)
}

func ulid2utctime(id string) string {
	u, err := ulid.Parse(id)
	if err != nil {
		return "(couldn't parse time from ID)"
	}
	var (
		msec = u.Time()
		sec  = msec / 1e3
		nsec = (msec % 1e3) * 1e6
		t    = time.Unix(int64(sec), int64(nsec))
	)
	return t.UTC().Format(time.RFC3339Nano)
}

const myDate = "Monday 02 Jan 2006 15:04:05 MST"
