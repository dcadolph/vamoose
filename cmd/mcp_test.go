package cmd

import (
	"context"
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
	}
	for _, name := range want {
		if !strings.Contains(out.String(), `"name":"`+name+`"`) {
			t.Errorf("tools/list missing %q:\n%s", name, out.String())
		}
	}
}
