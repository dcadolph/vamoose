package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRenderService(t *testing.T) {
	t.Parallel()
	m := serviceManifest{
		Label:    "com.test.vamoose",
		Exe:      "/usr/local/bin/vamoose",
		Args:     []string{"/usr/local/bin/vamoose", "daemon", "--interval", "1m0s"},
		Interval: "1m0s",
		LogPath:  "/tmp/vamoose.log",
	}
	tests := []struct {
		GOOS    string
		WantSub []string
		WantErr bool
	}{{ // Test 0: macOS renders a launchd plist.
		GOOS:    "darwin",
		WantSub: []string{"<key>Label</key>", "com.test.vamoose", "/usr/local/bin/vamoose", "daemon"},
	}, { // Test 1: Linux renders a systemd unit.
		GOOS:    "linux",
		WantSub: []string{"ExecStart=/usr/local/bin/vamoose daemon --interval 1m0s", "WantedBy=default.target"},
	}, { // Test 2: An unsupported platform errors.
		GOOS: "plan9", WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := renderService(&buf, test.GOOS, m)
			if (err != nil) != test.WantErr {
				t.Fatalf("renderService(%q) err = %v, wantErr %v", test.GOOS, err, test.WantErr)
			}
			if err != nil {
				return
			}
			for _, sub := range test.WantSub {
				if !strings.Contains(buf.String(), sub) {
					t.Errorf("output missing %q:\n%s", sub, buf.String())
				}
			}
		})
	}
}
