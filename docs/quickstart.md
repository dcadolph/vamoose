<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Quickstart

Zero to your first approved time-off hold in a few minutes.

## 1. Install

```sh
brew install dcadolph/tap/vamoose
```

Or build from source with Go 1.26 or newer:

```sh
go install github.com/dcadolph/vamoose@latest
```

## 2. Pick a calendar backend

vamoose works with Microsoft Graph (Outlook, Microsoft 365, Teams), Google Calendar, Apple iCloud, and any standard CalDAV host. Set one and export its credentials. Google is the quickest to try:

```sh
export VAMOOSE_PROVIDER=google
export VAMOOSE_GOOGLE_CLIENT_ID=<oauth-desktop-client-id>
export VAMOOSE_GOOGLE_CLIENT_SECRET=<oauth-desktop-client-secret>
export VAMOOSE_TIMEZONE=America/Chicago
```

Every backend's setup is in [providers](providers.md).

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

Add `--watch` and run the daemon in the background; it promotes the team the moment your manager approves:

```sh
vamoose off next week --subject "Out: beach week" --manager boss@work.com --watch
vamoose daemon
```

`vamoose service` prints a launchd or systemd manifest to run the daemon unattended.

## Where next

- [Concepts](concepts.md): how holds, approval, and workflows fit together.
- [Workflows](workflows.md): design your own, with branching, delays, guards, and multi-level approval.
- [Commands](commands.md): every command, flag, and environment variable.
- [Slack](slack.md) and [Claude](claude-guide.md): drive vamoose from where you already work.
