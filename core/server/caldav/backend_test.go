package caldav

import (
	"context"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// The authorization tests.
//
// These are the ones that matter. The lifted backend has NO Go permission
// callbacks: it asks the collections' own PocketBase rules via CanAccessRecord,
// which is what lets a tenant process (linking no feature package) enforce exactly
// what the single-tenant app does. So the property under test is not "the Go
// checks membership" but "the shipped rule is what decides", including the
// viewer/editor split the old requireEditorRole enforced by hand.
//
// Every deny case asserts a not-found sentinel rather than a permission error:
// a 403 on another user's calendar path would confirm that path exists.

// testSource is the field mapping the calendar package ships.
var testSource = Source{
	Slug:               "calendar",
	Prefix:             "/caldav",
	CalendarCollection: "calendar_calendars",
	EventCollection:    "calendar_events",
	Calendar:           CalendarMap{Name: "name", Description: "description"},
	Event:              testEventMap,
}

// authzEnv is a live app with the calendar schema, its shipped rules, three
// users (owner / viewer / outsider), one calendar and one event.
type authzEnv struct {
	app      *tests.TestApp
	backend  *Backend
	owner    *core.Record
	viewer   *core.Record
	outsider *core.Record
	calendar *core.Record
	event    *core.Record
}

// The rules below are the ones the calendar package's migrations ship
// (1715000000 phase 2, as amended by 1715200000 / 1830000003). They are stated
// here so a drift between this fixture and the migration shows up as a failing
// test rather than a test that validates its own invention — and every one is
// exercised by a deny case below, so neutering it goes red.
const (
	calMemberRule            = "calendar_members_via_calendar.user ?= @request.auth.id"
	calMemberViaCalendarRule = "calendar.calendar_members_via_calendar.user ?= @request.auth.id"
	calEditorViaCalendarRule = "calendar.calendar_members_via_calendar.user ?= @request.auth.id && " +
		"calendar.calendar_members_via_calendar.role ?!= \"viewer\""
)

func setupAuthzEnv(t *testing.T) *authzEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}

	newUser := func(email string) *core.Record {
		r := core.NewRecord(users)
		r.Set("email", email)
		r.Set("password", "password123")
		if err := app.Save(r); err != nil {
			t.Fatalf("save user %s: %v", email, err)
		}
		return r
	}
	owner := newUser("owner@example.com")
	viewer := newUser("viewer@example.com")
	outsider := newUser("outsider@example.com")

	// calendar_calendars
	calendars := core.NewBaseCollection("calendar_calendars")
	calendars.Fields.Add(
		&core.TextField{Name: "name", Required: true},
		&core.TextField{Name: "description"},
	)
	if err := app.Save(calendars); err != nil {
		t.Fatalf("save calendar_calendars: %v", err)
	}

	// calendar_members
	members := core.NewBaseCollection("calendar_members")
	members.Fields.Add(
		&core.RelationField{Name: "calendar", Required: true, CollectionId: calendars.Id, MaxSelect: 1},
		&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1},
		&core.SelectField{Name: "role", Required: true, Values: []string{"owner", "editor", "viewer"}, MaxSelect: 1},
	)
	if err := app.Save(members); err != nil {
		t.Fatalf("save calendar_members: %v", err)
	}

	// calendar_events
	events := core.NewBaseCollection("calendar_events")
	events.Fields.Add(
		&core.RelationField{Name: "calendar", Required: true, CollectionId: calendars.Id, MaxSelect: 1},
		&core.RelationField{Name: "created_by", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "ical_uid"},
		&core.TextField{Name: "title"},
		&core.TextField{Name: "description"},
		&core.TextField{Name: "location"},
		&core.TextField{Name: "start"},
		&core.TextField{Name: "end"},
		&core.BoolField{Name: "all_day"},
		&core.TextField{Name: "recurrence"},
		&core.JSONField{Name: "guests", MaxSize: 100000},
		&core.NumberField{Name: "reminder"},
		// Required selects with NO default, exactly as the migration ships them.
		// This is what makes the minimal-VEVENT test below meaningful.
		&core.SelectField{Name: "busy_status", Required: true, MaxSelect: 1,
			Values: []string{"busy", "free"}},
		&core.SelectField{Name: "visibility", Required: true, MaxSelect: 1,
			Values: []string{"default", "public", "private"}},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	if err := app.Save(events); err != nil {
		t.Fatalf("save calendar_events: %v", err)
	}

	// Rules applied after all three exist, so the back-relations resolve.
	calendars.ListRule = ptr(calMemberRule)
	calendars.ViewRule = ptr(calMemberRule)
	if err := app.Save(calendars); err != nil {
		t.Fatalf("apply calendar rules: %v", err)
	}

	events.ListRule = ptr(calMemberViaCalendarRule)
	events.ViewRule = ptr(calMemberViaCalendarRule)
	events.CreateRule = ptr(calEditorViaCalendarRule)
	events.UpdateRule = ptr(calEditorViaCalendarRule)
	events.DeleteRule = ptr(calEditorViaCalendarRule)
	if err := app.Save(events); err != nil {
		t.Fatalf("apply event rules: %v", err)
	}

	calendar := core.NewRecord(calendars)
	calendar.Set("name", "Team")
	if err := app.Save(calendar); err != nil {
		t.Fatalf("save calendar: %v", err)
	}

	addMember := func(user *core.Record, role string) {
		m := core.NewRecord(members)
		m.Set("calendar", calendar.Id)
		m.Set("user", user.Id)
		m.Set("role", role)
		if err := app.Save(m); err != nil {
			t.Fatalf("save membership %s: %v", role, err)
		}
	}
	addMember(owner, "owner")
	addMember(viewer, "viewer")

	event := core.NewRecord(events)
	event.Set("calendar", calendar.Id)
	event.Set("created_by", owner.Id)
	event.Set("ical_uid", "urn:uuid:evt-1")
	event.Set("busy_status", "busy")
	event.Set("visibility", "default")
	event.Set("title", "Kickoff")
	event.Set("start", "2026-07-26 09:00:00.000Z")
	event.Set("end", "2026-07-26 10:00:00.000Z")
	if err := app.Save(event); err != nil {
		t.Fatalf("save event: %v", err)
	}

	return &authzEnv{
		app: app, backend: NewBackend(app, testSource),
		owner: owner, viewer: viewer, outsider: outsider,
		calendar: calendar, event: event,
	}
}

