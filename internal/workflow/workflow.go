// Package workflow defines vamoose workflows: named sequences of the vamoose verbs
// (hold, approve, notify, note, away, event, cancel) that the run command executes
// and the daemon advances. Steps run in order by default, but an approve step can
// branch on its outcome with `on: {accepted: <id>, declined: <id>}`, and any step
// can set `next`, making a workflow a small graph. Definitions are JSON, loaded
// from a built-in template embedded in the binary or a user file under the config
// directory.
package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// Approval outcomes an approve step can branch on.
const (
	// OutcomeAccepted is the branch key for the manager accepting.
	OutcomeAccepted = "accepted"
	// OutcomeDeclined is the branch key for the manager declining.
	OutcomeDeclined = "declined"
	// OutcomeExpired is the branch key for the approver not responding before the
	// step's timeout elapses.
	OutcomeExpired = "expired"
)

// Verb is a single action a workflow step performs. Each maps to a vamoose command.
type Verb string

const (
	// VerbHold creates an event shown free, inviting the approver when an approve
	// step follows.
	VerbHold Verb = "hold"
	// VerbApprove waits for the required approver to accept the hold.
	VerbApprove Verb = "approve"
	// VerbNotify adds the team to the hold as optional attendees.
	VerbNotify Verb = "notify"
	// VerbNote creates a short informational event on the requester's own calendar,
	// used to mark an outcome such as a decline.
	VerbNote Verb = "note"
	// VerbAway marks the user out of office with no attendees.
	VerbAway Verb = "away"
	// VerbEvent creates a quick event, optionally inviting attendees.
	VerbEvent Verb = "event"
	// VerbCancel deletes the hold and notifies its attendees.
	VerbCancel Verb = "cancel"
	// VerbMessage posts a message to a comms channel, such as a Slack channel, to
	// announce the workflow's outcome.
	VerbMessage Verb = "message"
)

// Creates reports whether the verb creates the hold a workflow acts on. Exactly one
// creating step leads every workflow. Note creates a side marker, not the hold.
func (v Verb) Creates() bool {
	switch v {
	case VerbHold, VerbAway, VerbEvent:
		return true
	default:
		return false
	}
}

// Waits reports whether the verb blocks on an external condition the daemon polls
// rather than completing as soon as it runs.
func (v Verb) Waits() bool {
	return v == VerbApprove
}

// known reports whether the verb is recognized.
func (v Verb) known() bool {
	switch v {
	case VerbHold, VerbApprove, VerbNotify, VerbNote, VerbAway, VerbEvent, VerbCancel, VerbMessage:
		return true
	default:
		return false
	}
}

// Step is one action in a workflow.
type Step struct {
	// Verb is the action this step performs.
	Verb Verb `json:"verb"`
	// ID names the step so branches can target it. Optional for linear workflows.
	ID string `json:"id,omitempty"`
	// On, on an approve step, maps an outcome (accepted, declined) to the next step
	// id to run, or "end". Absent outcomes fall through: accepted to the next step,
	// declined stops.
	On map[string]string `json:"on,omitempty"`
	// Next overrides the step to run after this one: a step id or "end".
	Next string `json:"next,omitempty"`
	// Timeout, on an approve step, is how long to wait for the approver before the
	// expired branch runs, as a Go duration such as "72h". It requires an expired
	// branch in On.
	Timeout string `json:"timeout,omitempty"`
	// When guards the step: it runs only when the guard's conditions hold at
	// execution time, otherwise the flow skips it. The zero value never gates.
	When When `json:"when,omitempty"`
	// ShowAs overrides the free/busy status for creating steps. Empty uses the verb
	// default: free for hold and event, out of office for away.
	ShowAs calendar.ShowAs `json:"showAs,omitempty"`
	// Manager, on an approve step, waits on the manager resolved from the directory
	// or the --manager flag. Only the first approve in a chain may use it.
	Manager bool `json:"manager,omitempty"`
	// Approver, on an approve step, names an explicit approver by email, used for the
	// later links of a multi-approver chain where the directory knows only the manager.
	Approver string `json:"approver,omitempty"`
	// Team sets the role used when a notify step adds the team. Empty means optional.
	Team calendar.Role `json:"team,omitempty"`
	// Subject overrides the event title for a note or event step, or the message text
	// for a message step. Empty defaults per verb.
	Subject string `json:"subject,omitempty"`
	// Channel is the destination for a message step, such as a Slack channel id or
	// name. Required on a message step, unused otherwise.
	Channel string `json:"channel,omitempty"`
}

// ParsedTimeout returns the approve step's timeout as a duration, or zero when it is
// unset. Validate rejects an unparseable timeout, so a nonzero return here means the
// step waits that long before its expired branch runs.
func (s Step) ParsedTimeout() time.Duration {
	if s.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 0
	}
	return d
}

