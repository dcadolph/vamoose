package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/hris"
	"github.com/dcadolph/vamoose/internal/workflow"
)

// TestWalkStepsFilesLeave confirms a leave step files the hold's dates with the HR filer,
// carrying the configured employee and type and the step note. It sets environment, so it
// is not parallel.
func TestWalkStepsFilesLeave(t *testing.T) {
	isolateConfig(t)
	t.Setenv("VAMOOSE_BAMBOOHR_EMPLOYEE_ID", "E1")
	t.Setenv("VAMOOSE_BAMBOOHR_TYPE_ID", "78")
	wf := workflow.Workflow{Name: "leave", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbLeave, Subject: "Vacation", Next: "end"},
	}}
	var got hris.Leave
	deps := stepDeps{filer: hris.FilerFunc(func(_ context.Context, l hris.Leave) (string, error) {
		got = l
		return "req-9", nil
	})}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	hold := calendar.Hold{ID: "h1", Subject: "Out", Start: start, End: end}
	if _, err := walkSteps(context.Background(), &mockProvider{}, deps, "graph", wf, 1, hold); err != nil {
		t.Fatal(err)
	}
	if got.EmployeeID != "E1" || got.TypeID != "78" || got.Note != "Vacation" || !got.Start.Equal(start) || !got.End.Equal(end) {
		t.Errorf("filed leave = %+v, want E1/78/Vacation with the hold dates", got)
	}
}

// TestFileLeaveNoFiler confirms a leave step errors when no HR system is configured,
// rather than silently skipping.
func TestFileLeaveNoFiler(t *testing.T) {
	t.Parallel()
	if err := fileLeave(context.Background(), nil, workflow.Step{Verb: workflow.VerbLeave}, calendar.Hold{}); err == nil {
		t.Fatal("want an error when no HR system is configured")
	}
}