func ptr(s string) *string { return &s }

func (e *authzEnv) ctx(user *core.Record) context.Context {
	return withUser(context.Background(), user)
}

func (e *authzEnv) calPath() string {
	return "/caldav/u/cal/" + e.calendar.Id + "/"
}

func (e *authzEnv) eventPath() string {
	return e.calPath() + "urn:uuid:evt-1.ics"
}

// --- listing ---------------------------------------------------------------

func TestListCalendars_MemberSeesOwn(t *testing.T) {
	env := setupAuthzEnv(t)
	cals, err := env.backend.ListCalendars(env.ctx(env.owner))
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(cals) != 1 {
		t.Fatalf("owner saw %d calendars, want 1", len(cals))
	}
	if cals[0].Name != "Team" {
		t.Errorf("calendar name = %q, want Team", cals[0].Name)
	}
}

// The deny case for calMemberRule. Neuter that rule and this goes red.
func TestListCalendars_OutsiderSeesNothing(t *testing.T) {
	env := setupAuthzEnv(t)
	cals, err := env.backend.ListCalendars(env.ctx(env.outsider))
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(cals) != 0 {
		t.Fatalf("outsider saw %d calendars, want 0", len(cals))
	}
}

func TestGetCalendar_OutsiderGetsNotFound(t *testing.T) {
	env := setupAuthzEnv(t)
	_, err := env.backend.GetCalendar(env.ctx(env.outsider), env.calPath())
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want the not-found sentinel (a 403 would confirm the path exists)", err)
	}
}

func TestListCalendarObjects_MemberSeesEvent(t *testing.T) {
	env := setupAuthzEnv(t)
	objs, err := env.backend.ListCalendarObjects(env.ctx(env.viewer), env.calPath(), nil)
	if err != nil {
		t.Fatalf("ListCalendarObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("viewer saw %d objects, want 1", len(objs))
	}
}

func TestListCalendarObjects_OutsiderGetsNotFound(t *testing.T) {
	env := setupAuthzEnv(t)
	_, err := env.backend.ListCalendarObjects(env.ctx(env.outsider), env.calPath(), nil)
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want the not-found sentinel", err)
	}
}

