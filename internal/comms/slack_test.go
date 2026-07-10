package comms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSlackNotify covers a successful post, a Slack-reported error, and the empty
// channel guard, checking the request carries the token, channel, and text.
func TestSlackNotify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name        string
		Channel     string
		Text        string
		APIResponse string
		WantErr     bool
		WantSubstr  string
	}{{ // Test 0: A successful post carries the channel and text.
		Name: "ok", Channel: "#team", Text: "Alice is out next week",
		APIResponse: `{"ok":true}`,
	}, { // Test 1: A Slack error is surfaced.
		Name: "slack error", Channel: "#team", Text: "hi",
		APIResponse: `{"ok":false,"error":"channel_not_found"}`,
		WantErr:     true, WantSubstr: "channel_not_found",
	}, { // Test 2: An empty channel fails before any request.
		Name: "empty channel", Channel: "", Text: "hi", WantErr: true, WantSubstr: "empty channel",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var gotAuth, gotBody string
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				gotAuth = r.Header.Get("Authorization")
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				_, _ = io.WriteString(w, test.APIResponse)
			}))
			defer srv.Close()

			n := NewSlackNotifier("xoxb-token", WithSlackBaseURL(srv.URL))
			err := n.Notify(context.Background(), test.Channel, test.Text)
			if (err != nil) != test.WantErr {
				t.Fatalf("%s: Notify err = %v, wantErr %v", test.Name, err, test.WantErr)
			}
			if test.WantSubstr != "" && (err == nil || !strings.Contains(err.Error(), test.WantSubstr)) {
				t.Errorf("%s: err = %v, want substring %q", test.Name, err, test.WantSubstr)
			}
			if test.Channel == "" {
				if called {
					t.Errorf("%s: an empty channel should not call the API", test.Name)
				}
				return
			}
			if gotAuth != "Bearer xoxb-token" {
				t.Errorf("%s: auth header = %q, want the bearer token", test.Name, gotAuth)
			}
			var sent map[string]string
			if jerr := json.Unmarshal([]byte(gotBody), &sent); jerr != nil {
				t.Fatalf("%s: request body not JSON: %v", test.Name, jerr)
			}
			if sent["channel"] != test.Channel || sent["text"] != test.Text {
				t.Errorf("%s: sent = %v, want channel %q text %q", test.Name, sent, test.Channel, test.Text)
			}
		})
	}
}
