package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dcadolph/vamoose/internal/calendar"
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
// prints a report of what is set up and what is missing, to ease first-time setup. With
// --live it also probes the provider with a real API call to confirm access works.
func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var (
		live     = fs.Bool("live", false, "Probe the provider with a real API call to confirm access")
		provider = fs.String("provider", "", "Calendar provider to check; overrides VAMOOSE_PROVIDER")
		tzFlag   = fs.String("tz", "", "IANA time zone for the live check")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	getenv := os.Getenv
	if *provider != "" {
		base := getenv
		getenv = func(k string) string {
			if k == envProvider {
				return *provider
			}
			return base(k)
		}
	}

	missing := 0
	for _, c := range doctorChecks(getenv) {
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
	if *live {
		doctorLive(ctx, resolveProvider(*provider), resolveTimeZone(*tzFlag))
	}
	if missing == 0 {
		fmt.Fprintln(os.Stdout, "\nReady. Run 'vamoose whoami' to confirm access.")
	} else {
		fmt.Fprintf(os.Stdout, "\n%d required setting(s) missing. See the hints above.\n", missing)
	}
	return nil
}

// doctorLive probes the provider with real API calls, so the report can confirm not just
// that settings are present but that they reach the calendar and resolve the manager.
func doctorLive(ctx context.Context, provName, tz string) {
	fmt.Fprintf(os.Stdout, "\nLive check (%s):\n", provName)
	prov, err := newProvider(provName, tz)
	if err != nil {
		fmt.Fprintf(os.Stdout, "[miss] Build provider: %v\n", err)
		return
	}
	reportProbe(ctx, os.Stdout, prov)
}

// reportProbe writes the result of signing in and resolving the manager to w. It is split
// from doctorLive so it can be tested against a stub provider.
func reportProbe(ctx context.Context, w io.Writer, prov calendar.Provider) {
	me, err := prov.Me(ctx)
	if err != nil {
		fmt.Fprintf(w, "[miss] Sign-in: %v\n", err)
		return
	}
	fmt.Fprintf(w, "[ok]   Signed in as %s\n", personLabel(me))
	mgr, err := prov.Manager(ctx)
	switch {
	case errors.Is(err, calendar.ErrNoManager):
		fmt.Fprintln(w, "[--]   Manager: none set in the directory")
	case errors.Is(err, calendar.ErrNoDirectory):
		fmt.Fprintln(w, "[--]   Manager: this backend has no directory")
	case err != nil:
		fmt.Fprintf(w, "[--]   Manager: %v\n", err)
	default:
		fmt.Fprintf(w, "[ok]   Manager: %s\n", personLabel(mgr))
	}
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
		doctorCheck{Label: "Webhook messaging: use an https URL as a message channel for Teams, Google Chat, and similar", OK: true, Optional: true},
		doctorCheck{Label: "HR leave filing (BambooHR or webhook)", OK: set("VAMOOSE_BAMBOOHR_SUBDOMAIN") || set("VAMOOSE_LEAVE_WEBHOOK_URL"), Optional: true, Hint: "set VAMOOSE_BAMBOOHR_* or VAMOOSE_LEAVE_WEBHOOK_URL to file approved time off as real leave"},
		doctorCheck{Label: "Secrets: " + secrets, OK: true, Optional: true},
		doctorCheck{Label: "Run history: recorded to the audit log (see 'vamoose history')", OK: true, Optional: true},
	)
	if db := getenv("VAMOOSE_DB_PATH"); db != "" {
		checks = append(checks, doctorCheck{Label: "Multi-tenant store: embedded database at " + db, OK: true, Optional: true})
	}
	return checks
}
