package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dcadolph/vamoose/internal/audit"
)

// auditPath returns the run-history file location: VAMOOSE_AUDIT_FILE when set, which the
// Slack server uses to give each linked user their own history file, otherwise the
// default under the user config directory.
func auditPath() (string, error) {
	if p := os.Getenv("VAMOOSE_AUDIT_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "audit.json"), nil
}

// auditStore returns the run-history store, encrypted at rest when VAMOOSE_SECRET_KEY is
// set.
func auditStore() (*audit.FileStore, error) {
	path, err := auditPath()
	if err != nil {
		return nil, err
	}
	return audit.NewFileStore(path)
}

// resolveRecorder returns the run-history recorder, or a no-op recorder when the store
// cannot be built, so audit keeping never blocks a workflow.
func resolveRecorder() audit.Recorder {
	rec, err := auditStore()
	if err != nil {
		return audit.Nop
	}
	return rec
}

// recordAudit stamps and appends an event, logging but not failing on error, so keeping
// history never breaks a workflow.
func recordAudit(ctx context.Context, rec audit.Recorder, e audit.Event) {
	if rec == nil {
		return
	}
	e.Time = time.Now()
	if err := rec.Record(ctx, e); err != nil {
		fmt.Fprintf(os.Stderr, "vamoose: audit record failed: %v\n", err)
	}
}
