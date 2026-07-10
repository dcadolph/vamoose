<div class="vamoose-hero" markdown>

![vamoose](assets/vamoose-mark.png){ .hero-logo }

# vamoose

<p class="tagline">Calendar workflows, minus the tedium.</p>

<p class="subtitle">The moose does the paperwork. You go to the beach. Four calendar backends behind one workflow engine, driven from your terminal, Claude, or Slack.</p>

[Get started](quickstart.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/dcadolph/vamoose){ .md-button }

</div>

<div class="vamoose-terminal">
<div class="vamoose-terminal-bar"><span class="dot red"></span><span class="dot yellow"></span><span class="dot green"></span><span class="title">vamoose</span></div>
<div class="vamoose-terminal-body"><span class="prompt">$</span> <span class="cmd">vamoose off next week --subject "Out: beach week"</span>
<span class="out">Hold created and sent to boss@work.com for approval. Hold id: AAMk…</span>
<span class="prompt">$</span> <span class="cmd">vamoose check</span>
<span class="out">Approved by boss@work.com.</span>
<span class="prompt">$</span> <span class="cmd">vamoose promote</span>
<span class="out">Added 4 team members as optional from the directory. Everyone notified.</span></div>
</div>

<div class="vamoose-install" markdown>

```sh
brew install dcadolph/tap/vamoose
```

</div>

## How it works

<div class="vamoose-steps">
<div class="step"><span class="num">1</span><strong>Request</strong>Block the dates shown free, so no calendar is touched, and invite your manager.</div>
<div class="step"><span class="num">2</span><strong>Approve</strong>Your manager accepts the invite. That acceptance is the approval, no extra tool.</div>
<div class="step"><span class="num">3</span><strong>Notify</strong>The team is added as optional attendees, so everyone sees you are out.</div>
</div>

## Why vamoose

<div class="grid cards" markdown>

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

    Wait steps pause a run; schedules rerun a whole workflow weekly. It moves on its own, in the background.

-   :material-robot-happy:{ .lg .middle } __Drivable by AI__

    ---

    Over MCP, an agent discovers, previews, runs, schedules, and even authors workflows. The calendar-workflow layer for Claude.

-   :material-bullhorn:{ .lg .middle } __Tells everyone__

    ---

    Fan out to the team as optional attendees, and announce the outcome to a Slack channel or by email.

</div>

## Start here

- [Quickstart](quickstart.md) — zero to your first approved hold in a few minutes.
- [Concepts](concepts.md) — how holds, approval, and workflows fit together.
- [Commands](commands.md) — every command, flag, and environment variable.
- [Workflows](workflows.md) — design your own, with branching, delays, and guards.
