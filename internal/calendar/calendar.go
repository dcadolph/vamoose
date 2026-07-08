// Package calendar defines the provider-agnostic model for vacation holds and
// the Provider interface that calendar backends implement.
package calendar

import (
	"context"
	"strings"
	"time"
)

// ShowAs is the free/busy status a calendar shows for an event.
type ShowAs string

const (
	// ShowFree marks time as available so it does not block a calendar.
	ShowFree ShowAs = "free"
	// ShowTentative marks time as tentatively busy.
	ShowTentative ShowAs = "tentative"
	// ShowBusy marks time as busy.
	ShowBusy ShowAs = "busy"
	// ShowOOF marks time as out of office.
	ShowOOF ShowAs = "oof"
)

// Response is an attendee's reply to an invite.
type Response string

const (
	// ResponseNone means no response has been recorded.
	ResponseNone Response = "none"
	// ResponseNotResponded means the attendee has not yet replied.
	ResponseNotResponded Response = "notResponded"
	// ResponseTentative means the attendee tentatively accepted.
	ResponseTentative Response = "tentativelyAccepted"
	// ResponseAccepted means the attendee accepted. Vamoose treats this as approval.
	ResponseAccepted Response = "accepted"
	// ResponseDeclined means the attendee declined. Vamoose treats this as rejection.
	ResponseDeclined Response = "declined"
)

// Role is an attendee's participation requirement.
type Role string

const (
	// RoleRequired marks an attendee whose presence is required. The manager
	// carries this role so their acceptance stands in for approval.
	RoleRequired Role = "required"
	// RoleOptional marks an attendee whose presence is optional. Team members
	// carry this role so the hold never blocks their calendars.
	RoleOptional Role = "optional"
)

// Person identifies a user by display name and email address.
type Person struct {
	// Name is the display name.
	Name string
	// Email is the SMTP address, used as the attendee key.
	Email string
}

// Attendee is a Person invited to an event in a given Role.
type Attendee struct {
	// Person is the invited user.
	Person Person
	// Role is required or optional.
	Role Role
	// Response is the attendee's latest reply.
	Response Response
}

// Hold is a vacation request modeled as a calendar event.
type Hold struct {
	// ID is the provider event identifier, empty before creation.
	ID string
	// Subject is the event title.
	Subject string
	// Body is the event description.
	Body string
	// Start is the departure time.
	Start time.Time
	// End is the return time. For all-day holds it is exclusive.
	End time.Time
	// AllDay marks the hold as spanning full days.
	AllDay bool
	// ShowAs is the free/busy status shown to attendees.
	ShowAs ShowAs
	// Attendees are the people invited to the hold.
	Attendees []Attendee
}

// ApprovedBy reports whether the given email has accepted the hold.
func (h Hold) ApprovedBy(email string) bool {
	for _, a := range h.Attendees {
		if strings.EqualFold(a.Person.Email, email) {
			return a.Response == ResponseAccepted
		}
	}
	return false
}

// DeclinedBy reports whether the given email has declined the hold.
func (h Hold) DeclinedBy(email string) bool {
	for _, a := range h.Attendees {
		if strings.EqualFold(a.Person.Email, email) {
			return a.Response == ResponseDeclined
		}
	}
	return false
}

// Provider creates and advances vacation holds on a calendar backend.
type Provider interface {
	// Me returns the signed-in user.
	Me(ctx context.Context) (Person, error)
	// Manager returns the signed-in user's manager.
	Manager(ctx context.Context) (Person, error)
	// Team returns the signed-in user's peers, the manager's direct reports.
	Team(ctx context.Context) ([]Person, error)
	// CreateHold creates the event and sends invites to its attendees.
	CreateHold(ctx context.Context, hold Hold) (Hold, error)
	// GetHold fetches a hold's current state, including attendee responses.
	GetHold(ctx context.Context, id string) (Hold, error)
	// UpdateHold patches an existing hold and sends updates to attendees.
	UpdateHold(ctx context.Context, hold Hold) (Hold, error)
	// DeleteHold cancels the hold and notifies its attendees.
	DeleteHold(ctx context.Context, id string) error
}
