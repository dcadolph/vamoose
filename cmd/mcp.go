package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
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
			"provider": strProp("Calendar provider: graph or google"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			m := parseArgs(raw)
			return execSelf(ctx, withProvider([]string{"whoami"}, m)...)
		},
	})
	srv.Register(mcp.Tool{
		Name:        "request_time_off",
		Description: "Create a vacation hold shown as free and invite the manager to approve it.",
		InputSchema: objectSchema([]string{"start", "end", "subject"}, map[string]any{
			"start":    strProp("Start date as YYYY-MM-DD or an RFC3339 time"),
			"end":      strProp("End date as YYYY-MM-DD or an RFC3339 time"),
			"subject":  strProp("Event subject"),
			"manager":  strProp("Manager email; omit to resolve from the directory"),
			"provider": strProp("Calendar provider: graph or google"),
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
			"provider": strProp("Calendar provider: graph or google"),
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
			"provider": strProp("Calendar provider: graph or google"),
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
			"provider": strProp("Calendar provider: graph or google"),
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
			"provider": strProp("Calendar provider: graph or google"),
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
		Name:        "create_event",
		Description: "Create a quick calendar event, optionally inviting attendees.",
		InputSchema: objectSchema([]string{"start", "end", "subject"}, map[string]any{
			"start":     strProp("Start date as YYYY-MM-DD or an RFC3339 time"),
			"end":       strProp("End date as YYYY-MM-DD or an RFC3339 time"),
			"subject":   strProp("Event subject"),
			"attendees": strProp("Comma-separated attendee emails to invite"),
			"free":      boolProp("Show the event as free instead of busy"),
			"provider":  strProp("Calendar provider: graph or google"),
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
