# Report zone times and the record panel — Part 1 (Tasks 1–6, with 4b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The report engine reads a `Select` field over a text column and a source over a view collection (T3); a report cell, row label, option and chip of a date-time shows day and time in the reader's zone and the document header names the zone (T1); a date range or a day comparison over a date-time (with a day param, a range's bound or a day typed in a condition, Q3) keeps the instants of the reader's local days, in a run and in param options (T2); a package declares `reportPanels` in its manifest and the generator writes `lib/generated/report-panels.ts` (D5, first half).

**Architecture:** See the outline `docs/plans/2026-11-15-report-zone-times-and-record-panel.md` (Architecture, Names changed, Rulings R1–R19). T3 is one entry of `check.go`'s `compatible` map, proven over a `zoo_*` view. T1 is a new pure module `core/reports/lib/date-time.ts` (Intl for the wall-clock numbers, core's own month words) that `format.ts` re-exports and calls from `cellDisplay`. T2 is a new engine file `zone.go` (the zone, a zoned day, the zoned comparisons) that `params.go`, `formula.go` and `run.go` call (Task 4); Task 4b zones a day typed in a condition through a new `fctx` method, `readerDay` (Q3); the client adds `timeZone` to the run and param-options bodies when it sends them. D5's generator is `scripts/gen-report-panels.ts`, called from `generate.ts`.

**Tech Stack:** Go 1.26 (PocketBase, SQLite, `time`, `time/tzdata`), TypeScript (React Native + web, Hermes `Intl`), vitest.

**Spec:** `/Users/nas/tmp/tinycld/books-reports/books/docs/specs/audit-log-design.md` §4 (T1, T2, T3, D5). Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-audit-log/cp/cp0.md`. Outline (source of truth for names and rulings): `docs/plans/2026-11-15-report-zone-times-and-record-panel.md`.

Paths are relative to `/Users/nas/tmp/tinycld/books-reports/tinycld` (core repo, branch `books/p5-audit-log`). Go tests: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run '<Regex>' -count=1`. Vitest, from `/Users/nas/tmp/tinycld/books-reports/tinycld`: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run <file>`. Typecheck, from the same directory: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck`. Biome on your own files only: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check <files>`.

## Corrections to the outline (all applied to the outline)

Each item below was found while this part was written. Each one is now applied to the outline (Architecture, Names changed, Global constraints, the task summaries, File structure, Rulings), so the outline and this part name the same things. The items stay here as the record of why.

- **C1 — Task 2 also changes `core/reports/lib/column-measure.ts`.** Its worst case for a `datetime` column and a `datetime` row label is `shortDay('2026-09-30')` (lines 79–82, 226–228). A date-time is a figure type (never cut, line 63), so a column sized by a short first page would clip a later "Sep 30, 2026, 12:59 PM". Task 2 adds `WORST_DATE_TIME` and a test in `column-measure.test.ts`. *Applied to the outline.*
- **C2 — a date range's `expr` carries `zonedFrom` and `zonedTo`, not one `zoned` span.** `bound` must give each end its own whole day: `gt(x, {param period, bound from})` is `x >= from.end`, which a `{fromZ.start, toZ.end}` span cannot give. So `expr` gains `zoned` (a day: a date or asOf param, or a bound) and `zonedFrom`, `zonedTo` (a range's first and last day). `between(x, range)` reads `zonedFrom.start` and `zonedTo.end`. No other name changes. *Applied to the outline.*
- **C3 — `TestRunEndpointTakesTimeZone` is in Task 4, not Task 5.** `Options.TimeZone` and its JSON tag land in Task 4; in Task 5 the test could not be red. Task 4 passes `""` as the zone at `options.go:319` until Task 5 replaces it with `req.TimeZone`. *Applied to the outline.*
- **C4 — a local midnight that a clock change skips.** In `America/Santiago` the clock jumps from 00:00 to 01:00 on 2026-09-06, and Go's `time.Date(2026, 9, 6, 0, 0, 0, 0, loc)` answers 2026-09-05 23:00 (checked with Go 1.26 + tzdata). `zoneDay` steps forward to the first instant of the day (`localMidnight`), and Task 4 adds `TestZoneDayWhereMidnightIsSkipped`. *Applied to the outline.*
- **C5 — the T3 view fixture.** `zoo_feedings` has no `created` column (`NewBaseCollection` adds only `id`), and every zoo user is signed in, so `@request.auth.id != ""` refuses no reader. `addZooFeedLog` adds an autodate `created` to `zoo_feedings` and one feeding with note "secret", and the view's list rule is `@request.auth.role = "member" && note != "secret"`: a guest (`z.person(t, "gus", "guest")`) is the refused reader. *Applied to the outline.*
- **C6 — an existing test changes in Task 5.** `run-client.test.ts` "posts the request to the param-options route and returns the page" expects the exact body; it now expects the body with `timeZone: readerTimeZone()`. *Applied to the outline.*
- **C7 — guards among the new tests.** `TestSelectOverNumberIsStillRefused` (Task 1) passes before the change; `TestDateColumnIgnoresTheZone` and `TestDatePartsAndDayBucketsKeepTheUTCDay` (Task 4) are red first only because `Options.TimeZone` does not compile. Each pins a rule the change must keep; the steps say so. *Applied to the outline.*
- **C8 — Q3 answered (human, 2026-11-15): a typed day is zoned too.** Task 4b (after Task 4) gives a text literal that is a day `expr.zoned` through `fctx.readerDay`, so `zonedCompare` and `between` compare it with a `DateTime` as a `date` param. A `DateTime` compared with a `Date` column is unchanged. `TestTypedDayAgainstADateColumnIgnoresTheZone` is a guard (C7). *Applied to the outline (R1, Architecture, Names changed, Task 4b, Task 9, Coverage check).*

## Global Constraints

- No spec version bump (Q9): T2 is a run option, T3 a registration rule, D5 a manifest field.
- Core names no package: Go tests use `zoo_*` collections and `zoo.*` sources; the generator test and every comment in a core file use the fictional `@acme/gizmos` (slug `gizmos`).
- "A missing or unknown zone is UTC (today's result)." `""`, the names of UTC itself (`"UTC"`, `"Etc/UTC"`, `"Etc/GMT"`, `"GMT"`, the client's `UTC_ZONES`), `"Local"` and any name `time.LoadLocation` refuses read as UTC, and a UTC reader's statements are today's statements (R1, R2). PocketBase names a rule's placeholders and join aliases at random on every build, so a test compares two runs' statements only after `sameShape` (Task 4) renames them.
- "Buckets and date parts of a date-time (`year`, `month`, bucket by day) keep the UTC day (Q7)."
- Date-time text: "Nov 14, 2026, 4:07 PM" (12- or 24-hour by the device locale); the zone line "Times in Central Time".
- Go keeps the engine's style: values bound with `c.bind`, refusals through `invalid`, gofmt.
- Client: biome (4 spaces, single quotes), no `any`, no biome-ignore, no `console.*`; comments say why; web and native alike (the zone and the text come from `Intl`, R3, R4).
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. A reader whose zone the server does not know, `"Local"`, or a name of UTC itself (`Etc/UTC`, `GMT`): the run is UTC with today's statements, and the client writes UTC text and "Times in UTC" — Task 4 `TestZoneUnknownOrLocalIsUTC`, Task 2 "writes an unknown zone as UTC", "names UTC for UTC and for a zone it does not know".
2. A day of 23 or 25 hours: the range keeps exactly the instants of the local day — Task 4 `TestBetweenAcrossDSTEnd`, `TestBetweenAcrossDSTStart`.
3. A zone whose clock change skips local midnight: the day starts when the clock jumps, and the day before ends there — Task 4 `TestZoneDayWhereMidnightIsSkipped`.
4. A date-time column sized by a first page of short times later shows "Sep 30, 2026, 12:59 PM" uncut — Task 2 column-measure test.
5. A stored value no option of a select-over-text names: the cell holds it and the group is labelled with it — Task 1 `TestSelectOverTextShowsAnUnnamedValueAsItself`.
6. A device whose `Intl` leaves out `hourCycle` (a Hermes build) but gives `hour12: false`: cells use the 24-hour clock — Task 2 `isTwelveHour`. The reviewer confirms on RN 0.88 Hermes which of the two `resolvedOptions()` gives (R4).
7. A day typed in a condition (Q3): compared with a `DateTime` it has the same edges as a `date` param in the reader's zone; for a UTC reader its statements are today's; against a `Date` column it does not move — Task 4b `TestTypedDayComparisonsOfADateTimeInTheZone`, `TestTypedDayWithNoZoneIsUTC`, `TestTypedDayAgainstADateColumnIgnoresTheZone`. A text literal that is not a day (an instant) is not zoned — `TestTypedInstantIsNotZoned`.

---

### Task 1: T3 Go — Select over text; a source over a view

**Files:**
- Modify: `core/server/reports/check.go` (`compatible`, line 148)
- Create: `core/server/reports/viewsource_test.go`

**Interfaces:**
- Consumes: `newZoo`, `z.run`, `z.save`, `z.rec`, `z.id`, `z.person` (saved_fixture_test.go 62), `rule` (zoo_test.go 38), `requireDetails` / `detailCells` (viewsort_test.go 9–24), `CheckSources`, `Refused`, `RegisterSource`.
- Produces: `compatible[Select] = {core.FieldTypeSelect, core.FieldTypeText}`; test helpers `addZooNotes()` (source `zoo.notes`: `zoo_feedings.note` read as the Select field `kind`, options f1 "First", f2 "Second") and `addZooFeedLog(t testing.TB, z *zoo)` (view `zoo_feed_log`, source `zoo.feed_log`: `created` DateTime, `size` Select big/small, `note` Text).

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/viewsource_test.go`:

```go
package reports

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addZooNotes registers zoo.notes: the feedings' text column note read as a
// Select field. Its options name two of the stored values (f1, f2); f3 and f4
// are values no option names.
func addZooNotes() {
	RegisterSource(Source{
		Key: "zoo.notes", Collection: "zoo_feedings", Label: "Notes",
		Fields: []Field{{Key: "kind", Column: "note", Label: "Kind", Type: Select,
			Options: []Option{{Value: "f1", Label: "First"}, {Value: "f2", Label: "Second"}}}},
	})
}

// addZooFeedLog adds the view zoo_feed_log over zoo_feedings, as a package's
// computed list would be: a bare autodate column (typed as the source field,
// so DateTime reads it), a computed text column (CAST … AS TEXT, so Select
// reads it) and a text column. Its list rule lets members read every row but
// one: the feeding whose note is "secret". A guest reads none.
//
//	f1 500 g big   f2 800 g big   f3 300 g small
//	f4 100 g small f5  50 g small f6  20 g small   secret 900 g (never read)
func addZooFeedLog(t testing.TB, z *zoo) {
	t.Helper()
	feedings, err := z.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	feedings.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	z.save(t, feedings)
	z.rec(t, "secret", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5, "currency": "USD", "grams": 900, "note": "secret"})
	view := core.NewViewCollection("zoo_feed_log")
	view.ListRule = rule(`@request.auth.role = "member" && note != "secret"`)
	view.ViewRule = view.ListRule
	view.ViewQuery = `SELECT f.id, f.created, CAST(CASE WHEN f.grams >= 500 THEN 'big' ELSE 'small' END AS TEXT) AS size, f.note FROM zoo_feedings f`
	z.save(t, view)
	RegisterSource(Source{
		Key: "zoo.feed_log", Collection: "zoo_feed_log", Label: "Feed log",
		Fields: []Field{
			{Key: "created", Label: "Created", Type: DateTime, Aggregates: MinMax},
			{Key: "size", Label: "Size", Type: Select, Options: []Option{{Value: "big", Label: "Big"}, {Value: "small", Label: "Small"}}},
			{Key: "note", Label: "Note", Type: Text},
		},
	})
}

func requireNoRefusal(t *testing.T, app core.App, key string) {
	t.Helper()
	if err := CheckSources(app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	if reason, bad := Refused(app)[key]; bad {
		t.Fatalf("%s refused: %s", key, reason)
	}
	if lookupSource(app, key) == nil {
		t.Fatalf("%s is not usable", key)
	}
}

func TestSelectOverTextPassesCheckSources(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
}

const notesByKind = `{"version":4,"source":"zoo.notes",
	"params":[{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"f1","label":"First"},{"value":"f3","label":"Third"}]}],
	"columns":[{"id":"kind","path":[{"field":"kind"}]}],
	"filter":{"op":"in","args":[{"path":[{"field":"kind"}]},{"param":"kinds"}]},
	"sort":[{"col":"kind"}]}`

func TestSelectOverTextFiltersWithIn(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
	// ann keeps leo and zed: feedings f1–f4.
	requireDetails(t, z.run(t, "ann", notesByKind, Options{}, map[string]string{"kinds": `["f1","f3"]`}), "kind", "f1", "f3")
	requireDetails(t, z.run(t, "ann", notesByKind, Options{}, nil), "kind", "f1", "f2", "f3", "f4")
}

// R16: a stored value no option names is shown as stored: the cell holds the
// value (the client labels a cell from the column's options and falls back to
// it), and a group by the field is labelled with it.
func TestSelectOverTextShowsAnUnnamedValueAsItself(t *testing.T) {
	z := newZoo(t)
	addZooNotes()
	requireNoRefusal(t, z.app, "zoo.notes")
	const detail = `{"version":4,"source":"zoo.notes","columns":[{"id":"kind","path":[{"field":"kind"}]}],"sort":[{"col":"kind"}]}`
	requireDetails(t, z.run(t, "ann", detail, Options{}, nil), "kind", "f1", "f2", "f3", "f4")
	const grouped = `{"version":4,"source":"zoo.notes","columns":[{"id":"kind","path":[{"field":"kind"}]}],"groups":[{"id":"g","col":"kind"}]}`
	res := z.run(t, "ann", grouped, Options{}, nil)
	var labels []string
	for _, r := range res.Rows {
		if r.Kind == KindGroup {
			labels = append(labels, r.Label)
		}
	}
	sort.Strings(labels)
	if want := []string{"First", "Second", "f3", "f4"}; !slices.Equal(labels, want) {
		t.Fatalf("group labels = %v, want %v", labels, want)
	}
}

func TestViewSourcePassesCheckSources(t *testing.T) {
	z := newZoo(t)
	addZooFeedLog(t, z)
	requireNoRefusal(t, z.app, "zoo.feed_log")
}

const feedLog = `{"version":4,"source":"zoo.feed_log","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"size","path":[{"field":"size"}]},
	{"id":"created","path":[{"field":"created"}]}],
	"sort":[{"col":"note"}]}`

func TestViewSourceRunsUnderTheViewsListRule(t *testing.T) {
	z := newZoo(t)
	addZooFeedLog(t, z)
	z.person(t, "gus", "guest")
	requireNoRefusal(t, z.app, "zoo.feed_log")
	res := z.run(t, "ann", feedLog, Options{}, nil)
	requireDetails(t, res, "note", "f1", "f2", "f3", "f4", "f5", "f6")
	requireDetails(t, res, "size", "big", "big", "small", "small", "small", "small")
	types := make([]FieldType, 0, len(res.Columns))
	for _, c := range res.Columns {
		types = append(types, c.Type)
	}
	if want := []FieldType{Text, Select, DateTime}; !slices.Equal(types, want) {
		t.Fatalf("column types = %v, want %v", types, want)
	}
	for _, r := range res.Rows {
		if strings.Contains(r.Label, "secret") {
			t.Fatalf("the run read a row the view's list rule hides: %+v", r)
		}
	}
	// The rule refuses a guest every row.
	requireDetails(t, z.run(t, "gus", feedLog, Options{}, nil), "note")
}

// A select still reads only a select or a text column.
func TestSelectOverNumberIsStillRefused(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.x", Collection: "zoo_feedings", Label: "X",
		Fields: []Field{{Key: "grams", Label: "Grams", Type: Select, Options: []Option{{Value: "1", Label: "One"}}}}})
	err := CheckSources(z.app)
	want := `column "grams" is a number field, which a select field cannot read`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CheckSources = %v, want it to contain %q", err, want)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestSelectOver|TestViewSource' -count=1`
