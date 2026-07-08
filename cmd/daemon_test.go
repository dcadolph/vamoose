package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// mockProvider is a calendar.Provider whose responses are set per test.
type mockProvider struct {
	// hold is returned by GetHold.
	hold calendar.Hold
	// getErr is returned by GetHold when set.
	getErr error
	// team is returned by Team for the promote path.
	team []calendar.Person
	// updated records the last hold passed to UpdateHold.
	updated *calendar.Hold
}

func (m *mockProvider) Me(context.Context) (calendar.Person, error) { return calendar.Person{}, nil }
func (m *mockProvider) Manager(context.Context) (calendar.Person, error) {
	return calendar.Person{}, nil
}
func (m *mockProvider) Team(context.Context) ([]calendar.Person, error) { return m.team, nil }

func (m *mockProvider) CreateHold(context.Context, calendar.Hold) (calendar.Hold, error) {
	return calendar.Hold{}, nil
}

func (m *mockProvider) GetHold(context.Context, string) (calendar.Hold, error) {
	return m.hold, m.getErr
}

func (m *mockProvider) UpdateHold(_ context.Context, h calendar.Hold) (calendar.Hold, error) {
	m.updated = &h
	return h, nil
}

func (m *mockProvider) DeleteHold(context.Context, string) error { return nil }

// managerHold builds a hold whose required attendee carries the given response.
func managerHold(resp calendar.Response) calendar.Hold {
	return calendar.Hold{Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "mgr@x.com"}, Role: calendar.RoleRequired, Response: resp},
	}}
}

func TestAdvanceHold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Hold    calendar.Hold
		GetErr  error
		WantRes pollResult
	}{{ // Test 0: Accepted approves.
		Hold: managerHold(calendar.ResponseAccepted), WantRes: pollApproved,
	}, { // Test 1: Declined.
		Hold: managerHold(calendar.ResponseDeclined), WantRes: pollDeclined,
	}, { // Test 2: Not responded pends.
		Hold: managerHold(calendar.ResponseNotResponded), WantRes: pollPending,
	}, { // Test 3: A fetch error fails.
		GetErr: errors.New("boom"), WantRes: pollFailed,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			prov := &mockProvider{hold: test.Hold, getErr: test.GetErr}
			got, _ := advanceHold(context.Background(), prov, "id", false)
			if got != test.WantRes {
				t.Errorf("advanceHold = %v, want %v", got, test.WantRes)
			}
			if prov.updated != nil {
				t.Error("UpdateHold called with autoPromote off")
			}
		})
	}
}

// TestAdvanceHoldPromotes confirms approval with autoPromote fans out the team.
// It cannot run in parallel because it isolates HOME to avoid team config.
func TestAdvanceHoldPromotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	prov := &mockProvider{
		hold: managerHold(calendar.ResponseAccepted),
		team: []calendar.Person{{Email: "peer@x.com"}},
	}
	res, err := advanceHold(context.Background(), prov, "id", true)
	if err != nil {
		t.Fatalf("advanceHold: %v", err)
	}
	if res != pollApproved {
		t.Errorf("res = %v, want approved", res)
	}
	if prov.updated == nil {
		t.Fatal("promote did not update the hold")
	}
	found := false
	for _, a := range prov.updated.Attendees {
		if a.Person.Email == "peer@x.com" && a.Role == calendar.RoleOptional {
			found = true
		}
	}
	if !found {
		t.Error("peer was not added as an optional attendee")
	}
}

// TestWatchStore exercises the watch list on disk against an isolated HOME.
// It cannot run in parallel because it sets process environment variables.
func TestWatchStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	if w, err := loadWatches(); err != nil || w != nil {
		t.Fatalf("loadWatches on empty = %v, %v; want nil, nil", w, err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e1", AutoPromote: true}); err != nil {
		t.Fatalf("addWatch: %v", err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e2"}); err != nil {
		t.Fatalf("addWatch: %v", err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e1", Subject: "replaced"}); err != nil {
		t.Fatalf("addWatch replace: %v", err)
	}
	got, err := loadWatches()
	if err != nil {
		t.Fatalf("loadWatches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("count = %d, want 2", len(got))
	}
	for _, w := range got {
		if w.HoldID == "e1" && w.Subject != "replaced" {
			t.Errorf("e1 subject = %q, want replaced", w.Subject)
		}
	}
}
