// Command vamoose creates a vacation calendar hold, routes it to your manager
// for approval, then fans it out to your team as optional attendees.
package main

import (
	"os"

	"github.com/dcadolph/vamoose/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
