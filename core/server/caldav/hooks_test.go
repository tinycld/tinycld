package caldav

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// fakePoint is a TSHookPoint whose behaviour the test dictates, standing in for
// a compiled JS handler.
type fakePoint struct {
	enabled bool
	value   any
	handled bool
	err     error
	calls   int
	last    map[string]any
}

func (p *fakePoint) Enabled() bool { return p.enabled }

func (p *fakePoint) Call(payload map[string]any) (any, bool, error) {
	p.calls++
	p.last = payload
	return p.value, p.handled, p.err
}

// A disabled point must never be called — that is the whole performance
// contract. An org that customizes nothing must not touch the JS runtime pool.
func TestDisabledPointIsNeverCalled(t *testing.T) {
	p := &fakePoint{enabled: false}
	b := &Backend{src: testSource, ts: TSHooks{BeforeWrite: p, BeforeDelete: p, CanRead: p, FilterList: p}}

	if err := b.hookBeforeWrite("u1", "cal1", "uid1", true); err != nil {
		t.Fatalf("hookBeforeWrite: %v", err)
	}
	if !b.hookCanRead("u1", "cal1", newEventRecord(t, map[string]any{"ical_uid": "uid1"})) {
		t.Error("hookCanRead denied with a disabled point")
	}
	if p.calls != 0 {
		t.Errorf("a disabled point was called %d times, want 0", p.calls)
	}
}

// A nil point (the package registered no hooks at all) behaves the same.
func TestNilPointsAreSkipped(t *testing.T) {
	b := &Backend{src: testSource}

	if err := b.hookBeforeWrite("u1", "cal1", "uid1", true); err != nil {
		t.Fatalf("hookBeforeWrite with nil point: %v", err)
	}
	if err := b.hookBeforeDelete("u1", "cal1", newEventRecord(t, nil)); err != nil {
		t.Fatalf("hookBeforeDelete with nil point: %v", err)
	}
	if !b.hookCanRead("u1", "cal1", newEventRecord(t, nil)) {
		t.Error("hookCanRead with a nil point denied")
	}
}

func TestBeforeWritePayloadShape(t *testing.T) {
	p := &fakePoint{enabled: true}
	b := &Backend{src: testSource, ts: TSHooks{BeforeWrite: p}}

	if err := b.hookBeforeWrite("user-1", "cal-1", "urn:uuid:x", true); err != nil {
		t.Fatalf("hookBeforeWrite: %v", err)
	}
	for k, want := range map[string]any{
		"slug": "calendar", "op": "write", "calendarId": "cal-1",
		"icalUid": "urn:uuid:x", "userId": "user-1", "isCreate": true,
	} {
		if got := p.last[k]; got != want {
			t.Errorf("payload[%q] = %v, want %v", k, got, want)
		}
	}
}

// A thrown handler error is the rejection: that is how a veto point works.
func TestBeforeWriteErrorRejects(t *testing.T) {
	p := &fakePoint{enabled: true, err: errors.New("no writes on Sundays")}
	b := &Backend{src: testSource, ts: TSHooks{BeforeWrite: p}}

	err := b.hookBeforeWrite("u1", "cal1", "uid1", true)
	if err == nil {
		t.Fatal("a throwing beforeWrite handler did not reject the write")
	}
	if !errors.Is(err, p.err) {
		t.Errorf("err = %v, want it to wrap the handler error", err)
	}
}

func TestBeforeDeleteErrorRejects(t *testing.T) {
	p := &fakePoint{enabled: true, err: errors.New("nope")}
	b := &Backend{src: testSource, ts: TSHooks{BeforeDelete: p}}

	if err := b.hookBeforeDelete("u1", "cal1", newEventRecord(t, nil)); err == nil {
		t.Fatal("a throwing beforeDelete handler did not reject the delete")
	}
}

func TestCanReadExplicitFalseDenies(t *testing.T) {
	p := &fakePoint{enabled: true, handled: true, value: false}
	b := &Backend{src: testSource, ts: TSHooks{CanRead: p}}

	if b.hookCanRead("u1", "cal1", newEventRecord(t, nil)) {
		t.Error("canRead returning false did not deny")
	}
}

// A handler that only inspects (returns undefined) must leave Go's decision
// intact rather than being read as a denial.
func TestCanReadUnhandledLeavesGoDecision(t *testing.T) {
	p := &fakePoint{enabled: true, handled: false}
	b := &Backend{src: testSource, ts: TSHooks{CanRead: p}}

	if !b.hookCanRead("u1", "cal1", newEventRecord(t, nil)) {
		t.Error("an inspecting-only canRead handler denied the read")
	}
}

func TestCanReadErrorDenies(t *testing.T) {
	p := &fakePoint{enabled: true, err: errors.New("boom")}
	b := &Backend{src: testSource, ts: TSHooks{CanRead: p}}

	if b.hookCanRead("u1", "cal1", newEventRecord(t, nil)) {
		t.Error("a throwing canRead handler must deny, not fail open")
	}
}

// The security property: filterList may hide, never reveal. A UID the handler
// invents was never authorized by Go and must be discarded.
func TestFilterListCannotWidenTheListing(t *testing.T) {
	records := []*core.Record{
		newEventRecord(t, map[string]any{"ical_uid": "a"}),
		newEventRecord(t, map[string]any{"ical_uid": "b"}),
	}

	p := &fakePoint{enabled: true, handled: true, value: []any{"a", "invented-by-the-handler"}}
	b := &Backend{src: testSource, ts: TSHooks{FilterList: p}}

	kept, err := b.hookFilterList("u1", "cal1", records)
	if err != nil {
		t.Fatalf("hookFilterList: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept %d records, want 1", len(kept))
	}
	if got := kept[0].GetString("ical_uid"); got != "a" {
		t.Errorf("kept %q, want only the authorized 'a'", got)
	}
}

