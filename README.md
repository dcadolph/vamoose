<p align="center">
  <img src="assets/vamoose-banner.png" alt="vamoose" width="100%">
</p>

<h1 align="center">vamoose</h1>

<p align="center">Calendar workflows, minus the tedium.</p>

The moose does the paperwork. You go to the beach.

> **Status: v0.2.0, early.**
> The core flow runs live on Google Calendar: sign-in, request, manager approval,
> team promote, quick actions, and the watch daemon. The v0.2.0 JSON workflow engine
> (`vamoose run`, custom workflows, a workflow-driven daemon) is unit-tested end to
> end, but its run and daemon-advance path has not been live-vetted yet. The Microsoft
> Graph path is unit-tested but hasn't been run against a live Microsoft 365 tenant yet.

Calendar busywork is death by a thousand cuts: block the dates, ping your manager,
wait for a nod, then re-send the event to the team so nobody schedules over you.
vamoose turns those chores into **workflows** it runs for you and advances in the
background. Time off is the flagship workflow, and you can define your own.

<p align="center"><img src="assets/vamoose-demo.gif" alt="vamoose demo" width="100%"></p>

## How it works

1. `vamoose request` creates a calendar event over your dates, shown as **free**
   so it blocks nobody, and invites your **manager** as a required attendee.
2. Your manager **accepts the invite**. That acceptance is the approval signal.
   There is no separate approval product to buy or install.
3. `vamoose promote` adds your whole **team as optional attendees** and resends,
   so everyone sees you are out without their calendars getting blocked.

