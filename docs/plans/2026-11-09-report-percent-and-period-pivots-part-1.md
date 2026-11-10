# Report percent of a section and period pivots — Part 1 (Tasks 1–8)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The report engine and the client library gain bucket labels in words (P4), pivot columns before their total (P3), a pivot over every bucket of a date range (P2) and a percent of a layout section's total (P1).

**Architecture:** See the outline, `docs/plans/2026-11-09-report-percent-and-period-pivots.md` (Architecture, Rulings R1–R11). In short: P4 types a bucketed label (`labelType` = the bucket, `labelValue` = the raw key) and the client writes it in words; P3 changes the result's column order under `pivot.order: "spec"`; P2 builds the pivot's columns in Go from the range param's `from`..`to`; P1 compiles `percentOfSection` as a window op with a NULL placeholder and fills it in `assemble` from the section's total per currency block.

**Tech Stack:** Go 1.26 (PocketBase, SQLite), TypeScript, zod v4, vitest, Playwright.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/profit-and-loss-design.md` §4. Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-profit-and-loss/cp/cp0.md`.

Paths are relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld` (branch `books/p5-profit-and-loss`). Go tests: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run '<Regex>' -count=1`. Vitest, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`. One e2e file, from the same directory, after the port check (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts`.

## Global Constraints

- No spec version bump: every new field is optional at version 4 (`Load` decodes strictly).
- Core names no package: Go tests use the `zoo_*` fixture (`zoo_test.go`); nothing names Books.
- Refusals go through `b.bad` / `b.missing` in the binder and `invalid` / `unknownRef` in the compiler; SQL values are bound with `s.bind`.
- The Go `Label` text of a bucketed label stays the raw key; only the client writes words (R8). `typedLabelTypes` (fit.go) and `TYPED_LABELS` (column-measure.ts) do not gain the bucket types (R9).
- `percentOfSection` is a column's whole formula, and no other formula reads that column (R2).
- Comments say why, not what. No `any` in TS, no biome-ignore, no `console.*`. Biome: 4 spaces, single quotes.
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. A month key that is not in the bucket's form shows as it is, never "undefined 2026" — Task 2 test `bucketWords keeps a key it cannot read`.
2. A range far wider than `maxColumns` builds at most `maxColumns` keys and counts the rest arithmetically — Task 5 test `TestBucketSeriesCountsWithoutBuildingPastTheCap`.
3. A range param on All dates, or a pivot with no `range`, keeps today's columns — Task 6 test `TestPeriodPivotOnAllDatesKeepsTodaysColumns`.
4. Each currency block of a layout report divides by its own section total — Task 8 test `TestPercentOfSectionDividesByItsOwnCurrencysTotal`.
5. A folded section's rows divide by the section's whole total, read from the statement rows — Task 8 test `TestPercentOfSectionReadsTheTotalWhenTheSectionIsFolded`.

---

### Task 1: P4 Go — bucket label types

**Files:**
- Modify: `core/server/reports/registry.go` (FieldType constants after `Percent`; `bucketType` after the const block)
- Modify: `core/server/reports/pivot.go` (`pivotLabel`, lines 181–190)
- Modify: `core/server/reports/assemble.go` (`group`, lines 597–613)
- Modify: `core/server/reports/run.go` (`PivotColumn` doc comment, lines 130–132)
- Create: `core/server/reports/bucketlabel_test.go`

**Interfaces:**
- Consumes: `rowLabel{text, typ, value, currency}` (assemble.go), `rn.display(cp, v, bucketed)`, `Pivot.Bucket`, `Group.Bucket`.
- Produces: `reports.Month = "month"`, `reports.Quarter = "quarter"`, `reports.Year = "year"`, `reports.FiscalYear = "fiscalYear"`, `reports.Day = "day"` (FieldType); `bucketType(bucket string) FieldType` ("" for no bucket). A bucketed pivot column's `PivotColumn.LabelType` / `LabelValue` and a bucketed group row's (and its subtotal's) `Row.LabelType` / `LabelValue` are the bucket's type and the raw key. `PivotColumn.Key`, `Row.GroupKey`, `Row.Label` and `ResultColumn.Label` keep the raw key. Fiscal year keys are `int64` end years.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/bucketlabel_test.go`:

```go
package reports

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// feedingsPivotBy pivots ann's feeding cost by species and a bucket of fed:
// f1 Jan 5, f2 Feb 10, f3 Jan 20, f4 Mar 3 (2026).
const feedingsPivotBy = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"%s","cell":"cost"}}`

// feedingsGroupedBy groups ann's feedings by a bucket of fed, with subtotals.
const feedingsGroupedBy = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"m","col":"fed","bucket":"%s","subtotals":true}]}`

// pivotLabelTypes is each pivot column's label as "type=value".
func pivotLabelTypes(res *Result) []string {
	var out []string
	for _, c := range res.Columns {
		if c.Pivot != nil {
			out = append(out, fmt.Sprintf("%s=%v", c.Pivot.LabelType, c.Pivot.LabelValue))
		}
	}
	return out
}

// groupLabelTypes is each group and subtotal row's label as
// "label type=value".
func groupLabelTypes(res *Result) []string {
	var out []string
	for _, r := range res.Rows {
		if r.Kind == KindGroup || r.Kind == KindSubtotal {
			out = append(out, fmt.Sprintf("%s %s=%v", r.Label, r.LabelType, r.LabelValue))
		}
	}
	return out
}

func TestABucketedPivotColumnCarriesItsBucketType(t *testing.T) {
	z := newZoo(t)
	for bucket, want := range map[string][]string{
		"month":   {"month=2026-01", "month=2026-02", "month=2026-03"},
		"quarter": {"quarter=2026-Q1"},
		"year":    {"year=2026"},
		"day":     {"day=2026-01-05", "day=2026-01-20", "day=2026-02-10", "day=2026-03-03"},
	} {
		t.Run(bucket, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(feedingsPivotBy, bucket), Options{}, nil)
			if got := pivotLabelTypes(res); !slices.Equal(got, want) {
				t.Fatalf("pivot labels = %v, want %v", got, want)
			}
			// The key and the label text stay the raw key (R8).
			for _, c := range res.Columns {
				if c.Pivot != nil && (c.Pivot.Key != c.Pivot.LabelValue || c.Label != c.Pivot.Key) {
					t.Fatalf("column %s: key %#v, label %q, label value %#v", c.ID, c.Pivot.Key, c.Label, c.Pivot.LabelValue)
				}
			}
		})
	}
}

func TestABucketedGroupRowCarriesItsBucketType(t *testing.T) {
	z := newZoo(t)
	for bucket, want := range map[string][]string{
		"month": {
			"2026-01 month=2026-01", "Total 2026-01 month=2026-01",
			"2026-02 month=2026-02", "Total 2026-02 month=2026-02",
			"2026-03 month=2026-03", "Total 2026-03 month=2026-03",
		},
		"quarter": {"2026-Q1 quarter=2026-Q1", "Total 2026-Q1 quarter=2026-Q1"},
		"year":    {"2026 year=2026", "Total 2026 year=2026"},
		"day": {
			"2026-01-05 day=2026-01-05", "Total 2026-01-05 day=2026-01-05",
			"2026-01-20 day=2026-01-20", "Total 2026-01-20 day=2026-01-20",
			"2026-02-10 day=2026-02-10", "Total 2026-02-10 day=2026-02-10",
			"2026-03-03 day=2026-03-03", "Total 2026-03-03 day=2026-03-03",
		},
	} {
		t.Run(bucket, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, bucket), Options{ExpandAll: true}, nil)
			if got := groupLabelTypes(res); !slices.Equal(got, want) {
				t.Fatalf("group labels = %v\nwant %v", got, want)
			}
		})
	}
	res := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, "month"), Options{}, nil)
	// The key is the raw month, then the currency (groups_test.go pins the
	// currency last).
	if key := res.Rows[0].GroupKey; !reflect.DeepEqual(key, []any{"2026-01", "USD"}) {
		t.Fatalf("group key = %#v, want the raw month and USD", key)
	}
}

func TestAFiscalYearBucketIsTypedWithItsEndYear(t *testing.T) {
	z := newZoo(t)
	zooFiscalStart = 7
	t.Cleanup(func() { zooFiscalStart = 1 })
	z.feeding(t, "f9", z.recs["leo"], 400, "2026-08-01", 10)
	// Jan to Mar 2026 fall in the year to June 2026; Aug 2026 in the year to
	// June 2027.
	pivot := z.run(t, "ann", fmt.Sprintf(feedingsPivotBy, "fiscalYear"), Options{}, nil)
	if got, want := pivotLabelTypes(pivot), []string{"fiscalYear=2026", "fiscalYear=2027"}; !slices.Equal(got, want) {
		t.Fatalf("pivot labels = %v, want %v", got, want)
	}
	if k := pivot.Columns[len(pivot.Columns)-1].Pivot.Key; k != int64(2027) {
		t.Fatalf("fiscal key = %#v, want int64 2027", k)
	}
	grouped := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, "fiscalYear"), Options{ExpandAll: true}, nil)
	want := []string{"2026 fiscalYear=2026", "Total 2026 fiscalYear=2026", "2027 fiscalYear=2027", "Total 2027 fiscalYear=2027"}
	if got := groupLabelTypes(grouped); !slices.Equal(got, want) {
		t.Fatalf("group labels = %v, want %v", got, want)
	}
}

// A bucket type is a label's type only: no source may declare a field of it.
func TestABucketTypeIsNeverAFieldsType(t *testing.T) {
	for _, typ := range []FieldType{Month, Quarter, Year, FiscalYear, Day} {
		if err := validateField(&Source{}, &Field{Label: "When", Type: typ}); err == nil {
			t.Fatalf("a field of type %q was accepted", typ)
		}
	}
}
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestABucketed|TestAFiscalYearBucket|TestABucketType' -count=1`
Expected: build failure, `undefined: Month` (and the other four).

- [ ] **Step 3: Implement**

In `registry.go`, after `Percent FieldType = "percent"` inside the const block, add:

