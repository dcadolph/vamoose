package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/graph"
)

// TestGraphApprovalFlow drives the whole flagship flow end to end against a
// stateful in-memory Graph: create a hold, poll while pending, flip the manager
// to accepted, then auto-promote the directory team. It exercises the real
// provider, the daemon's advanceRun, and promoteHold together, with no account.
func TestGraphApprovalFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	type att struct{ Email, Type string }
	var (
		mu       sync.Mutex
		stored   []att
		accepted bool
	)
	emit := func(w http.ResponseWriter) {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		b.WriteString(`{"id":"evt-1","subject":"beach","showAs":"free","attendees":[`)
		for i, a := range stored {
			response := "none"
			if accepted && a.Type == "required" {
				response = "accepted"
			}
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"emailAddress":{"address":%q},"type":%q,"status":{"response":%q}}`, a.Email, a.Type, response)
		}
		b.WriteString(`]}`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, b.String())
	}
	store := func(r *http.Request) {
		var body struct {
			Attendees []struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
				Type string `json:"type"`
			} `json:"attendees"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		stored = stored[:0]
		for _, a := range body.Attendees {
			stored = append(stored, att{Email: a.EmailAddress.Address, Type: a.Type})
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/me/events":
			store(r)
			emit(w)
		case r.Method == http.MethodGet && r.URL.Path == "/me/events/evt-1":
			emit(w)
		case r.Method == http.MethodPatch && r.URL.Path == "/me/events/evt-1":
			store(r)
			emit(w)
		case r.URL.Path == "/me":
			_, _ = io.WriteString(w, `{"mail":"me@x.com"}`)
		case strings.HasPrefix(r.URL.Path, "/me/manager/directReports"):
			_, _ = io.WriteString(w, `{"value":[{"mail":"me@x.com"},{"mail":"peer@x.com"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prov := graph.NewProvider(
		func(context.Context) (string, error) { return "test-token", nil },
		graph.WithBaseURL(srv.URL),
	)
	ctx := context.Background()

	// request: create the hold with the manager as a required attendee.
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

	// daemon before approval: still pending, nothing promoted.
	item := watchItem{Provider: "graph", HoldID: "evt-1", Workflow: "pto", Step: 1}
	if res, _, _ := advanceRun(ctx, prov, item); res != pollPending {
		t.Fatalf("before approval = %v, want pending", res)
	}

	// manager accepts the invite.
	mu.Lock()
	accepted = true
	mu.Unlock()

	// daemon after approval: approved, and the directory team is fanned out.
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
		if a.Email == "peer@x.com" && a.Type == "optional" {
			peerOptional = true
		}
	}
	if !peerOptional {
		t.Errorf("peer was not promoted as optional: %+v", stored)
	}
}
