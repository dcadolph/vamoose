<p align="center"><img src="assets/vamoose-moosercycle.png" alt="vamoose" width="100%"></p>

# Concepts

The ideas behind vamoose, and how they fit together.

## Hold

A **hold** is a calendar event vamoose creates and manages, such as a time-off block. A hold is shown **free** by default, so it blocks nobody's scheduling while it waits for approval. Every workflow acts on one hold.

## Workflow, step, verb

A **workflow** is an ordered list of **steps** that vamoose runs and the daemon advances. Each step has a **verb**, the action it performs: `hold`, `approve`, `notify`, `note`, `away`, `event`, `cancel`, `message`, or `wait`. Steps run in order, but an `approve` step can branch on its outcome, any step can redirect with `next`, and a `when` guard can skip a step. Workflows are JSON, either built in or your own. See [workflows](workflows.md).

The built-in `pto` workflow is three steps: create the hold, wait for the manager to approve, then notify the team.

## Approval

vamoose does not add an approval product. The **approval signal is the manager accepting the calendar invite.** The hold invites the manager as a required attendee; when they accept, that is approval, and when they decline, that is rejection. A workflow can require more than one approver in sequence, such as a manager then a director.

## Promote

To **promote** a hold is to add your team as **optional** attendees and resend, so everyone sees you are out without their calendars getting blocked. This is what the `notify` verb does, and the `promote` command runs it directly.

## Watch and the daemon

Running a workflow with `--watch` records the hold at its gate in a watch list. The **daemon** (`vamoose daemon`) polls the watch list and advances each workflow when its condition is met: the manager responds, a timeout passes, or a `wait` delay elapses. This is what makes approval and delays fire on their own, in the background.

## The three adapters

vamoose keeps all logic in a core and talks to the outside through three kinds of adapter, kept separate:

- **Calendar** creates and reads holds: Microsoft Graph, Google, iCloud, or CalDAV.
- **Directory** resolves your manager and team: Graph has one, and the others do not, so you pass `--manager` and set the team by hand.
- **Comms** sends messages for a `message` step: Slack or email.

The same commands and workflows work across every calendar backend. Pick one with `--provider` or `VAMOOSE_PROVIDER`. See [architecture](architecture.md) for how the layers fit.

## Backend differences

One behavior is not uniform: **approval detection**. Microsoft Graph, Google, and standard CalDAV hosts report a manager's accept or decline over their API, so `check` and the daemon detect approval directly. **Apple iCloud does not report it over CalDAV.** On iCloud, approval is recovered two ways: the macOS EventKit helper reads it from your local Calendar.app, or a Slack Approve button acts on it regardless of backend. Without either, you promote by hand once you know the manager accepted. See [providers](providers.md).

## Surfaces

The same core is driven from several **surfaces**: the CLI, a local MCP server for Claude, a Slack app, and the background daemon. They are thin clients; the workflow logic is the same underneath. See [Slack](slack.md) and [Claude](claude-guide.md).
