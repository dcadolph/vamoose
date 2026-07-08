package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

func TestMergeTeam(t *testing.T) {
	t.Parallel()
	dirPeople := []calendar.Person{{Email: "peer@x.com"}}
	failDir := func() ([]calendar.Person, error) { return nil, errors.New("directory called") }
	okDir := func() ([]calendar.Person, error) { return dirPeople, nil }

	tests := []struct {
		Directory  func() ([]calendar.Person, error)
		Config     []string
		WantSource teamSource
		WantCount  int
	}{{ // Test 0: Config wins and the directory is never called.
		Config: []string{"a@x.com", "b@x.com"}, Directory: failDir,
		WantSource: sourceConfig, WantCount: 2,
	}, { // Test 1: Empty config falls back to the directory.
		Config: nil, Directory: okDir,
		WantSource: sourceDirectory, WantCount: 1,
	}, { // Test 2: Blank-only config is treated as empty.
		Config: []string{"  ", ""}, Directory: okDir,
		WantSource: sourceDirectory, WantCount: 1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			people, source, err := mergeTeam(test.Config, test.Directory)
			if err != nil {
				t.Fatalf("mergeTeam: %v", err)
			}
			if source != test.WantSource {
				t.Errorf("source = %q, want %q", source, test.WantSource)
			}
			if len(people) != test.WantCount {
				t.Errorf("count = %d, want %d", len(people), test.WantCount)
			}
		})
	}
}

func TestPeopleFromEmails(t *testing.T) {
	t.Parallel()
	got := peopleFromEmails([]string{" a@x.com ", "", "b@x.com", "   "})
	want := []calendar.Person{{Email: "a@x.com"}, {Email: "b@x.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("peopleFromEmails = %+v, want %+v", got, want)
	}
}

func TestPersonLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         calendar.Person
		WantResult string
	}{{ // Test 0: Name and email.
		In: calendar.Person{Name: "Ann", Email: "ann@x.com"}, WantResult: "Ann <ann@x.com>",
	}, { // Test 1: Email only.
		In: calendar.Person{Email: "ann@x.com"}, WantResult: "ann@x.com",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := personLabel(test.In); got != test.WantResult {
				t.Errorf("personLabel = %q, want %q", got, test.WantResult)
			}
		})
	}
}

// TestTeamConfigRoundTrip exercises the on-disk config against an isolated HOME.
// It cannot run in parallel because it sets process environment variables.
func TestTeamConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	if got, err := loadTeamConfig(); err != nil || got != nil {
		t.Fatalf("loadTeamConfig on empty = %v, %v; want nil, nil", got, err)
	}
	want := []string{"a@x.com", "b@x.com"}
	if err := saveTeamConfig(want); err != nil {
		t.Fatalf("saveTeamConfig: %v", err)
	}
	got, err := loadTeamConfig()
	if err != nil {
		t.Fatalf("loadTeamConfig: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaded = %+v, want %+v", got, want)
	}
	if err := clearTeamConfig(); err != nil {
		t.Fatalf("clearTeamConfig: %v", err)
	}
	if got, err := loadTeamConfig(); err != nil || got != nil {
		t.Fatalf("loadTeamConfig after clear = %v, %v; want nil, nil", got, err)
	}
}