Expected: FAIL. `TestSelectOverTextPassesCheckSources`, `…FiltersWithIn`, `…ShowsAnUnnamedValueAsItself` with `CheckSources: reports: source "zoo.notes": field "kind": column "note" is a text field, which a select field cannot read`; both `TestViewSource…` with `source "zoo.feed_log": field "size": column "size" is a text field, which a select field cannot read`. `TestSelectOverNumberIsStillRefused` PASSES (a guard, C7).

If a `TestViewSource…` message names anything other than the `size` field (the view's list rule, its collection, its `created` column), that is a refusal of a source over a view (spec §5 "Source": a core fix here). Fix it where the message points (`partCollection` or `checkField` in `check.go`, `ruleSelect` in `rules.go`), and name the message and the fix in the task's hand-back.

- [ ] **Step 3: Accept a text column in a Select field**

In `core/server/reports/check.go`, replace line 148:

```go
	Select:   {core.FieldTypeSelect},
```

with:

```go
	// A view's computed column is text, never select (T3); its stored values
	// are not checked against the options, and one no option names shows as
	// stored (R16).
	Select: {core.FieldTypeSelect, core.FieldTypeText},
```

`checkField`'s option check (lines 123–132) runs only for a `*core.SelectField`, so a text column passes with its options unchecked; it needs no change. Run `gofmt -w core/server/reports/check.go` (it realigns the map).

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestSelectOver|TestViewSource|TestCheckSources' -count=1`
Expected: PASS (`TestCheckSources*` still pass: the zoo has six sources, and `addZoo*` run only in the new tests).

- [ ] **Step 5: Hand the diff to the driver**

`git -C /Users/nas/tmp/tinycld/books-reports/tinycld status --short` lists `core/server/reports/check.go` and `core/server/reports/viewsource_test.go`. No commit.

---

### Task 2: T1 client — date-time text

**Files:**
- Create: `core/reports/lib/date-time.ts`
- Modify: `core/reports/lib/format.ts` (imports, line 1; `cellDisplay`, lines 199–201)
- Modify: `core/reports/lib/column-measure.ts` (lines 79–82, 226–228) — C1
- Create: `core/reports/lib/__tests__/date-time.test.ts`
- Test: `core/reports/lib/__tests__/format.test.ts`, `core/reports/lib/__tests__/param-options.test.ts`, `core/reports/lib/__tests__/column-measure.test.ts`

**Interfaces:**
- Consumes: `isIsoDay`, `shortDay` (dates.ts 29, 51); `cellDisplay`, `rowLabelText`, `typedLabelText` (format.ts); `optionText`, `labelsOfPages` (param-options.ts); `chipText` (params.ts 369).
- Produces: `readerTimeZone(): string`; `isTwelveHour(resolved: Pick<Intl.ResolvedDateTimeFormatOptions, 'hourCycle' | 'hour12'>): boolean`; `formatDateTime(value: string, timeZone: string, hour12: boolean): string`; `dateTimeText(value: string, timeZone?: string): string`; `timeZoneWords(timeZone?: string): string` — all in `core/reports/lib/date-time.ts`; `dateTimeText` and `timeZoneWords` re-exported from `core/reports/lib/format.ts` (the brief's import path `@tinycld/core/reports/lib/format`). `cellDisplay` of a `datetime` value is `dateTimeText(value)`; a `date` value stays the short day.

- [ ] **Step 1: Write the failing tests**

Create `core/reports/lib/__tests__/date-time.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
    dateTimeText,
    formatDateTime,
    isTwelveHour,
    readerTimeZone,
    timeZoneWords,
} from '../date-time'

const AT = '2026-11-14 22:07:00.000Z'
const CHICAGO = 'America/Chicago'

describe('formatDateTime', () => {
    it("writes an instant as the day and time on the zone's wall clock", () => {
        expect(formatDateTime(AT, CHICAGO, true)).toBe('Nov 14, 2026, 4:07 PM')
        expect(formatDateTime(AT, CHICAGO, false)).toBe('Nov 14, 2026, 16:07')
    })

    it('writes local midnight as 12:00 AM, or 00:00', () => {
        expect(formatDateTime('2026-11-15 06:00:00.000Z', CHICAGO, true)).toBe(
            'Nov 15, 2026, 12:00 AM'
        )
        expect(formatDateTime('2026-11-15 06:00:00.000Z', CHICAGO, false)).toBe(
            'Nov 15, 2026, 00:00'
        )
    })

    it('follows the clock change at the end of daylight time', () => {
        // Nov 1 2026: 06:30Z is 1:30 AM CDT, and 07:30Z is 1:30 AM CST an hour later.
        expect(formatDateTime('2026-11-01 06:30:00.000Z', CHICAGO, true)).toBe(
            'Nov 1, 2026, 1:30 AM'
        )
        expect(formatDateTime('2026-11-01 07:30:00.000Z', CHICAGO, true)).toBe(
            'Nov 1, 2026, 1:30 AM'
        )
        expect(formatDateTime('2026-11-01 04:59:59.000Z', CHICAGO, true)).toBe(
            'Oct 31, 2026, 11:59 PM'
        )
    })

    it('writes an unknown zone as UTC, as the server reads it', () => {
        expect(formatDateTime(AT, 'Not/AZone', true)).toBe('Nov 14, 2026, 10:07 PM')
    })

    it('reads an instant written with a T', () => {
        expect(formatDateTime('2026-11-14T22:07:00Z', CHICAGO, true)).toBe('Nov 14, 2026, 4:07 PM')
    })

    it('shows a bare day as the short day and any other text as it is', () => {
        expect(formatDateTime('2026-11-14', CHICAGO, true)).toBe('Nov 14, 2026')
        expect(formatDateTime('soon', CHICAGO, true)).toBe('soon')
        expect(formatDateTime('2026-13-40 22:07:00.000Z', CHICAGO, true)).toBe(
            '2026-13-40 22:07:00.000Z'
        )
    })
})

describe('dateTimeText', () => {
    it("writes the instant in the zone given, in the device's hour cycle", () => {
        // The test runtime's locale is en-US, a 12-hour locale (R17).
        expect(dateTimeText(AT, CHICAGO)).toBe('Nov 14, 2026, 4:07 PM')
    })

    it("defaults to the reader's zone", () => {
        expect(dateTimeText(AT)).toBe(dateTimeText(AT, readerTimeZone()))
    })
})

describe('readerTimeZone', () => {
    it("is the device's IANA zone, UTC when it has none", () => {
        expect(readerTimeZone()).toBe(new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
    })
})

describe('isTwelveHour', () => {
    it("reads the locale's hour cycle", () => {
        expect(isTwelveHour({ hourCycle: 'h12' })).toBe(true)
        expect(isTwelveHour({ hourCycle: 'h11' })).toBe(true)
        expect(isTwelveHour({ hourCycle: 'h23' })).toBe(false)
        expect(isTwelveHour({ hourCycle: 'h24' })).toBe(false)
    })

    // A Hermes build may leave out hourCycle and still give hour12 (R4).
    it('falls back to hour12, then to the 12-hour clock', () => {
        expect(isTwelveHour({ hour12: false })).toBe(false)
        expect(isTwelveHour({ hour12: true })).toBe(true)
        expect(isTwelveHour({})).toBe(true)
    })
})

describe('timeZoneWords', () => {
    it('names a zone by its generic name', () => {
        expect(timeZoneWords(CHICAGO)).toBe('Central Time')
    })

    it('names UTC for UTC and for a zone it does not know', () => {
        expect(timeZoneWords('UTC')).toBe('UTC')
        expect(timeZoneWords('Etc/UTC')).toBe('UTC')
        expect(timeZoneWords('Not/AZone')).toBe('UTC')
    })

    it("defaults to the reader's zone", () => {
        expect(timeZoneWords()).toBe(timeZoneWords(readerTimeZone()))
    })
})
```

In `core/reports/lib/__tests__/format.test.ts`, add `dateTimeText` and `timeZoneWords` to the import list from `'../format'` (alphabetical: after `columnName`, and after `shownColumns`), and append:

```ts
describe('date-time cells (T1)', () => {
    const AT = '2026-11-14 22:07:00.000Z'
    const CREATED: ResultColumn = { id: 'created', label: 'Created', type: 'datetime' }
    const DAY: ResultColumn = { id: 'day', label: 'Day', type: 'date' }

    // Zone-free: each compares with dateTimeText, which holds in any test zone (R17).
    it("shows a date-time cell as its day and time in the reader's zone", () => {
        const text = cellDisplay(CREATED, row('detail'), AT, PLAIN).text
        expect(text).toBe(dateTimeText(AT))
        expect(text).toMatch(/^[A-Z][a-z]{2} \d{1,2}, 2026, \d{1,2}:\d{2}/)
    })

    it('labels a row typed datetime with the same text', () => {
        const labelled: ResultRow = { ...row('detail'), labelType: 'datetime', labelValue: AT }
        expect(rowLabelText(labelled, PLAIN)).toBe(dateTimeText(AT))
    })

    it('keeps a date cell a day', () => {
        expect(cellDisplay(DAY, row('detail'), '2026-11-14', PLAIN).text).toBe('Nov 14, 2026')
        expect(cellDisplay(DAY, row('detail'), '2026-11-14 00:00:00.000Z', PLAIN).text).toBe(
            'Nov 14, 2026'
        )
    })

    it('exports the zone words a package screen reads', () => {
        expect(timeZoneWords('America/Chicago')).toBe('Central Time')
    })
})
```

In `core/reports/lib/__tests__/param-options.test.ts`, add `import { dateTimeText } from '../format'` and `import { chipText } from '../params'`, and append:

```ts
describe('a date-time option (T1)', () => {
    const AT = '2026-11-14 22:07:00.000Z'
    const option = { value: 'e1', label: AT, type: 'datetime' as const }
    const ENTRY: ParamDef = {
        id: 'entry',
        type: 'choice',
        label: 'Entry',
        optionsFrom: { source: 'zoo.entries', value: [{ field: 'id' }], label: [{ field: 'created' }] },
    }

    it('writes the option and its chip as a date-time cell', () => {
        expect(optionText(option)).toBe(dateTimeText(AT))
        const labels = labelsOfPages([{ options: [option], groups: [] }])
        expect(chipText(ENTRY, { kind: 'choice', values: ['e1'] }, labels)).toBe(
            `Entry: ${dateTimeText(AT)}`
        )
    })
})
```

In `core/reports/lib/__tests__/column-measure.test.ts`, append inside `describe('measureColumns', …)`:

```ts
    // C1: a date-time is a figure, never cut, so its column and a date-time row
    // label keep room for the widest date and time a later page may bring.
    it('keeps room for a date and a time in a date-time column and label', () => {
        const AT = '2026-01-02 03:04:00.000Z'
        const at: ResultColumn = { id: 'at', label: 'At', type: 'datetime' }
        const short = item('detail', AT, { at: AT }, { labelType: 'datetime', labelValue: AT })
        const m = measureColumns({
            columns: [COLUMNS[0] as ResultColumn, at],
            rows: [short],
            format: DEFAULT_DISPLAY,
            levels: FLAT,
        })
        const worst = textWidth('Sep 30, 2026, 12:59 PM', CELL)
        expect(m.at.natural).toBeGreaterThanOrEqual(worst + CELL_PADDING)
        expect(m.account.natural).toBeGreaterThanOrEqual(
            INDENT + short.row.depth * INDENT + worst + CELL_PADDING / 2
        )
    })
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/date-time.test.ts core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/param-options.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: FAIL. `date-time.test.ts` with `Failed to resolve import "../date-time"` (this also holds the `isTwelveHour` cases); the new `format.test.ts` and `param-options.test.ts` cases with `TypeError: dateTimeText is not a function` (and `timeZoneWords is not a function`); the column-measure case with `expected … to be greater than or equal to …` (the worst case is still "Sep 30, 2026"). Every older case passes.

- [ ] **Step 3: Write `date-time.ts`**

Create `core/reports/lib/date-time.ts`:

```ts
import { isIsoDay, shortDay } from './dates'

// A date-time value is an instant as PocketBase stores it ("2026-11-14 22:07:00.000Z").
// It shows as the day and the time on the reader's wall clock. Intl gives only the
// numbers; the words are core's own, as the day words are (dates.ts), because
// Hermes ships little locale data (R4).

const INSTANT = /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d{1,3})?)?Z$/
// The names of UTC itself. The server's readerZone (zone.go) reads the same set as
// UTC, so the two never disagree on which reader is in UTC.
const UTC_ZONES: ReadonlySet<string> = new Set(['UTC', 'Etc/UTC', 'Etc/GMT', 'GMT'])
const WALL_CLOCK: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hourCycle: 'h23',
}
// Any day serves: a zone's generic name does not change with the season.
const REFERENCE_DAY = new Date(Date.UTC(2026, 0, 15))

