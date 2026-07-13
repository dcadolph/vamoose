//go:build !windows

package cmd

import "syscall"

// trayChildAttr returns no special attributes: only Windows needs children hidden
// from the desktop.
func trayChildAttr() *syscall.SysProcAttr { return nil }
