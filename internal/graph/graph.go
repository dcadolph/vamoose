// Package graph implements calendar.Provider using Microsoft Graph, which backs
// Outlook, Microsoft 365, and Teams calendars through one API.
package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TokenSource returns a valid Graph bearer access token.
type TokenSource func(ctx context.Context) (string, error)

// Provider is a Microsoft Graph calendar.Provider.
type Provider struct {
	// token supplies bearer tokens for each request.
	token TokenSource
	// client issues HTTP requests to Graph.
	client *http.Client
	// baseURL is the Graph API root.
	baseURL string
	// timeZone is the IANA zone used when sending event times.
	timeZone string
}

// Option configures a Provider.
type Option func(*Provider)

// WithHTTPClient sets the HTTP client.
func WithHTTPClient(c *http.Client) Option { return func(p *Provider) { p.client = c } }

// WithBaseURL overrides the Graph API root, mainly for testing.
func WithBaseURL(u string) Option { return func(p *Provider) { p.baseURL = u } }

// WithTimeZone sets the IANA time zone used for event times.
func WithTimeZone(tz string) Option {
	return func(p *Provider) {
		if tz != "" {
			p.timeZone = tz
		}
	}
}

// NewProvider returns a Graph Provider using the given token source.
// It panics if token is nil, signaling developer error.
func NewProvider(token TokenSource, opts ...Option) *Provider {
	if token == nil {
		panic("graph.NewProvider: token source required")
	}
	p := &Provider{
		token:    token,
		client:   &http.Client{Timeout: 30 * time.Second},
		baseURL:  "https://graph.microsoft.com/v1.0",
		timeZone: "UTC",
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Me returns the signed-in user.
func (p *Provider) Me(ctx context.Context) (calendar.Person, error) {
	var u graphUser
	if err := p.do(ctx, http.MethodGet, "/me?$select=displayName,mail,userPrincipalName", nil, &u); err != nil {
		return calendar.Person{}, err
	}
	return u.person(), nil
}

// Manager returns the signed-in user's manager, or ErrNoManager when unset.
func (p *Provider) Manager(ctx context.Context) (calendar.Person, error) {
	var u graphUser
	err := p.do(ctx, http.MethodGet, "/me/manager?$select=displayName,mail,userPrincipalName", nil, &u)
	if errors.Is(err, calendar.ErrNotFound) {
		return calendar.Person{}, calendar.ErrNoManager
	}
	if err != nil {
		return calendar.Person{}, err
	}
	return u.person(), nil
}

// Team returns the manager's direct reports, excluding the signed-in user.
func (p *Provider) Team(ctx context.Context) ([]calendar.Person, error) {
	var out struct {
		Value []graphUser `json:"value"`
	}
	if err := p.do(ctx, http.MethodGet, "/me/manager/directReports?$select=displayName,mail,userPrincipalName", nil, &out); err != nil {
		return nil, err
	}
	me, err := p.Me(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]calendar.Person, 0, len(out.Value))
	for _, u := range out.Value {
		person := u.person()
		if strings.EqualFold(person.Email, me.Email) {
			continue
		}
		people = append(people, person)
	}
	return people, nil
}

// CreateHold creates the event and sends invites to its attendees.
func (p *Provider) CreateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	var ev graphEvent
	if err := p.do(ctx, http.MethodPost, "/me/events", p.toGraphEvent(h), &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGraphEvent(ev), nil
}

// GetHold fetches a hold's current state, including attendee responses.
func (p *Provider) GetHold(ctx context.Context, id string) (calendar.Hold, error) {
	path := "/me/events/" + url.PathEscape(id) + "?$select=id,subject,showAs,isAllDay,body,attendees"
	var ev graphEvent
	if err := p.do(ctx, http.MethodGet, path, nil, &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGraphEvent(ev), nil
}

// UpdateHold patches an existing hold and sends updates to attendees.
func (p *Provider) UpdateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	if h.ID == "" {
		return calendar.Hold{}, calendar.ErrNotFound
	}
	patch := graphEvent{
		Subject:           h.Subject,
		ShowAs:            showAsToGraph(h.ShowAs),
		ResponseRequested: true,
		Attendees:         toGraphAttendees(h.Attendees),
	}
	var ev graphEvent
	if err := p.do(ctx, http.MethodPatch, "/me/events/"+url.PathEscape(h.ID), patch, &ev); err != nil {
		return calendar.Hold{}, err
	}
	return fromGraphEvent(ev), nil
}

// DeleteHold cancels the event and notifies its attendees. Graph sends
// cancellations to attendees automatically on delete, so no send flag is needed.
func (p *Provider) DeleteHold(ctx context.Context, id string) error {
	return p.do(ctx, http.MethodDelete, "/me/events/"+url.PathEscape(id), nil, nil)
}

// do executes a Graph request, encoding in as JSON and decoding into out.
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

// apiError maps a non-2xx Graph response to an error.
func apiError(status int, data []byte) error {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: %s", calendar.ErrNotFound, e.Error.Message)
	}
	if e.Error.Code != "" {
		return fmt.Errorf("%w: %d %s: %s", ErrGraph, status, e.Error.Code, e.Error.Message)
	}
	return fmt.Errorf("%w: status %d", ErrGraph, status)
}
