// Package cmd implements the vamoose command-line interface.
package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
)

// Execute parses args, runs the selected subcommand, and returns an exit code.
func Execute(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return codeUsage
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "run":
		return run(runRun(ctx, rest))
	case "workflows":
		return run(runWorkflows(ctx, rest))
	case "request":
		return run(runRequest(ctx, rest))
	case "off":
		return run(runOff(ctx, rest))
	case "check":
		return run(runCheck(ctx, rest))
	case "history":
		return run(runHistory(ctx, rest))
	case "promote":
		return run(runPromote(ctx, rest))
	case "cancel":
		return run(runCancel(ctx, rest))
	case "away":
		return run(runAway(ctx, rest))
	case "balance":
		return run(runBalance(ctx, rest))
	case "coverage":
		return run(runCoverage(ctx, rest))
	case "event":
		return run(runEvent(ctx, rest))
	case "daemon":
		return run(runDaemon(ctx, rest))
	case "service":
		return run(runService(ctx, rest))
	case "mcp":
		return run(runMCP(ctx, rest))
	case "slack":
		return run(runSlack(ctx, rest))
	case "app":
		return run(runApp(ctx, rest))
	case "login":
		return run(runLogin(ctx, rest))
	case "whoami":
		return run(runWhoami(ctx, rest))
	case "calendars":
		return run(runCalendars(ctx, rest))
	case "team":
		return run(runTeam(ctx, rest))
	case "schedule":
		return run(runSchedule(ctx, rest))
	case "doctor":
		return run(runDoctor(ctx, rest))
	case "version", "-v", "--version":
		fmt.Fprintln(os.Stdout, versionString())
		return codeOK
	case "help", "-h", "--help":
		usage(os.Stdout)
		return codeOK
	default:
		fmt.Fprintf(os.Stderr, "vamoose: unknown command %q\n\n", sub)
		usage(os.Stderr)
		return codeUsage
	}
}

// run maps a subcommand error to an exit code, printing it to stderr.
func run(err error) int {
	if err == nil {
		return codeOK
	}
	if errors.Is(err, flag.ErrHelp) {
		return codeOK
	}
	fmt.Fprintln(os.Stderr, "vamoose:", err)
	return codeRuntime
}
