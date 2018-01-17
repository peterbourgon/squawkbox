package main

import (
	"testing"

	"github.com/oklog/ulid"
)

var (
	e1            = event{ULID: ulid.MustNew(100, nil).String()}
	e2            = event{ULID: ulid.MustNew(200, nil).String()}
	e3            = event{ULID: ulid.MustNew(300, nil).String()}
	e4            = event{ULID: ulid.MustNew(400, nil).String()}
	e5            = event{ULID: ulid.MustNew(500, nil).String()}
	e6            = event{ULID: ulid.MustNew(600, nil).String()}
	e7            = event{ULID: ulid.MustNew(700, nil).String()}
	e8            = event{ULID: ulid.MustNew(800, nil).String()}
	eventsFixture = []event{e8, e7, e6, e5, e4, e3, e2, e1}
)

func TestEventLogListPagination(t *testing.T) {
	el := newEventLog(nil)
	el.events = eventsFixture // hack

	for _, testcase := range []struct {
		name  string
		from  string
		count int
		want  []event
	}{
		{"zero from the top",
			"", 0,
			[]event{e8, e7, e6, e5, e4, e3, e2, e1}, // should resolve to 10
		},
		{"three from the top",
			"", 3,
			[]event{e8, e7, e6},
		},
		{"two from the middle",
			e6.ULID, 2,
			[]event{e5, e4}, // should skip the passed ULID
		},
		{"past the oldest",
			e1.ULID, 99,
			[]event{}, // should be nothing
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			have := el.listEvents(testcase.from, testcase.count)
			if !compareEventULIDs(testcase.want, have) {
				t.Errorf("(%s, %d): want %v, have %v", testcase.from, testcase.count, testcase.want, have)
			}
		})
	}
}

func compareEventULIDs(a, b []event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i].ULID != b[i].ULID {
			return false
		}
	}
	return true
}
