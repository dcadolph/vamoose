<p align="center"><img src="../assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Workflows

A workflow is an ordered list of steps vamoose runs and the daemon advances. Run one with `vamoose run <name>` and list them with `vamoose workflows`.

## Built-in workflows

| Name          | Steps                            | Use                            &nbsp; |
| ------------- | -------------------------------- | ------------------------------------- |
| `pto`         | hold shown free, approve, notify | Time off a manager approves.          |
| `notify-only` | hold shown free, notify          | Tell the team, no approval.           |
| `away`        | out-of-office block              | Personal out of office, no fanout.    |

`request`, `off`, `check`, and `promote` are short fronts over the `pto` workflow.

## Custom workflows

Drop a JSON file in `~/.config/vamoose/workflows/<name>.json`. A file there overrides a built-in of the same name. Then run `vamoose run <name>`.

```json
{
  "name": "team-heads-up",
  "description": "Hold shown free, tell the team, no approval.",
  "steps": [
    { "verb": "hold", "showAs": "free" },
    { "verb": "notify", "team": "optional" }
  ]
}
```

## Step fields

| Field    | Applies to        | Meaning                                                        &nbsp; |
| -------- | ----------------- | -------------------------------------------------------------------- |
| `verb`   | all               | The action (see below). Required.                                    |
| `showAs` | hold, away, event | Free/busy: `free`, `busy`, `tentative`, `oof`. Defaults per verb.    |
| `manager`| approve           | Wait on the manager, from the directory or `--manager`.              |
| `team`   | notify            | Role for the team. Must be `optional`.                               |

## Verbs

- `hold` creates the event shown free, inviting the manager when an `approve` step follows.
- `approve` waits for the manager to accept the invite.
- `notify` adds the team as optional attendees.
- `away` marks out of office with no attendees.
- `event` creates a plain event, with attendees from `--attendees`.
- `cancel` deletes the hold.

## Rules

A workflow must:

- start with exactly one creating step (`hold`, `away`, or `event`)
- have at most one `approve` step, and only in a `hold`-led workflow
- run `notify` after `approve`, never before
- keep the `notify` team `optional`, so it never blocks a teammate's calendar

Run `vamoose run <name> --dry-run` to print the plan without touching the calendar.

## Watching for approval

`vamoose run <name> --watch` records the hold at its approval gate. `vamoose daemon` then advances the workflow, notifying the team for `pto`, once the manager accepts.

## Coming next

Conditional and branching workflows (if-this-then-that, `a` then `b` then `c`, else `d`) are the next step. Today's engine is linear: ordered steps and a single approval gate.
