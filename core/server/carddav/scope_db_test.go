package carddav

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupScopeApp builds an in-memory app with a users collection and one member
// user, returning the app and the user. Single-org: contacts reference the
// users collection directly (the former user_org junction is gone), so the
// owner id IS the authenticated user's id.
func setupScopeApp(t *testing.T) (*tests.TestApp, *core.Record) {
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

	user := core.NewRecord(users)
	user.Set("email", "alice@example.com")
	user.Set("password", "password123")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	return app, user
}

func TestSingleOrgScope_ResolvesOwnBook(t *testing.T) {
	app, user := setupScopeApp(t)
	s := singleOrgScope{bookSegment: "default"}

	books, err := s.Books(app, user)
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if len(books) != 1 || books[0].Path != "/carddav/u/ab/default/" {
		t.Fatalf("Books = %+v, want one default book", books)
	}

	// orgHint is ignored under single-org; the owner id is the user's id.
	ownerID, bookPath, err := s.ResolveBook(app, user, "ignored-hint")
	if err != nil {
		t.Fatalf("ResolveBook: %v", err)
	}
	if ownerID != user.Id {
		t.Errorf("ownerID = %q, want the user's id %q", ownerID, user.Id)
	}
	if bookPath != "/carddav/u/ab/default/" {
		t.Errorf("bookPath = %q", bookPath)
	}
}

func TestSingleOrgScope_UnauthenticatedHidesBook(t *testing.T) {
	app, _ := setupScopeApp(t)
	s := singleOrgScope{bookSegment: "default"}

	// A nil user (unauthenticated) sees no books.
	books, err := s.Books(app, nil)
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("unauthenticated user should see no books, got %+v", books)
	}
}

func TestSingleOrgScope_ContactsFilteredToOwner(t *testing.T) {
	app, user := setupScopeApp(t)

	// Seed a contacts collection whose owner relation points at the users
	// collection directly (single-org model), plus a stranger who owns their
	// own contact so we can assert the scope filters by owner id.
	users, _ := app.FindCollectionByNameOrId("users")
	stranger := core.NewRecord(users)
	stranger.Set("email", "bob@example.com")
	stranger.Set("password", "password123")
	if err := app.Save(stranger); err != nil {
		t.Fatalf("save stranger: %v", err)
	}

	contacts := core.NewBaseCollection("contacts")
	contacts.Fields.Add(&core.RelationField{Name: "owner", Required: true, CollectionId: users.Id, MaxSelect: 1})
	contacts.Fields.Add(&core.TextField{Name: "first_name"})
	contacts.Fields.Add(&core.TextField{Name: "deleted_at"})
	if err := app.Save(contacts); err != nil {
		t.Fatalf("save contacts: %v", err)
	}

	mine := core.NewRecord(contacts)
	mine.Set("owner", user.Id)
	mine.Set("first_name", "Mine")
	if err := app.Save(mine); err != nil {
		t.Fatalf("save mine: %v", err)
	}

	theirs := core.NewRecord(contacts)
	theirs.Set("owner", stranger.Id)
	theirs.Set("first_name", "Theirs")
	if err := app.Save(theirs); err != nil {
		t.Fatalf("save theirs: %v", err)
	}

	s := singleOrgScope{bookSegment: "default"}
	ownerID, _, err := s.ResolveBook(app, user, "")
	if err != nil {
		t.Fatalf("ResolveBook: %v", err)
	}

	// The resolved owner id is what the backend binds to {:ownerId}; a query
	// scoped to it must return only the user's own contact.
	rows, err := app.FindRecordsByFilter("contacts", "owner = {:ownerId}", "", 0, 0,
		map[string]any{"ownerId": ownerID})
	if err != nil {
		t.Fatalf("find contacts: %v", err)
	}
	if len(rows) != 1 || rows[0].GetString("first_name") != "Mine" {
		t.Fatalf("owner-scoped query = %+v, want only the user's own contact", rows)
	}
}
