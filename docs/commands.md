<p align="center"><img src="assets/vamoose-mapmoose.png" alt="vamoose" width="100%"></p>

# Commands

Every vamoose command, its flags, and the environment it reads. Run `vamoose <command> -h` for a command's full flag list, and `vamoose doctor` to check your setup.

## Common flags

Most commands accept:

- `--provider`: calendar backend, one of `graph` (default), `google`, `icloud`, or `caldav`. Overrides `VAMOOSE_PROVIDER`. See [providers](providers.md).
- `--tz`: IANA time zone for event times. Defaults to `UTC` or `VAMOOSE_TIMEZONE`.
- `--dry-run`: print the plan without calling the calendar (on `run`, `request`, `off`).
- `--id`: target a specific hold, for commands that act on an existing one (`check`, `promote`, `cancel`). Defaults to the most recent hold.

Dates take `YYYY-MM-DD` for an all-day span or RFC3339 for a partial day.

## Workflows

### run

`vamoose run <workflow> [date phrase | --start --end] [flags]`

Run a workflow by name. Creates the first step's hold, runs the immediate steps, and stops at a gate. Flags: `--subject`, `--body`, `--manager`, `--attendees` (event workflows), `--watch`, `--dry-run`.

```sh
vamoose run pto next week --watch
vamoose run away --start 2026-07-20 --end 2026-07-24
```

See [workflows](workflows.md) to write your own.

### workflows

`vamoose workflows [list | add [--file <path>] | remove <name>]`

Manage workflows. `list` (the default) shows the available workflows, built-in and user-defined, with user workflows marked and overriding built-ins of the same name. `add` saves a user workflow from a JSON definition read from `--file` or stdin, validating it and taking its name from the definition. `remove` deletes a user workflow.

```sh
cat team-heads-up.json | vamoose workflows add
vamoose workflows remove team-heads-up
```

See [workflows](workflows.md) for the definition format.

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

`vamoose daemon [--interval 1m] [--once] [--prune]`

Poll watched holds and advance their workflows when the manager responds or a delay passes. `--once` does a single pass and exits. `--prune` drops watched holds whose provider is no longer configured.

### schedule

`vamoose schedule [add <workflow> --every <dur> --phrase <window> | list | remove <index>]`

Rerun a workflow on an interval. `add` schedules it, `list` shows the schedules with their index, and `remove` drops one. The daemon fires due schedules, so run `vamoose daemon`. Add flags: `--subject`, `--manager`, `--provider`.

```sh
vamoose schedule add pto --every 168h --phrase "next week" --manager boss@work.com
```

### service

`vamoose service [--interval 1m] [--label <name>]`

Print a launchd (macOS) or systemd (Linux) manifest to run the daemon unattended. The manifest goes to stdout, so redirect it to a file. Install steps print to stderr.

## Integrations

### mcp

`vamoose mcp`

Serve vamoose to Claude over the Model Context Protocol on stdio. See [Claude](claude-guide.md).

### slack

`vamoose slack [--addr :8080]`

Serve the vamoose Slack app: run vamoose from slash commands, with Approve and Decline buttons. Needs `VAMOOSE_SLACK_SIGNING_SECRET` and a public URL. See [Slack](slack.md).

## Info

### login

`vamoose login [--provider <name>]`

Sign in to the selected calendar provider and confirm access. For Google it uses the built-in OAuth client, so no Cloud project is needed: your browser opens once for consent and the token is cached for later commands. Graph signs in with the device-code prompt; iCloud and CalDAV verify the credentials you exported.

### whoami

`vamoose whoami`

Print the signed-in user, manager, and resolved team. Run it after `vamoose login` to confirm auth and directory access.

### team

`vamoose team [list | set <email...> | clear]`

Show or set your default team, used when the directory lookup is wrong or unavailable.

### calendars

`vamoose calendars [list | create <name>]`

List or create calendars. iCloud and CalDAV only. Useful to create a dedicated calendar so vamoose never touches your main one.

### doctor

`vamoose doctor`

Check your configuration and report what is set up or missing: the selected provider's credentials, time zone, and the optional Slack and email backends for message steps.

