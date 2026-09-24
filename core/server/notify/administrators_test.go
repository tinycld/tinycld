package notify

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupAdminApp builds the two collections Administrators touches and seeds
// one user per role.
func setupAdminApp(t *testing.T) *tests.TestApp {
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
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	notifications := core.NewBaseCollection("notifications")
	notifications.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	notifications.Fields.Add(&core.TextField{Name: "type"})
	notifications.Fields.Add(&core.TextField{Name: "package"})
	notifications.Fields.Add(&core.TextField{Name: "title"})
	notifications.Fields.Add(&core.TextField{Name: "body"})
	notifications.Fields.Add(&core.TextField{Name: "url"})
	notifications.Fields.Add(&core.JSONField{Name: "metadata"})
	notifications.Fields.Add(&core.BoolField{Name: "read"})
	if err := app.Save(notifications); err != nil {
		t.Fatal(err)
	}

	return app
}

func seedUser(t *testing.T, app *tests.TestApp, email, role string, disabled bool) *core.Record {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.Set("email", email)
	u.SetPassword("Password123!")
	u.Set("role", role)
	u.Set("disabled", disabled)
	if err := app.Save(u); err != nil {
		t.Fatalf("seed %s: %v", email, err)
	}
	return u
}

func notificationCount(t *testing.T, app *tests.TestApp) int {
	t.Helper()
	recs, err := app.FindRecordsByFilter("notifications", "id != ''", "", 0, 0, nil)
	if err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return len(recs)
}

// Addressed to the role, not to a person: the owner may be away, and the
// point is that somebody who can act finds out.
func TestAdministrators_ReachesEveryOwnerAndAdmin(t *testing.T) {
	app := setupAdminApp(t)
	seedUser(t, app, "owner@example.com", "owner", false)
	seedUser(t, app, "admin@example.com", "admin", false)
	seedUser(t, app, "member@example.com", "member", false)
	seedUser(t, app, "guest@example.com", "guest", false)

	delivered, err := Administrators(app, NotifyParams{Type: "system.notice", Title: "T", Body: "B"})
	if err != nil {
		t.Fatalf("Administrators: %v", err)
	}
	if delivered != 2 {
		t.Errorf("delivered = %d, want 2 (the owner and the admin)", delivered)
	}
	if got := notificationCount(t, app); got != 2 {
		t.Errorf("%d notifications written, want 2 — a member must not be told", got)
	}
}

// A disabled administrator cannot act on it, and for the case that motivated
// this helper one of them may be the reason the notice exists.
func TestAdministrators_SkipsDisabledAdministrators(t *testing.T) {
	app := setupAdminApp(t)
	seedUser(t, app, "owner@example.com", "owner", false)
	seedUser(t, app, "gone@example.com", "admin", true)

	delivered, err := Administrators(app, NotifyParams{Type: "system.notice", Title: "T", Body: "B"})
	if err != nil {
		t.Fatalf("Administrators: %v", err)
	}
	if delivered != 1 {
		t.Errorf("delivered = %d, want 1 — a disabled admin must be skipped", delivered)
	}
}

// Each recipient gets their own row addressed to them, rather than one row
// shared or the last writer winning.
func TestAdministrators_AddressesEachRecipient(t *testing.T) {
	app := setupAdminApp(t)
	owner := seedUser(t, app, "owner@example.com", "owner", false)
	admin := seedUser(t, app, "admin@example.com", "admin", false)

	if _, err := Administrators(app, NotifyParams{Type: "system.notice", Title: "T", Body: "B"}); err != nil {
		t.Fatalf("Administrators: %v", err)
	}

	for _, u := range []*core.Record{owner, admin} {
		recs, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0, map[string]any{"u": u.Id})
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 1 {
			t.Errorf("user %s got %d notifications, want 1", u.Id, len(recs))
		}
	}
}

// A deployment with no administrators is a real state (mid-provisioning), and
// it is not an error — there is simply nobody to tell.
func TestAdministrators_NoAdministratorsIsNotAnError(t *testing.T) {
	app := setupAdminApp(t)
	seedUser(t, app, "member@example.com", "member", false)

	delivered, err := Administrators(app, NotifyParams{Type: "system.notice", Title: "T", Body: "B"})
	if err != nil {
		t.Errorf("no administrators must not be an error, got %v", err)
	}
	if delivered != 0 {
		t.Errorf("delivered = %d, want 0", delivered)
	}
}
