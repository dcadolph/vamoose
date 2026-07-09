<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# vamoose with Claude

Two ways to drive vamoose from Claude.

## MCP server

`vamoose mcp` speaks the Model Context Protocol over stdio, exposing the commands as tools. Point an MCP client at the binary:

```json
{ "mcpServers": { "vamoose": { "command": "vamoose", "args": ["mcp"] } } }
```

Authenticate once first with `vamoose whoami` so the server reuses the cached token.

Tools exposed: `whoami`, `request_time_off`, `time_off_status`, `promote_to_team`, `cancel_hold`, `set_away`, `create_event`. Each shells out to the vamoose binary, so behavior matches the CLI exactly.

## Skill

`skill/SKILL.md` is a Claude skill that drives the time-off request flow end to end. It gathers the dates and subject, reads them back to confirm, then runs request, check, and promote. Use the skill for a guided flow and the MCP server for direct tool calls.
