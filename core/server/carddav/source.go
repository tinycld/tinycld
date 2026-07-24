// Package carddav is core's shared CardDAV server. It serves the CardDAV/RFC-6352
// protocol (PROPFIND/REPORT, ETags, Basic-Auth, vCard encode/decode) for any
// feature collection, driven by per-package config (a Source, materialized from
// each package's manifest `carddav` block).
//
// Why it lives in core, not per-feature: in the multi-org model a tenant runs
// stock PocketBase with no feature Go, so the protocol server and its per-feature
// record↔vCard mapping must run host-side in the trusted core process, fed by
// materialized config. Core imports no feature package; a package contributes
// only data (the Source) plus, for anything a field map can't express, TS record
// hooks. The vCard codec stays Go here — only the field map is data.
package carddav

// Source describes one feature collection served over CardDAV. One Source maps a
// PocketBase collection to a vCard address book, scoped per org.
type Source struct {
	// Slug is the owning package slug (e.g. "contacts"), for log context.
	Slug string

	// Collection is the PocketBase collection served as vCards (e.g. "contacts").
	Collection string

	// ListFilter is the PB filter selecting a book's objects, with {:ownerId}
	// bound to the resolved owner (user_org) id — e.g.
	// `owner = {:ownerId} && deleted_at = ''`.
	ListFilter string

	// Sort is the PB sort expression for listings (e.g. "-updated").
	Sort string

	// OwnerField is the collection field holding the owner reference
	// (a relation to a user_org row), e.g. "owner".
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
}

// NameMap binds the vCard N (and composed FN) to record name fields.
type NameMap struct {
	Given  string // record field for the given/first name
	Family string // record field for the family/last name
}
