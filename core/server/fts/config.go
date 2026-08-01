// Package fts is core's shared full-text-search capability. It owns the raw
// SQLite FTS5 index sync + query for any feature collection, driven by
// per-package config (materialized from each package's manifest `fts` block).
//
// Why it lives in core, not per-feature: a tenant process links no feature
// package, so index-sync hooks registered by a feature would simply not run
// there and the index would silently rot. Keeping it here also means the raw
// SQL surface never crosses the untrusted tenant-TS boundary — packages
// declare an `fts` config block and
// core registers the index-sync record hooks + the search route itself. The
// `$fts` JS binding (bindings.go) exists for the rare package that must query
// imperatively from TS, but the data-plane path is pure core Go.
//
// The FTS virtual table itself is created by the package's JS pb-migrations
// (the schema source of truth); this package only reads and writes it.
package fts

// Config describes one feature collection's FTS index. One Config drives both
// the index-sync hooks (Table + Columns) and the search route (Route + Owner +
// Output). It is the single interface both the manifest reader and the direct
// wiring use.
type Config struct {
	// Slug is the owning package slug (e.g. "contacts") — used for the
	// generated search route path /api/{slug}/search and for log context.
	Slug string

	// Collection is the PocketBase collection whose records are indexed
	// (e.g. "contacts").
	Collection string

	// Table is the FTS5 virtual table name (e.g. "fts_contacts"), created by
	// the package's JS migration.
	Table string

	// Columns maps each FTS5 column to the source record field. Order is not
	// significant. The FTS row's `record_id` is always the record Id and is
	// not listed here. A column whose Strip is true has HTML tags removed
	// before indexing (editor fields).
	Columns []Column

	// Owner scopes search to the records owned by the requesting user.
	Owner OwnerScope

	// Output lists the collection columns the search route returns per hit,
	// in addition to the record id. These are read straight from the joined
	// collection row (never from the FTS table), so they are safe raw values.
	Output []OutputColumn

	// SoftDeleteField, when set, is the collection field the search route uses
	// to split live vs. deleted results (empty string = live). Mirrors each
	// feature's soft-delete convention. Empty disables the split.
	SoftDeleteField string
}

// OutputColumn is one column the search route returns, with the JSON type it
// should be coerced to so the response shape matches the collection schema (e.g.
// a `favorite` bool must not come back as the string "0", which is truthy in JS).
type OutputColumn struct {
	// Name is the collection column (and the JSON key).
	Name string
	// Type coerces the raw string value: "" or "string" (default), "bool", "number".
	Type string
}

// Column binds an FTS5 column to a record field.
type Column struct {
	// FTS is the FTS5 column name (must match the CREATE VIRTUAL TABLE).
	FTS string
	// Field is the source record field name.
	Field string
	// Strip removes HTML tags before indexing (for editor/rich-text fields).
	Strip bool
}

// OwnerScope declares how a record's owner resolves to the requesting user, so
// search results stay scoped to the caller's own records. Single-org: the owner
// field holds a users id directly (the former user_org junction is gone), so
// search is scoped to owner == the authenticated user's id.
type OwnerScope struct {
	// Field is the collection field holding the owner reference
	// (e.g. "owner", a relation to users).
	Field string
}
