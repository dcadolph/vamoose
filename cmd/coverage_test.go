package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/coverage"
)

// TestForgetCoverage confirms the cmd wrapper resolves the ledger from
// VAMOOSE_COVERAGE_FILE and removes a hold, the path a declined or canceled hold takes.
func TestForgetCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.json")
	t.Setenv("VAMOOSE_COVERAGE_FILE", path)

	ledger := coverage.NewLedger(path)
	start := time.Date(2999, 8, 3, 0, 0, 0, 0, time.UTC)
	if err := ledger.Record(coverage.Entry{Owner: "me@x.com", HoldID: "h1", Start: start, End: start.AddDate(0, 0, 2)}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	forgetCoverage("h1")

	got, err := ledger.Overlapping(start, start.AddDate(0, 0, 2), "")
	if err != nil {
		t.Fatalf("Overlapping: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("after forget = %+v, want empty", got)
	}
}
