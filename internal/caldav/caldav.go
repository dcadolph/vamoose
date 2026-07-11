// Package caldav implements calendar.Provider over CalDAV, targeting Apple iCloud
// by default. CalDAV has no directory, so manager and team lookups are unsupported;
// callers pass the manager explicitly and set the team with the team config.
//
// iCloud sends invitations to attendees when an event is created, but it does not
// reliably report attendee accept/decline back over CalDAV, so the approval-based
// flow (check, daemon auto-promote) cannot detect approval on iCloud. Use CalDAV
// for creating holds, quick events, away blocks, and notifying the team; promote
// by hand once you know the manager accepted.
package caldav

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// Provider is a CalDAV calendar.Provider.
type Provider struct {
	// client issues CalDAV requests to the server.
	client *caldav.Client
	// httpc is the authenticated HTTP client, retained for raw MKCALENDAR requests.
	httpc webdav.HTTPClient
	// endpoint is the CalDAV server root, used to resolve discovered URLs.
	endpoint string
	// username is the account address, used as the ORGANIZER and returned by Me.
	username string
	// calendarName is the preferred calendar to write to, empty for the first one.
	calendarName string
	// timeZone is the IANA zone used when reading event times.
	timeZone string
	// prodID is the iCalendar PRODID stamped on created events.
	prodID string
	// status optionally reads attendee responses from an external source, such as
	// macOS EventKit, recovering approval detection that iCloud CalDAV omits.
	status StatusFunc

	// mu guards the lazily discovered paths.
	mu sync.Mutex
	// calendarPath is the resolved target calendar collection, discovered on first use.
	calendarPath string
	// homeSet is the discovered calendar home set, where new calendars are created.
	homeSet string
}

// Option configures a Provider.
type Option func(*Provider)

// WithTimeZone sets the IANA time zone used when reading event times.
func WithTimeZone(tz string) Option {
	return func(p *Provider) {
		if tz != "" {
			p.timeZone = tz
		}
	}
}

// WithCalendarName sets the target calendar by display name, defaulting to the
// first calendar that supports events.
func WithCalendarName(name string) Option {
	return func(p *Provider) { p.calendarName = name }
}

// StatusFunc returns attendee responses for an event UID, keyed by lowercase email.
type StatusFunc func(ctx context.Context, uid string) (map[string]calendar.Response, error)

// WithStatus sets a source of attendee responses that overrides those from the
// server, for backends that do not report accept/decline over CalDAV.
func WithStatus(fn StatusFunc) Option { return func(p *Provider) { p.status = fn } }

// NewProvider returns a CalDAV Provider for the endpoint, authenticating with the
// username and password over HTTP Basic. For iCloud the password is an
// app-specific password.
func NewProvider(endpoint, username, password string, opts ...Option) (*Provider, error) {
	if endpoint == "" || username == "" || password == "" {
		return nil, fmt.Errorf("%w: endpoint, username, and password required", ErrCalDAV)
	}
	httpc := webdav.HTTPClientWithBasicAuth(&http.Client{Timeout: 30 * time.Second}, username, password)
	cl, err := caldav.NewClient(httpc, endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: client: %v", ErrCalDAV, err)
	}
	p := &Provider{
		client:   cl,
		httpc:    httpc,
		endpoint: endpoint,
		username: username,
		timeZone: "UTC",
		prodID:   "-//vamoose//caldav//EN",
	}
	for _, o := range opts {
		o(p)
	}
	return p, nil
}

// Me returns the signed-in account.
func (p *Provider) Me(context.Context) (calendar.Person, error) {
	return calendar.Person{Email: p.username}, nil
}

// Manager is unsupported on CalDAV, which has no directory.
func (p *Provider) Manager(context.Context) (calendar.Person, error) {
	return calendar.Person{}, calendar.ErrNoManager
}

// Team is unsupported on CalDAV, which has no directory. Callers set the team
// explicitly instead.
func (p *Provider) Team(context.Context) ([]calendar.Person, error) {
	return nil, calendar.ErrNoDirectory
}

// CreateHold creates the event and lets the server invite its attendees.
func (p *Provider) CreateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	if err := p.ensure(ctx); err != nil {
		return calendar.Hold{}, err
	}
	uid, err := newUID()
	if err != nil {
		return calendar.Hold{}, err
	}
	cal := p.toCalendar(h, uid, time.Now().UTC())
	path := p.calendarPath + uid + ".ics"
	obj, err := p.client.PutCalendarObject(ctx, path, cal)
	if err != nil {
		return calendar.Hold{}, fmt.Errorf("%w: create: %v", ErrCalDAV, err)
	}
	if obj != nil && obj.Path != "" {
		path = obj.Path
	}
	h.ID = path
	return h, nil
}

