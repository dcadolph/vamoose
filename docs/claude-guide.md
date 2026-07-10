<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# vamoose with Claude

Drive vamoose from Claude two ways: the MCP server for direct tool calls, and the skill for a guided time-off flow.

## MCP server

`vamoose mcp` speaks the Model Context Protocol over stdio, exposing the commands as tools so Claude can book time off, check approval, and fan out to the team on your behalf. Each tool shells out to the vamoose binary, so behavior matches the CLI exactly.

### Setup

Sign in once so the server reuses your cached token. This is interactive (it may open a browser), which an MCP subprocess cannot do, so run it yourself first:

```sh
vamoose login
```

Then point an MCP client at the binary. For Claude Desktop, add this to its config
(`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "vamoose": {
      "command": "vamoose",
      "args": ["mcp"],
      "env": {
        "VAMOOSE_PROVIDER": "google",
        "VAMOOSE_TIMEZONE": "America/Chicago"
      }
    }
  }
}
```

Set the provider and any credentials the backend needs in `env`, the same variables the CLI reads. See [providers](providers.md) and [commands](commands.md). Restart the client to pick up the server.

### Tools

The fixed time-off tools:

| Tool                | Does                                                        &nbsp; |
| ------------------- | ----------------------------------------------------------------- |
| `whoami`            | Show the signed-in user, manager, and team.                       |
| `request_time_off`  | Create a hold shown free and invite the manager to approve.       |
| `time_off_status`   | Report whether the manager has approved a hold.                   |
| `promote_to_team`   | Add the team as optional attendees once approved.                 |
| `cancel_hold`       | Cancel a hold and notify its attendees.                           |
| `set_away`          | Mark yourself out of office over a range, no approval.            |
| `create_event`      | Create a quick event, optionally inviting attendees.              |

The workflow engine, so an agent can drive any workflow, not just time off:

| Tool                | Does                                                          &nbsp; |
| ------------------- | ------------------------------------------------------------------- |
| `list_workflows`    | List the workflows available to run, with descriptions.             |
| `preview_workflow`  | Show a workflow's plan for a date window, without changing anything. |
| `run_workflow`      | Run any workflow, optionally watching for approval.                 |
| `create_workflow`   | Author or replace a reusable workflow from a JSON definition.        |
| `list_schedules`    | List the recurring workflow schedules.                              |
| `schedule_workflow` | Schedule a workflow to rerun on an interval.                        |

An agent can discover, preview, run, schedule, and even author workflows, so vamoose is a calendar-workflow layer it drives and extends, not a fixed set of commands.

### What you can say

With the server connected, ask Claude in plain language, for example:

- "Request time off next week and send it to my manager for approval."
- "Has my manager approved my time off yet? If so, let my team know I'm out."
- "What workflows can I run?"
- "Preview the pto workflow for the week of the 20th before running it."
- "Make a workflow that holds the time, waits two days, then notifies the team, and run it for next week."
- "Schedule the notify-only workflow to run every week for the week ahead."

## Skill

`skill/SKILL.md` is a Claude skill that drives the time-off request flow end to end. It gathers the dates and subject, reads them back to confirm, then runs request, check, and promote for you. Use the skill for a guided, conversational flow, and the MCP server for direct tool calls when you know exactly what you want.
