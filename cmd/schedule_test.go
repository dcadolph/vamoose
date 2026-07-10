package cmd

import (
	"context"
	"fmt"
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

// TestScheduleAddRemove drives the schedule command: add validates the workflow and
// interval, list persists, and remove drops by index. It isolates the config dir.
func TestScheduleAddRemove(t *testing.T) {
	isolateConfig(t)
	if err := scheduleAdd(context.Background(), []string{"pto", "--every", "168h", "--phrase", "next week", "--manager", "boss@x.com"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	items, _ := loadSchedules()
	if len(items) != 1 || items[0].Workflow != "pto" || items[0].Phrase != "next week" {
		t.Fatalf("schedules = %+v", items)
	}
	// A missing interval fails.
	if err := scheduleAdd(context.Background(), []string{"pto", "--phrase", "today"}); err == nil {
		t.Error("want an error without --every")
	}
	// A missing phrase fails.
	if err := scheduleAdd(context.Background(), []string{"pto", "--every", "24h"}); err == nil {
		t.Error("want an error without --phrase")
	}
	// An unknown workflow fails.
	if err := scheduleAdd(context.Background(), []string{"ghost", "--every", "1h", "--phrase", "today"}); err == nil {
		t.Error("want an error for an unknown workflow")
	}
	// Remove the one schedule.
	if err := scheduleRemove([]string{"0"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := loadSchedules(); len(got) != 0 {
		t.Errorf("after remove = %d, want 0", len(got))
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

// TestPollSchedules confirms the daemon fires a due schedule, and that a run against an
// unbuildable provider still advances the schedule instead of retrying every poll.
func TestPollSchedules(t *testing.T) {
	isolateConfig(t)
	past := time.Now().Add(-time.Hour)
	if err := saveSchedules([]scheduleItem{{
		Workflow: "pto", Provider: "no-such-provider", Every: "168h",
		NextRun: past, Phrase: "next week", Manager: "boss@x.com",
	}}); err != nil {
		t.Fatal(err)
	}
	pollSchedules(context.Background(), log.New(io.Discard, "", 0))
	got, _ := loadSchedules()
	if len(got) != 1 {
		t.Fatalf("schedules = %d, want 1", len(got))
	}
	if !got[0].NextRun.After(past.Add(time.Hour)) {
		t.Errorf("next run = %v, want it advanced past the fire time", got[0].NextRun)
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

// TestScheduleGuards covers the interval floor, duplicate rejection, and the count cap
// that keep an automated caller from flooding the daemon. It isolates the config dir.
func TestScheduleGuards(t *testing.T) {
	isolateConfig(t)
	// A sub-minute interval is rejected.
	if err := scheduleAdd(context.Background(), []string{"pto", "--every", "30s", "--phrase", "today"}); err == nil {
		t.Error("want an error for a sub-minute interval")
	}
	// An identical schedule is rejected the second time.
	item := scheduleItem{Workflow: "pto", Every: "168h", Phrase: "next week"}
	if err := addSchedule(item); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := addSchedule(item); err == nil {
		t.Error("want an error adding an identical schedule")
	}
	// The count is capped.
	full := make([]scheduleItem, maxSchedules)
	for i := range full {
		full[i] = scheduleItem{Workflow: fmt.Sprintf("w%d", i), Every: "168h", Phrase: "next week"}
	}
	if err := saveSchedules(full); err != nil {
		t.Fatal(err)
	}
	if err := addSchedule(scheduleItem{Workflow: "one-more", Every: "168h", Phrase: "next week"}); err == nil {
		t.Error("want an error when the schedule count is at the cap")
	}
}
