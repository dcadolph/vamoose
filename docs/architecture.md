<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Architecture

vamoose is built in layers with hard boundaries.

## Surfaces

Thin clients with no business logic: the CLI, the MCP server (`vamoose mcp`), the Slack app (`vamoose slack`), and the Claude skill. A surface parses input and calls the core.

## Core

The request lifecycle, the workflow engine, and state. `internal/workflow` holds the workflow model, validation, and built-in templates. The command layer runs a workflow's steps, and the daemon advances a watched run when the manager responds. All logic lives here.

## Adapters

Calendar backends behind one `Provider` interface in `internal/calendar`: Microsoft Graph, Google Calendar, and Apple iCloud over CalDAV, each mapping its own values to a neutral model at the boundary. A registry selects one by name, so a new backend is one package and one registration.

## Dependencies

vamoose is standard-library only, with one exception: the CalDAV backend uses `emersion/go-webdav` and `emersion/go-ical`, because hand-rolling iCalendar against iCloud is error-prone. Everything else stays stdlib. Workflows are JSON, the daemon logs with the standard `log` package, and the MCP server speaks JSON-RPC by hand. Adding a dependency is a deliberate decision, not a default.