// GetHold fetches a hold's current state, including any attendee responses the
// server exposes.
func (p *Provider) GetHold(ctx context.Context, id string) (calendar.Hold, error) {
	obj, err := p.client.GetCalendarObject(ctx, id)
	if err != nil {
		return calendar.Hold{}, fmt.Errorf("%w: get: %v", ErrCalDAV, err)
	}
	comp := firstEvent(obj.Data)
	if comp == nil {
		return calendar.Hold{}, calendar.ErrNotFound
	}
	h := p.fromEvent(comp, id)
	if p.status != nil {
		if uid, _ := comp.Props.Text(ical.PropUID); uid != "" {
			if statuses, serr := p.status(ctx, uid); serr == nil {
				applyStatuses(&h, statuses)
			}
		}
	}
	return h, nil
}

// applyStatuses overrides attendee responses from an external status map keyed by
// lowercase email, recovering approvals a backend does not report over CalDAV.
func applyStatuses(h *calendar.Hold, statuses map[string]calendar.Response) {
	for i := range h.Attendees {
		if r, ok := statuses[strings.ToLower(h.Attendees[i].Person.Email)]; ok {
			h.Attendees[i].Response = r
		}
	}
}

// UpdateHold rewrites an existing hold, preserving its identity, and lets the
// server send updates to attendees.
func (p *Provider) UpdateHold(ctx context.Context, h calendar.Hold) (calendar.Hold, error) {
	if h.ID == "" {
		return calendar.Hold{}, calendar.ErrNotFound
	}
	obj, err := p.client.GetCalendarObject(ctx, h.ID)
	if err != nil {
		return calendar.Hold{}, fmt.Errorf("%w: get for update: %v", ErrCalDAV, err)
	}
	comp := firstEvent(obj.Data)
	if comp == nil {
		return calendar.Hold{}, calendar.ErrNotFound
	}
	p.applyHold(comp, h)
	if _, err := p.client.PutCalendarObject(ctx, h.ID, obj.Data); err != nil {
		return calendar.Hold{}, fmt.Errorf("%w: update: %v", ErrCalDAV, err)
	}
	return h, nil
}

// DeleteHold cancels the event and lets the server notify its attendees. It refuses
// to delete anything but a single event object, so it can never remove a calendar
// collection and wipe events it did not create.
func (p *Provider) DeleteHold(ctx context.Context, id string) error {
	if !isEventPath(id) {
		return fmt.Errorf("%w: refusing to delete %q: not a single event", ErrCalDAV, id)
	}
	if err := p.client.RemoveAll(ctx, id); err != nil {
		return fmt.Errorf("%w: delete: %v", ErrCalDAV, err)
	}
	return nil
}

// isEventPath reports whether id addresses a single event object rather than a
// calendar collection, guarding against deleting a whole calendar.
func isEventPath(id string) bool {
	return id != "" && !strings.HasSuffix(id, "/") && strings.HasSuffix(id, ".ics")
}

// ensure discovers the target calendar collection once and caches its path.
func (p *Provider) ensure(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calendarPath != "" {
		return nil
	}
	principal, err := p.client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return fmt.Errorf("%w: find principal: %v", ErrCalDAV, err)
	}
	home, err := p.client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return fmt.Errorf("%w: find calendar home: %v", ErrCalDAV, err)
	}
	cals, err := p.client.FindCalendars(ctx, home)
	if err != nil {
		return fmt.Errorf("%w: list calendars: %v", ErrCalDAV, err)
	}
	path := pickCalendar(cals, p.calendarName)
	if path == "" {
		return fmt.Errorf("%w: no writable calendar found", ErrCalDAV)
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	p.calendarPath = path
	return nil
}

// toCalendar builds an iCalendar object for a new hold.
func (p *Provider) toCalendar(h calendar.Hold, uid string, stamp time.Time) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, p.prodID)
	ev := ical.NewEvent()
	ev.Props.SetText(ical.PropUID, uid)
	ev.Props.SetDateTime(ical.PropDateTimeStamp, stamp)
	p.applyHold(ev.Component, h)
	cal.Children = append(cal.Children, ev.Component)
	return cal
}

// applyHold writes a hold's fields onto an event component, replacing the subject,
// times, transparency, organizer, and attendees while leaving identity untouched.
func (p *Provider) applyHold(comp *ical.Component, h calendar.Hold) {
	props := comp.Props
	props.SetText(ical.PropSummary, h.Subject)
	if h.Body != "" {
		props.SetText(ical.PropDescription, h.Body)
	} else {
		props.Del(ical.PropDescription)
	}
	if h.AllDay {
		props.SetDate(ical.PropDateTimeStart, h.Start)
		props.SetDate(ical.PropDateTimeEnd, h.End)
	} else {
		props.SetDateTime(ical.PropDateTimeStart, h.Start.UTC())
		props.SetDateTime(ical.PropDateTimeEnd, h.End.UTC())
	}
	props.SetText(ical.PropTransparency, transparency(h.ShowAs))
	props.Set(&ical.Prop{Name: ical.PropOrganizer, Value: mailto(p.username)})
	props.Del(ical.PropAttendee)
	for _, a := range h.Attendees {
		props.Add(attendeeProp(a))
	}
}