```go
	// Month, Quarter, Year, FiscalYear and Day are never a field's type: a
	// bucketed group row's or pivot column's label carries one, with the raw
	// bucket key as its value, so the client writes the key in words.
	Month      FieldType = "month"
	Quarter    FieldType = "quarter"
	Year       FieldType = "year"
	FiscalYear FieldType = "fiscalYear"
	Day        FieldType = "day"
```

After the const block, add:

```go
// bucketType is the label type of a bucket's keys, "" for no bucket.
func bucketType(bucket string) FieldType {
	switch bucket {
	case "day":
		return Day
	case "month":
		return Month
	case "quarter":
		return Quarter
	case "year":
		return Year
	case "fiscalYear":
		return FiscalYear
	}
	return ""
}
```

`validateField`'s `switch f.Type` lists the field types by name, so it already refuses the five; leave it.

In `pivot.go`, replace `pivotLabel`'s last line and its comment:

```go
// pivotLabel is the j-th pivot value as a column label: a record's typed
// label for a pivot over a link, an amount typed with its currency, a bucket
// typed with its bucket (the client writes it in words), else the value as a
// row label shows it.
func (rn *runner) pivotLabel(j int, key any) rowLabel {
	pv := rn.c.p.pivot
	if pv.path.target != nil || pv.path.field.Type == Money {
		return rn.pivotLabels[j]
	}
	label := rowLabel{text: rn.display(&colPlan{col: &Column{}, path: pv.path}, key, pv.pivot.Bucket != "")}
	if typ := bucketType(pv.pivot.Bucket); typ != "" && key != nil && key != "" {
		label.typ, label.value = typ, key
	}
	return label
}
```

In `assemble.go` `group`, replace the first `label :=` line:

```go
	raw := row[fmt.Sprintf("k%d", n-1)]
	label := rowLabel{text: rn.display(g.col, raw, g.group.Bucket != "")}
	if typ := bucketType(g.group.Bucket); typ != "" && raw != nil && raw != "" {
		label.typ, label.value = typ, raw
	}
```

The `groupShown` and label-formula overrides below it replace the whole label, as today.

In `run.go`, replace the `PivotColumn` field comment above `LabelType`:

```go
	// LabelType, LabelValue and LabelCurrency type the column label as Row's
	// do: a pivot over a link's record label, an amount, or a bucket (Month,
	// Quarter, Year, FiscalYear, Day with the raw key), which the column's
	// Label holds as plain text.
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestABucketed|TestAFiscalYearBucket|TestABucketType|TestPivot|TestLayout' -count=1`
Expected: PASS (the existing pivot and layout tests still read `Label` "2026-01").

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): type bucketed group and pivot labels by their bucket`

---

### Task 2: P4 client — bucket labels in words

**Files:**
- Modify: `core/reports/lib/types.ts` (lines 5–16)
- Modify: `core/reports/lib/dates.ts` (new export after `shortDay`)
- Modify: `core/reports/lib/format.ts` (`typedLabelText`, line 205; imports lines 1–3)
- Modify: `core/reports/lib/builder/filter.ts` (`OPS_BY_TYPE`, line 193)
- Modify: `core/reports/lib/__tests__/dates.test.ts`, `core/reports/lib/__tests__/format.test.ts`
- Modify: `tests/e2e/reports-group-pivot.spec.ts` (lines 66–67)

**Interfaces:**
- Consumes: Task 1's label types on `ResultRow.labelType` and `ResultColumn.pivot.labelType`.
- Produces: `BUCKET_LABEL_TYPES = ['month', 'quarter', 'year', 'fiscalYear', 'day'] as const`, `type BucketLabelType`, `isBucketLabelType(type: FieldType): type is BucketLabelType` (types.ts); `FieldType` gains the five; `bucketWords(type: BucketLabelType, key: string | number): string` (dates.ts); `typedLabelText` returns `bucketWords(...)` for a bucket type, so `rowLabelText`, `columnName` and `headingText` write "Jan 2026", "Q1 2026", "2026", "FY 2026", "Jan 5, 2026".

Core e2e that read a bucketed label (checked with `grep -rn "2026-0\|-Q[1-4]\|bucket" tests/e2e`): `reports-group-pivot.spec.ts:66–67` reads the month headings and changes here. `reports-view.spec.ts:497` builds a month pivot but reads only the `pivotCap` notice text, so it does not change. No other core e2e reads a bucket label.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/dates.test.ts`, add `bucketWords` to the import from `'../dates'` and append:

```ts
describe('bucketWords', () => {
    it('writes each bucket key in words', () => {
        expect(bucketWords('month', '2026-01')).toBe('Jan 2026')
        expect(bucketWords('month', '2026-12')).toBe('Dec 2026')
        expect(bucketWords('quarter', '2026-Q1')).toBe('Q1 2026')
        expect(bucketWords('year', '2026')).toBe('2026')
        // The engine keys a fiscal year by the year it ends in, as a number.
        expect(bucketWords('fiscalYear', 2026)).toBe('FY 2026')
        expect(bucketWords('day', '2026-01-05')).toBe('Jan 5, 2026')
    })

    it('bucketWords keeps a key it cannot read', () => {
        expect(bucketWords('month', '2026-13')).toBe('2026-13')
        expect(bucketWords('month', 'soon')).toBe('soon')
        expect(bucketWords('quarter', '2026-Q5')).toBe('2026-Q5')
        expect(bucketWords('fiscalYear', 'FY')).toBe('FY')
        expect(bucketWords('day', '2026-02-30')).toBe('2026-02-30')
    })
})
```

In `core/reports/lib/__tests__/format.test.ts`, inside `describe('typed labels', …)`, append:

```ts
    it('writes a bucketed group label and its subtotal in words', () => {
        const month = labelled({ label: '2026-01', labelType: 'month', labelValue: '2026-01' })
        expect(rowLabelText(month, PLAIN)).toBe('Jan 2026')
        expect(rowLabelText({ ...month, kind: 'subtotal', label: 'Total 2026-01' }, PLAIN)).toBe(
            'Total Jan 2026'
        )
        const fiscal = labelled({ label: '2027', labelType: 'fiscalYear', labelValue: 2027 })
        expect(rowLabelText(fiscal, PLAIN)).toBe('FY 2027')
    })

    it('names a bucketed pivot column in words', () => {
        const quarter: ResultColumn = {
            id: 'cost.p0',
            label: '2026-Q1',
            type: 'money',
            pivot: { cell: 'cost', key: '2026-Q1', labelType: 'quarter', labelValue: '2026-Q1' },
        }
        expect(columnName(quarter, PLAIN)).toBe('Q1 2026')
        expect(headingText(quarter, PLAIN)).toBe('Q1 2026')
    })
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/dates.test.ts core/reports/lib/__tests__/format.test.ts`
Expected: FAIL — `bucketWords` is not exported; the format cases read `2026-01` / `2026-Q1`.

- [ ] **Step 3: Implement**

In `types.ts`, replace lines 5–16:

```ts
/**
 * The types a bucketed group row's or pivot column's label carries (registry.go
 * Month … Day), with the raw bucket key as its value. Never a field's type.
 */
export const BUCKET_LABEL_TYPES = ['month', 'quarter', 'year', 'fiscalYear', 'day'] as const
export type BucketLabelType = (typeof BUCKET_LABEL_TYPES)[number]

export const FIELD_TYPES = [
    'text',
    'number',
    'money',
    'date',
    'datetime',
    'bool',
    'select',
    'relation',
    'percent',
    ...BUCKET_LABEL_TYPES,
] as const
export type FieldType = (typeof FIELD_TYPES)[number]

const BUCKET_LABEL_SET: ReadonlySet<FieldType> = new Set(BUCKET_LABEL_TYPES)

export function isBucketLabelType(type: FieldType): type is BucketLabelType {
    return BUCKET_LABEL_SET.has(type)
}
```

In `dates.ts`, add the import `import type { BucketLabelType } from './types'` at the top and, after `shortDay`:

```ts
const MONTH_KEY = /^(\d{4})-(\d{2})$/
const QUARTER_KEY = /^(\d{4})-Q([1-4])$/
const YEAR_KEY = /^\d{4}$/

/**
 * A bucket key as the view shows it: month "Jan 2026", quarter "Q1 2026", year
 * "2026", fiscal year "FY 2026" (the year it ends in), day "Jan 5, 2026". A key
 * not in its bucket's form shows as it is.
 */
export function bucketWords(type: BucketLabelType, key: string | number): string {
    const text = String(key)
    switch (type) {
        case 'month': {
            const match = MONTH_KEY.exec(text)
            const name = match ? SHORT_MONTH_NAMES[Number(match[2]) - 1] : undefined
            return match && name ? `${name} ${match[1]}` : text
        }
        case 'quarter': {
            const match = QUARTER_KEY.exec(text)
            return match ? `Q${match[2]} ${match[1]}` : text
        }
        case 'fiscalYear':
            return YEAR_KEY.test(text) ? `FY ${text}` : text
        case 'day':
            return isIsoDay(text) ? shortDay(text) : text
        case 'year':
            return text
    }
}
```

In `format.ts`, change the imports:

```ts
import { bucketWords, isIsoDay, shortDay } from './dates'
import { formatMoney, groupDigits, MAX_STORED_AMOUNT, roundHalfAwayFromZero } from './money'
import {
    type CellValue,
    type FieldType,
    isBucketLabelType,
    type ResultColumn,
    type ResultRow,
    type RowKind,
} from './types'
```

and make `typedLabelText`'s body start:

```ts
    if (isBucketLabelType(type) && (typeof value === 'string' || typeof value === 'number')) {
        return bucketWords(type, value)
    }
```

In `types.ts`, change the doc above `ResultColumn.pivot.labelType` (line 58) to: `/** The column label typed, as a row's \`labelType\`: a pivot over a link's record label, an amount, or a bucket (month … day) with the raw key. */`.

In `builder/filter.ts` `OPS_BY_TYPE`, add (a label type is never a field a condition reads):

```ts
    // Label types only: no field has them, so no condition reads one.
    month: [],
    quarter: [],
    year: [],
    fiscalYear: [],
    day: [],
```

