package caldav

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// pkg_access_test.go pins that a readonly org_pkg_access grant binds CalDAV
// writes: the protocol bypasses the REST layer, so without its own check a
// readonly user's calendar client could still create and delete events.

// giveReadonly grants user a readonly override for the backend's package.
func giveReadonly(t *testing.T, app *tests.TestApp, user *core.Record, slug string) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	access := core.NewBaseCollection("org_pkg_access")
	access.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	access.Fields.Add(&core.TextField{Name: "pkg", Required: true})
	access.Fields.Add(&core.SelectField{
		Name: "access", Required: true, Values: []string{"full", "readonly", "none"}, MaxSelect: 1,
	})
	if err := app.Save(access); err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(access)
	row.Set("user", user.Id)
	row.Set("pkg", slug)
	row.Set("access", "readonly")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
}

func TestPutCalendarObject_ReadonlyGrantRefused(t *testing.T) {
	env := setupAuthzEnv(t)
	giveReadonly(t, env.app, env.owner, "calendar")

	cal, err := recordToCalendar(env.event, testEventMap)
	if err != nil {
		t.Fatal(err)
	}
	cal.Events()[0].Props.SetText("SUMMARY", "readonly write")

	_, err = env.backend.PutCalendarObject(env.ctx(env.owner), env.eventPath(), cal, nil)
	if err == nil {
		t.Fatal("readonly user's PUT succeeded")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("err = %v, want a read-only refusal", err)
	}

	reloaded, _ := env.app.FindRecordById("calendar_events", env.event.Id)
	if got := reloaded.GetString("title"); got != "Kickoff" {
		t.Errorf("title = %q; the refused PUT was written anyway", got)
	}
}

func TestDeleteCalendarObject_ReadonlyGrantRefused(t *testing.T) {
	env := setupAuthzEnv(t)
	giveReadonly(t, env.app, env.owner, "calendar")

	if err := env.backend.DeleteCalendarObject(env.ctx(env.owner), env.eventPath()); err == nil {
		t.Fatal("readonly user's DELETE succeeded")
	}
	if _, err := env.app.FindRecordById("calendar_events", env.event.Id); err != nil {
		t.Fatal("the refused DELETE removed the event anyway")
	}
}

// Reads stay available: readonly means read.
func TestGetCalendarObject_ReadonlyGrantStillReads(t *testing.T) {
	env := setupAuthzEnv(t)
	giveReadonly(t, env.app, env.owner, "calendar")

	obj, err := env.backend.GetCalendarObject(env.ctx(env.owner), env.eventPath(), nil)
	if err != nil {
		t.Fatalf("readonly user should still read: %v", err)
	}
	if obj == nil {
		t.Fatal("no object returned")
	}
}
