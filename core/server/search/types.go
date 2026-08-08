// Package search federates one query across every installed package.
//
// The palette used to fan out from the browser — one request per package — and
// normalize each response with a TypeScript adapter in that package's repo. A
// Go CLI cannot import those adapters, so a terminal search would have had to
// reimplement every package's field mapping in Go, with nothing spanning the
// two copies to keep them honest. Normalizing here instead means the mapping
// exists once and both clients read the same rows.
package search

// Row is one normalized hit. The display fields mirror the web's SearchRow so
// the palette can render a row without knowing which package produced it.
type Row struct {
	// Slug names the producing package. Set by the aggregator, not the source,
	// so a source cannot mislabel its own rows.
	Slug string `json:"slug"`
	// ID is the record id within that package, and is what a client hands back
	// to the package's own selection handler.
	ID string `json:"id"`
	// Title is the name a user would recognize — subject, file name, card
	// title. Sources substitute their own placeholder rather than leaving it
	// empty, so a row is never unclickable.
	Title string `json:"title"`
	// Subtitle is identifying detail: participants, a match snippet, an email.
	Subtitle string `json:"subtitle,omitempty"`
	// Meta is trailing detail, usually a date or container name.
	Meta string `json:"meta,omitempty"`
	// Fields carries package-specific values a display row has no place for —
	// mailbox_id, mime_type, is_folder — so a scripted `--json` caller can
	// filter on them. Values are limited to string, number and bool: anything
	// richer would make the contract a per-package shape again, which is what
	// this package exists to avoid.
	Fields map[string]any `json:"fields,omitempty"`
}

// Query is a parsed search. The client owns the grammar (`pkg:` chips and
// `-term` exclusions); by the time it reaches here the terms are already split.
type Query struct {
	// Include terms must match. An empty Include yields no results: FTS5
	// cannot subtract from nothing, so an exclude-only query has no meaning.
	Include []string
	// Exclude terms must not match.
	Exclude []string
	// Slugs restricts the search to these packages. Empty means every
	// registered source the caller may see.
	Slugs []string
	// Limit and Offset page the MERGED result set. Each source is asked for
	// more than Limit so cross-package ranking has something to choose from —
	// see sourceLimit.
	Limit  int
	Offset int
}

// Result is what one source returns. Total is that package's full match count,
// which can exceed len(Rows) when the source was asked for a bounded slice.
type Result struct {
	Rows  []Row
	Total int
}

// Response is the aggregated answer.
type Response struct {
	// Rows is the merged, scored, ordered slice.
	Rows []Row `json:"rows"`
	// Counts maps slug to that package's total match count, so a client can
	// say "12 in Mail" without counting rows it was not sent.
	Counts map[string]int `json:"counts"`
	// Partial names packages that failed or timed out. They contribute no rows,
	// and callers are expected to say so: a search that silently dropped a
	// package reads identically to one that found nothing there, which is the
	// worse of the two lies.
	Partial []string `json:"partial,omitempty"`
	// Truncated names packages with more matches than were returned, so a
	// client can offer "see all in Mail" rather than implying completeness.
	Truncated []string `json:"truncated,omitempty"`
}
