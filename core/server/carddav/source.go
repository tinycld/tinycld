// Package carddav is core's shared CardDAV server. It serves the CardDAV/RFC-6352
// protocol (PROPFIND/REPORT, ETags, Basic-Auth, vCard encode/decode) for any
// feature collection, driven by per-package config (a Source, materialized from
// each package's manifest `carddav` block).
//
// Why it lives in core, not per-feature: CardDAV is a protocol server, and a
// protocol server has to be reachable on a port. The hosting router owns
// every listening socket — a tenant serves on a unix socket handed down to it —
// so anything that must be *bound* belongs to core, where the router can open
// it. That is the rule; it is about ports, not about Go being unwelcome.
//
// A second, independent fact shapes the config seam: `serve-org` links no
// feature package today, so a Source cannot be a Go literal registered by the
// feature — it arrives as data the router materialized from the manifest.
// Core imports no feature package; a package contributes the Source plus, for
// anything a field map can't express, TS record hooks. The vCard codec stays
// Go here — performance-sensitive work belongs in Go — and only the field map
// is data.
//
// Where it runs: in the single-tenant app, in-process. Under hosting's
// per-process tenant isolation, inside each org's own process — the router
// materializes the Sources and reverse-proxies to that process, and never holds
// a tenant app object itself.
package carddav

// Source describes one feature collection served over CardDAV. One Source maps a
// PocketBase collection to a vCard address book, scoped per org.
type Source struct {
	// Slug is the owning package slug (e.g. "contacts"), for log context.
	Slug string

	// Collection is the PocketBase collection served as vCards (e.g. "contacts").
	Collection string

	// ListFilter is the PB filter selecting a book's objects, with {:ownerId}
	// bound to the resolved owner (users) id — e.g.
	// `owner = {:ownerId} && deleted_at = ''`.
	ListFilter string

	// Sort is the PB sort expression for listings (e.g. "-updated").
	Sort string

	// OwnerField is the collection field holding the owner reference
	// (a relation to a users row), e.g. "owner".
	OwnerField string

	// UIDField is the collection field holding the vCard UID (e.g. "vcard_uid").
	UIDField string

	// SoftDeleteField, when set, is the field a DELETE stamps (with the current
	// time) instead of hard-deleting, mirroring the web UI. Empty = hard delete.
	SoftDeleteField string

	// VCard is the declarative record↔vCard field map.
	VCard VCardMap
}

// VCardMap declares how record fields translate to/from vCard properties. It
// covers the common contact shape declaratively; the codec composes FN from the
// name parts and formats REV from the record's updated timestamp.
type VCardMap struct {
	// Version is the vCard version string emitted (e.g. "4.0").
	Version string

	// Name maps the structured N property. Given/Family are record field names.
	Name NameMap

	// Simple maps a single record field to a single-value vCard property. The
	// key is the vCard property name (EMAIL, TEL, ORG, TITLE, NOTE, …); the
	// value is the record field.
	Simple map[string]string

	// RevField is the record field (a datetime) emitted as the REV property.
	RevField string

	// UIDField is the record field emitted as the UID property (e.g.
	// "vcard_uid"). It duplicates Source.UIDField because the codec receives
	// only a VCardMap, never the Source.
	//
	// CardDAV itself does not need UID in the card body — it addresses objects
	// by URL path (backend.go builds `<book>/<uid>.vcf`). A vCard *file* has no
	// path, so without this a file export carries no identity and re-importing
	// it duplicates every record instead of matching. UID is a mandatory
	// property in vCard 4.0 (RFC 6350), so emitting it is also more correct.
	UIDField string
}

// NameMap binds the vCard N (and composed FN) to record name fields.
type NameMap struct {
	Given  string // record field for the given/first name
	Family string // record field for the family/last name
}
