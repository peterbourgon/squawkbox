package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
)

const myDate = "Monday 02 Jan 2006 15:04:05 MST"

type eventLog struct {
	mtx      sync.Mutex
	filename string
	entropy  io.Reader
}

type event struct {
	ULID      string            `json:"ulid"`
	Timestamp string            `json:"timestamp"`
	HumanTime string            `json:"human_time"`
	Kind      eventKind         `json:"kind"`
	Data      map[string]string `json:"data"`
}

type eventKind string

const (
	eventKindDoorbellGreeting  eventKind = "Doorbell greeting"
	eventKindDoorbellForward   eventKind = "Doorbell forward"
	eventKindDoorbellBypass    eventKind = "Doorbell bypass"
	eventKindDoorbellRecording eventKind = "Doorbell recording"

	eventKindAdminIndex          eventKind = "Admin index"
	eventKindAdminListEvents     eventKind = "Admin list events"
	eventKindAdminGetEvent       eventKind = "Admin get event"
	eventKindAdminListCodes      eventKind = "Admin list codes"
	eventKindAdminCreateCode     eventKind = "Admin create code"
	eventKindAdminRevokeCode     eventKind = "Admin revoke code"
	eventKindAdminListRecordings eventKind = "Admin list recordings"
	eventKindAdminGetRecording   eventKind = "Admin get recording"

	eventKindGenericHTTPRequest eventKind = "Generic HTTP request"
	eventKind404                eventKind = "HTTP Not Found"
)

func newEventLog(filename string, entropy io.Reader) (*eventLog, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if err := writeEvents(filename, []event{}); err != nil {
			return nil, errors.Wrap(err, "couldn't create events file")
		}
	}
	return &eventLog{
		filename: filename,
		entropy:  entropy,
	}, nil
}

func (el *eventLog) logEvent(kind eventKind, data map[string]string) (string, error) {
	el.mtx.Lock()
	defer el.mtx.Unlock()

	var (
		now  = time.Now()
		ulid = ulid.MustNew(ulid.Timestamp(now), el.entropy).String()
	)

	events, err := readEvents(el.filename)
	if err != nil {
		return "", errors.Wrap(err, "couldn't read existing events")
	}

	events = append([]event{event{
		ULID:      ulid,
		Timestamp: now.UTC().Format(time.RFC3339),
		HumanTime: now.Format(myDate),
		Kind:      kind,
		Data:      data,
	}}, events...)

	if err := writeEvents(el.filename, events); err != nil {
		return "", errors.Wrap(err, "couldn't re-write events file")
	}

	return ulid, nil
}

func (el *eventLog) listEvents(fromULID string, count int) ([]event, error) {
	if fromULID == "" {
		fromULID = ulid.MustNew(ulid.Now(), nil).String()
	}

	from := ulid.MustParse(fromULID)
	if count <= 0 {
		count = 100
	}

	el.mtx.Lock()
	defer el.mtx.Unlock()

	events, err := readEvents(el.filename)
	if err != nil {
		return []event{}, errors.Wrap(err, "couldn't read events file")
	}

	var res []event
	for _, e := range events {
		if ulid.MustParse(e.ULID).Compare(from) >= 0 {
			continue
		}
		res = append(res, e)
		if len(res) >= count {
			break
		}
	}
	return res, nil
}

func (el *eventLog) getEvent(ulid string) (event, error) {
	el.mtx.Lock()
	defer el.mtx.Unlock()

	events, err := readEvents(el.filename)
	if err != nil {
		return event{}, errors.Wrap(err, "couldn't read events file")
	}

	for _, e := range events {
		if e.ULID == ulid {
			return e, nil
		}
	}
	return event{}, errors.New("not found")
}

func readEvents(filename string) ([]event, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return []event{}, errors.Wrap(err, "couldn't open events file")
	}

	events := []event{}
	if err := json.Unmarshal(buf, &events); err != nil {
		return []event{}, errors.Wrap(err, "couldn't unmarshal events file")
	}

	return events, nil
}

func writeEvents(filename string, events []event) error {
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
