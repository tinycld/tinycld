// Package caldav is core's shared CalDAV server. It serves the CalDAV/RFC-4791
// protocol (PROPFIND/REPORT/PUT/GET/DELETE over calendar collections and
// calendar objects, Basic-Auth) over any feature collection pair shaped as
// "calendars containing events", driven by per-package config (a Source,
// materialized from each package's manifest `caldav` block).
//
// Why it lives in core, not per-feature: a protocol server has to be reachable
// on a port, and the hosting router owns every listening socket — a tenant
// serves on a unix socket handed down to it. Anything that must be *bound*
// therefore belongs to core, where the router can open it. The rule is about
// ports, not about Go: performance-sensitive work belongs in Go, and this
// package is Go.
//
// Separately, `serve-org` links no feature package today, so the Source cannot
// be a Go literal the feature registers — it arrives as data the router
// materialized from the manifest. Core imports no feature package.
//
// What a package contributes:
//
//   - Data (this Source): which collections back the calendar list and the
//     event list, and which of their fields carry title/start/end/RRULE/… .
//     This covers the whole protocol surface.
//   - Opt-in TS hook points, for behaviour an org wants to customize without
//     writing Go. See hooks.go; the fast path never touches a JS VM.
//
// Notably there are no Go permission callbacks. Authorization comes from the
// collections' own PocketBase rules, evaluated via app.CanAccessRecord — see
// backend.go. A Go closure cannot cross a process boundary, so anything a
// tenant must enforce has to be expressible as data, and a rule is a string
// that travels in the schema.
//
// Where it runs: in the single-tenant app, in-process. Under hosting's
// per-process tenant isolation, inside each org's own process — the router
// materializes the Sources and reverse-proxies to that process.
package caldav

import "context"

// Source describes one feature's calendar tree served over CalDAV.
//
// The field names are data because the protocol only ever needs to read a
// title, a time range, a recurrence rule and a UID — nothing about what the
// feature means by them. The feature-specific question — may this user read or
// write this row — is answered by the collections' own access rules, not here.
type Source struct {
	// Slug is the owning package slug (e.g. "calendar"), for log context and
	// for namespacing this source's TS hook points.
	Slug string

	// Prefix is the URL path the tree is mounted at, without a trailing slash
	// (e.g. "/caldav"). Also the first path segment clients see.
	Prefix string

	// CalendarCollection is the PocketBase collection holding one row per
	// calendar (e.g. "calendar_calendars").
	CalendarCollection string

	// EventCollection is the PocketBase collection holding one row per event
	// (e.g. "calendar_events").
	EventCollection string

	// Calendar maps the calendar-list semantics onto CalendarCollection fields.
	Calendar CalendarMap

	// Event maps the VEVENT semantics onto EventCollection fields.
	Event EventMap

	// OnError, when set, is called with every non-nil error a backend method
	// returns, tagged with the method name.
	//
	// It exists because go-webdav's caldav.Handler turns a backend error into
	// an http.Error response and then returns nil, so the error never reaches
	// request middleware — this is the only seam where the real error and its
	// stack are still available. The single-tenant app points it at Sentry.
	//
	// Purely observational: it cannot alter the outcome. A tenant process may
	// leave it nil, which costs reporting, not correctness.
	OnError func(ctx context.Context, op string, err error)
}

// CalendarMap binds calendar-collection semantics to field names.
type CalendarMap struct {
	// Name is the field holding the calendar's display name (e.g. "name").
	Name string

	// Description is the field holding its description (e.g. "description").
	// Optional: when empty, no description is advertised.
	Description string
}

// EventMap binds VEVENT semantics to event-collection field names.
//
// Only Calendar, UID, Start and End are required; an empty name means the
// property is simply not mapped in either direction, which is what lets a
// package adopt CalDAV before it has every field.
type EventMap struct {
	// Calendar is the relation from an event to its calendar (e.g. "calendar").
	Calendar string

	// UID is the field holding the stable iCalendar UID (e.g. "ical_uid").
	// Object paths are built from it, so it must be unique within a calendar.
	UID string

	// Owner is the relation to the users row that created the event
	// (e.g. "created_by"). Stamped on every create.
	Owner string

	// Title maps to SUMMARY (e.g. "title").
	Title string

	// Description maps to DESCRIPTION (e.g. "description").
	Description string

	// Location maps to LOCATION (e.g. "location").
	Location string

	// Start maps to DTSTART (e.g. "start").
	Start string

	// End maps to DTEND (e.g. "end").
	End string

	// AllDay is the bool field selecting DATE vs DATE-TIME value type
	// (e.g. "all_day").
	AllDay string

	// Recurrence maps to RRULE (e.g. "recurrence"). Simple tokens
	// ("daily"/"weekly"/…) are translated; anything else round-trips verbatim.
	Recurrence string

	// Guests is the json field holding the attendee list, mapped to ATTENDEE
	// (e.g. "guests"). Entries carry name/email/rsvp/role.
	Guests string

	// Reminder is the number field holding minutes-before, mapped to a
	// DISPLAY VALARM with a negative TRIGGER (e.g. "reminder").
	Reminder string

	// BusyStatus maps to TRANSP: the value "free" becomes TRANSPARENT and
	// anything else OPAQUE (e.g. "busy_status").
	BusyStatus string

	// Visibility maps to CLASS (e.g. "visibility"). The value "default" is
	// omitted rather than emitted as CLASS:DEFAULT.
	Visibility string

	// Updated is the autodate field surfaced as LAST-MODIFIED/DTSTAMP and as
	// the object's ETag (e.g. "updated").
	Updated string

	// Created is the autodate field surfaced as CREATED (e.g. "created").
	Created string

	// Defaults are field values stamped on a newly created event before the
	// VEVENT is applied, for fields the schema requires but iCalendar may omit.
	//
	// This has to be data rather than a Go hook: a required select with no
	// schema default (calendar's busy_status and visibility are both) rejects
	// the save outright when a client PUTs a minimal VEVENT carrying neither
	// TRANSP nor CLASS. A tenant process links no feature package, so a callback
	// could not supply them there — the write would simply fail.
	//
	// Applied before the codec, so anything the VEVENT does specify wins.
	Defaults map[string]any
}

// eventSort is the order events are listed in. Newest-modified first matches
// what clients expect from a collection they are syncing.
func (s Source) eventSort() string {
	if s.Event.Updated == "" {
		return ""
	}
	return "-" + s.Event.Updated
}
