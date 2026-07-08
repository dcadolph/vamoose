---
name: vamoose
description: >-
  Book time off without the busywork. Prompts for your dates and subject, creates
  a calendar hold shown as free, invites your manager to approve it, then fans the
  event out to your team as optional attendees once approved. Backs onto Microsoft
  365, Outlook, and Teams via Microsoft Graph, or Google Calendar. Trigger when the user wants to
  request vacation, set an out-of-office hold, or route time off for manager
  approval.
---

# vamoose

Drive the vamoose CLI to run a vacation request end to end.

## Before you start

Confirm the environment is configured. If `VAMOOSE_CLIENT_ID` is unset, stop and
point the user at the README setup section; the flow cannot run without it.

```sh
vamoose version   # confirm the binary is on PATH
```

If the binary is missing, build it from the repo root with `go build -o vamoose .`
and use `./vamoose`.

## Gather the request

Ask the user for, and confirm back:

- Departure date (or date-time). Accept `YYYY-MM-DD` or RFC3339.
- Return date (or date-time).
- Subject line, for example "Out: beach week".
- Optional: a note for the body, and a manager email if they do not want the
  directory lookup.

Do not guess dates. If the user is vague ("next week"), resolve to explicit
calendar dates and read them back before running anything.

## Run the flow

1. Create the hold and invite the manager:

   ```sh
   vamoose request --start <start> --end <end> --subject "<subject>"
   ```

   Add `--body`, `--manager`, or `--tz` when the user supplied them. Offer
   `--dry-run` first if the user wants to preview.

2. Tell the user the manager was invited and must accept the invite to approve.
   The command prints the hold id and caches it, so later steps need no id.

3. When the user says the manager approved, or to check, run:

   ```sh
   vamoose check
   ```

4. Once `check` reports approval, fan out to the team:

   ```sh
   vamoose promote
   ```

   Or run `vamoose check --promote` to promote automatically the moment approval
   lands.

## Notes

- The hold is shown as free, so it never blocks anyone's calendar.
- The manager is a required attendee; their acceptance is the approval.
- Team members are added as optional attendees.
- Two backends: `--provider graph` (default) or `--provider google`. Google has no
  directory, so on Google pass `--manager` and set the team with `vamoose team set`.
- Report command output plainly. On an error, surface the exact message rather
  than retrying blindly.
