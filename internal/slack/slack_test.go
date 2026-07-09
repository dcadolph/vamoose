package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

// sign computes a Slack v0 signature over the timestamp and body.
func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, "v0:"+ts+":")
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// noopRunner is a Runner that returns nothing, for building a Server in tests.
func noopRunner(context.Context, []string) (string, error) { return "", nil }

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
