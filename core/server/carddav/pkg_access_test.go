package carddav

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// pkg_access_test.go pins that a readonly org_pkg_access grant binds CardDAV
// writes — same rationale as CalDAV's twin: the protocol bypasses the REST
// layer where the request-hook guard lives.

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

func TestPutAddressObject_ReadonlyGrantRefused(t *testing.T) {
	app, backend, user := setupPutAuthzApp(t)
	setContactRules(t, app, `owner = @request.auth.id`, `owner = @request.auth.id`)
	giveReadonly(t, app, user, "contacts")
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:ro.vcf",
		newCard("urn:uuid:ro", "Refused"), nil)
	if err == nil {
		t.Fatal("readonly user's PUT succeeded")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("err = %v, want a read-only refusal", err)
	}
}

func TestDeleteAddressObject_ReadonlyGrantRefused(t *testing.T) {
	app, backend, user := setupPutAuthzApp(t)
	setContactRules(t, app, `owner = @request.auth.id`, `owner = @request.auth.id`)
	rec := seedContact(t, app, user.Id, "urn:uuid:keep", "Kept")
	giveReadonly(t, app, user, "contacts")
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	if err := backend.DeleteAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:keep.vcf"); err == nil {
		t.Fatal("readonly user's DELETE succeeded")
	}
	if _, err := app.FindRecordById("contacts", rec.Id); err != nil {
		t.Fatal("the refused DELETE removed the contact anyway")
	}
}
