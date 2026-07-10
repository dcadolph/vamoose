<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Providers

vamoose talks to a calendar backend through one `Provider` interface. Four ship today: Microsoft Graph, Google Calendar, Apple iCloud, and any standard CalDAV host. Select one with `--provider` or `VAMOOSE_PROVIDER` (default `graph`). They run the same commands.

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

Use `--provider google`, then sign in:

```sh
export VAMOOSE_PROVIDER=google
vamoose login
```

vamoose ships with a built-in OAuth desktop client, so there is no Google Cloud project to create. `login` opens your browser for consent on a local loopback address, then caches and refreshes tokens.

**Bring your own client.** Prefer your own OAuth client, self-hosting, or running an enterprise Google Workspace that allowlists third-party apps? Create an OAuth **desktop app** client in the Google Cloud console, enable the Google Calendar API, and export it; vamoose then uses your client instead of the built-in one:

```sh
export VAMOOSE_GOOGLE_CLIENT_ID=<oauth-desktop-client-id>
export VAMOOSE_GOOGLE_CLIENT_SECRET=<oauth-desktop-client-secret>
```

For your own unverified client, add the signing-in account under Google Auth Platform, Audience, Test users, or consent is denied.

Google Calendar has no directory, so pass your approver with `--manager` and set your team with `vamoose team set`.

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

## Any CalDAV host (Fastmail, Nextcloud, and more)

Use `--provider caldav` for any standard CalDAV server. Point it at the server URL and pass your account credentials, using an app-specific password where the host offers one:

```sh
export VAMOOSE_PROVIDER=caldav
export VAMOOSE_CALDAV_URL=https://caldav.fastmail.com
export VAMOOSE_CALDAV_USERNAME=you@fastmail.com
export VAMOOSE_CALDAV_PASSWORD=xxxx-xxxx-xxxx-xxxx
```

Set a target calendar with `VAMOOSE_CALDAV_CALENDAR="Work"`. The default is the first calendar that accepts events. Like Google and iCloud, a CalDAV host has no directory, so pass your approver with `--manager` and set your team with `vamoose team set`.

Unlike iCloud, a standard CalDAV host reports the manager's accept or decline over CalDAV, so `check` and the daemon detect approval with no extra setup.

## Tokens

Tokens are cached per provider under your user config directory and refreshed on use. Run `vamoose login` to sign in and cache a token, then `vamoose whoami` to confirm auth and directory access before creating holds.
