//go:build !windows && !linux

package cmd

import (
	"context"
	"errors"
)

// runTray reports that the Go tray only targets Windows and Linux: macOS has the
// native menu bar app, built with `make tray`.
func runTray(context.Context, []string) error {
	return errors.New("the tray targets Windows and Linux; on macOS build VamooseTray.app with `make tray`")
}
