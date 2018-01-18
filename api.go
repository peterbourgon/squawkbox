package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type api struct {
	EventLog         *eventLog
	Authenticate     func(http.Handler) http.Handler
	GreetingText     string
	ForwardText      string
	ForwardNumber    string
	NoResponseText   string
	CodeManager      *codeManager
	RecordingManager *recordingManager
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set up initial values.
	var (
		begin = time.Now()
		iw    = &interceptingWriter{code: http.StatusOK, ResponseWriter: w}
	)
	w = iw

	// Each request has base event data.
	data := eventData{}
	data.addData(
		"HTTP RemoteAddr", r.RemoteAddr,
		"HTTP Method", r.Method,
		"HTTP URI", r.URL.RequestURI(),
		"HTTP ContentLength", fmt.Sprint(r.ContentLength),
	)
	for k, vs := range r.Header {
		data.addData("HTTP Header "+k, strings.Join(vs, ", "))
	}

	// Handlers will add to that event data.
	var (
		reqctx = r.Context()
		newctx = context.WithValue(reqctx, dataCollectorContextKey, data)
	)
	r = r.WithContext(newctx)

	// Build the route muxer.
	router := mux.NewRouter()
	router.StrictSlash(true)
	router.Methods("GET").Path("/v1/greeting").HandlerFunc(a.handleGreeting)
	router.Methods("GET").Path("/v1/bypass").HandlerFunc(a.handleBypass)
	router.Methods("GET").Path("/v1/forward").HandlerFunc(a.handleForward)
	router.Methods("GET").Path("/v1/recordings").HandlerFunc(a.handlePostRecording)
	router.Methods("GET").Path("/").Handler(a.Authenticate(http.HandlerFunc(a.handleGetIndex)))
	router.Methods("GET").Path("/events").Handler(a.Authenticate(http.HandlerFunc(a.handleGetEvents)))
	router.Methods("GET").Path("/events/{id}").Handler(a.Authenticate(http.HandlerFunc(a.handleGetEvent)))
	router.Methods("GET").Path("/codes").Handler(a.Authenticate(http.HandlerFunc(a.handleGetCodes)))
	router.Methods("POST").Path("/codes").Handler(a.Authenticate(http.HandlerFunc(a.handlePostCodes)))
	router.Methods("POST").Path("/codes/{id}").Handler(a.Authenticate(http.HandlerFunc(a.handleDeleteCodes)))
	router.Methods("GET").Path("/recordings").Handler(a.Authenticate(http.HandlerFunc(a.handleGetRecordings)))

	// Serve the request.
	router.ServeHTTP(w, r)

	// Add final event data.
	data.addData(
		"HTTP Request Duration", time.Since(begin).String(),
		"HTTP Status Code", fmt.Sprint(iw.code)+" "+http.StatusText(iw.code),
	)

	// Extract the Kind.
	kind := eventKind(data[dataKeyKind])
	if kind == "" {
		kind = eventKindGenericHTTPRequest
	}
	delete(data, dataKeyKind)

	// Log the event.
	switch kind {
	case eventKindAdminIndex:
	case eventKindAdminListCodes, eventKindAdminListEvents, eventKindAdminListRecordings:
	case eventKindAdminGetEvent, eventKindAdminGetRecording:
		// Don't log these ones.
	default:
		a.EventLog.logEvent(kind, data)
	}
}

func (a *api) handleGreeting(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindDoorbellGreeting))

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
		<Response> 
			<Gather input="dtmf" action="/v1/bypass" timeout="5" finishOnKey="#">
				<Say>%s</Say>
			</Gather>
			<Redirect>/v1/forward</Redirect>
		</Response>
	`, a.GreetingText)
}

func (a *api) handleBypass(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindDoorbellBypass))

	r.ParseForm()
	digits := r.FormValue("Digits")
	dc.addData(dataKeyBypassAttemptWith, digits)

	if digits != "" && a.CodeManager.checkCode(digits) == nil {
		dc.addData(dataKeyBypassResult, dataValueSuccess)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
			<Response> 
				<Play digits="w9"></Play>
				<Hangup />
			</Response>
		`)
		return
	}

	dc.addData(dataKeyBypassResult, dataValueFailed)
	a.handleForward(w, r)
}