type DeviceClock = { timeZone: string; hour12: boolean }
let deviceClock: DeviceClock | null = null

/**
 * Whether the device locale's clock is 12-hour, from Intl's resolved options. An
 * engine that leaves out hourCycle (some Hermes builds) may still give hour12; a
 * locale that gives neither reads 12-hour (R4).
 */
export function isTwelveHour(
    resolved: Pick<Intl.ResolvedDateTimeFormatOptions, 'hourCycle' | 'hour12'>
): boolean {
    if (resolved.hourCycle) return resolved.hourCycle === 'h11' || resolved.hourCycle === 'h12'
    if (resolved.hour12 !== undefined) return resolved.hour12
    return true
}

// Read once per app start (R3), so a cell, the header's zone line and the run
// request always name one zone.
function readDeviceClock(): DeviceClock {
    if (deviceClock) return deviceClock
    const timeZone = new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
    const resolved = new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).resolvedOptions()
    deviceClock = { timeZone, hour12: isTwelveHour(resolved) }
    return deviceClock
}

/** The reader's IANA time zone, as the run request sends it. */
export function readerTimeZone(): string {
    return readDeviceClock().timeZone
}

function isKnownZone(timeZone: string): boolean {
    try {
        return new Intl.DateTimeFormat('en-US', { timeZone }).resolvedOptions().timeZone !== ''
    } catch (error) {
        if (error instanceof RangeError) return false
        throw error
    }
}

const formatters = new Map<string, Intl.DateTimeFormat>()

// A zone Intl does not know reads as UTC, as the server reads it (R2), so the
// cells and the rows a filter keeps agree.
function wallClockFormat(timeZone: string): Intl.DateTimeFormat {
    const known = formatters.get(timeZone)
    if (known) return known
    const zone = isKnownZone(timeZone) ? timeZone : 'UTC'
    const made = new Intl.DateTimeFormat('en-US', { ...WALL_CLOCK, timeZone: zone })
    formatters.set(timeZone, made)
    return made
}

const pad = (n: number) => String(n).padStart(2, '0')

function partOf(parts: readonly Intl.DateTimeFormatPart[], type: Intl.DateTimeFormatPartTypes) {
    return parts.find(part => part.type === type)?.value ?? ''
}

// The hour on a 24-hour clock. An engine that ignores hourCycle answers with a
// day period (AM/PM), which is read too.
function hour24(parts: readonly Intl.DateTimeFormatPart[]): number {
    const hour = Number(partOf(parts, 'hour'))
    const period = partOf(parts, 'dayPeriod')
    if (!period) return hour % 24
    return (hour % 12) + (/^p/i.test(period) ? 12 : 0)
}

function clockText(hour: number, minute: string, hour12: boolean): string {
    if (!hour12) return `${pad(hour)}:${minute}`
    return `${hour % 12 || 12}:${minute} ${hour < 12 ? 'AM' : 'PM'}`
}

function instantOf(value: string): Date | null {
    if (!INSTANT.test(value)) return null
    const date = new Date(value.replace(' ', 'T'))
    return Number.isNaN(date.getTime()) ? null : date
}

/**
 * An instant as "Nov 14, 2026, 4:07 PM" (or "Nov 14, 2026, 16:07") on the zone's
 * wall clock. A bare day shows as the short day; any other text shows as it is.
 */
export function formatDateTime(value: string, timeZone: string, hour12: boolean): string {
    if (isIsoDay(value)) return shortDay(value)
    const instant = instantOf(value)
    if (!instant) return value
    const parts = wallClockFormat(timeZone).formatToParts(instant)
    const month = pad(Number(partOf(parts, 'month')))
    const day = pad(Number(partOf(parts, 'day')))
    const date = `${partOf(parts, 'year')}-${month}-${day}`
    return `${shortDay(date)}, ${clockText(hour24(parts), partOf(parts, 'minute'), hour12)}`
}

/** A date-time as a report cell shows it: in the reader's zone and hour cycle. */
export function dateTimeText(value: string, timeZone?: string): string {
    const clock = readDeviceClock()
    return formatDateTime(value, timeZone ?? clock.timeZone, clock.hour12)
}

function zoneName(timeZone: string, style: 'long' | 'longGeneric'): string | null {
    try {
        const parts = new Intl.DateTimeFormat('en-US', {
            timeZone,
            timeZoneName: style,
        }).formatToParts(REFERENCE_DAY)
        const name = partOf(parts, 'timeZoneName')
        return name && !name.startsWith('GMT') ? name : null
    } catch (error) {
        // An engine without the generic style refuses it.
        if (error instanceof RangeError || error instanceof TypeError) return null
        throw error
    }
}

/**
 * A zone in words for the header's line (R6): "Central Time"; "UTC" for UTC and
 * for a zone Intl does not know; else the zone's city ("Chicago time").
 */
export function timeZoneWords(timeZone = readerTimeZone()): string {
    if (UTC_ZONES.has(timeZone) || !isKnownZone(timeZone)) return 'UTC'
    const generic = zoneName(timeZone, 'longGeneric')
    if (generic) return generic
    const long = zoneName(timeZone, 'long')
    if (long) return long.replace(' Standard ', ' ').replace(' Daylight ', ' ')
    const city = timeZone.split('/').pop() ?? timeZone
    return `${city.split('_').join(' ')} time`
}
```

- [ ] **Step 4: Show a date-time cell through it, and re-export it from `format.ts`**

In `core/reports/lib/format.ts`, under the first import (line 1), add:

```ts
import { dateTimeText, timeZoneWords } from './date-time'
```

After the `CellDisplay` type (after line 39), add:

```ts
// A package screen writes a date-time, and the zone line, as a report does (spec T1).
export { dateTimeText, timeZoneWords }
```

Replace lines 199–201 of `cellDisplay`:

```ts
    if ((type === 'date' || type === 'datetime') && typeof value === 'string') {
        return plainDisplay(dayText(value), align)
    }
```

with:

```ts
    // A date-time is an instant: its day and time in the reader's zone. A date is a day.
    if (type === 'datetime' && typeof value === 'string') {
        return plainDisplay(dateTimeText(value), align)
    }
    if (type === 'date' && typeof value === 'string') {
        return plainDisplay(dayText(value), align)
    }
