package cmd

import (
	"slices"
	"testing"
)

// TestMergeEnv confirms injected per-user keys replace inherited ones while
// unrelated variables are preserved, so a linked user's credentials always win.
func TestMergeEnv(t *testing.T) {
	t.Parallel()
	base := []string{"PATH=/bin", "VAMOOSE_PROVIDER=graph", "VAMOOSE_GOOGLE_ACCESS_TOKEN=old", "HOME=/h"}
	inject := []string{"VAMOOSE_PROVIDER=google", "VAMOOSE_GOOGLE_ACCESS_TOKEN=new"}
	got := mergeEnv(base, inject)

	if !slices.Contains(got, "PATH=/bin") || !slices.Contains(got, "HOME=/h") {
		t.Errorf("unrelated variables dropped: %v", got)
	}
	if slices.Contains(got, "VAMOOSE_PROVIDER=graph") || slices.Contains(got, "VAMOOSE_GOOGLE_ACCESS_TOKEN=old") {
		t.Errorf("stale per-user variables kept: %v", got)
	}
	if !slices.Contains(got, "VAMOOSE_PROVIDER=google") || !slices.Contains(got, "VAMOOSE_GOOGLE_ACCESS_TOKEN=new") {
		t.Errorf("injected variables missing: %v", got)
	}
	if base2 := mergeEnv(base, nil); len(base2) != len(base) {
		t.Errorf("nil inject changed base: %v", base2)
	}
}