func (a *api) handleForward(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindDoorbellForward))

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
		<Response> 
			<Say>%s</Say>
			<Dial record="record-from-ringing" recordingStatusCallback="/v1/recordings" recordingStatusCallbackMethod="GET">
				<Number>%s</Number>
			</Dial>
			<Say>%s</Say>
			<Hangup />
		</Response>
	`, a.ForwardText, a.ForwardNumber, a.NoResponseText)
}

func (a *api) handlePostRecording(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindDoorbellRecording))

	r.ParseForm()
	var (
		url = r.FormValue("RecordingUrl")
		sid = r.FormValue("RecordingSid")
		dur = r.FormValue("RecordingDuration")
	)
	if url == "" || sid == "" || dur == "" {
		dc.addData(
			dataKeyRecordingSaveResult, dataValueFailed,
			dataKeyRecordingFailReason, "missing data",
		)
		http.NotFound(w, r)
		return
	}

	var (
		date = time.Now().Format("2006-01-02-15-04-05")
		name = date + "-" + dur + "sec" + "-" + sid + ".wav"
	)
	if err := a.RecordingManager.saveRecording(name, url); err != nil {
		dc.addData(
			dataKeyRecordingSaveResult, dataValueFailed,
			dataKeyRecordingFailReason, err.Error(),
		)
		http.Error(w, errors.Wrap(err, "saving recording").Error(), http.StatusInternalServerError)
		return
	}

	dc.addData(dataKeyRecordingSaveResult, dataValueSuccess)
	fmt.Fprintf(w, "Saved %s OK\n", name)
}

func (a *api) handleGetRecordings(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	if name := r.FormValue("name"); name == "" {
		dc.addData(dataKeyKind, string(eventKindAdminListRecordings))
		buf, err := json.MarshalIndent(a.RecordingManager.listRecordings(), "", "    ")
		if err != nil {
			http.Error(w, errors.Wrap(err, "marshaling list of recordings").Error(), http.StatusInternalServerError)
			return
		}
		w.Write(buf)
		return
	} else {
		dc.addData(
			dataKeyKind, string(eventKindAdminGetRecording),
			dataKeyRecordingName, name,
		)
		r, err := a.RecordingManager.getRecording(name)
		if err != nil {
			http.Error(w, errors.Wrap(err, "fetching recording").Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		io.Copy(w, r)
		return
	}
}

func (a *api) handleGetIndex(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminIndex))

	http.Redirect(w, r, "/events", http.StatusTemporaryRedirect)
}

func (a *api) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminListEvents))

	r.ParseForm()
	var (
		from     = r.FormValue("from")
		countStr = r.FormValue("count")
		count, _ = strconv.Atoi(countStr)
		events   = a.EventLog.listEvents(from, count)
	)

	type templateEvent struct {
		Color   string
		ULID    string
		Time    string
		Kind    string
		Details []string
	}

	templateEvents := make([]templateEvent, len(events))
	for i, event := range events {
		generalDetails, _ := parseDetails(event.Data)
		templateEvents[i] = templateEvent{
			Color:   colorFor(event.Kind),
			ULID:    event.ULID,
			Time:    event.HumanTime,
			Kind:    string(event.Kind),
			Details: generalDetails,
		}
	}

	var nextPage string
	if len(templateEvents) > 0 {
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
}

func (a *api) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminGetEvent))

	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, "no event ID provided; bad routing", http.StatusInternalServerError)
		return
	}

	ev, ok := a.EventLog.getEvent(id)
	if !ok {
		http.Error(w, fmt.Sprintf("event %s not found", id), http.StatusNotFound)
		return
	}

	generalDetails, httpDetails := parseDetails(ev.Data)

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
		Color:   colorFor(ev.Kind),
		ULID:    ev.ULID,
		Time:    ev.HumanTime,
		UTC:     ev.Timestamp,
		Kind:    string(ev.Kind),
		Details: generalDetails,
		HTTP:    httpDetails,
	}); err != nil {
		http.Error(w, errors.Wrap(err, "executing event template").Error(), http.StatusInternalServerError)
		return
	}
}

func (a *api) handleGetCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminListCodes))

	codes := a.CodeManager.listCodes()
	flat := make([]bypassCode, 0, len(codes))
	for _, code := range codes {
		if t, err := time.Parse(time.RFC3339, code.ExpiresAt); err == nil {
			code.ExpiresAt = t.Format(myDate) // for display
		}
		flat = append(flat, code)
	}

	aggregate := headerTemplate + codesTemplate + footerTemplate
	if err := template.Must(template.New("event").Parse(aggregate)).Execute(w, struct {
		Codes []bypassCode
	}{
		Codes: flat,
	}); err != nil {
		http.Error(w, errors.Wrap(err, "executing codes template").Error(), http.StatusInternalServerError)
		return
	}
}

func (a *api) handlePostCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminCreateCode))

	r.ParseForm()
	var (
		code         = r.FormValue("code")
		useCountStr  = r.FormValue("use_count")
		expiresIn    = r.FormValue("expires_in")
		expiresAtStr = r.FormValue("expires_at")
	)
	if code == "" {
		http.Error(w, "code not specified", http.StatusBadRequest)
		return
	}
	if !isNumeric(code) {
		http.Error(w, "code is non-numeric", http.StatusBadRequest)
		return
	}

	var expiresAt time.Time
	if expiresIn != "" {
		d, err := time.ParseDuration(expiresIn)
		if err != nil {
			http.Error(w, "expires_in is invalid", http.StatusBadRequest)
			return
		}
		expiresAt = time.Now().Add(d)
	}
	if expiresAtStr != "" {
		t, err := time.Parse(time.RFC3339, expiresAtStr)
		if err != nil {
			http.Error(w, "expires_at is invalid", http.StatusBadRequest)
			return
		}
		expiresAt = t
	}
	if expiresAt.IsZero() {
		http.Error(w, "either expires_in or expires_at must be specified", http.StatusBadRequest)
		return
	}

	useCount, err := strconv.Atoi(useCountStr)
	if err != nil {
		http.Error(w, errors.Wrap(err, "parsing use_count as integer").Error(), http.StatusBadRequest)
		return
	}
	if useCount <= 0 {
		http.Error(w, "use_count must be > 0", http.StatusBadRequest)
		return
	}
	if err := a.CodeManager.addCode(code, useCount, expiresAt); err != nil {
		http.Error(w, errors.Wrap(err, "adding code").Error(), http.StatusInternalServerError)
		return
	}

	expiring := expiresAt.Format(time.RFC3339)
	dc.addData(
		dataKeyBypassCode, code,
		dataKeyBypassCodeUseCount, fmt.Sprint(useCount),
		dataKeyBypassCodeExpiresAt, expiring,
	)

	r.Method = "GET"
	http.Redirect(w, r, "/codes", http.StatusSeeOther)
}

func (a *api) handleDeleteCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminRevokeCode))

	r.ParseForm()
	if r.FormValue("delete") == "" {
		http.Error(w, "POST code without delete param", http.StatusBadRequest)
		return
	}

	code := mux.Vars(r)["id"]
	if code == "" {
		http.Error(w, "code not specified", http.StatusBadRequest)
		return
	}
	if err := a.CodeManager.revokeCode(code); err != nil {
		http.Error(w, errors.Wrap(err, "revoking code").Error(), http.StatusBadRequest)
		return
	}
	dc.addData(dataKeyBypassCode, code)

	r.Method = "GET"
	http.Redirect(w, r, "/codes", http.StatusSeeOther)
}

type interceptingWriter struct {
	code int
	http.ResponseWriter
}

func (iw *interceptingWriter) WriteHeader(code int) {
	iw.code = code
	iw.ResponseWriter.WriteHeader(code)
}

var dataCollectorContextKey string = "data-collector"

type eventDataCollector interface{ addData(keyvals ...string) }

type eventData map[string]string

func (ed eventData) addData(keyvals ...string) {
	for i := 0; i < len(keyvals); i += 2 {
		ed[keyvals[i]] = keyvals[i+1]
	}
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

var (
	dataKeyKind                = "Kind"
	dataKeyRecordingSaveResult = "Recording save result"
	dataKeyRecordingFailReason = "Recording fail reason"
	dataKeyRecordingName       = "Recording name"
	dataKeyBypassAttemptWith   = "Bypass attempt with"
	dataKeyBypassResult        = "Bypass result"
	dataKeyBypassCode          = "Bypass code"
	dataKeyBypassCodeUseCount  = "Bypass code use count"
	dataKeyBypassCodeExpiresAt = "Bypass code expires at"

	dataValueSuccess = "SUCCESS"
	dataValueFailed  = "FAILED"
)

func colorFor(kind eventKind) string {
	switch kind {
	case eventKindAdminCreateCode, eventKindAdminRevokeCode, eventKindAdminGetRecording:
		return "orange"
	case eventKindDoorbellGreeting, eventKindDoorbellForward, eventKindDoorbellRecording:
		return "lightblue"
	case eventKindDoorbellBypass:
		return "red"
	default:
		return "white"
	}
}

func parseDetails(data map[string]string) (general, http []string) {
	for k, v := range data {
		var (
			httpPrefix = strings.HasPrefix(k, "HTTP ")
			statusCode = k == "HTTP Status Code"
		)
		if httpPrefix {
			http = append(http, fmt.Sprintf("%s: %s", strings.TrimPrefix(k, "HTTP "), v))
		}
		if !httpPrefix || statusCode {
			general = append(general, fmt.Sprintf("%s: %s", k, v))
		}
	}
	sort.Strings(general)
	sort.Strings(http)
	return general, http
}
