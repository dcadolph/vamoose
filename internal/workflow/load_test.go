package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		JSON string
		Want error
	}{{ // Test 0: A valid definition parses.
		JSON: `{"name":"x","steps":[{"verb":"away"}]}`, Want: nil,
	}, { // Test 1: Malformed JSON is invalid.
		JSON: `{"name":`, Want: ErrInvalid,
	}, { // Test 2: An unknown field is rejected.
		JSON: `{"name":"x","step":[{"verb":"away"}]}`, Want: ErrInvalid,
	}, { // Test 3: A parseable but semantically invalid definition is invalid.
		JSON: `{"name":"x","steps":[{"verb":"notify"}]}`, Want: ErrInvalid,
	}, { // Test 4: An unknown verb is reported.
		JSON: `{"name":"x","steps":[{"verb":"nope"}]}`, Want: ErrUnknownVerb,
	}, { // Test 5: A second JSON value after the workflow is rejected.
		JSON: `{"name":"x","steps":[{"verb":"away"}]} {"name":"evil"}`, Want: ErrInvalid,
	}, { // Test 6: Trailing non-JSON garbage is rejected.
		JSON: `{"name":"x","steps":[{"verb":"away"}]} garbage`, Want: ErrInvalid,
	}, { // Test 7: Trailing whitespace is fine.
		JSON: "{\"name\":\"x\",\"steps\":[{\"verb\":\"away\"}]}\n\t ", Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(test.JSON))
			if !errors.Is(err, test.Want) {
				t.Errorf("Parse err = %v, want %v", err, test.Want)
			}
		})
	}
}

// TestLoadBuiltin confirms the embedded templates load and unknown or unsafe names
// are rejected.
func TestLoadBuiltin(t *testing.T) {
	t.Parallel()
	l := Loader{}
	tests := []struct {
		Name      string
		WantSteps int
		Want      error
	}{{ // Test 0: The pto template has three steps.
		Name: "pto", WantSteps: 3, Want: nil,
	}, { // Test 1: The away template has one step.
		Name: "away", WantSteps: 1, Want: nil,
	}, { // Test 2: The notify-only template has two steps.
		Name: "notify-only", WantSteps: 2, Want: nil,
	}, { // Test 3: An unknown name is not found.
		Name: "nope", Want: ErrUnknownWorkflow,
	}, { // Test 4: A traversal name is rejected.
		Name: "../secrets", Want: ErrUnknownWorkflow,
	}, { // Test 5: An empty name is rejected.
		Name: "", Want: ErrUnknownWorkflow,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			w, err := l.Load(test.Name)
			if !errors.Is(err, test.Want) {
				t.Fatalf("Load(%q) err = %v, want %v", test.Name, err, test.Want)
			}
			if test.Want == nil {
				if w.Name != test.Name {
					t.Errorf("name = %q, want %q", w.Name, test.Name)
				}
				if len(w.Steps) != test.WantSteps {
					t.Errorf("steps = %d, want %d", len(w.Steps), test.WantSteps)
				}
			}
		})
	}
}

// TestLoadUserOverride confirms a user directory file wins over a built-in and that
// custom names load.
func TestLoadUserOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pto.json"), `{"name":"pto","steps":[{"verb":"away"}]}`)
	writeFile(t, filepath.Join(dir, "custom.json"), `{"name":"custom","steps":[{"verb":"event"}]}`)
	l := Loader{UserDir: dir}

	pto, err := l.Load("pto")
	if err != nil {
		t.Fatalf("Load(pto): %v", err)
	}
	if len(pto.Steps) != 1 || pto.Steps[0].Verb != VerbAway {
		t.Errorf("user pto not used: %+v", pto.Steps)
	}
	custom, err := l.Load("custom")
	if err != nil {
		t.Fatalf("Load(custom): %v", err)
	}
	if custom.Steps[0].Verb != VerbEvent {
		t.Errorf("custom verb = %q, want event", custom.Steps[0].Verb)
	}
	// A name present neither in the user dir nor built-ins is still unknown.
	if _, err := l.Load("ghost"); !errors.Is(err, ErrUnknownWorkflow) {
		t.Errorf("Load(ghost) err = %v, want ErrUnknownWorkflow", err)
	}
}

// TestRaw confirms Raw returns the definition bytes as written with the right source:
// a user file wins over a built-in, a built-in falls through, and an unknown or unsafe
// name is rejected.
func TestRaw(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pto.json"), `{"name":"pto","steps":[{"verb":"away"}]}`)
	l := Loader{UserDir: dir}

	data, source, err := l.Raw("pto")
	if err != nil || source != SourceUser {
		t.Fatalf("Raw(pto) source = %v, err = %v; want user", source, err)
	}
	if string(data) != `{"name":"pto","steps":[{"verb":"away"}]}` {
		t.Errorf("Raw(pto) did not return the bytes as written: %s", data)
	}
	if _, source, err = l.Raw("away"); err != nil || source != SourceBuiltin {
		t.Errorf("Raw(away) source = %v, err = %v; want builtin", source, err)
	}
	if _, _, err = l.Raw("ghost"); !errors.Is(err, ErrUnknownWorkflow) {
		t.Errorf("Raw(ghost) err = %v, want ErrUnknownWorkflow", err)
	}
	if _, _, err = l.Raw("../evil"); !errors.Is(err, ErrUnknownWorkflow) {
		t.Errorf("Raw(../evil) err = %v, want ErrUnknownWorkflow", err)
	}
}

// TestList confirms listing merges built-ins with user overrides and sorts by name.
func TestList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pto.json"), `{"name":"pto","description":"mine","steps":[{"verb":"away"}]}`)
	writeFile(t, filepath.Join(dir, "custom.json"), `{"name":"custom","description":"c","steps":[{"verb":"event"}]}`)
	l := Loader{UserDir: dir}

	infos, err := l.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make(map[string]Info, len(infos))
	names := make([]string, len(infos))
	for i, in := range infos {
		got[in.Name] = in
		names[i] = in.Name
	}
	if !sortedStrings(names) {
		t.Errorf("List not sorted: %v", names)
	}
	for _, name := range []string{"pto", "away", "notify-only", "custom"} {
		if _, ok := got[name]; !ok {
			t.Errorf("List missing %q", name)
		}
	}
	if got["pto"].Source != SourceUser || got["pto"].Description != "mine" {
		t.Errorf("pto not overridden by user: %+v", got["pto"])
	}
	if got["away"].Source != SourceBuiltin {
		t.Errorf("away source = %q, want builtin", got["away"].Source)
	}
}

// TestBuiltinsValid confirms every shipped template loads and validates, guarding
// the JSON files against regressions.
func TestBuiltinsValid(t *testing.T) {
	t.Parallel()
	l := Loader{}
	infos, err := l.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("no built-in templates found")
	}
	for _, in := range infos {
		if _, err := l.Load(in.Name); err != nil {
			t.Errorf("built-in %q failed to load: %v", in.Name, err)
		}
	}
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// sortedStrings reports whether s is in non-decreasing order.
func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
