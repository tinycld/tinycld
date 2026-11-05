package reports

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// requireNotViewable fails unless a FindViewableRecord call answered "no
// such record" (sql.ErrNoRows) with no record.
func requireNotViewable(t *testing.T, what string, rec *core.Record, err error) {
	t.Helper()
	if rec != nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%s: record %v, err %v; want sql.ErrNoRows", what, rec, err)
	}
}

// requireViewable fails unless a FindViewableRecord call returned the record
// with id.
func requireViewable(t *testing.T, what string, rec *core.Record, err error, id string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if rec == nil || rec.Id != id {
		t.Fatalf("%s: record %v, want %s", what, rec, id)
	}
}

// The view rule decides: its owner views the office row; another user, a
// missing id and a null rule are all "no such record", as PocketBase's view
// API answers 404 for each.
func TestFindViewableRecordFollowsTheViewRule(t *testing.T) {
	z := newZoo(t)
	office := addOffice(t, z)

	rec, err := FindViewableRecord(z.app, z.as("ann"), "zoo_office", office.Id)
	requireViewable(t, "the owner", rec, err, office.Id)
	if got := rec.GetString("name"); got != officeName {
		t.Fatalf("name = %q, want %q", got, officeName)
	}

	rec, err = FindViewableRecord(z.app, z.as("bob"), "zoo_office", office.Id)
	requireNotViewable(t, "another user", rec, err)

	rec, err = FindViewableRecord(z.app, z.as("ann"), "zoo_office", "nosuchrecord00")
	requireNotViewable(t, "a missing id", rec, err)

	// zoo_vault has no rules: only a superuser may view it.
	for _, user := range []string{"ann", "bob"} {
		rec, err = FindViewableRecord(z.app, z.as(user), "zoo_vault", z.id("v1"))
		requireNotViewable(t, user+" on a null view rule", rec, err)
	}
}

// An empty view rule is public; the view rule, not the list rule, decides.
func TestFindViewableRecordReturnsARowUnderAPublicViewRule(t *testing.T) {
	z := newZoo(t)
	board := core.NewBaseCollection("zoo_board")
	board.ViewRule = rule("")
	board.Fields.Add(&core.TextField{Name: "name"})
	z.save(t, board)
	notice := z.rec(t, "notice", "zoo_board", map[string]any{"name": "Closed Mondays"})
	rec, err := FindViewableRecord(z.app, z.as("bob"), "zoo_board", notice.Id)
	requireViewable(t, "a public view rule", rec, err, notice.Id)
}

// Review Focus 4: a view rule that reads @request.headers sees the headers
// the reader info carries.
func TestFindViewableRecordReadsTheReadersHeaders(t *testing.T) {
	z := newZoo(t)
	gate := core.NewBaseCollection("zoo_gate")
	gate.ViewRule = rule(`@request.headers.x_zoo = "open"`)
	gate.Fields.Add(&core.TextField{Name: "name"})
	z.save(t, gate)
	east := z.rec(t, "east", "zoo_gate", map[string]any{"name": "East gate"})

	open := listRequestInfo(z.recs["ann"], map[string]string{"x_zoo": "open"})
	rec, err := FindViewableRecord(z.app, open, "zoo_gate", east.Id)
	requireViewable(t, "with the header", rec, err, east.Id)

	rec, err = FindViewableRecord(z.app, z.as("ann"), "zoo_gate", east.Id)
	requireNotViewable(t, "without the header", rec, err)
}

// Review Focus 3: a database fault while the record is read is a fault, not
// "no such record". The fault is the table renamed away under the cached
// collection: the collection lookup reads the app's cache and still finds
// zoo_office, so the fault reaches the record read. (A closed test app
// answers sql.ErrNoRows, so it cannot stand in for a fault.)
func TestFindViewableRecordReturnsADatabaseFault(t *testing.T) {
	z := newZoo(t)
	office := addOffice(t, z)
	if _, err := z.app.DB().NewQuery("ALTER TABLE zoo_office RENAME TO gone_zoo_office").Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := z.app.FindCachedCollectionByNameOrId("zoo_office"); err != nil {
		t.Fatalf("the cached collection is gone too: %v", err)
	}
	rec, err := FindViewableRecord(z.app, z.as("ann"), "zoo_office", office.Id)
	if err == nil || errors.Is(err, sql.ErrNoRows) || rec != nil {
		t.Fatalf("record %v, err %v; want the database fault", rec, err)
	}
}

// A collection that does not exist is a package wiring mistake, not a
// record the reader may not view: PocketBase answers sql.ErrNoRows for it,
// which a caller would read as "hidden" and silently fall back.
func TestFindViewableRecordReturnsAnErrorForAnUnknownCollection(t *testing.T) {
	z := newZoo(t)
	rec, err := FindViewableRecord(z.app, z.as("ann"), "zoo_nothing", z.id("f1"))
	if err == nil || errors.Is(err, sql.ErrNoRows) || rec != nil {
		t.Fatalf("record %v, err %v; want an error that is not sql.ErrNoRows", rec, err)
	}
}

// Review Focus 3: a rule that cannot be built is a fault, not "no such
// record", or the caller would silently fall back to a wrong value.
func TestFindViewableRecordReturnsAFaultyRulesError(t *testing.T) {
	z := newZoo(t)
	shed := core.NewBaseCollection("zoo_shed")
	shed.ViewRule = rule(`name = "x"`)
	shed.Fields.Add(&core.TextField{Name: "name"})
	z.save(t, shed)
	tools := z.rec(t, "tools", "zoo_shed", map[string]any{"name": "x"})
	// Save refuses a rule on a field that does not exist; a collection can
	// still carry one (an edited schema, a hand-written migration).
	shed.ViewRule = rule(`no_such_field = "x"`)
	if err := z.app.SaveNoValidate(shed); err != nil {
		t.Fatal(err)
	}
	rec, err := FindViewableRecord(z.app, z.as("ann"), "zoo_shed", tools.Id)
	if err == nil || errors.Is(err, sql.ErrNoRows) || rec != nil {
		t.Fatalf("record %v, err %v; want the rule's error", rec, err)
	}
}
