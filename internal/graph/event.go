package graph

import (
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// graphUser is the subset of a Graph user resource vamoose reads.
type graphUser struct {
	// DisplayName is the user's display name.
	DisplayName string `json:"displayName"`
	// Mail is the primary SMTP address, sometimes empty.
	Mail string `json:"mail"`
	// UserPrincipalName is the sign-in name, used when Mail is empty.
	UserPrincipalName string `json:"userPrincipalName"`
}

// person converts a Graph user into a calendar.Person.
func (u graphUser) person() calendar.Person {
	email := u.Mail
	if email == "" {
		email = u.UserPrincipalName
	}
	return calendar.Person{Name: u.DisplayName, Email: email}
}

// graphDateTime is a Graph date/time paired with its time zone.
type graphDateTime struct {
	// DateTime is the wall-clock time in the given zone, without an offset.
	DateTime string `json:"dateTime"`
	// TimeZone is the IANA zone name for DateTime.
	TimeZone string `json:"timeZone"`
}

// graphEmailAddress is a Graph email address with an optional display name.
type graphEmailAddress struct {
	// Address is the SMTP address.
	Address string `json:"address"`
	// Name is the display name.
	Name string `json:"name"`
}

// graphAttendeeStatus is a Graph attendee's response status.
type graphAttendeeStatus struct {
	// Response is the attendee's reply, such as accepted or declined.
	Response string `json:"response"`
}

// graphAttendee is a Graph event attendee.
type graphAttendee struct {
	// EmailAddress identifies the attendee.
	EmailAddress graphEmailAddress `json:"emailAddress"`
	// Type is required, optional, or resource.
	Type string `json:"type"`
	// Status is the attendee's response, present on reads.
	Status *graphAttendeeStatus `json:"status,omitempty"`
}

// graphBody is a Graph item body.
type graphBody struct {
	// ContentType is HTML or text.
	ContentType string `json:"contentType"`
	// Content is the body content.
	Content string `json:"content"`
}

// graphEvent is the subset of a Graph event resource vamoose reads and writes.
type graphEvent struct {
	// ID is the event identifier, empty on create requests.
	ID string `json:"id,omitempty"`
	// Subject is the event title.
	Subject string `json:"subject,omitempty"`
	// Body is the event description.
	Body *graphBody `json:"body,omitempty"`
	// Start is the event start.
	Start *graphDateTime `json:"start,omitempty"`
	// End is the event end.
	End *graphDateTime `json:"end,omitempty"`
	// IsAllDay marks an all-day event.
	IsAllDay bool `json:"isAllDay,omitempty"`
	// ShowAs is the free/busy status.
	ShowAs string `json:"showAs,omitempty"`
	// ResponseRequested asks attendees to reply.
	ResponseRequested bool `json:"responseRequested,omitempty"`
	// Attendees are the invited people.
	Attendees []graphAttendee `json:"attendees,omitempty"`
}

// toGraphEvent converts a Hold into a Graph event for create requests.
func (p *Provider) toGraphEvent(h calendar.Hold) graphEvent {
	ev := graphEvent{
		Subject:           h.Subject,
		IsAllDay:          h.AllDay,
		ShowAs:            string(h.ShowAs),
		ResponseRequested: true,
		Start:             &graphDateTime{DateTime: formatTime(h.Start, h.AllDay), TimeZone: p.timeZone},
		End:               &graphDateTime{DateTime: formatTime(h.End, h.AllDay), TimeZone: p.timeZone},
		Attendees:         toGraphAttendees(h.Attendees),
	}
	if h.Body != "" {
		ev.Body = &graphBody{ContentType: "HTML", Content: h.Body}
	}
	return ev
}

// toGraphAttendees converts calendar attendees into Graph attendees.
func toGraphAttendees(in []calendar.Attendee) []graphAttendee {
	if len(in) == 0 {
		return nil
	}
	out := make([]graphAttendee, 0, len(in))
	for _, a := range in {
		out = append(out, graphAttendee{
			EmailAddress: graphEmailAddress{Address: a.Person.Email, Name: a.Person.Name},
			Type:         string(a.Role),
		})
	}
	return out
}

// formatTime renders a time as a Graph wall-clock string. All-day events are
// pinned to midnight so Graph treats the date boundaries correctly.
func formatTime(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("2006-01-02T00:00:00")
	}
	return t.Format("2006-01-02T15:04:05")
}

// fromGraphEvent converts a Graph event into a Hold.
func fromGraphEvent(ev graphEvent) calendar.Hold {
	h := calendar.Hold{
		ID:      ev.ID,
		Subject: ev.Subject,
		AllDay:  ev.IsAllDay,
		ShowAs:  calendar.ShowAs(ev.ShowAs),
	}
	if ev.Body != nil {
		h.Body = ev.Body.Content
	}
	for _, a := range ev.Attendees {
		att := calendar.Attendee{
			Person:   calendar.Person{Name: a.EmailAddress.Name, Email: a.EmailAddress.Address},
			Role:     calendar.Role(a.Type),
			Response: calendar.ResponseNone,
		}
		if a.Status != nil && a.Status.Response != "" {
			att.Response = calendar.Response(a.Status.Response)
		}
		h.Attendees = append(h.Attendees, att)
	}
	return h
}