```

Row labels (`rowLabelText` → `typedLabelText`), pivot headings, options and chips (`optionText` → `typedLabelText`) and the drill panel's table all reach `cellDisplay`; they need no change.

- [ ] **Step 5: Keep room for a date-time in `column-measure.ts` (C1)**

In `core/reports/lib/column-measure.ts`, replace lines 79–82:

```ts
const WORST_DAY = shortDay('2026-09-30')
const WORST_TEXT: Partial<Record<FieldType, string>> = {
    date: WORST_DAY,
    datetime: WORST_DAY,
```

with:

```ts
const WORST_DAY = shortDay('2026-09-30')
// A date-time shows its time too (dateTimeText); 12-hour is the wider clock.
const WORST_DATE_TIME = `${WORST_DAY}, 12:59 PM`
const WORST_TEXT: Partial<Record<FieldType, string>> = {
    date: WORST_DAY,
    datetime: WORST_DATE_TIME,
```

and replace lines 226–229 of `labelWidths`:

```ts
        if (row.labelType === 'date' || row.labelType === 'datetime') {
            const start = INDENT + row.depth * INDENT
            widest = Math.max(widest, start + textWidth(WORST_DAY, CELL) + CELL_PADDING / 2)
        }
```

with:

```ts
        if (row.labelType === 'date' || row.labelType === 'datetime') {
            const start = INDENT + row.depth * INDENT
            const worst = row.labelType === 'datetime' ? WORST_DATE_TIME : WORST_DAY
            widest = Math.max(widest, start + textWidth(worst, CELL) + CELL_PADDING / 2)
        }
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/date-time.test.ts core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/param-options.test.ts core/reports/lib/__tests__/column-measure.test.ts core/reports/components/__tests__/report-table-layout.test.ts`
Expected: PASS, every case.

- [ ] **Step 7: Typecheck and lint**

Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check core/reports/lib/date-time.ts core/reports/lib/format.ts core/reports/lib/column-measure.ts core/reports/lib/__tests__/date-time.test.ts core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/param-options.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: no errors. (Fix a formatting finding with `biome check --write` on these files only.)

- [ ] **Step 8: Hand the diff to the driver**

No commit. The diff: `date-time.ts` (new), `format.ts`, `column-measure.ts`, four test files.

---

### Task 3: T1 view — the header's zone line

**Files:**
- Modify: `core/reports/lib/format.ts` (add `zoneLine` after `shownColumns`, line 284)
- Modify: `core/reports/hooks/useReportView.ts` (imports, line 12; model, line 322)
- Modify: `core/reports/components/ReportDocumentHeader.tsx` (props and the zone line)
- Modify: `core/reports/components/ReportView.tsx` (lines 220–225)
- Modify: `core/reports/components/ReportDrillPanel.tsx` (lines 142–147)
- Test: `core/reports/lib/__tests__/format.test.ts`

**Interfaces:**
- Consumes: `timeZoneWords` (Task 2, imported in format.ts), `shownColumns` (format.ts 279).
- Produces: `zoneLine(columns: readonly ResultColumn[], timeZone?: string): string | null` ("Times in Central Time", or null when no column is a date-time); `ReportViewModel.zoneWords: string | null` (required: a hand-built model passes it); `ReportDocumentHeader` prop `zoneWords: string | null`, line test id `report-document-zone` (`text-sm text-muted-foreground`, under `report-document-period`).

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/format.test.ts`, add `zoneLine` to the import list from `'../format'` (last), and append:

```ts
describe('zoneLine (T1, R5)', () => {
    const CHICAGO = 'America/Chicago'
    const USER: ResultColumn = { id: 'user', label: 'User', type: 'text' }
    const DAY: ResultColumn = { id: 'day', label: 'Day', type: 'date' }
    const CREATED: ResultColumn = { id: 'created', label: 'Created', type: 'datetime' }
    const ENTRY: ResultColumn = {
        id: 'entry',
        label: 'Entry',
        type: 'relation',
        labelCell: 'entry@label',
        labelType: 'datetime',
    }

    it('names the zone once when a shown column is a date-time', () => {
        expect(zoneLine([DAY, CREATED], CHICAGO)).toBe('Times in Central Time')
        expect(zoneLine([CREATED, { ...CREATED, id: 'updated' }], CHICAGO)).toBe(
            'Times in Central Time'
        )
    })

    it('is null when no shown column is a date-time', () => {
        expect(zoneLine([DAY, USER], CHICAGO)).toBeNull()
        expect(zoneLine([], CHICAGO)).toBeNull()
    })

    it('counts a link whose record label is a date-time', () => {
        expect(zoneLine([USER, ENTRY], CHICAGO)).toBe('Times in Central Time')
    })

    // The first column cannot be hidden (shownColumns), so the date-time is second.
    it('is null when Customize hid the only date-time column', () => {
        const columns = [USER, CREATED]
        expect(zoneLine(shownColumns(columns, ['created']), CHICAGO)).toBeNull()
        expect(zoneLine(shownColumns(columns, []), CHICAGO)).toBe('Times in Central Time')
    })

    it("names the reader's zone by default", () => {
        expect(zoneLine([CREATED])).toBe(`Times in ${timeZoneWords()}`)
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/format.test.ts`
Expected: FAIL in the five new cases with `TypeError: zoneLine is not a function`; every other case passes.

- [ ] **Step 3: Write `zoneLine`**

In `core/reports/lib/format.ts`, append after `shownColumns`:

```ts
/**
 * The document header's zone line (T1): "Times in Central Time" when a shown column
 * is a date-time, or a link whose record label is one; else null. Only the columns
 * are read: rows page in later, and the line must not appear late (R5).
 */
export function zoneLine(columns: readonly ResultColumn[], timeZone?: string): string | null {
    const hasDateTime = columns.some(
        column => column.type === 'datetime' || column.labelType === 'datetime'
    )
    return hasDateTime ? `Times in ${timeZoneWords(timeZone)}` : null
}
```

- [ ] **Step 4: Put the line on the model and draw it**

In `core/reports/hooks/useReportView.ts`, replace line 12:

```ts
import { DEFAULT_DISPLAY, type DisplayOptions, shownColumns } from '../lib/format'
```

with:

```ts
import { DEFAULT_DISPLAY, type DisplayOptions, shownColumns, zoneLine } from '../lib/format'
```

After line 200 (`const allColumns = …`), add:

```ts
    const columns = shownColumns(allColumns, options.display.hidden)
```

and replace line 322:

```ts
        columns: shownColumns(allColumns, options.display.hidden),
```

with:

```ts
        columns,
        // Required in the model, so a hand-built model states its line too (B4).
        zoneWords: zoneLine(columns),
```

Replace `core/reports/components/ReportDocumentHeader.tsx` lines 20–62 (the props interface and the component) with:

```tsx
interface ReportDocumentHeaderProps {
    /** The source's heading, else the organization's name (E9). */
    heading: string
    title: string
    periodWords: string | null
    /** "Times in Central Time" when a shown column is a date-time (T1); else null. */
    zoneWords: string | null
    isThousands: boolean
}

// The finished-document strip above the table: who, what, which period and, for
// date-times, which zone.
export function ReportDocumentHeader({
    heading,
    title,
    periodWords,
    zoneWords,
    isThousands,
}: ReportDocumentHeaderProps) {
    const businessName = heading.trim() || null
    const thousandsNote = isThousands ? 'Amounts in thousands' : null
    return (
        <View
            testID="report-document-header"
            className="gap-0.5 border-y border-border bg-card px-4 py-3"
        >
            <Line
                text={businessName}
                testID="report-business-name"
                className="text-xs text-muted-foreground"
            />
            <Text testID="report-document-title" className="text-lg font-semibold text-foreground">
                {title}
            </Text>
            <Line
                text={periodWords}
                testID="report-document-period"
                className="text-sm text-muted-foreground"
            />
            <Line
                text={zoneWords}
                testID="report-document-zone"
                className="text-sm text-muted-foreground"
            />
            <Line
                text={thousandsNote}
                testID="report-thousands-note"
                className="text-sm text-muted-foreground"
            />
        </View>
    )
}
```

In `core/reports/components/ReportView.tsx`, replace lines 220–225:

```tsx
            <ReportDocumentHeader
                heading={model.heading}
                title={model.title}
                periodWords={model.periodWords}
                isThousands={model.display.divideBy1000}
            />
```

with:

```tsx
            <ReportDocumentHeader
                heading={model.heading}
                title={model.title}
                periodWords={model.periodWords}
                zoneWords={model.zoneWords}
                isThousands={model.display.divideBy1000}
            />
```

In `core/reports/components/ReportDrillPanel.tsx`, replace lines 142–147:

```tsx
            <ReportDocumentHeader
                heading={view.heading}
                title={view.title}
                periodWords={view.periodWords}
                isThousands={view.display.divideBy1000}
            />
```

with:

```tsx
            <ReportDocumentHeader
                heading={view.heading}
                title={view.title}
                periodWords={view.periodWords}
                zoneWords={view.zoneWords}
                isThousands={view.display.divideBy1000}
            />
```

- [ ] **Step 5: Run the tests to see them pass**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/format.test.ts`
Expected: PASS.

- [ ] **Step 6: Typecheck and lint**

Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check core/reports/lib/format.ts core/reports/hooks/useReportView.ts core/reports/components/ReportDocumentHeader.tsx core/reports/components/ReportView.tsx core/reports/components/ReportDrillPanel.tsx core/reports/lib/__tests__/format.test.ts`
Expected: no errors. Core has no hand-built `ReportViewModel`; the Books scratch route's does (B4, gate 2), and Books is not typechecked here.

How the model's case is proven: the model passes `zoneLine` the same `columns` the table draws (`shownColumns(allColumns, options.display.hidden)`), so "Customize hid the only date-time column → no line" is the Step 1 test `is null when Customize hid the only date-time column` over `shownColumns`. That the model and the header draw the line is proven by Task 11's e2e test 1 (`report-document-zone` reads "Times in Central Time", once), and Task 11 Step 3 mutation 4 (`zoneWords: null` in `useReportView`) proves that assertion fails without this step's wiring.

- [ ] **Step 7: Hand the diff to the driver**

No commit.

---

### Task 4: T2 Go — ranges and day comparisons in the reader's zone

**Files:**
- Create: `core/server/reports/zone.go`
- Modify: `core/server/reports/run.go` (`Options`, after `Today`, line 42; `Run`, line 327; `readerCompiler`, lines 363–386; `todayOf`, lines 388–397)
- Modify: `core/server/reports/params.go` (`paramValue`, lines 47–61; `periodContext`, lines 64–67; `resolveParams`, line 169; `paramExpr`, lines 484 and 508; `rangeBound`, lines 522–526)
- Modify: `core/server/reports/formula.go` (`expr`, after `scan`, line 65; `lt`/`lte`/`gt`/`gte`, line 852; `between` call, line 854; `between`, lines 1068–1094)
- Modify: `core/server/reports/options.go` (line 319: pass `""` until Task 5, C3)
- Create: `core/server/reports/zonerange_test.go`

**Interfaces:**
- Consumes: `fctx.bind`, `invalid`, `isDay`, `ordered`, `requireDetails`, `groupLabelTypes` (bucketlabel_test.go 37), `observeReads` / `readStatement` (statement_test.go 326–340), `z.token`, `Register`.
- Produces: `Options.TimeZone string` (JSON `timeZone,omitempty`); `readerCompiler(app core.App, spec *Spec, auth *core.Record, headers map[string]string, given map[string]json.RawMessage, today, todayAt, timeZone string) (*compiler, error)`; `todayOf(s, at string, loc *time.Location) (time.Time, error)`; `periodContext.loc *time.Location`; `paramValue.dayZ, fromZ, toZ *zonedDay`; `expr.zoned, zonedFrom, zonedTo *zonedDay` (C2); `between(c fctx, args []expr, at string) (expr, error)`; in zone.go: `instantLayout`, `type zonedDay struct{ start, end string }`, `utcNames` (the names of UTC itself: `""`, `UTC`, `Etc/UTC`, `Etc/GMT`, `GMT`, as the client's `UTC_ZONES`), `readerZone(name string) *time.Location`, `zoneDay(day string, loc *time.Location) zonedDay`, `localMidnight(day time.Time, loc *time.Location) time.Time`, `withZonedDays(v paramValue, loc *time.Location) paramValue`, `zonedSpan(c fctx, x expr, start, end string) expr`, `zonedCompare(c fctx, op string, a, b expr) (expr, bool)`. Test helper `addZooSightings(t testing.TB, z *zoo)` (collection `zoo_sightings`, source `zoo.sightings`) and the spec constants `sightingsInPeriod`, `nov14`, used again by Task 5; test helper `sameShape(statements []readStatement) []readStatement` (PocketBase's random placeholder and join-alias names renamed in order of first appearance, the params renamed to match).

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/zonerange_test.go`:

```go
package reports

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// PocketBase names some parts of a rule's SQL at random on every build: a
// bound placeholder ({:f…} for @request.auth.*, {:t…} for a literal;
// core/record_field_resolver.go:377, tools/search/filter.go:279) and a join
// alias suffix (__collection_…, __ml…, __sm…; record_field_resolver.go:170,
// filter.go:649). Two runs of one rule differ only in them.
var (
	placeholderName = regexp.MustCompile(`\{:([^{}]+)\}`)
	joinAlias       = regexp.MustCompile(`__\w+`)
)

// sameShape is each statement with its placeholders and join aliases renamed
// in order of first appearance, and its params renamed to match, so two runs
// compare by the SQL they send and the values they bind, never by
// PocketBase's random names. A param the SQL does not reference (PocketBase
// binds a rule literal such as "" under a random key, then inlines it) is
// dropped: its key is random and it sends nothing.
func sameShape(statements []readStatement) []readStatement {
	out := make([]readStatement, 0, len(statements))
	for _, s := range statements {
		names := map[string]string{}
		sql := placeholderName.ReplaceAllStringFunc(s.sql, func(m string) string {
			name := m[2 : len(m)-1]
			if _, ok := names[name]; !ok {
				names[name] = fmt.Sprintf("v%d", len(names))
			}
			return "{:" + names[name] + "}"
		})
		aliases := map[string]string{}
		sql = joinAlias.ReplaceAllStringFunc(sql, func(m string) string {
			if _, ok := aliases[m]; !ok {
				aliases[m] = fmt.Sprintf("__a%d", len(aliases))
			}
			return aliases[m]
		})
		params := dbx.Params{}
		for k, v := range s.params {
			renamed, ok := names[k]
			if !ok {
				continue // bound by PocketBase but not in the SQL (a literal it inlined)
			}
			params[renamed] = v
		}
		out = append(out, readStatement{sql: sql, params: params})
	}
	return out
}

// The zoo's sightings: one row on each side of the edges of three local days
// in America/Chicago. Each note names its row.
//
//	day          local midnight      the day ends     before / start / end / after
//	2026-03-08   06:00Z (DST start)  2026-03-09 05:00Z  before8 start8 end8 after8
//	2026-11-01   05:00Z (DST end)    2026-11-02 06:00Z  before1 start1 end1 after1
//	2026-11-14   06:00Z              2026-11-15 06:00Z  before14 start14 end14 after14
var sightingRows = []struct{ note, seen string }{
	{"before8", "2026-03-08 05:59:59.000Z"}, {"start8", "2026-03-08 06:00:00.000Z"},
	{"end8", "2026-03-09 04:59:59.000Z"}, {"after8", "2026-03-09 05:00:00.000Z"},
	{"before1", "2026-11-01 04:59:59.000Z"}, {"start1", "2026-11-01 05:00:00.000Z"},
	{"end1", "2026-11-02 05:59:59.000Z"}, {"after1", "2026-11-02 06:00:00.000Z"},
	{"before14", "2026-11-14 05:59:59.000Z"}, {"start14", "2026-11-14 06:00:00.000Z"},
	{"end14", "2026-11-15 05:59:59.000Z"}, {"after14", "2026-11-15 06:00:00.000Z"},
}

// addZooSightings adds zoo_sightings (a date column holding full instants,
// which an autodate cannot be set to) and its source zoo.sightings.
func addZooSightings(t testing.TB, z *zoo) {
	t.Helper()
	sightings := core.NewBaseCollection("zoo_sightings")
	sightings.ListRule = rule(`@request.auth.id != ""`)
	sightings.ViewRule = sightings.ListRule
	sightings.Fields.Add(&core.DateField{Name: "seen"}, &core.TextField{Name: "note"})
	z.save(t, sightings)
	for _, s := range sightingRows {
		z.rec(t, s.note, "zoo_sightings", map[string]any{"seen": s.seen, "note": s.note})
	}
	RegisterSource(Source{
		Key: "zoo.sightings", Collection: "zoo_sightings", Label: "Sightings",
		Fields: []Field{
			{Key: "seen", Label: "Seen", Type: DateTime, Aggregates: MinMax},
			{Key: "note", Label: "Note", Type: Text},
		},
	})
}

const chicagoZone = "America/Chicago"

// nov14 is a date range of one day, 2026-11-14.
const nov14 = `{"from":"2026-11-14","to":"2026-11-14"}`

const sightingsInPeriod = `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"seen","path":[{"field":"seen"}]}],
	"filter":{"op":"between","args":[{"path":[{"field":"seen"}]},{"param":"period"}]},
	"sort":[{"col":"seen"}]}`

func rangeOf(from, to string) string { return `{"from":"` + from + `","to":"` + to + `"}` }

func TestBetweenDateTimeInTheReadersZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": nov14}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start14", "end14")
	// With no zone the day is the UTC day, as before.
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{}, period), "note", "before14", "start14")
	// A range of many days keeps from the first local midnight to the last.
	many := map[string]string{"period": rangeOf("2026-11-01", "2026-11-14")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, many), "note",
		"start1", "end1", "after1", "before14", "start14", "end14")
}

// A 25-hour day: local midnight is 05:00Z on Nov 1 and 06:00Z on Nov 2.
func TestBetweenAcrossDSTEnd(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": rangeOf("2026-11-01", "2026-11-01")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start1", "end1")
}

// A 23-hour day: local midnight is 06:00Z on Mar 8 and 05:00Z on Mar 9.
func TestBetweenAcrossDSTStart(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": rangeOf("2026-03-08", "2026-03-08")}
	requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: chicagoZone}, period), "note", "start8", "end8")
}

// sightingsWhere is every sighting, sorted by when it was seen, filtered by
// where (a formula that reads the params day and period).
func sightingsWhere(where string) string {
	return `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"day","type":"date"},{"id":"period","type":"dateRange"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"seen","path":[{"field":"seen"}]}],
	"filter":` + where + `,"sort":[{"col":"seen"}]}`
}

func opOf(op, left, right string) string { return `{"op":"` + op + `","args":[` + left + `,` + right + `]}` }

const (
	seenLeaf  = `{"path":[{"field":"seen"}]}`
	dayLeaf   = `{"param":"day"}`
	fromBound = `{"param":"period","bound":"from"}`
	toBound   = `{"param":"period","bound":"to"}`
)

// A comparison of a date-time with a day of the reader's zone compares the
// instants of that local day: gte from its start, gt from its end, lt before
// its start, lte before its end (R1); the day on the left mirrors; a range's
// bound is its first or last day.
func TestDayComparisonsOfADateTimeInTheZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	all := []any{"before8", "start8", "end8", "after8", "before1", "start1", "end1", "after1", "before14", "start14", "end14", "after14"}
	fromStart := []any{"start14", "end14", "after14"}
	fromEnd := []any{"after14"}
	beforeStart := all[:9]
	beforeEnd := all[:11]
	params := map[string]string{"day": `"2026-11-14"`, "period": nov14}
	for name, c := range map[string]struct {
		where string
		want  []any
	}{
		"gte a day":               {opOf("gte", seenLeaf, dayLeaf), fromStart},
		"gt a day":                {opOf("gt", seenLeaf, dayLeaf), fromEnd},
		"lt a day":                {opOf("lt", seenLeaf, dayLeaf), beforeStart},
		"lte a day":               {opOf("lte", seenLeaf, dayLeaf), beforeEnd},
		"a day lte":               {opOf("lte", dayLeaf, seenLeaf), fromStart},
		"a day gt":                {opOf("gt", dayLeaf, seenLeaf), beforeStart},
		"gte the range's from":    {opOf("gte", seenLeaf, fromBound), fromStart},
		"gt the range's from":     {opOf("gt", seenLeaf, fromBound), fromEnd},
		"lt the range's to":       {opOf("lt", seenLeaf, toBound), beforeStart},
		"lte the range's to":      {opOf("lte", seenLeaf, toBound), beforeEnd},
		"between a day and a day": {`{"op":"between","args":[` + seenLeaf + `,` + dayLeaf + `,` + dayLeaf + `]}`, []any{"start14", "end14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", sightingsWhere(c.where), Options{TimeZone: chicagoZone}, params)
			requireDetails(t, res, "note", c.want...)
		})
	}
}

