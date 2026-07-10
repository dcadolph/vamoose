package cmd

import (
	"context"
	"io"
	"log"
	"testing"
	"time"
)

// TestScheduleStore exercises the schedule list on disk against an isolated config
// directory. It cannot run in parallel because it sets process environment variables.
func TestScheduleStore(t *testing.T) {
	isolateConfig(t)
	if s, err := loadSchedules(); err != nil || s != nil {
		t.Fatalf("empty load = %v, %v; want nil, nil", s, err)
	}
	items := []scheduleItem{{Workflow: "pto", Every: "168h", NextRun: time.Unix(1000, 0), Phrase: "next week"}}
	if err := saveSchedules(items); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadSchedules()
	if err != nil || len(got) != 1 {
		t.Fatalf("load = %+v, %v; want one item", got, err)
	}
	if got[0].Workflow != "pto" || got[0].Phrase != "next week" || got[0].ParsedEvery() != 168*time.Hour {
		t.Errorf("loaded schedule = %+v", got[0])
	}
}

// TestFireSchedules confirms a due schedule runs and advances to its next interval,
// while a future one is left alone.
func TestFireSchedules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	var ran []string
	runner := func(_ context.Context, s scheduleItem) error {
		ran = append(ran, s.Workflow)
		return nil
	}
	logger := log.New(io.Discard, "", 0)
	schedules := []scheduleItem{
		{Workflow: "due", Every: "168h", NextRun: now.Add(-time.Hour)},
		{Workflow: "future", Every: "24h", NextRun: now.Add(time.Hour)},
	}
	out := fireSchedules(context.Background(), now, schedules, runner, logger)

	if len(ran) != 1 || ran[0] != "due" {
		t.Fatalf("ran = %v, want [due]", ran)
	}
	if !out[0].NextRun.Equal(now.Add(168 * time.Hour)) {
		t.Errorf("due next run = %v, want now+168h", out[0].NextRun)
	}
	if !out[1].NextRun.Equal(now.Add(time.Hour)) {
		t.Errorf("future next run = %v, want it unchanged", out[1].NextRun)
	}
}

// TestFireSchedulesAdvancesOnError confirms a failing run still advances the schedule,
// so a persistent error does not fire every poll.
func TestFireSchedulesAdvancesOnError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	runner := func(context.Context, scheduleItem) error { return context.DeadlineExceeded }
	logger := log.New(io.Discard, "", 0)
	schedules := []scheduleItem{{Workflow: "flaky", Every: "1h", NextRun: now.Add(-time.Minute)}}
	out := fireSchedules(context.Background(), now, schedules, runner, logger)
	if !out[0].NextRun.Equal(now.Add(time.Hour)) {
		t.Errorf("next run = %v, want now+1h even after an error", out[0].NextRun)
	}
}
