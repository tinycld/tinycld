package carddav

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupScopeApp builds an in-memory app with users + orgs + user_org, one org
// ("acme") and one member user, returning the app, the user, and the user_org id.
func setupScopeApp(t *testing.T) (*tests.TestApp, *core.Record, string) {
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

	orgs := core.NewBaseCollection("orgs")
	orgs.Fields.Add(&core.TextField{Name: "name", Required: true})
	orgs.Fields.Add(&core.TextField{Name: "slug", Required: true})
	if err := app.Save(orgs); err != nil {
		t.Fatalf("save orgs: %v", err)
	}

	userOrg := core.NewBaseCollection("user_org")
	userOrg.Fields.Add(&core.RelationField{Name: "org", Required: true, CollectionId: orgs.Id, MaxSelect: 1})
	userOrg.Fields.Add(&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(userOrg); err != nil {
		t.Fatalf("save user_org: %v", err)
	}

	user := core.NewRecord(users)
	user.Set("email", "alice@example.com")
	user.Set("password", "password123")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	org := core.NewRecord(orgs)
	org.Set("name", "Acme")
	org.Set("slug", "acme")
	if err := app.Save(org); err != nil {
		t.Fatalf("save org: %v", err)
	}

	uo := core.NewRecord(userOrg)
	uo.Set("org", org.Id)
	uo.Set("user", user.Id)
	if err := app.Save(uo); err != nil {
		t.Fatalf("save user_org: %v", err)
	}

	return app, user, uo.Id
}

func TestSingleOrgScope_ResolvesOwnMembership(t *testing.T) {
	app, user, uoID := setupScopeApp(t)
	s := singleOrgScope{bookSegment: "default"}

	books, err := s.Books(app, user)
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if len(books) != 1 || books[0].Path != "/carddav/u/ab/default/" {
		t.Fatalf("Books = %+v, want one default book", books)
	}

	ownerID, bookPath, err := s.ResolveBook(app, user, "ignored-hint")
	if err != nil {
		t.Fatalf("ResolveBook: %v", err)
	}
	if ownerID != uoID {
		t.Errorf("ownerID = %q, want the user's user_org id %q", ownerID, uoID)
	}
	if bookPath != "/carddav/u/ab/default/" {
		t.Errorf("bookPath = %q", bookPath)
	}
}

func TestSingleOrgScope_NoMembershipHidesBook(t *testing.T) {
	app, _, _ := setupScopeApp(t)
	// A different user with no user_org row.
	users, _ := app.FindCollectionByNameOrId("users")
	stranger := core.NewRecord(users)
	stranger.Set("email", "bob@example.com")
	stranger.Set("password", "password123")
	if err := app.Save(stranger); err != nil {
		t.Fatalf("save stranger: %v", err)
	}

	s := singleOrgScope{bookSegment: "default"}
	books, err := s.Books(app, stranger)
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("stranger should see no books, got %+v", books)
	}
	if _, _, err := s.ResolveBook(app, stranger, ""); err == nil {
		t.Error("ResolveBook should error for a non-member")
	}
}

func TestSharedDBScope_BooksPerMembershipAndResolveBySlug(t *testing.T) {
	app, user, uoID := setupScopeApp(t)
	s := sharedDBScope{}

	books, err := s.Books(app, user)
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if len(books) != 1 || books[0].Path != "/carddav/u/ab/acme/" {
		t.Fatalf("Books = %+v, want one acme book", books)
	}

	ownerID, bookPath, err := s.ResolveBook(app, user, "acme")
	if err != nil {
		t.Fatalf("ResolveBook: %v", err)
	}
	if ownerID != uoID {
		t.Errorf("ownerID = %q, want %q", ownerID, uoID)
	}
	if bookPath != "/carddav/u/ab/acme/" {
		t.Errorf("bookPath = %q", bookPath)
	}

	if _, _, err := s.ResolveBook(app, user, "ghost"); err == nil {
		t.Error("ResolveBook for a non-member slug should error")
	}
}
