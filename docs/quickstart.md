---
description: Install vamoose with Homebrew, sign in to Google or Microsoft 365, and get a first approved time-off hold in minutes.
---

<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Quickstart

Zero to your first approved time-off hold in a few minutes.

## 1. Install

macOS and Linux:

```sh
brew install dcadolph/tap/vamoose
```

Windows:

```powershell
scoop bucket add vamoose https://github.com/dcadolph/scoop-vamoose
scoop install vamoose
```

Or build from source with Go 1.26 or newer:

```sh
go install github.com/dcadolph/vamoose@latest
```

Prebuilt zips and tarballs for every platform are on the
[releases page](https://github.com/dcadolph/vamoose/releases).

## 2. Sign in

vamoose works with Microsoft Graph (Outlook, Microsoft 365, Teams), Google Calendar, Apple iCloud, and any standard CalDAV host. Google is the quickest to try, and it needs no project of your own: vamoose ships with a built-in OAuth client, so you just sign in.

```sh
export VAMOOSE_PROVIDER=google
export VAMOOSE_TIMEZONE=America/Chicago
vamoose login
```

Your browser opens once to grant access, and the token is cached for later commands. Prefer your own OAuth client, or running an enterprise Google Workspace that allowlists apps? Export `VAMOOSE_GOOGLE_CLIENT_ID` and `VAMOOSE_GOOGLE_CLIENT_SECRET` and vamoose uses those instead. Every backend's setup is in [providers](providers.md).

## 3. Check your setup

```sh
vamoose doctor
```

It reports what is configured and what is missing, so setup is a checklist, not a guess. Then confirm access:

```sh
vamoose whoami
```

This prints the signed-in user and, where the backend has a directory, your manager and team. Google, iCloud, and CalDAV have no directory, so pass your approver with `--manager` and set your team with `vamoose team set`.

## 4. Request time off

Create a hold shown free, so it blocks nobody, and invite your manager to approve it:

```sh
vamoose off next week --subject "Out: beach week" --manager boss@work.com
```

See whether they have accepted:

```sh
vamoose check
```

Once approved, fan out to the team as optional attendees, so everyone sees you are out without their calendars getting blocked:

```sh
vamoose promote
```

That is the built-in `pto` workflow: hold, approve, notify.

## 5. Let it advance on its own

Add `--watch` and run the daemon in the background. It promotes the team the moment your manager approves:

```sh
vamoose off next week --subject "Out: beach week" --manager boss@work.com --watch
vamoose daemon
```

`vamoose service` prints a launchd or systemd manifest to run the daemon unattended.

## 6. Prefer clicking?

`vamoose app` opens a local dashboard: run workflows, design them on a node canvas with drag-to-wire branches, act on watched holds, manage schedules, and read the run history. On Windows and Linux, `vamoose tray` puts the moose in the system tray to act on holds from a dropdown and keep the daemon running for you. On macOS, `make tray` builds the native menu bar version.

## Where next

| Guide                            | What's in it                                             &nbsp; |
| -------------------------------- | ---------------------------------------------------------------- |
| [Concepts](concepts.md)          | How holds, approval, and workflows fit together.                 |
| [Workflows](workflows.md)        | Design your own, with branching, delays, guards, and approvers.  |
| [Commands](commands.md)          | Every command, flag, and environment variable.                   |
| [Slack](slack.md)                | Run vamoose from slash commands, with approval buttons.          |
| [Claude](claude-guide.md)        | Drive and author workflows from Claude over MCP.                 |
