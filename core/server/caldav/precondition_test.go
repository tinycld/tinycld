package caldav

import (
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
)

// precondition_test.go pins If-Match / If-None-Match enforcement on PUT.
// go-webdav parses the headers and hands them to the backend (PutOptions);
// a backend that ignores them turns every concurrent edit into silent
// last-writer-wins data loss — no 412, so clients never re-fetch first.

func wantPrecondFailed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("PUT succeeded, want 412 Precondition Failed")
	}
	if !strings.Contains(err.Error(), "412") {
		t.Fatalf("err = %v, want a 412 Precondition Failed", err)
	}
}

// If-None-Match: * is "create only" — a client that believes the resource does
// not exist yet. When it does, the PUT must fail rather than overwrite.
func TestPutCalendarObject_IfNoneMatchRefusesOverwrite(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatal(err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "should never land")

	_, err = env.backend.PutCalendarObject(env.ctx(env.owner), env.eventPath(), cal,
		&caldav.PutCalendarObjectOptions{IfNoneMatch: "*"})
	wantPrecondFailed(t, err)

	reloaded, _ := env.app.FindRecordById("calendar_events", env.event.Id)
	if got := reloaded.GetString("title"); got != "Kickoff" {
		t.Errorf("title = %q; the refused PUT was written anyway", got)
	}
}

// If-Match with a stale ETag is the lost-update guard: the client's copy is
// outdated, so its overwrite must be refused.
func TestPutCalendarObject_IfMatchStaleRefused(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatal(err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "based on a stale copy")

	_, err = env.backend.PutCalendarObject(env.ctx(env.owner), env.eventPath(), cal,
		&caldav.PutCalendarObjectOptions{IfMatch: `"2001-01-01 00:00:00.000Z"`})
	wantPrecondFailed(t, err)

	reloaded, _ := env.app.FindRecordById("calendar_events", env.event.Id)
	if got := reloaded.GetString("title"); got != "Kickoff" {
		t.Errorf("title = %q; the refused PUT was written anyway", got)
	}
}

func TestPutCalendarObject_IfMatchCurrentSucceeds(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatal(err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "conditional update")

	etag := fmt.Sprintf("%q", env.event.GetString("updated"))
	if _, err := env.backend.PutCalendarObject(env.ctx(env.owner), env.eventPath(), cal,
		&caldav.PutCalendarObjectOptions{IfMatch: webdav.ConditionalMatch(etag)}); err != nil {
		t.Fatalf("PUT with the current ETag should succeed, got %v", err)
	}

	reloaded, _ := env.app.FindRecordById("calendar_events", env.event.Id)
	if got := reloaded.GetString("title"); got != "conditional update" {
		t.Errorf("title = %q, want the conditional update applied", got)
	}
}

// If-Match against a resource that does not exist must 412 (RFC 9110 §13.1.1),
// not create it.
func TestPutCalendarObject_IfMatchOnMissingRefused(t *testing.T) {
	env := setupAuthzEnv(t)

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatal(err)
	}
	cal.Events()[0].Props.SetText("UID", "urn:uuid:never-created")

	_, err = env.backend.PutCalendarObject(env.ctx(env.owner),
		env.calPath()+"urn:uuid:never-created.ics", cal,
		&caldav.PutCalendarObjectOptions{IfMatch: `"whatever"`})
	wantPrecondFailed(t, err)
}
