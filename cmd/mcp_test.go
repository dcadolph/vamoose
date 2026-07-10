package cmd

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dcadolph/vamoose/internal/mcp"
)

func TestRegisterTools(t *testing.T) {
	t.Parallel()
	srv := mcp.NewServer("vamoose", "test")
	registerTools(srv)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	var out strings.Builder
	if err := srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	want := []string{
		"whoami", "request_time_off", "time_off_status",
		"promote_to_team", "cancel_hold", "set_away", "create_event",
		"list_workflows", "preview_workflow", "run_workflow",
		"list_schedules", "schedule_workflow", "create_workflow",
	}
	for _, name := range want {
		if !strings.Contains(out.String(), `"name":"`+name+`"`) {
			t.Errorf("tools/list missing %q:\n%s", name, out.String())
		}
	}
}

// TestRunWorkflowArgs covers building the run command line for previewing and running a
// workflow from MCP tool arguments.
func TestRunWorkflowArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name    string
		Args    map[string]any
		Preview bool
		Want    []string
	}{{ // Test 0: A preview dry-runs with the phrase.
		Name: "preview", Args: map[string]any{"workflow": "pto", "phrase": "next week"}, Preview: true,
		Want: []string{"run", "pto", "next week", "--dry-run"},
	}, { // Test 1: A run with a manager and watch.
		Name: "run watched", Args: map[string]any{"workflow": "pto", "phrase": "next week", "manager": "b@x.com", "watch": true},
		Want: []string{"run", "pto", "next week", "--manager", "b@x.com", "--watch"},
	}, { // Test 2: Explicit dates and provider.
		Name: "dates", Args: map[string]any{"workflow": "away", "start": "2026-07-20", "end": "2026-07-24", "provider": "icloud"},
		Want: []string{"run", "away", "--start", "2026-07-20", "--end", "2026-07-24", "--provider", "icloud"},
	}, { // Test 3: A preview never watches, even when asked.
		Name: "preview ignores watch", Args: map[string]any{"workflow": "pto", "watch": true}, Preview: true,
		Want: []string{"run", "pto", "--dry-run"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := runWorkflowArgs(test.Args, test.Preview)
			if !reflect.DeepEqual(got, test.Want) {
				t.Errorf("%s: args = %v, want %v", test.Name, got, test.Want)
			}
		})
	}
}

// TestScheduleWorkflowArgs confirms the schedule-add command line is built from the
// tool arguments.
func TestScheduleWorkflowArgs(t *testing.T) {
	t.Parallel()
	got := scheduleWorkflowArgs(map[string]any{
		"workflow": "pto", "every": "168h", "phrase": "next week", "manager": "b@x.com", "provider": "google",
	})
	want := []string{"schedule", "add", "pto", "--every", "168h", "--phrase", "next week", "--manager", "b@x.com", "--provider", "google"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// TestMergeProps confirms schemas combine without mutating the shared base map.
func TestMergeProps(t *testing.T) {
	t.Parallel()
	base := map[string]any{"a": 1, "b": 2}
	got := mergeProps(base, map[string]any{"c": 3})
	if len(got) != 3 || got["a"] != 1 || got["c"] != 3 {
		t.Errorf("merge = %v, want a b c", got)
	}
	if len(base) != 2 {
		t.Error("merge mutated the base map")
	}
}