// R2: no zone, the names of UTC itself, the server's own zone and a name the
// server does not know all run in UTC, with the statements of a run with no
// zone (compared after sameShape: PocketBase's rule SQL has random names).
func TestZoneUnknownOrLocalIsUTC(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	period := map[string]string{"period": nov14}
	reads := func(zone string, want ...any) []readStatement {
		seen := observeReads(t)
		requireDetails(t, z.run(t, "ann", sightingsInPeriod, Options{TimeZone: zone}, period), "note", want...)
		return sameShape(*seen)
	}
	none := reads("", "before14", "start14")
	// The compare sees a zone: a Chicago run's statements differ.
	if chicago := reads(chicagoZone, "start14", "end14"); reflect.DeepEqual(chicago, none) {
		t.Fatal("a Chicago run sent the statements of a run with no zone: the compare cannot see a zone")
	}
	for _, zone := range []string{"UTC", "Etc/UTC", "Etc/GMT", "GMT", "Local", "Not/AZone"} {
		if got := reads(zone, "before14", "start14"); !reflect.DeepEqual(got, none) {
			t.Fatalf("zone %q: the statements differ from a run with no zone:\n got  %v\n want %v", zone, got, none)
		}
	}
}

// A Date field is a day: a zone does not move it.
func TestDateColumnIgnoresTheZone(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		"sort":[{"col":"note"}]}`
	period := map[string]string{"period": rangeOf("2026-01-05", "2026-01-20")}
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, period), "note", "f1", "f3")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, period), "note", "f1", "f3")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed a date column's statements:\n got  %v\n want %v", got, utc)
	}
}

// Q7: a date part and a bucket by day of a date-time keep the UTC day.
// 2026-11-01 04:59:59Z is Oct 31 in Chicago, and stays November 1.
func TestDatePartsAndDayBucketsKeepTheUTCDay(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	chicago := Options{TimeZone: chicagoZone}
	const onlyBefore1 = `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"before1"}]}`
	parts := `{"version":4,"source":"zoo.sightings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"month","formula":{"op":"month","args":[{"path":[{"field":"seen"}]}]}}],` + onlyBefore1 + `}`
	requireDetails(t, z.run(t, "ann", parts, chicago, nil), "month", int64(11))
	buckets := `{"version":4,"source":"zoo.sightings","columns":[
		{"id":"seen","path":[{"field":"seen"}]},
		{"id":"note","path":[{"field":"note"}]}],` + onlyBefore1 + `,
		"groups":[{"id":"d","col":"seen","bucket":"day"}]}`
	if got, want := groupLabelTypes(z.run(t, "ann", buckets, chicago, nil)), []string{"2026-11-01 day=2026-11-01"}; !slices.Equal(got, want) {
		t.Fatalf("group labels = %v, want %v", got, want)
	}
}

// C4: in America/Santiago the clock jumps from 00:00 to 01:00 on 2026-09-06,
// so that day starts at 01:00 local (04:00Z), where the day before ends.
func TestZoneDayWhereMidnightIsSkipped(t *testing.T) {
	loc := readerZone("America/Santiago")
	if loc == nil {
		t.Fatal("America/Santiago is not known")
	}
	if got, want := zoneDay("2026-09-06", loc), (zonedDay{start: "2026-09-06 04:00:00.000Z", end: "2026-09-07 03:00:00.000Z"}); got != want {
		t.Fatalf("2026-09-06 = %+v, want %+v", got, want)
	}
	if before := zoneDay("2026-09-05", loc); before.end != "2026-09-06 04:00:00.000Z" {
		t.Fatalf("2026-09-05 ends at %s, want where 2026-09-06 starts", before.end)
	}
}

// R2: a run with no today resolves its presets on the reader's day. Kiritimati
// (UTC+14) and Pago Pago (UTC−11) are never on one day.
func TestTodayIsTheReadersDay(t *testing.T) {
	days := map[string]string{}
	for _, name := range []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"} {
		loc := readerZone(name)
		before := time.Now().In(loc).Format(time.DateOnly)
		got, err := todayOf("", "options.today", loc)
		after := time.Now().In(loc).Format(time.DateOnly)
		if err != nil {
			t.Fatal(err)
		}
		day := got.Format(time.DateOnly)
		if day != before && day != after {
			t.Fatalf("%s: today = %s, want %s", name, day, before)
		}
		days[name] = day
	}
	if days["Pacific/Kiritimati"] == days["Pacific/Pago_Pago"] {
		t.Fatalf("both zones read one day: %v", days)
	}
}

// C3: the run endpoint takes options.timeZone (decodeStrict refuses an
// unknown key) and answers in the zone.
func TestRunEndpointTakesTimeZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	Register(z.app)
	sc := tests.ApiScenario{
		Name: "a run in the reader's zone", Method: http.MethodPost, URL: "/api/reports/run",
		Body: strings.NewReader(`{"spec":` + sightingsInPeriod + `,"params":{"period":` + nov14 + `},"options":{"timeZone":"` + chicagoZone + `"}}`),
		Headers:            map[string]string{"Authorization": z.token(t, "ann")},
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{`"note":"start14"`, `"note":"end14"`},
		NotExpectedContent: []string{`"note":"before14"`, `"note":"after14"`},
		TestAppFactory:     func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestBetween|TestDayComparisons|TestZone|TestDateColumnIgnores|TestDateParts|TestTodayIs|TestRunEndpointTakesTimeZone' -count=1`
Expected: FAIL to build: `unknown field TimeZone in struct literal of type Options`, `undefined: readerZone`, `undefined: zoneDay`, `undefined: zonedDay`, `too many arguments in call to todayOf`. (The Q7 test and `TestDateColumnIgnoresTheZone` are guards, C7: red only for the build.)

- [ ] **Step 3: Write `zone.go`**

Create `core/server/reports/zone.go`:

```go
package reports

import (
	"time"

	// A server image without /usr/share/zoneinfo still knows every zone, so a
	// reader's zone never silently reads as UTC (R2).
	_ "time/tzdata"
)

// instantLayout is how PocketBase stores a date or autodate value, so a bound
// instant compares with a stored one as text.
const instantLayout = "2006-01-02 15:04:05.000Z"

// zonedDay is one day of the reader's zone as the UTC instants of the local
// midnight that starts it and of the one that ends it: 23 or 25 hours apart
// across a clock change.
type zonedDay struct{ start, end string }

// utcNames are no name and the names of UTC itself. They read as nil, so a
// UTC reader's statements stay today's; the client's UTC_ZONES
// (core/reports/lib/date-time.ts) is the same set, so the server and the
// client agree on which reader is in UTC.
var utcNames = map[string]bool{"": true, "UTC": true, "Etc/UTC": true, "Etc/GMT": true, "GMT": true}

// readerZone reads the reader's IANA zone. nil is UTC: no name, a name of
// UTC itself (utcNames), a name the server does not know, and "Local", which
// is the server's own zone, never the reader's (R2).
func readerZone(name string) *time.Location {
	if utcNames[name] || name == "Local" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// zoneDay is day (YYYY-MM-DD, already checked by isDay) in loc.
func zoneDay(day string, loc *time.Location) zonedDay {
	d, _ := time.Parse(time.DateOnly, day)
	start := localMidnight(d, loc)
	end := localMidnight(d.AddDate(0, 0, 1), loc)
	return zonedDay{start: start.UTC().Format(instantLayout), end: end.UTC().Format(instantLayout)}
}

// localMidnight is the first instant of day in loc. Where a clock change
// skips midnight (America/Santiago), time.Date may answer an instant of the
// day before; the day then starts when the clock jumps, so step to it. The
// loop ends: each step moves the clock forward, and a skipped whole day
// (Pacific/Apia, 2011) passes the day.
func localMidnight(day time.Time, loc *time.Location) time.Time {
	t := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	want := day.Format(time.DateOnly)
	for t.Format(time.DateOnly) < want {
		t = t.Add(15 * time.Minute)
	}
	return t
}

// withZonedDays gives a set day or date range param its days in the reader's
// zone. A UTC reader's param has none, so its SQL is today's (R1).
func withZonedDays(v paramValue, loc *time.Location) paramValue {
	if loc == nil || v.unset {
		return v
	}
	switch v.typ {
	case ParamDate, ParamAsOf:
		if day, ok := v.value.(string); ok {
			z := zoneDay(day, loc)
			v.dayZ = &z
		}
	case ParamDateRange:
		// All dates has no days to zone.
		if v.from != "" && v.to != "" {
			from, to := zoneDay(v.from, loc), zoneDay(v.to, loc)
			v.fromZ, v.toZ = &from, &to
		}
	}
	return v
}

// zonedSpan is x within [start, end): the instants of whole local days.
func zonedSpan(c fctx, x expr, start, end string) expr {
	return expr{sql: "(" + x.sql + " >= " + c.bind(start) + " AND " + x.sql + " < " + c.bind(end) + ")", typ: Bool}
}

var mirrored = map[string]string{"lt": "gt", "lte": "gte", "gt": "lt", "gte": "lte"}

// zonedCompare compares a date-time with a day of the reader's zone as
// instants (R1): gte is from the day's start, gt from its end, lt before its
// start, lte before its end. A day on the left mirrors. ok is false when the
// comparison is not zoned; the caller then compiles it as before.
func zonedCompare(c fctx, op string, a, b expr) (expr, bool) {
	if a.zoned != nil && b.typ == DateTime {
		a, b, op = b, a, mirrored[op]
	}
	if a.typ != DateTime || b.zoned == nil {
		return expr{}, false
	}
	bound, cmp := b.zoned.start, " >= "
	switch op {
	case "gt":
		bound = b.zoned.end
	case "lt":
		cmp = " < "
	case "lte":
		bound, cmp = b.zoned.end, " < "
	}
	return expr{sql: "(" + a.sql + cmp + c.bind(bound) + ")", typ: Bool}, true
}
```

- [ ] **Step 4: Carry the zone through the run**

In `core/server/reports/run.go`, after the `Today` field (line 42), add:

```go
	// TimeZone is the reader's IANA zone (Intl's resolvedOptions().timeZone).
	// A date range, a date or asOf param, and a range's bound compared with a
	// date-time compare the instants of the reader's local days; a missing or
	// unknown zone, a name of UTC itself ("UTC", "Etc/UTC", "Etc/GMT", "GMT")
	// and "Local" are UTC (readerZone). Buckets and date
	// parts keep the UTC day (Q7).
	TimeZone string `json:"timeZone,omitempty"`
```

Replace line 327:

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Options.Today, "options.today")
```

with:

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Options.Today, "options.today", req.Options.TimeZone)
```

Replace `readerCompiler` and `todayOf` (lines 359–397) with:

```go
// readerCompiler binds a spec for its reader and resolves its params for the
// reader's day (today, given at todayAt) in the reader's zone (timeZone):
// what every read of a report, a run or a param's options, starts from. A
// report never reads as a superuser or as nobody.
func readerCompiler(app core.App, spec *Spec, auth *core.Record, headers map[string]string, given map[string]json.RawMessage, today, todayAt, timeZone string) (*compiler, error) {
	if auth == nil || auth.IsSuperuser() {
		return nil, ErrNoReader
	}
	p, specErr := bind(app, spec)
	if specErr != nil {
		return nil, specErr
	}
	loc := readerZone(timeZone)
	day, err := todayOf(today, todayAt, loc)
	if err != nil {
		return nil, err
	}
	info := listRequestInfo(auth, headers)
	// One read of the fiscal start month serves the presets and the
	// formulas, so a settings change mid-read cannot make them disagree.
	fiscal := fiscalStartOf(p.base, app, info)
	params, specErr := resolveParams(p.spec.Params, given, periodContext{today: day, fiscalStart: fiscal, loc: loc})
	if specErr != nil {
		return nil, specErr
	}
	c := newCompilerWithFiscal(p, app, info, params, fiscal)
	c.given = given
	return c, nil
}

// todayOf reads the reader's day, given at at in the request. With none it
// is today in the reader's zone (loc; nil is UTC).
func todayOf(s, at string, loc *time.Location) (time.Time, error) {
	if s == "" {
		if loc == nil {
			return time.Now().UTC().Truncate(24 * time.Hour), nil
		}
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	if !isDay(s) {
		return time.Time{}, invalid(at, "today is YYYY-MM-DD")
	}
	return time.Parse(time.DateOnly, s)
}
```

In `core/server/reports/options.go`, replace line 319:

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Today, "today")
```

with (Task 5 replaces `""` with `req.TimeZone`, C3):

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Today, "today", "")
```

- [ ] **Step 5: Zone the params**

In `core/server/reports/params.go`, in `paramValue`, after the `currency` field (line 60), add:

```go
	// dayZ, fromZ and toZ are the day, or the range's first and last day, in
	// the reader's zone (withZonedDays); nil for a UTC reader.
	dayZ, fromZ, toZ *zonedDay
```

Replace `periodContext` (lines 63–67) with:

```go
// periodContext is what a preset is resolved against, and the reader's zone
// (nil is UTC) a day of a param is read in.
type periodContext struct {
	today       time.Time
	fiscalStart int
	loc         *time.Location
}
```

In `resolveParams`, replace line 169:

```go
		out[d.ID] = v
```

with:

```go
		out[d.ID] = withZonedDays(v, pc.loc)
```

In `paramExpr`, replace line 484:

```go
		return expr{rng: true, lo: c.bind(v.from), hi: c.bind(v.to)}, nil
```