// When gates a step. The step runs only when every condition it sets holds; an unset
// condition does not constrain, so the zero value always allows the step. Unlike On,
// which branches an approve step by its outcome, When can gate any step.
type When struct {
	// DayOfWeek limits the step to the named days, as a comma-separated set of
	// three-letter days or inclusive ranges, such as "mon-fri" or "sat,sun". Empty
	// allows any day. It is evaluated against the execution-time clock.
	DayOfWeek string `json:"dayOfWeek,omitempty"`
	// MinAttendees requires the hold to have at least this many attendees. Zero does
	// not constrain.
	MinAttendees int `json:"minAttendees,omitempty"`
	// MaxAttendees requires the hold to have at most this many attendees. Zero does
	// not constrain.
	MaxAttendees int `json:"maxAttendees,omitempty"`
}

// Allows reports whether the guard permits the step to run at time at with the given
// attendee count. An unset condition does not constrain, so the zero value always
// allows. A malformed day set denies; Validate rejects such a guard up front.
func (w When) Allows(at time.Time, attendees int) bool {
	if w.DayOfWeek != "" {
		days, err := parseDays(w.DayOfWeek)
		if err != nil || !days[at.Weekday()] {
			return false
		}
	}
	if w.MinAttendees > 0 && attendees < w.MinAttendees {
		return false
	}
	if w.MaxAttendees > 0 && attendees > w.MaxAttendees {
		return false
	}
	return true
}

// validate checks the guard is well formed: a parseable day set and non-negative,
// consistent attendee bounds.
func (w When) validate() error {
	if w.DayOfWeek != "" {
		if _, err := parseDays(w.DayOfWeek); err != nil {
			return fmt.Errorf("invalid dayOfWeek %q: %w", w.DayOfWeek, err)
		}
	}
	if w.MinAttendees < 0 {
		return fmt.Errorf("minAttendees cannot be negative")
	}
	if w.MaxAttendees < 0 {
		return fmt.Errorf("maxAttendees cannot be negative")
	}
	if w.MinAttendees > 0 && w.MaxAttendees > 0 && w.MinAttendees > w.MaxAttendees {
		return fmt.Errorf("minAttendees %d exceeds maxAttendees %d", w.MinAttendees, w.MaxAttendees)
	}
	return nil
}

// parseDay maps a three-letter day abbreviation to its weekday, case-insensitive.
func parseDay(name string) (time.Weekday, bool) {
	switch strings.ToLower(name) {
	case "sun":
		return time.Sunday, true
	case "mon":
		return time.Monday, true
	case "tue":
		return time.Tuesday, true
	case "wed":
		return time.Wednesday, true
	case "thu":
		return time.Thursday, true
	case "fri":
		return time.Friday, true
	case "sat":
		return time.Saturday, true
	default:
		return 0, false
	}
}

// parseDays parses a day-of-week set such as "mon-fri" or "sat,sun" into the weekdays
// it names. Tokens are comma-separated; each is a single day or an inclusive range
// like "mon-fri" that wraps past Saturday. It errors on an unknown day or an empty set.
func parseDays(spec string) (map[time.Weekday]bool, error) {
	set := make(map[time.Weekday]bool)
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(tok, "-")
		start, ok := parseDay(lo)
		if !ok {
			return nil, fmt.Errorf("unknown day %q", lo)
		}
		if !isRange {
			set[start] = true
			continue
		}
		end, ok := parseDay(hi)
		if !ok {
			return nil, fmt.Errorf("unknown day %q", hi)
		}
		for d := start; ; d = (d + 1) % 7 {
			set[d] = true
			if d == end {
				break
			}
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no days named")
	}
	return set, nil
}

// Workflow is a named sequence of steps forming a small graph.
type Workflow struct {
	// Name identifies the workflow and matches its file name without the extension.
	Name string `json:"name"`
	// Description is a one-line summary shown in listings.
	Description string `json:"description,omitempty"`
	// Steps run in order unless a branch or next redirects the flow.
	Steps []Step `json:"steps"`
}

