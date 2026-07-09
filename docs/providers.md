<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Providers

vamoose talks to a calendar backend through one `Provider` interface. Three ship today: Microsoft Graph, Google Calendar, and Apple iCloud. Select one with `--provider` or `VAMOOSE_PROVIDER` (default `graph`). They run the same commands.

## Microsoft Graph (Outlook, Microsoft 365, Teams)

vamoose acts as you through the OAuth device-code flow.

1. Register an application in the Microsoft Entra admin center. Enable **Allow public client flows** so device code works.
2. Grant these delegated permissions with admin consent: `User.Read`, `User.Read.All`, `Calendars.ReadWrite`, `MailboxSettings.ReadWrite`, `offline_access`.
3. Export the settings:

```sh
export VAMOOSE_CLIENT_ID=<application-client-id>
export VAMOOSE_TENANT=<tenant-id-or-organizations>
export VAMOOSE_TIMEZONE=America/Chicago
```

The first command opens a device-code prompt. Tokens cache under your config directory and refresh automatically.

## Google Calendar

Use `--provider google`. Create an OAuth **desktop app** client in the Google Cloud console, enable the Google Calendar API, and export:

```sh
export VAMOOSE_PROVIDER=google
export VAMOOSE_GOOGLE_CLIENT_ID=<oauth-desktop-client-id>
export VAMOOSE_GOOGLE_CLIENT_SECRET=<oauth-desktop-client-secret>
```

The first command opens your browser for consent on a local loopback address, then caches and refreshes tokens.

Google Calendar has no directory, so pass your approver with `--manager` and set your team with `vamoose team set`. Add the signing-in account under Google Auth Platform, Audience, Test users, or consent is denied.

## Apple iCloud (CalDAV)

Use `--provider icloud`. iCloud speaks CalDAV. Create an app-specific password at [appleid.apple.com](https://appleid.apple.com) (your Apple ID must have two-factor auth on), then export:

```sh
export VAMOOSE_PROVIDER=icloud
export VAMOOSE_ICLOUD_USERNAME=you@icloud.com
export VAMOOSE_ICLOUD_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx
```

Set a target calendar with `VAMOOSE_ICLOUD_CALENDAR="Home"`. The default is the first calendar that accepts events. Like Google, iCloud has no directory, so pass your approver with `--manager` and set your team with `vamoose team set`.

**Approval on iCloud.** iCloud creates the hold and emails the manager the invite, but it does not report the manager's accept or decline back over CalDAV. Two ways still get you approval:

- **macOS EventKit.** Build the helper with `make eventkit`. vamoose then reads the manager's accept or decline from your local Calendar.app, so `check` and the daemon detect approval on iCloud too. Grant calendar access on the first run.
- **Slack.** Approve or decline with a button in Slack, which works regardless of backend. See [Slack](slack.md).

Without either, promote by hand once you know the manager accepted.

## Tokens

Tokens are cached per provider under your user config directory and refreshed on use. Run `vamoose whoami` to confirm auth and directory access before creating holds.
