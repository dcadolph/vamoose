<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Providers

vamoose talks to a calendar backend through one `Provider` interface. Two ship today: Microsoft Graph and Google Calendar. Select one with `--provider` or `VAMOOSE_PROVIDER` (default `graph`). Both run the same commands.

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

## Tokens

Tokens are cached per provider under your user config directory and refreshed on use. Run `vamoose whoami` to confirm auth and directory access before creating holds.
