// Package eventkit reads calendar attendee responses from the local macOS
// Calendar.app through a Swift helper binary. iCloud does not report attendee
// accept/decline over CalDAV, but the Apple-synced local copy does, so on a Mac
// with calendar access granted this recovers approval detection.
package eventkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// helperName is the compiled Swift helper binary name.
const helperName = "vamoose-eventkit"

// helperBundlePath is the helper inside its macOS .app bundle. A signed bundle is
// its own TCC subject, so macOS prompts for and attaches calendar access to it
// rather than to the parent terminal.
const helperBundlePath = "vamoose-eventkit.app/Contents/MacOS/vamoose-eventkit"

// Status returns attendee responses for the event with the given iCalendar UID,
// keyed by lowercase email, read from the local Calendar.app.
func Status(ctx context.Context, uid string) (map[string]calendar.Response, error) {
	helper, err := helperPath()
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, helper, "status", uid).Output()
	if err != nil {
		return nil, fmt.Errorf("eventkit helper: %w", err)
	}
	var parsed struct {
		Attendees []struct {
			Email  string `json:"email"`
			Status string `json:"status"`
		} `json:"attendees"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("eventkit helper output: %w", err)
	}
	m := make(map[string]calendar.Response, len(parsed.Attendees))
	for _, a := range parsed.Attendees {
		if a.Email == "" {
			continue
		}
		key := strings.ToLower(a.Email)
		next := responseFrom(a.Status)
		// The same uid can match in more than one synced calendar, so an attendee may
		// appear twice. Keep the stronger response so a stale non-reply copy does not
		// overwrite a real accept or decline.
		if cur, ok := m[key]; !ok || responseRank(next) > responseRank(cur) {
			m[key] = next
		}
	}
	return m, nil
}

// responseRank orders responses so that when the same attendee appears under more than
// one matched calendar, a definitive reply beats a non-reply. A decline outranks an
// accept so a conflict is never read as approval.
func responseRank(r calendar.Response) int {
	switch r {
	case calendar.ResponseDeclined:
		return 4
	case calendar.ResponseAccepted:
		return 3
	case calendar.ResponseTentative:
		return 2
	case calendar.ResponseNotResponded:
		return 1
	default:
		return 0
	}
}

// helperPath locates the helper binary: an explicit override, next to the running
// executable, or on PATH.
func helperPath() (string, error) {
	if p := os.Getenv("VAMOOSE_EVENTKIT_HELPER"); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, cand := range []string{filepath.Join(dir, helperBundlePath), filepath.Join(dir, helperName)} {
			if _, statErr := os.Stat(cand); statErr == nil {
				return cand, nil
			}
		}
	}
	if p, err := exec.LookPath(helperName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("eventkit helper %q not found; build it with make eventkit", helperName)
}

// responseFrom maps an EventKit participant status word to a response.
func responseFrom(s string) calendar.Response {
	switch strings.ToLower(s) {
	case "accepted":
		return calendar.ResponseAccepted
	case "declined":
		return calendar.ResponseDeclined
	case "tentative":
		return calendar.ResponseTentative
	default:
		return calendar.ResponseNotResponded
	}
}
