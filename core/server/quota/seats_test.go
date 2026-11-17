package quota

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newSeatsApp builds a test app whose users collection carries the fields the
// seat rule reads: role and disabled.
func newSeatsApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("new test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if users.Fields.GetByName("role") == nil {
		users.Fields.Add(&core.TextField{Name: "role"})
	}
	if users.Fields.GetByName("disabled") == nil {
		users.Fields.Add(&core.BoolField{Name: "disabled"})
	}
	if err := app.Save(users); err != nil {
		t.Fatalf("save users: %v", err)
	}

	// tests.NewTestApp seeds the users collection from PocketBase's own
	// fixture data.db (test@example.com and friends). Those rows have no
	// role set, so they count as seats and would throw off every count in
	// this file; the seat rule only cares about the users this test creates.
	seeded, err := app.FindAllRecords("users")
	if err != nil {
		t.Fatalf("find seeded users: %v", err)
	}
	for _, rec := range seeded {
		if err := app.Delete(rec); err != nil {
			t.Fatalf("delete seeded user %s: %v", rec.Id, err)
		}
	}

	return app
}

// newUserRecord builds an unsaved user. The username is set from the email
// because the fixture's users collection otherwise draws it at random from
// users[0-9]{5}: two users can draw the same name, and the save then fails
// on the unique index. A refusal test would pass on that error for the
// wrong reason.
func newUserRecord(col *core.Collection, email, role string) *core.Record {
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.Set("username", strings.ReplaceAll(email, "@", "."))
	rec.SetPassword("secret12345")
	rec.Set("role", role)
	return rec
}

func addUser(t *testing.T, app core.App, email, role string, disabled bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	rec := newUserRecord(col, email, role)
	rec.Set("disabled", disabled)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}

	// Save() does not refresh rec's own originalData (PostScan only runs on
	// a DB-scanned read), so a caller that later mutates and re-saves this
	// same in-memory record would see a blank Original() rather than what
	// was just persisted. Re-fetching mirrors how real update handlers work
	// (FindRecordById, then Set, then Save) and keeps Original() honest.
	fresh, err := app.FindRecordById("users", rec.Id)
	if err != nil {
		t.Fatalf("reload user %s: %v", email, err)
	}
	return fresh
}

// limitOf returns a resolver over *n, so a test can change the plan mid-test.
func limitOf(n *int) SeatLimitFunc { return func(core.App) int { return *n } }

func bindWithLimit(t *testing.T, app core.App, n *int) {
	t.Helper()
	SetSeatLimit(app, limitOf(n))
	BindSeats(app)
}

func TestCountSeats_ExcludesGuestsAndDisabled(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	addUser(t, app, "owner@x.test", "owner", false)
	addUser(t, app, "member@x.test", "member", false)
	addUser(t, app, "guest@x.test", "guest", false)
	addUser(t, app, "off@x.test", "member", true)
	if got, err := CountSeats(app); err != nil || got != 2 {
		t.Fatalf("CountSeats = %d, %v; want 2", got, err)
	}
}

func TestSeats_RefusesACreateOverTheCap(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 2
	bindWithLimit(t, app, &limit)
	addUser(t, app, "a@x.test", "member", false)
	addUser(t, app, "b@x.test", "member", false)

	col, _ := app.FindCollectionByNameOrId("users")
	err := app.Save(newUserRecord(col, "c@x.test", "member"))
	if err == nil {
		t.Fatal("a third member on a 2-seat plan must be refused")
	}
	if !strings.Contains(err.Error(), "User limit reached: 2 of 2 seats in use") {
		t.Fatalf("error = %q, want the seat message", err.Error())
	}
}

// Guests do not consume seats, so an org at its cap can still add one.
func TestSeats_AllowsAGuestAtTheCap(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 1
	bindWithLimit(t, app, &limit)
	addUser(t, app, "a@x.test", "member", false)
	col, _ := app.FindCollectionByNameOrId("users")
	if err := app.Save(newUserRecord(col, "g@x.test", "guest")); err != nil {
		t.Fatalf("a guest must not consume a seat: %v", err)
	}
}

// The bypass a create-only check would leave open.
func TestSeats_RefusesPromotingAGuestOverTheCap(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 1
	bindWithLimit(t, app, &limit)
	addUser(t, app, "a@x.test", "member", false)
	guest := addUser(t, app, "g@x.test", "guest", false)
	guest.Set("role", "member")
	if err := app.Save(guest); err == nil {
		t.Fatal("promoting a guest past the cap must be refused")
	}
}

// The other bypass: re-enabling a disabled user is a transition INTO a seat.
func TestSeats_RefusesReenablingOverTheCap(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 1
	bindWithLimit(t, app, &limit)
	addUser(t, app, "a@x.test", "member", false)
	off := addUser(t, app, "b@x.test", "member", true)
	off.Set("disabled", false)
	if err := app.Save(off); err == nil {
		t.Fatal("re-enabling a user past the cap must be refused")
	}
}

// Freeze, not revoke: an over-limit org must still be able to get back under.
func TestSeats_AllowsDemotionAndDisableWhenOverTheCap(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 2
	bindWithLimit(t, app, &limit)
	a := addUser(t, app, "a@x.test", "member", false)
	b := addUser(t, app, "b@x.test", "member", false)
	limit = 1
	b.Set("disabled", true)
	if err := app.Save(b); err != nil {
		t.Fatalf("disabling while over the cap must be allowed: %v", err)
	}
	a.Set("role", "guest")
	if err := app.Save(a); err != nil {
		t.Fatalf("demoting while over the cap must be allowed: %v", err)
	}
}

// A blank Original() must not be read as "was already a seat".
func TestSeats_RefusesWhenOriginalIsBlank(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	limit := 1
	bindWithLimit(t, app, &limit)
	col, _ := app.FindCollectionByNameOrId("users")
	rec := newUserRecord(col, "g@x.test", "guest")
	if err := app.Save(rec); err != nil {
		t.Fatalf("creating a guest at a 1-seat cap must be allowed: %v", err)
	}
	addUser(t, app, "a@x.test", "member", false)
	rec.Set("role", "member")
	if err := app.Save(rec); err == nil {
		t.Fatal("promoting past the cap must be refused even when Original() is blank")
	}
}

func TestSeats_UnlimitedWhenNobodyClaimsAndNoSetting(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	BindSeats(app)
	for _, e := range []string{"a@x.test", "b@x.test", "c@x.test"} {
		addUser(t, app, e, "member", false)
	}
	if got := SeatLimit(app); got != 0 {
		t.Fatalf("SeatLimit = %d, want 0", got)
	}
}

// The first claim wins, so nothing registered later can raise the limit.
func TestSetSeatLimit_FirstClaimWins(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	SetSeatLimit(app, func(core.App) int { return 3 })
	SetSeatLimit(app, func(core.App) int { return 99 })
	if got := SeatLimit(app); got != 3 {
		t.Fatalf("SeatLimit = %d, want 3", got)
	}
}

// Enforcement fails OPEN: a seat limit is a commercial limit, not a security
// boundary, so an unreadable count must not block the write.
func TestCheckSeat_AllowsTheWriteWhenTheCountFails(t *testing.T) {
	t.Parallel()
	app := newSeatsApp(t)
	SetSeatLimit(app, func(core.App) int { return 1 })
	if _, err := app.DB().NewQuery("DROP TABLE users").Execute(); err != nil {
		t.Fatalf("drop users: %v", err)
	}
	if err := checkSeat(app); err != nil {
		t.Fatalf("an unreadable seat count must not refuse the write, got: %v", err)
	}
}
