package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/coverage"
)

// runCoverage reports who has time off booked through vamoose over a window, so a person
// or a manager can see team coverage before booking or approving more.
func runCoverage(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	var (
		start = fs.String("start", "", "Explicit start date; overrides the phrase")
		end   = fs.String("end", "", "Explicit end date; overrides the phrase")
	)
	phraseWords, flagArgs := splitPhrase(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	phrase := strings.Join(phraseWords, " ")
	if phrase == "" && fs.NArg() > 0 {
		phrase = strings.Join(fs.Args(), " ")
	}
	if phrase == "" && *start == "" {
		phrase = "next week"
	}
	startAt, endAt, _, err := resolveWindow(*start, *end, phrase)
	if err != nil {
		return fmt.Errorf("coverage: %w", err)
	}
	ledger, err := coverageLedger()
	if err != nil {
		return err
	}
	entries, err := ledger.Overlapping(startAt, endAt, "")
	if err != nil {
		return fmt.Errorf("coverage: %w", err)
	}
	label := fmt.Sprintf("%s through %s", startAt.Format("Mon 2006-01-02"), endAt.AddDate(0, 0, -1).Format("Mon 2006-01-02"))
	if len(entries) == 0 {
		fmt.Fprintf(os.Stdout, "No one is off %s.\n", label)
		return nil
	}
	fmt.Fprintf(os.Stdout, "Off %s:\n", label)
	for _, e := range entries {
		fmt.Fprintf(os.Stdout, "  %-28s %s to %s  %s\n",
			e.Owner, e.Start.Format("2006-01-02"), e.End.AddDate(0, 0, -1).Format("2006-01-02"), e.Subject)
	}
	return nil
}

// coverageLedgerPath returns the coverage ledger file, honoring VAMOOSE_COVERAGE_FILE so a
// hosted or per-user deployment can point at its own file.
func coverageLedgerPath() (string, error) {
	if p := os.Getenv("VAMOOSE_COVERAGE_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "coverage.json"), nil
}

// coverageLedger returns the coverage ledger.
func coverageLedger() (*coverage.Ledger, error) {
	path, err := coverageLedgerPath()
	if err != nil {
		return nil, err
	}
	return coverage.NewLedger(path), nil
}

// recordCoverage adds a created hold to the coverage ledger, best-effort, so team coverage
// checks see time off booked through vamoose. A failure is not fatal to the request.
func recordCoverage(ctx context.Context, prov calendar.Provider, hold calendar.Hold) {
	me, err := prov.Me(ctx)
	if err != nil || me.Email == "" {
		return
	}
	ledger, err := coverageLedger()
	if err != nil {
		return
	}
	_ = ledger.Record(coverage.Entry{
		Owner: me.Email, HoldID: hold.ID, Start: hold.Start, End: hold.End, Subject: hold.Subject,
	})
}

// forgetCoverage removes a hold from the coverage ledger, best-effort, when it is
// canceled so it no longer counts against coverage.
func forgetCoverage(holdID string) {
	ledger, err := coverageLedger()
	if err != nil {
		return
	}
	_ = ledger.Remove(holdID)
}

// warnCoverage prints a heads-up when the number of people already off in the window
// meets or exceeds VAMOOSE_MAX_TEAM_OFF. It is best-effort and never blocks a request.
func warnCoverage(start, end time.Time) {
	limit, err := strconv.Atoi(os.Getenv("VAMOOSE_MAX_TEAM_OFF"))
	if err != nil || limit <= 0 {
		return
	}
	ledger, err := coverageLedger()
	if err != nil {
		return
	}
	n, err := ledger.CountOff(start, end, "")
	if err != nil || n < limit {
		return
	}
	fmt.Fprintf(os.Stdout, "Heads up: %d already have time off overlapping this window (limit %d).\n", n, limit)
}
