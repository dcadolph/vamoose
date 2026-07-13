---
title: Automate time off and calendar workflows
description: >-
  vamoose books time off for you. It holds the dates, gets your manager's approval from
  the calendar accept, invites the team, announces in Slack, and files the leave in HR.
  Works with Outlook, Google Calendar, iCloud, and CalDAV.
hide:
  - navigation
  - toc
---

<div class="hero-split" markdown>
<div class="hero-copy" markdown>

![vamoose](assets/vamoose-mark.png){ .hero-mark }

# vamoose

<p class="tagline">Calendar workflows, minus the tedium.</p>

<p class="subtitle">The moose does the paperwork. You go to the beach. The office rituals that sprawl across calendar invites, email threads, approval chases, and HR portals become one command that runs itself, on Microsoft Graph, Google, iCloud, or any CalDAV host.</p>

[Get started](quickstart.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/dcadolph/vamoose){ .md-button }

```sh
brew install dcadolph/tap/vamoose
```

</div>
<div class="hero-visual">
<div class="term">
<div class="term-bar"><span class="dot red"></span><span class="dot yellow"></span><span class="dot green"></span><span class="term-title">vamoose</span></div>
<div class="term-body"><span class="p">$</span> <span class="c">vamoose off next week --subject "Out: beach week"</span>
<span class="o">Hold created, sent to boss@work.com for approval.</span>
<span class="p">$</span> <span class="c">vamoose check</span>
<span class="o">Approved by boss@work.com ✓</span>
<span class="p">$</span> <span class="c">vamoose promote</span>
<span class="o">Added 4 teammates as optional. Everyone notified ✓</span></div>
</div>
</div>
</div>

## You know this dance { .section-head }

<div class="oldway">
<div class="oldway-panel oldway-manual">
<p class="oldway-label">By hand</p>
<p class="oldway-title">Booking a week off</p>
<ol class="oldway-steps">
<li>Open the work calendar, Google or Exchange. Create the event, mark it Free, invite exactly one person: your manager, as optional.</li>
<li>Slack your manager so the invite doesn't land cold: "thinking about the 20th through the 24th, that OK?"</li>
<li>Approved. Back into the event: add every teammate covering for you, one at a time.</li>
<li>Triple-check it still shows Free, or six calendars go busy for a week.</li>
<li>Your own calendar still shows you open, so create a second event, just you, marked Out of office.</li>
<li>Log in to the HR portal and file the leave.</li>
<li>Next quarter, do all of it again from memory.</li>
</ol>
</div>
<div class="oldway-arrow" aria-hidden="true">&rarr;</div>
<div class="oldway-panel oldway-new">
<p class="oldway-label oldway-label-new">With vamoose</p>
<p class="oldway-cmd"><span class="p">$</span> vamoose off next week</p>
<p class="oldway-body">One workflow runs the whole thing. The hold goes out marked Free with your manager on it, their calendar accept is the approval, the team lands as optional attendees the moment it arrives, Slack and email get told, the leave gets filed in HR. The blocked-out event for your own calendar is one more line: <code>vamoose away</code>.</p>
<p class="oldway-body">Same command whether this job runs on Microsoft, Google, or iCloud. The workflow never notices which service is behind it, so neither do you.</p>
</div>
</div>

## How it works { .section-head }

<div class="flow">
<div class="flow-step"><span class="num">1</span><div class="flow-text"><strong>Declare</strong><span>A workflow is ordered steps: create a hold, branch on the outcome, wait, and gate on approval. Time off is built in, or author your own in JSON or with an AI agent.</span></div></div>
<div class="flow-step"><span class="num">2</span><div class="flow-text"><strong>Run</strong><span>Drive it from your terminal, Claude, Slack, a local dashboard, or the menu bar moose, on Microsoft Graph, Google, iCloud, or any CalDAV host. Swappable doors, swappable backends.</span></div></div>
<div class="flow-step"><span class="num">3</span><div class="flow-text"><strong>Advance</strong><span>The daemon moves it along on its own: approvals, timeouts, waits, recurring schedules, and the notify, note, and message steps.</span></div></div>
</div>

## Why vamoose { .section-head }

<div class="grid cards vamoose-features" markdown>

-   :material-calendar-sync:{ .lg .middle } __Any calendar__

    ---

    Microsoft Graph, Google, Apple iCloud, and any CalDAV host, behind one interface. Switch providers, change nothing.

-   :material-sitemap:{ .lg .middle } __A real workflow engine__

    ---

    Branch on outcomes, gate on approval, guard by day or headcount. It books half days, counts working days around holidays, and warns when too many teammates are already off. A state machine in JSON, not a dumb calendar rule.

-   :material-account-check:{ .lg .middle } __Approvals that mean it__

    ---

    Manager, then director, in sequence. Timeouts that act on silence, decline paths, and the accept-the-invite signal, no approval product to buy.

-   :material-timer-sand:{ .lg .middle } __Acts on time__

    ---

    Wait steps pause a run. Schedules rerun a whole workflow weekly. It moves on its own in the background, and resumes where it left off after a crash.

-   :material-robot-happy:{ .lg .middle } __Drivable by AI__

    ---

    Over MCP, an agent discovers, previews, runs, schedules, and even authors workflows. The calendar-workflow layer for Claude.

-   :material-bullhorn:{ .lg .middle } __Finishes the job__

    ---

    Fan out to the team, announce to Slack, email, or any webhook, file the leave in your HR system, show what you have left, and keep a run history of who approved what.

</div>

## Start here { .section-head }

| Guide                       | What's in it                                         |
| --------------------------- | ---------------------------------------------------- |
| [Quickstart](quickstart.md) | Zero to your first approved hold in a few minutes.   |
| [Concepts](concepts.md)     | How holds, approval, and workflows fit together.     |
| [Commands](commands.md)     | Every command, flag, and environment variable.       |
| [Workflows](workflows.md)   | Design your own, with branching, delays, and guards. |