These three steps are the built-in **pto** workflow, and `request`, `check`, and
`promote` are fronts over its steps. vamoose runs other workflows too, and you can
define your own. See [Workflows](#workflows).

Two backends ship behind one provider interface: Microsoft Graph (Outlook,
Microsoft 365, and Teams) and Google Calendar. Pick one with `--provider` or the
`VAMOOSE_PROVIDER` environment variable, and every command works the same on both.

## Why not just calendar rules?

You can rig a version of this with one calendar's rules or a saved email. vamoose
earns its keep the moment you have more than one account:

- **One brain for every account.** The same workflows, commands, and setup whether
  you are on Google, Outlook, or Microsoft 365. Learn it once, not once per client.
- **No rebuilding per client.** Native rules live inside one app and stop at its edge.
  Define a workflow once and point it at any backend with `--provider`.
- **No drift.** Change your time-off flow in one place. Hand-built rules drift the day
  you update Outlook and forget Gmail.
- **Workflows are files, not clicks.** JSON you can read, diff, share, version, and
  dry-run, instead of a settings panel you rebuild by hand on every machine.
- **Runs where you already are.** The CLI, Claude, and (soon) Slack, not one vendor's
  web UI.

## Setup

### Microsoft 365 / Outlook

vamoose talks to Microsoft Graph as you, using the OAuth device-code flow.

1. Register an application in the Microsoft Entra admin center (single tenant is
   fine). Enable **Allow public client flows** so device code works.
2. Grant these delegated permissions and admin consent:
   - `User.Read`, `User.Read.All` (read your manager and their direct reports)
   - `Calendars.ReadWrite` (create and update the hold)
   - `MailboxSettings.ReadWrite` (reserved for the out-of-office reply)
   - `offline_access` (stay signed in between runs)
3. Export the settings:

```sh
export VAMOOSE_CLIENT_ID=<application-client-id>
export VAMOOSE_TENANT=<tenant-id-or-organizations>
export VAMOOSE_TIMEZONE=America/Chicago
```

The first command opens a device-code prompt. Tokens are cached under your user
config directory and refreshed automatically after that. Run `vamoose whoami`
first to confirm auth and directory access before creating any holds.

### Google Calendar

For `--provider google`, create an OAuth **desktop app** client in the Google Cloud
console, enable the Google Calendar API, and export its credentials:

```sh
export VAMOOSE_PROVIDER=google
export VAMOOSE_GOOGLE_CLIENT_ID=<oauth-desktop-client-id>
export VAMOOSE_GOOGLE_CLIENT_SECRET=<oauth-desktop-client-secret>
```

The first command opens your browser for consent on a local loopback address, then
caches and refreshes tokens after that. Google Calendar has no directory, so pass
your approver with `--manager` and set your team with `vamoose team set`.

## Usage

```sh
# Confirm auth and directory access work in your tenant.
vamoose whoami

# Create the hold and invite your manager. Manager is resolved from the directory.
vamoose request --start 2026-07-20 --end 2026-07-24 --subject "Out: beach week"

# Or request time off from a plain-language phrase.
vamoose off next week --subject "Out: beach week"

# See whether your manager has approved.
vamoose check

# Once approved, fan out to the team as optional attendees.
vamoose promote

# Changed plans? Cancel the hold and notify everyone.
vamoose cancel

# Or let check promote the moment approval lands.
vamoose check --promote

# Hands-off: watch for approval and let the daemon auto-promote in the background.
vamoose off next week --watch
vamoose daemon

# Run the daemon unattended (prints a launchd or systemd manifest to install).
vamoose service
```

Times accept `YYYY-MM-DD` for all-day holds or RFC3339 for partial days. Pass
`--manager you@work.com` to skip directory lookup, or `--dry-run` on request to
preview without sending. `off` also accepts explicit `--start`/`--end`.

## Quick actions

Not everything needs approval:

```sh
# Block yourself out of office over a range, no approval or fanout.
vamoose away --start 2026-07-20 --end 2026-07-24

# Create a quick event, optionally inviting others.
vamoose event --start 2026-07-20T15:00:00Z --end 2026-07-20T15:30:00Z \
  --subject "1:1" --attendees boss@work.com
```

## Workflows

A workflow is an ordered list of steps that vamoose runs and the daemon advances.
The request-approve-promote flow above is the built-in **pto** workflow. Run a
workflow by name, with a date phrase or explicit `--start`/`--end`:

```sh
vamoose run pto next week --watch
vamoose run notify-only next week
vamoose run away --start 2026-07-20 --end 2026-07-24
vamoose workflows            # list the available workflows
```

Three workflows ship built in:

| Name          | Steps                                | Use                               &nbsp; |
| ------------- | ------------------------------------ | ---------------------------------------- |
| `pto`         | hold shown free, approve, notify     | Time off that a manager approves.        |
| `notify-only` | hold shown free, notify              | Tell the team, no approval needed.       |
| `away`        | out-of-office block                  | Personal out of office, no fanout.       |

Define your own by dropping a JSON file in `~/.config/vamoose/workflows/<name>.json`.
A file there overrides a built-in of the same name.

```json
{
  "name": "team-heads-up",
  "description": "Hold shown free, tell the team, no approval.",
  "steps": [
    { "verb": "hold", "showAs": "free" },
    { "verb": "notify", "team": "optional" }
  ]
}
```

Then `vamoose run team-heads-up next week`. Steps use these verbs:

- `hold` creates the event and invites the manager when an `approve` step follows.
- `approve` waits for the manager to accept the invite.
- `notify` adds the team as optional attendees.
- `away` marks you out of office with no attendees.
- `event` creates a plain event, with attendees from `--attendees`.
- `cancel` deletes the hold.

A workflow starts with exactly one creating step (`hold`, `away`, or `event`).
Approval waits on the manager that only a `hold` invites, so an `approve` step
needs a `hold`, and only `notify` may follow approval. With `--watch`, the hold is
recorded and `vamoose daemon` runs the remaining steps once the manager accepts.

## Defining your team

By default `promote` derives your team from the directory: your manager's direct
reports, minus you. That assumption breaks if you are the manager, your team is a
distribution list, or the directory is sparse. Set an explicit team instead:

```sh
vamoose team set alex@work.com jordan@work.com sam@work.com
vamoose team list     # show the current team
vamoose team clear    # revert to the directory
```

The list is stored as JSON under your user config directory
(`team.json`). When it is set, `promote` and `whoami` use it; when it is absent,
they fall back to the directory.

## Claude (MCP)

`vamoose mcp` speaks the Model Context Protocol over stdio, exposing the commands as
tools so Claude can book time off for you. Point an MCP client at the binary:

```json
{ "mcpServers": { "vamoose": { "command": "vamoose", "args": ["mcp"] } } }
```

Authenticate once first with `vamoose whoami`; the server reuses the cached token.

## Docs

- [Command reference](cmd/README.md): every command and its flags.
- [Workflows](docs/workflows.md): built-in and custom workflows.
- [Providers](docs/providers.md): Microsoft Graph and Google Calendar setup.
- [Claude](docs/claude.md): the MCP server and the skill.
- [Architecture](docs/architecture.md): surfaces, core, and adapters.

## Roadmap

- Conditional and branching workflows: if-this-then-that and multi-step chains
  beyond the built-in time-off flow.
- Auto-promote via Graph change-notification webhooks instead of polling.
- Store tokens in the OS keychain rather than a config file.
- Set the scheduled out-of-office auto-reply for the time-off window.
- Harden the CLI and auth with cobra and MSAL.

## Status

v0.2.0. The core flow runs live on Google Calendar end to end. The v0.2.0 workflow
engine (run, custom workflows, a workflow-driven daemon) is unit-tested, but its run
and daemon-advance path has not been live-vetted yet. The Microsoft Graph path is
unit-tested but hasn't hit a live tenant yet.
