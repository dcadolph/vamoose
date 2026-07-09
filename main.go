// Command vamoose runs calendar workflows, from a time-off hold that a manager
// approves and the team is notified of, to quick actions and your own workflows.
package main

import (
	"os"

	"github.com/dcadolph/vamoose/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
