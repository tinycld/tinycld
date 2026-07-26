package caldav

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// The opt-in TS hook points.
//
// Everything below is gated on TSHooks.<point>.Enabled() — one atomic load —
// before any payload is built or any VM is borrowed. An org that registers no
// handlers runs the pure-Go path and never touches the JS runtime pool. That is
// deliberate: the pool falls back to constructing a whole new sobek.Runtime
// once every executor is busy, which is exactly what a PROPFIND/REPORT fan-out
// over a calendar would trigger. Orgs that customize pay for it; orgs that
// don't pay nothing.
//
// Wiring is injected rather than imported: core/caldav does not depend on
// coreserver (which would be an import cycle, since coreserver composes the
// app). The host sets TSHooks on the Backend; see coreserver's registration.

// TSHookPoint is the subset of coreserver.HookPoint this package needs. Keeping
// it an interface here inverts the dependency — coreserver knows about caldav,
// not the reverse.
type TSHookPoint interface {
	Enabled() bool
	Call(payload map[string]any) (value any, handled bool, err error)
}

// TSHooks are the optional TS interception points, in the order a request
// encounters them. A nil point is simply skipped.
//
// There is no BeforeMove: this backend has no cross-calendar MOVE — a client
// relocating an event PUTs it to the new calendar and DELETEs the old object,
// which the write and delete points already cover.
type TSHooks struct {
	// BeforeWrite fires on PUT before the event is created or updated. A thrown
	// error rejects the operation.
	BeforeWrite TSHookPoint

	// BeforeDelete fires on DELETE. A thrown error rejects the operation.
	BeforeDelete TSHookPoint

	// CanRead is consulted per event AFTER Go's own authorization has already
	// approved it. It can only narrow the result — returning true for an event
	// Go denied has no effect, because Go's check runs first and short-circuits.
	CanRead TSHookPoint

	// FilterList fires once per calendar listing with the whole batch of
	// already-authorized events, and may return a subset. Batching is what keeps
	// this affordable: one VM borrow per calendar, not one per event.
	FilterList TSHookPoint
}

// SetTSHooks installs the TS hook points on the backend. Call before serving.
func (b *Backend) SetTSHooks(h TSHooks) { b.ts = h }

// hookBeforeWrite runs the BeforeWrite point. A handler error rejects the PUT.
func (b *Backend) hookBeforeWrite(userID, calendarID, icalUID string, isCreate bool) error {
	hp := b.ts.BeforeWrite
	if hp == nil || !hp.Enabled() {
		return nil
	}
	_, _, err := hp.Call(map[string]any{
		"slug":       b.src.Slug,
		"op":         "write",
		"calendarId": calendarID,
		"icalUid":    icalUID,
		"userId":     userID,
		"isCreate":   isCreate,
	})
	if err != nil {
		return rejected(err)
	}
	return nil
}

// hookBeforeDelete runs the BeforeDelete point. A handler error rejects the
// DELETE.
func (b *Backend) hookBeforeDelete(userID, calendarID string, record *core.Record) error {
	hp := b.ts.BeforeDelete
	if hp == nil || !hp.Enabled() {
		return nil
	}
	_, _, err := hp.Call(map[string]any{
		"slug":       b.src.Slug,
		"op":         "delete",
		"id":         record.Id,
		"calendarId": calendarID,
		"icalUid":    record.GetString(b.src.Event.UID),
		"userId":     userID,
	})
	if err != nil {
		return rejected(err)
	}
	return nil
}

// hookCanRead narrows an already-granted read. It is called ONLY for events Go
// has already authorized, so it can restrict and never widen — a handler
// returning true for a denied event is never even consulted.
//
// A handler error, or an explicit false, denies. Any other return (including
// undefined, from a handler that only inspects) leaves the Go decision intact.
func (b *Backend) hookCanRead(userID, calendarID string, record *core.Record) bool {
	hp := b.ts.CanRead
	if hp == nil || !hp.Enabled() {
		return true
	}
	v, handled, err := hp.Call(map[string]any{
		"slug":       b.src.Slug,
		"id":         record.Id,
		"calendarId": calendarID,
		"icalUid":    record.GetString(b.src.Event.UID),
		"userId":     userID,
	})
	if err != nil {
		return false
	}
	if !handled {
		return true
	}
	if allowed, ok := v.(bool); ok {
		return allowed
	}
	return true
}

// hookFilterList offers the whole already-authorized batch to TS and accepts a
// returned subset. UIDs the handler adds that were not in the input are
// discarded — the point can hide, never reveal.
func (b *Backend) hookFilterList(userID, calendarID string, records []*core.Record) ([]*core.Record, error) {
	// Per-event CanRead first, so a caller registering only CanRead still gets
	// it applied to listings.
	if hp := b.ts.CanRead; hp != nil && hp.Enabled() {
		kept := records[:0]
		for _, record := range records {
			if b.hookCanRead(userID, calendarID, record) {
				kept = append(kept, record)
			}
		}
		records = kept
	}

	hp := b.ts.FilterList
	if hp == nil || !hp.Enabled() {
		return records, nil
	}

	uidField := b.src.Event.UID
	uids := make([]any, len(records))
	for i, record := range records {
		uids[i] = record.GetString(uidField)
	}

	v, handled, err := hp.Call(map[string]any{
		"slug":       b.src.Slug,
		"calendarId": calendarID,
		"userId":     userID,
		"items":      uids,
	})
	if err != nil {
		return nil, rejected(err)
	}
	if !handled || v == nil {
		return records, nil
	}

	returned, ok := v.([]any)
	if !ok {
		// A handler that returned something other than a list is a bug in the
		// hook, not a reason to leak the unfiltered listing — but neither is it
		// a reason to fail the request. Keep the Go answer and say so.
		b.app.Logger().Warn("caldav: filterList handler returned a non-list; ignoring",
			"slug", b.src.Slug, "type", fmt.Sprintf("%T", v))
		return records, nil
	}

	allowed := make(map[string]bool, len(returned))
	for _, u := range returned {
		if s, ok := u.(string); ok {
			allowed[s] = true
		}
	}

	// Intersect rather than trust: a UID the handler invented was never
	// authorized by Go and must not appear.
	kept := make([]*core.Record, 0, len(records))
	for _, record := range records {
		if allowed[record.GetString(uidField)] {
			kept = append(kept, record)
		}
	}
	return kept, nil
}

// rejected wraps a TS veto so the CalDAV layer surfaces it as a refusal rather
// than an internal error, preserving the handler's message.
func rejected(err error) error {
	return fmt.Errorf("rejected by hook: %w", err)
}