In `tests/e2e/reports-group-pivot.spec.ts`, lines 66–67:

```ts
    await expect(p.getByTestId('report-heading-amount.p0')).toHaveText('Jan 2026')
    await expect(p.getByTestId('report-heading-amount.p1')).toHaveText('Feb 2026')
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/dates.test.ts core/reports/lib/__tests__/format.test.ts`
Expected: PASS.
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && pnpm exec tinycld-pkg typecheck`
Expected: no errors (a `Record<FieldType, …>` that misses a type would show here).
Run (port check first): `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-group-pivot.spec.ts`
Expected: PASS. If this run takes more than about 4 minutes, hand back `NEEDS-RUN: <the command>` instead.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): write bucket labels in words`

---

### Task 3: P3 Go — pivot columns before their total

**Files:**
- Modify: `core/server/reports/spec.go` (`Pivot`, line 246; new const)
- Modify: `core/server/reports/bind.go` (`groupsAndPivot`, after the `pp.cell` checks at lines 859–865)
- Modify: `core/server/reports/assemble.go` (`columns`, lines 14–53)
- Modify: `core/server/reports/fit.go` (comment at lines 146–147)
- Create: `core/server/reports/pivotorder_test.go`

**Interfaces:**
- Consumes: `pivotPlan{pivot, cell}`, `rn.pivotValues`, `rn.pivotLabel` (Task 1).
- Produces: `Pivot.Order string` (JSON `order`), `const PivotOrderSpec = "spec"`; `(rn *runner) pivotColumns() []ResultColumn`; refusals at `pivot.order`: 400 `order is "spec"`; 400 when the cell column is the spec's first column. `c.p.cols` already keeps the spec's order (`b.columns` appends in spec order), so `fit.go` reads the label as `Columns[0]` in both orders.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/pivotorder_test.go`:

```go
package reports

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

const speciesCostByMonth = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"%s}}`

func columnIDs(res *Result) []string {
	ids := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		ids[i] = c.ID
	}
	return ids
}

