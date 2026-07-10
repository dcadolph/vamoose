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
| `manager`| approve           | Wait on the directory manager (or `--manager`). The first approver.  |
| `approver`| approve          | Name a later approver by email in a multi-approver chain.            |
| `team`   | notify            | Role for the team. Must be `optional`.                               |
| `id`     | any               | Names the step so branches can target it.                            |
| `on`     | approve           | Branch by outcome: `{"accepted": id, "declined": id, "expired": id}`. |
| `timeout`| approve           | Duration to wait (e.g. `72h`) before the `expired` branch runs.      |
| `when`   | any but the first | Guard that skips the step unless its conditions hold. See Guards.    |
| `next`   | any               | The step id to run next, or `end`. Defaults to the following step.   |
| `subject`| note, event, message | Event title, or the text for a message step.                     |
| `channel`| message           | Where a message step posts, such as a Slack channel. Required.       |

## Verbs

- `hold` creates the event shown free, inviting the manager when an `approve` step follows.
- `approve` waits for an approver to accept the invite; chain several for multi-level approval.
- `notify` adds the team as optional attendees.
- `note` creates a short event on your own calendar to mark an outcome, such as a decline.
- `away` marks out of office with no attendees.
- `event` creates a plain event, with attendees from `--attendees`.
- `cancel` deletes the hold.
- `message` posts to a comms channel, such as a Slack channel, to announce the outcome.

## Rules

A workflow must:

- start with exactly one creating step (`hold`, `away`, or `event`)
- put any `approve` steps in a `hold`-led workflow, naming later chain approvers with `approver`
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

## Multiple approvers

Chain `approve` steps so a workflow needs more than one sign-off, in order. The first approve waits
on the manager (`manager: true` or `--manager`); each later approve names its approver by email with
`approver`, since the directory knows only the one manager. vamoose invites the next approver only
after the previous one accepts, so the director is not asked until the manager has signed off.

The built-in `pto-two-level` needs the manager, then a named director, before the team is told. Edit
the director email for your team:

```json
{
  "name": "pto-two-level",
  "steps": [
    { "id": "hold", "verb": "hold", "showAs": "free" },
    { "id": "manager", "verb": "approve", "manager": true },
    { "id": "director", "verb": "approve", "approver": "director@example.com" },
    { "id": "notify", "verb": "notify", "team": "optional", "next": "end" }
  ]
}
```

Run it with `--watch` and `vamoose daemon` so each acceptance advances the chain automatically. A
decline at any gate stops the workflow, or takes that gate's `declined` branch if it has one.

## Guards

A step can carry a `when` guard that gates whether it runs. When the conditions do not hold, the workflow skips the step and continues, so a guard trims a workflow rather than branching it. Guards layer on top of `on`: `on` chooses a path by the approval outcome, `when` gates any step on that path. The creating step always runs, so it cannot carry a guard.

Conditions are all optional and combine with and:

| Condition      | Meaning                                                       &nbsp; |
| -------------- | -------------------------------------------------------------------- |
| `dayOfWeek`    | Run only on the named days, checked at execution time.               |
| `minAttendees` | Run only when the hold has at least this many attendees.             |
| `maxAttendees` | Run only when the hold has at most this many attendees.              |

`dayOfWeek` is a comma-separated set of three-letter days or inclusive ranges, such as `mon-fri` or `sat,sun`. A range wraps past Saturday, so `fri-mon` is Friday through Monday. The attendee counts read the hold's invitees when the step runs, so an event that invites a crowd can trigger a follow-on step a small one skips.

The built-in `pto-notify-weekdays` approves time off but tells the team only on weekdays, so an approval that lands over the weekend does not page the team:

```json
{
  "name": "pto-notify-weekdays",
  "steps": [
    { "verb": "hold", "showAs": "free" },
    { "verb": "approve", "manager": true },
    { "verb": "notify", "team": "optional", "when": { "dayOfWeek": "mon-fri" } }
  ]
}
```

A skipped step does not run later; the guard drops it for this run.

## Messages

A `message` step posts to a comms channel, such as a Slack channel, so a workflow announces
its outcome where the team already is, not only on the calendar. Set a `channel` on the step.
The message text is the step's `subject`, or the hold's subject when the step sets none, so an
announcement carries the run's context without the workflow hardcoding it.

Messaging needs a comms backend. For Slack, create a bot token with the `chat:write` scope and
export it:

```sh
export VAMOOSE_SLACK_BOT_TOKEN=xoxb-...
```

The built-in `pto-announce` approves time off, announces it to a channel, then notifies the team:

```json
{
  "name": "pto-announce",
  "steps": [
    { "id": "hold", "verb": "hold", "showAs": "free" },
    { "id": "approval", "verb": "approve", "manager": true, "on": { "accepted": "announce" } },
    { "id": "announce", "verb": "message", "channel": "#out-of-office", "next": "notify" },
    { "id": "notify", "verb": "notify", "team": "optional", "next": "end" }
  ]
}
```
