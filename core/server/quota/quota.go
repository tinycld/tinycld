// Package quota is core's shared storage-limit enforcement. It sums the bytes a
// deployment's feature collections hold and refuses writes that would exceed a
// per-user or per-org ceiling, driven by per-package config (materialized from
// each package's manifest `quota` block).
//
// Why it lives in core, and why it is a RECORD HOOK: a quota is an enforcement
// boundary, so it must not be skippable. Binding OnRecordCreate/OnRecordUpdate
// here means every write path is covered by construction — the REST API, WebDAV
// (whose writes go through app.Save), IMAP delivery, a package's own endpoints,
// and anything a future feature adds. A check that lived in one protocol's
// handler would leave the others open.
//
// It also has to be core rather than a feature's own hook because of hosting: a tenant
// process links no feature package, so a limit enforced by a feature's own hook
// would simply not exist there — the org could fill the host's disk. Config
// crosses a process boundary; a Go closure does not. (Same reasoning as the
// access rules in core/webdav.)
//
// What is deliberately NOT enforced: reads and deletes. An org moved to a
// smaller plan must still be able to read its data and delete enough of it to
// get back under the ceiling.
package quota

import (
	"fmt"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Source describes one collection whose rows consume storage.
//
// A source with an OwnerField counts toward both the per-user and the per-org
// total. One without counts only toward the org total — which is the honest
// model for shared data: a message in a mailbox several people share is not
// chargeable to any one of them.
type Source struct {
	// Slug is the owning package slug (e.g. "drive"), for log context.
	Slug string

	// Collection is the PocketBase collection holding the rows.
	Collection string

	// SizeField is the number field holding each row's byte count.
	SizeField string

	// OwnerField is the relation to the users row that owns the bytes
	// (e.g. "created_by"). Empty when the data has no single owner, in which
	// case the source is excluded from per-user totals.
	OwnerField string
}

// Limits are the ceilings in bytes. Zero means unlimited.
type Limits struct {
	// PerUser caps one user's owned bytes across all owned sources.
	PerUser int64

	// PerOrg caps the whole deployment's bytes across every source.
	//
	// Under hosting this is the plan limit the ROUTER assigns, delivered in
	// the org's runtime config — deliberately not something the org's own
	// superuser can raise by editing a settings row.
	PerOrg int64
}

// LimitsFunc resolves the current ceilings. It is called per write, so an
// implementation should be cheap; the settings-backed one is a single indexed
// query and the runtime-config one is a struct read.
type LimitsFunc func(app core.App) Limits

// ExceededError reports a refused write. Callers surface it as HTTP 413.
type ExceededError struct {
	Scope     string // "user" or "organization"
	Used      int64
	Limit     int64
	Requested int64
}

func (e *ExceededError) Error() string {
	available := e.Limit - e.Used
	if available < 0 {
		available = 0
	}
	return fmt.Sprintf("%s storage limit exceeded: %s used of %s limit, %s available but %s requested",
		e.Scope,
		FormatBytes(e.Used), FormatBytes(e.Limit),
		FormatBytes(available), FormatBytes(e.Requested))
}

// FormatBytes renders a byte count for human-facing error messages.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

// The source registry.
//
// A package declares its storage-bearing collections from its own Register(app)
// — the same place it binds every other hook — and core reads the accumulated
// set when it wires enforcement. That ordering works because coreserver calls
// RegisterExtras before quota.Register.
//
// A registry rather than a generator contract, for two reasons: a package that
// is not installed contributes nothing without anyone maintaining a list, and
// the org-wide total is only correct if it spans every installed package, which
// is exactly what the registry accumulates.
var (
	registryMu sync.RWMutex
	registry   []Source
)

// RegisterSources adds a package's sources. Idempotent per (collection,
// sizeField) so a re-registration during dev reload does not double-count.
func RegisterSources(sources ...Source) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, src := range sources {
		dup := false
		for _, existing := range registry {
			if existing.Collection == src.Collection && existing.SizeField == src.SizeField {
				dup = true
				break
			}
		}
		if !dup {
			registry = append(registry, src)
		}
	}
}

// RegisteredSources returns a snapshot of everything declared so far.
func RegisteredSources() []Source {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Source, len(registry))
	copy(out, registry)
	return out
}

// ResetSourcesForTesting clears the registry.
func ResetSourcesForTesting() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}
