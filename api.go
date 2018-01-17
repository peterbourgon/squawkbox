package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// Service the request.
	method, path := r.Method, r.URL.Path
	switch {
	case method == "GET" && path == "/v1/greeting":
		a.handleGreeting(w, r)
	case method == "GET" && path == "/v1/bypass":
		a.handleBypass(w, r)
	case method == "GET" && path == "/v1/forward":
		a.handleForward(w, r)
	case method == "GET" && path == "/v1/recordings":
		a.handlePostRecording(w, r)

	case method == "GET" && path == "/":
		a.Authenticate(http.HandlerFunc(a.handleGetIndex)).ServeHTTP(w, r)
	case method == "GET" && path == "/events":
		a.Authenticate(http.HandlerFunc(a.handleGetEvents)).ServeHTTP(w, r)
	case method == "GET" && path == "/codes":
		a.Authenticate(http.HandlerFunc(a.handleGetCodes)).ServeHTTP(w, r)
	case method == "POST" && path == "/codes":
		a.Authenticate(http.HandlerFunc(a.handlePostCodes)).ServeHTTP(w, r)
	case method == "DELETE" && path == "/codes":
		a.Authenticate(http.HandlerFunc(a.handleDeleteCodes)).ServeHTTP(w, r)
	case method == "GET" && path == "/recordings":
		a.Authenticate(http.HandlerFunc(a.handleGetRecordings)).ServeHTTP(w, r)

	default:
		http.NotFound(w, r)
	}

	// Add final event data.
	data.addData(
		"HTTP Request Duration", time.Since(begin).String(),
		"HTTP Status Code", fmt.Sprint(iw.code)+" "+http.StatusText(iw.code),
	)

	// Log the event.
	kind := eventKind(data[dataKeyKind])
	if kind == "" {
		kind = eventKindGenericHTTPRequest
	}
	a.EventLog.logEvent(kind, data)
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

	var (
		from     = r.FormValue("from")
		countStr = r.FormValue("count")
		count, _ = strconv.Atoi(countStr)
	)
	buf, err := json.MarshalIndent(a.EventLog.listEvents(from, count), "", "    ")
	if err != nil {
		http.Error(w, errors.Wrap(err, "encoding events").Error(), http.StatusInternalServerError)
		return
	}
	w.Write(buf)
}

func (a *api) handleGetCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminListCodes))

	codes := a.CodeManager.listCodes()
	buf, err := json.MarshalIndent(codes, "", "    ")
	if err != nil {
		http.Error(w, errors.Wrap(err, "encoding current codes").Error(), http.StatusInternalServerError)
		return
	}
	w.Write(buf)
}

func (a *api) handlePostCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminCreateCode))

	r.ParseForm()
	var (
		code         = r.FormValue("code")
		useCountStr  = r.FormValue("use_count")
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
	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		http.Error(w, "expires_at not specified", http.StatusBadRequest)
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
	fmt.Fprintf(w, "Added code %s, use count %d, expiring on %s -- OK\n", code, useCount, expiring)
}

func (a *api) handleDeleteCodes(w http.ResponseWriter, r *http.Request) {
	dc := r.Context().Value(dataCollectorContextKey).(eventDataCollector)
	dc.addData(dataKeyKind, string(eventKindAdminRevokeCode))

	r.ParseForm()
	code := r.FormValue("code")
	if code == "" {
		http.Error(w, "code not specified", http.StatusBadRequest)
		return
	}
	if err := a.CodeManager.revokeCode(code); err != nil {
		http.Error(w, errors.Wrap(err, "revoking code").Error(), http.StatusBadRequest)
		return
	}
	dc.addData(dataKeyBypassCode, code)
	fmt.Fprintf(w, "Revoked code %s -- OK\n", code)
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
