package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
)

// serviceManifest holds the values rendered into a service definition.
type serviceManifest struct {
	// Label is the launchd service label.
	Label string
	// Exe is the absolute path to the vamoose binary.
	Exe string
	// Args are the full launchd ProgramArguments, including the binary.
	Args []string
	// Interval is the daemon polling interval, as a duration string.
	Interval string
	// LogPath is where the launchd service writes daemon output.
	LogPath string
}

// launchdManifest is the macOS LaunchAgent plist template.
const launchdManifest = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
{{range .Args}}		<string>{{.}}</string>
{{end}}	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{.LogPath}}</string>
</dict>
</plist>
`

// systemdManifest is the Linux systemd user unit template.
const systemdManifest = `[Unit]
Description=vamoose calendar-workflow daemon
After=network-online.target

[Service]
ExecStart={{.Exe}} daemon --interval {{.Interval}}
Restart=on-failure

[Install]
WantedBy=default.target
`

// Service definition templates, parsed once at startup.
var (
	launchdTmpl = template.Must(template.New("launchd").Parse(launchdManifest))
	systemdTmpl = template.Must(template.New("systemd").Parse(systemdManifest))
)

// runService prints a service manifest that runs the daemon unattended. The
// manifest goes to stdout; install instructions go to stderr, so redirecting
// stdout captures a clean file.
func runService(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	var (
		interval = fs.Duration("interval", defaultInterval, "Polling interval passed to the daemon")
		label    = fs.String("label", "com.dcadolph.vamoose", "launchd service label (macOS)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	logPath, err := daemonLogPath()
	if err != nil {
		return err
	}
	m := serviceManifest{
		Label:    *label,
		Exe:      exe,
		Args:     []string{exe, "daemon", "--interval", interval.String()},
		Interval: interval.String(),
		LogPath:  logPath,
	}
	if err := renderService(os.Stdout, runtime.GOOS, m); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	printServiceInstructions(os.Stderr, runtime.GOOS, *label)
	return nil
}

// renderService writes the manifest for the given OS, erroring on unsupported ones.
func renderService(w io.Writer, goos string, m serviceManifest) error {
	switch goos {
	case "darwin":
		return launchdTmpl.Execute(w, m)
	case "linux":
		return systemdTmpl.Execute(w, m)
	default:
		return fmt.Errorf("unsupported platform %q; run 'vamoose daemon' manually", goos)
	}
}

// printServiceInstructions writes the install steps as comments to w.
func printServiceInstructions(w io.Writer, goos, label string) {
	switch goos {
	case "darwin":
		fmt.Fprintf(w, "\n# Save the plist above, then load it:\n")
		fmt.Fprintf(w, "#   vamoose service > ~/Library/LaunchAgents/%s.plist\n", label)
		fmt.Fprintf(w, "#   launchctl load ~/Library/LaunchAgents/%s.plist\n", label)
	case "linux":
		fmt.Fprintf(w, "\n# Save the unit above, then enable it:\n")
		fmt.Fprintf(w, "#   vamoose service > ~/.config/systemd/user/vamoose.service\n")
		fmt.Fprintf(w, "#   systemctl --user enable --now vamoose\n")
	}
}

// daemonLogPath returns where the launchd service writes daemon output.
func daemonLogPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "daemon.log"), nil
}
