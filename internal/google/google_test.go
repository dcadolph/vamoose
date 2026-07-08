package google

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// staticToken is a TokenSource that always returns the same token.
func staticToken(_ context.Context) (string, error) { return "test-token", nil }

func TestProviderCreateHold(t *testing.T) {
	t.Parallel()
	var gotAuth, gotMethod, gotPath, gotSend string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSend = r.URL.Query().Get("sendUpdates")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"evt123","summary":"Beach","transparency":"transparent",` +
			`"attendees":[{"email":"mgr@x.com","displayName":"Mgr","responseStatus":"needsAction"}]}`))
	}))
	defer srv.Close()

	p := NewProvider(staticToken, WithBaseURL(srv.URL))
	hold := calendar.Hold{
		Subject: "Beach",
		ShowAs:  calendar.ShowFree,
		Attendees: []calendar.Attendee{
			{Person: calendar.Person{Email: "mgr@x.com", Name: "Mgr"}, Role: calendar.RoleRequired},
		},
	}
	got, err := p.CreateHold(context.Background(), hold)
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/calendars/primary/events" {
		t.Errorf("request = %s %s, want POST /calendars/primary/events", gotMethod, gotPath)
	}
	if gotSend != "all" {
		t.Errorf("sendUpdates = %q, want all", gotSend)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if got.ID != "evt123" {
		t.Errorf("ID = %q, want evt123", got.ID)
	}
	if len(got.Attendees) != 1 {
		t.Fatalf("attendees = %+v, want one", got.Attendees)
	}
	if got.Attendees[0].Role != calendar.RoleRequired {
		t.Errorf("role = %q, want required", got.Attendees[0].Role)
	}
	if got.Attendees[0].Response != calendar.ResponseNotResponded {
		t.Errorf("response = %q, want notResponded", got.Attendees[0].Response)
	}
}

func TestProviderManagerUnsupported(t *testing.T) {
	t.Parallel()
	p := NewProvider(staticToken)
	if _, err := p.Manager(context.Background()); !errors.Is(err, calendar.ErrNoManager) {
		t.Errorf("Manager err = %v, want ErrNoManager", err)
	}
}

func TestProviderTeamUnsupported(t *testing.T) {
	t.Parallel()
	p := NewProvider(staticToken)
	if _, err := p.Team(context.Background()); !errors.Is(err, calendar.ErrNoDirectory) {
		t.Errorf("Team err = %v, want ErrNoDirectory", err)
	}
}

func TestProviderGetHoldNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not Found"}}`))
	}))
	defer srv.Close()

	p := NewProvider(staticToken, WithBaseURL(srv.URL))
	_, err := p.GetHold(context.Background(), "missing")
	if !errors.Is(err, calendar.ErrNotFound) {
		t.Fatalf("GetHold err = %v, want ErrNotFound", err)
	}
}

func TestProviderDeleteHold(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath, gotSend string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSend = r.URL.Query().Get("sendUpdates")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := NewProvider(staticToken, WithBaseURL(srv.URL))
	if err := p.DeleteHold(context.Background(), "evt123"); err != nil {
		t.Fatalf("DeleteHold: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/calendars/primary/events/evt123" {
		t.Errorf("request = %s %s, want DELETE /calendars/primary/events/evt123", gotMethod, gotPath)
	}
	if gotSend != "all" {
		t.Errorf("sendUpdates = %q, want all", gotSend)
	}
}
