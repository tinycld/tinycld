package carddav

import (
	"context"
	"net/http"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// PutAddressObject evaluates no collection rule on either branch — it saves a
// new record, or applies the vCard to an existing one and saves, and asks
// nothing.
//
// Today that is self-consistent rather than exploitable: the collection is
// owner-scoped, resolveObjectByPath filters to the caller's own book, and the
// owner field is stamped from the authenticated user. The hole is latent. The
// moment contacts' migration adds a clause the Go does not mirror — a disabled
// check, a guest exclusion, a read-only role — CardDAV keeps writing straight
// past it, exactly as WebDAV's create path did before P0-3, and for the same
// reason: the write path never asks the rule engine.
//
// These tests pin the property, so the rule engine is what decides rather than
// a Go path that happens to agree with it.

func setupPutAuthzApp(t *testing.T) (*tests.TestApp, *Backend, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	user := core.NewRecord(users)
	user.Set("email", "alice@example.com")
	user.Set("password", "Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	contacts := core.NewBaseCollection("contacts")
	contacts.Fields.Add(&core.TextField{Name: "uid", Required: true})
	contacts.Fields.Add(&core.TextField{Name: "full_name"})
	contacts.Fields.Add(&core.RelationField{
		Name: "owner", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(contacts); err != nil {
		t.Fatal(err)
	}

	src := Source{
		Slug:       "contacts",
		Collection: "contacts",
		UIDField:   "uid",
		OwnerField: "owner",
		VCard:      VCardMap{Version: "4.0", Name: NameMap{Given: "full_name"}},
	}
	backend := &Backend{app: app, sources: []Source{src}, scope: singleOrgScope{bookSegment: "default"}}
	return app, backend, user
}

// setContactRules installs the collection rules under test.
func setContactRules(t *testing.T, app *tests.TestApp, create, update string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	own := "owner = @request.auth.id"
	col.ListRule = &own
	col.ViewRule = &own
	col.CreateRule = ptr(create)
	col.UpdateRule = ptr(update)
	col.DeleteRule = &own
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
}

// authedCtx builds the context the backend reads credentials from.
func authedCtx(t *testing.T, email, password string) context.Context {
	t.Helper()
	r, err := http.NewRequest(http.MethodPut, "/carddav/u/ab/default/x.vcf", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.SetBasicAuth(email, password)
	return context.WithValue(context.Background(), httpRequestKey, r)
}

func newCard(uid, name string) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, name)
	return card
}

// A create the collection's CreateRule forbids must not be written.
func TestPutAddressObject_CreateDeniedByCreateRule(t *testing.T) {
	app, backend, _ := setupPutAuthzApp(t)
	// A rule nobody satisfies. A create path that consults it fails; one that
	// consults nothing sails through.
	setContactRules(t, app, `owner = "nobody"`, `owner = @request.auth.id`)

	ctx := authedCtx(t, "alice@example.com", "Password123!")
	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:new.vcf",
		newCard("urn:uuid:new", "Blocked"), nil)
	if err == nil {
		t.Fatal("a create the CreateRule forbids was accepted")
	}

	found, _ := app.FindRecordsByFilter("contacts", "uid = {:u}", "", 0, 0,
		map[string]any{"u": "urn:uuid:new"})
	if len(found) != 0 {
		t.Fatal("a denied create left a record behind")
	}
}

// An update the collection's UpdateRule forbids must not be applied.
func TestPutAddressObject_UpdateDeniedByUpdateRule(t *testing.T) {
	app, backend, user := setupPutAuthzApp(t)
	setContactRules(t, app, `@request.auth.id != ""`, `@request.auth.id != ""`)

	// Seed a contact the user owns.
	col, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("uid", "urn:uuid:existing")
	rec.Set("full_name", "Original")
	rec.Set("owner", user.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	// Now forbid updates and try to overwrite it.
	setContactRules(t, app, `@request.auth.id != ""`, `owner = "nobody"`)

	ctx := authedCtx(t, "alice@example.com", "Password123!")
	_, err = backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:existing.vcf",
		newCard("urn:uuid:existing", "Overwritten"), nil)
	if err == nil {
		t.Fatal("an update the UpdateRule forbids was accepted")
	}

	fresh, err := app.FindRecordById("contacts", rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetString("full_name") != "Original" {
		t.Fatalf("the record was modified despite the rule: %q", fresh.GetString("full_name"))
	}
}

// The positive control: with rules the caller satisfies, both branches still
// work. Without it, refusing everything would pass the two tests above — and
// CardDAV silently refusing every write is a worse outcome than the latent
// hole it replaces.
func TestPutAddressObject_AllowedWritesStillSucceed(t *testing.T) {
	app, backend, user := setupPutAuthzApp(t)
	own := `owner = @request.auth.id`
	setContactRules(t, app, `@request.auth.id != ""`, own)

	ctx := authedCtx(t, "alice@example.com", "Password123!")

	if _, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:ok.vcf",
		newCard("urn:uuid:ok", "Created"), nil); err != nil {
		t.Fatalf("an allowed create was refused: %v", err)
	}

	found, err := app.FindRecordsByFilter("contacts", "uid = {:u}", "", 0, 0,
		map[string]any{"u": "urn:uuid:ok"})
	if err != nil || len(found) != 1 {
		t.Fatalf("create did not persist (err=%v, n=%d)", err, len(found))
	}
	if found[0].GetString("owner") != user.Id {
		t.Fatal("owner not stamped from the authenticated user")
	}

	if _, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:ok.vcf",
		newCard("urn:uuid:ok", "Updated"), nil); err != nil {
		t.Fatalf("an allowed update was refused: %v", err)
	}
	fresh, err := app.FindRecordById("contacts", found[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetString("full_name") != "Updated" {
		t.Fatalf("update did not persist: %q", fresh.GetString("full_name"))
	}
}
