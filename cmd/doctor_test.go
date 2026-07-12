package cmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TestDoctorChecks confirms the report counts the right number of missing required
// settings per provider, and that optional comms checks never count as missing.
func TestDoctorChecks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name        string
		Env         map[string]string
		WantMissing int
	}{{ // Test 0: Graph with a client id is fully configured.
		Name: "graph ok", Env: map[string]string{"VAMOOSE_PROVIDER": "graph", "VAMOOSE_CLIENT_ID": "id"},
	}, { // Test 1: Graph without a client id is missing one setting.
		Name: "graph missing", Env: map[string]string{"VAMOOSE_PROVIDER": "graph"}, WantMissing: 1,
	}, { // Test 2: An injected Graph token skips the client id.
		Name: "graph token", Env: map[string]string{"VAMOOSE_PROVIDER": "graph", "VAMOOSE_GRAPH_ACCESS_TOKEN": "t"},
	}, { // Test 3: Google needs both id and secret.
		Name: "google half", Env: map[string]string{"VAMOOSE_PROVIDER": "google", "VAMOOSE_GOOGLE_CLIENT_ID": "id"}, WantMissing: 1,
	}, { // Test 4: CalDAV with url, username, and password is configured.
		Name: "caldav ok", Env: map[string]string{"VAMOOSE_PROVIDER": "caldav", "VAMOOSE_CALDAV_URL": "u", "VAMOOSE_CALDAV_USERNAME": "n", "VAMOOSE_CALDAV_PASSWORD": "p"},
	}, { // Test 5: An unknown provider is flagged.
		Name: "unknown", Env: map[string]string{"VAMOOSE_PROVIDER": "nope"}, WantMissing: 1,
	}, { // Test 6: Optional comms backends unset do not count as missing.
		Name: "icloud only", Env: map[string]string{"VAMOOSE_PROVIDER": "icloud", "VAMOOSE_ICLOUD_USERNAME": "me@x.com", "VAMOOSE_ICLOUD_APP_PASSWORD": "p"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			getenv := func(k string) string { return test.Env[k] }
			missing := 0
			for _, c := range doctorChecks(getenv) {
				if !c.OK && !c.Optional {
					missing++
				}
			}
			if missing != test.WantMissing {
				t.Errorf("%s: missing = %d, want %d", test.Name, missing, test.WantMissing)
			}
		})
	}
}

// TestReportProbe confirms the live probe reports the signed-in user and the manager
// state, including the no-manager case for a directory-less backend.
func TestReportProbe(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name    string
		Prov    *mockProvider
		WantHas []string
	}{{ // Test 0: A resolved manager is reported.
		Name: "with manager", Prov: &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}},
		WantHas: []string{"Signed in as", "Manager: boss@x.com"},
	}, { // Test 1: A directory-less backend reports no manager rather than an error.
		Name: "no manager", Prov: &mockProvider{mgrErr: calendar.ErrNoManager},
		WantHas: []string{"Signed in as", "none set in the directory"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			reportProbe(context.Background(), &buf, test.Prov)
			out := buf.String()
			for _, want := range test.WantHas {
				if !strings.Contains(out, want) {
					t.Errorf("%s: output %q missing %q", test.Name, out, want)
				}
			}
		})
	}
}
