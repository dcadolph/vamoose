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
          ( |    | )         vacation holds, minus the tedium
            \\  //
             ""  ""
`

// usage writes the vamoose banner and help text to w.
func usage(w io.Writer) {
	fmt.Fprint(w, moose)
	fmt.Fprint(w, `
Usage: vamoose <command> [flags]

Commands:
  request   Create a vacation hold and invite your manager to approve it.
  check     Show whether your manager has approved the hold.
  promote   Add your team as optional attendees once approved.
  version   Print the vamoose version.

Run "vamoose <command> -h" for command flags.

Setup (Microsoft 365 / Outlook):
  VAMOOSE_CLIENT_ID   Entra application (client) id (required)
  VAMOOSE_TENANT      Entra tenant id or "organizations" (default: organizations)
  VAMOOSE_TIMEZONE    IANA time zone for event times (default: UTC)
`)
}
