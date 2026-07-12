package google

import (
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// googleCalendar is the subset of a Calendar resource vamoose reads. The primary
// calendar's id is the signed-in user's email address.
type googleCalendar struct {
	// ID is the calendar id, the user's email for the primary calendar.
	ID string `json:"id"`
	// Summary is the calendar's display name.
	Summary string `json:"summary"`
}

// googleEventDateTime is a Calendar event boundary, either an all-day date or a
// timed date-time paired with its zone.
type googleEventDateTime struct {
	// Date is the all-day date as YYYY-MM-DD. The end date is exclusive.
	Date string `json:"date,omitempty"`
	// DateTime is the timed wall-clock value in TimeZone, formatted per RFC3339.
	DateTime string `json:"dateTime,omitempty"`
	// TimeZone is the IANA zone name for DateTime.
	TimeZone string `json:"timeZone,omitempty"`
}

// googleAttendee is a Calendar event attendee.
type googleAttendee struct {
	// Email is the attendee's address, used as the key.
	Email string `json:"email"`
	// DisplayName is the attendee's display name.
	DisplayName string `json:"displayName,omitempty"`
	// Optional marks a guest whose presence is optional.
	Optional bool `json:"optional,omitempty"`
	// ResponseStatus is the attendee's reply, present on reads.
	ResponseStatus string `json:"responseStatus,omitempty"`
	// Organizer marks the event organizer, present on reads.
	Organizer bool `json:"organizer,omitempty"`
	// Self marks the signed-in user, present on reads.
	Self bool `json:"self,omitempty"`
}

// googleEvent is the subset of a Calendar event resource vamoose reads and writes.
type googleEvent struct {
	// ID is the event identifier, empty on create requests.
	ID string `json:"id,omitempty"`
	// Summary is the event title.
	Summary string `json:"summary,omitempty"`
	// Description is the event body.
	Description string `json:"description,omitempty"`
	// Start is the event start boundary.
	Start *googleEventDateTime `json:"start,omitempty"`
	// End is the event end boundary.
	End *googleEventDateTime `json:"end,omitempty"`
	// Transparency is "transparent" for free or "opaque" for busy.
	Transparency string `json:"transparency,omitempty"`
	// EventType is "default" or "outOfOffice".
	EventType string `json:"eventType,omitempty"`
	// Attendees are the invited people.
	Attendees []googleAttendee `json:"attendees,omitempty"`
}

// toGoogleEvent converts a Hold into a Calendar event for create requests.
// An out-of-office hold maps to a plain busy (opaque) block: Google's outOfOffice
// event type is Workspace-only, so it is not used, keeping away working on any
// account. Reads still recognize outOfOffice on a Workspace event.
func (p *Provider) toGoogleEvent(h calendar.Hold) googleEvent {
	return googleEvent{
		Summary:      h.Subject,
		Description:  h.Body,
		Transparency: transparencyFromShowAs(h.ShowAs),
		Start:        p.boundary(h.Start, h.AllDay),
		End:          p.boundary(h.End, h.AllDay),
		Attendees:    toGoogleAttendees(h.Attendees),
	}
}

// boundary renders a time as a Calendar event boundary. All-day boundaries use a bare
// date and name a calendar day, so they are not converted between zones. Timed
// boundaries are converted into the provider zone before formatting, so the labeled
// wall-clock string denotes the same instant even when t carries a different offset.
func (p *Provider) boundary(t time.Time, allDay bool) *googleEventDateTime {
	if allDay {
		return &googleEventDateTime{Date: t.Format("2006-01-02")}
	}
	return &googleEventDateTime{DateTime: t.In(p.location()).Format("2006-01-02T15:04:05"), TimeZone: p.timeZone}
}

// location returns the provider's time zone, falling back to UTC when the configured
// zone name does not load.
func (p *Provider) location() *time.Location {
	if loc, err := time.LoadLocation(p.timeZone); err == nil {
		return loc
	}
	return time.UTC
}

// toGoogleAttendees converts calendar attendees into Calendar attendees.
func toGoogleAttendees(in []calendar.Attendee) []googleAttendee {
	if len(in) == 0 {
		return nil
	}
	out := make([]googleAttendee, 0, len(in))
	for _, a := range in {
		out = append(out, googleAttendee{
			Email:       a.Person.Email,
			DisplayName: a.Person.Name,
			Optional:    optionalFromRole(a.Role),
		})
	}
	return out
}

// parseBoundary reads a Calendar event boundary into a time. All-day boundaries
// use the bare date; timed boundaries parse the RFC3339 date-time. A nil or
// unparseable boundary yields the zero time.
func parseBoundary(b *googleEventDateTime) time.Time {
	if b == nil {
		return time.Time{}
	}
	if b.Date != "" {
		t, _ := time.Parse("2006-01-02", b.Date)
		return t
	}
	t, _ := time.Parse(time.RFC3339, b.DateTime)
	return t
}

// fromGoogleEvent converts a Calendar event into a Hold.
func fromGoogleEvent(ev googleEvent) calendar.Hold {
	h := calendar.Hold{
		ID:      ev.ID,
		Subject: ev.Summary,
		Body:    ev.Description,
		Start:   parseBoundary(ev.Start),
		End:     parseBoundary(ev.End),
		AllDay:  ev.Start != nil && ev.Start.Date != "",
		ShowAs:  showAsFromTransparency(ev.Transparency),
	}
	if ev.EventType == "outOfOffice" {
		h.ShowAs = calendar.ShowOOF
	}
	for _, a := range ev.Attendees {
		h.Attendees = append(h.Attendees, calendar.Attendee{
			Person:   calendar.Person{Name: a.DisplayName, Email: a.Email},
			Role:     roleFromOptional(a.Optional),
			Response: responseFromGoogle(a.ResponseStatus),
		})
	}
	return h
}

// transparencyFromShowAs maps a neutral free/busy status to Google transparency.
// Google models only free versus busy, so anything but free is opaque (busy).
func transparencyFromShowAs(s calendar.ShowAs) string {
	if s == calendar.ShowFree {
		return "transparent"
	}
	return "opaque"
}

// showAsFromTransparency maps Google transparency to a neutral free/busy status.
func showAsFromTransparency(t string) calendar.ShowAs {
	if t == "transparent" {
		return calendar.ShowFree
	}
	return calendar.ShowBusy
}

// optionalFromRole reports whether a neutral role maps to an optional guest.
func optionalFromRole(r calendar.Role) bool {
	return r == calendar.RoleOptional
}

// roleFromOptional maps Google's optional guest flag to a neutral role.
func roleFromOptional(optional bool) calendar.Role {
	if optional {
		return calendar.RoleOptional
	}
	return calendar.RoleRequired
}

// responseFromGoogle maps a Google attendee response status to a neutral response.
func responseFromGoogle(s string) calendar.Response {
	switch s {
	case "needsAction":
		return calendar.ResponseNotResponded
	case "tentative":
		return calendar.ResponseTentative
	case "accepted":
		return calendar.ResponseAccepted
	case "declined":
		return calendar.ResponseDeclined
	default:
		return calendar.ResponseNone
	}
}
