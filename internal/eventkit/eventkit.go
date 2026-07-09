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
		if a.Email != "" {
			m[strings.ToLower(a.Email)] = responseFrom(a.Status)
		}
	}
	return m, nil
}

// helperPath locates the helper binary: an explicit override, next to the running
// executable, or on PATH.
func helperPath() (string, error) {
	if p := os.Getenv("VAMOOSE_EVENTKIT_HELPER"); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), helperName)
		if _, statErr := os.Stat(cand); statErr == nil {
			return cand, nil
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
