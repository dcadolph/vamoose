<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Hosting

Run vamoose as an always-on service. One process serves everything: the Slack server handles slash commands and buttons, and its poll loop advances each linked user's watched holds and fires recurring schedules. No laptop required.

## Encrypt secrets at rest

On a server the OS keychain is not available, so set an encryption key and vamoose seals tokens and per-user links with AES-256-GCM instead of leaving them in a plaintext file. Generate one key and keep it safe:

```sh
openssl rand -base64 32
```

Set it as `VAMOOSE_SECRET_KEY`. Without it, the store falls back to a `0600` file. Run `vamoose doctor` to confirm which is in effect.

## Run with Docker

The repository ships a `Dockerfile` and a `docker-compose.yml`. Put your settings in a `.env` file next to the compose file:

```sh
VAMOOSE_SECRET_KEY=<from openssl rand -base64 32>
VAMOOSE_SLACK_SIGNING_SECRET=...
VAMOOSE_SLACK_CLIENT_ID=...
VAMOOSE_SLACK_CLIENT_SECRET=...
VAMOOSE_SLACK_PUBLIC_URL=https://vamoose.example.com
VAMOOSE_GOOGLE_CLIENT_ID=...
VAMOOSE_GOOGLE_CLIENT_SECRET=...
VAMOOSE_TIMEZONE=America/Chicago
```

Then:

```sh
docker compose up -d
```

The server listens on port 8080. A named volume persists the encrypted tokens, links, and watch state across restarts, so a redeploy does not lose anyone's link.

## Public URL

Slack needs a public HTTPS URL for its slash command, interactivity, and OAuth callbacks. Put the server behind a reverse proxy or a tunnel and set `VAMOOSE_SLACK_PUBLIC_URL` to it. Point the Slack app's request URLs at `/slack/commands`, `/slack/interactivity`, and the install and link callbacks under the same host. See [Slack](slack.md) for the per-user setup.

## Without Docker

`vamoose service` prints a launchd (macOS) or systemd (Linux) manifest to run the same server unattended from a binary. Set the same environment, including `VAMOOSE_SECRET_KEY`, in the service's environment.

## Storage and scale

By default, tokens, per-user links, and watch state are files under the config directory, encrypted when a key is set. Set `VAMOOSE_DB_PATH` to back the server's per-workspace tokens and per-user links with a single embedded database (bbolt) instead, for atomic transactional writes and multi-tenant scale past what a rewritten JSON file handles well. The server owns this database; keep `VAMOOSE_DB_PATH` set only on the long-running server, not on ad-hoc CLI commands, since the database takes an exclusive file lock. Values in it are encrypted at rest with the same `VAMOOSE_SECRET_KEY`.

Run history and watch state stay in files even with `VAMOOSE_DB_PATH` set, because the shelled-out per-user commands write them concurrently. The daemon writes watch progress after each step, so a crash resumes mid-workflow rather than replaying it.

## Observability

The server serves Prometheus metrics at `/metrics` (dispatched commands, approval actions, rejected actions, command errors, and installs) and a liveness check at `/health`. Point a scraper at `/metrics`.

Set `VAMOOSE_LOG_FORMAT=json` for machine-parseable structured logs, and `VAMOOSE_LOG_LEVEL` (`debug`, `info`, `warn`, `error`, default `info`) to set verbosity. Events carry the workspace, user, command, and outcome as fields, so a command run, an approval, or a rejected click is one queryable line.

## Filing real leave (BambooHR)

A `leave` step files the time off as a real leave request in your HR system, so the system of record matches the calendar rather than only a calendar hold. BambooHR is the first system:

```sh
export VAMOOSE_BAMBOOHR_SUBDOMAIN=<company>      # from your BambooHR URL
export VAMOOSE_BAMBOOHR_API_KEY=<api-key>        # a BambooHR API key
export VAMOOSE_BAMBOOHR_EMPLOYEE_ID=<id>         # the employee taking leave
export VAMOOSE_BAMBOOHR_TYPE_ID=<time-off-type>  # the BambooHR time-off type id
export VAMOOSE_BAMBOOHR_STATUS=requested         # or approved, default requested
```

The built-in `pto-file-leave` workflow files leave once the manager approves, then notifies the team. Without these variables, a `leave` step reports that no HR system is configured, so add the step only when the HR system is set up.

For any other system, point a `leave` step at a webhook instead of BambooHR and receive the leave with your own glue, such as Zapier, n8n, or a small endpoint:

```sh
export VAMOOSE_LEAVE_WEBHOOK_URL=https://hooks.example.com/leave
export VAMOOSE_LEAVE_WEBHOOK_AUTH="Bearer <token>"   # optional, sent as Authorization
```

vamoose posts a JSON body with `employee_id`, `type_id`, `start`, `end`, and `note`. BambooHR is used when its variables are set; otherwise the webhook is used.
