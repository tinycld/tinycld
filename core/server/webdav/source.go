// Package webdav is core's shared WebDAV server. It serves the WebDAV/RFC-4918
// protocol (PROPFIND/PUT/MKCOL/MOVE/DELETE, locking, Basic-Auth) over any
// feature collection shaped as a file tree, driven by per-package config (a
// Source, materialized from each package's manifest `webdav` block).
//
// Why it lives in core, not per-feature: in the multi-org model a tenant runs
// stock PocketBase with no feature Go, so the protocol server must be trusted
// core code fed by materialized config, never feature Go with full $app reach.
// Core imports no feature package.
//
// What a package contributes:
//
//   - Data (this Source): which collection backs the tree and which of its
//     fields carry name/parent/size/…. This covers the whole protocol surface.
//   - Go callbacks (Hooks): the little that is neither config nor an
//     enforcement boundary — currently just the version snapshot on overwrite.
//     Authorization comes from the collection's PB rules and quota from
//     core/quota, both of which a tenant process gets too.
//   - Opt-in TS hook points, for behaviour an org wants to customize without
//     writing Go. See hooks.go; the fast path never touches a JS VM.
//
// Where it runs: in the single-tenant app, in-process. Under multi-org's
// per-process tenant isolation, inside each org's own process — the router
// materializes the Sources and reverse-proxies to that process.
package webdav

import (
	"github.com/pocketbase/pocketbase/core"
)

// Source describes one feature collection served as a WebDAV file tree.
//
// The field names are data because the protocol only ever needs to read a
// name, a size, a parent pointer and a blob — nothing about what the feature
// means by them. The feature-specific questions — may this user read this row,
// does this write fit the quota — are answered by the collection's own access
// rules and by core/quota, not here.
type Source struct {
	// Slug is the owning package slug (e.g. "drive"), for log context.
	Slug string

	// Prefix is the URL path the tree is mounted at, without a trailing
	// slash (e.g. "/drive"). Also the first path segment clients see.
	Prefix string

	// Collection is the PocketBase collection backing the tree
	// (e.g. "drive_items").
	Collection string

	// Fields maps the tree's structural roles onto collection fields.
	Fields FieldMap

	// Hooks are the optional feature side effects. A nil hook means "no extra
	// work"; it can no longer mean "no restriction", since nothing that
	// restricts lives here any more.
	Hooks Hooks
}

// FieldMap binds tree semantics to collection field names.
type FieldMap struct {
	// Name is the field holding the entry's basename (e.g. "name").
	Name string

	// Parent is the self-relation to the containing folder; empty string
	// value means a root-level entry (e.g. "parent").
	Parent string

	// IsFolder is the bool field marking directories (e.g. "is_folder").
	IsFolder string

	// Size is the number field holding the blob size in bytes (e.g. "size").
	Size string

	// MimeType is the text field holding the content type (e.g. "mime_type").
	// Optional: when empty, no MIME is stamped on create.
	MimeType string

	// File is the file field holding the blob (e.g. "file").
	File string

	// Owner is the relation to the users row that created the entry
	// (e.g. "created_by"). Stamped on every create.
	Owner string

	// Updated is the autodate field surfaced as the entry's mtime
	// (e.g. "updated"). Optional: when empty, mtime is the zero time.
	Updated string
}

// Hooks are the per-feature side effects the protocol defers to the package.
//
// Authorization is deliberately NOT here. WebDAV evaluates the collection's own
// PocketBase access rules (List/View/Update/Delete) via app.CanAccessRecord, so
// there is exactly one place a tree's permissions are defined — the migration —
// and the DAV path cannot drift from what the REST API and the web UI enforce.
//
// It also has to be that way for multi-org. A rule is a string and travels in
// the schema; a Go closure cannot cross a process boundary. Were authorization
// a hook, a tenant process (which links no feature Go) would serve the tree with
// no per-record checks at all.
//
// What remains here is the work that genuinely is not an access decision.
//
// Storage quota is NOT here either, and for the same reason: it is an
// enforcement boundary, so it lives in core/quota as a record hook that every
// write path — including this one, whose writes go through app.Save — passes
// through by construction.
//
// NOTE: BeforeOverwrite is nil in a tenant process, so a tenant-served write
// does not archive the previous version. Tracked in the router's HANDOFF.
type Hooks struct {
	// BeforeOverwrite runs just before an existing entry's blob is replaced —
	// drive uses it to snapshot the outgoing version. An error here is logged,
	// not fatal: failing to archive the old blob must not lose the new write.
	BeforeOverwrite func(app core.App, userID string, record *core.Record) error
}