with:

```go
		return expr{rng: true, lo: c.bind(v.from), hi: c.bind(v.to), zonedFrom: v.fromZ, zonedTo: v.toZ}, nil
```

and line 508:

```go
	return expr{sql: c.bind(v.value), typ: typ}, nil
```

with:

```go
	return expr{sql: c.bind(v.value), typ: typ, zoned: v.dayZ}, nil
```

In `rangeBound`, replace lines 522–526:

```go
	day := e.lo
	if bound == "to" {
		day = e.hi
	}
	return expr{sql: day, typ: Date}, nil
```

with:

```go
	day, zoned := e.lo, e.zonedFrom
	if bound == "to" {
		day, zoned = e.hi, e.zonedTo
	}
	return expr{sql: day, typ: Date, zoned: zoned}, nil
```

- [ ] **Step 6: Compare a date-time with a zoned day**

In `core/server/reports/formula.go`, in `expr`, after the `scan` field (line 65), add:

```go
	// zoned is a day of the reader's zone (a date or asOf param, or a range's
	// bound); zonedFrom and zonedTo are a date range's first and last day. A
	// comparison or a between of a date-time with them compares the instants
	// of those local days (R1). All nil for a UTC reader, whose SQL stays as
	// it was.
	zoned, zonedFrom, zonedTo *zonedDay
```

In `operatorExpr`, replace line 852:

```go
		return expr{sql: "(" + args[0].sql + " " + comparisons[n.Op] + " " + args[1].sql + ")", typ: Bool}, nil
```

with:

```go
		if e, ok := zonedCompare(c, n.Op, args[0], args[1]); ok {
			return e, nil
		}
		return expr{sql: "(" + args[0].sql + " " + comparisons[n.Op] + " " + args[1].sql + ")", typ: Bool}, nil
```

and line 854:

```go
		return between(args, at)
```

with:

```go
		return between(c, args, at)
```

Replace `between` (lines 1068–1094) with:

```go
func between(c fctx, args []expr, at string) (expr, error) {
	// A date range has no single value to compare: it is only ever the bounds.
	for i, a := range args {
		if a.rng && (len(args) != 2 || i != 1) {
			return expr{}, invalid(fmt.Sprintf("%s.args.%d", at, i), "a date range param is the second of two between values")
		}
	}
	switch {
	case len(args) == 2 && args[1].rng:
		x, r := args[0], args[1]
		if !isDateLike(x.typ) {
			return expr{}, invalid(at+".args.0", "a date range holds dates")
		}
		if r.all {
			// All dates: every row, a row with no day too (R1).
			return expr{sql: "1", typ: Bool}, nil
		}
		if x.typ == DateTime && r.zonedFrom != nil {
			return zonedSpan(c, x, r.zonedFrom.start, r.zonedTo.end), nil
		}
		day := "substr(" + x.sql + ", 1, 10)"
		return expr{sql: "(" + day + " >= " + r.lo + " AND " + day + " <= " + r.hi + ")", typ: Bool}, nil
	case len(args) == 3:
		if !ordered(args[0].typ, args[1].typ) || !ordered(args[0].typ, args[2].typ) {
			return expr{}, invalid(at, "between compares numbers, dates or text")
		}
		if lo, hi := args[1].zoned, args[2].zoned; args[0].typ == DateTime && lo != nil && hi != nil {
			return zonedSpan(c, args[0], lo.start, hi.end), nil
		}
		return expr{sql: "(" + args[0].sql + " >= " + args[1].sql + " AND " + args[0].sql + " <= " + args[2].sql + ")", typ: Bool}, nil
	}
	return expr{}, invalid(at, "between takes a value and a date range param, or a value, a low and a high")
}
```

Run `gofmt -w core/server/reports/zone.go core/server/reports/run.go core/server/reports/params.go core/server/reports/formula.go core/server/reports/options.go core/server/reports/zonerange_test.go`.

- [ ] **Step 7: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestBetween|TestDayComparisons|TestZone|TestDateColumnIgnores|TestDateParts|TestTodayIs|TestRunEndpointTakesTimeZone' -count=1`
Expected: PASS. `TestZoneUnknownOrLocalIsUTC` and `TestDateColumnIgnoresTheZone` pass only through `sameShape`: two builds of one rule CTE bind PocketBase's random placeholder names (`{:fV4ozOBf0la}` in one run, `{:fag2nta1JOD}` in the next), so a raw `reflect.DeepEqual` of the statements always fails. If one of the two still fails, print the two `sameShape` results and find the part that differs. The random names are in the SQL placeholders, in the join aliases, and in the param keys the SQL does not use (dropped); a random name `sameShape` still misses is added to it (and named in the hand-back). A difference in the engine's own SQL or params is a real regression: fix it in the engine.

The `Etc/UTC`, `Etc/GMT` and `GMT` cases bite: with `readerZone` reading only `"UTC"` as UTC (the first draft of this plan), `Etc/UTC` loads a real location, the SQL is zoned, and the test fails with `zone "Etc/UTC": the statements differ…`. Prove it once: change `if utcNames[name] || name == "Local"` to `if name == "" || name == "UTC" || name == "Local"`, run the test, see that failure, and undo the change with the Edit tool. Write the failing line into the hand-back.

- [ ] **Step 8: Run the whole reports package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: `ok  tinycld.org/core/reports`. A failure here is a regression of a UTC run (R1 keeps its SQL as today's; PocketBase's random rule names aside, which `sameShape` covers): diagnose it at its source.

- [ ] **Step 9: Hand the diff to the driver**

No commit. The diff: `zone.go`, `zonerange_test.go` (new), `run.go`, `params.go`, `formula.go`, `options.go`.

---

### Task 4b: T2 Go — a typed day in the reader's zone (Q3)

The human's answer to Q3 (2026-11-15): a day typed in a condition (`gte(seen, {value: "2026-11-14"})`, `between(seen, {value: d1}, {value: d2})`) is a day of the reader's zone too, when the other side is a `DateTime`. `literal` gives a text literal that is a day (`isDay`) its zoned day on `expr.zoned`, as `paramExpr` does for a `date` param; `zonedCompare` and `between` (Task 4) then compare it as instants with no change. A literal only reaches the compiler through `fctx`, so `fctx` gains `readerDay(day string) *zonedDay`, which each context reads from its compiler's zone (`compiler.loc`, set by `readerCompiler`). A UTC reader and a dry compile have no `loc`, so their SQL stays as today's (R1). A typed day against a `Date` or text column is not zoned (`zonedCompare` needs a `DateTime` side).

This task touches 12 files, but 8 of them gain one line each: the `fctx` method must land in every context at once, or the package does not compile. About 130 lines with the tests.

**Files:**
- Modify: `core/server/reports/formula.go` (`expr.zoned` comment, Task 4 Step 6; `fctx`, after `fiscalStart() string`, line 287 before Task 4, about 293 after it; `literal`, the `case string:` lines 541–542 before Task 4, about 547–548 after it)
- Modify: `core/server/reports/zone.go` (`(*compiler).readerDay`, at the end)
- Modify: `core/server/reports/rows.go` (`compiler`, after `fiscal int`, line 22; import `time`)
- Modify: `core/server/reports/run.go` (`readerCompiler`: `c.loc = loc`; the `Options.TimeZone` comment)
- Modify: one `readerDay` line beside `fiscalStart` in `detail.go` (`rowsCtx`, line 344), `heading.go` (`headingCtx`, line 46), `join.go` (`joinCtx`, line 169; `fillCtx` embeds it), `layout.go` (`layoutCtx`, line 965), `levels.go` (`levelCtx`, line 1346), `lookup.go` (`lookupCtx`, line 583), `formula_cases_test.go` (`tableCtx`, line 152; `tableLookupCtx`, line 248)
- Test: `core/server/reports/zonerange_test.go` (append)

**Interfaces:**
- Consumes: `zoneDay`, `zonedDay`, `zonedCompare`, `between(c, args, at)`, `isDay`, `readerCompiler`'s `loc` (Task 4); test helpers `addZooSightings`, `sightingsWhere`, `opOf`, `seenLeaf`, `dayLeaf`, `nov14`, `chicagoZone`, `sameShape`, `observeReads`, `requireDetails` (Task 4 and the package's tests).
- Produces: `compiler.loc *time.Location`; `(c *compiler) readerDay(day string) *zonedDay` (nil when `loc` is nil); `fctx.readerDay(day string) *zonedDay`; `expr.zoned` on a text literal that is a day, for a reader with a zone. Test helper `typedDay(day string) string` (`{"value":"<day>"}`).

- [ ] **Step 1: Write the failing tests**

Append to `core/server/reports/zonerange_test.go`:

```go
// typedDay is a day typed in a condition: a builder filter's {value}.
func typedDay(day string) string { return `{"value":"` + day + `"}` }

// Q3: a day typed in a condition is a day of the reader's zone, as a date
// param is: a comparison or a between of a date-time with it compares the
// instants of that local day, with the same edges (R1).
func TestTypedDayComparisonsOfADateTimeInTheZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	all := []any{"before8", "start8", "end8", "after8", "before1", "start1", "end1", "after1", "before14", "start14", "end14", "after14"}
	day := typedDay("2026-11-14")
	params := map[string]string{"day": `"2026-11-14"`, "period": nov14}
	betweenOf := func(lo, hi string) string {
		return `{"op":"between","args":[` + seenLeaf + `,` + lo + `,` + hi + `]}`
	}
	for name, c := range map[string]struct {
		where string
		want  []any
	}{
		"gte a typed day":                   {opOf("gte", seenLeaf, day), []any{"start14", "end14", "after14"}},
		"gt a typed day":                    {opOf("gt", seenLeaf, day), []any{"after14"}},
		"lt a typed day":                    {opOf("lt", seenLeaf, day), all[:9]},
		"lte a typed day":                   {opOf("lte", seenLeaf, day), all[:11]},
		"a typed day lte":                   {opOf("lte", day, seenLeaf), []any{"start14", "end14", "after14"}},
		"a typed day gt":                    {opOf("gt", day, seenLeaf), all[:9]},
		"between two typed days":            {betweenOf(day, day), []any{"start14", "end14"}},
		"between typed days, the DST end":   {betweenOf(typedDay("2026-11-01"), typedDay("2026-11-01")), []any{"start1", "end1"}},
		"between typed days, the DST start": {betweenOf(typedDay("2026-03-08"), typedDay("2026-03-08")), []any{"start8", "end8"}},
		"between a typed day and a param":   {betweenOf(typedDay("2026-11-01"), dayLeaf), []any{"start1", "end1", "after1", "before14", "start14", "end14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", sightingsWhere(c.where), Options{TimeZone: chicagoZone}, params)
			requireDetails(t, res, "note", c.want...)
		})
	}
}

// R1: for a UTC reader (no zone, a name of UTC itself, a name the server does
// not know) a typed day compares as before, with the statements of a run
// with no zone; a Chicago run's statements differ, so the compare can see a
// zone.
func TestTypedDayWithNoZoneIsUTC(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	spec := sightingsWhere(opOf("gte", seenLeaf, typedDay("2026-11-14")))
	reads := func(zone string, want ...any) []readStatement {
		seen := observeReads(t)
		requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: zone}, nil), "note", want...)
		return sameShape(*seen)
	}
	none := reads("", "before14", "start14", "end14", "after14")
	if chicago := reads(chicagoZone, "start14", "end14", "after14"); reflect.DeepEqual(chicago, none) {
		t.Fatal("a Chicago run sent the statements of a run with no zone: the compare cannot see a zone")
	}
	for _, zone := range []string{"UTC", "Not/AZone"} {
		if got := reads(zone, "before14", "start14", "end14", "after14"); !reflect.DeepEqual(got, none) {
			t.Fatalf("zone %q: the statements differ from a run with no zone:\n got  %v\n want %v", zone, got, none)
		}
	}
}

// Text that is not a day (an instant) is not zoned: it compares as before.
func TestTypedInstantIsNotZoned(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	spec := sightingsWhere(opOf("gte", seenLeaf, typedDay("2026-11-14 06:00:00.000Z")))
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, nil), "note", "start14", "end14", "after14")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, nil), "note", "start14", "end14", "after14")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed the statements of a typed instant:\n got  %v\n want %v", got, utc)
	}
}

// A Date field is a day: a typed day compares with it as before, zone or not.
func TestTypedDayAgainstADateColumnIgnoresTheZone(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":4,"source":"zoo.feedings",
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"gte","args":[{"path":[{"field":"fed"}]},{"value":"2026-01-20"}]},
		"sort":[{"col":"note"}]}`
	seen := observeReads(t)
	requireDetails(t, z.run(t, "ann", spec, Options{}, nil), "note", "f2", "f3", "f4")
	utc := sameShape(*seen)
	*seen = nil
	requireDetails(t, z.run(t, "ann", spec, Options{TimeZone: chicagoZone}, nil), "note", "f2", "f3", "f4")
	if got := sameShape(*seen); !reflect.DeepEqual(got, utc) {
		t.Fatalf("a zone changed a date column's statements:\n got  %v\n want %v", got, utc)
	}
}
```

(`ann` sees the feedings of leo and zed only: f1 Jan 5, f2 Feb 10, f3 Jan 20, f4 Mar 3.)

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestTypedDay|TestTypedInstant' -count=1`
Expected: FAIL. Each of the ten `TestTypedDayComparisonsOfADateTimeInTheZone` cases fails with the UTC rows, for example `gte a typed day`: `detail note = []interface {}{"before14", "start14", "end14", "after14"}`, and `between two typed days`: `detail note = []interface {}(nil)` (today a typed day compares as text, and no instant of Nov 14 is `<= "2026-11-14"`). `TestTypedDayWithNoZoneIsUTC` fails on its Chicago run with the same four rows. `TestTypedDayAgainstADateColumnIgnoresTheZone` and `TestTypedInstantIsNotZoned` pass: they are guards (C7) that the change must keep. The instant guard bites: with `if isDay(x)` made `if true`, `zoneDay` cannot parse the text and starts the day in year 1, and the Chicago run returns all twelve sightings.