// --- the viewer/editor split ----------------------------------------------
//
// This is what the deleted requireEditorRole used to enforce. It now comes from
// calEditorViaCalendarRule, so the test proves the rule replaced the Go check
// rather than the check simply being dropped.

func TestPutCalendarObject_ViewerDenied(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "Viewer tried to rename this")

	_, err = env.backend.PutCalendarObject(env.ctx(env.viewer), env.eventPath(), cal, nil)
	if err == nil {
		t.Fatal("a viewer's PUT succeeded; the editor rule is not being enforced")
	}

	reloaded, err := env.app.FindRecordById("calendar_events", env.event.Id)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got := reloaded.GetString("title"); got != "Kickoff" {
		t.Errorf("title = %q, want the write to have been refused", got)
	}
}

func TestPutCalendarObject_OwnerCanUpdate(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "Renamed by owner")

	if _, err := env.backend.PutCalendarObject(env.ctx(env.owner), env.eventPath(), cal, nil); err != nil {
		t.Fatalf("owner PUT: %v", err)
	}

	reloaded, err := env.app.FindRecordById("calendar_events", env.event.Id)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got := reloaded.GetString("title"); got != "Renamed by owner" {
		t.Errorf("title = %q, want the owner's update to land", got)
	}
}

func TestDeleteCalendarObject_ViewerDenied(t *testing.T) {
	env := setupAuthzEnv(t)

	if err := env.backend.DeleteCalendarObject(env.ctx(env.viewer), env.eventPath()); err == nil {
		t.Fatal("a viewer's DELETE succeeded; the editor rule is not being enforced")
	}
	if _, err := env.app.FindRecordById("calendar_events", env.event.Id); err != nil {
		t.Errorf("event was deleted by a viewer: %v", err)
	}
}

func TestDeleteCalendarObject_OwnerCanDelete(t *testing.T) {
	env := setupAuthzEnv(t)

	if err := env.backend.DeleteCalendarObject(env.ctx(env.owner), env.eventPath()); err != nil {
		t.Fatalf("owner DELETE: %v", err)
	}
	if _, err := env.app.FindRecordById("calendar_events", env.event.Id); err == nil {
		t.Error("event still present after the owner deleted it")
	}
}

func TestGetCalendarObject_OutsiderGetsNotFound(t *testing.T) {
	env := setupAuthzEnv(t)
	_, err := env.backend.GetCalendarObject(env.ctx(env.outsider), env.eventPath(), nil)
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want the not-found sentinel", err)
	}
}

// --- unauthenticated -------------------------------------------------------

func TestNoUserInContext_IsNotFound(t *testing.T) {
	env := setupAuthzEnv(t)
	bare := context.Background()

	if _, err := env.backend.CurrentUserPrincipal(bare); !IsNotFound(err) {
		t.Errorf("CurrentUserPrincipal err = %v, want not-found", err)
	}
	if _, err := env.backend.ListCalendars(bare); !IsNotFound(err) {
		t.Errorf("ListCalendars err = %v, want not-found", err)
	}
	if _, err := env.backend.GetCalendar(bare, env.calPath()); !IsNotFound(err) {
		t.Errorf("GetCalendar err = %v, want not-found", err)
	}
}

// --- paths -----------------------------------------------------------------

func TestPrincipalAndHomeSetPaths(t *testing.T) {
	env := setupAuthzEnv(t)

	principal, err := env.backend.CurrentUserPrincipal(env.ctx(env.owner))
	if err != nil {
		t.Fatalf("CurrentUserPrincipal: %v", err)
	}
	if principal != "/caldav/u/" {
		t.Errorf("principal = %q", principal)
	}

	home, err := env.backend.CalendarHomeSetPath(env.ctx(env.owner))
	if err != nil {
		t.Fatalf("CalendarHomeSetPath: %v", err)
	}
	if home != "/caldav/u/cal/" {
		t.Errorf("home set = %q", home)
	}
}

