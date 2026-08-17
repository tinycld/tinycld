// Package pkgaccess enforces org_pkg_access levels server-side.
//
// A user's access to a package is 'full', 'readonly', or 'none'
// (org_pkg_access rows, managed from the members panel). The client resolves
// the level to gate screens (core/lib/use-pkg-access.ts — the two resolutions
// MUST agree), but a level the server never checks is advisory: a readonly
// user's direct REST write to a package's collections sailed through on the
// collection rules, which know nothing about package access.
//
// Ownership is by naming convention: a collection belongs to the installed
// package (pkg_registry, status installed|bundled) whose slug it carries as
// its name or name prefix (`drive` owns `drive_items`; dashes in slugs match
// underscores). That is how every first-party package names its collections,
// it needs no per-package Go or manifest wiring, and it covers TS-only
// packages automatically. Collections matching no installed slug — core's
// own (users, labels, settings, …) — are outside package access entirely.
//
// What this guards: authenticated REST record writes (create/update/delete
// request hooks) — reads stay rule-governed, and system writes (inbound mail
// delivery, seeds, migrations, feature Go) carry no request auth and are
// untouched. Protocol servers enforce the same level at their own write
// entry points via CanWrite (they bypass the REST layer): core's DAV
// backends and mail's IMAP/SMTP sessions.
package pkgaccess

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("pkgaccess")

const (
	LevelFull     = "full"
	LevelReadonly = "readonly"
	LevelNone     = "none"
)

// Register binds the REST write guard on every collection. Bound once by
// coreserver's shared registration, so both compositions enforce it.
func Register(app core.App) {
	check := func(e *core.RecordRequestEvent) error {
		if err := guardWrite(e.App, e.Auth, e.Collection.Name); err != nil {
			return err
		}
		return e.Next()
	}
	app.OnRecordCreateRequest().BindFunc(check)
	app.OnRecordUpdateRequest().BindFunc(check)
	app.OnRecordDeleteRequest().BindFunc(check)
}

// CanWrite reports whether the user's level for the package admits writes.
// The error form of the same decision is WriteError. For protocol servers
// (DAV, IMAP/SMTP) whose writes bypass the REST hooks.
func CanWrite(app core.App, user *core.Record, slug string) bool {
	return WriteError(app, user, slug) == nil
}

// WriteError returns nil when the user may write to the package, or the
// refusal (a 403 apis.ApiError for HTTP surfaces) when they may not.
func WriteError(app core.App, user *core.Record, slug string) error {
	if user == nil || user.IsSuperuser() {
		return nil
	}
	level, err := LevelFor(app, user, slug)
	if err != nil {
		// An unreadable override is not permission to proceed.
		return err
	}
	switch level {
	case LevelFull:
		return nil
	case LevelReadonly:
		return apis.NewForbiddenError(
			fmt.Sprintf("Your access to the %s package is read-only.", slug), nil)
	default:
		return apis.NewForbiddenError(
			fmt.Sprintf("You do not have access to the %s package.", slug), nil)
	}
}

// LevelFor resolves the user's level for a package — the same resolution as
// the client's usePkgAccess: owners/admins are always full; a member is full
// unless an override row restricts; a guest is none unless one grants.
//
// A deployment without the org_pkg_access collection at all (a bare test
// fixture, a composition predating package access) has no levels to enforce
// and resolves to full; an existing collection whose QUERY fails is a real
// error and is not permission to proceed.
func LevelFor(app core.App, user *core.Record, slug string) (string, error) {
	role := user.GetString("role")
	if role == "owner" || role == "admin" {
		return LevelFull, nil
	}

	if _, err := app.FindCachedCollectionByNameOrId("org_pkg_access"); err != nil {
		return LevelFull, nil
	}

	rows, err := app.FindRecordsByFilter("org_pkg_access",
		"user = {:user} && pkg = {:pkg}", "", 1, 0,
		map[string]any{"user": user.Id, "pkg": slug})
	if err != nil {
		return "", fmt.Errorf("pkgaccess: read override for %s/%s: %w", user.Id, slug, err)
	}
	if len(rows) > 0 {
		return rows[0].GetString("access"), nil
	}
	if role == "guest" {
		return LevelNone, nil
	}
	return LevelFull, nil
}

// guardWrite is the REST hook body: refuse the write when the collection is
// package-owned and the caller's level for that package is not full.
func guardWrite(app core.App, auth *core.Record, collection string) error {
	if auth == nil || auth.IsSuperuser() {
		// Unauthenticated writes are the collection rules' problem; package
		// access is a per-user overlay on top of them.
		return nil
	}
	slug := ownerSlug(app, collection)
	if slug == "" {
		return nil
	}
	return WriteError(app, auth, slug)
}

// ownerSlug resolves which installed package owns a collection, or "" for
// none. Longest matching slug wins so a package can never shadow another
// whose slug extends its own.
//
// A registry read failure resolves to "no owner" (logged): the collection
// rules remain the primary authorization, and failing closed here would 403
// every write in the deployment over a transient registry error.
func ownerSlug(app core.App, collection string) string {
	pkgs, err := app.FindRecordsByFilter("pkg_registry",
		"status = 'installed' || status = 'bundled'", "", 0, 0)
	if err != nil {
		log.Warn("cannot read pkg_registry; skipping package-access check",
			"collection", collection, "err", err)
		return ""
	}

	best := ""
	for _, pkg := range pkgs {
		slug := pkg.GetString("slug")
		norm := strings.ReplaceAll(slug, "-", "_")
		if norm == "" {
			continue
		}
		if collection == norm || strings.HasPrefix(collection, norm+"_") {
			if len(norm) > len(strings.ReplaceAll(best, "-", "_")) {
				best = slug
			}
		}
	}
	return best
}