- [ ] **Step 3: The compiler holds the reader's zone**

In `core/server/reports/rows.go`, add `"time"` to the standard imports (after `"strings"`), and in `compiler`, after `fiscal int` (line 22), add:

```go
	// loc is the reader's zone, nil for a UTC reader and a dry compile: a
	// day typed in a condition is a day of it (readerDay, R1).
	loc *time.Location
```

In `core/server/reports/run.go`, in `readerCompiler` (Task 4 Step 4), after `c.given = given`, add:

```go
	c.loc = loc
```

and in the `Options.TimeZone` comment, replace

```go
	// A date range, a date or asOf param, and a range's bound compared with a
	// date-time compare the instants of the reader's local days; a missing or
```

with:

```go
	// A date range, a date or asOf param, a range's bound and a day typed in
	// a condition compared with a date-time compare the instants of the
	// reader's local days; a missing or
```

At the end of `core/server/reports/zone.go`, add:

```go
// readerDay is day (YYYY-MM-DD) in the reader's zone, nil for a UTC reader
// and a dry compile, whose SQL stays as it was (R1).
func (c *compiler) readerDay(day string) *zonedDay {
	if c.loc == nil {
		return nil
	}
	z := zoneDay(day, c.loc)
	return &z
}
```

- [ ] **Step 4: Every formula context reads it**

In `core/server/reports/formula.go`, in `fctx`, after `fiscalStart() string` (line 287 before Task 4, about 293 after it), add:

```go
	// readerDay is day (YYYY-MM-DD) in the reader's zone, nil for a UTC
	// reader (R1).
	readerDay(day string) *zonedDay
```

Add one method beside each context's `fiscalStart`:

```go
func (rc *rowsCtx) readerDay(day string) *zonedDay { return rc.r.c.readerDay(day) }      // detail.go, after line 344
func (h *headingCtx) readerDay(day string) *zonedDay { return h.c.readerDay(day) }       // heading.go, after line 46
func (j *joinCtx) readerDay(day string) *zonedDay { return j.r.c.readerDay(day) }        // join.go, after line 169
func (lc *layoutCtx) readerDay(day string) *zonedDay { return lc.c.readerDay(day) }      // layout.go, after line 965
func (lc *levelCtx) readerDay(day string) *zonedDay { return lc.lv.c.readerDay(day) }    // levels.go, after line 1346
func (l *lookupCtx) readerDay(day string) *zonedDay { return l.r.c.readerDay(day) }      // lookup.go, after line 583
func (l *tableLookupCtx) readerDay(day string) *zonedDay { return l.outer.readerDay(day) } // formula_cases_test.go, after line 248
```

(The trailing comments say where each line goes; do not copy them.) In `formula_cases_test.go`, before `func (c *tableCtx) fiscalStart() string {` (line 152), add:

```go
// readerDay: the formula table runs in UTC.
func (c *tableCtx) readerDay(string) *zonedDay { return nil }

```

`fillCtx` (fill.go) embeds `*joinCtx` and needs no line.

- [ ] **Step 5: Zone a typed day**

In `core/server/reports/formula.go`, in `literal`, replace (lines 541–542 before Task 4, about 547–548 after it):

```go
	case string:
		return expr{sql: c.bind(x), typ: Text, text: &x}, nil
```

with:

```go
	case string:
		// A day typed in a condition is a day of the reader's zone, as a
		// date param is (R1, Q3); it stays text for every other use.
		var zoned *zonedDay
		if isDay(x) {
			zoned = c.readerDay(x)
		}
		return expr{sql: c.bind(x), typ: Text, text: &x, zoned: zoned}, nil
```

In `expr` (Task 4 Step 6), replace the first two lines of the `zoned` comment:

```go
	// zoned is a day of the reader's zone (a date or asOf param, or a range's
	// bound); zonedFrom and zonedTo are a date range's first and last day. A
```

with:

```go
	// zoned is a day of the reader's zone (a date or asOf param, a range's
	// bound, or a day typed in a condition); zonedFrom and zonedTo are a
	// date range's first and last day. A
```

Then rewrap that comment to the file's width. Run `gofmt -w core/server/reports/formula.go core/server/reports/zone.go core/server/reports/rows.go core/server/reports/run.go core/server/reports/detail.go core/server/reports/heading.go core/server/reports/join.go core/server/reports/layout.go core/server/reports/levels.go core/server/reports/lookup.go core/server/reports/formula_cases_test.go core/server/reports/zonerange_test.go` (it realigns each block of one-line methods).

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestTypedDay|TestTypedInstant|TestBetween|TestDayComparisons|TestZone|TestDateColumnIgnores|TestDateParts' -count=1`
Expected: PASS (the ten typed-day cases, the three guards, and Task 4's tests unchanged).

- [ ] **Step 7: Run the whole reports package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: `ok  tinycld.org/core/reports` (about 95 s). No other test runs with a zone, so a failure here is a regression of a UTC run or of a literal: diagnose it at its source.

- [ ] **Step 8: Hand the diff to the driver**

No commit. The diff: `formula.go`, `zone.go`, `rows.go`, `run.go`, `detail.go`, `heading.go`, `join.go`, `layout.go`, `levels.go`, `lookup.go`, `formula_cases_test.go`, `zonerange_test.go`.

---

### Task 5: T2 wire — param options and the client

**Files:**
- Modify: `core/server/reports/options.go` (`OptionsRequest`, after `Today`, line 270; line 319; `paramOptionsBody`, after `Today`, line 601; `handleParamOptions`, line 622)
- Create: `core/server/reports/zoneoptions_test.go`
- Modify: `core/reports/lib/run-client.ts` (`RunOptions`, line 37; `runReport`, lines 148–158; `ParamOptionsBody`, line 258; `fetchParamOptions`, lines 262–273)
- Test: `core/reports/lib/__tests__/run-client.test.ts`

**Interfaces:**
- Consumes: `addZooSightings`, `nov14`, `chicagoZone` (Task 4); `z.options`, `optionLabels` (options_test.go 162, 176); `readerCompiler(…, timeZone string)` (Task 4); `readerTimeZone()` (Task 2).
- Produces: `OptionsRequest.TimeZone string`; `paramOptionsBody.TimeZone string` (JSON `timeZone`); client `RunOptions.timeZone?: string`, `ParamOptionsBody.timeZone?: string`; `runReport` and `fetchParamOptions` add `timeZone: readerTimeZone()` to the body they send, never to the body a caller keys a query by (R3).

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/zoneoptions_test.go`:

```go
package reports

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// sightingNotes offers the notes of the sightings seen in the period, a
// date-time-filtered option list.
const sightingNotes = `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"period","type":"dateRange"},
		{"id":"notes","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.sightings",
			"value":[{"field":"note"}],"label":[{"field":"note"}],
			"where":{"op":"between","args":[{"path":[{"field":"seen"}]},{"param":"period"}]}}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}]}`

func TestParamOptionsReadInTheReadersZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	given := map[string]json.RawMessage{"period": json.RawMessage(nov14)}
	for name, c := range map[string]struct {
		zone string
		want []string
	}{
		"Chicago":         {chicagoZone, []string{"end14", "start14"}},
		"no zone":         {"", []string{"before14", "start14"}},
		"an unknown zone": {"Not/AZone", []string{"before14", "start14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.options(t, "ann", sightingNotes, OptionsRequest{Param: "notes", Params: given, TimeZone: c.zone})
			if got := optionLabels(res); !slices.Equal(got, c.want) {
				t.Fatalf("options = %v, want %v", got, c.want)
			}
		})
	}
}

// The endpoint takes "timeZone" (decodeStrict refuses an unknown key) and
// answers in the zone.
func TestParamOptionsEndpointTakesTimeZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	Register(z.app)
	body := `{"spec":` + sightingNotes + `,"param":"notes","params":{"period":` + nov14 + `},"timeZone":"` + chicagoZone + `"}`
	sc := tests.ApiScenario{
		Name: "options in the reader's zone", Method: http.MethodPost, URL: "/api/reports/param-options",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": z.token(t, "ann")},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"label":"start14"`, `"label":"end14"`},
		NotExpectedContent:    []string{`"label":"before14"`, `"label":"after14"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}
```

- [ ] **Step 2: Run the Go tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestParamOptions.*Zone|TestParamOptionsEndpointTakesTimeZone' -count=1`
Expected: FAIL to build: `unknown field TimeZone in struct literal of type OptionsRequest`. (With that line removed, the endpoint test fails with status 400, `unknown field "timeZone"`.)

- [ ] **Step 3: Take the zone in param options**

In `core/server/reports/options.go`, in `OptionsRequest`, after `Today  string` (line 270), add:

```go
	// TimeZone is the reader's IANA zone, as Options.TimeZone: a where that
	// compares a date-time with a day or range param reads its local days.
	TimeZone string
```

Replace line 319 (Task 4's `""`):

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Today, "today", "")
```

with:

```go
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Today, "today", req.TimeZone)
```

In `paramOptionsBody`, after `Today    string                     `json:"today"`` (line 601), add:

```go
	TimeZone string                     `json:"timeZone"`
```

In `handleParamOptions`, replace line 622:

```go
		Values: body.Values, Limit: body.Limit, Params: body.Params, Today: body.Today,
```

with:

```go
		Values: body.Values, Limit: body.Limit, Params: body.Params, Today: body.Today, TimeZone: body.TimeZone,
```

Run `gofmt -w core/server/reports/options.go core/server/reports/zoneoptions_test.go`.

- [ ] **Step 4: Run the Go tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestParamOptions|TestRunEndpoint' -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing client tests**

In `core/reports/lib/__tests__/run-client.test.ts`, add `runReport` to the import list from `'../run-client'` (after `rowsKeyOf`), add `type ParamOptionsBody` (after `pageRowCount`), and add `import { readerTimeZone } from '../date-time'`.

Replace the body of the existing test "posts the request to the param-options route and returns the page" (lines 103–112) so the sent body carries the zone (C6):

```ts
        const signal = new AbortController().signal
        const body = { reportId: 'r1', param: 'accounts', search: 'ba', today: '2026-11-01' }
        await expect(fetchParamOptions(body, signal)).resolves.toEqual(page)
        expect(send).toHaveBeenCalledWith(PARAM_OPTIONS_PATH, {
            method: 'POST',
            body: { ...body, timeZone: readerTimeZone() },
            signal,
            requestKey: null,
        })
        expect(PARAM_OPTIONS_PATH).toBe('/api/reports/param-options')
```

Append:

```ts
describe("the reader's time zone (T2)", () => {
    const zone = new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
    const empty: RunResult = {
        columns: [],
        rows: [],
        notices: [],
        topics: [],
        page: null,
        params: [],
    }

    it('rides on every run, outside the rows and fold keys', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(empty)
        const body: RunBody = { reportId: 'r1', params: {}, options: { today: '2026-11-06' } }
        await runReport(body)
        const sent = (send.mock.calls[0]?.[1] as { body: RunBody }).body
        expect(sent.options.timeZone).toBe(zone)
        expect(sent.options.today).toBe('2026-11-06')
        // Read once per app start (R3): the zone never splits a cached run.
        expect(body.options.timeZone).toBeUndefined()
        expect(rowsKeyOf(body)).toBe('report:r1')
        expect(foldKeyOf(body)).not.toContain('timeZone')
    })

    it('rides on every param-options request', async () => {
        send.mockReset()
        send.mockResolvedValueOnce({ options: [], groups: [] })
        const body: ParamOptionsBody = { reportId: 'r1', param: 'accounts', today: '2026-11-01' }
        await fetchParamOptions(body)
        const sent = (send.mock.calls[0]?.[1] as { body: ParamOptionsBody }).body
        expect(sent.timeZone).toBe(zone)
        expect(body.timeZone).toBeUndefined()
    })
})
```

- [ ] **Step 6: Run the client tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/run-client.test.ts`
Expected: FAIL in three cases: "posts the request to the param-options route…" (the sent body has no `timeZone`), "rides on every run…" (`expected undefined to be '<zone>'`), "rides on every param-options request" (same). Every other case passes.

- [ ] **Step 7: Send the zone**

In `core/reports/lib/run-client.ts`, add after line 5 (`import { ClientResponseError } from 'pocketbase'`):

```ts
import { readerTimeZone } from './date-time'
```

In `RunOptions`, after `today: string` (line 37), add:

```ts
    /**
     * The reader's IANA zone (T2), added by runReport when it sends the body, so it
     * is never in the rows or fold key.
     */
    timeZone?: string
```

Replace lines 148–158 of `runReport`:

```ts
export async function runReport(body: RunBody, signal?: AbortSignal): Promise<RunOutcome> {
    try {
        // requestKey null: two reports may run at once (a frozen tab, the builder's
        // preview); the SDK would otherwise cancel one for the other.
        const result = await pb.send<RunResult>(RUN_PATH, {
            method: 'POST',
            body,
            signal,
            requestKey: null,
        })
```

with:

```ts
export async function runReport(body: RunBody, signal?: AbortSignal): Promise<RunOutcome> {
    // The zone goes on the sent body only: it is read once per app start (R3), so
    // it never needs to split a cached run.
    const sent = { ...body, options: { ...body.options, timeZone: readerTimeZone() } }
    try {
        // requestKey null: two reports may run at once (a frozen tab, the builder's
        // preview); the SDK would otherwise cancel one for the other.
        const result = await pb.send<RunResult>(RUN_PATH, {
            method: 'POST',
            body: sent,
            signal,
            requestKey: null,
        })
```

In `ParamOptionsBody`, after `today: string` (line 258), add:

```ts
    /** The reader's IANA zone (T2), added by fetchParamOptions when it sends the body. */
    timeZone?: string
```

Replace lines 267–272 of `fetchParamOptions`:

