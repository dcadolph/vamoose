package eventkit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// skipOnWindows skips tests that fake the helper with a shell script, which does not
// execute on Windows. The helper itself is a macOS bridge, so nothing is lost.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake helper is a shell script; the real helper is macOS-only")
	}
}

// TestStatus runs Status against a fake helper script and checks the mapping.
func TestStatus(t *testing.T) {
	skipOnWindows(t)
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

// TestStatusDuplicatePrefersStronger confirms that when the same attendee appears twice,
// from a uid matched in two calendars, the stronger response wins regardless of order, so
// a stale non-reply cannot mask a real accept.
func TestStatusDuplicatePrefersStronger(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	helper := filepath.Join(dir, "vamoose-eventkit")
	// boss appears pending-then-accepted; peer appears accepted-then-pending.
	script := "#!/bin/sh\n" +
		`printf '{"attendees":[{"email":"boss@x.com","status":"pending"},{"email":"boss@x.com","status":"accepted"},{"email":"peer@x.com","status":"accepted"},{"email":"peer@x.com","status":"pending"}]}'` + "\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("VAMOOSE_EVENTKIT_HELPER", helper)

	got, err := Status(context.Background(), "uid-1")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got["boss@x.com"] != calendar.ResponseAccepted {
		t.Errorf("boss = %q, want accepted (stronger wins over pending)", got["boss@x.com"])
	}
	if got["peer@x.com"] != calendar.ResponseAccepted {
		t.Errorf("peer = %q, want accepted (order independent)", got["peer@x.com"])
	}
}

// TestStatusHelperMissing confirms a missing helper errors rather than panicking.
func TestStatusHelperMissing(t *testing.T) {
	t.Setenv("VAMOOSE_EVENTKIT_HELPER", filepath.Join(t.TempDir(), "nope"))
	if _, err := Status(context.Background(), "uid"); err == nil {
		t.Error("want error for a missing helper")
	}
}
