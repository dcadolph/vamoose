package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/dcadolph/vamoose/internal/mcp"
)

// runMCP serves vamoose as a Model Context Protocol server over stdio, letting a
// client such as Claude drive the commands as tools.
func runMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv := mcp.NewServer("vamoose", version)
	registerTools(srv)
	return srv.Serve(ctx, os.Stdin, os.Stdout)
}

// registerTools wires the vamoose commands onto the MCP server as tools. Each
// tool shells out to this binary so it shares one code path with the CLI.
func registerTools(srv *mcp.Server) {
	srv.Register(mcp.Tool{
		Name:        "whoami",
		Description: "Print the signed-in user, manager, and resolved team.",
		InputSchema: objectSchema(nil, map[string]any{
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			return execSelf(ctx, withProvider([]string{"whoami"}, m)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "request_time_off",
		Description: "Create a time-off hold shown as free and invite the manager to approve it.",
		InputSchema: objectSchema([]string{"start", "end", "subject"}, map[string]any{
			"start":    strProp("Start date as YYYY-MM-DD or an RFC3339 time"),
			"end":      strProp("End date as YYYY-MM-DD or an RFC3339 time"),
			"subject":  strProp("Event subject"),
			"manager":  strProp("Manager email; omit to resolve from the directory"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
			"watch":    boolProp("Auto-promote to the team once approved"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			args := []string{"request", "--start", argString(m, "start"), "--end", argString(m, "end"), "--subject", argString(m, "subject")}
			if v := argString(m, "manager"); v != "" {
				args = append(args, "--manager", v)
			}
			if argBool(m, "watch") {
				args = append(args, "--watch")
			}
			return execSelf(ctx, withProvider(args, m)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "time_off_status",
		Description: "Show whether the manager has approved the latest or given hold.",
		InputSchema: objectSchema(nil, map[string]any{
			"id":       strProp("Hold id; omit for the most recent hold"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, withID([]string{"check"}, parseArgs(raw))...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "promote_to_team",
		Description: "Add the team as optional attendees to an approved hold.",
		InputSchema: objectSchema(nil, map[string]any{
			"id":       strProp("Hold id; omit for the most recent hold"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, withID([]string{"promote"}, parseArgs(raw))...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "cancel_hold",
		Description: "Cancel a hold and notify its attendees.",
		InputSchema: objectSchema(nil, map[string]any{
			"id":       strProp("Hold id; omit for the most recent hold"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, withID([]string{"cancel"}, parseArgs(raw))...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "set_away",
		Description: "Mark yourself out of office over a date range, with no approval.",
		InputSchema: objectSchema([]string{"start", "end"}, map[string]any{
			"start":    strProp("Start date as YYYY-MM-DD or an RFC3339 time"),
			"end":      strProp("End date as YYYY-MM-DD or an RFC3339 time"),
			"subject":  strProp("Event subject (default: Out of office)"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			args := []string{"away", "--start", argString(m, "start"), "--end", argString(m, "end")}
			if v := argString(m, "subject"); v != "" {
				args = append(args, "--subject", v)
			}
			return execSelf(ctx, withProvider(args, m)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "leave_balance",
		Description: "Show remaining time off read from the configured HR system.",
		InputSchema: objectSchema(nil, map[string]any{
			"as_of": strProp("Date to check the balance as of, YYYY-MM-DD; omit for today"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			args := []string{"balance"}
			if v := argString(m, "as_of"); v != "" {
				args = append(args, "--as-of", v)
			}
			return execSelf(ctx, args...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "team_coverage",
		Description: "Show who already has time off booked in a window, before booking or approving more.",
		InputSchema: objectSchema(nil, map[string]any{
			"phrase": strProp("Date phrase such as \"next week\"; omit for next week"),
			"start":  strProp("Explicit start date as YYYY-MM-DD; overrides the phrase"),
			"end":    strProp("Explicit end date as YYYY-MM-DD; overrides the phrase"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			args := []string{"coverage"}
			if v := argString(m, "phrase"); v != "" {
				args = append(args, v)
			}
			if v := argString(m, "start"); v != "" {
				args = append(args, "--start", v)
			}
			if v := argString(m, "end"); v != "" {
				args = append(args, "--end", v)
			}
			return execSelf(ctx, args...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "create_event",
		Description: "Create a quick calendar event, optionally inviting attendees.",
		InputSchema: objectSchema([]string{"start", "end", "subject"}, map[string]any{
			"start":     strProp("Start date as YYYY-MM-DD or an RFC3339 time"),
			"end":       strProp("End date as YYYY-MM-DD or an RFC3339 time"),
			"subject":   strProp("Event subject"),
			"attendees": strProp("Comma-separated attendee emails to invite"),
			"free":      boolProp("Show the event as free instead of busy"),
			"provider":  strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			args := []string{"event", "--start", argString(m, "start"), "--end", argString(m, "end"), "--subject", argString(m, "subject")}
			if v := argString(m, "attendees"); v != "" {
				args = append(args, "--attendees", v)
			}
			if argBool(m, "free") {
				args = append(args, "--free")
			}
			return execSelf(ctx, withProvider(args, m)...)
		},
	})
	registerWorkflowTools(srv)
}

// registerWorkflowTools exposes the workflow engine to an agent: discover the available
// workflows, preview a run without touching the calendar, run any workflow, and manage
// recurring schedules. These make vamoose a workflow layer an agent can drive, beyond
// the fixed time-off tools.
func registerWorkflowTools(srv *mcp.Server) {
	srv.Register(mcp.Tool{
		Name:        "list_workflows",
		Description: "List every workflow that can be run, built-in and user-defined, with its description. Call this first to discover what run_workflow and preview_workflow can run.",
		InputSchema: objectSchema(nil, map[string]any{}),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return execSelf(ctx, "workflows")
		},
	})
	srv.Register(mcp.Tool{
		Name:        "preview_workflow",
		Description: "Show the plan a workflow would carry out for a date window, without creating or changing anything. Use this to explain what will happen before calling run_workflow.",
		InputSchema: objectSchema([]string{"workflow"}, workflowRunProps()),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, runWorkflowArgs(parseArgs(raw), true)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "run_workflow",
		Description: "Run a named workflow over a date phrase or explicit dates: it creates the hold and runs the steps up to any approval gate. Set watch so the daemon advances it once approved. Prefer preview_workflow first.",
		InputSchema: objectSchema([]string{"workflow"}, mergeProps(workflowRunProps(), map[string]any{
			"watch": boolProp("Watch for approval so the daemon advances the workflow once the manager accepts"),
		})),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, runWorkflowArgs(parseArgs(raw), false)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "list_schedules",
		Description: "List the recurring workflow schedules and when each next runs.",
		InputSchema: objectSchema(nil, map[string]any{}),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return execSelf(ctx, "schedule", "list")
		},
	})
	srv.Register(mcp.Tool{
		Name:        "schedule_workflow",
		Description: "Schedule a workflow to rerun on an interval, such as weekly. The daemon fires it, resolving the date phrase fresh each run. Intervals are Go durations, so weekly is 168h.",
		InputSchema: objectSchema([]string{"workflow", "every", "phrase"}, map[string]any{
			"workflow": strProp("Workflow name to rerun (see list_workflows)"),
			"every":    strProp("Interval between runs as a Go duration, e.g. 168h for weekly"),
			"phrase":   strProp("Date phrase resolved each run, e.g. \"next week\""),
			"subject":  strProp("Event subject"),
			"manager":  strProp("Approver email for a directory-less backend"),
			"provider": strProp("Calendar provider: graph, google, icloud, or caldav"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execSelf(ctx, scheduleWorkflowArgs(parseArgs(raw))...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "create_workflow",
		Description: "Create or replace a reusable user workflow from a JSON definition, so it can then be previewed and run by name. The definition is validated and rejected with the reason if invalid. Model it on the built-ins from list_workflows.",
		InputSchema: objectSchema([]string{"definition"}, map[string]any{
			"definition": strProp("The workflow as a JSON object with a name, description, and steps. Step verbs are hold, approve, notify, note, away, event, cancel, message, and wait."),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			def := argString(parseArgs(raw), "definition")
			if def == "" {
				return "", fmt.Errorf("create_workflow: definition is required")
			}
			f, err := os.CreateTemp("", "vamoose-workflow-*.json")
			if err != nil {
				return "", err
			}
			defer func() { _ = os.Remove(f.Name()) }()
			if _, err := f.WriteString(def); err != nil {
				_ = f.Close()
				return "", err
			}
			_ = f.Close()
			return execSelf(ctx, "workflows", "add", "--file", f.Name())
		},
	})
}

// workflowRunProps returns the shared input schema for previewing and running a
// workflow: the workflow name, a date window, and the common run options.
func workflowRunProps() map[string]any {
	return map[string]any{
		"workflow":  strProp("Workflow name, e.g. pto, notify-only, away (see list_workflows)"),
		"phrase":    strProp("Date phrase like \"next week\" or \"tomorrow\""),
		"start":     strProp("Start date as YYYY-MM-DD; overrides the phrase"),
		"end":       strProp("End date as YYYY-MM-DD; overrides the phrase"),
		"subject":   strProp("Event subject; defaults per workflow"),
		"manager":   strProp("Approver email for a directory-less backend"),
		"attendees": strProp("Comma-separated attendees, for an event workflow"),
		"provider":  strProp("Calendar provider: graph, google, icloud, or caldav"),
	}
}

// runWorkflowArgs builds the run command line from tool arguments. When preview is set
// it dry-runs and never watches, so a preview cannot touch the calendar.
func runWorkflowArgs(m map[string]any, preview bool) []string {
	args := []string{"run", argString(m, "workflow")}
	if p := argString(m, "phrase"); p != "" {
		args = append(args, p)
	}
	for _, f := range []struct{ key, flag string }{
		{"start", "--start"}, {"end", "--end"}, {"subject", "--subject"},
		{"manager", "--manager"}, {"attendees", "--attendees"},
	} {
		if v := argString(m, f.key); v != "" {
			args = append(args, f.flag, v)
		}
	}
	if preview {
		args = append(args, "--dry-run")
	} else if argBool(m, "watch") {
		args = append(args, "--watch")
	}
	return withProvider(args, m)
}

// scheduleWorkflowArgs builds the schedule-add command line from tool arguments.
func scheduleWorkflowArgs(m map[string]any) []string {
	args := []string{"schedule", "add", argString(m, "workflow"), "--every", argString(m, "every"), "--phrase", argString(m, "phrase")}
	if v := argString(m, "subject"); v != "" {
		args = append(args, "--subject", v)
	}
	if v := argString(m, "manager"); v != "" {
		args = append(args, "--manager", v)
	}
	return withProvider(args, m)
}

// mergeProps returns a new map combining base and extra, so schemas can share a common
// set of properties and add their own.
func mergeProps(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// execSelf runs this binary with the given arguments and returns the combined
// output. A non-zero exit is returned as an error alongside the output.
func execSelf(ctx context.Context, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	return out.String(), err
}

// parseArgs decodes tool arguments into a map, tolerating empty input.
func parseArgs(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// withProvider appends a --provider flag when the arguments carry one.
func withProvider(args []string, m map[string]any) []string {
	if v := argString(m, "provider"); v != "" {
		return append(args, "--provider", v)
	}
	return args
}

// withID appends --id and --provider flags when the arguments carry them.
func withID(args []string, m map[string]any) []string {
	if v := argString(m, "id"); v != "" {
		args = append(args, "--id", v)
	}
	return withProvider(args, m)
}

// argString returns the string value at key, or empty when absent or not a string.
func argString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// argBool returns the bool value at key, or false when absent or not a bool.
func argBool(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// objectSchema builds a JSON Schema object with optional required fields.
func objectSchema(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// strProp is a JSON Schema string property with a description.
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// boolProp is a JSON Schema boolean property with a description.
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
