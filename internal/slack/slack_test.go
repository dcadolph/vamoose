package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// buttonValue pulls the first non-empty button value out of a posted Slack message.
func buttonValue(t *testing.T, body []byte) string {
	t.Helper()
	var msg struct {
		Blocks []struct {
			Elements []struct {
				Value string `json:"value"`
			} `json:"elements"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	for _, b := range msg.Blocks {
		for _, e := range b.Elements {
			if e.Value != "" {
				return e.Value
			}
		}
	}
	return ""
}

// sign computes a Slack v0 signature over the timestamp and body.
func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, "v0:"+ts+":")
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// noopRunner is a Runner that returns nothing, for building a Server in tests.
func noopRunner(context.Context, []string, []string) (string, error) { return "", nil }

// captureServer returns a test server that sends each posted body to the channel.
func captureServer(t *testing.T) (*httptest.Server, <-chan []byte) {
	t.Helper()
	ch := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- b
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func TestVerify(t *testing.T) {
	t.Parallel()
	const secret = "shh"
	body := []byte("token=x&text=off+next+week")
	const ts = "1000000000"
	now := time.Unix(1000000000, 0)
	s := NewServer(secret, noopRunner, WithClock(func() time.Time { return now }))
	good := sign(secret, ts, body)

	tests := []struct {
		Name    string
		TS      string
		Sig     string
		Body    []byte
		WantErr bool
	}{{ // Test 0: A valid signature and fresh timestamp pass.
		Name: "valid", TS: ts, Sig: good, Body: body, WantErr: false,
	}, { // Test 1: A wrong signature is rejected.
		Name: "bad signature", TS: ts, Sig: "v0=deadbeef", Body: body, WantErr: true,
	}, { // Test 2: A stale timestamp is rejected even with a valid signature.
		Name: "stale", TS: "999999000", Sig: sign(secret, "999999000", body), Body: body, WantErr: true,
	}, { // Test 3: A non-numeric timestamp is rejected.
		Name: "bad timestamp", TS: "nope", Sig: good, Body: body, WantErr: true,
	}, { // Test 4: A tampered body no longer matches the signature.
		Name: "tampered body", TS: ts, Sig: good, Body: []byte("token=x&text=EVIL"), WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("POST", "/slack/commands", bytes.NewReader(test.Body))
			req.Header.Set("X-Slack-Request-Timestamp", test.TS)
			req.Header.Set("X-Slack-Signature", test.Sig)
			_, err := s.verify(req)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: verify err = %v, wantErr %v", test.Name, err, test.WantErr)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In      string
		Want    []string
		WantErr bool
	}{{ // Test 0: Plain words split on spaces.
		In: "off next week", Want: []string{"off", "next", "week"},
	}, { // Test 1: A double-quoted value stays one argument.
		In: `off next week --subject "beach week"`, Want: []string{"off", "next", "week", "--subject", "beach week"},
	}, { // Test 2: Single quotes work too.
		In: `run pto --manager 'boss@x.com'`, Want: []string{"run", "pto", "--manager", "boss@x.com"},
	}, { // Test 3: Empty text yields no arguments.
		In: "", Want: nil,
	}, { // Test 4: An unterminated quote errors.
		In: `--subject "beach`, WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := tokenize(test.In)
			if (err != nil) != test.WantErr {
				t.Fatalf("tokenize(%q) err = %v, wantErr %v", test.In, err, test.WantErr)
			}
			if !test.WantErr && !reflect.DeepEqual(got, test.Want) {
				t.Errorf("tokenize(%q) = %#v, want %#v", test.In, got, test.Want)
			}
		})
	}
}

func TestHoldID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   string
		Want string
	}{{ // Test 0: A hold id line is extracted.
		In: "Hold created and sent to boss for approval.\nHold id: EVT123", Want: "EVT123",
	}, { // Test 1: Case and spacing are tolerated.
		In: "hold ID:   abc@vamoose", Want: "abc@vamoose",
	}, { // Test 2: No hold id yields empty.
		In: "Marked out of office.", Want: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := holdID(test.In); got != test.Want {
				t.Errorf("holdID = %q, want %q", got, test.Want)
			}
		})
	}
}

