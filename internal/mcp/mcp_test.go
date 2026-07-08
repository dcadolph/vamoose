package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	t.Parallel()
	srv := NewServer("test", "1.0")
	srv.Register(Tool{
		Name:        "echo",
		Description: "echoes its arguments",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			return "handled:" + string(args), nil
		},
	})

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"bogus"}`,
	}, "\n") + "\n"

	var out strings.Builder
	if err := srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")

	// The notification produces no response, so five requests yield five lines.
	if len(lines) != 5 {
		t.Fatalf("got %d responses, want 5:\n%s", len(lines), out.String())
	}
	want := []string{
		`"protocolVersion":"2025-06-18"`, // initialize echoes the version
		`"name":"echo"`,                  // tools/list advertises the tool
		`handled:`,                       // tools/call ran the handler
		`-32602`,                         // unknown tool is a protocol error
		`-32601`,                         // unknown method is method-not-found
	}
	for i, sub := range want {
		if !strings.Contains(lines[i], sub) {
			t.Errorf("line %d = %s\nwant substring %q", i, lines[i], sub)
		}
	}
}
