//go:build windows

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// trayDetach relaunches the tray as a windowless background process, so the console
// this command was typed into is free immediately and no console window follows the
// tray around. It reports true when the caller should exit in favor of the child.
func trayDetach(addr string) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate vamoose binary: %w", err)
	}
	cmd := exec.Command(exe, "tray", "--foreground", "--addr", addr)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start tray: %w", err)
	}
	// Let the child outlive this console process rather than being reaped by it.
	_ = cmd.Process.Release()
	fmt.Fprintln(os.Stdout, "vamoose tray is running in the notification area")
	return true, nil
}
