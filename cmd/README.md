<p align="center"><img src="../assets/vamoose-mapmoose.png" alt="vamoose" width="100%"></p>

# Command reference

Every vamoose command. Run `vamoose <command> -h` for the full flag list.

## Common flags

Most commands accept:

- `--provider`: calendar backend, `graph` (default) or `google`. Overrides `VAMOOSE_PROVIDER`. See [providers](../docs/providers.md).
- `--tz`: IANA time zone for event times. Defaults to `UTC` or `VAMOOSE_TIMEZONE`.
- `--dry-run`: print the plan without calling the calendar (on `run`, `request`, `off`).
- `--id`: target a specific hold, for commands that act on an existing one (`check`, `promote`, `cancel`). Defaults to the most recent hold.

Dates take `YYYY-MM-DD` for an all-day span or RFC3339 for a partial day.

## Workflows

### run

`vamoose run <workflow> [date phrase | --start --end] [flags]`

Run a workflow by name. Creates the first step's hold, runs the immediate steps, and stops at an approval gate. Flags: `--subject`, `--body`, `--manager`, `--attendees` (event workflows), `--watch`, `--dry-run`.

```sh
vamoose run pto next week --watch
vamoose run away --start 2026-07-20 --end 2026-07-24
```

See [workflows](../docs/workflows.md) to write your own.

### workflows

`vamoose workflows`

List the available workflows, built-in and user-defined. User workflows are marked and override built-ins of the same name.

## Time off

The `pto` workflow, with a short front for each step.

### request

`vamoose request --start <start> --end <end> --subject <subject> [flags]`

Create a time-off hold shown free and invite the manager to approve it. Runs the pto workflow. Flags: `--body`, `--manager`, `--watch`, `--dry-run`.

### off

`vamoose off <date phrase>` (or `--start`/`--end`)

Friendly front for request. Understands `today`, `tomorrow`, `next week`, and weekday names.

```sh
vamoose off next week --subject "Out: beach week"
```

### check

`vamoose check [--id] [--promote]`

Report the manager's response to a hold. `--promote` fans out to the team the moment approval lands.

### promote

`vamoose promote [--id] [--force]`

Add the team as optional attendees once approved. `--force` promotes even without approval.

### cancel

`vamoose cancel [--id]`

Delete the hold, notify its attendees, and stop watching it.

## Quick actions

### away

`vamoose away --start <start> --end <end> [--subject]`

Mark yourself out of office over a range. No approval, no fanout.

### event

`vamoose event --start <start> --end <end> --subject <subject> [--attendees a@x,b@y] [--free]`

Create a quick event, optionally inviting attendees. Shown busy unless `--free`.

## Background

### daemon

`vamoose daemon [--interval 1m] [--once]`

Poll watched holds and advance their workflows when the manager responds. `--once` does a single pass and exits.

### service

`vamoose service [--interval 1m] [--label <name>]`

Print a launchd (macOS) or systemd (Linux) manifest to run the daemon unattended. The manifest goes to stdout, so redirect it to a file; install steps print to stderr.

## Integrations

### mcp

`vamoose mcp`

Serve vamoose to Claude over the Model Context Protocol on stdio. See [Claude](../docs/claude.md).

## Info

### whoami

`vamoose whoami`

Print the signed-in user, manager, and resolved team. Run it first to confirm auth and directory access.

### team

`vamoose team [list | set <email...> | clear]`

Show or set your default team, used when the directory lookup is wrong or unavailable.

### version

`vamoose version`

Print the version.
