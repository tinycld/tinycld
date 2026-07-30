package carddav

import (
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// precondition_test.go pins If-Match / If-None-Match enforcement on PUT —
// see caldav's twin for the rationale (silent last-writer-wins otherwise).

func setupPrecondApp(t *testing.T) (*tests.TestApp, *Backend, *core.Record) {
	app, backend, user := setupPutAuthzApp(t)
	setContactRules(t, app, `owner = @request.auth.id`, `owner = @request.auth.id`)

	// The shipped contacts schema has an autodate `updated` — the ETag's
	// source. The shared fixture predates it, and with no stored stamp every
	// If-Match 412s regardless of correctness.
	col, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		t.Fatal(err)
	}
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	return app, backend, user
}

// seedContact creates a contact owned by user and returns it.
func seedContact(t *testing.T, app *tests.TestApp, userID, uid, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("uid", uid)
	rec.Set("full_name", name)
	rec.Set("owner", userID)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func wantPrecondFailed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("PUT succeeded, want 412 Precondition Failed")
	}
	if !strings.Contains(err.Error(), "412") {
		t.Fatalf("err = %v, want a 412 Precondition Failed", err)
	}
}

func TestPutAddressObject_IfNoneMatchRefusesOverwrite(t *testing.T) {
	app, backend, user := setupPrecondApp(t)
	rec := seedContact(t, app, user.Id, "urn:uuid:pc1", "Original")
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:pc1.vcf",
		newCard("urn:uuid:pc1", "Clobbered"),
		&carddav.PutAddressObjectOptions{IfNoneMatch: "*"})
	wantPrecondFailed(t, err)

	reloaded, _ := app.FindRecordById("contacts", rec.Id)
	if got := reloaded.GetString("full_name"); got != "Original" {
		t.Errorf("full_name = %q; the refused PUT was written anyway", got)
	}
}

func TestPutAddressObject_IfMatchStaleRefused(t *testing.T) {
	app, backend, user := setupPrecondApp(t)
	rec := seedContact(t, app, user.Id, "urn:uuid:pc2", "Original")
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:pc2.vcf",
		newCard("urn:uuid:pc2", "Based on stale copy"),
		&carddav.PutAddressObjectOptions{IfMatch: `"2001-01-01 00:00:00.000Z"`})
	wantPrecondFailed(t, err)

	reloaded, _ := app.FindRecordById("contacts", rec.Id)
	if got := reloaded.GetString("full_name"); got != "Original" {
		t.Errorf("full_name = %q; the refused PUT was written anyway", got)
	}
}

func TestPutAddressObject_IfMatchCurrentSucceeds(t *testing.T) {
	app, backend, user := setupPrecondApp(t)
	rec := seedContact(t, app, user.Id, "urn:uuid:pc3", "Original")
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	etag := fmt.Sprintf("%q", rec.GetString("updated"))
	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:pc3.vcf",
		newCard("urn:uuid:pc3", "ConditionalUpdate"),
		&carddav.PutAddressObjectOptions{IfMatch: webdav.ConditionalMatch(etag)})
	if err != nil {
		t.Fatalf("PUT with the current ETag should succeed, got %v", err)
	}

	reloaded, _ := app.FindRecordById("contacts", rec.Id)
	if got := reloaded.GetString("full_name"); got != "ConditionalUpdate" {
		t.Errorf("full_name = %q, want the conditional update applied", got)
	}
}

func TestPutAddressObject_IfMatchOnMissingRefused(t *testing.T) {
	_, backend, _ := setupPrecondApp(t)
	ctx := authedCtx(t, "alice@example.com", "Password123!")

	_, err := backend.PutAddressObject(ctx, "/carddav/u/ab/default/urn:uuid:ghost.vcf",
		newCard("urn:uuid:ghost", "Never created"),
		&carddav.PutAddressObjectOptions{IfMatch: `"whatever"`})
	wantPrecondFailed(t, err)
}