### version

`vamoose version`

Print the version.

## Environment variables

Selection:

| Variable            | Purpose                                                          &nbsp; |
| ------------------- | ----------------------------------------------------------------------- |
| `VAMOOSE_PROVIDER`  | Calendar backend: `graph`, `google`, `icloud`, or `caldav`.             |
| `VAMOOSE_TIMEZONE`  | IANA time zone for event times. Default `UTC`.                          |

Microsoft Graph:

| Variable            | Purpose                                                          &nbsp; |
| ------------------- | ----------------------------------------------------------------------- |
| `VAMOOSE_CLIENT_ID` | Entra application (client) id.                                          |
| `VAMOOSE_TENANT`    | Entra tenant id, or `organizations`. Default `organizations`.           |

Google Calendar (a built-in OAuth client is used by default; set both to bring your own):

| Variable                       | Purpose                                               &nbsp; |
| ------------------------------ | ------------------------------------------------------------ |
| `VAMOOSE_GOOGLE_CLIENT_ID`     | OAuth desktop client id. Optional override.                  |
| `VAMOOSE_GOOGLE_CLIENT_SECRET` | OAuth desktop client secret. Optional override.              |

Apple iCloud:

| Variable                      | Purpose                                                &nbsp; |
| ----------------------------- | ------------------------------------------------------------- |
| `VAMOOSE_ICLOUD_USERNAME`     | Apple ID email.                                               |
| `VAMOOSE_ICLOUD_APP_PASSWORD` | App-specific password from appleid.apple.com.                 |
| `VAMOOSE_ICLOUD_CALENDAR`     | Target calendar name. Optional, default the first writable.   |

Generic CalDAV:

| Variable                   | Purpose                                                   &nbsp; |
| -------------------------- | ---------------------------------------------------------------- |
| `VAMOOSE_CALDAV_URL`       | CalDAV server URL, such as `https://caldav.fastmail.com`.        |
| `VAMOOSE_CALDAV_USERNAME`  | Account username.                                                |
| `VAMOOSE_CALDAV_PASSWORD`  | Account password or app-specific password.                       |
| `VAMOOSE_CALDAV_CALENDAR`  | Target calendar name. Optional, default the first writable.      |

Messaging (for `message` steps, optional):

| Variable                   | Purpose                                                   &nbsp; |
| -------------------------- | ---------------------------------------------------------------- |
| `VAMOOSE_SLACK_BOT_TOKEN`  | Slack bot token with `chat:write`, for messages to Slack.       |
| `VAMOOSE_SMTP_HOST`        | SMTP server host, for messages to email.                        |
| `VAMOOSE_SMTP_PORT`        | SMTP port. Default `587`.                                        |
| `VAMOOSE_SMTP_USERNAME`    | SMTP username.                                                   |
| `VAMOOSE_SMTP_PASSWORD`    | SMTP password.                                                   |
| `VAMOOSE_SMTP_FROM`        | Sender address.                                                  |

The Slack server also reads `VAMOOSE_SLACK_SIGNING_SECRET` and, for install and per-user mode, `VAMOOSE_SLACK_CLIENT_ID`, `VAMOOSE_SLACK_CLIENT_SECRET`, and `VAMOOSE_SLACK_PUBLIC_URL`. See [Slack](slack.md).

## Files and storage

- **Tokens** are stored in the OS keychain when it is reachable, otherwise a `0600` file under your config directory. They refresh automatically. No setup needed. On a server, set `VAMOOSE_SECRET_KEY` (a base64 32-byte key, from `openssl rand -base64 32`) and tokens and per-user links are sealed with AES-256-GCM at rest instead. See [hosting](hosting.md).
- **Config directory** is the OS user config directory, `vamoose/` within it: `~/.config/vamoose` on Linux, `~/Library/Application Support/vamoose` on macOS.
- **Watch state** for `--watch` holds is `watches.json` in the config directory, or the path in `VAMOOSE_WATCH_FILE` when set (the Slack server uses this to give each linked user their own file).
- **Schedules** from `vamoose schedule` are `schedules.json` in the config directory, which the daemon reads to fire recurring runs.