func TestPivotOrderSpecPutsThePivotColumnsBeforeTheirCellColumn(t *testing.T) {
	z := newZoo(t)
	after := z.run(t, "ann", fmt.Sprintf(speciesCostByMonth, ""), Options{}, nil)
	if got, want := columnIDs(after), []string{"species", "cost", "grams", "cost.p0", "cost.p1", "cost.p2"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	before := z.run(t, "ann", fmt.Sprintf(speciesCostByMonth, `,"order":"spec"`), Options{}, nil)
	if got, want := columnIDs(before), []string{"species", "cost.p0", "cost.p1", "cost.p2", "cost", "grams"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	// Only the order changes.
	for _, id := range []string{"cost", "cost.p0", "cost.p1", "cost.p2", "grams"} {
		if a, b := rowsOf(after, id), rowsOf(before, id); !reflect.DeepEqual(a, b) {
			t.Fatalf("%s = %v with order spec, %v without", id, b, a)
		}
	}
	requireRows(t, before, "cost.p1", int64(2500), int64(0), int64(2500))
}

func TestPivotOrderRefusals(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", fmt.Sprintf(speciesCostByMonth, `,"order":"after"`), Options{}, nil)
	requireSpecError(t, err, 400, "pivot.order", `order is "spec"`)

	first := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","order":"spec"}}`
	_, err = z.try(t, "ann", first, Options{}, nil)
	requireSpecError(t, err, 400, "pivot.order", "first column")
}

// fittedByMonth fits the label column 20ch wide on a month pivot; first is
// the label column, id "label".
func fittedByMonth(first, order string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[` + first + `,
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"label"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"` + order + `},
		"display":{"columns":{"label":{"width":"20ch"}}}}`
}

// The label column is the spec's first column in both orders, so a fit on it
// is judged the same way.
func TestPivotOrderKeepsTheLabelColumnsFit(t *testing.T) {
	app := withZooRegistry(t)
	typed := `{"id":"label","path":[{"link":"animal"}],"labelField":"born"}`
	text := `{"id":"label","path":[{"link":"animal"},{"field":"species"}]}`
	for _, order := range []string{"", `,"order":"spec"`} {
		requireSpecError(t, Validate(app, loadSpec(t, fittedByMonth(typed, order))), 400, "display.columns.label.width", neverCutMessage)
		if err := Validate(app, loadSpec(t, fittedByMonth(text, order))); err != nil {
			t.Fatalf("order %q: a text label's width was refused: %v", order, err)
		}
	}
}
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPivotOrder' -count=1`
Expected: FAIL — `Load` refuses the unknown field `order` (strict decode), so every case errors at load.

- [ ] **Step 3: Implement**

In `spec.go`, add to `Pivot` after `LabelField`:

```go
	// Order "spec" (PivotOrderSpec) puts the pivot columns just before their
	// cell column, so its total reads after them; absent, they come after
	// every column.
	Order string `json:"order,omitempty"`
```

and after the struct:

```go
// PivotOrderSpec is the one Pivot.Order value.
const PivotOrderSpec = "spec"
```

In `bind.go` `groupsAndPivot`, after the `pp.cell.col.Rollup` check:

```go
	switch {
	case pv.Order == "":
	case pv.Order != PivotOrderSpec:
		b.bad("pivot.order", `order is "spec"`)
	case len(spec.Columns) > 0 && spec.Columns[0].ID == pv.Cell:
		b.bad("pivot.order", "the pivot cell is the first column, the label: no pivot column may come before it")
	}
```

In `assemble.go`, replace `columns` with:

```go
func (rn *runner) columns() []ResultColumn {
	c := rn.c
	pivots := rn.pivotColumns()
	// Under order "spec" the pivot columns come just before their cell
	// column, so its total reads after them; else after every column.
	before := c.p.pivot != nil && c.p.pivot.pivot.Order == PivotOrderSpec
	out := make([]ResultColumn, 0, len(c.p.cols)+len(pivots))
	for _, cp := range c.p.cols {
		if before && cp == c.p.pivot.cell {
			out = append(out, pivots...)
		}
		if cp.col.Hidden {
			continue
		}
		rc := ResultColumn{ID: cp.col.ID, Label: labelOf(cp), Aggregate: cp.col.Aggregate, Format: cp.col.Format}
		rc.Type, rc.TotalType = c.resultType(cp)
		if cp.path != nil && cp.path.field.Type == Select {
			rc.Options = cp.path.field.Options
		} else if cp.path == nil && rc.Type == Select {
			rc.Options = c.formulaOptions[cp.col.ID]
		}
		if typ, ok := c.labelTypes[cp.col.ID]; ok {
			rc.LabelCell, rc.LabelType = cp.col.ID+"@label", typ
			if typ == Money {
				rc.LabelCurrencyCell = cp.col.ID + "@labelCurrency"
			}
		}
		rc.Sortable = c.p.pivot == nil && c.sortable[cp.col.ID]
		rc.Heading = rn.headings[cp.col.ID]
		rn.currencyOfColumn(cp, &rc)
		out = append(out, rc)
	}
	if !before {
		out = append(out, pivots...)
	}
	return out
}

// pivotColumns is one column per pivot value, named by its typed label.
func (rn *runner) pivotColumns() []ResultColumn {
	c := rn.c
	pv := c.p.pivot
	if pv == nil {
		return nil
	}
	cell := pv.cell
	out := make([]ResultColumn, 0, len(rn.pivotValues))
	for j, v := range rn.pivotValues {
		label := rn.pivotLabel(j, v.key)
		rc := ResultColumn{
			ID: fmt.Sprintf("%s.p%d", cell.col.ID, j), Label: label.text,
			Type: c.levelTypes[cell.col.ID], Aggregate: cell.col.Aggregate, Format: cell.col.Format,
			Pivot: &PivotColumn{Cell: cell.col.ID, Key: v.key, LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency},
		}
		rn.currencyOfColumn(cell, &rc)
		out = append(out, rc)
	}
	return out
}
```

In `fit.go`, replace the two comment lines above `label := c.p.spec.Columns[0].ID`:

```go
	// The label column is the spec's first column. c.p.cols keeps the spec's
	// order, and pivot columns (after every column, or under order "spec"
	// before their cell column, which is never first) never come first, so
	// the label is the same in both orders.
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPivot|TestAFigureIsNeverCut|TestLayout' -count=1`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): pivot.order "spec" puts pivot columns before their total`

---

### Task 4: P2/P3 client library — spec fields and arranged pivot columns

**Files:**
- Modify: `core/reports/lib/spec.ts` (pivot schema, lines 268–276; `SpecNode.subtotal` doc, line 29)
- Modify: `core/reports/lib/table-lines.ts` (`expandPivots`, lines 275–299)
- Modify: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/__tests__/table-lines.test.ts`, `core/reports/lib/__tests__/column-measure.test.ts`

`core/reports/lib/column-measure.ts` does not change (R10): it measures headings through `headingText` (words after Task 2) and takes `columns[0]` as the label, which P3 never moves. The tests below pin both.

**Interfaces:**
- Consumes: Task 2's `typedLabelText` words; Task 3's result order.
- Produces: pivot schema fields `range?: string` and `order?: 'spec'`; `expandPivots` expands an arranged cell that names a pivot's cell column to its pivot columns, then the cell column itself when the result puts the cell column right after them (order "spec"); else to the pivot columns only, as today.

- [ ] **Step 1: Write the failing tests**

In `spec.test.ts`, inside `describe('parseSpec', …)`, append:

```ts
    it('reads a pivot over a range with its total after the pivot columns', () => {
        const spec = {
            ...FEEDINGS,
            pivot: {
                path: [{ field: 'fed' }],
                bucket: 'month',
                cell: 'cost',
                range: 'period',
                order: 'spec',
            },
        }
        expect(parseSpec(spec)).toEqual(spec)
        expect(parseSpec({ ...spec, pivot: { ...spec.pivot, order: 'after' } })).toBeNull()
    })

    it('reads a percent of a section with the section it names', () => {
        const share = {
            id: 'share',
            formula: { op: 'percentOfSection', args: [{ col: 'cost' }], subtotal: 'income', scale: 1 },
        }
        const spec = { ...FEEDINGS, columns: [...FEEDINGS.columns, share] }
        expect(parseSpec(spec)).toEqual(spec)
    })
```

In `table-lines.test.ts`, after the `it('gives each pivot column the span and lines of its entry', …)` block, add:

```ts
    it('lays the total after its pivot columns when the result puts it there', () => {
        const total: ResultColumn = { id: 'amount', label: 'Total', type: 'money' }
        const [account, number, ...pivots] = PIVOTS
        const after = [account, number, ...pivots, total].filter(
            (c): c is ResultColumn => c !== undefined
        )
        const lines = (columns: ResultColumn[]) =>
            shapeOf(
                tableLines({
                    columns,
                    width: 500,
                    measures: uniform(columns, 110),
                    layouts: [
                        {
                            width: 'mobile',
                            tracks: 4,
                            cells: [
                                { col: 'account', line: 1, span: 4 },
                                { col: 'amount', line: 2 },
                            ],
                        },
                    ],
                })
            )
        expect(lines(after)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 4,
                        parts: [
                            { block: pivotCell('amount_q1'), tracks: 1 },
                            { block: pivotCell('amount_q2'), tracks: 1 },
                            { block: pivotCell('amount_q3'), tracks: 1 },
                            { block: pivotCell('amount'), tracks: 1 },
                        ],
                    },
                ],
            },
        })
        // With the total before its pivot columns (no order), the entry is the
        // pivot columns only, as before.
        const first = [account, number, total, ...pivots].filter(
            (c): c is ResultColumn => c !== undefined
        )
        expect(lines(first)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 4,
                        parts: [
                            { block: pivotCell('amount_q1'), tracks: 1 },
                            { block: pivotCell('amount_q2'), tracks: 1 },
                            { block: pivotCell('amount_q3'), tracks: 1 },
                            { block: { kind: 'gap', lines: 1 }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
```

In `column-measure.test.ts`, append:

```ts
describe('pivot columns', () => {
    const SPECIES: ResultColumn = { id: 'species', label: 'Species', type: 'text' }
    const COST: ResultColumn = { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' }
    const month = (j: number, key: string): ResultColumn => ({
        id: `cost.p${j}`,
        label: key,
        type: 'text',
        pivot: { cell: 'cost', key, labelType: 'month', labelValue: key },
    })

    it('measures a month heading in words', () => {
        const m = measureColumns({
            columns: [SPECIES, month(0, '2026-01')],
            rows: [],
            format: DEFAULT_DISPLAY,
            levels: FLAT,
        })
        expect(m['cost.p0']?.natural).toBe(textWidth('Jan 2026', HEADING) + CELL_PADDING)
    })

    it('keeps the first column as the label when the total follows its pivot columns', () => {
        const measure = (columns: ResultColumn[]) =>
            measureColumns({ columns, rows: [GROUP], format: DEFAULT_DISPLAY, levels: GROUPED })
        const totalAfter = measure([SPECIES, month(0, '2026-01'), month(1, '2026-02'), COST])
        const totalFirst = measure([SPECIES, COST, month(0, '2026-01'), month(1, '2026-02')])
        expect(totalAfter.species).toEqual(totalFirst.species)
        expect(totalAfter.cost).toEqual(totalFirst.cost)
    })
})
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: FAIL — `parseSpec` returns null for `range` / `order` (strict object); the total-after shape lacks `amount`. The two column-measure cases pass already (they pin R10).

- [ ] **Step 3: Implement**

In `spec.ts`, the pivot schema:

```ts
    pivot: z
        .strictObject({
            path: z.array(specHopSchema),
            bucket: z.enum(BUCKETS).optional(),
            cell: z.string(),
            maxColumns: z.number().int().optional(),
            labelField: z.string().optional(),
            // A dateRange param: every bucket its from..to touches is a column.
            range: z.string().min(1).optional(),
            // 'spec': the pivot columns come just before their cell column.
            order: z.literal('spec').optional(),
        })
        .nullish(),
```

and above `subtotal?: string` in `SpecNode`:

```ts
    /** A computed row's section or earlier row; on percentOfSection, the section it divides by. */
```

In `table-lines.ts`, replace `expandPivots`:

```ts
// A pivot cell column's entry stands for all its pivot columns, each as wide
// and as tall as the entry, side by side from where the entry is placed and
// wrapping onto the lines below it, from the first track. When the result puts
// the cell column right after its pivot columns (order "spec"), the entry ends
// with the cell column too, so the total reads after them as on one line.
function expandPivots(
    layout: DisplayLayout,
    placed: readonly Placed[],
    columns: readonly ResultColumn[]
): DisplayCell[] {
    return layout.cells.flatMap((cell, index) => {
        const pivots = columns.filter(c => c.pivot?.cell === cell.col)
        const at = placed[index]
        if (pivots.length === 0 || !at) return [cell]
        const ids = pivots.map(p => p.id)
        if (totalFollowsPivots(columns, cell.col)) ids.push(cell.col)
        let track = at.start - 1
        let line = at.line
        return ids.map(col => {
            if (track + at.span > layout.tracks) {
                track = 0
                line += at.lines
            }
            const start = track + 1
            track += at.span
            return { col, line, start, span: at.span, lines: at.lines, justify: cell.justify }
        })
    })
}

function totalFollowsPivots(columns: readonly ResultColumn[], cell: string): boolean {
    const at = columns.findIndex(c => c.id === cell && !c.pivot)
    return at > 0 && columns[at - 1]?.pivot?.cell === cell
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): read pivot range and order; lay the total after its pivot columns`

---

### Task 5: P2 Go — range binding and the bucket series

**Files:**
- Modify: `core/server/reports/spec.go` (`Pivot`: `Range`)
- Modify: `core/server/reports/bind.go` (`groupsAndPivot`: call `b.pivotRange`; new method after `groupsAndPivot`)
- Modify: `core/server/reports/pivot.go` (new `bucketSeries`, `bucketIndex`, `bucketKey`)
- Create: `core/server/reports/periodpivot_test.go`

**Interfaces:**
- Consumes: `ParamDateRange` (params.go), `buckets`, `pivotPlan.path`.
- Produces: `Pivot.Range string` (JSON `range`); `(b *binder) pivotRange(p *plan, pp *pivotPlan)`; refusals at `pivot.range`: 422 `unknown_param` "the report has no param"; 400 "a date range param"; 400 "only a pivot over a date"; 400 "a bucket"; `bucketSeries(bucket, from, to string, fiscalStart, max int) (keys []any, total int)` — keys in `bucketSQL`'s forms (text, fiscal year `int64`), at most `max` of them, `total` the full count (0 for days it cannot read).

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/periodpivot_test.go`:

```go
package reports

import (
	"reflect"
	"testing"
)

func TestBucketSeries(t *testing.T) {
	for name, c := range map[string]struct {
		bucket, from, to string
		start, max       int
		keys             []any
		total            int
	}{
		"months":          {"month", "2026-01-15", "2026-03-10", 1, 50, []any{"2026-01", "2026-02", "2026-03"}, 3},
		"across a year":   {"month", "2025-11-30", "2026-02-01", 1, 50, []any{"2025-11", "2025-12", "2026-01", "2026-02"}, 4},
		"quarters":        {"quarter", "2026-02-01", "2026-09-30", 1, 50, []any{"2026-Q1", "2026-Q2", "2026-Q3"}, 3},
		"years":           {"year", "2025-06-01", "2026-01-01", 1, 50, []any{"2025", "2026"}, 2},
		"leap days":       {"day", "2028-02-27", "2028-03-01", 1, 50, []any{"2028-02-27", "2028-02-28", "2028-02-29", "2028-03-01"}, 4},
		"one day":         {"day", "2026-01-05", "2026-01-05", 1, 50, []any{"2026-01-05"}, 1},
		"fiscal years":    {"fiscalYear", "2026-01-01", "2026-12-31", 7, 50, []any{int64(2026), int64(2027)}, 2},
		"calendar fiscal": {"fiscalYear", "2026-01-01", "2026-12-31", 1, 50, []any{int64(2026)}, 1},
		"capped":          {"month", "2026-01-01", "2026-12-31", 1, 2, []any{"2026-01", "2026-02"}, 12},
	} {
		t.Run(name, func(t *testing.T) {
			keys, total := bucketSeries(c.bucket, c.from, c.to, c.start, c.max)
			if !reflect.DeepEqual(keys, c.keys) || total != c.total {
				t.Fatalf("got %#v (%d), want %#v (%d)", keys, total, c.keys, c.total)
			}
		})
	}
}

// Two centuries of days: three keys built, every day counted.
func TestBucketSeriesCountsWithoutBuildingPastTheCap(t *testing.T) {
	keys, total := bucketSeries("day", "1900-01-01", "2100-12-31", 1, 3)
	if want := []any{"1900-01-01", "1900-01-02", "1900-01-03"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v", keys)
	}
	// 201 years of 365 days and 49 leap days (1900 and 2100 are not leap years).
	if total != 73414 {
		t.Fatalf("total = %d, want 73414", total)
	}
}

// rangePivot is a feedings pivot with the given params and pivot.
func rangePivot(params, pivot string) string {
	return `{"version":4,"source":"zoo.feedings","params":[` + params + `],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":` + pivot + `}`
}

const periodParam = `{"id":"period","type":"dateRange"}`

func TestPivotRangeRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct {
		params, pivot, want string
		status              int
	}{
		"unknown param":   {periodParam, `{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","range":"when"}`, "no param", 422},
		"not a range":     {`{"id":"day","type":"date"}`, `{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","range":"day"}`, "a date range param", 400},
		"not over a date": {periodParam, `{"path":[{"field":"note"}],"cell":"cost","range":"period"}`, "only a pivot over a date", 400},
		"no bucket":       {periodParam, `{"path":[{"field":"fed"}],"cell":"cost","range":"period"}`, "a bucket", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", rangePivot(c.params, c.pivot), Options{}, nil)
			requireSpecError(t, err, c.status, "pivot.range", c.want)
		})
	}
}
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestBucketSeries|TestPivotRange' -count=1`
Expected: build failure, `undefined: bucketSeries`.

