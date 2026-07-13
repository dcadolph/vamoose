---
title: Slack approval buttons for time off
description: Drive vamoose from Slack. Slash commands, approval buttons bound to the real approver, per-user calendar linking, and background auto-advance.
---

<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Slack

Run vamoose from Slack. `/vamoose off next week` creates the hold on your calendar, and the manager approves or declines with a button. Approval through Slack works on any backend, including iCloud, because it does not depend on reading a calendar accept.

## How it works

- `/vamoose <command>` runs the vamoose CLI command and replies with its output. Calendar and workflow commands work: `off`, `request`, `away`, `event`, `run`, `workflows`, `check`, `schedule`, `balance`, `coverage`, `team`, `calendars`, `doctor`. Server and host commands (`daemon`, `slack`, `mcp`, `login`) and the approval actions (`promote`, `cancel`) are not reachable from a slash command; approvals happen only through the buttons.
- When a command creates a hold awaiting approval, the reply carries **Approve** and **Decline** buttons. Approve runs `promote` to notify the team. Decline runs `cancel`. Only the approver the hold was sent to can use the buttons, and the button value is signed, so a click cannot be forged or made by anyone else.
- Every request is verified against the signing secret, so only Slack can drive it.

The server runs vamoose subcommands as subprocesses, the same pattern as the MCP server, so behavior matches the CLI exactly.

## Set up

This is a single-workspace app. The server runs as your vamoose, with your backend credentials, and drives your calendar.

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps), From scratch.
2. **Slash Commands**: create `/vamoose` with request URL `https://<your-url>/slack/commands`.
3. **Interactivity & Shortcuts**: turn it on with request URL `https://<your-url>/slack/interactivity`.
4. **Basic Information**: copy the **Signing Secret**.
5. **Basic Information → Display Information**: upload `assets/vamoose-slack-icon.png` as the app icon. It is the white moose on black, square at 1024x1024, so Slack shows it on the bot's messages and in the app directory.
6. Install the app to your workspace.

Run the server with your calendar backend and the signing secret set:

```sh
export VAMOOSE_SLACK_SIGNING_SECRET=<signing-secret>
export VAMOOSE_PROVIDER=google   # or graph, or icloud (see providers)
vamoose slack --addr :8080
```

Slack needs a public HTTPS URL. For development, expose the local server with a tunnel and use its HTTPS URL as `<your-url>` above:

```sh
cloudflared tunnel --url http://localhost:8080
# or: ngrok http 8080
```

## Distributable install (Add to Slack)

To let any workspace add vamoose, set the app's OAuth credentials and a public URL:

```sh
export VAMOOSE_SLACK_CLIENT_ID=<client-id>
export VAMOOSE_SLACK_CLIENT_SECRET=<client-secret>
export VAMOOSE_SLACK_PUBLIC_URL=https://<your-host>
```

In the Slack app under **OAuth & Permissions**, set the redirect URL to `https://<your-host>/slack/oauth/callback` and the bot scopes to `commands`, `chat:write`, and `users:read.email`. The email scope lets the server map an approver's email to their Slack user so it can verify that only the approver clicks an approval button. Then point people at `https://<your-host>/slack/install`, or an "Add to Slack" button linking there. Each install stores that workspace's bot token.

Without these variables, the server runs single-workspace as above.

## Per-user mode (multi-tenant)

In per-user mode every Slack user links their **own** calendar, and each command runs against that user's calendar. One server, many users, each with their own provider and credentials.

Enable it alongside the install flow (the iCloud modal needs the workspace bot token an install provides):

```sh
export VAMOOSE_SLACK_PER_USER=1
export VAMOOSE_SLACK_PUBLIC_URL=https://<your-host>
# Google linking (web OAuth client):
export VAMOOSE_GOOGLE_CLIENT_ID=<web-client-id>
export VAMOOSE_GOOGLE_CLIENT_SECRET=<web-client-secret>
# Microsoft 365 linking (confidential web app):
export VAMOOSE_CLIENT_ID=<entra-app-id>
export VAMOOSE_GRAPH_CLIENT_SECRET=<entra-web-secret>
export VAMOOSE_TENANT=organizations
```

Add `https://<your-host>/slack/link/callback` as a redirect URL on the Google OAuth client and the Entra web app. iCloud needs no server credentials.

**Linking.** Each user links once:

- `/vamoose link google` or `/vamoose link graph` replies with a consent link. After they authorize, vamoose stores their refresh token.
- `/vamoose link icloud` opens a modal for an Apple ID and app-specific password, kept out of the channel.
- `/vamoose unlink` removes their link.

**Running.** After linking, `/vamoose off next week`, and every other command, runs against that user's calendar. Unlinked users are told to link first. Approve and Decline buttons run as the hold's owner, so they touch the requester's calendar, not the clicker's. Only the approver the hold was sent to may click: the server resolves their email to a Slack user through `users:read.email` and refuses anyone else. If it cannot resolve the approver, because the scope is missing or the approver is outside the workspace, it withholds the buttons, and the approver instead approves by accepting the calendar invite, which the poll loop advances.

**Auto-advance.** The server polls each linked user's watched holds on an interval and advances them with that user's credentials, so `--watch` flows finish without anyone clicking a button. A fresh access token is minted on each poll.

Per-user mode is opt-in. Without `VAMOOSE_SLACK_PER_USER`, the server runs single-workspace as above.

## Scope

Per-user mode is experimental and not yet vetted against a live Slack workspace. The standalone `vamoose daemon` advances the CLI's own watches. Per-user watches are advanced by the Slack server's poll loop instead, since only it holds each user's credentials.
