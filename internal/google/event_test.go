package google

import (
	"fmt"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

func TestResponseFromGoogle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult calendar.Response
	}{{ // Test 0: Needs action maps to not responded.
		In: "needsAction", WantResult: calendar.ResponseNotResponded,
	}, { // Test 1: Accepted maps to accepted.
		In: "accepted", WantResult: calendar.ResponseAccepted,
	}, { // Test 2: Declined maps to declined.
		In: "declined", WantResult: calendar.ResponseDeclined,
	}, { // Test 3: Tentative maps to tentative.
		In: "tentative", WantResult: calendar.ResponseTentative,
	}, { // Test 4: Empty maps to none.
		In: "", WantResult: calendar.ResponseNone,
	}, { // Test 5: Unknown maps to none.
		In: "mystery", WantResult: calendar.ResponseNone,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := responseFromGoogle(test.In); got != test.WantResult {
				t.Errorf("responseFromGoogle(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

func TestTransparencyRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         calendar.ShowAs
		WantResult calendar.ShowAs
	}{{ // Test 0: Free stays free.
		In: calendar.ShowFree, WantResult: calendar.ShowFree,
	}, { // Test 1: Busy stays busy.
		In: calendar.ShowBusy, WantResult: calendar.ShowBusy,
	}, { // Test 2: Tentative collapses to busy, Google's only non-free state.
		In: calendar.ShowTentative, WantResult: calendar.ShowBusy,
	}, { // Test 3: Out of office collapses to busy.
		In: calendar.ShowOOF, WantResult: calendar.ShowBusy,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := showAsFromTransparency(transparencyFromShowAs(test.In)); got != test.WantResult {
				t.Errorf("round trip(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

func TestRoleOptionalRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In calendar.Role
	}{{ // Test 0: Required.
		In: calendar.RoleRequired,
	}, { // Test 1: Optional.
		In: calendar.RoleOptional,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := roleFromOptional(optionalFromRole(test.In)); got != test.In {
				t.Errorf("round trip = %q, want %q", got, test.In)
			}
		})
	}
}

func TestToGoogleEventOOFIsBusyBlock(t *testing.T) {
	t.Parallel()
	p := NewProvider(staticToken)
	// Google's outOfOffice type is Workspace-only, so an OOF hold maps to a plain
	// busy (opaque) event with no special type, working on any account.
	ev := p.toGoogleEvent(calendar.Hold{ShowAs: calendar.ShowOOF})
	if ev.EventType != "" {
		t.Errorf("eventType = %q, want empty (no Workspace-only type)", ev.EventType)
	}
	if ev.Transparency != "opaque" {
		t.Errorf("transparency = %q, want opaque (busy)", ev.Transparency)
	}
}

func TestFromGoogleEventReadsDatesAndOOF(t *testing.T) {
	t.Parallel()
	ev := googleEvent{
		ID:        "e1",
		Summary:   "away",
		Start:     &googleEventDateTime{Date: "2026-07-20"},
		End:       &googleEventDateTime{Date: "2026-07-25"},
		EventType: "outOfOffice",
	}
	h := fromGoogleEvent(ev)
	if !h.AllDay {
		t.Error("want all-day")
	}
	if h.ShowAs != calendar.ShowOOF {
		t.Errorf("showAs = %q, want oof (from eventType)", h.ShowAs)
	}
	if got := h.Start.Format("2006-01-02"); got != "2026-07-20" {
		t.Errorf("start = %q, want 2026-07-20", got)
	}
	if got := h.End.Format("2006-01-02"); got != "2026-07-25" {
		t.Errorf("end = %q, want 2026-07-25", got)
	}
}

// TestBoundaryConvertsTimedToProviderZone confirms a timed boundary is converted into
// the provider zone before formatting, so an input carrying a different offset keeps its
// instant instead of being relabeled and silently shifted.
func TestBoundaryConvertsTimedToProviderZone(t *testing.T) {
	t.Parallel()
	p := NewProvider(staticToken) // provider zone defaults to UTC
	start := time.Date(2026, 8, 3, 9, 0, 0, 0, time.FixedZone("CDT", -5*3600))
	got := p.boundary(start, false)
	if got.DateTime != "2026-08-03T14:00:00" {
		t.Errorf("timed dateTime = %q, want 2026-08-03T14:00:00 (09:00-05:00 in UTC)", got.DateTime)
	}
	if got.TimeZone != "UTC" {
		t.Errorf("timeZone = %q, want UTC", got.TimeZone)
	}
	// An all-day boundary names a calendar day and must not be zone-converted.
	if day := p.boundary(start, true); day.Date != "2026-08-03" {
		t.Errorf("all-day date = %q, want 2026-08-03", day.Date)
	}
}