- [ ] **Step 3: Implement**

In `spec.go` `Pivot`, before `Order`:

```go
	// Range names a dateRange param: when it resolves to two days, the
	// pivot's columns are every bucket from..to touches, rows or not.
	Range string `json:"range,omitempty"`
```

In `bind.go` `groupsAndPivot`, after the `pv.Bucket` checks (line 858):

```go
	if pv.Range != "" {
		b.pivotRange(p, pp)
	}
```

and after `groupsAndPivot`:

```go
// pivotRange refuses a range that names no date range param, or a pivot
// whose columns cannot be the range's buckets: one not over a date, or with
// no bucket.
func (b *binder) pivotRange(p *plan, pp *pivotPlan) {
	pv := pp.pivot
	i := slices.IndexFunc(p.spec.Params, func(d ParamDef) bool { return d.ID == pv.Range })
	switch {
	case i < 0:
		b.missing("pivot.range", "unknown_param", fmt.Sprintf("the report has no param %q", pv.Range))
	case p.spec.Params[i].Type != ParamDateRange:
		b.bad("pivot.range", "range names a date range param")
	case pp.path != nil && pp.path.field.Type != Date && pp.path.field.Type != DateTime:
		b.bad("pivot.range", "only a pivot over a date has a range")
	case pv.Bucket == "":
		b.bad("pivot.range", "a range pivot needs a bucket: its columns are the range's days, months, quarters or years")
	}
}
```

In `pivot.go`, add `"time"` to the imports and append:

```go
// bucketSeries is every bucket key the days from..to touch, in order, in the
// forms bucketSQL gives (text; a fiscal year is its end year as int64): at
// most max keys, and how many there are. It counts by arithmetic, so a range
// of many buckets builds only max keys.
func bucketSeries(bucket, from, to string, fiscalStart, max int) ([]any, int) {
	first, okFirst := bucketIndex(bucket, from, fiscalStart)
	last, okLast := bucketIndex(bucket, to, fiscalStart)
	if !okFirst || !okLast || last < first {
		return nil, 0
	}
	total := last - first + 1
	keys := make([]any, 0, min(total, max))
	for i := range min(total, max) {
		keys = append(keys, bucketKey(bucket, first+i))
	}
	return keys, total
}

// bucketIndex numbers a day's bucket so that buckets next to each other are
// one apart: days since 1970, months and quarters since year 0, the year, or
// the fiscal year's end year (tc_fiscal_year).
func bucketIndex(bucket, day string, fiscalStart int) (int, bool) {
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return 0, false
	}
	year, month := d.Year(), int(d.Month())
	switch bucket {
	case "day":
		return int(d.Unix() / 86400), true
	case "month":
		return year*12 + month - 1, true
	case "quarter":
		return year*4 + (month-1)/3, true
	case "year":
		return year, true
	}
	if fiscalStart > 1 && month >= fiscalStart {
		year++
	}
	return year, true
}

// bucketKey is the key of the bucket numbered i (bucketIndex).
func bucketKey(bucket string, i int) any {
	switch bucket {
	case "day":
		return time.Unix(int64(i)*86400, 0).UTC().Format(time.DateOnly)
	case "month":
		return fmt.Sprintf("%04d-%02d", i/12, i%12+1)
	case "quarter":
		return fmt.Sprintf("%04d-Q%d", i/4, i%4+1)
	case "year":
		return fmt.Sprintf("%04d", i)
	}
	return int64(i)
}
```

`bind.go` already imports `slices`.

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestBucketSeries|TestPivotRange|TestPivot' -count=1`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): bind pivot.range and build a range's bucket series`

---

### Task 6: P2 Go — the pivot pass over the range

**Files:**
- Modify: `core/server/reports/pivot.go` (`pivotPass`, lines 44–103; new `rangeSeries`, `pivotCapNotice`)
- Modify: `core/server/reports/periodpivot_test.go`

**Interfaces:**
- Consumes: Task 5's `bucketSeries`; `rn.c.params[id]` (`paramValue.from`, `.to`); `rn.c.fiscal`; Task 1's typed `pivotLabel`.
- Produces: `(rn *runner) rangeSeries() (keys []any, total int, ok bool)`; `pivotCapNotice(max, left int) Notice`; `pivotPass` returns the range's buckets when the range param resolves to two days, with the `pivotCap` notice `LeftOut = total − max` past `maxColumns`. On All dates, or with no `range`, it runs today's query. Summed cells over a bucket with no row are 0 already (`aggregateSQL` Sum is `COALESCE(sum(…), 0)`); `pivotCells` does not change.

- [ ] **Step 1: Write the failing tests**

Append to `core/server/reports/periodpivot_test.go` (add `"fmt"` and `"slices"` to its imports):

```go
// zebrasBy is ann's zebra feedings in the period (f3 700, grams 300 on Jan
// 20; f4 300, grams 100 on Mar 3) by species, pivoted on fed.
func zebrasBy(bucket, cell, more string) string {
	return `{"version":4,"source":"zoo.feedings","params":[` + periodParam + `],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"max"}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"value":"zebra"}]},
			{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]}]},
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"` + bucket + `","cell":"` + cell + `"` + more + `}}`
}

func period(from, to string) map[string]string {
	return map[string]string{"period": fmt.Sprintf(`{"from":%q,"to":%q}`, from, to)}
}

const byPeriod = `,"range":"period"`

func requirePivotIDs(t *testing.T, res *Result, want ...string) {
	t.Helper()
	if got := pivotIDs(res); !slices.Equal(got, want) {
		t.Fatalf("pivot columns = %v\nwant %v", got, want)
	}
}

func TestPeriodPivotShowsEveryMonthOfTheRange(t *testing.T) {
	z := newZoo(t)
	q1 := period("2026-01-01", "2026-03-31")
	res := z.run(t, "ann", zebrasBy("month", "cost", byPeriod), Options{}, q1)
	requirePivotIDs(t, res, "cost.p0=2026-01", "cost.p1=2026-02", "cost.p2=2026-03")
	// Rows: the zebra group, then the total. February has no feeding: a sum
	// reads 0 there.
	requireRows(t, res, "cost.p0", int64(700), int64(700))
	requireRows(t, res, "cost.p1", int64(0), int64(0))
	requireRows(t, res, "cost.p2", int64(300), int64(300))
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Pivot.LabelType != Month {
			t.Fatalf("%s label type = %q", c.ID, c.Pivot.LabelType)
		}
	}
	// A max over no rows stays blank, as today.
	most := z.run(t, "ann", zebrasBy("month", "grams", byPeriod), Options{}, q1)
	requireRows(t, most, "grams.p1", nil, nil)
	requireRows(t, most, "grams.p0", int64(300), int64(300))
	// Without a range, only the months with rows are columns.
	plain := z.run(t, "ann", zebrasBy("month", "cost", ""), Options{}, q1)
	requirePivotIDs(t, plain, "cost.p0=2026-01", "cost.p1=2026-03")
}

func TestPeriodPivotCountsEveryMonthTheRangeTouches(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", zebrasBy("month", "cost", byPeriod), Options{}, period("2026-01-15", "2026-03-10"))
	requirePivotIDs(t, res, "cost.p0=2026-01", "cost.p1=2026-02", "cost.p2=2026-03")
}

func TestPeriodPivotByQuarter(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", zebrasBy("quarter", "cost", byPeriod), Options{}, period("2026-01-01", "2026-09-30"))
	requirePivotIDs(t, res, "cost.p0=2026-Q1", "cost.p1=2026-Q2", "cost.p2=2026-Q3")
	requireRows(t, res, "cost.p0", int64(1000), int64(1000))
	requireRows(t, res, "cost.p1", int64(0), int64(0))
}

