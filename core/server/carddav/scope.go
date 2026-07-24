package carddav

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// OrgScope resolves which address books a user sees and, per request, the owner
// id + book path an object operation is scoped to. It is the single seam that
// differs between deployment models:
//
//   - sharedDBScope (single-tenant): one process holds every org's data in one
//     DB. Books = one per the user's org membership; the owner id is the user's
//     user_org row for that org; the book path carries the org slug.
//   - singleOrgScope (multi-org tenant): the process IS one org (stock PB per
//     tenant, dispatched by subdomain). One book; the owner id is the user's own
//     membership in this org; no slug in the path.
//
// The protocol mechanics (PROPFIND/REPORT, ETag), the vCard codec, and the
// Source/VCardMap config are identical across both — only org resolution varies.
type OrgScope interface {
	// Books lists the address books visible to the authenticated user.
	Books(app core.App, user *core.Record) ([]Book, error)

	// ResolveBook maps a request to the owner id its objects are filtered by
	// (bound to {:ownerId} in Source.ListFilter and set on Source.OwnerField for
	// new records) and the book path used to build object URLs. orgHint is the
	// org slug the backend extracted from the request (Host subdomain or path);
	// singleOrgScope ignores it, sharedDBScope uses it.
	ResolveBook(app core.App, user *core.Record, orgHint string) (ownerID, bookPath string, err error)
}

// Book is one address book in a listing.
type Book struct {
	Path        string
	Name        string
	Description string
}

// --- shared-DB scope (single-tenant: org-by-slug across one DB) ---

type sharedDBScope struct{}

func (sharedDBScope) Books(app core.App, user *core.Record) ([]Book, error) {
	userOrgs, err := app.FindRecordsByFilter("user_org", "user = {:userId}", "", 0, 0,
		map[string]any{"userId": user.Id})
	if err != nil {
		return nil, err
	}
	var books []Book
	for _, uo := range userOrgs {
		org, err := app.FindRecordById("orgs", uo.GetString("org"))
		if err != nil {
			continue
		}
		slug := org.GetString("slug")
		books = append(books, Book{
			Path:        fmt.Sprintf("/carddav/u/ab/%s/", slug),
			Name:        org.GetString("name"),
			Description: fmt.Sprintf("Contacts for %s", org.GetString("name")),
		})
	}
	return books, nil
}

func (sharedDBScope) ResolveBook(app core.App, user *core.Record, orgHint string) (string, string, error) {
	userOrg, err := findUserOrgBySlug(app, user.Id, orgHint)
	if err != nil {
		return "", "", err
	}
	return userOrg.Id, fmt.Sprintf("/carddav/u/ab/%s/", orgHint), nil
}

// findUserOrgBySlug resolves the user's user_org row for the org with the given
// slug (shared-DB model).
func findUserOrgBySlug(app core.App, userID, orgSlug string) (*core.Record, error) {
	orgs, err := app.FindRecordsByFilter("orgs", "slug = {:slug}", "", 1, 0,
		map[string]any{"slug": orgSlug})
	if err != nil || len(orgs) == 0 {
		return nil, fmt.Errorf("org not found: %s", orgSlug)
	}
	userOrgs, err := app.FindRecordsByFilter("user_org", "user = {:userId} && org = {:orgId}", "", 1, 0,
		map[string]any{"userId": userID, "orgId": orgs[0].Id})
	if err != nil || len(userOrgs) == 0 {
		return nil, fmt.Errorf("user is not a member of org %s", orgSlug)
	}
	return userOrgs[0], nil
}

// --- single-org scope (multi-org tenant: this DB IS the org) ---

// singleOrgScope serves one org: the whole tenant DB. There is exactly one book;
// its objects are filtered to the authenticated user's membership in THIS org.
// The owner id is the user's single user_org row (this DB holds one org, so a
// user has at most one). No org slug appears in paths.
type singleOrgScope struct {
	// bookSegment is the fixed path segment for the sole book (e.g. "default").
	bookSegment string
}

func (s singleOrgScope) book() Book {
	return Book{
		Path:        fmt.Sprintf("/carddav/u/ab/%s/", s.bookSegment),
		Name:        "Contacts",
		Description: "Contacts",
	}
}

func (s singleOrgScope) Books(app core.App, user *core.Record) ([]Book, error) {
	// Only surface the book if the user actually has a membership in this org
	// (guards an authenticated-but-unprovisioned user).
	if _, err := s.ownerID(app, user); err != nil {
		return nil, nil
	}
	return []Book{s.book()}, nil
}

func (s singleOrgScope) ResolveBook(app core.App, user *core.Record, _ string) (string, string, error) {
	ownerID, err := s.ownerID(app, user)
	if err != nil {
		return "", "", err
	}
	return ownerID, s.book().Path, nil
}

// ownerID returns the user's user_org row id in this (single-org) DB.
func (s singleOrgScope) ownerID(app core.App, user *core.Record) (string, error) {
	userOrgs, err := app.FindRecordsByFilter("user_org", "user = {:userId}", "", 1, 0,
		map[string]any{"userId": user.Id})
	if err != nil || len(userOrgs) == 0 {
		return "", fmt.Errorf("user has no membership in this org")
	}
	return userOrgs[0].Id, nil
}
