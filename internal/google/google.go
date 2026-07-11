// Package google implements calendar.Provider using the Google Calendar API v3.
// Google Calendar has no directory, so manager and team lookups are unsupported;
// callers pass the manager explicitly and set the team with the team config.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TokenSource returns a valid Google OAuth bearer access token.
type TokenSource func(ctx context.Context) (string, error)

// Provider is a Google Calendar calendar.Provider.
type Provider struct {
	// token supplies bearer tokens for each request.
	token TokenSource
	// client issues HTTP requests to the Calendar API.
	client *http.Client
	// baseURL is the Calendar API root.
	baseURL string
	// timeZone is the IANA zone used when sending timed event times.
	timeZone string
	// calendarID is the target calendar, "primary" for the signed-in user.
	calendarID string
}

// Option configures a Provider.
type Option func(*Provider)

// WithHTTPClient sets the HTTP client.
func WithHTTPClient(c *http.Client) Option { return func(p *Provider) { p.client = c } }

// WithBaseURL overrides the Calendar API root, mainly for testing.
func WithBaseURL(u string) Option { return func(p *Provider) { p.baseURL = u } }

// WithTimeZone sets the IANA time zone used for timed event times.
func WithTimeZone(tz string) Option {
	return func(p *Provider) {
		if tz != "" {
			p.timeZone = tz
		}
	}
}

// WithCalendarID sets the target calendar id, defaulting to the primary calendar.
func WithCalendarID(id string) Option {
	return func(p *Provider) {
		if id != "" {
			p.calendarID = id
		}
	}
}

// NewProvider returns a Google Provider using the given token source.
// It panics if token is nil, signaling developer error.
func NewProvider(token TokenSource, opts ...Option) *Provider {
	if token == nil {
		panic("google.NewProvider: token source required")
	}
	p := &Provider{
		token:      token,
		client:     &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://www.googleapis.com/calendar/v3",
		timeZone:   "UTC",
		calendarID: "primary",
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Me returns the signed-in user, whose email is the primary calendar id.
func (p *Provider) Me(ctx context.Context) (calendar.Person, error) {
	var c googleCalendar
	if err := p.do(ctx, http.MethodGet, "/calendars/primary", nil, &c); err != nil {
		return calendar.Person{}, err
	}
	return calendar.Person{Email: c.ID}, nil
}

// Manager is unsupported on Google Calendar, which has no directory.
func (p *Provider) Manager(context.Context) (calendar.Person, error) {
	return calendar.Person{}, calendar.ErrNoManager
}

// Team is unsupported on Google Calendar, which has no directory. Callers set
// the team explicitly instead.
func (p *Provider) Team(context.Context) ([]calendar.Person, error) {
	return nil, calendar.ErrNoDirectory
}

// CreateHold creates the event and sends invites to its attendees.
func (p *Provider) CreateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	var ev googleEvent
	if err := p.do(ctx, http.MethodPost, p.eventsPath()+"?sendUpdates=all", p.toGoogleEvent(h), &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGoogleEvent(ev), nil
}

// GetHold fetches a hold's current state, including attendee responses.
func (p *Provider) GetHold(ctx context.Context, id string) (calendar.Hold, error) {
	var ev googleEvent
	if err := p.do(ctx, http.MethodGet, p.eventPath(id), nil, &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGoogleEvent(ev), nil
}

// UpdateHold patches an existing hold and sends updates to attendees.
func (p *Provider) UpdateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	if h.ID == "" {
		return calendar.Hold{}, calendar.ErrNotFound
	}
	patch := googleEvent{
		Summary:      h.Subject,
		Transparency: transparencyFromShowAs(h.ShowAs),
		Attendees:    toGoogleAttendees(h.Attendees),
	}
	var ev googleEvent
	if err := p.do(ctx, http.MethodPatch, p.eventPath(h.ID)+"?sendUpdates=all", patch, &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGoogleEvent(ev), nil
}

// DeleteHold cancels the event and notifies its attendees.
func (p *Provider) DeleteHold(ctx context.Context, id string) error {
	return p.do(ctx, http.MethodDelete, p.eventPath(id)+"?sendUpdates=all", nil, nil)
}

// eventsPath is the collection path for the target calendar's events.
func (p *Provider) eventsPath() string {
	return "/calendars/" + url.PathEscape(p.calendarID) + "/events"
}

// eventPath is the resource path for a single event by id.
func (p *Provider) eventPath(id string) string {
	return p.eventsPath() + "/" + url.PathEscape(id)
}

// do executes a Calendar API request, encoding in as JSON and decoding into out.
// A nil in sends no body; a nil out discards the response body.
func (p *Provider) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return err
	}
	tok, err := p.token(ctx)
	if err != nil {
		return fmt.Errorf("acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// apiError maps a non-2xx Calendar API response to an error.
func apiError(status int, data []byte) error {
	var e struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: %s", calendar.ErrNotFound, e.Error.Message)
	}
	if e.Error.Message != "" {
		return fmt.Errorf("%w: %d %s", ErrGoogle, status, e.Error.Message)
	}
	return fmt.Errorf("%w: status %d", ErrGoogle, status)
}
