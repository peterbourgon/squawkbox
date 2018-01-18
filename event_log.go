package main

import (
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid"
)

const myDate = "Monday 02 Jan 2006 15:04:05 MST"

type eventLog struct {
	mtx     sync.Mutex
	entropy io.Reader
	events  []event
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

func newEventLog(entropy io.Reader) *eventLog {
	return &eventLog{
		entropy: entropy,
	}
}

func (el *eventLog) logEvent(kind eventKind, data map[string]string) string {
	el.mtx.Lock()
	defer el.mtx.Unlock()
	var (
		now  = time.Now()
		ulid = ulid.MustNew(ulid.Timestamp(now), el.entropy).String()
	)
	el.events = append([]event{event{
		ULID:      ulid,
		Timestamp: now.UTC().Format(time.RFC3339),
		HumanTime: now.Format(myDate),
		Kind:      kind,
		Data:      data,
	}}, el.events...)
	return ulid
}

func (el *eventLog) listEvents(fromULID string, count int) []event {
	if fromULID == "" {
		fromULID = ulid.MustNew(ulid.Now(), nil).String()
	}
	from := ulid.MustParse(fromULID)
	if count <= 0 {
		count = 100
	}
	el.mtx.Lock()
	defer el.mtx.Unlock()
	var res []event
	for _, e := range el.events {
		if ulid.MustParse(e.ULID).Compare(from) >= 0 {
			continue
		}
		res = append(res, e)
		if len(res) >= count {
			break
		}
	}
	return res
}

func (el *eventLog) getEvent(ulid string) (event, bool) {
	el.mtx.Lock()
	defer el.mtx.Unlock()
	for _, e := range el.events {
		if e.ULID == ulid {
			return e, true
		}
	}
	return event{}, false
}
