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
// the index-sync hooks (Table + Columns) and the search route (Scope +
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

	// Scope constrains results to rows the caller may see.
	Scope Scope

	// ExcludeField, when set, drops rows whose BOOL field is true (e.g.
	// boards' `archived`).
	//
	// Deliberately distinct from SoftDeleteField: that one splits on
	// `field = ''` vs `!= ''`, which is correct for a TEXT timestamp but
	// misbehaves against a bool column under SQLite's loose typing. Two
	// mechanisms, two column types — do not conflate them.
	ExcludeField string

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

// Scope constrains search results to rows the requesting user may see. It is an
// interface because ownership is not uniform: some collections hold the owner's
// id directly, others grant access through a membership table.
type Scope interface {
	// clause returns a SQL fragment ANDed into the WHERE. Identifiers come
	// from config (trusted); the user id is always a bound parameter.
	clause() string
	params(userID string) map[string]any
}

// OwnerScope resolves ownership through a single relation field holding the
// user's id directly (single-org: the former user_org junction is gone).
type OwnerScope struct {
	// Field is the collection field holding the owner reference.
	Field string
}

func (s OwnerScope) clause() string {
	return "c." + s.Field + " IN ({:scopeUser})"
}

func (s OwnerScope) params(userID string) map[string]any {
	return map[string]any{"scopeUser": userID}
}

// MemberScope resolves access through a membership table: the record is visible
// when the user holds a row granting them its parent. Emitted as a live
// subquery rather than a cached grant, so removing a member takes effect on the
// next search.
type MemberScope struct {
	// Table is the membership collection (e.g. "boards_project_members").
	Table string
	// MemberField is the column in Table pointing at the parent record.
	MemberField string
	// UserField is the column in Table pointing at the user.
	UserField string
	// RecordField is the column on the SEARCHED collection pointing at the
	// same parent.
	RecordField string
}

func (s MemberScope) clause() string {
	return "c." + s.RecordField + " IN (SELECT " + s.MemberField +
		" FROM " + s.Table + " WHERE " + s.UserField + " = {:scopeUser})"
}

func (s MemberScope) params(userID string) map[string]any {
	return map[string]any{"scopeUser": userID}
}
