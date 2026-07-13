//go:build windows || linux

package cmd

import (
	"context"
	"flag"
	"os/signal"
	"syscall"
)

// runTray puts the moose in the system tray: a badge of the holds the daemon is
// watching, per-hold check, promote, and cancel, recent history, and a dashboard
// shortcut. It spawns `vamoose app` and `vamoose daemon` when they are not already
// running, so the tray is all the ambient lifecycle a user needs.
func runTray(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tray", flag.ContinueOnError)
	var (
		addr       = fs.String("addr", "127.0.0.1:8787", "Dashboard address to attach to or serve on")
		foreground = fs.Bool("foreground", false, "Run attached to the console instead of detaching")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*foreground {
		if detached, err := trayDetach(*addr); err != nil {
			return err
		} else if detached {
			return nil
		}
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return trayMain(ctx, *addr)
}
