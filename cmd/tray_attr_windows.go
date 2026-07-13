//go:build windows

package cmd

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// trayChildAttr hides spawned children from the desktop: without CREATE_NO_WINDOW each
// console child would pop its own console window next to the tray.
func trayChildAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