func TestPeriodPivotWiderThanItsColumnsSaysHowManyAreLeftOut(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", zebrasBy("day", "cost", byPeriod+`,"maxColumns":10`), Options{}, period("2026-01-01", "2026-01-31"))
	ids := pivotIDs(res)
	if len(ids) != 10 || ids[0] != "cost.p0=2026-01-01" || ids[9] != "cost.p9=2026-01-10" {
		t.Fatalf("pivot columns = %v", ids)
	}
	i := slices.IndexFunc(res.Notices, func(n Notice) bool { return n.Code == "pivotCap" })
	if i < 0 || res.Notices[i].LeftOut != 21 || res.Notices[i].Message != "The pivot shows its first 10 columns; 21 more are left out." {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestPeriodPivotOnAllDatesKeepsTodaysColumns(t *testing.T) {
	z := newZoo(t)
	all := map[string]string{"period": `{"preset":"allDates"}`}
	res := z.run(t, "ann", zebrasBy("month", "cost", byPeriod), Options{}, all)
	requirePivotIDs(t, res, "cost.p0=2026-01", "cost.p1=2026-03")
	// period has no default: with no value it is unset.
	unset := z.run(t, "ann", zebrasBy("month", "cost", byPeriod), Options{}, nil)
	requirePivotIDs(t, unset, "cost.p0=2026-01", "cost.p1=2026-03")
}
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPeriodPivot' -count=1`
Expected: FAIL — the range runs give only `2026-01` and `2026-03` (or `2026-01-20` for days) and no `pivotCap` notice; `TestPeriodPivotOnAllDatesKeepsTodaysColumns` passes already.

- [ ] **Step 3: Implement**

In `pivot.go` `pivotPass`, right after `if pv == nil { return nil, nil }`:

```go
	if keys, total, ok := rn.rangeSeries(); ok {
		values := make([]pivotValue, len(keys))
		for j, key := range keys {
			values[j] = pivotValue{key: key}
		}
		if total > pv.max {
			rn.noticeOnce(pivotCapNotice(pv.max, total-pv.max))
		}
		rn.pivotValues = values
		return values, nil
	}
```

Replace the existing notice line with `rn.noticeOnce(pivotCapNotice(pv.max, left))`, and add:

```go
// rangeSeries is the pivot's range's buckets, at most maxColumns of them, and
// how many there are; ok is false for a pivot with no range, or a range on
// All dates, whose columns are the buckets that have rows.
func (rn *runner) rangeSeries() (keys []any, total int, ok bool) {
	pv := rn.c.p.pivot
	if pv.pivot.Range == "" {
		return nil, 0, false
	}
	v := rn.c.params[pv.pivot.Range]
	if v.from == "" || v.to == "" {
		return nil, 0, false
	}
	keys, total = bucketSeries(pv.pivot.Bucket, v.from, v.to, rn.c.fiscal, pv.max)
	return keys, total, true
}

func pivotCapNotice(max, left int) Notice {
	return Notice{Code: "pivotCap", Message: fmt.Sprintf("The pivot shows its first %d columns; %d more are left out.", max, left), LeftOut: left}
}
```

A pivot over a date is never a link or an amount, so `pivotLabel` (Task 1) types each range column from its key; `rn.pivotLabels` is not read for it.

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPeriodPivot|TestPivot|TestBucketSeries' -count=1`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): a range pivot shows every bucket of its period`

---

### Task 7: P1 Go — bind and compile `percentOfSection`

**Files:**
- Create: `core/server/reports/section_percent.go`
- Modify: `core/server/reports/bind.go` (`plan` struct line 36; `bind` line 216: call `b.sectionPercents(p)` after `b.layout(p)`)
- Modify: `core/server/reports/formula.go` (`compileNode` lines 341–371 and 394; `nodeKinds` line 486)
- Modify: `core/server/reports/rows.go` (`windowOp`, line 397)
- Modify: `core/server/reports/detail.go` (`windowSQL`, line 478)
- Modify: `core/server/reports/layout.go` (`cellOf`, line 417)
- Create: `core/server/reports/sectionpercent_test.go`

**Interfaces:**
- Consumes: `Node.Subtotal`, `needScale`, `windowSQL`, `nodeKinds`, `unknownColumn`, `LayoutItem.cellIDs`, `Opening.cellIDs`, `p.lookups`.
- Produces: `type sectionPercent struct { col, arg *colPlan; section string; scale int }`; `plan.percents map[string]*sectionPercent` (by column id); `(b *binder) sectionPercents(p *plan)`, `(b *binder) sectionPercent(p *plan, cp *colPlan, sections map[string]bool)`, `walkSpecFormulas(p *plan, visit func(n *Node, at string, columnRoot bool))`; formula op `percentOfSection` (a window op; type Percent; SQL `NULL`); a `subtotal` beside an op other than `percentOfSection` is refused ("only percentOfSection takes a subtotal beside its op"). Task 8 fills the figures.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/sectionpercent_test.go`:

```go
package reports

import "testing"

// percentStatement is ann's statement: Lions (f1 1000, f2 2500) and Zebras
// (f3 700, f4 300), labelled by note, with share = cost as a percent of a
// section. share may close its column and add more; extra adds top-level
// parts; layout adds layout items.
func percentStatement(share, groups, extra, layout string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"share","formula":` + share + `}],
		"groups":` + groups + `,` + extra + `
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"}` + layout + `]}`
}

const (
	shareOfLions = `{"op":"percentOfSection","args":[{"col":"cost"}],"subtotal":"cats","scale":1}`
	bySpecies    = `[{"id":"g","col":"species"}]`
	sortByNote   = `"sort":[{"col":"note"}],`
	gapRow       = `,{"row":"gap","label":"Lions less zebras","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}`
)

func TestPercentOfSectionIsAPercentColumn(t *testing.T) {
	z := newZoo(t)
	raw := percentStatement(shareOfLions, bySpecies, sortByNote, gapRow)
	if err := Validate(z.app, loadSpec(t, raw)); err != nil {
		t.Fatal(err)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if c := res.Columns[3]; c.ID != "share" || c.Type != Percent {
		t.Fatalf("share column = %+v", c)
	}
}

func TestPercentOfSectionRefusals(t *testing.T) {
	z := newZoo(t)
	notLayout := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"share","formula":` + shareOfLions + `}],"groups":[{"id":"g","col":"species"}]}`
	cases := map[string]struct {
		raw, at, want string
		status        int
	}{
		"outside a layout": {notLayout, "columns.2.formula", "only a report with a layout", 400},
		"unknown section": {percentStatement(`{"op":"percentOfSection","args":[{"col":"cost"}],"subtotal":"dogs","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.subtotal", "no section", 422},
		"no section": {percentStatement(`{"op":"percentOfSection","args":[{"col":"cost"}],"scale":1}`, bySpecies, "", ""),
			"columns.3.formula.subtotal", "names its section", 400},
		"argument not a column": {percentStatement(`{"op":"percentOfSection","args":[{"value":5}],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "takes one column", 400},
		"a null argument": {percentStatement(`{"op":"percentOfSection","args":[null],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "takes one column", 400},
		"argument not a number": {percentStatement(`{"op":"percentOfSection","args":[{"col":"note"}],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula", "needs a number", 400},
		"inside an operator": {percentStatement(`{"op":"add","args":[`+shareOfLions+`,{"value":1}]}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "whole formula", 400},
		"inside a window op": {percentStatement(`{"op":"runningSum","args":[`+shareOfLions+`]}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "whole formula", 400},
		"read by another column": {percentStatement(shareOfLions+`},{"id":"double","formula":{"op":"mul","args":[{"col":"share"},{"value":2}]}`, bySpecies, "", ""),
			"columns.4.formula.args.0", "no formula reads", 400},
		"with an aggregate": {percentStatement(shareOfLions+`,"aggregate":"sum"`, bySpecies, "", ""),
			"columns.3.aggregate", "no aggregate", 400},
		"the pivot's cell": {percentStatement(shareOfLions, bySpecies, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"share"},`, ""),
			"pivot.cell", "a column with an aggregate", 400},
		"a computed row's cell": {percentStatement(shareOfLions, bySpecies, "", `,{"row":"r","label":"R","formula":{"subtotal":"cats"},"cells":{"share":{"value":1}}}`),
			"layout.2.cells.share", "no cell formula", 400},
		"subtotal beside another op": {percentStatement(`{"op":"add","subtotal":"cats","args":[{"col":"cost"},{"value":1}]}`, bySpecies, "", ""),
			"columns.3.formula", "only percentOfSection takes a subtotal", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", c.raw, Options{}, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
		})
	}
}
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPercentOfSection' -count=1`
Expected: FAIL — the valid spec is refused (`percentOfSection` with `subtotal` has two node kinds: "errNodeKind"), and the refusal cases report other places or messages.

- [ ] **Step 3: Implement**

Create `core/server/reports/section_percent.go`:

```go
package reports

import "fmt"

// sectionPercent is a column whose formula is percentOfSection: each row's
// value of arg as a percent of the named section's total of arg, at scale.
// Its cells are filled after the statements (sectionPercentCell): a
// section's total holds its within rows, which only the layout's union legs
// compute, so no statement can read it beside the rows it divides (R1).
type sectionPercent struct {
	col, arg *colPlan
	section  string
	scale    int
}

// sectionPercents binds every percentOfSection column. It refuses the
// operator anywhere but a column's whole formula, and any formula, group or
// cell that reads such a column: its value is not in the statement, so a
// reader there would see blank (R2).
func (b *binder) sectionPercents(p *plan) {
	p.percents = map[string]*sectionPercent{}
	sections := map[string]bool{}
	for i := range p.spec.Layout {
		if s := p.spec.Layout[i].Section; s != "" {
			sections[s] = true
		}
	}
	for _, cp := range p.cols {
		if n := cp.col.Formula; n != nil && n.Op == "percentOfSection" {
			b.sectionPercent(p, cp, sections)
		}
	}
	walkSpecFormulas(p, func(n *Node, at string, columnRoot bool) {
		switch {
		case n.Op == "percentOfSection" && !columnRoot:
			b.bad(at, "percentOfSection is a column's whole formula, not a part of one")
		case n.Col != "" && p.percents[n.Col] != nil:
			b.bad(at, fmt.Sprintf("no formula reads %q: a percent of a section is filled after the report's totals", n.Col))
		}
	})
	for i := range p.spec.Groups {
		if p.percents[p.spec.Groups[i].Col] != nil {
			b.bad(fmt.Sprintf("groups.%d.col", i), "a report does not group by a percent of a section")
		}
	}
	for i := range p.spec.Layout {
		for _, id := range p.spec.Layout[i].cellIDs() {
			if p.percents[id] != nil {
				b.bad(fmt.Sprintf("layout.%d.cells.%s", i, id), "a percent of a section divides the row's own cell: it takes no cell formula")
			}
		}
	}
}

func (b *binder) sectionPercent(p *plan, cp *colPlan, sections map[string]bool) {
	n, at := cp.col.Formula, cp.at+".formula"
	switch {
	case len(p.spec.Layout) == 0:
		b.bad(at, "percentOfSection divides by a layout section's total: only a report with a layout has sections")
		return
	case n.Subtotal == "":
		b.bad(at+".subtotal", "percentOfSection names its section in subtotal")
		return
	case !sections[n.Subtotal]:
		b.missing(at+".subtotal", "unknown_subtotal", fmt.Sprintf("the layout has no section %q", n.Subtotal))
		return
	case cp.col.Aggregate != "":
		b.bad(cp.at+".aggregate", "a percent of a section has no aggregate: each row shows its own share")
		return
	// A null argument decodes to a nil node; refuse it here, before compile.
	case len(n.Args) != 1 || n.Args[0] == nil || n.Args[0].Col == "" || nodeKinds(n.Args[0]) != 1:
		b.bad(at+".args.0", `percentOfSection takes one column, {"col": <id>}`)
		return
	}
	arg := p.byID[n.Args[0].Col]
	if arg == nil {
		b.merge(unknownColumn(at+".args.0", n.Args[0].Col))
		return
	}
	// A missing or bad scale is refused where the formula compiles (needScale).
	scale := 0
	if n.Scale != nil {
		scale = *n.Scale
	}
	p.percents[cp.col.ID] = &sectionPercent{col: cp, arg: arg, section: n.Subtotal, scale: scale}
}

// walkSpecFormulas calls visit on every node of every formula the spec
// holds, with where it is. columnRoot is true only for the root node of a
// column's formula.
func walkSpecFormulas(p *plan, visit func(n *Node, at string, columnRoot bool)) {
	var walk func(n *Node, at string, root bool)
	walk = func(n *Node, at string, root bool) {
		if n == nil {
			return
		}
		visit(n, at, root)
		for i, a := range n.Args {
			walk(a, fmt.Sprintf("%s.args.%d", at, i), false)
		}
		walk(n.Where, at+".where", false)
		if l, ok := p.lookups[n]; ok {
			walk(l.value, at+".value", false)
		}
	}
	s := p.spec
	walk(s.Filter, "filter", false)
	for i := range s.Columns {
		c, at := &s.Columns[i], fmt.Sprintf("columns.%d", i)
		walk(c.Formula, at+".formula", true)
		walk(c.Rollup, at+".rollup", false)
		walk(c.Heading, at+".heading", false)
	}
	for i := range s.Sort {
		walk(s.Sort[i].Formula, fmt.Sprintf("sort.%d.formula", i), false)
	}
	for i := range s.Groups {
		g, at := &s.Groups[i], fmt.Sprintf("groups.%d", i)
		walk(g.Label, at+".label", false)
		walk(g.Having, at+".having", false)
		if g.Fill != nil {
			walk(g.Fill.Where, at+".fill.where", false)
		}
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
				walk(g.Opening.Cells[id], at+".opening.cells."+id, false)
			}
		}
	}
	for i := range s.Layout {
		item, at := &s.Layout[i], fmt.Sprintf("layout.%d", i)
		walk(item.Formula, at+".formula", false)
		for _, id := range item.cellIDs() {
			walk(item.Cells[id], at+".cells."+id, false)
		}
	}
	if s.Detail != nil {
		walk(s.Detail.Label, "detail.label", false)
	}
	for i := range s.Params {
		if o := s.Params[i].OptionsFrom; o != nil {
			walk(o.Where, fmt.Sprintf("params.%d.optionsFrom.where", i), false)
		}
	}
}
```