func TestCreateCalendarUnsupported(t *testing.T) {
	env := setupAuthzEnv(t)
	if err := env.backend.CreateCalendar(env.ctx(env.owner), nil); err == nil {
		t.Fatal("CreateCalendar should report that MKCALENDAR is unsupported")
	}
}

func TestHasPrefixAndPrefixes(t *testing.T) {
	sources := []Source{testSource}

	for _, p := range []string{"/caldav", "/caldav/u/cal/x/", "/.well-known/caldav"} {
		if !HasPrefix(sources, p) {
			t.Errorf("HasPrefix(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/carddav", "/api/collections", "/caldavx"} {
		if HasPrefix(sources, p) {
			t.Errorf("HasPrefix(%q) = true, want false", p)
		}
	}

	if HasPrefix(nil, "/.well-known/caldav") {
		t.Error("the well-known alias must not match when no sources are registered")
	}

	got := Prefixes(sources)
	if len(got) != 2 || got[0] != "/.well-known/caldav" || got[1] != "/caldav" {
		t.Errorf("Prefixes = %v", got)
	}
	if Prefixes(nil) != nil && len(Prefixes(nil)) != 0 {
		t.Errorf("Prefixes(nil) = %v, want empty", Prefixes(nil))
	}
}

// --- create --------------------------------------------------------------
//
// A create is the one authorization case CanAccessRecord cannot answer on an
// unsaved record: it evaluates the rule as a query filtered to `id = record.Id`,
// so a row that does not exist yet matches nothing and EVERY create rule denies.
// PutCalendarObject therefore saves inside a transaction and asks the rule with
// the row present, rolling back on refusal.
//
// The first live PUT against a real server returned 500 "not found" for exactly
// this reason — invisible to a test that only exercised updates.

func TestPutCalendarObject_OwnerCanCreate(t *testing.T) {
	env := setupAuthzEnv(t)

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:brand-new")
	event.Props.SetText(ical.PropSummary, "Created over CalDAV")
	event.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC))
	event.Props.SetDateTime(ical.PropDateTimeEnd, time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, event.Component)

	obj, err := env.backend.PutCalendarObject(
		env.ctx(env.owner), env.calPath()+"urn:uuid:brand-new.ics", cal, nil)
	if err != nil {
		t.Fatalf("owner create over CalDAV: %v", err)
	}
	if obj == nil {
		t.Fatal("PutCalendarObject returned no object")
	}

	created := env.backend.findEventByUID(env.calendar.Id, "urn:uuid:brand-new")
	if created == nil {
		t.Fatal("the created event is not in the collection")
	}
	if got := created.GetString("title"); got != "Created over CalDAV" {
		t.Errorf("title = %q", got)
	}
	if got := created.GetString("created_by"); got != env.owner.Id {
		t.Errorf("created_by = %q, want the authenticated user %q", got, env.owner.Id)
	}
}

// A viewer must not be able to create either — and the refusal must leave no
// row behind, since the record is saved before the rule is consulted.
func TestPutCalendarObject_ViewerCannotCreate(t *testing.T) {
	env := setupAuthzEnv(t)

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:viewer-made")
	event.Props.SetText(ical.PropSummary, "Viewer tried to create this")
	event.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC))
	event.Props.SetDateTime(ical.PropDateTimeEnd, time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, event.Component)

	_, err := env.backend.PutCalendarObject(
		env.ctx(env.viewer), env.calPath()+"urn:uuid:viewer-made.ics", cal, nil)
	if err == nil {
		t.Fatal("a viewer created an event over CalDAV; the create rule is not enforced")
	}

	// The transaction must have rolled back — a denied create leaves nothing.
	if leaked := env.backend.findEventByUID(env.calendar.Id, "urn:uuid:viewer-made"); leaked != nil {
		t.Error("the refused create left a row behind; the rollback did not happen")
	}
}

func TestPutCalendarObject_OutsiderGetsNotFound(t *testing.T) {
	env := setupAuthzEnv(t)

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:outsider")
	event.Props.SetText(ical.PropSummary, "Outsider")
	cal.Children = append(cal.Children, event.Component)

	_, err := env.backend.PutCalendarObject(
		env.ctx(env.outsider), env.calPath()+"urn:uuid:outsider.ics", cal, nil)
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want the not-found sentinel", err)
	}
}

