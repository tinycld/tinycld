package caldav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/core"
)

// Backend implements go-webdav's caldav.Backend against a PocketBase app,
// driven by one Source.
type Backend struct {
	app core.App
	src Source
	ts  TSHooks
}

// NewBackend builds a Backend for one source.
func NewBackend(app core.App, src Source) *Backend {
	return &Backend{app: app, src: src}
}

type contextKey string

// userKey carries the authenticated user resolved once per request by the
// router glue, so backend methods never re-verify credentials per call.
const userKey contextKey = "caldavUser"

// errNotFound is what every unauthorized or missing resource resolves to.
//
// Returning it (rather than a permission error) is deliberate: a 403 on another
// user's calendar path confirms that path exists, so an unauthorized probe could
// enumerate calendars. Answering 404 for both is what makes the masking hold.
//
// It is wrapped in a go-webdav HTTPError so the handler actually SENDS 404 — a
// plain error becomes a 500, which leaks nothing but misreports a routine miss
// as a server fault (and buries real faults in the same bucket). errors.Is still
// matches, so IsNotFound and the hook filter keep working.
var errNotFoundCause = errors.New("not found")

var errNotFound = webdav.NewHTTPError(http.StatusNotFound, errNotFoundCause)

// ---------------------------------------------------------------------------
// Authorization
//
// CalDAV does not go through PocketBase's REST layer, so no rule is applied for
// us — but reimplementing membership and role checks in Go would mean two
// definitions of the same permission, free to drift. Instead we ask the rule
// engine the same question the REST API would, with the DAV request's
// authenticated user.
//
// This is also what makes the package safe to run inside a tenant process. A
// rule is a string and travels in the schema; a Go closure cannot cross a
// process boundary. Were authorization a Source callback, a tenant (which links
// no feature Go) would serve every calendar with no per-record checks at all.
//
// The event rules already encode the viewer/editor distinction the old
// per-feature code enforced by hand, so CanAccessRecord subsumes both the
// membership check and the "viewers may not write" check.
// ---------------------------------------------------------------------------

type ruleKind int

const (
	ruleList ruleKind = iota
	ruleView
	ruleCreate
	ruleUpdate
	ruleDelete
)

// ruleFor returns the collection's rule for the given operation. A nil *string
// means "superusers only" and an empty string means "public" — CanAccessRecord
// gives both their PocketBase meaning, so they are passed through untouched.
func ruleFor(collection *core.Collection, kind ruleKind) *string {
	switch kind {
	case ruleList:
		return collection.ListRule
	case ruleView:
		return collection.ViewRule
	case ruleCreate:
		return collection.CreateRule
	case ruleUpdate:
		return collection.UpdateRule
	case ruleDelete:
		return collection.DeleteRule
	}
	return nil
}

// canCreate evaluates the collection's CreateRule for a record that has just
// been saved inside a transaction. Split from authorize because it must run on
// the transaction's app handle — the row is not visible outside it yet.
func (b *Backend) canCreate(txApp core.App, user *core.Record, record *core.Record) (bool, error) {
	info := &core.RequestInfo{
		Auth:    user,
		Method:  "POST",
		Context: core.RequestInfoContextDefault,
	}
	ok, err := txApp.CanAccessRecord(record, info, ruleFor(record.Collection(), ruleCreate))
	if err != nil {
		// An unevaluable rule must not fail open.
		b.app.Logger().Warn("caldav: create rule evaluation failed",
			"slug", b.src.Slug, "id", record.Id, "error", err)
		return false, nil
	}
	return ok, nil
}

// authorize evaluates the record's own collection rule for this user. An
// unevaluable rule must not fail open.
func (b *Backend) authorize(user *core.Record, record *core.Record, kind ruleKind) error {
	if record == nil {
		return errNotFound
	}

	info := &core.RequestInfo{
		Auth:    user,
		Method:  "GET",
		Context: core.RequestInfoContextDefault,
	}

	ok, err := b.app.CanAccessRecord(record, info, ruleFor(record.Collection(), kind))
	if err != nil {
		b.app.Logger().Warn("caldav: access rule evaluation failed",
			"slug", b.src.Slug, "id", record.Id, "error", err)
		return errNotFound
	}
	if !ok {
		return errNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// caldav.Backend
// ---------------------------------------------------------------------------

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	if _, err := b.userFromContext(ctx); err != nil {
		return "", err
	}
	return b.src.Prefix + "/u/", nil
}

func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	if _, err := b.userFromContext(ctx); err != nil {
		return "", err
	}
	return b.src.Prefix + "/u/cal/", nil
}

// ListCalendars returns every calendar the user may list, per the calendar
// collection's ListRule.
func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return nil, err
	}

	records, err := b.app.FindRecordsByFilter(b.src.CalendarCollection, "1=1", "", 0, 0, nil)
	if err != nil {
		return nil, err
	}

	calendars := make([]caldav.Calendar, 0, len(records))
	for _, record := range records {
		if b.authorize(user, record, ruleList) != nil {
			continue
		}
		calendars = append(calendars, b.toCalendar(record))
	}
	return calendars, nil
}

