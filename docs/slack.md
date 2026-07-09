<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Slack

Run vamoose from Slack. `/vamoose off next week` creates the hold on your calendar, and the manager approves or declines with a button. Approval through Slack works on any backend, including iCloud, because it does not depend on reading a calendar accept.

## How it works

- `/vamoose <command>` runs the vamoose CLI command and replies with its output. Any command works: `off`, `run`, `away`, `event`, `check`, `promote`, `cancel`, `workflows`.
- When a command creates a hold awaiting approval, the reply carries **Approve** and **Decline** buttons. Approve runs `promote` to notify the team. Decline runs `cancel`.
- Every request is verified against the signing secret, so only Slack can drive it.

The server runs vamoose subcommands as subprocesses, the same pattern as the MCP server, so behavior matches the CLI exactly.

## Set up

This is a single-workspace app. The server runs as your vamoose, with your backend credentials, and drives your calendar.

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps), From scratch.
2. **Slash Commands**: create `/vamoose` with request URL `https://<your-url>/slack/commands`.
3. **Interactivity & Shortcuts**: turn it on with request URL `https://<your-url>/slack/interactivity`.
4. **Basic Information**: copy the **Signing Secret**.
5. Install the app to your workspace.

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

## Scope

The server drives one backend account, so v1 is your personal bridge. Per-user calendars, an "Add to Slack" install flow, and hosting are the later multi-tenant phase.