// Validate checks that the workflow is well formed: it has a name and steps, known
// verbs, valid enums, unique step ids, exactly one creating step (hold/away/event)
// first, at most one approve (which needs a hold), no notify before approval, and
// every branch or next target resolving to a real step or "end".
func (w Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("%w: missing name", ErrInvalid)
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("%w: %q has no steps", ErrInvalid, w.Name)
	}

	ids := make(map[string]bool)
	creators, approves, firstApprove, firstNotify := 0, 0, -1, -1
	for i, s := range w.Steps {
		if !s.Verb.known() {
			return fmt.Errorf("%w: step %d: %w %q", ErrInvalid, i, ErrUnknownVerb, s.Verb)
		}
		if s.ShowAs != "" && !s.ShowAs.Valid() {
			return fmt.Errorf("%w: step %d: invalid showAs %q", ErrInvalid, i, s.ShowAs)
		}
		if s.Team != "" && !s.Team.Valid() {
			return fmt.Errorf("%w: step %d: invalid team role %q", ErrInvalid, i, s.Team)
		}
		if s.Verb == VerbNotify && s.Team == calendar.RoleRequired {
			return fmt.Errorf("%w: step %d: notify team must be optional so it never blocks teammates", ErrInvalid, i)
		}
		if s.Verb == VerbMessage && s.Channel == "" {
			return fmt.Errorf("%w: step %d: a message step needs a channel", ErrInvalid, i)
		}
		if s.Channel != "" && s.Verb != VerbMessage {
			return fmt.Errorf("%w: step %d: only a message step may set a channel", ErrInvalid, i)
		}
		if s.Approver != "" && s.Verb != VerbApprove {
			return fmt.Errorf("%w: step %d: only an approve step may set an approver", ErrInvalid, i)
		}
		if s.ID != "" {
			if ids[s.ID] {
				return fmt.Errorf("%w: duplicate step id %q", ErrInvalid, s.ID)
			}
			ids[s.ID] = true
		}
		if len(s.On) > 0 && s.Verb != VerbApprove {
			return fmt.Errorf("%w: step %d: only an approve step may branch with on", ErrInvalid, i)
		}
		for outcome := range s.On {
			if outcome != OutcomeAccepted && outcome != OutcomeDeclined && outcome != OutcomeExpired {
				return fmt.Errorf("%w: step %d: invalid branch outcome %q", ErrInvalid, i, outcome)
			}
		}
		if s.Timeout != "" {
			if s.Verb != VerbApprove {
				return fmt.Errorf("%w: step %d: only an approve step may set a timeout", ErrInvalid, i)
			}
			if _, err := time.ParseDuration(s.Timeout); err != nil {
				return fmt.Errorf("%w: step %d: invalid timeout %q", ErrInvalid, i, s.Timeout)
			}
			if _, ok := s.On[OutcomeExpired]; !ok {
				return fmt.Errorf("%w: step %d: a timeout needs an expired branch", ErrInvalid, i)
			}
		}
		if _, ok := s.On[OutcomeExpired]; ok && s.Timeout == "" {
			return fmt.Errorf("%w: step %d: an expired branch needs a timeout", ErrInvalid, i)
		}
		if err := s.When.validate(); err != nil {
			return fmt.Errorf("%w: step %d: %w", ErrInvalid, i, err)
		}
		if s.Verb.Creates() && s.When != (When{}) {
			return fmt.Errorf("%w: step %d: the creating step cannot have a when guard", ErrInvalid, i)
		}
		if s.Verb.Creates() {
			creators++
			if i != 0 {
				return fmt.Errorf("%w: step %d: %q must be the first step", ErrInvalid, i, s.Verb)
			}
		}
		if s.Verb == VerbApprove {
			approves++
			if approves > 1 {
				if s.Approver == "" {
					return fmt.Errorf("%w: step %d: a later approve must name an approver by email; only the first uses the manager", ErrInvalid, i)
				}
				if s.Manager {
					return fmt.Errorf("%w: step %d: only the first approve may use the directory manager", ErrInvalid, i)
				}
			}
			if firstApprove < 0 {
				firstApprove = i
			}
		}
		if s.Verb == VerbNotify && firstNotify < 0 {
			firstNotify = i
		}
	}
	if creators != 1 {
		return fmt.Errorf("%w: %q needs exactly one creating step (hold/away/event) but has %d",
			ErrInvalid, w.Name, creators)
	}
	if firstApprove >= 0 && w.Steps[0].Verb != VerbHold {
		return fmt.Errorf("%w: approval requires a hold: %q starts with %s", ErrInvalid, w.Name, w.Steps[0].Verb)
	}
	if firstApprove >= 0 && firstNotify >= 0 && firstNotify < firstApprove {
		return fmt.Errorf("%w: %q notifies the team before approval", ErrInvalid, w.Name)
	}
	for i, s := range w.Steps {
		for outcome, target := range s.On {
			if target != "end" && w.StepIndex(target) < 0 {
				return fmt.Errorf("%w: step %d: branch %q targets unknown step %q", ErrInvalid, i, outcome, target)
			}
		}
		if s.Next != "" && s.Next != "end" && w.StepIndex(s.Next) < 0 {
			return fmt.Errorf("%w: step %d: next targets unknown step %q", ErrInvalid, i, s.Next)
		}
	}
	return nil
}

// StepIndex returns the index of the step with the given id, or -1 when none match.
func (w Workflow) StepIndex(id string) int {
	if id == "" {
		return -1
	}
	for i, s := range w.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// Next returns the index of the step to run after step i. An outcome (accepted or
// declined, empty for non-approval steps) follows a matching branch first; otherwise
// an explicit next applies, otherwise the flow falls through to the following step.
// It returns -1 to stop.
func (w Workflow) Next(i int, outcome string) int {
	if i < 0 || i >= len(w.Steps) {
		return -1
	}
	s := w.Steps[i]
	if outcome != "" {
		if target, ok := s.On[outcome]; ok {
			return w.resolve(target)
		}
	}
	if s.Next != "" {
		return w.resolve(s.Next)
	}
	if i+1 < len(w.Steps) {
		return i + 1
	}
	return -1
}

// resolve maps a target id to its index, treating "end" and unknowns as stop.
func (w Workflow) resolve(target string) int {
	if target == "end" {
		return -1
	}
	return w.StepIndex(target)
}

// Has reports whether the workflow contains a step with the given verb.
func (w Workflow) Has(v Verb) bool {
	for _, s := range w.Steps {
		if s.Verb == v {
			return true
		}
	}
	return false
}
