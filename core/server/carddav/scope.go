package carddav

import (
	"github.com/pocketbase/pocketbase/core"
)

// OrgScope resolves which address books a user sees and, per request, the owner
// id + book path an object operation is scoped to.
//
// Single-org deployment: the process IS one org (whether the standalone app or a
// multi-org tenant). A user sees exactly one book, and its objects are the ones
// they own. Contact records now reference the `users` collection directly (the
// former `user_org` junction is gone), so the owner id IS the authenticated
// user's id — no membership lookup, no org slug in paths.
type OrgScope interface {
	// Books lists the address books visible to the authenticated user.
	Books(app core.App, user *core.Record) ([]Book, error)

	// ResolveBook maps a request to the owner id its objects are filtered by
	// (bound to {:ownerId} in Source.ListFilter and set on Source.OwnerField for
	// new records) and the book path used to build object URLs.
	ResolveBook(app core.App, user *core.Record) (ownerID, bookPath string, err error)
}

// Book is one address book in a listing.
type Book struct {
	Path        string
	Name        string
	Description string
}

// singleOrgScope serves one org: the whole DB. There is exactly one book; its
// objects are filtered to the authenticated user (the owner field now holds a
// users id). No org slug appears in paths.
type singleOrgScope struct {
	// bookSegment is the fixed path segment for the sole book (e.g. "default").
	bookSegment string
}

func (s singleOrgScope) book() Book {
	return Book{
		Path:        "/carddav/u/ab/" + s.bookSegment + "/",
		Name:        "Contacts",
		Description: "Contacts",
	}
}

func (s singleOrgScope) Books(_ core.App, user *core.Record) ([]Book, error) {
	if user == nil || user.Id == "" {
		return nil, nil
	}
	return []Book{s.book()}, nil
}

func (s singleOrgScope) ResolveBook(_ core.App, user *core.Record) (string, string, error) {
	return user.Id, s.book().Path, nil
}