// TestApprovalButtons confirms an approval-awaiting hold posts Approve and Decline
// buttons whose signed value binds the resolved approver.
func TestApprovalButtons(t *testing.T) {
	t.Parallel()
	sink, _ := mockSlackAPI(t)
	srv, ch := captureServer(t)
	runner := func(context.Context, []string, []string) (string, error) {
		return "Hold created and sent to boss@x.com for approval.\nHold id: EVT1", nil
	}
	s := NewServer("shh", runner,
		WithOAuth("cid", "csec", sink.URL, memTokens{"T1": "xoxb"}),
		WithOAuthBaseURL(sink.URL),
	)
	s.runCommand(srv.URL, []string{"off", "next", "week"}, nil, "T1", "")
	body := <-ch
	if !bytes.Contains(body, []byte(actionApprove)) || !bytes.Contains(body, []byte(actionDecline)) {
		t.Errorf("approve/decline actions missing: %s", body)
	}
	if !bytes.Contains(body, []byte(`"in_channel"`)) {
		t.Errorf("approval message should be in_channel: %s", body)
	}
	if p, ok := s.decodeApprovalValue(buttonValue(t, body)); !ok || p.H != "EVT1" || p.A != "UBOSS" {
		t.Errorf("button value did not decode to hold EVT1 approver UBOSS: ok=%v p=%+v", ok, p)
	}
}

// TestActionRuns confirms Approve promotes and Decline cancels the referenced hold.
func TestActionRuns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Action   string
		WantArgs []string
	}{{ // Test 0: Approve promotes the hold.
		Action: actionApprove, WantArgs: []string{"promote", "--id", "EVT1"},
	}, { // Test 1: Decline cancels the hold.
		Action: actionDecline, WantArgs: []string{"cancel", "--id", "EVT1"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, ch := captureServer(t)
			var gotArgs []string
			runner := func(_ context.Context, args, _ []string) (string, error) {
				gotArgs = args
				return "done", nil
			}
			s := NewServer("shh", runner)
			s.runAction(srv.URL, "U0", test.Action, s.approvalValue(approvalPayload{H: "EVT1"}))
			body := <-ch
			if !reflect.DeepEqual(gotArgs, test.WantArgs) {
				t.Errorf("args = %v, want %v", gotArgs, test.WantArgs)
			}
			if !bytes.Contains(body, []byte("replace_original")) {
				t.Errorf("action should replace the original message: %s", body)
			}
		})
	}
}

// TestActionError confirms a failed Approve or Decline reports the base verb, not
// the past-tense label: "Could not approve", never "Could not approved".
func TestActionError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Action   string
		WantText string
	}{{ // Test 0: A failed approve reads "Could not approve".
		Action: actionApprove, WantText: "Could not approve:",
	}, { // Test 1: A failed decline reads "Could not decline".
		Action: actionDecline, WantText: "Could not decline:",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, ch := captureServer(t)
			runner := func(context.Context, []string, []string) (string, error) {
				return "boom", fmt.Errorf("exit status 1")
			}
			s := NewServer("shh", runner)
			s.runAction(srv.URL, "U0", test.Action, s.approvalValue(approvalPayload{H: "EVT1"}))
			body := <-ch
			if !bytes.Contains(body, []byte(test.WantText)) {
				t.Errorf("error text = %s, want substring %q", body, test.WantText)
			}
			if bytes.Contains(body, []byte("boom")) {
				t.Errorf("raw command output should not be echoed to the channel: %s", body)
			}
		})
	}
}

// TestActionRejectsNonApprover confirms that when a button binds an approver, a click
// from anyone else is rejected and the action never runs.
func TestActionRejectsNonApprover(t *testing.T) {
	t.Parallel()
	srv, ch := captureServer(t)
	ran := false
	runner := func(context.Context, []string, []string) (string, error) {
		ran = true
		return "", nil
	}
	s := NewServer("shh", runner)
	value := s.approvalValue(approvalPayload{H: "EVT1", T: "T1", U: "U1", A: "UBOSS"})
	s.runAction(srv.URL, "UINTRUDER", actionApprove, value)
	body := <-ch
	if ran {
		t.Error("a non-approver's click must not run the action")
	}
	if !bytes.Contains(body, []byte("Only")) {
		t.Errorf("want a rejection message, got: %s", body)
	}
}

