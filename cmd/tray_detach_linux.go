//go:build linux

package cmd

// trayDetach is a no-op on Linux: launching from a terminal keeps the process attached,
// which is the expected Unix behavior, and a login autostart runs it detached already.
func trayDetach(string) (bool, error) { return false, nil }
