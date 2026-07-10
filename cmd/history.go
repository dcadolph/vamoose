package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dcadolph/vamoose/internal/audit"
)

// runHistory prints the recorded run history: what each hold did, when, and who approved
// it. It reads the audit log the executor and daemon write.
func runHistory(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	var (
		hold   = fs.String("hold", "", "Show only events for this hold id")
		limit  = fs.Int("limit", 0, "Show only the most recent N events (0 shows all)")
		asJSON = fs.Bool("json", false, "Emit the events as JSON")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := auditStore()
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	events, err := store.Events()
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if *hold != "" {
		events = eventsForHold(events, *hold)
	}
	if *limit > 0 && len(events) > *limit {
		events = events[len(events)-*limit:]
	}
	if *asJSON {
		if events == nil {
			events = []audit.Event{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}
	if len(events) == 0 {
		fmt.Fprintln(os.Stdout, "No run history yet.")
		return nil
	}
	for _, e := range events {
		printEvent(os.Stdout, e)
	}
	return nil
}

// eventsForHold returns the events belonging to the given hold id.
func eventsForHold(events []audit.Event, holdID string) []audit.Event {
	out := make([]audit.Event, 0, len(events))
	for _, e := range events {
		if e.HoldID == holdID {
			out = append(out, e)
		}
	}
	return out
}

// printEvent writes one history line: local time, action, workflow, hold, and the actor
// or, for a system step, its detail.
func printEvent(w io.Writer, e audit.Event) {
	who := e.Actor
	if who == "" {
		who = e.Detail
	}
	fmt.Fprintf(w, "%s  %-9s  %-14s  %-16s  %s\n",
		e.Time.Local().Format("2006-01-02 15:04:05"), e.Action, e.Workflow, e.HoldID, who)
}
