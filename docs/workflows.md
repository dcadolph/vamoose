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
| `id`     | any               | Names the step so branches can target it.                            |
| `on`     | approve           | Branch by outcome: `{"accepted": id, "declined": id, "expired": id}`. |
| `timeout`| approve           | Duration to wait (e.g. `72h`) before the `expired` branch runs.      |
| `next`   | any               | The step id to run next, or `end`. Defaults to the following step.   |
| `subject`| note, event       | Event title for a note or event step.                               |

## Verbs

- `hold` creates the event shown free, inviting the manager when an `approve` step follows.
- `approve` waits for the manager to accept the invite.
- `notify` adds the team as optional attendees.
- `note` creates a short event on your own calendar to mark an outcome, such as a decline.
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

## Branching

An approve step can branch on its outcome with `on`, so a workflow does one thing when the manager accepts and another when they decline. Give steps an `id` and route with `on` and `next`. Absent branches fall through: accepted runs the next step, declined stops. Steps with no `id`, `on`, or `next` behave exactly as before, so linear workflows are unchanged.

The built-in `pto-notify-on-decline` shows it. On accept it notifies the team, on decline it notes the outcome on your calendar:

```json
{
  "name": "pto-notify-on-decline",
  "steps": [
    { "id": "hold", "verb": "hold", "showAs": "free" },
    { "id": "approval", "verb": "approve", "manager": true,
      "on": { "accepted": "notify", "declined": "denied" } },
    { "id": "notify", "verb": "notify", "team": "optional", "next": "end" },
    { "id": "denied", "verb": "note", "subject": "Time off declined", "next": "end" }
  ]
}
```

## Timeouts

An `approve` step can set a `timeout` and an `expired` branch, so a workflow acts on its own when the manager never responds. The daemon runs the expired branch once the timeout passes with no accept or decline. The built-in `pto-cancel-on-timeout` cancels the hold after 72 hours of silence:

```json
{
  "name": "pto-cancel-on-timeout",
  "steps": [
    { "id": "hold", "verb": "hold", "showAs": "free" },
    { "id": "approval", "verb": "approve", "manager": true, "timeout": "72h",
      "on": { "accepted": "notify", "expired": "expired" } },
    { "id": "notify", "verb": "notify", "team": "optional", "next": "end" },
    { "id": "expired", "verb": "cancel", "next": "end" }
  ]
}
```

## Coming next

More branch conditions (day of week, attendee counts) come in a later version. Today branches turn on the approval outcome and, for `approve`, a timeout.
