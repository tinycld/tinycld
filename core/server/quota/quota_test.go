package quota

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupQuotaApp builds two storage-bearing collections: one owned (drive-like,
// with a created_by) and one shared (mail-like, with no owner at all). The
// asymmetry is the point — it is what the per-user vs per-org split turns on.
func setupQuotaApp(t *testing.T) (*tests.TestApp, *pocketbase.PocketBase, *core.Record, *core.Record) {
	t.Helper()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(testApp.Cleanup)

	users, err := testApp.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	mkUser := func(email string) *core.Record {
		u := core.NewRecord(users)
		u.Set("email", email)
		u.Set("password", "password123")
		if err := testApp.Save(u); err != nil {
			t.Fatalf("save %s: %v", email, err)
		}
		return u
	}
	alice, bob := mkUser("alice@example.com"), mkUser("bob@example.com")

	owned := core.NewBaseCollection("owned_items")
	owned.Fields.Add(&core.TextField{Name: "name"})
	owned.Fields.Add(&core.NumberField{Name: "size"})
	owned.Fields.Add(&core.RelationField{
		Name: "created_by", CollectionId: users.Id, MaxSelect: 1,
	})
	if err := testApp.Save(owned); err != nil {
		t.Fatal(err)
	}

	shared := core.NewBaseCollection("shared_items")
	shared.Fields.Add(&core.TextField{Name: "name"})
	shared.Fields.Add(&core.NumberField{Name: "total_size"})
	if err := testApp.Save(shared); err != nil {
		t.Fatal(err)
	}

	// A PocketBase app sharing the test app's data dir, so record hooks can be
	// bound the way coreserver does.
	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: testApp.DataDir()})

	return testApp, pbApp, alice, bob
}

func testSources() []Source {
	return []Source{
		{Slug: "drive", Collection: "owned_items", SizeField: "size", OwnerField: "created_by"},
		{Slug: "mail", Collection: "shared_items", SizeField: "total_size"},
	}
}

func addOwned(t *testing.T, app core.App, owner *core.Record, name string, size int) error {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("owned_items")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("size", size)
	if owner != nil {
		r.Set("created_by", owner.Id)
	}
	return app.Save(r)
}

func addShared(t *testing.T, app core.App, name string, size int) error {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("shared_items")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("total_size", size)
	return app.Save(r)
}

