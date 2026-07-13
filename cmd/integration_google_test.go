package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/google"
)

// TestGoogleApprovalFlow drives the flagship flow end to end against a stateful
// in-memory Google Calendar: create a hold, poll while pending, flip the manager
// to accepted, then auto-promote the configured team. Google has no directory,
// so the team comes from config. No account required.
func TestGoogleApprovalFlow(t *testing.T) {
	isolateConfig(t)

	if err := saveTeamConfig([]string{"peer@x.com"}); err != nil {
		t.Fatalf("saveTeamConfig: %v", err)
	}

	type att struct {
		Email    string
		Optional bool
	}
	var (
		mu       sync.Mutex
		stored   []att
		accepted bool
	)
	emit := func(w http.ResponseWriter) {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		b.WriteString(`{"id":"evt-1","summary":"beach","transparency":"transparent","attendees":[`)
		for i, a := range stored {
			status := "needsAction"
			if accepted && !a.Optional {
				status = "accepted"
			}
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"email":%q,"optional":%t,"responseStatus":%q}`, a.Email, a.Optional, status)
		}
		b.WriteString(`]}`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, b.String())
	}
	store := func(r *http.Request) {
		var body struct {
			Attendees []struct {
				Email    string `json:"email"`
				Optional bool   `json:"optional"`
			} `json:"attendees"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		stored = stored[:0]
		for _, a := range body.Attendees {
			stored = append(stored, att{Email: a.Email, Optional: a.Optional})
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/calendars/primary/events":
			store(r)
			emit(w)
		case r.Method == http.MethodGet && r.URL.Path == "/calendars/primary/events/evt-1":
			emit(w)
		case r.Method == http.MethodPatch && r.URL.Path == "/calendars/primary/events/evt-1":
			store(r)
			emit(w)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prov := google.NewProvider(
		func(context.Context) (string, error) { return "test-token", nil },
		google.WithBaseURL(srv.URL),
	)
	ctx := context.Background()

	created, err := prov.CreateHold(ctx, calendar.Hold{
		Subject: "beach",
		ShowAs:  calendar.ShowFree,
		Attendees: []calendar.Attendee{
			{Person: calendar.Person{Email: "mgr@x.com"}, Role: calendar.RoleRequired},
		},
	})
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if created.ID != "evt-1" {
		t.Fatalf("created id = %q, want evt-1", created.ID)
	}

	item := watchItem{Provider: "google", HoldID: "evt-1", Workflow: "pto", Step: 1}
	if res, _, _ := advanceRun(ctx, prov, item); res != pollPending {
		t.Fatalf("before approval = %v, want pending", res)
	}

	mu.Lock()
	accepted = true
	mu.Unlock()

	res, _, err := advanceRun(ctx, prov, item)
	if err != nil {
		t.Fatalf("advanceRun: %v", err)
	}
	if res != pollApproved {
		t.Fatalf("after approval = %v, want approved", res)
	}

	mu.Lock()
	defer mu.Unlock()
	var peerOptional bool
	for _, a := range stored {
		if a.Email == "peer@x.com" && a.Optional {
			peerOptional = true
		}
	}
	if !peerOptional {
		t.Errorf("peer not promoted as optional: %+v", stored)
	}
}