// fromEvent maps an event component to a neutral hold.
func (p *Provider) fromEvent(comp *ical.Component, id string) calendar.Hold {
	loc := p.location()
	h := calendar.Hold{ID: id}
	h.Subject, _ = comp.Props.Text(ical.PropSummary)
	h.Body, _ = comp.Props.Text(ical.PropDescription)
	if dt := comp.Props.Get(ical.PropDateTimeStart); dt != nil {
		if strings.EqualFold(param(dt, "VALUE"), string(ical.ValueDate)) {
			h.AllDay = true
		}
		if t, err := comp.Props.DateTime(ical.PropDateTimeStart, loc); err == nil {
			h.Start = t
		}
	}
	if t, err := comp.Props.DateTime(ical.PropDateTimeEnd, loc); err == nil {
		h.End = t
	}
	if t, _ := comp.Props.Text(ical.PropTransparency); strings.EqualFold(t, "TRANSPARENT") {
		h.ShowAs = calendar.ShowFree
	} else {
		h.ShowAs = calendar.ShowBusy
	}
	for _, pr := range comp.Props.Values(ical.PropAttendee) {
		pr := pr
		h.Attendees = append(h.Attendees, calendar.Attendee{
			Person:   calendar.Person{Email: unMailto(pr.Value), Name: param(&pr, ical.ParamCommonName)},
			Role:     roleFrom(param(&pr, ical.ParamRole)),
			Response: responseFrom(param(&pr, ical.ParamParticipationStatus)),
		})
	}
	return h
}

// location returns the provider's time zone, falling back to UTC.
func (p *Provider) location() *time.Location {
	if loc, err := time.LoadLocation(p.timeZone); err == nil {
		return loc
	}
	return time.UTC
}

// pickCalendar chooses the target calendar: the one matching name, else the first
// that supports events, else the first available.
func pickCalendar(cals []caldav.Calendar, name string) string {
	if name != "" {
		for _, c := range cals {
			if strings.EqualFold(c.Name, name) {
				return c.Path
			}
		}
	}
	for _, c := range cals {
		if supportsEvents(c) {
			return c.Path
		}
	}
	if len(cals) > 0 {
		return cals[0].Path
	}
	return ""
}

// supportsEvents reports whether a calendar accepts events. An empty component set
// is treated as permissive.
func supportsEvents(c caldav.Calendar) bool {
	if len(c.SupportedComponentSet) == 0 {
		return true
	}
	for _, s := range c.SupportedComponentSet {
		if s == "VEVENT" {
			return true
		}
	}
	return false
}

// firstEvent returns the first VEVENT component of a calendar, or nil.
func firstEvent(cal *ical.Calendar) *ical.Component {
	if cal == nil {
		return nil
	}
	for _, c := range cal.Children {
		if c.Name == "VEVENT" {
			return c
		}
	}
	return nil
}

// attendeeProp builds an ATTENDEE property from an attendee.
func attendeeProp(a calendar.Attendee) *ical.Prop {
	role := "REQ-PARTICIPANT"
	if a.Role == calendar.RoleOptional {
		role = "OPT-PARTICIPANT"
	}
	params := ical.Params{
		ical.ParamRole:                {role},
		ical.ParamParticipationStatus: {partstat(a.Response)},
		ical.ParamRSVP:                {"TRUE"},
	}
	if a.Person.Name != "" {
		params[ical.ParamCommonName] = []string{a.Person.Name}
	}
	return &ical.Prop{Name: ical.PropAttendee, Value: mailto(a.Person.Email), Params: params}
}

// transparency maps a free/busy status to an iCalendar TRANSP value.
func transparency(s calendar.ShowAs) string {
	if s == calendar.ShowFree {
		return "TRANSPARENT"
	}
	return "OPAQUE"
}

// partstat maps a response to an iCalendar PARTSTAT value.
func partstat(r calendar.Response) string {
	switch r {
	case calendar.ResponseAccepted:
		return "ACCEPTED"
	case calendar.ResponseDeclined:
		return "DECLINED"
	case calendar.ResponseTentative:
		return "TENTATIVE"
	default:
		return "NEEDS-ACTION"
	}
}