```ts
    return pb.send<ParamOptionsPage>(PARAM_OPTIONS_PATH, {
        method: 'POST',
        body,
        signal,
        requestKey: null,
    })
```

with:

```ts
    return pb.send<ParamOptionsPage>(PARAM_OPTIONS_PATH, {
        method: 'POST',
        body: { ...body, timeZone: readerTimeZone() },
        signal,
        requestKey: null,
    })
```

- [ ] **Step 8: Run the client tests to see them pass, then typecheck and lint**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/run-client.test.ts`
Expected: PASS.
Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check core/reports/lib/run-client.ts core/reports/lib/__tests__/run-client.test.ts`
Expected: no errors.

- [ ] **Step 9: Hand the diff to the driver**

No commit. The diff: `options.go`, `zoneoptions_test.go` (new), `run-client.ts`, `run-client.test.ts`.

---

### Task 6: D5 — the manifest field and the generator

**Files:**
- Modify: `core/lib/packages/types.ts` (add `ReportPanelDecl`; `PackageManifest.reportPanels`, after `setupSteps`, line 310)
- Modify: `scripts/load-manifest.ts` (import, line 4; `PackageManifest.reportPanels`, after `setupSteps`, line 91)
- Create: `scripts/gen-report-panels.ts`
- Modify: `scripts/generate.ts` (import, after line 37; write after `package-icons.ts`, line 644)
- Modify: `core/types/app-generated.d.ts` (append a module), `core/tests/unit-setup.ts` (after line 66)
- Create: `core/reports/hooks/usePanelTitle.ts` (the props type only; Task 8 adds the hook)
- Create: `core/reports/lib/record-panel.ts` (the entry type only; Task 7 adds the rules)
- Create: `scripts/__tests__/gen-report-panels.test.ts`

**Interfaces:**
- Consumes: `assertSafeExportSubpath`, `assertSafeImportField` (scripts/validate-generated-field.ts 23, 37); `GENERATED_DIR`, `features` in `generate.ts` `main`.
- Produces: `interface ReportPanelDecl { path: string; component: string }` (core/lib/packages/types.ts); `PackageManifest.reportPanels?: ReportPanelDecl[]` in both manifest types; `buildReportPanelsSource(features: { name: string; manifest: { slug: string; reportPanels?: ReportPanelDecl[] } }[]): string`; the generated `lib/generated/report-panels.ts` (`export const reportPanels: ReportPanelEntry[] = [{ slug, path, load: () => import('<package>/<component>') }]`); `type ReportPanelProps = { segments: string[] }` (core/reports/hooks/usePanelTitle.ts); `type ReportPanelEntry = { slug: string; path: string; load: () => Promise<{ default: ComponentType<ReportPanelProps> }> }` (core/reports/lib/record-panel.ts); ambient module `@tinycld/app-generated/report-panels`; unit-setup mock `{ reportPanels: [] }`.

- [ ] **Step 1: Write the failing test**

Create `scripts/__tests__/gen-report-panels.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ReportPanelDecl } from '../../core/lib/packages/types'
import { buildReportPanelsSource } from '../gen-report-panels'

const pkg = (name: string, slug: string, reportPanels?: ReportPanelDecl[]) => ({
    name,
    manifest: { slug, reportPanels },
})

const gizmos = pkg('@acme/gizmos', 'gizmos', [
    { path: 'history', component: 'components/history/Panel' },
])
const widgets = pkg('@acme/widgets', 'widgets', [
    { path: 'audit/log', component: 'components/AuditPanel' },
])
const plain = pkg('@acme/plain', 'plain')

describe('buildReportPanelsSource', () => {
    it("lists a package's panel with a lazy import of its component", () => {
        const src = buildReportPanelsSource([gizmos])
        expect(src).toContain(
            "import type { ReportPanelEntry } from '@tinycld/core/reports/lib/record-panel'"
        )
        expect(src).toContain('export const reportPanels: ReportPanelEntry[] = [')
        expect(src).toContain(
            `{ slug: "gizmos", path: "history", load: () => import('@acme/gizmos/components/history/Panel') },`
        )
    })

    it("lists every package's panels in package order", () => {
        const src = buildReportPanelsSource([widgets, plain, gizmos])
        const first = src.indexOf('slug: "widgets"')
        expect(first).toBeGreaterThan(-1)
        expect(src.indexOf('slug: "gizmos"')).toBeGreaterThan(first)
        expect(src).toContain("import('@acme/widgets/components/AuditPanel')")
        expect(src).not.toContain('"plain"')
    })

    it('emits an empty list when no package declares a panel', () => {
        for (const features of [[], [plain]]) {
            const src = buildReportPanelsSource(features)
            expect(src).toContain('export const reportPanels: ReportPanelEntry[] = []')
            expect(src).not.toContain('import(')
        }
    })

    it('refuses a component outside the package', () => {
        for (const component of ['../other/Panel', '/abs/Panel', "x'); evil('"]) {
            const bad = pkg('@acme/gizmos', 'gizmos', [{ path: 'history', component }])
            expect(() => buildReportPanelsSource([bad])).toThrow(/reportPanels\[\]\.component/)
        }
    })

    it('refuses a path that is not slash-joined segments of letters, digits, _ and -', () => {
        for (const path of ['', '/history', 'history/', 'a//b', '..', 'a/../b', 'his tory', 'his.tory']) {
            const bad = pkg('@acme/gizmos', 'gizmos', [{ path, component: 'components/Panel' }])
            expect(() => buildReportPanelsSource([bad])).toThrow(/reportPanels\[\]\.path/)
        }
    })

    it('refuses two panels of one package on one path', () => {
        const twice = pkg('@acme/gizmos', 'gizmos', [
            { path: 'history', component: 'components/A' },
            { path: 'history', component: 'components/B' },
        ])
        expect(() => buildReportPanelsSource([twice])).toThrow(/"history" twice/)
        // Two packages may each have a panel at one path: the slug sets them apart.
        expect(() => buildReportPanelsSource([gizmos, pkg('@acme/other', 'other', [{ path: 'history', component: 'components/P' }])])).not.toThrow()
    })
})
```

- [ ] **Step 2: Run the test to see it fail**

Run (from `/Users/nas/tmp/tinycld/books-reports/tinycld`): `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run scripts/__tests__/gen-report-panels.test.ts`
Expected: FAIL with `Failed to resolve import "../gen-report-panels"`.

- [ ] **Step 3: Declare the manifest field**

In `core/lib/packages/types.ts`, before `export interface PackageManifest` (line 3), add:

```ts
/**
 * A record panel a package draws beside a report (manifest `reportPanels`, D5). A
 * report's detail row whose `open` is `<slug>/<path>/<segments…>` opens
 * `component` in the report's side panel on a window of 1360 px or more, else
 * pushes the path. `path` is one or more segments of `[A-Za-z0-9_-]` joined by
 * `/`. `component` is a module path under the package's `tinycld/<slug>/`,
 * resolved as `<package name>/<component>` through the package's exports map; its
 * default export is a React component that takes `ReportPanelProps`
 * (`@tinycld/core/reports/hooks/usePanelTitle`).
 */
export interface ReportPanelDecl {
    path: string
    component: string
}
```

and in `PackageManifest`, after `setupSteps` (line 310):

```ts

    /**
     * Record panels this package draws beside a report (ReportPanelDecl). The
     * generator lists them in `lib/generated/report-panels.ts`.
     */
    reportPanels?: ReportPanelDecl[]
```

In `scripts/load-manifest.ts`, after line 4 add:

```ts
import type { ReportPanelDecl } from '../core/lib/packages/types'
```

and in its `PackageManifest`, after `setupSteps` (line 91):

```ts
    // Record panels beside a report. See ReportPanelDecl in core/lib/packages/types.ts.
    reportPanels?: ReportPanelDecl[]
```

- [ ] **Step 4: Write the generator**

Create `scripts/gen-report-panels.ts`:

```ts
import type { ReportPanelDecl } from '../core/lib/packages/types'
import { assertSafeExportSubpath, assertSafeImportField } from './validate-generated-field'

// lib/generated/report-panels.ts: every installed package's record panels (manifest
// `reportPanels`, D5), each a lazy import of its component. Core reads the list in
// one module (core/reports/lib/registered-panels.ts), so no package is named in core.

export interface ReportPanelFeature {
    name: string
    manifest: { slug: string; reportPanels?: ReportPanelDecl[] }
}

// One or more segments of letters, digits, _ and -, joined by '/': the path is
// matched against a row's open path segment by segment.
const PANEL_PATH = /^[A-Za-z0-9_-]+(\/[A-Za-z0-9_-]+)*$/

function checkPanels(feature: ReportPanelFeature): void {
    assertSafeImportField('name', feature.name)
    const paths = new Set<string>()
    for (const panel of feature.manifest.reportPanels ?? []) {
        assertSafeExportSubpath('reportPanels[].component', panel.component)
        if (!PANEL_PATH.test(panel.path)) {
            throw new Error(
                `Refusing to generate: ${feature.name} manifest field "reportPanels[].path" has the value ${JSON.stringify(
                    panel.path
                )}. A panel path is segments of [A-Za-z0-9_-] joined by '/'.`
            )
        }
        if (paths.has(panel.path)) {
            throw new Error(
                `Refusing to generate: ${feature.name} declares the reportPanels path ${JSON.stringify(panel.path)} twice.`
            )
        }
        paths.add(panel.path)
    }
}

function panelLine(feature: ReportPanelFeature, panel: ReportPanelDecl): string {
    const slug = JSON.stringify(feature.manifest.slug)
    return `    { slug: ${slug}, path: ${JSON.stringify(panel.path)}, load: () => import('${feature.name}/${panel.component}') },`
}

export function buildReportPanelsSource(features: ReportPanelFeature[]): string {
    for (const feature of features) checkPanels(feature)
    const entries = features.flatMap(feature =>
        (feature.manifest.reportPanels ?? []).map(panel => panelLine(feature, panel))
    )
    const list =
        entries.length === 0
            ? ['export const reportPanels: ReportPanelEntry[] = []']
            : ['export const reportPanels: ReportPanelEntry[] = [', ...entries, ']']
    return `${[
        '// Auto-generated by scripts/generate.ts — do not edit',
        "import type { ReportPanelEntry } from '@tinycld/core/reports/lib/record-panel'",
        '',
        ...list,
    ].join('\n')}\n`
}
```

- [ ] **Step 5: Run the test to see it pass**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run scripts/__tests__/gen-report-panels.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 6: Write the file from `generate.ts`, and declare the types core reads**

In `scripts/generate.ts`, after line 37 (`import { buildPackageIconsSource } from './gen-icons'`), add:

```ts
import { buildReportPanelsSource } from './gen-report-panels'
```

After the `package-icons.ts` write (after line 644), add:

```ts

    fs.writeFileSync(
        path.join(GENERATED_DIR, 'report-panels.ts'),
        buildReportPanelsSource(features.map(f => ({ name: f.name, manifest: f.manifest })))
    )
```

Create `core/reports/hooks/usePanelTitle.ts` (Task 8 adds `usePanelTitle` and `PanelTitleProvider` to this file):

```ts
/**
 * The props core gives a package's record panel (manifest `reportPanels`, D5):
 * the path segments of the row's `open` after `<slug>/<path>/`. For `open`
 * `gizmos/history/c/1` and the panel path `history`, the segments are
 * `['c', '1']`.
 */
export type ReportPanelProps = {
    segments: string[]
}
```

Create `core/reports/lib/record-panel.ts` (Task 7 adds the rules to this file):

```ts
import type { ComponentType } from 'react'
import type { ReportPanelProps } from '../hooks/usePanelTitle'

/** One package's record panel, as the generator lists it (lib/generated/report-panels.ts). */
export type ReportPanelEntry = {
    slug: string
    path: string
    load: () => Promise<{ default: ComponentType<ReportPanelProps> }>
}
```

Append to `core/types/app-generated.d.ts`:

```ts

declare module '@tinycld/app-generated/report-panels' {
    import type { ReportPanelEntry } from '@tinycld/core/reports/lib/record-panel'

    export const reportPanels: ReportPanelEntry[]
}
```

In `core/tests/unit-setup.ts`, after the `package-icons` mock (line 66), add:

```ts
vi.mock('@tinycld/app-generated/report-panels', () => ({
    reportPanels: [],
}))
```

- [ ] **Step 7: Generate, typecheck and lint**

No e2e run may be in progress (generation rewrites `lib/generated`). Run from `/Users/nas/tmp/tinycld/books-reports/tinycld`: `pnpm run packages:generate`
Expected: it finishes, and `lib/generated/report-panels.ts` holds `export const reportPanels: ReportPanelEntry[] = []` (no installed package declares a panel yet; Task 10 adds the stub's). Check with `command grep -n "reportPanels" lib/generated/report-panels.ts`.

Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check scripts/gen-report-panels.ts scripts/generate.ts scripts/load-manifest.ts scripts/__tests__/gen-report-panels.test.ts core/lib/packages/types.ts core/types/app-generated.d.ts core/tests/unit-setup.ts core/reports/hooks/usePanelTitle.ts core/reports/lib/record-panel.ts`
Expected: no errors. `lib/generated/` is already in biome's exclude and in `.gitignore` (`/lib/generated/`), so the new file needs no entry in either.

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run scripts/__tests__/gen-report-panels.test.ts scripts/__tests__/gen-icons.test.ts scripts/__tests__/load-manifest.test.ts`
Expected: PASS.

- [ ] **Step 8: Hand the diff to the driver**

No commit. The diff: `core/lib/packages/types.ts`, `scripts/load-manifest.ts`, `scripts/gen-report-panels.ts` (new), `scripts/generate.ts`, `core/types/app-generated.d.ts`, `core/tests/unit-setup.ts`, `core/reports/hooks/usePanelTitle.ts` (new), `core/reports/lib/record-panel.ts` (new), `scripts/__tests__/gen-report-panels.test.ts` (new). `lib/generated/report-panels.ts` is generated and gitignored.
