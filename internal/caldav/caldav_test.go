package caldav

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// testProvider returns a Provider usable for mapping tests without a network client.
func testProvider() *Provider {
	return &Provider{username: "me@icloud.com", timeZone: "UTC", prodID: "-//vamoose//test//EN"}
}

// TestHoldRoundTrip builds a hold, serializes it to iCalendar, decodes it, and maps
// it back, confirming the fields and attendee responses survive the round trip.
func TestHoldRoundTrip(t *testing.T) {
	t.Parallel()
	p := testProvider()
	start := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		Name string
		Hold calendar.Hold
	}{{ // Test 0: timed pto with a pending manager and an optional peer.
		Name: "timed pto",
		Hold: calendar.Hold{
			Subject: "Beach", Body: "note", Start: start, End: start.Add(2 * time.Hour),
			ShowAs: calendar.ShowFree,
			Attendees: []calendar.Attendee{
				{Person: calendar.Person{Email: "boss@x.com", Name: "Boss"}, Role: calendar.RoleRequired, Response: calendar.ResponseNotResponded},
				{Person: calendar.Person{Email: "peer@x.com"}, Role: calendar.RoleOptional, Response: calendar.ResponseNotResponded},
			},
		},
	}, { // Test 1: all-day away, no attendees.
		Name: "all day away",
		Hold: calendar.Hold{
			Subject: "OOO", Start: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
			End: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), AllDay: true, ShowAs: calendar.ShowBusy,
		},
	}, { // Test 2: the manager has accepted, the approval-read path.
		Name: "accepted manager",
		Hold: calendar.Hold{
			Subject: "Vet", Start: start, End: start.Add(time.Hour), ShowAs: calendar.ShowFree,
			Attendees: []calendar.Attendee{
				{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseAccepted},
			},
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			cal := p.toCalendar(test.Hold, "uid-x", time.Unix(0, 0).UTC())
			var buf bytes.Buffer
			if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
				t.Fatalf("encode: %v", err)
			}
			dec, err := ical.NewDecoder(bytes.NewReader(buf.Bytes())).Decode()
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := p.fromEvent(firstEvent(dec), "cal/uid-x.ics")

			if got.Subject != test.Hold.Subject {
				t.Errorf("subject = %q, want %q", got.Subject, test.Hold.Subject)
			}
			if got.AllDay != test.Hold.AllDay {
				t.Errorf("allDay = %v, want %v", got.AllDay, test.Hold.AllDay)
			}
			if got.ShowAs != test.Hold.ShowAs {
				t.Errorf("showAs = %q, want %q", got.ShowAs, test.Hold.ShowAs)
			}
			if !got.Start.Equal(test.Hold.Start) {
				t.Errorf("start = %v, want %v", got.Start, test.Hold.Start)
			}
			if !got.End.Equal(test.Hold.End) {
				t.Errorf("end = %v, want %v", got.End, test.Hold.End)
			}
			if len(got.Attendees) != len(test.Hold.Attendees) {
				t.Fatalf("attendees = %d, want %d", len(got.Attendees), len(test.Hold.Attendees))
			}
			byEmail := make(map[string]calendar.Attendee, len(got.Attendees))
			for _, a := range got.Attendees {
				byEmail[a.Person.Email] = a
			}
			for _, want := range test.Hold.Attendees {
				g, ok := byEmail[want.Person.Email]
				if !ok {
					t.Errorf("missing attendee %s", want.Person.Email)
					continue
				}
				if g.Role != want.Role || g.Response != want.Response {
					t.Errorf("attendee %s = role %s resp %s, want role %s resp %s",
						want.Person.Email, g.Role, g.Response, want.Role, want.Response)
				}
			}
		})
	}
}