// responseFrom maps an iCalendar PARTSTAT value to a response.
func responseFrom(v string) calendar.Response {
	switch strings.ToUpper(v) {
	case "ACCEPTED":
		return calendar.ResponseAccepted
	case "DECLINED":
		return calendar.ResponseDeclined
	case "TENTATIVE":
		return calendar.ResponseTentative
	default:
		return calendar.ResponseNotResponded
	}
}

// roleFrom maps an iCalendar ROLE value to an attendee role.
func roleFrom(v string) calendar.Role {
	if strings.EqualFold(v, "OPT-PARTICIPANT") {
		return calendar.RoleOptional
	}
	return calendar.RoleRequired
}

// param returns the first value of a property parameter, or empty.
func param(p *ical.Prop, name string) string {
	if p == nil {
		return ""
	}
	if vs := p.Params[name]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// mailto prefixes an address with the mailto scheme for calendar user addresses.
func mailto(email string) string {
	if strings.Contains(email, ":") {
		return email
	}
	return "mailto:" + email
}

// unMailto strips the mailto scheme from a calendar user address.
func unMailto(v string) string {
	return strings.TrimPrefix(strings.TrimPrefix(v, "mailto:"), "MAILTO:")
}

// newUID returns a unique iCalendar UID.
func newUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: uid: %v", ErrCalDAV, err)
	}
	return hex.EncodeToString(b) + "@vamoose", nil
}

// CalendarInfo names a calendar collection.
type CalendarInfo struct {
	// Name is the calendar display name, used to select it with the calendar option.
	Name string
	// Path is the calendar collection path.
	Path string
}

// ListCalendars returns the calendars in the account.
func (p *Provider) ListCalendars(ctx context.Context) ([]CalendarInfo, error) {
	home, err := p.home(ctx)
	if err != nil {
		return nil, err
	}
	cals, err := p.client.FindCalendars(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("%w: list calendars: %v", ErrCalDAV, err)
	}
	out := make([]CalendarInfo, 0, len(cals))
	for _, c := range cals {
		out = append(out, CalendarInfo{Name: c.Name, Path: c.Path})
	}
	return out, nil
}

// CreateCalendar creates a new calendar with the given display name and returns its
// path. It lets a caller make a dedicated calendar without the Calendar app.
func (p *Provider) CreateCalendar(ctx context.Context, displayName string) (string, error) {
	if displayName == "" {
		return "", fmt.Errorf("%w: calendar name required", ErrCalDAV)
	}
	home, err := p.home(ctx)
	if err != nil {
		return "", err
	}
	slug, err := randomSlug()
	if err != nil {
		return "", err
	}
	base, err := url.Parse(p.endpoint)
	if err != nil {
		return "", fmt.Errorf("%w: endpoint url: %v", ErrCalDAV, err)
	}
	ref, err := url.Parse(home)
	if err != nil {
		return "", fmt.Errorf("%w: home url: %v", ErrCalDAV, err)
	}
	u := *base.ResolveReference(ref)
	u.Path = strings.TrimRight(u.Path, "/") + "/vamoose-" + slug + "/"

	req, err := http.NewRequestWithContext(ctx, "MKCALENDAR", u.String(), strings.NewReader(mkcalendarBody(displayName)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := p.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: mkcalendar: %v", ErrCalDAV, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("%w: mkcalendar failed: %s: %s", ErrCalDAV, resp.Status, strings.TrimSpace(string(b)))
	}
	return u.Path, nil
}

// home discovers and caches the calendar home set, where new calendars are created.
func (p *Provider) home(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.homeSet != "" {
		return p.homeSet, nil
	}
	principal, err := p.client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: find principal: %v", ErrCalDAV, err)
	}
	hs, err := p.client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return "", fmt.Errorf("%w: find calendar home: %v", ErrCalDAV, err)
	}
	p.homeSet = hs
	return hs, nil
}

// mkcalendarTemplate is the MKCALENDAR request body, with the display name escaped in.
const mkcalendarTemplate = `<?xml version="1.0" encoding="utf-8"?>
<C:mkcalendar xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:set>
    <D:prop>
      <D:displayname>%s</D:displayname>
      <C:supported-calendar-component-set>
        <C:comp name="VEVENT"/>
      </C:supported-calendar-component-set>
    </D:prop>
  </D:set>
</C:mkcalendar>`

// mkcalendarBody renders the MKCALENDAR body with the display name XML-escaped.
func mkcalendarBody(displayName string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(displayName))
	return fmt.Sprintf(mkcalendarTemplate, b.String())
}

// randomSlug returns a short random hex string for a unique collection segment.
func randomSlug() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: slug: %v", ErrCalDAV, err)
	}
	return hex.EncodeToString(b), nil
}
