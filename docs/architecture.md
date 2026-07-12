---
description: How vamoose is built. Doors (CLI, MCP, Slack, dashboard) in front of one workflow core, with swappable calendar backends.
---

<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Architecture

vamoose is built in three layers with hard boundaries: thin surfaces, a core that holds all logic, and adapters that reach the outside world. The same core runs whether you drive it from the terminal, Claude, or Slack.

## Layers

**Surfaces** parse input and call the core, and hold no business logic:

- the **CLI** (`cmd/`), the primary surface.
- the **MCP server** (`vamoose mcp`, `internal/mcp`) that exposes the commands to Claude.
- the **Slack app** (`vamoose slack`, `internal/slack`) with slash commands and approval buttons.
- the **daemon** (`vamoose daemon`) that advances watched runs on a poll loop.

**Core** is the request lifecycle, the workflow engine, and state. `internal/workflow` holds the workflow model, validation, and the built-in JSON templates. The command layer runs a workflow's steps against a calendar, and the daemon advances a watched run when its gate opens. All logic lives here, so every surface behaves the same.

**Adapters** map the outside world to a neutral model at the boundary, split three ways that are never conflated.

## The three adapters

- **Calendar** creates and reads holds, behind one `Provider` interface in `internal/calendar`. Four backends implement it: Microsoft Graph (`internal/graph`), Google (`internal/google`), and Apple iCloud and any CalDAV host (`internal/caldav`). Each maps its own values to the neutral model. A registry (`cmd/provider.go`) selects one by name, so a new backend is one package and one registration.
- **Directory** resolves your manager and team. Microsoft Graph has one. Google, iCloud, and CalDAV do not, so you pass `--manager` and set the team by hand. On Apple, native approval detection uses macOS EventKit (`internal/eventkit`).
- **Comms** sends a message for a `message` step, behind a `Notifier` interface in `internal/comms`: Slack (`chat.postMessage`) or email (SMTP), routed by the channel.

## The workflow engine

A workflow is a small graph. Steps run in order, but an `approve` step branches on its outcome, any step can redirect with `next`, a `when` guard can skip a step, and a `wait` step pauses for a duration. The executor walks the graph from the creating step until it reaches a gate (an approval or a delay) or the end.

Running with `--watch` records the hold at its gate in a watch list. The daemon polls the list and advances each run: it reads the approver's response, times a `wait` or a `timeout`, invites the next approver in a chain, and runs the remaining steps once the gate opens. A run persists its pending gate, so the daemon can pick it up across restarts.

## Package layout

- `cmd/` is the CLI: command handlers, the provider registry, workflow execution, and the daemon.
- `internal/calendar` is the neutral model and the `Provider` interface. `internal/graph`, `internal/google`, and `internal/caldav` implement it.
- `internal/workflow` is the workflow model, validation, and templates.
- `internal/comms`, `internal/slack`, `internal/mcp`, and `internal/eventkit` are the comms, Slack, MCP, and EventKit adapters.
- `internal/auth` and `internal/googleauth` handle OAuth and token storage.

## Dependencies

vamoose is standard-library only, with two exceptions: the CalDAV backend uses `emersion/go-webdav` and `emersion/go-ical` (hand-rolling iCalendar against iCloud is error-prone), and token storage uses `zalando/go-keyring` for the OS keychain, falling back to a `0600` config file where the keychain is unavailable. Everything else stays stdlib: workflows are JSON, the daemon logs with the standard `log` package, and the MCP and Slack servers speak their protocols by hand. Adding a dependency is a deliberate decision, not a default, which keeps the license path to Apache-2.0 open.
