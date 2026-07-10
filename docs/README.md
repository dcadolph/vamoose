---
hide:
  - navigation
  - toc
---

<div class="hero-split" markdown>
<div class="hero-copy" markdown>

![vamoose](assets/vamoose-mark.png){ .hero-mark }

# vamoose

<p class="tagline">Calendar workflows, minus the tedium.</p>

<p class="subtitle">The moose does the paperwork. You go to the beach. Four calendar backends behind one workflow engine, driven from your terminal, Claude, or Slack.</p>

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

## How it works { .section-head }

<div class="flow">
<div class="flow-step"><span class="num">1</span><div class="flow-text"><strong>Request</strong><span>Block the dates shown free, so no calendar is touched, and invite your manager.</span></div></div>
<div class="flow-step"><span class="num">2</span><div class="flow-text"><strong>Approve</strong><span>Your manager accepts the invite. That acceptance is the approval, no extra tool.</span></div></div>
<div class="flow-step"><span class="num">3</span><div class="flow-text"><strong>Notify</strong><span>The team is added as optional attendees, so everyone sees you are out.</span></div></div>
</div>

## Why vamoose { .section-head }

<div class="grid cards vamoose-features" markdown>

-   :material-calendar-sync:{ .lg .middle } __Any calendar__

    ---

    Microsoft Graph, Google, Apple iCloud, and any CalDAV host, behind one interface. Switch providers, change nothing.

-   :material-sitemap:{ .lg .middle } __A real workflow engine__

    ---

    Branch on outcomes, gate on approval, guard by day or headcount. A state machine in JSON, not a dumb calendar rule.

-   :material-account-check:{ .lg .middle } __Approvals that mean it__

    ---

    Manager, then director, in sequence. Timeouts that act on silence, decline paths, and the accept-the-invite signal, no approval product to buy.

-   :material-timer-sand:{ .lg .middle } __Acts on time__

    ---

    Wait steps pause a run. Schedules rerun a whole workflow weekly. It moves on its own, in the background.

-   :material-robot-happy:{ .lg .middle } __Drivable by AI__

    ---

    Over MCP, an agent discovers, previews, runs, schedules, and even authors workflows. The calendar-workflow layer for Claude.

-   :material-bullhorn:{ .lg .middle } __Tells everyone__

    ---

    Fan out to the team as optional attendees, and announce the outcome to a Slack channel or by email.

</div>

## Start here { .section-head }

| Guide                        | What's in it                                         &nbsp; |
| ---------------------------- | ----------------------------------------------------------- |
| [Quickstart](quickstart.md)  | Zero to your first approved hold in a few minutes.          |
| [Concepts](concepts.md)      | How holds, approval, and workflows fit together.            |
| [Commands](commands.md)      | Every command, flag, and environment variable.              |
| [Workflows](workflows.md)    | Design your own, with branching, delays, and guards.        |
