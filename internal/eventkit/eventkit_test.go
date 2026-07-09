package eventkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TestStatus runs Status against a fake helper script and checks the mapping.
func TestStatus(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "vamoose-eventkit")
	script := "#!/bin/sh\n" +
		`printf '{"uid":"%s","attendees":[{"email":"Boss@X.com","status":"accepted"},{"email":"peer@x.com","status":"pending"}]}' "$2"` + "\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("VAMOOSE_EVENTKIT_HELPER", helper)

	got, err := Status(context.Background(), "uid-1")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got["boss@x.com"] != calendar.ResponseAccepted {
		t.Errorf("boss = %q, want accepted", got["boss@x.com"])
	}
	if got["peer@x.com"] != calendar.ResponseNotResponded {
		t.Errorf("peer (pending) = %q, want notResponded", got["peer@x.com"])
	}
}

// TestStatusHelperMissing confirms a missing helper errors rather than panicking.
func TestStatusHelperMissing(t *testing.T) {
	t.Setenv("VAMOOSE_EVENTKIT_HELPER", filepath.Join(t.TempDir(), "nope"))
	if _, err := Status(context.Background(), "uid"); err == nil {
		t.Error("want error for a missing helper")
	}
}
