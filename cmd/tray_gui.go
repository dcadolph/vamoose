//go:build windows || linux

package cmd

import (
	"context"
	"fmt"
	"time"

	"fyne.io/systray"

	"github.com/dcadolph/vamoose/internal/audit"
)

// trayHistoryLines is how many recent events the dropdown shows.
const trayHistoryLines = 5

// trayMain runs the system tray loop until the user quits or ctx is canceled, then
// stops any children the tray started.
func trayMain(ctx context.Context, addr string) error {
	client := newTrayClient(addr)
	services := &trayServices{addr: addr}

	// requestRefresh nudges the menu loop without blocking or queueing more than one.
	refreshCh := make(chan struct{}, 1)
	requestRefresh := func() {
		select {
		case refreshCh <- struct{}{}:
		default:
		}
	}

	onReady := func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("vamoose")
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			// epoch is closed on each rebuild so the previous menu's click listeners
			// exit instead of piling up.
			var epoch chan struct{}
			rebuild := func() {
				if epoch != nil {
					close(epoch)
				}
				epoch = make(chan struct{})
				trayRebuild(ctx, client, services, epoch, requestRefresh)
			}
			rebuild()
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-ticker.C:
					rebuild()
				case <-refreshCh:
					rebuild()
				}
			}
		}()
	}
	systray.Run(onReady, func() {})
	services.Terminate()
	return nil
}

// trayRebuild redraws the dropdown from live state: watched holds with their actions,
// recent history, and the dashboard and quit controls. When the server is down it
// starts it and schedules a follow-up refresh for once the port is likely bound.
func trayRebuild(ctx context.Context, client *trayClient, services *trayServices, epoch chan struct{}, requestRefresh func()) {
	up := client.Health(ctx)
	startupNote := ""
	if !up {
		if _, err := services.Ensure(ctx, client); err != nil {
			startupNote = err.Error()
		} else {
			startupNote = "Starting the vamoose server…"
			go func() {
				select {
				case <-time.After(2 * time.Second):
					requestRefresh()
				case <-epoch:
				}
			}()
		}
	}

	var watches []watchItem
	var history []audit.Event
	version := ""
	if up {
		watches, _ = client.Watches(ctx)
		history, _ = client.History(ctx, trayHistoryLines)
		version = client.Version(ctx)
	}

	systray.ResetMenu()
	tooltip := "vamoose"
	if n := len(watches); n > 0 {
		tooltip = fmt.Sprintf("vamoose · watching %d", n)
		systray.SetTitle(fmt.Sprintf("🫎 %d", n))
	} else {
		systray.SetTitle("🫎")
	}
	systray.SetTooltip(tooltip)

	// onClick runs fn on each click until this menu generation is replaced.
	onClick := func(mi *systray.MenuItem, fn func()) {
		go func() {
			for {
				select {
				case <-epoch:
					return
				case _, ok := <-mi.ClickedCh:
					if !ok {
						return
					}
					fn()
				}
			}
		}()
	}

	switch {
	case !up:
		systray.AddMenuItem(startupNote, "").Disable()
	case len(watches) == 0:
		systray.AddMenuItem("Nothing is being watched", "").Disable()
	default:
		systray.AddMenuItem("Watching", "").Disable()
		for _, w := range watches {
			hold := systray.AddMenuItem(trayWatchLine(w), "")
			for _, act := range []string{"check", "promote", "cancel"} {
				item := hold.AddSubMenuItem(titleCase(act), "")
				action, holdID, provider := act, w.HoldID, w.Provider
				onClick(item, func() {
					_ = client.Action(ctx, action, holdID, provider)
					requestRefresh()
				})
			}
		}
	}

	if len(history) > 0 {
		systray.AddSeparator()
		systray.AddMenuItem("Recent", "").Disable()
		for _, e := range history {
			systray.AddMenuItem(trayEventLine(e), "").Disable()
		}
	}

	systray.AddSeparator()
	onClick(systray.AddMenuItem("Open dashboard", ""), func() {
		openBrowser("http://" + services.addr)
	})
	onClick(systray.AddMenuItem("Refresh", ""), requestRefresh)
	systray.AddSeparator()
	if version != "" {
		systray.AddMenuItem(version, "").Disable()
	}
	onClick(systray.AddMenuItem("Quit vamoose tray", ""), systray.Quit)
}