func (b *Backend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return nil, err
	}

	record, err := b.calendarByPath(user, path, ruleView)
	if err != nil {
		return nil, err
	}

	cal := b.toCalendar(record)
	return &cal, nil
}

func (b *Backend) CreateCalendar(_ context.Context, _ *caldav.Calendar) error {
	// Calendars carry feature-owned setup (an owner membership row, a color, a
	// subscription URL) that a bare MKCALENDAR cannot express. Clients fall back
	// to using the calendars the web UI created.
	return fmt.Errorf("creating calendars over CalDAV is not supported")
}

func (b *Backend) ListCalendarObjects(ctx context.Context, path string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return nil, err
	}

	calRecord, err := b.calendarByPath(user, path, ruleList)
	if err != nil {
		return nil, err
	}
	calID := calRecord.Id

	// The field NAME comes from the Source (config the host materialized), not
	// from the request, so building it into the filter text is safe. Only the
	// value is a bound parameter. Same shape as webdav's listChildren.
	records, err := b.app.FindRecordsByFilter(
		b.src.EventCollection,
		b.src.Event.Calendar+" = {:calId}",
		b.src.eventSort(), 0, 0,
		map[string]any{"calId": calID},
	)
	if err != nil {
		return nil, err
	}

	authorized := make([]*core.Record, 0, len(records))
	for _, record := range records {
		if b.authorize(user, record, ruleList) != nil {
			continue
		}
		b.backfillUID(record)
		authorized = append(authorized, record)
	}

	authorized, err = b.hookFilterList(user.Id, calID, authorized)
	if err != nil {
		return nil, err
	}

	objects := make([]caldav.CalendarObject, 0, len(authorized))
	for _, record := range authorized {
		obj, err := b.toCalendarObject(record, calID)
		if err != nil {
			b.app.Logger().Debug("caldav: skipping unencodable event",
				"slug", b.src.Slug, "id", record.Id, "error", err)
			continue
		}
		objects = append(objects, *obj)
	}
	return objects, nil
}

func (b *Backend) GetCalendarObject(ctx context.Context, path string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return nil, err
	}

	record, calID, err := b.eventByPath(user, path, ruleView)
	if err != nil {
		return nil, err
	}
	if !b.hookCanRead(user.Id, calID, record) {
		return nil, errNotFound
	}
	return b.toCalendarObject(record, calID)
}

func (b *Backend) QueryCalendarObjects(ctx context.Context, path string, _ *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	// Time-range filtering is left to the client: the whole collection is
	// returned and the client narrows it. Correct, if chattier than a REPORT
	// that filtered server-side.
	return b.ListCalendarObjects(ctx, path, nil)
}