func TestUserUsageCountsOnlyOwnedSources(t *testing.T) {
	app, _, alice, bob := setupQuotaApp(t)

	if err := addOwned(t, app, alice, "a", 100); err != nil {
		t.Fatal(err)
	}
	if err := addOwned(t, app, bob, "b", 500); err != nil {
		t.Fatal(err)
	}
	// Shared bytes belong to nobody in particular.
	if err := addShared(t, app, "msg", 9000); err != nil {
		t.Fatal(err)
	}

	got, err := UserUsage(app, testSources(), alice.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got != 100 {
		t.Fatalf("alice usage = %d, want 100 (bob's and the shared rows must not count)", got)
	}
}

func TestOrgUsageCountsEverySource(t *testing.T) {
	app, _, alice, bob := setupQuotaApp(t)

	addOwned(t, app, alice, "a", 100)
	addOwned(t, app, bob, "b", 500)
	addShared(t, app, "msg", 9000)

	got, err := OrgUsage(app, testSources())
	if err != nil {
		t.Fatal(err)
	}
	if got != 9600 {
		t.Fatalf("org usage = %d, want 9600 (owned + shared)", got)
	}
}

// A package can declare a quota source and simply not be installed. Summing
// must treat that as zero, not fail — the lean-shell guarantee.
func TestUsageToleratesMissingCollection(t *testing.T) {
	app, _, alice, _ := setupQuotaApp(t)
	addOwned(t, app, alice, "a", 100)

	sources := append(testSources(), Source{
		Slug: "absent", Collection: "not_installed", SizeField: "size", OwnerField: "created_by",
	})

	got, err := OrgUsage(app, sources)
	if err != nil {
		t.Fatalf("a missing collection must not be an error: %v", err)
	}
	if got != 100 {
		t.Fatalf("org usage = %d, want 100", got)
	}
}

func TestValidateSourceRejectsUnsafeIdentifiers(t *testing.T) {
	bad := []Source{
		{Collection: "items; DROP TABLE users--", SizeField: "size"},
		{Collection: "items", SizeField: "size; --"},
		{Collection: "items", SizeField: "size", OwnerField: `owner"`},
		{Collection: "", SizeField: "size"},
		{Collection: "items", SizeField: ""},
	}
	for _, src := range bad {
		if err := ValidateSource(src); err == nil {
			t.Fatalf("expected rejection of %+v", src)
		}
	}
	if err := ValidateSource(testSources()[0]); err != nil {
		t.Fatalf("the reference Source must validate: %v", err)
	}
}

// THE enforcement property: a write that would breach the per-user ceiling is
// refused at the record layer, so no protocol can route around it.
func TestPerUserCeilingRefusesWrite(t *testing.T) {
	app, pbApp, alice, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{PerUser: 1000}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if err := addOwned(t, pbApp, alice, "fits", 900); err != nil {
		t.Fatalf("a write within the ceiling must succeed: %v", err)
	}
	err := addOwned(t, pbApp, alice, "overflows", 200)
	if err == nil {
		t.Fatal("a write breaching the per-user ceiling must be refused")
	}
	// NewApiError capitalizes the message, so match case-insensitively on the
	// scope word rather than the whole sentence.
	if !strings.Contains(strings.ToLower(err.Error()), "user storage limit exceeded") {
		t.Fatalf("error %q should name the user ceiling", err)
	}

	// The refused row must not have landed.
	got, _ := UserUsage(app, testSources(), alice.Id)
	if got != 900 {
		t.Fatalf("usage after a refused write = %d, want 900", got)
	}
}

// The org ceiling spans packages, including sources with no owner — which is
// the whole reason mail participates.
func TestPerOrgCeilingCountsSharedData(t *testing.T) {
	_, pbApp, alice, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{PerOrg: 1000}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	// Shared (unowned) bytes fill most of the org ceiling...
	if err := addShared(t, pbApp, "big-message", 900); err != nil {
		t.Fatal(err)
	}
	// ...so an owned write that fits the user ceiling is still refused.
	err := addOwned(t, pbApp, alice, "small", 200)
	if err == nil {
		t.Fatal("shared bytes must count toward the org ceiling")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "organization storage limit exceeded") {
		t.Fatalf("error %q should name the org ceiling", err)
	}
}

// An update consumes only the growth. Charging the full new size would refuse
// writes that shrink a record or leave it unchanged.
func TestUpdateChargesOnlyTheDelta(t *testing.T) {
	_, pbApp, alice, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{PerUser: 1000}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if err := addOwned(t, pbApp, alice, "doc", 900); err != nil {
		t.Fatal(err)
	}
	rec, err := pbApp.FindFirstRecordByData("owned_items", "name", "doc")
	if err != nil {
		t.Fatal(err)
	}

	// 900 -> 950 is +50, comfortably inside the ceiling even though 950+900
	// would not be.
	rec.Set("size", 950)
	if err := pbApp.Save(rec); err != nil {
		t.Fatalf("a growth of 50 within the ceiling must succeed: %v", err)
	}

	// Shrinking must always be allowed — it is how an over-quota org recovers.
	rec.Set("size", 10)
	if err := pbApp.Save(rec); err != nil {
		t.Fatalf("shrinking must never be refused: %v", err)
	}

	// And a growth that genuinely breaches is still refused.
	rec.Set("size", 2000)
	if err := pbApp.Save(rec); err == nil {
		t.Fatal("a growth past the ceiling must be refused")
	}
}

// Deleting must stay possible at any usage level, or an org moved to a smaller
// plan could never get back under it.
func TestDeleteIsNeverRefused(t *testing.T) {
	_, pbApp, alice, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{PerUser: 1000, PerOrg: 1000}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if err := addOwned(t, pbApp, alice, "doc", 900); err != nil {
		t.Fatal(err)
	}
	rec, err := pbApp.FindFirstRecordByData("owned_items", "name", "doc")
	if err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Delete(rec); err != nil {
		t.Fatalf("delete must never be refused: %v", err)
	}
}

// Zero means unlimited; a deployment that has set no limit is not throttled.
func TestZeroLimitIsUnlimited(t *testing.T) {
	_, pbApp, alice, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if err := addOwned(t, pbApp, alice, "huge", 1<<30); err != nil {
		t.Fatalf("no ceiling must mean no refusal: %v", err)
	}
}

// A row with no owner cannot breach a per-user ceiling — there is nobody to
// charge — but it still counts toward the org total (covered above).
func TestSharedRowSkipsPerUserCheck(t *testing.T) {
	_, pbApp, _, _ := setupQuotaApp(t)

	if err := Register(pbApp, testSources(), func(core.App) Limits {
		return Limits{PerUser: 100}
	}); err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if err := addShared(t, pbApp, "msg", 5000); err != nil {
		t.Fatalf("unowned data must not be charged to a user: %v", err)
	}
}