// TestPickCalendar confirms calendar selection by name, then by event support.
func TestPickCalendar(t *testing.T) {
	t.Parallel()
	cals := []caldav.Calendar{
		{Path: "/tasks/", Name: "Reminders", SupportedComponentSet: []string{"VTODO"}},
		{Path: "/home/", Name: "Home", SupportedComponentSet: []string{"VEVENT"}},
		{Path: "/work/", Name: "Work", SupportedComponentSet: []string{"VEVENT"}},
	}
	tests := []struct {
		In   string
		Want string
	}{{ // Test 0: empty name picks the first event-capable calendar.
		In: "", Want: "/home/",
	}, { // Test 1: an exact name wins.
		In: "Work", Want: "/work/",
	}, { // Test 2: a name miss falls back to the first event-capable calendar.
		In: "Nope", Want: "/home/",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := pickCalendar(cals, test.In); got != test.Want {
				t.Errorf("pickCalendar(%q) = %q, want %q", test.In, got, test.Want)
			}
		})
	}
}

// TestParticipationMapping confirms response and role values map both directions.
func TestParticipationMapping(t *testing.T) {
	t.Parallel()
	responses := []calendar.Response{
		calendar.ResponseAccepted, calendar.ResponseDeclined,
		calendar.ResponseTentative, calendar.ResponseNotResponded,
	}
	for _, r := range responses {
		if got := responseFrom(partstat(r)); got != r {
			t.Errorf("response round trip %s = %s", r, got)
		}
	}
	if roleFrom("OPT-PARTICIPANT") != calendar.RoleOptional {
		t.Error("OPT-PARTICIPANT should map to optional")
	}
	if roleFrom("REQ-PARTICIPANT") != calendar.RoleRequired {
		t.Error("REQ-PARTICIPANT should map to required")
	}
	if transparency(calendar.ShowFree) != "TRANSPARENT" {
		t.Error("free should map to TRANSPARENT")
	}
	if unMailto(mailto("a@b.com")) != "a@b.com" {
		t.Error("mailto round trip failed")
	}
}

// TestApplyStatuses confirms external responses override matching attendees by
// case-insensitive email and leave others untouched.
func TestApplyStatuses(t *testing.T) {
	t.Parallel()
	h := calendar.Hold{Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "Boss@X.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseNotResponded},
		{Person: calendar.Person{Email: "peer@x.com"}, Role: calendar.RoleOptional, Response: calendar.ResponseNotResponded},
	}}
	applyStatuses(&h, map[string]calendar.Response{"boss@x.com": calendar.ResponseAccepted})
	if h.Attendees[0].Response != calendar.ResponseAccepted {
		t.Errorf("boss response = %q, want accepted", h.Attendees[0].Response)
	}
	if h.Attendees[1].Response != calendar.ResponseNotResponded {
		t.Errorf("peer response changed unexpectedly to %q", h.Attendees[1].Response)
	}
}

// TestIsEventPath confirms only single event objects are considered deletable.
func TestIsEventPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   string
		Want bool
	}{{ // Test 0: An event object is deletable.
		In: "/calendars/home/abc123.ics", Want: true,
	}, { // Test 1: A collection path is not.
		In: "/calendars/home/", Want: false,
	}, { // Test 2: A path without the event suffix is not.
		In: "/calendars/home", Want: false,
	}, { // Test 3: Empty is not.
		In: "", Want: false,
	}, { // Test 4: The root is not.
		In: "/", Want: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := isEventPath(test.In); got != test.Want {
				t.Errorf("isEventPath(%q) = %v, want %v", test.In, got, test.Want)
			}
		})
	}
}

// TestDeleteHoldRefusesCollection confirms DeleteHold rejects non-event paths before
// touching the server, so it can never wipe a calendar.
func TestDeleteHoldRefusesCollection(t *testing.T) {
	t.Parallel()
	p := &Provider{} // nil client: the guard must return before any request.
	for _, bad := range []string{"", "/calendars/home/", "/calendars/home"} {
		if err := p.DeleteHold(context.Background(), bad); err == nil {
			t.Errorf("DeleteHold(%q) should be refused", bad)
		}
	}
}
