// Package workflow defines vamoose workflows: named, ordered sequences of the
// vamoose verbs (hold, approve, notify, away, event, cancel) that the run command
// executes and the daemon advances. Definitions are JSON, loaded either from a
// built-in template embedded in the binary or a user file under the config
// directory.
package workflow

import (
	"fmt"

	"github.com/dcadolph/vamoose/internal/calendar"
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
	// VerbAway marks the user out of office with no attendees.
	VerbAway Verb = "away"
	// VerbEvent creates a quick event, optionally inviting attendees.
	VerbEvent Verb = "event"
	// VerbCancel deletes the hold and notifies its attendees.
	VerbCancel Verb = "cancel"
)

// Creates reports whether the verb creates the hold a workflow acts on. Exactly
// one creating step leads every workflow.
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
	case VerbHold, VerbApprove, VerbNotify, VerbAway, VerbEvent, VerbCancel:
		return true
	default:
		return false
	}
}

// Step is one action in a workflow.
type Step struct {
	// Verb is the action this step performs.
	Verb Verb `json:"verb"`
	// ShowAs overrides the free/busy status for creating steps. Empty uses the
	// verb default: free for hold and event, out of office for away.
	ShowAs calendar.ShowAs `json:"showAs,omitempty"`
	// Manager, on an approve step, waits on the manager resolved from the directory
	// or the --manager flag. Reserved for future per-approver selection.
	Manager bool `json:"manager,omitempty"`
	// Team sets the role used when a notify step adds the team. Empty means optional.
	Team calendar.Role `json:"team,omitempty"`
}

// Workflow is a named, ordered sequence of steps.
type Workflow struct {
	// Name identifies the workflow and matches its file name without the extension.
	Name string `json:"name"`
	// Description is a one-line summary shown in listings.
	Description string `json:"description,omitempty"`
	// Steps run in order.
	Steps []Step `json:"steps"`
}

// Validate checks that the workflow is well formed: it has a name and at least one
// step, every verb is known, enum values are valid, exactly one creating step
// (hold, away, or event) leads the sequence, and no notify precedes an approve.
func (w Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("%w: missing name", ErrInvalid)
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("%w: %q has no steps", ErrInvalid, w.Name)
	}

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
	if firstApprove >= 0 && firstNotify >= 0 && firstNotify < firstApprove {
		return fmt.Errorf("%w: %q notifies the team before approval", ErrInvalid, w.Name)
	}
	if approves > 1 {
		return fmt.Errorf("%w: %q allows only one approve step", ErrInvalid, w.Name)
	}
	if firstApprove >= 0 {
		// Approval waits on the manager, whom only a hold step invites.
		if w.Steps[0].Verb != VerbHold {
			return fmt.Errorf("%w: approval requires a hold: %q starts with %s", ErrInvalid, w.Name, w.Steps[0].Verb)
		}
		// The engine advances only notify after approval today; anything else would
		// be silently skipped once the manager accepts.
		for i := firstApprove + 1; i < len(w.Steps); i++ {
			if w.Steps[i].Verb != VerbNotify {
				return fmt.Errorf("%w: only notify may follow approval: %q runs %s",
					ErrInvalid, w.Name, w.Steps[i].Verb)
			}
		}
	}
	return nil
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
