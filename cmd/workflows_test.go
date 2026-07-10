package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWorkflowAddRemove confirms a valid definition is saved and loadable, an invalid
// one is rejected, and remove deletes the user workflow. It isolates the config dir.
func TestWorkflowAddRemove(t *testing.T) {
	isolateConfig(t)
	def := `{"name":"my-flow","description":"test","steps":[{"verb":"hold"},{"verb":"notify","team":"optional"}]}`
	good := filepath.Join(t.TempDir(), "def.json")
	if err := os.WriteFile(good, []byte(def), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workflowAdd([]string{"--file", good}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := workflowLoader().Load("my-flow"); err != nil {
		t.Errorf("workflow not loadable after add: %v", err)
	}

	// An invalid definition is rejected, not written.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"name":"broken","steps":[{"verb":"teleport"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workflowAdd([]string{"--file", bad}); err == nil {
		t.Error("want an error for an invalid definition")
	}
	if _, err := workflowLoader().Load("broken"); err == nil {
		t.Error("an invalid workflow should not have been saved")
	}

	// Remove deletes the user workflow.
	if err := workflowRemove([]string{"my-flow"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := workflowLoader().Load("my-flow"); err == nil {
		t.Error("workflow still loads after remove")
	}
	// Removing an absent one errors.
	if err := workflowRemove([]string{"ghost"}); err == nil {
		t.Error("want an error removing an unknown workflow")
	}
}

// TestSafeWorkflowName covers the name guard against path traversal.
func TestSafeWorkflowName(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"pto": true, "team-heads-up": true,
		"": false, "../secrets": false, "a/b": false, "dot.name": false,
	}
	for name, want := range tests {
		if got := safeWorkflowName(name); got != want {
			t.Errorf("safeWorkflowName(%q) = %v, want %v", name, got, want)
		}
	}
}
