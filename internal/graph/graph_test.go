package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// staticToken is a TokenSource that always returns the same token.
func staticToken(_ context.Context) (string, error) { return "test-token", nil }

func TestProviderCreateHold(t *testing.T) {
	t.Parallel()
	var gotAuth, gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"evt123","subject":"Beach","showAs":"free",` +
			`"attendees":[{"emailAddress":{"address":"mgr@x.com","name":"Mgr"},"type":"required","status":{"response":"none"}}]}`))
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
	if gotMethod != http.MethodPost || gotPath != "/me/events" {
		t.Errorf("request = %s %s, want POST /me/events", gotMethod, gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if got.ID != "evt123" {
		t.Errorf("ID = %q, want evt123", got.ID)
	}
	if len(got.Attendees) != 1 || got.Attendees[0].Response != calendar.ResponseNone {
		t.Errorf("attendees = %+v, want one with response none", got.Attendees)
	}
}

func TestProviderTeamExcludesSelf(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/me/manager/directReports"):
			_, _ = w.Write([]byte(`{"value":[` +
				`{"displayName":"Me","mail":"me@x.com"},` +
				`{"displayName":"Peer","mail":"peer@x.com"}]}`))
		case r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"displayName":"Me","mail":"me@x.com"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewProvider(staticToken, WithBaseURL(srv.URL))
	team, err := p.Team(context.Background())
	if err != nil {
		t.Fatalf("Team: %v", err)
	}
	if len(team) != 1 || team[0].Email != "peer@x.com" {
		t.Errorf("team = %+v, want only peer@x.com", team)
	}
}

func TestProviderGetHoldNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound","message":"not found"}}`))
	}))
	defer srv.Close()

	p := NewProvider(staticToken, WithBaseURL(srv.URL))
	_, err := p.GetHold(context.Background(), "missing")
	if err == nil {
		t.Fatal("GetHold: want error, got nil")
	}
}