Before writing, confirm three names with one search each: the column `at` form (`grep -n 'at: *fmt.Sprintf("columns' core/server/reports/bind.go`), `SortKey.Formula`, and `LayoutItem.cellIDs` (`grep -n "func (.*LayoutItem) cellIDs" core/server/reports/*.go`). If one differs, use the code's name.

In `bind.go`, add to `plan` after `derived`:

```go
	// percents binds each percentOfSection column by id (sectionPercents).
	percents map[string]*sectionPercent
```

and in `bind`, after `b.layout(p)`:

```go
	b.sectionPercents(p)
```

In `formula.go` `compileNode`, before `if kinds := nodeKinds(n); kinds != 1 {`:

```go
	if n.Subtotal != "" && n.Op != "" && n.Op != "percentOfSection" {
		return expr{}, invalid(at, "only percentOfSection takes a subtotal beside its op")
	}
```

change the switch case `case n.Subtotal != "":` to `case n.Subtotal != "" && n.Op == "":`, and the window case to:

```go
	case "runningSum", "percentOfParent", "percentOfTotal", "percentOfSection":
```

In `nodeKinds`, count the subtotal as a kind only on a leaf (percentOfSection's `subtotal` names its section beside its op):

```go
	for _, set := range []bool{n.Op != "", n.Col != "", path, n.Outer != nil, n.Param != "", n.Value != nil, n.Subtotal != "" && n.Op == "", n.Record != ""} {
```

In `rows.go` `windowOp`:

```go
	case "runningSum", "percentOfParent", "percentOfTotal", "percentOfSection":
```

In `detail.go` `windowSQL`, after the `!isNumeric(e.typ) || e.win` check:

```go
	if op == "percentOfSection" {
		if e.typ == Bool {
			return expr{}, invalid(at, op+" needs a number that is not itself a running value")
		}
		// A placeholder: the figure needs the section's total with its within
		// rows, which only the layout's legs hold, so assemble fills it
		// (sectionPercentCell).
		return expr{sql: "NULL", typ: Percent, win: true}, nil
	}
```

In `layout.go` `cellOf`, make the switch's first case:

```go
	case lc.c.p.percents[id] != nil:
		// A computed row's or section total's percent divides its own cell;
		// assemble fills it (sectionPercentCell).
		return expr{sql: "NULL", typ: Percent}, nil
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPercentOfSection|TestRunningValues|TestARunningValue|TestLayout|TestFormula' -count=1`
Expected: PASS. If a refusal reports a different `at` for the same problem (for example the compiler refuses before the binder walk reaches it), keep the binder's place and message: the binder runs first, so its refusal is the one a spec author sees.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): bind and compile percentOfSection`

---

### Task 8: P1 Go — the figures

**Files:**
- Modify: `core/server/reports/section_percent.go` (new `readSectionBases`, `sectionPercentCell`)
- Modify: `core/server/reports/layout.go` (`layoutRows`, lines 723–764: split out `sectionsByKey` and `sectionRow`)
- Modify: `core/server/reports/assemble.go` (`assemble` line 285; `cells` line 814)
- Modify: `core/server/reports/run.go` (`runner` struct line 234; `run` after `rn.assemble(...)` line 444 and its `openGroups` calls lines 410, 419; `openGroups` line 1523 returns `([][]any, error)`)
- Modify: `core/server/reports/sectionpercent_test.go`

**Interfaces:**
- Consumes: Task 7's `plan.percents`; `sqlPct` (functions.go); `withinTotal`, `sectionKey`, `sectionTotalID`, `emptySectionID`, `rn.currencyKey`.
- Produces: `(rn *runner) sectionsByKey(t *tree, ck string) map[string]map[string]any`; `(rn *runner) sectionRow(t *tree, item *LayoutItem, total map[string]any, byKey map[string]map[string]any) map[string]any`; `runner.sectionBases map[string]any` (key: percent column id + `"|"` + currency key); `(rn *runner) readSectionBases(t *tree, total map[string]any)`; `(rn *runner) sectionPercentCell(sp *sectionPercent, row map[string]any) any`; `runner.assembleErr error`; `(rn *runner) openGroups(groupRows []map[string]any) ([][]any, error)` (was `[][]any`). Every detail, group, subtotal, section and computed row carries the percent (an opening row has its own cells, R3); a zero or blank section total gives nil.

- [ ] **Step 1: Write the failing tests**

Append to `core/server/reports/sectionpercent_test.go` (imports become `"maps"`, `"reflect"`, `"strings"`, `"testing"`):

```go
// sharesOf is the share cell of each row of the kinds (every kind when none),
// by label.
func sharesOf(res *Result, kinds ...string) map[string]any {
	out := map[string]any{}
	for _, r := range res.Rows {
		if len(kinds) == 0 || slices.Contains(kinds, r.Kind) {
			out[r.Label] = r.Cells["share"]
		}
	}
	return out
}

func requireShares(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !maps.EqualFunc(got, want, func(a, b any) bool { return reflect.DeepEqual(a, b) }) {
		t.Fatalf("shares = %v\nwant     %v", got, want)
	}
}

func TestPercentOfSectionOnEveryRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{ExpandAll: true}, nil)
	// Every row divides its own cost by the Lions total, 3500.
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "f1": 28.6, "f2": 71.4, "Total Lions": 100.0,
		"Zebras": 28.6, "f3": 20.0, "f4": 8.6, "Total Zebras": 28.6,
		"Lions less zebras": 71.4,
	})
	stripes := strings.ReplaceAll(shareOfLions, `"cats"`, `"stripes"`)
	res = z.run(t, "ann", percentStatement(stripes, bySpecies, sortByNote, gapRow), Options{ExpandAll: true}, nil)
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 350.0, "f1": 100.0, "f2": 250.0, "Total Lions": 350.0,
		"Zebras": 100.0, "f3": 70.0, "f4": 30.0, "Total Zebras": 100.0,
		"Lions less zebras": 250.0,
	})
}

// A section's total holds its within rows, so the percent divides by it.
func TestPercentOfSectionDividesByTheTotalWithItsWithinRows(t *testing.T) {
	z := newZoo(t)
	within := `,{"row":"fees","label":"Lion fees","within":"cats","formula":{"subtotal":"stripes"}}` + gapRow
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, within), Options{ExpandAll: true}, nil)
	// Lions 3500 + fees 1000 = 4500.
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "f1": 22.2, "f2": 55.6, "Lion fees": 22.2, "Total Lions": 100.0,
		"Zebras": 22.2, "f3": 15.6, "f4": 6.7, "Total Zebras": 22.2,
		"Lions less zebras": 77.8,
	})
}

func TestPercentOfSectionOnASecondLevelSubtotal(t *testing.T) {
	z := newZoo(t)
	twoLevels := `[{"id":"g","col":"species"},{"id":"n","col":"note","subtotals":true}]`
	res := z.run(t, "ann", percentStatement(shareOfLions, twoLevels, sortByNote, ""), Options{ExpandAll: true}, nil)
	// The second-level group rows (f1 … f4) and their subtotals, with the
	// sections' own rows, each divide by the Lions total.
	requireShares(t, sharesOf(res, KindGroup, KindSubtotal), map[string]any{
		"Lions": 100.0, "f1": 28.6, "f2": 71.4,
		"Total f1": 28.6, "Total f2": 71.4, "Total Lions": 100.0,
		"Zebras": 28.6, "f3": 20.0, "f4": 8.6,
		"Total f3": 20.0, "Total f4": 8.6, "Total Zebras": 28.6,
	})
}

func TestPercentOfAnEmptySectionHasNoFigure(t *testing.T) {
	z := newZoo(t)
	// Ann sees no monkey: the Monkeys total is 0.
	apes := strings.ReplaceAll(shareOfLions, `"cats"`, `"apes"`)
	monkeys := `,{"section":"apes","title":"Monkeys","key":"monkey"}` + gapRow
	// lionShare in the same report shows the run fills figures at all.
	withLions := apes + `},{"id":"lionShare","formula":` + shareOfLions
	res := z.run(t, "ann", percentStatement(withLions, bySpecies, sortByNote, monkeys), Options{}, nil)
	for label, share := range sharesOf(res) {
		if share != nil {
			t.Fatalf("%s share = %v, want blank", label, share)
		}
	}
	if got := res.Rows[0].Cells["lionShare"]; got != 100.0 {
		t.Fatalf("Lions lionShare = %v, want 100.0", got)
	}
}

// On a pivot report the percent reads the cell column's total across the
// pivot (D10).
func TestPercentOfSectionOnAPivotReport(t *testing.T) {
	z := newZoo(t)
	pivot := `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},`
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, pivot, gapRow), Options{}, nil)
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "Total Lions": 100.0, "Zebras": 28.6, "Total Zebras": 28.6, "Lions less zebras": 71.4,
	})
	requireRows(t, res, "cost.p1", int64(2500), int64(2500), int64(0), int64(0), int64(2500))
}

func TestPercentOfSectionDividesByItsOwnCurrencysTotal(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f9", "zoo_feedings", map[string]any{
		"animal": z.recs["leo"].Id, "cost": 1000, "currency": "EUR", "fed": "2026-01-07 00:00:00.000Z", "grams": 5, "note": "f9", "due": "2026-01-07",
	})
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{}, nil)
	var lions []any
	var zebrasUSD any
	for _, r := range res.Rows {
		switch {
		case r.Kind == KindGroup && r.Label == "Lions":
			lions = append(lions, r.Cells["share"])
		case r.Kind == KindGroup && r.Label == "Zebras" && strings.Contains(r.ID, `"USD"`):
			zebrasUSD = r.Cells["share"]
		}
	}
	// EUR Lions 1000 of 1000, USD Lions 3500 of 3500: each its own block's.
	if !reflect.DeepEqual(lions, []any{100.0, 100.0}) || zebrasUSD != 28.6 {
		t.Fatalf("lions = %v, USD zebras = %v", lions, zebrasUSD)
	}
}

func TestPercentOfSectionReadsTheTotalWhenTheSectionIsFolded(t *testing.T) {
	z := newZoo(t)
	raw := percentStatement(shareOfLions, bySpecies, sortByNote, gapRow)
	want := map[string]any{"Lions": 100.0, "Total Lions": 100.0, "Zebras": 28.6, "Total Zebras": 28.6, "Lions less zebras": 71.4}
	folded := z.run(t, "ann", raw, Options{}, nil)
	requireShares(t, sharesOf(folded), want)
	open := z.run(t, "ann", raw, Options{ExpandAll: true}, nil)
	requireShares(t, sharesOf(open, KindGroup, KindSubtotal, KindComputed), want)
}
```

