package cmd

import (
	"fmt"
	"io"
)

// moose is the vamoose mascot shown in help output.
const moose = `
      \/,,        ,,\/
       \_\\  ..  //_/
          ( o  o )
           \  --  /          V A M O O S E
           /|    |\
          ( |    | )         calendar workflows, minus the tedium
            \\  //
             ""  ""
`

// usage writes the vamoose banner and help text to w.
func usage(w io.Writer) {
	fmt.Fprint(w, moose)
	fmt.Fprint(w, `
Usage: vamoose <command> [flags]

Commands:
  run       Run a workflow, e.g. "run pto next week" (see workflows).
  workflows List the available workflows, built-in and user-defined.
  request   Create a time-off hold and invite your manager to approve it.
  off       Request time off from a date phrase, e.g. "off next week".
  check     Show whether your manager has approved the hold.
  promote   Add your team as optional attendees once approved.
  cancel    Cancel a hold and notify its attendees.
  away      Mark yourself out of office over a date range.
  event     Create a quick calendar event, optionally inviting others.
  daemon    Poll watched holds and auto-promote them when the manager approves.
  service   Print a launchd or systemd manifest to run the daemon unattended.
  mcp       Serve vamoose to Claude over the Model Context Protocol (stdio).
  whoami    Print the signed-in user, manager, and resolved team.
  team      Show or set your default team: team [list|set <email...>|clear].
  version   Print the vamoose version.

Run "vamoose <command> -h" for command flags.

Setup (Microsoft 365 / Outlook):
  VAMOOSE_CLIENT_ID   Entra application (client) id (required)
  VAMOOSE_TENANT      Entra tenant id or "organizations" (default: organizations)
  VAMOOSE_TIMEZONE    IANA time zone for event times (default: UTC)
  VAMOOSE_PROVIDER    Calendar provider name: graph, google, or icloud (default: graph)

Setup (Google Calendar, --provider google):
  VAMOOSE_GOOGLE_CLIENT_ID      OAuth desktop client id
  VAMOOSE_GOOGLE_CLIENT_SECRET  OAuth desktop client secret

Setup (Apple iCloud, --provider icloud):
  VAMOOSE_ICLOUD_USERNAME      Apple ID email
  VAMOOSE_ICLOUD_APP_PASSWORD  App-specific password (appleid.apple.com)
  VAMOOSE_ICLOUD_CALENDAR      Target calendar name (optional; default: first)
  Note: iCloud sends invites but does not report approvals; promote by hand.
`)
}
