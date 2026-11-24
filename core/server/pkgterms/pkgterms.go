// Package pkgterms holds the terms and conditions of packages: which version
// a package declares, which version the owner accepted, and the refusal of
// every request to a package whose current version is not accepted.
//
// A package declares terms in its manifest (`terms`). The generator carries
// the version and the text into bundled-packages.json; the boot sync writes
// the version to pkg_registry.terms_version and the text to pkg_terms (Sync).
// The owner accepts by creating a pkg_terms_decisions row; a hook checks it
// and writes pkg_registry.terms_accepted_version (Register). While the two
// versions differ the package is pending, and SlugError / CollectionError
// refuse it. A package with no terms has a blank terms_version and is never
// pending.
//
// The package imports no core package that asks it (pkgaccess, search,
// reports), so each of them can.
package pkgterms

import (
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

const (
	RegistryCollection  = "pkg_registry"
	TermsCollection     = "pkg_terms"
	DecisionsCollection = "pkg_terms_decisions"

	// FieldTermsVersion is the version the installed package declares.
	FieldTermsVersion = "terms_version"
	// FieldAcceptedVersion is the version the owner accepted.
	FieldAcceptedVersion = "terms_accepted_version"

	DecisionAccepted = "accepted"
	DecisionDeclined = "declined"
)

// Pending is true when the package has terms and the owner has not accepted
// their current version.
func Pending(pkg *core.Record) bool {
	version := pkg.GetString(FieldTermsVersion)
	return version != "" && pkg.GetString(FieldAcceptedVersion) != version
}

// Sync stores the terms text of one package, or removes it when the package
// has none (a blank version). It runs at boot for every bundled package.
//
// A database without the pkg_terms collection can hold no terms: that is an
// error for a package that has them, and nothing to do for one that has none.
func Sync(app core.App, slug, version, body string) error {
	col, err := app.FindCollectionByNameOrId(TermsCollection)
	if err != nil {
		if version == "" {
			return nil
		}
		return fmt.Errorf("pkgterms: package %s has terms but there is no %s collection: %w", slug, TermsCollection, err)
	}
	rows, err := app.FindRecordsByFilter(TermsCollection, "pkg = {:pkg}", "", 1, 0, map[string]any{"pkg": slug})
	if err != nil {
		return fmt.Errorf("pkgterms: read the terms of %s: %w", slug, err)
	}
	if version == "" {
		for _, row := range rows {
			if err := app.Delete(row); err != nil {
				return fmt.Errorf("pkgterms: remove the terms of %s: %w", slug, err)
			}
		}
		return nil
	}
	if body == "" {
		return errors.New("pkgterms: package " + slug + " has a terms version but no terms text")
	}
	row := core.NewRecord(col)
	if len(rows) > 0 {
		row = rows[0]
		if row.GetString("version") == version && row.GetString("body") == body {
			return nil
		}
	}
	row.Set("pkg", slug)
	row.Set("version", version)
	row.Set("body", body)
	if err := app.Save(row); err != nil {
		return fmt.Errorf("pkgterms: store the terms of %s: %w", slug, err)
	}
	return nil
}
