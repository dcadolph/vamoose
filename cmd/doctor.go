package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dcadolph/vamoose/internal/secret"
)

// doctorCheck is one line of the doctor report.
type doctorCheck struct {
	// Label describes what was checked.
	Label string
	// OK reports whether the check passed.
	OK bool
	// Hint is a remedy shown when the check fails.
	Hint string
	// Optional marks an informational check, so failing it is not a problem.
	Optional bool
}

// runDoctor checks the environment for the selected provider and comms backends and
// prints a report of what is set up and what is missing, to ease first-time setup.
func runDoctor(_ context.Context, _ []string) error {
	missing := 0
	for _, c := range doctorChecks(os.Getenv) {
		switch {
		case c.OK:
			fmt.Fprintf(os.Stdout, "[ok]   %s\n", c.Label)
		case c.Optional:
			fmt.Fprintf(os.Stdout, "[--]   %s%s\n", c.Label, hintSuffix(c.Hint))
		default:
			missing++
			fmt.Fprintf(os.Stdout, "[miss] %s%s\n", c.Label, hintSuffix(c.Hint))
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		fmt.Fprintf(os.Stdout, "[ok]   Config directory: %s\n", filepath.Join(dir, "vamoose"))
	}
	if missing == 0 {
		fmt.Fprintln(os.Stdout, "\nReady. Run 'vamoose whoami' to confirm access.")
	} else {
		fmt.Fprintf(os.Stdout, "\n%d required setting(s) missing. See the hints above.\n", missing)
	}
	return nil
}

// hintSuffix formats a hint for display, or an empty string when there is none.
func hintSuffix(hint string) string {
	if hint == "" {
		return ""
	}
	return " -> " + hint
}

// doctorChecks builds the configuration report from the given environment lookup, so it
// is testable without touching the process environment.
func doctorChecks(getenv func(string) string) []doctorCheck {
	set := func(k string) bool { return getenv(k) != "" }
	provider := getenv(envProvider)
	if provider == "" {
		provider = defaultProvider
	}
	checks := []doctorCheck{{Label: "Provider: " + provider, OK: true}}

	switch provider {
	case defaultProvider:
		if set("VAMOOSE_GRAPH_ACCESS_TOKEN") {
			checks = append(checks, doctorCheck{Label: "Graph access token set", OK: true})
		} else {
			checks = append(checks, doctorCheck{Label: "VAMOOSE_CLIENT_ID (Entra app client id)", OK: set("VAMOOSE_CLIENT_ID"), Hint: "register an Entra app and export its client id"})
		}
	case providerGoogle:
		switch {
		case set("VAMOOSE_GOOGLE_ACCESS_TOKEN"):
			checks = append(checks, doctorCheck{Label: "Google access token set", OK: true})
		default:
			_, _, ok := googleClientCredsFrom(getenv)
			switch {
			case ok && set("VAMOOSE_GOOGLE_CLIENT_ID"):
				checks = append(checks, doctorCheck{Label: "Google OAuth client: your override (run 'vamoose login')", OK: true})
			case ok:
				checks = append(checks, doctorCheck{Label: "Google OAuth client: built-in (run 'vamoose login')", OK: true})
			default:
				checks = append(checks, doctorCheck{Label: "Google OAuth client", OK: false, Hint: "set VAMOOSE_GOOGLE_CLIENT_ID and VAMOOSE_GOOGLE_CLIENT_SECRET, or use a release build with a built-in client"})
			}
		}
	case providerICloud:
		checks = append(checks,
			doctorCheck{Label: "VAMOOSE_ICLOUD_USERNAME", OK: set("VAMOOSE_ICLOUD_USERNAME"), Hint: "your Apple ID email"},
			doctorCheck{Label: "VAMOOSE_ICLOUD_APP_PASSWORD", OK: set("VAMOOSE_ICLOUD_APP_PASSWORD"), Hint: "app-specific password from appleid.apple.com"},
		)
	case providerCalDAV:
		checks = append(checks,
			doctorCheck{Label: "VAMOOSE_CALDAV_URL", OK: set("VAMOOSE_CALDAV_URL"), Hint: "your CalDAV server URL"},
			doctorCheck{Label: "VAMOOSE_CALDAV_USERNAME", OK: set("VAMOOSE_CALDAV_USERNAME"), Hint: "your CalDAV account username"},
			doctorCheck{Label: "VAMOOSE_CALDAV_PASSWORD", OK: set("VAMOOSE_CALDAV_PASSWORD"), Hint: "your CalDAV account password"},
		)
	default:
		checks = append(checks, doctorCheck{Label: "Unknown provider " + provider, Hint: "use graph, google, icloud, or caldav"})
	}

	tz := getenv("VAMOOSE_TIMEZONE")
	if tz == "" {
		tz = "UTC (default)"
	}
	secrets := "OS keychain or a local file"
	if set(secret.KeyEnv) {
		secrets = "encrypted at rest (VAMOOSE_SECRET_KEY set)"
	}
	checks = append(checks,
		doctorCheck{Label: "Time zone: " + tz, OK: true, Optional: true},
		doctorCheck{Label: "Slack messaging (VAMOOSE_SLACK_BOT_TOKEN)", OK: set("VAMOOSE_SLACK_BOT_TOKEN"), Optional: true, Hint: "set for message steps to Slack"},
		doctorCheck{Label: "Email messaging (VAMOOSE_SMTP_HOST)", OK: set("VAMOOSE_SMTP_HOST"), Optional: true, Hint: "set for message steps to email"},
		doctorCheck{Label: "Secrets: " + secrets, OK: true, Optional: true},
		doctorCheck{Label: "Run history: recorded to the audit log (see 'vamoose history')", OK: true, Optional: true},
	)
	return checks
}