func TestFilterListNarrows(t *testing.T) {
	records := []*core.Record{
		newEventRecord(t, map[string]any{"ical_uid": "a"}),
		newEventRecord(t, map[string]any{"ical_uid": "b"}),
		newEventRecord(t, map[string]any{"ical_uid": "c"}),
	}

	p := &fakePoint{enabled: true, handled: true, value: []any{"a", "c"}}
	b := &Backend{src: testSource, ts: TSHooks{FilterList: p}}

	kept, err := b.hookFilterList("u1", "cal1", records)
	if err != nil {
		t.Fatalf("hookFilterList: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %d records, want 2", len(kept))
	}
}

// A batch is offered in ONE call, not one per entry — that is what makes the
// point affordable on a listing.
func TestFilterListIsBatched(t *testing.T) {
	records := make([]*core.Record, 25)
	for i := range records {
		records[i] = newEventRecord(t, map[string]any{"ical_uid": string(rune('a' + i))})
	}

	p := &fakePoint{enabled: true, handled: true, value: []any{}}
	b := &Backend{src: testSource, ts: TSHooks{FilterList: p}}

	if _, err := b.hookFilterList("u1", "cal1", records); err != nil {
		t.Fatalf("hookFilterList: %v", err)
	}
	if p.calls != 1 {
		t.Errorf("filterList was called %d times for 25 events, want 1", p.calls)
	}
	items, ok := p.last["items"].([]any)
	if !ok || len(items) != 25 {
		t.Errorf("payload items = %v, want all 25 UIDs in one batch", p.last["items"])
	}
}

// A handler returning a non-list is a bug in the hook, but it must not leak the
// unfiltered listing and must not fail the request either.
func TestFilterListNonListReturnKeepsGoAnswer(t *testing.T) {
	env := setupAuthzEnv(t) // needs a real app for the Logger call
	records := []*core.Record{newEventRecord(t, map[string]any{"ical_uid": "a"})}

	p := &fakePoint{enabled: true, handled: true, value: "not a list"}
	env.backend.SetTSHooks(TSHooks{FilterList: p})

	kept, err := env.backend.hookFilterList("u1", "cal1", records)
	if err != nil {
		t.Fatalf("hookFilterList: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("kept %d records, want the Go answer preserved (1)", len(kept))
	}
}

// CanRead applies to listings too, so registering only CanRead still filters a
// PROPFIND rather than only single-object GETs.
func TestFilterListAppliesCanReadPerEntry(t *testing.T) {
	records := []*core.Record{
		newEventRecord(t, map[string]any{"ical_uid": "a"}),
		newEventRecord(t, map[string]any{"ical_uid": "b"}),
	}

	denyAll := &fakePoint{enabled: true, handled: true, value: false}
	b := &Backend{src: testSource, ts: TSHooks{CanRead: denyAll}}

	kept, err := b.hookFilterList("u1", "cal1", records)
	if err != nil {
		t.Fatalf("hookFilterList: %v", err)
	}
	if len(kept) != 0 {
		t.Errorf("kept %d records, want 0 — canRead must filter listings", len(kept))
	}
	if denyAll.calls != 2 {
		t.Errorf("canRead called %d times, want once per entry (2)", denyAll.calls)
	}
}

func TestFilterListErrorPropagates(t *testing.T) {
	records := []*core.Record{newEventRecord(t, map[string]any{"ical_uid": "a"})}
	p := &fakePoint{enabled: true, err: errors.New("boom")}
	b := &Backend{src: testSource, ts: TSHooks{FilterList: p}}

	if _, err := b.hookFilterList("u1", "cal1", records); err == nil {
		t.Fatal("a throwing filterList handler did not fail the listing")
	}
}

// --- the JS registration surface -------------------------------------------

func TestPointNamesAreSlugNamespaced(t *testing.T) {
	if got := pointName("calendar", "beforeWrite"); got != "caldav.calendar.beforeWrite" {
		t.Errorf("pointName = %q", got)
	}
	// Two packages serving calendars must not share a handler.
	if pointName("calendar", "canRead") == pointName("rooms", "canRead") {
		t.Error("hook points collide across source slugs")
	}
}

// Method shorthand stringifies to a method DEFINITION, which is not an
// expression and fails to compile standalone. This is the drive-migration bug
// that unit tests missed.
func TestNormalizeHandlerSourceFixesMethodShorthand(t *testing.T) {
	got := normalizeHandlerSource("beforeWrite", "beforeWrite(e) { return true }")
	if got != "function beforeWrite(e) { return true }" {
		t.Errorf("shorthand not normalized: %q", got)
	}
}

func TestNormalizeHandlerSourceLeavesValidExpressions(t *testing.T) {
	for _, src := range []string{
		"function (e) { return true }",
		"(e) => { return true }",
		"e => true",
	} {
		if got := normalizeHandlerSource("beforeWrite", src); got != src {
			t.Errorf("normalizeHandlerSource rewrote a valid expression %q → %q", src, got)
		}
	}
}

func TestIsKnownHookName(t *testing.T) {
	for _, name := range []string{"beforeWrite", "beforeDelete", "canRead", "filterList"} {
		if !isKnownHookName(name) {
			t.Errorf("%q should be a known hook", name)
		}
	}
	// beforeMove belongs to WebDAV, not here — CalDAV has no cross-calendar MOVE.
	for _, name := range []string{"beforeMove", "beforeRead", "", "canWrite"} {
		if isKnownHookName(name) {
			t.Errorf("%q should not be a known hook", name)
		}
	}
}
