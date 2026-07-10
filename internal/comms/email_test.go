package comms

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
)

// TestEmailNotify confirms an email carries the recipient, subject, and body, and uses
// the default port.
func TestEmailNotify(t *testing.T) {
	t.Parallel()
	n := NewEmailNotifier(EmailConfig{Host: "smtp.example.com", From: "vamoose@x.com", Username: "u", Password: "p"})
	var gotAddr, gotFrom, gotMsg string
	var gotTo []string
	n.send = func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, string(msg)
		return nil
	}
	if err := n.Notify(context.Background(), "team@x.com", "Alice is out next week"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotAddr != "smtp.example.com:587" {
		t.Errorf("addr = %q, want the default port 587", gotAddr)
	}
	if gotFrom != "vamoose@x.com" || len(gotTo) != 1 || gotTo[0] != "team@x.com" {
		t.Errorf("from/to = %q/%v", gotFrom, gotTo)
	}
	for _, want := range []string{"To: team@x.com", "Subject: Alice is out next week", "Alice is out next week"} {
		if !strings.Contains(gotMsg, want) {
			t.Errorf("message missing %q:\n%s", want, gotMsg)
		}
	}
}

// TestEmailNotifyEmptyRecipient confirms an empty recipient fails before any send.
func TestEmailNotifyEmptyRecipient(t *testing.T) {
	t.Parallel()
	n := NewEmailNotifier(EmailConfig{Host: "h", From: "f@x.com"})
	called := false
	n.send = func(string, smtp.Auth, string, []string, []byte) error { called = true; return nil }
	if err := n.Notify(context.Background(), "", "hi"); err == nil {
		t.Error("want an error for an empty recipient")
	}
	if called {
		t.Error("send should not run for an empty recipient")
	}
}

// TestRoute confirms channels route by shape: an address to email, otherwise Slack, and
// a missing backend errors.
func TestRoute(t *testing.T) {
	t.Parallel()
	var slackCh, emailCh string
	slack := NotifierFunc(func(_ context.Context, ch, _ string) error { slackCh = ch; return nil })
	email := NotifierFunc(func(_ context.Context, ch, _ string) error { emailCh = ch; return nil })
	r := Route(slack, email)

	if err := r.Notify(context.Background(), "#team", "x"); err != nil {
		t.Fatalf("slack route: %v", err)
	}
	if slackCh != "#team" || emailCh != "" {
		t.Errorf("slack route: slack=%q email=%q, want slack #team", slackCh, emailCh)
	}
	slackCh, emailCh = "", ""
	if err := r.Notify(context.Background(), "person@x.com", "x"); err != nil {
		t.Fatalf("email route: %v", err)
	}
	if emailCh != "person@x.com" || slackCh != "" {
		t.Errorf("email route: slack=%q email=%q, want email person@x.com", slackCh, emailCh)
	}
	if err := Route(nil, email).Notify(context.Background(), "#team", "x"); err == nil {
		t.Error("want an error when Slack is not configured")
	}
	if err := Route(slack, nil).Notify(context.Background(), "a@x.com", "x"); err == nil {
		t.Error("want an error when email is not configured")
	}
}
