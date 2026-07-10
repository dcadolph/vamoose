// Package audit records workflow run events for trust and debugging: what a hold did,
// when, and who approved it. It is append-only history, separate from the live watch
// state the daemon advances. Records go to a 0600 file, sealed with AES-256-GCM when an
// encryption key is set, so the history is safe at rest on a hosted server.
package audit

import (
	"context"
	"time"
)

// Action names a recorded workflow event.
type Action string

const (
	// ActionCreated is the initial hold creation.
	ActionCreated Action = "created"
	// ActionApproved is an approver accepting a gate.
	ActionApproved Action = "approved"
	// ActionDeclined is an approver declining a gate.
	ActionDeclined Action = "declined"
	// ActionExpired is a gate passing its timeout with no response.
	ActionExpired Action = "expired"
	// ActionAdvanced is the run moving to the next gate in a chain.
	ActionAdvanced Action = "advanced"
	// ActionNotified is the team being added to the hold.
	ActionNotified Action = "notified"
	// ActionNoted is a marker event written to the requester's calendar.
	ActionNoted Action = "noted"
	// ActionMessaged is a message posted to a comms channel.
	ActionMessaged Action = "messaged"
	// ActionCanceled is the hold being deleted.
	ActionCanceled Action = "canceled"
)

// Event is one recorded moment in a workflow run. The caller stamps Time so the record
// is deterministic in tests.
type Event struct {
	// Time is when the event happened.
	Time time.Time `json:"time"`
	// Workflow is the workflow name driving the hold.
	Workflow string `json:"workflow,omitempty"`
	// Provider is the calendar provider that owns the hold.
	Provider string `json:"provider,omitempty"`
	// HoldID is the provider event identifier.
	HoldID string `json:"hold_id,omitempty"`
	// Action is what happened.
	Action Action `json:"action"`
	// Actor is who caused it, such as the approver, empty for a system step.
	Actor string `json:"actor,omitempty"`
	// Detail is freeform context, such as a message channel or the next approver.
	Detail string `json:"detail,omitempty"`
}

// Recorder appends a workflow event to the run history.
type Recorder interface {
	// Record appends the event.
	Record(ctx context.Context, e Event) error
}

// RecorderFunc adapts a plain function to a Recorder.
type RecorderFunc func(ctx context.Context, e Event) error

// Record calls f.
func (f RecorderFunc) Record(ctx context.Context, e Event) error { return f(ctx, e) }

// Nop is a Recorder that discards events, used when a history file cannot be opened so
// auditing never blocks a workflow.
var Nop Recorder = RecorderFunc(func(context.Context, Event) error { return nil })
