package cmd

import (
	"fmt"
	"runtime/debug"
)

// version is the semantic version, overridable at build time via -ldflags.
var version = "0.8.0"

// versionString returns the version, appending the VCS revision when available.
func versionString() string {
	rev := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				rev = s.Value[:7]
			}
		}
	}
	if rev == "" {
		return fmt.Sprintf("vamoose %s", version)
	}
	return fmt.Sprintf("vamoose %s (%s)", version, rev)
}