// TestActionAllowsApprover confirms the bound approver's click runs the action.
func TestActionAllowsApprover(t *testing.T) {
	t.Parallel()
	srv, ch := captureServer(t)
	var gotArgs []string
	runner := func(_ context.Context, args, _ []string) (string, error) {
		gotArgs = args
		return "ok", nil
	}
	s := NewServer("shh", runner)
	value := s.approvalValue(approvalPayload{H: "EVT1", A: "UBOSS"})
	s.runAction(srv.URL, "UBOSS", actionApprove, value)
	<-ch
	if len(gotArgs) != 3 || gotArgs[0] != "promote" || gotArgs[2] != "EVT1" {
		t.Errorf("args = %v, want promote --id EVT1", gotArgs)
	}
}

// TestActionRejectsForgedValue confirms a value that fails signature verification is
// dropped and never runs a command.
func TestActionRejectsForgedValue(t *testing.T) {
	t.Parallel()
	srv, ch := captureServer(t)
	ran := false
	runner := func(context.Context, []string, []string) (string, error) {
		ran = true
		return "", nil
	}
	s := NewServer("shh", runner)
	s.runAction(srv.URL, "U0", actionApprove, "EVT1.not-a-valid-signature")
	body := <-ch
	if ran {
		t.Error("a forged value must not run the action")
	}
	if !bytes.Contains(body, []byte("invalid")) {
		t.Errorf("want an invalid-action message, got: %s", body)
	}
}

// TestServerMetrics confirms the server counts commands and rejected actions and serves
// them at /metrics in Prometheus text format.
func TestServerMetrics(t *testing.T) {
	t.Parallel()
	s := NewServer("shh", noopRunner)
	h := s.Handler()

	// A dispatched command is counted (the increment is synchronous in handleCommand).
	signedForm(t, h, "shh", "/slack/commands", url.Values{"text": {"off next week"}, "team_id": {"T1"}})
	// A forged action click is rejected and counted.
	s.runAction("", "U0", actionApprove, "forged.value")

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	body := w.Body.String()
	for _, want := range []string{"vamoose_slack_commands_total 1", "vamoose_slack_actions_rejected_total 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q:\n%s", want, body)
		}
	}
}

// TestAllowedSubcommand checks the Slack slash allowlist: user calendar and workflow
// verbs are reachable, host operations and the approval actions are not.
func TestAllowedSubcommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name string
		Want bool
	}{
		{"off", true}, {"request", true}, {"run", true}, {"workflows", true},
		{"check", true}, {"schedule", true}, {"away", true}, {"event", true},
		{"promote", false}, {"cancel", false}, {"daemon", false}, {"slack", false},
		{"mcp", false}, {"login", false}, {"service", false}, {"bogus", false},
	}
	for _, test := range tests {
		if got := allowedSubcommand(test.Name); got != test.Want {
			t.Errorf("allowedSubcommand(%q) = %v, want %v", test.Name, got, test.Want)
		}
	}
}

// TestSlashRejectsDisallowed confirms a disallowed subcommand is refused before it runs.
func TestSlashRejectsDisallowed(t *testing.T) {
	t.Parallel()
	ran := false
	runner := func(context.Context, []string, []string) (string, error) { ran = true; return "", nil }
	s := NewServer("shh", runner)
	h := s.Handler()
	w := signedForm(t, h, "shh", "/slack/commands", url.Values{"text": {"daemon --once"}})
	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("want a not-available message, got: %s", w.Body.String())
	}
	if ran {
		t.Error("a disallowed command must not run")
	}
}

// TestWithholdsButtonsWithoutApprover confirms that in both per-user and single-tenant
// mode, when the approver cannot be resolved (no workspace token here), the buttons are
// withheld so no unverified click can act.
func TestWithholdsButtonsWithoutApprover(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name  string
		Owner string
	}{
		{"per-user", "U1"},
		{"single-tenant", ""},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			srv, ch := captureServer(t)
			runner := func(context.Context, []string, []string) (string, error) {
				return "Hold created and sent to boss@x.com for approval.\nHold id: EVT1", nil
			}
			s := NewServer("shh", runner)
			s.runCommand(srv.URL, []string{"off"}, nil, "T1", test.Owner)
			body := <-ch
			if bytes.Contains(body, []byte(actionApprove)) {
				t.Errorf("approval buttons should be withheld without a verified approver: %s", body)
			}
			if !bytes.Contains(body, []byte("calendar invite")) {
				t.Errorf("want the calendar-invite fallback message, got: %s", body)
			}
		})
	}
}