// A create must round-trip the recurrence rule — the property a flat field map
// could not express, and the reason the codec is Go.
func TestPutCalendarObject_CreatePreservesRecurrence(t *testing.T) {
	env := setupAuthzEnv(t)

	rrule := "FREQ=WEEKLY;BYDAY=TU;UNTIL=20261231T235959Z"
	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:recurring")
	event.Props.SetText(ical.PropSummary, "Weekly")
	// Set the RECUR value the way a decoded client PUT carries it. SetText would
	// escape the separators (`FREQ=WEEKLY\;BYDAY=TU`), which is not what a real
	// request looks like — see the wire-format tests in ical_codec_test.go.
	rprop := ical.NewProp(ical.PropRecurrenceRule)
	rprop.Value = rrule
	event.Props.Set(rprop)
	event.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC))
	event.Props.SetDateTime(ical.PropDateTimeEnd, time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, event.Component)

	if _, err := env.backend.PutCalendarObject(
		env.ctx(env.owner), env.calPath()+"urn:uuid:recurring.ics", cal, nil); err != nil {
		t.Fatalf("create with RRULE: %v", err)
	}

	created := env.backend.findEventByUID(env.calendar.Id, "urn:uuid:recurring")
	if created == nil {
		t.Fatal("the recurring event was not created")
	}
	if got := created.GetString("recurrence"); got != rrule {
		t.Errorf("recurrence = %q, want the RRULE preserved verbatim (%q)", got, rrule)
	}
}

// A client may PUT a VEVENT carrying only the bare minimum. busy_status and
// visibility are required selects with no schema default, so without
// Source.Event.Defaults the save is rejected with "cannot be blank" — which is
// exactly what a live PUT returned before Defaults existed.
//
// Defaults are data, not a Go callback, precisely so a tenant process (which
// links no feature package) gets them too.
func TestPutCalendarObject_MinimalVEventUsesDefaults(t *testing.T) {
	env := setupAuthzEnv(t)

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:minimal")
	event.Props.SetText(ical.PropSummary, "Bare minimum")
	event.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC))
	event.Props.SetDateTime(ical.PropDateTimeEnd, time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, event.Component)

	if _, err := env.backend.PutCalendarObject(
		env.ctx(env.owner), env.calPath()+"urn:uuid:minimal.ics", cal, nil); err != nil {
		t.Fatalf("minimal VEVENT PUT: %v", err)
	}

	created := env.backend.findEventByUID(env.calendar.Id, "urn:uuid:minimal")
	if created == nil {
		t.Fatal("the minimal event was not created")
	}
	if got := created.GetString("busy_status"); got != "busy" {
		t.Errorf("busy_status = %q, want the default 'busy'", got)
	}
	if got := created.GetString("visibility"); got != "default" {
		t.Errorf("visibility = %q, want the default 'default'", got)
	}
}

// A default must never override what the VEVENT actually carries.
func TestPutCalendarObject_VEventOverridesDefaults(t *testing.T) {
	env := setupAuthzEnv(t)

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:explicit")
	event.Props.SetText(ical.PropSummary, "Explicitly free and private")
	event.Props.SetText(ical.PropTransparency, "TRANSPARENT")
	event.Props.SetText(ical.PropClass, "PRIVATE")
	event.Props.SetDateTime(ical.PropDateTimeStart, time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC))
	event.Props.SetDateTime(ical.PropDateTimeEnd, time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, event.Component)

	if _, err := env.backend.PutCalendarObject(
		env.ctx(env.owner), env.calPath()+"urn:uuid:explicit.ics", cal, nil); err != nil {
		t.Fatalf("PUT: %v", err)
	}

	created := env.backend.findEventByUID(env.calendar.Id, "urn:uuid:explicit")
	if got := created.GetString("busy_status"); got != "free" {
		t.Errorf("busy_status = %q, want 'free' from TRANSP:TRANSPARENT", got)
	}
	if got := created.GetString("visibility"); got != "private" {
		t.Errorf("visibility = %q, want 'private' from CLASS:PRIVATE", got)
	}
}