func (b *Backend) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar, _ *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return nil, err
	}

	calRecord, err := b.calendarByPath(user, path, ruleView)
	if err != nil {
		return nil, err
	}
	calID := calRecord.Id

	events := cal.Events()
	if len(events) == 0 {
		return nil, fmt.Errorf("no VEVENT found in calendar data")
	}
	icalUID, err := events[0].Props.Text(ical.PropUID)
	if err != nil || icalUID == "" {
		return nil, fmt.Errorf("VEVENT must have a UID")
	}

	existing := b.findEventByUID(calID, icalUID)

	if err := b.hookBeforeWrite(user.Id, calID, icalUID, existing == nil); err != nil {
		return nil, err
	}

	if existing != nil {
		// UpdateRule, not CreateRule: replacing an existing event is an update,
		// and it is the rule that denies a viewer.
		if err := b.authorize(user, existing, ruleUpdate); err != nil {
			return nil, err
		}
		if err := applyCalendarToRecord(cal, existing, b.src.Event); err != nil {
			return nil, err
		}
		if err := b.app.Save(existing); err != nil {
			return nil, err
		}
		return b.toCalendarObject(existing, calID)
	}

	collection, err := b.app.FindCollectionByNameOrId(b.src.EventCollection)
	if err != nil {
		return nil, err
	}

	record := core.NewRecord(collection)
	record.Set(b.src.Event.Calendar, calID)
	if b.src.Event.Owner != "" {
		record.Set(b.src.Event.Owner, user.Id)
	}
	record.Set(b.src.Event.UID, icalUID)

	// Schema-required fields iCalendar may omit, before the codec so anything
	// the VEVENT does carry overrides them.
	for field, value := range b.src.Event.Defaults {
		record.Set(field, value)
	}

	if err := applyCalendarToRecord(cal, record, b.src.Event); err != nil {
		return nil, err
	}

	// Authorizing a CREATE has to happen with the row present.
	//
	// CanAccessRecord evaluates the rule as a query filtered to `id = record.Id`,
	// so an unsaved record matches nothing and every create rule denies — and a
	// create rule that traverses a relation (as calendar_events' does, through
	// its calendar's memberships) could not be evaluated any other way. So save
	// inside a transaction, ask the rule, and roll back if it says no. This is
	// what PocketBase's own record-create API does.
	if err := b.app.RunInTransaction(func(txApp core.App) error {
		if err := txApp.Save(record); err != nil {
			return err
		}
		ok, err := b.canCreate(txApp, user, record)
		if err != nil {
			return err
		}
		if !ok {
			return errNotFound
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return b.toCalendarObject(record, calID)
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, path string) error {
	user, err := b.userFromContext(ctx)
	if err != nil {
		return err
	}

	record, calID, err := b.eventByPath(user, path, ruleDelete)
	if err != nil {
		return err
	}
	if err := b.hookBeforeDelete(user.Id, calID, record); err != nil {
		return err
	}
	return b.app.Delete(record)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (b *Backend) userFromContext(ctx context.Context) (*core.Record, error) {
	user, ok := ctx.Value(userKey).(*core.Record)
	if !ok || user == nil {
		return nil, errNotFound
	}
	return user, nil
}

func (b *Backend) toCalendar(record *core.Record) caldav.Calendar {
	description := ""
	if b.src.Calendar.Description != "" {
		description = record.GetString(b.src.Calendar.Description)
	}
	return caldav.Calendar{
		Path:                  b.calendarPath(record.Id),
		Name:                  record.GetString(b.src.Calendar.Name),
		Description:           description,
		SupportedComponentSet: []string{ical.CompEvent},
	}
}

func (b *Backend) toCalendarObject(record *core.Record, calID string) (*caldav.CalendarObject, error) {
	cal, err := recordToCalendar(record, b.src.Event)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}

	updated := ""
	if b.src.Event.Updated != "" {
		updated = record.GetString(b.src.Event.Updated)
	}
	modTime, _ := parsePBTime(updated)

	return &caldav.CalendarObject{
		Path:          b.calendarPath(calID) + record.GetString(b.src.Event.UID) + ".ics",
		ModTime:       modTime,
		ContentLength: int64(buf.Len()),
		ETag:          fmt.Sprintf(`"%s"`, updated),
		Data:          cal,
	}, nil
}

// calendarByPath resolves the calendar id in the path and authorizes it under
// the given rule. Every failure returns errNotFound so an unauthorized path is
// indistinguishable from a missing one.
func (b *Backend) calendarByPath(user *core.Record, path string, kind ruleKind) (*core.Record, error) {
	calID := extractCalendarID(path)
	if calID == "" {
		return nil, errNotFound
	}
	record, err := b.app.FindRecordById(b.src.CalendarCollection, calID)
	if err != nil || record == nil {
		return nil, errNotFound
	}
	if err := b.authorize(user, record, kind); err != nil {
		return nil, err
	}
	return record, nil
}

// eventByPath resolves an event object path to its record, authorizing both the
// containing calendar (view) and the event itself (under kind).
func (b *Backend) eventByPath(user *core.Record, path string, kind ruleKind) (*core.Record, string, error) {
	calRecord, err := b.calendarByPath(user, path, ruleView)
	if err != nil {
		return nil, "", err
	}

	icalUID := extractICalUID(path)
	if icalUID == "" {
		return nil, "", errNotFound
	}

	record := b.findEventByUID(calRecord.Id, icalUID)
	if record == nil {
		return nil, "", errNotFound
	}
	if err := b.authorize(user, record, kind); err != nil {
		return nil, "", err
	}
	return record, calRecord.Id, nil
}

// findEventByUID looks up one event by its iCalendar UID within a calendar.
// Returns nil when absent — callers distinguish "create" from "update" by it.
func (b *Backend) findEventByUID(calID, icalUID string) *core.Record {
	records, err := b.app.FindRecordsByFilter(
		b.src.EventCollection,
		b.src.Event.UID+" = {:uid} && "+b.src.Event.Calendar+" = {:calId}",
		"", 1, 0,
		map[string]any{"uid": icalUID, "calId": calID},
	)
	if err != nil || len(records) == 0 {
		return nil
	}
	return records[0]
}

// backfillUID stamps a UID on an event that predates CalDAV support, so a
// client gets a stable object path for it. A failure here is not fatal: the
// event is simply skipped by the caller when it cannot be encoded.
func (b *Backend) backfillUID(record *core.Record) {
	if record.GetString(b.src.Event.UID) != "" {
		return
	}
	record.Set(b.src.Event.UID, "urn:uuid:"+uuid.NewString())
	if err := b.app.Save(record); err != nil {
		b.app.Logger().Warn("caldav: failed to backfill event UID",
			"slug", b.src.Slug, "id", record.Id, "error", err)
	}
}

func (b *Backend) calendarPath(calID string) string {
	return b.src.Prefix + "/u/cal/" + calID + "/"
}

// extractCalendarID reads {calendarId} out of <prefix>/u/cal/{calendarId}/...
func extractCalendarID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// parts: <prefix> / u / cal / {calendarId} / ...
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

// extractICalUID reads {uid} out of <prefix>/u/cal/{calendarId}/{uid}.ics
func extractICalUID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 {
		return strings.TrimSuffix(parts[4], ".ics")
	}
	return ""
}

// withUser returns a context carrying the authenticated user, for the router
// glue to install once per request.
func withUser(ctx context.Context, user *core.Record) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// compile-time assertion that Backend satisfies the protocol interface.
var _ caldav.Backend = (*Backend)(nil)