Add `"slices"` to the imports too (`sharesOf` uses it).

- [ ] **Step 2: Run the tests and see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPercentOf' -count=1`
Expected: FAIL — every share is nil (the SQL placeholder).

- [ ] **Step 3: Implement**

In `layout.go`, replace the head of `layoutRows` and its section branch:

```go
func (rn *runner) layoutRows(t *tree, total map[string]any) {
	ck := rn.currencyKey(total)
	byKey := rn.sectionsByKey(t, ck)
	for _, item := range rn.c.p.layout {
		// A within row is emitted inside its section (groupWithLabel).
		if item.Within != "" {
			continue
		}
		if item.Row != "" {
			row := t.computed[item.Row+"|"+ck]
			if row == nil {
				row = map[string]any{}
			}
			id := "c:" + item.Row + rn.currencySuffix(total)
			rn.push(Row{ID: id, Kind: KindComputed, Label: item.Label, Cells: rn.cells(row)}, row)
			continue
		}
		row := rn.sectionRow(t, item, total, byKey)
		groupKey := rn.rowKey(row, 1)
		rn.groupWithLabel(t, row, 1, 0, "", "g:"+keyString(groupKey), rowLabel{text: item.Title}, groupKey, true, item)
	}
}

// sectionsByKey is one currency's first-level group rows by key.
func (rn *runner) sectionsByKey(t *tree, ck string) map[string]map[string]any {
	byKey := map[string]map[string]any{}
	for _, g := range t.children[ck] {
		byKey[keyString([]any{g["k0"]})] = g
	}
	return byKey
}

// sectionRow is a section's group row in one currency: its group's row, or
// a zero row for a section with no rows, with its within rows added. It may
// be the group's own row, so a caller must not change it.
func (rn *runner) sectionRow(t *tree, item *LayoutItem, total map[string]any, byKey map[string]map[string]any) map[string]any {
	ck := rn.currencyKey(total)
	key := sectionKey(item)
	row := byKey[keyString([]any{key})]
	if row == nil {
		row = map[string]any{"k0": key, "__level": int64(1)}
		for _, name := range rn.c.currencies {
			row[name] = total[name]
		}
		// A section the row cap cut stays blank: it may hold rows.
		if zero := t.computed[emptySectionID+"|"+ck]; zero != nil && !rn.sectionsCapped {
			row = rn.withinTotal(row, zero)
		}
	}
	if sum := t.computed[sectionTotalID(item.Section)+"|"+ck]; sum != nil {
		row = rn.withinTotal(row, sum)
	}
	return row
}
```

The group key was read before `withinTotal` and is now read after it; `withinTotal` changes only `v`, `vcur`, `vmx`, `pv` and `pvm` cells, never `k0` or a currency key, so the key is the same.

Append to `section_percent.go` (imports become `"database/sql/driver"` and `"fmt"`):

```go
// readSectionBases reads, for one currency block, each percent column's
// section total of its argument from the row the section shows, so a
// folded section divides by the same total.
func (rn *runner) readSectionBases(t *tree, total map[string]any) {
	if len(rn.c.p.percents) == 0 {
		return
	}
	ck := rn.currencyKey(total)
	byKey := rn.sectionsByKey(t, ck)
	for _, item := range rn.c.p.layout {
		if item.Section == "" {
			continue
		}
		row := rn.sectionRow(t, item, total, byKey)
		for id, sp := range rn.c.p.percents {
			if sp.section == item.Section {
				rn.sectionBases[id+"|"+ck] = row[fmt.Sprintf("v%d", rn.c.p.index(sp.arg))]
			}
		}
	}
}

// sectionPercentCell is a row's own argument cell as a percent of its
// currency block's section total, rounded as tc_pct rounds: blank for a
// blank cell or a zero or blank total.
func (rn *runner) sectionPercentCell(sp *sectionPercent, row map[string]any) any {
	part := row[fmt.Sprintf("v%d", rn.c.p.index(sp.arg))]
	whole := rn.sectionBases[sp.col.col.ID+"|"+rn.currencyKey(row)]
	v, err := sqlPct(nil, []driver.Value{part, whole, int64(sp.scale)})
	if err != nil {
		if rn.assembleErr == nil {
			rn.assembleErr = err
		}
		return nil
	}
	return v
}
```

In `run.go`, add to the `runner` struct:

```go
	// sectionBases holds each percentOfSection column's section total of its
	// argument per currency block, by column id + "|" + currency key
	// (readSectionBases).
	sectionBases map[string]any
	// assembleErr is the first error assembling the rows met (a percent that
	// overflows); run returns it.
	assembleErr error
```

and in `run`, right after `rn.assemble(groupRows, details, nested)` (line 444):

```go
	if rn.assembleErr != nil {
		return rn.assembleErr
	}
```

`openGroups` (run.go line 1523) assembles the same group rows a second time, before the detail pass (lines 410 and 419), and that assembly fills the percent cells of the group, subtotal and computed rows too, so it can meet the same error. Check it there as well. Replace `openGroups` with:

```go
// openGroups lists the open innermost groups in the order the report shows
// them (R9a): the group rows that an assembly with no detail rows marks
// expanded at the innermost depth. That assembly is thrown away, but an error
// it met (a percent that overflows) is the run's.
func (rn *runner) openGroups(groupRows []map[string]any) ([][]any, error) {
	rows, notices := rn.res.Rows, rn.res.Notices
	rn.res.Rows, rn.res.Notices = []Row{}, slices.Clone(notices)
	rn.assemble(groupRows, nil, nil)
	if rn.assembleErr != nil {
		return nil, rn.assembleErr
	}
	var out [][]any
	for _, r := range rn.res.Rows {
		if r.Kind == KindGroup && r.Expanded && r.Level == len(rn.c.p.groups) {
			out = append(out, r.GroupKey)
		}
	}
	rn.res.Rows, rn.res.Notices = rows, notices
	return out, nil
}
```

and its two callers in `run`:

```go
	case paged:
		// The page's open innermost groups, anchors included, in document
		// order. In mode after the cursor's own group was not read, so it is
		// never one of them; the group cursor was read and checked already.
		leaves, err := rn.openGroups(groupRows)
		if err != nil {
			return err
		}
		if len(leaves) > 0 {
			if details, err = rn.detailPass(leaves); err != nil {
				return err
			}
		}
	case rn.c.p.pivot == nil:
		var leaves [][]any
		grouped := len(rn.c.p.groups) > 0
		if grouped {
			if leaves, err = rn.openGroups(groupRows); err != nil {
				return err
			}
		}
```

(the rest of the `rn.c.p.pivot == nil` case is unchanged). `grep -n 'openGroups(' core/server/reports/*.go` must show only these two callers and the definition.

In `assemble.go` `assemble`, before `for _, total := range t.totals {`:

```go
	rn.sectionBases = map[string]any{}
```

and in the loop's layout branch:

```go
		if len(rn.c.p.layout) > 0 {
			rn.readSectionBases(t, total)
			rn.layoutRows(t, total)
			continue
		}
```

In `cells`, before the pivot block:

```go
	for id, sp := range rn.c.p.percents {
		if sp.col.col.Hidden {
			continue
		}
		if _, ok := row[fmt.Sprintf("v%d", rn.c.p.index(sp.col))]; ok {
			out[id] = rn.sectionPercentCell(sp, row)
		}
	}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestPercentOf|TestLayout|TestPivot|TestRunningValues' -count=1`
Expected: PASS.
Then the package once: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver**

Commit message: `feat(reports): fill percentOfSection from each section's total`

---

Next: Part 2 (Tasks 9–13: rule parity and the P&L-by-month statement test, the builder's "Percent of a section's total", the pivot range and order controls, the e2e, help and docs) — see the outline's Tasks list.
