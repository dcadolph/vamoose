// Package hris files approved leave with an HR system: the leave axis of the workflow
// engine. Once a manager approves time off, vamoose can record it as real leave, not just
// a calendar hold. BambooHR is the first system behind the Filer seam.
package hris

import (
	"context"
	"time"
)

// Leave is a time-off request to file: who, when, what type, and a note.
type Leave struct {
	// EmployeeID is the HR system's identifier for the person taking leave.
	EmployeeID string
	// TypeID is the HR system's time-off type identifier, such as vacation.
	TypeID string
	// Start is the first day off.
	Start time.Time
	// End is the last day off.
	End time.Time
	// Note is a short description recorded with the request.
	Note string
}

// Filer files approved leave with an HR system.
type Filer interface {
	// FileLeave records the leave and returns the created request's identifier.
	FileLeave(ctx context.Context, leave Leave) (string, error)
}

// FilerFunc adapts a plain function to a Filer.
type FilerFunc func(ctx context.Context, leave Leave) (string, error)

// FileLeave calls f.
func (f FilerFunc) FileLeave(ctx context.Context, leave Leave) (string, error) {
	return f(ctx, leave)
}
