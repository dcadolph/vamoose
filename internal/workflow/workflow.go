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

	"github.com/dcadolph/vamoose/internal/calendar"
)

// Approval outcomes an approve step can branch on.
const (
	// OutcomeAccepted is the branch key for the manager accepting.
	OutcomeAccepted = "accepted"
	// OutcomeDeclined is the branch key for the manager declining.
	OutcomeDeclined = "declined"
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
	case VerbHold, VerbApprove, VerbNotify, VerbNote, VerbAway, VerbEvent, VerbCancel:
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
	// ShowAs overrides the free/busy status for creating steps. Empty uses the verb
	// default: free for hold and event, out of office for away.
	ShowAs calendar.ShowAs `json:"showAs,omitempty"`
	// Manager, on an approve step, waits on the manager resolved from the directory
	// or the --manager flag. Reserved for future per-approver selection.
	Manager bool `json:"manager,omitempty"`
	// Team sets the role used when a notify step adds the team. Empty means optional.
	Team calendar.Role `json:"team,omitempty"`
	// Subject overrides the event title for a note or event step.
	Subject string `json:"subject,omitempty"`
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
			if outcome != OutcomeAccepted && outcome != OutcomeDeclined {
				return fmt.Errorf("%w: step %d: invalid branch outcome %q", ErrInvalid, i, outcome)
			}
		}
		if s.Verb.Creates() {
			creators++
			if i != 0 {
				return fmt.Errorf("%w: step %d: %q must be the first step", ErrInvalid, i, s.Verb)
			}
		}
		if s.Verb == VerbApprove {
			approves++
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
	if approves > 1 {
		return fmt.Errorf("%w: %q allows only one approve step", ErrInvalid, w.Name)
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
