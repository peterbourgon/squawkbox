package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
)

var entropy = rand.New(rand.NewSource(time.Now().UnixNano()))

type auditEvent struct {
	ID      string            `json:"id"`
	Kind    auditEventKind    `json:"kind"`
	Request auditEventRequest `json:"request"`
	Details []string          `json:"details"`
}

func newAuditEvent(r *http.Request) *auditEvent {
	return &auditEvent{
		ID:      ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy).String(),
		Kind:    unknown,
		Request: makeRequest(r),
	}
}

func (e *auditEvent) setKind(k auditEventKind) {
	e.Kind = k
}

func (e *auditEvent) eventLog(s string) {
	e.Details = append(e.Details, s)
}

func (e *auditEvent) eventLogf(format string, args ...interface{}) {
	e.Details = append(e.Details, fmt.Sprintf(format, args...))
}

func (e *auditEvent) finalize(took time.Duration, code int) {
	e.eventLogf("Request took %s", took)
	e.eventLogf("HTTP status %d %s", code, http.StatusText(code))
}

type auditEventKind struct {
	Name  string       `json:"name"`
	Color displayColor `json:"color"`
	List  bool         `json:"list"`
}

var (
	unknown            = auditEventKind{"Unknown kind", gray, true}
	doorbellGreeting   = auditEventKind{"Doorbell greeting", blue, true}
	doorbellForward    = auditEventKind{"Doorbell forward", blue, true}
	doorbellBypass     = auditEventKind{"Doorbell bypass", red, true}
	doorbellRecording  = auditEventKind{"Doorbell recording", blue, true}
	adminIndex         = auditEventKind{"Admin index", white, false}
	adminGetEvents     = auditEventKind{"Admin get events", white, false}
	adminGetEvent      = auditEventKind{"Admin get event", white, false}
	adminGetRecordings = auditEventKind{"Admin get recordings", white, false}
	adminGetRecording  = auditEventKind{"Admin get recording", white, false}
	genericHTTPRequest = auditEventKind{"Generic HTTP request", gray, true}
)

type auditEventRequest struct {
	Method  string      `json:"method"`
	URI     string      `json:"uri"`
	Headers http.Header `json:"headers"`
}

func makeRequest(r *http.Request) auditEventRequest {
	return auditEventRequest{
		Method:  r.Method,
		URI:     r.RequestURI,
		Headers: r.Header,
	}
}

type displayColor string

const (
	blue   displayColor = "lightblue"
	orange displayColor = "orange"
	red    displayColor = "red"
	white  displayColor = "white"
	gray   displayColor = "gray"
)

//
//
//

type auditLog struct {
	mtx      sync.Mutex
	filename string
}

func newAuditLog(filename string) (*auditLog, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if err := writeAuditEvents(filename, []auditEvent{}); err != nil {
			return nil, errors.Wrap(err, "couldn't create events file")
		}
	}
	return &auditLog{
		filename: filename,
	}, nil
}

func (log *auditLog) logEvent(e *auditEvent) error {
	log.mtx.Lock()
	defer log.mtx.Unlock()

	events, err := readAuditEvents(log.filename)
	if err != nil {
		return errors.Wrap(err, "couldn't read existing events")
	}

	events = append([]auditEvent{*e}, events...)

	if err := writeAuditEvents(log.filename, events); err != nil {
		return errors.Wrap(err, "couldn't re-write events file")
	}

	return nil
}

func (log *auditLog) getEvents(fromULID string, count int) ([]auditEvent, error) {
	if fromULID == "" {
		fromULID = ulid.MustNew(ulid.Now(), nil).String()
	}

	from := ulid.MustParse(fromULID)
	if count <= 0 {
		count = 100
	}

	log.mtx.Lock()
	defer log.mtx.Unlock()

	events, err := readAuditEvents(log.filename)
	if err != nil {
		return []auditEvent{}, errors.Wrap(err, "couldn't read events file")
	}

	var res []auditEvent
	for _, e := range events {
		if ulid.MustParse(e.ID).Compare(from) >= 0 {
			continue
		}
		if !e.Kind.List {
			continue
		}
		res = append(res, e)
		if len(res) >= count {
			break
		}
	}
	return res, nil
}

func (log *auditLog) getEvent(id string) (auditEvent, error) {
	log.mtx.Lock()
	defer log.mtx.Unlock()

	events, err := readAuditEvents(log.filename)
	if err != nil {
		return auditEvent{}, errors.Wrap(err, "couldn't read events file")
	}

	for _, e := range events {
		if e.ID == id {
			return e, nil
		}
	}
	return auditEvent{}, errors.New("not found")
}

//
//
//

func readAuditEvents(filename string) ([]auditEvent, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return []auditEvent{}, errors.Wrap(err, "couldn't open events file")
	}

	events := []auditEvent{}
	if err := json.Unmarshal(buf, &events); err != nil {
		return []auditEvent{}, errors.Wrap(err, "couldn't unmarshal events file")
	}

	return events, nil
}

func writeAuditEvents(filename string, events []auditEvent) error {
	buf, err := json.MarshalIndent(events, "", "    ")
	if err != nil {
		return errors.Wrap(err, "couldn't marshal events")
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, secureFileMode)
	if err != nil {
		return errors.Wrap(err, "couldn't create events file")
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return errors.Wrap(err, "couldn't write events file")
	}

	return nil
}
