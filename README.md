# vamoose

Vacation holds, minus the tedium.

Booking time off by hand is a chore: block the dates, ping your manager, wait for
a nod, then re-send the event to the team so nobody schedules over you. vamoose
runs that whole loop from one command.

## How it works

1. `vamoose request` creates a calendar event over your dates, shown as **free**
   so it blocks nobody, and invites your **manager** as a required attendee.
2. Your manager **accepts the invite**. That acceptance is the approval signal.
   There is no separate approval product to buy or install.
3. `vamoose promote` adds your whole **team as optional attendees** and resends,
   so everyone sees you are out without their calendars getting blocked.

The first backend is Microsoft Graph, which covers Outlook, Microsoft 365, and
Teams calendars through one API. The provider is an interface, so Google Calendar
and others can slot in behind the same three commands.

## Setup

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
config directory and refreshed automatically after that.

## Usage

```sh
# Create the hold and invite your manager. Manager is resolved from the directory.
vamoose request --start 2026-07-20 --end 2026-07-24 --subject "Out: beach week"

# See whether your manager has approved.
vamoose check

# Once approved, fan out to the team as optional attendees.
vamoose promote

# Or let check promote the moment approval lands.
vamoose check --promote
```

Times accept `YYYY-MM-DD` for all-day holds or RFC3339 for partial days. Pass
`--manager you@work.com` to skip directory lookup, or `--dry-run` on request to
preview without sending.

## Roadmap

- Auto-promote via Graph change-notification webhooks instead of polling.
- Store tokens in the OS keychain rather than a config file.
- Set the scheduled out-of-office auto-reply for the vacation window.
- Google Calendar provider behind the same interface.
- Harden the CLI and auth with cobra and MSAL.

## Status

Early. The Graph flow is wired end to end and unit tested; treat it as a working
first slice, not a finished product.
