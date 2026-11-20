# Report Engine Query Scaling Implementation Plan — part 16 (task 9c-0b, part b: the opening row's label names a value of the run)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A group level's opening row can name a value of the run in its label, so a report can show "Balance before Nov 2026" on screen (web and native) and in every export.

**Architecture:** The engine sends typed values and each view formats them; an opening row follows that rule. The spec's `opening` gets `labelValue` (a formula of params and values only, compiled as a column heading is) and `labelBucket` (cuts a day to its month, quarter or year); its `label` holds the token `{value}` once. The run reads the value in the statement that already reads the column headings (no new statement), and the opening row carries `labelTemplate` (the spec's label), `labelType` and `labelValue` (for a month: `month` and `"2026-11"`). The client (`rowLabelText`) and the exporter (`RowLabel`) format the value with the code they have (`bucketWords` gives "Nov 2026") and put it in the token's place.

**Tech Stack:** Go and SQLite (PocketBase) in `core/server/reports`; TypeScript, zod and vitest in `core/reports/lib`; Playwright in `tests/e2e`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part16.md` (sections "What the human asked" and "Design"). Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`. The code of the three tasks was built and run against core `68c6d3d4` before this plan was written (but not the e2e step of task 3: the planner did not run e2e); the diff is kept in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part16-scratch/part16.patch` (reference only: write the code from the steps).

This part holds part (b) of task 9c-0b only. Part (a) is `docs/plans/2026-11-20-report-engine-scale-part15.md`.

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core is `/Users/nas/tmp/tinycld/books-purchases/tinycld`. Branch `books/p5-report-engine-scale`. The three tasks are core tasks: they edit only files under `tinycld/core/server/reports`, `tinycld/core/reports/lib`, `tinycld/core/help`, `tinycld/tests/e2e` and `tinycld/docs/packages.md`.
- The tasks run in order: task 2 reads the JSON of task 1, and task 3 reads `LABEL_VALUE_TOKEN` and the two schema keys of task 2. Between task 2 and task 3 the builder drops `labelValue` and `labelBucket` on save; task 3 ends that.
- E2E: only `tinycld/tests/e2e/reports-ledger-v3.spec.ts`, in one foreground command, with the lane ports and the port check of the lane rules. The spec drives the builder through the UI: no `page.goto()` for in-app navigation, no raw PocketBase write, no bumped timeout.
- Core names no package: no feature slug, collection or route in code, tests, comments, docs or help. Go tests use the zoo sources (`zoo.*`).
- Native and web both: the label is made in `rowLabelText` (`core/reports/lib/format.ts`), which every platform's row component and the column measure call. No platform file changes.
- The token is `{value}`, exactly. Go: `LabelValueToken`. TS: `LABEL_VALUE_TOKEN`.
- The JSON names are `labelValue` and `labelBucket` (spec, in `groups[].opening`) and `labelTemplate` (result row).
- A row with no `labelTemplate` formats as before. The figures and labels of every existing test stay.
- No backward-compat code: the spec version does not change (the two fields are optional parts of version 4).
- Comments say why, not what. No timing probe in this part.
- Change files with the Edit tool only (hooks block scripted edits). Lane rules: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`.

## Review Focus

- A text value that holds `$&` or `$1`: JavaScript's `String.replace` reads those as patterns in a string replacement. The label must show the value as it is. Pinned by the vitest test "puts the typed value of an opening row in its label template" (task 2), by the two shared cases of `format-cases.json` (task 2) and by the Go test `TestAnOpeningLabelNamesATextOrAComputedValue` (task 1).
- A value that is blank on the run (an unset param, an empty or spaces-only text): the label shows without the token and without a double space, and the row has no typed label. Pinned by `TestAnOpeningLabelWithNoValueLeavesTheTokenOut`.
- A page after the first: the value is read on every run, so each page's opening rows have it. Pinned by `TestAnOpeningLabelValueIsOnEveryPage`.
- An opening on an inner level, and on a layout section: the value is found by the group's level (not the row's depth). Pinned by `TestAnOpeningLabelValueOnALayoutSectionAndAnInnerLevel`.
- A report opened in the builder and saved: the builder has no control for the label value, so it must keep `labelValue` and `labelBucket`, keep the param that only the label value reads, and refuse a label whose `{value}` was deleted before the server does. Pinned by the two draft tests of task 3, and on the builder screen by the e2e step of task 3 (a `{value}` typed in the **Row label** of a row that has no label value).
- A `labelBucket` on a value that is not a day (a text, a date-time, nothing): refused at `groups.<i>.opening.labelBucket`, because a date-time's day depends on the reader's time zone. Pinned by `TestOpeningLabelValueRefusals` ("bucket of a text", "bucket of nothing").

Not pinned by a test of this part:

- The label on the report screen. No shipped report of core has an opening row with a label value, so there is no screen to drive; task 9c-1 (Books) uses the field and its e2e spec reads the label on the screen.
- The builder messages `Write {value} once, where the row’s value goes` and `Write some text besides {value}` on the builder screen. They show only for a report that has a label value, and such a report comes only from a package, so no core screen can reach them. The draft test of task 3 pins their text and their path (`groups.<i>.opening.label`, the field the e2e step of task 3 reads the third message from); task 9c-1 (Books) covers them on the screen.

## File Structure

- Modify `core/server/reports/spec.go`: `Opening.LabelValue`, `Opening.LabelBucket`, `LabelValueToken`.
- Modify `core/server/reports/bind.go`: the label and bucket rules (`opening`), the formula size check (`formulaSizes`).
- Modify `core/server/reports/section_percent.go`: `walkSpecFormulas` visits the label value.
- Modify `core/server/reports/heading.go`: the label value compiles and is read with the headings.
- Modify `core/server/reports/run.go`: `Row.LabelTemplate`, `runner.openingLabels`, `compileAll`.
- Modify `core/server/reports/assemble.go`: the opening row gets its label.
- Modify `core/server/reports/export_format.go`: `RowLabel` fills the template.
- Create `core/server/reports/openinglabel_test.go`: the zoo tests.
- Modify `docs/packages.md`: the row kinds bullet and the `groups[].opening` row.
- Modify `core/reports/lib/types.ts`, `spec.ts`, `format.ts`: the row field, the spec fields and token, the label text.
- Modify `core/reports/lib/fixtures/format-cases.json`: two label cases that the client test (`core/reports/lib/__tests__/format-cases.test.ts`) and the exporter test (`TestExportFormatterRowLabelsMatchTheClient`) both read (task 2).
- Modify `core/reports/lib/__tests__/format.test.ts`, `spec.test.ts`: the tests of task 2.
- Modify `core/reports/lib/builder/draft.ts`: the builder keeps the two fields and checks the label (task 3).
- Modify `core/reports/lib/builder/__tests__/draft.test.ts`: the tests of task 3.
- Modify `tests/e2e/reports-ledger-v3.spec.ts`: the builder shows the message for a `{value}` in a label with no label value (task 3).
- Modify `core/help/reports.md`: the "Opening rows" topic (task 3).

---

### Task 1: The engine reads an opening row's label value and the exporter writes it

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/openinglabel_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/spec.go` (`Opening`, near line 339)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/bind.go` (`formulaSizes` near line 508, `opening` near line 1045)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/section_percent.go` (`walkSpecFormulas`, near line 162)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/heading.go` (whole file is 130 lines)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (`Row` near line 235, `runner.headings` near line 337, `compileAll` near line 2437)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/assemble.go` (`groupWithLabel`, near lines 744-779)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/export_format.go` (`RowLabel`, near line 722)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md` (lines 1496-1499 and 1561)

**Interfaces:**
- Consumes (they exist, do not change them): `newZoo(t) *zoo`, `(z *zoo) run(t, user, raw string, opts Options, params map[string]string) *Result` (`run_test.go`), `(z *zoo) validate(t, raw string) error` (`drill_test.go`), `requireSpecError(t, err, status int, at, contains string)` (`spec_test.go`), `requireRows`, `rowsOf` (`run_test.go`), `zooHeadings` (`colheading_test.go`), `fromFebruary` (`opening_test.go`), `newExportFormatter(exportDisplay{}, "UTC", false)` (`export_format.go`), `compileFormula(c fctx, n *Node, at string) (expr, error)`, `bucketSQL(c fctx, bucket, day string) string` (`rows.go`), `bucketType(bucket string) FieldType` (`registry.go`), `valueString(v any) string` (`export_format.go`), `groupPlan.at` (`"groups.<i>"`), `Page.Next`.
- Produces:
  - `Opening.LabelValue *Node` (`json:"labelValue,omitempty"`), `Opening.LabelBucket string` (`json:"labelBucket,omitempty"`), `const LabelValueToken = "{value}"`.
  - `Row.LabelTemplate string` (`json:"labelTemplate,omitempty"`): on an opening row with a value this run, with `Row.LabelType` and `Row.LabelValue`.
  - `(f *exportFormatter) RowLabel(row Row) string` fills the template.
  - Refusal paths: `groups.<i>.opening.label`, `groups.<i>.opening.labelBucket`, `groups.<i>.opening.labelValue` (all 400).
  - For task 9c-1 (Books): `Opening{Label: "Balance before {value}", LabelValue: &Node{Param: "<range param>", Bound: "from"}, LabelBucket: "month", Cells: …}` gives "Balance before Feb 2026" for a period that starts in February 2026 (pinned in the test below with a zoo `dateRange` param).

- [ ] **Step 1: Write the failing test**

Create `core/server/reports/openinglabel_test.go`:

```go
package reports

import (
	"fmt"
	"testing"
)

// gramsWithOpening groups ann's feedings by animal, each group opening onto
// the given opening row. asOf is a day param and note a text param: the two
// kinds of value an opening row's label names.
func gramsWithOpening(opening string) string {
	return `{"version":4,"source":"zoo.feedings",
		"params":[{"id":"asOf","type":"asOf","label":"As of"},{"id":"note","type":"text","label":"Note"}],
		"columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","open":true,"subtotals":true,"opening":` + opening + `}],
		"sort":[{"col":"note"}]}`
}

var julyEnd = map[string]string{"asOf": `"2026-07-31"`}

// openingRows are the run's opening rows, failing the test when it has none.
func openingRows(t *testing.T, res *Result) []Row {
	t.Helper()
	var out []Row
	for _, r := range res.Rows {
		if r.Kind == KindOpening {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no opening row in %v", rowsOf(res, "kind"))
	}
	return out
}

func TestAnOpeningLabelNamesTheMonthOfADayParam(t *testing.T) {
	z := newZoo(t)
	spec := gramsWithOpening(`{"label":"Balance before {value}","labelValue":{"param":"asOf"},"labelBucket":"month"}`)
	res := z.run(t, "ann", spec, Options{}, julyEnd)
	requireRows(t, res, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	for _, r := range openingRows(t, res) {
		if r.Label != "Balance before 2026-07" || r.LabelTemplate != "Balance before {value}" || r.LabelType != Month || r.LabelValue != "2026-07" || r.LabelCurrency != "" {
			t.Fatalf("opening row = %+v", r)
		}
		if got := f.RowLabel(r); got != "Balance before Jul 2026" {
			t.Fatalf("exported label = %q", got)
		}
	}
}

// Each bucket gives its key and its label type, as a bucketed group does; with
// no bucket the value is the day itself.
func TestAnOpeningLabelValueByBucket(t *testing.T) {
	z := newZoo(t)
	cases := []struct {
		bucket string
		typ    FieldType
		value  string
		words  string
	}{
		{"", Date, "2026-07-31", "Jul 31, 2026"},
		{"day", Day, "2026-07-31", "Jul 31, 2026"},
		{"month", Month, "2026-07", "Jul 2026"},
		{"quarter", Quarter, "2026-Q3", "Q3 2026"},
		{"year", Year, "2026", "2026"},
		{"fiscalYear", FiscalYear, "2026", "FY 2026"},
	}
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	for _, c := range cases {
		t.Run("bucket "+c.bucket, func(t *testing.T) {
			bucket := ""
			if c.bucket != "" {
				bucket = `,"labelBucket":"` + c.bucket + `"`
			}
			res := z.run(t, "ann", gramsWithOpening(`{"label":"Before {value}:","labelValue":{"param":"asOf"}`+bucket+`}`), Options{}, julyEnd)
			r := openingRows(t, res)[0]
			if r.LabelType != c.typ || fmt.Sprint(r.LabelValue) != c.value || r.Label != "Before "+c.value+":" {
				t.Fatalf("opening row = %+v", r)
			}
			if got := f.RowLabel(r); got != "Before "+c.words+":" {
				t.Fatalf("exported label = %q", got)
			}
		})
	}
}

func TestAnOpeningLabelNamesATextOrAComputedValue(t *testing.T) {
	z := newZoo(t)
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	text := z.run(t, "ann", gramsWithOpening(`{"label":"Before {value}","labelValue":{"param":"note"}}`), Options{}, map[string]string{"note": `"the $1 audit"`})
	r := openingRows(t, text)[0]
	if r.Label != "Before the $1 audit" || r.LabelType != Text || r.LabelValue != "the $1 audit" || f.RowLabel(r) != "Before the $1 audit" {
		t.Fatalf("text opening row = %+v, exported %q", r, f.RowLabel(r))
	}
	// The first day of a period: the bound of a date range is a day too.
	period := `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]},
		"groups":[{"id":"g","col":"animal","open":true,"opening":{"label":"Balance before {value}","labelValue":{"param":"period","bound":"from"},"labelBucket":"month"}}]}`
	r = openingRows(t, z.run(t, "ann", period, Options{}, fromFebruary))[0]
	if r.LabelType != Month || r.LabelValue != "2026-02" || f.RowLabel(r) != "Balance before Feb 2026" {
		t.Fatalf("period opening row = %+v, exported %q", r, f.RowLabel(r))
	}
	// A year before the day asked: the value is computed, then cut to its month.
	prior := `{"label":"Before {value}","labelValue":{"op":"addMonths","args":[{"param":"asOf"},{"value":-12}]},"labelBucket":"month"}`
	r = openingRows(t, z.run(t, "ann", gramsWithOpening(prior), Options{}, julyEnd))[0]
	if r.LabelValue != "2025-07" || f.RowLabel(r) != "Before Jul 2025" {
		t.Fatalf("computed opening row = %+v, exported %q", r, f.RowLabel(r))
	}
}

// A value that is blank on this run leaves the token out, with the space it
// stood in, and the row has no typed label: a view shows Label as it is.
func TestAnOpeningLabelWithNoValueLeavesTheTokenOut(t *testing.T) {
	z := newZoo(t)
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	spec := gramsWithOpening(`{"label":"Balance before {value} (opening)","labelValue":{"param":"note"}}`)
	for name, params := range map[string]map[string]string{
		"unset":  nil,
		"empty":  {"note": `""`},
		"spaces": {"note": `"  "`},
	} {
		t.Run(name, func(t *testing.T) {
			for _, r := range openingRows(t, z.run(t, "ann", spec, Options{}, params)) {
				if r.Label != "Balance before (opening)" || r.LabelTemplate != "" || r.LabelType != "" || r.LabelValue != nil {
					t.Fatalf("opening row = %+v", r)
				}
				if got := f.RowLabel(r); got != "Balance before (opening)" {
					t.Fatalf("exported label = %q", got)
				}
			}
		})
	}
}

// An opening row with no label value is as before: a plain label, no template.
func TestRegressionGuardAnOpeningLabelWithNoLabelValueIsPlainText(t *testing.T) {
	z := newZoo(t)
	for _, r := range openingRows(t, z.run(t, "ann", gramsWithOpening(`{"label":"Before the period"}`), Options{}, julyEnd)) {
		if r.Label != "Before the period" || r.LabelTemplate != "" || r.LabelType != "" || r.LabelValue != nil {
			t.Fatalf("opening row = %+v", r)
		}
	}
}

// A layout section's opening row is made on the same path, and so is one on
// an inner level: the value is found by the group's level, not its depth.
func TestAnOpeningLabelValueOnALayoutSectionAndAnInnerLevel(t *testing.T) {
	z := newZoo(t)
	opening := `{"label":"Start of {value}","labelValue":{"param":"asOf"},"labelBucket":"year"}`
	layout := `{"version":4,"source":"zoo.feedings","params":[{"id":"asOf","type":"asOf","label":"As of"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","opening":` + opening + `}],
		"sort":[{"col":"note"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"}]}`
	inner := `{"version":4,"source":"zoo.feedings","params":[{"id":"asOf","type":"asOf","label":"As of"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","open":true},{"id":"g","col":"animal","open":true,"opening":` + opening + `}],
		"sort":[{"col":"note"}]}`
	for name, spec := range map[string]string{"layout": layout, "inner": inner} {
		t.Run(name, func(t *testing.T) {
			for _, r := range openingRows(t, z.run(t, "ann", spec, Options{ExpandAll: true}, julyEnd)) {
				if r.Label != "Start of 2026" || r.LabelType != Year || r.LabelValue != "2026" {
					t.Fatalf("opening row = %+v", r)
				}
			}
		})
	}
}

// A page that starts after the first carries the label value too: the value
// is read on every run, not only on the first page.
func TestAnOpeningLabelValueIsOnEveryPage(t *testing.T) {
	z := newZoo(t)
	spec := gramsWithOpening(`{"label":"Balance before {value}","labelValue":{"param":"asOf"},"labelBucket":"month"}`)
	whole := openingRows(t, z.run(t, "ann", spec, Options{}, julyEnd))
	var paged []Row
	opts := Options{Limit: 3}
	for range 20 {
		res := z.run(t, "ann", spec, opts, julyEnd)
		for _, r := range res.Rows {
			if r.Kind == KindOpening {
				paged = append(paged, r)
			}
		}
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		opts.Cursor = res.Page.Next
	}
	if len(paged) != len(whole) {
		t.Fatalf("%d opening rows over the pages, %d in the whole run", len(paged), len(whole))
	}
	for _, r := range paged {
		if r.Label != "Balance before 2026-07" || r.LabelType != Month || r.LabelValue != "2026-07" || r.LabelTemplate != "Balance before {value}" {
			t.Fatalf("paged opening row = %+v", r)
		}
	}
}

func TestOpeningLabelValueRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ opening, at, want string }{
		"no token":             {`{"label":"Balance before","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "{value} once in its label"},
		"two tokens":           {`{"label":"{value} to {value}","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "{value} once in its label"},
		"only the token":       {`{"label":" {value} ","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "text besides {value}"},
		"token with no value":  {`{"label":"Balance before {value}"}`, "groups.0.opening.label", "needs a labelValue"},
		"bucket with no value": {`{"label":"Balance before","labelBucket":"month"}`, "groups.0.opening.labelBucket", "needs a labelValue"},
		"unknown bucket":       {`{"label":"Before {value}","labelValue":{"param":"asOf"},"labelBucket":"week"}`, "groups.0.opening.labelBucket", "day, month, quarter, year or fiscalYear"},
		"bucket of a text":     {`{"label":"Before {value}","labelValue":{"param":"note"},"labelBucket":"month"}`, "groups.0.opening.labelBucket", "a labelValue that is a day"},
		"bucket of nothing":    {`{"label":"Before {value}","labelValue":{"value":null},"labelBucket":"month"}`, "groups.0.opening.labelBucket", "a labelValue that is a day"},
		"path leaf":            {`{"label":"Before {value}","labelValue":{"path":[{"field":"note"}]}}`, "groups.0.opening.labelValue", "an opening row's label value reads params and values only"},
		"aggregate":            {`{"label":"Before {value}","labelValue":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}`, "groups.0.opening.labelValue", "params and values only"},
		"column":               {`{"label":"Before {value}","labelValue":{"col":"grams"}}`, "groups.0.opening.labelValue", "params and values only"},
		"yes or no":            {`{"label":"Before {value}","labelValue":{"value":true}}`, "groups.0.opening.labelValue", "an opening row's label value is a text, a day or a number"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, z.validate(t, gramsWithOpening(c.opening)), 400, c.at, c.want)
		})
	}
}

// The refusal of a column heading keeps its own words.
func TestRegressionGuardAHeadingRefusalNamesAHeading(t *testing.T) {
	z := newZoo(t)
	requireSpecError(t, z.validate(t, fmt.Sprintf(zooHeadings, `{"col":"cost"}`)), 400, "columns.3.heading", "a heading reads params and values only")
	requireSpecError(t, z.validate(t, fmt.Sprintf(zooHeadings, `{"value":true}`)), 400, "columns.3.heading", "a heading is a text, a day or a number")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'OpeningLabel' -count=1 2>&1 | command grep -m 10 .`
Expected: FAIL, a build failure that names `r.LabelTemplate undefined (type Row has no field or method LabelTemplate)`.

- [ ] **Step 3: Add the spec fields and the token (`spec.go`)**

Replace the `Opening` struct (keep its comment above it):

```go
type Opening struct {
	Label string `json:"label"`
	// LabelValue is a value of the run the label names, a formula of params
	// and values only (as Column.Heading): a text, a day or a number. Label
	// then holds LabelValueToken once, where the value goes. The row carries
	// the value typed (Row.LabelTemplate), so a view writes it in the reader's
	// form. A value that is blank on a run leaves the token out of the label.
	LabelValue *Node `json:"labelValue,omitempty"`
	// LabelBucket cuts a LabelValue that is a day to its day, month, quarter,
	// year or fiscalYear, as Group.Bucket cuts a group's key: the label then
	// names the period ("Jul 2026"), not the day.
	LabelBucket string           `json:"labelBucket,omitempty"`
	Cells       map[string]*Node `json:"cells,omitempty"`
}

// LabelValueToken stands for the label value in Opening.Label.
const LabelValueToken = "{value}"
```

- [ ] **Step 4: Add the binder rules (`bind.go`)**

In `(b *binder) opening`, replace

```go
	if n := characters(op.Label); n < 1 || n > maxLabel {
		b.bad(at+".label", fmt.Sprintf("an opening row has a label of 1 to %d characters", maxLabel))
	}
```

with

```go
	tokens := strings.Count(op.Label, LabelValueToken)
	switch n := characters(op.Label); {
	case n < 1 || n > maxLabel:
		b.bad(at+".label", fmt.Sprintf("an opening row has a label of 1 to %d characters", maxLabel))
	case op.LabelValue == nil && tokens > 0:
		b.bad(at+".label", fmt.Sprintf("%s in an opening row's label needs a labelValue", LabelValueToken))
	case op.LabelValue != nil && tokens != 1:
		b.bad(at+".label", fmt.Sprintf("an opening row with a labelValue has %s once in its label", LabelValueToken))
	case op.LabelValue != nil && strings.TrimSpace(strings.Replace(op.Label, LabelValueToken, "", 1)) == "":
		// A run with no value leaves the token out: the row still needs a name.
		b.bad(at+".label", fmt.Sprintf("an opening row's label has text besides %s", LabelValueToken))
	}
	switch {
	case op.LabelBucket == "":
	case op.LabelValue == nil:
		b.bad(at+".labelBucket", "a labelBucket needs a labelValue")
	case bucketType(op.LabelBucket) == "":
		b.bad(at+".labelBucket", "a labelBucket is day, month, quarter, year or fiscalYear")
	}
```

(`bind.go` already imports `strings` and `fmt`.) Replace the whole comment above the function (4 lines) with:

```go
// opening checks a group's opening row: a label of 1 to maxLabel characters,
// which holds LabelValueToken once exactly when the row has a label value; a
// label bucket only with a label value; and cells only in the report's columns
// after the first, whose cell the label fills, and not in a child row column.
// A cell's type and currency are checked where it compiles, on its level
// (level.openingCells).
```

In `formulaSizes`, replace

```go
		if o := p.spec.Groups[i].Opening; o != nil {
			for _, id := range o.cellIDs() {
```

with

```go
		if o := p.spec.Groups[i].Opening; o != nil {
			if n := o.LabelValue; n != nil {
				check(fmt.Sprintf("groups.%d.opening.labelValue", i), p.nodeSize(n, memo, map[string]bool{}, true))
			}
			for _, id := range o.cellIDs() {
```

- [ ] **Step 5: Let the formula walker visit the label value (`section_percent.go`)**

In `walkSpecFormulas`, replace

```go
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
```

with

```go
		if g.Opening != nil {
			walk(g.Opening.LabelValue, at+".opening.labelValue", false)
			for _, id := range g.Opening.cellIDs() {
```

- [ ] **Step 6: Compile and read the label value with the headings (`heading.go`)**

Replace the `headingCtx` type and its `only` method with:

```go
type headingCtx struct {
	c *compiler
	s *stmt
	// what names the value in a refusal: "a heading", or an opening row's
	// label value, which is one value for the whole run too.
	what string
}

func (h *headingCtx) only(at string) error {
	return invalid(at, h.what+" reads params and values only")
}
```

Replace everything from the comment `// headings compiles every column's heading; …` to the end of the file with:

```go
// headings compiles every column's heading; a heading on an unset param is
// left out, so the column shows its label.
func (c *compiler) headings(s *stmt) ([]headingPlan, error) {
	var out []headingPlan
	for _, cp := range c.p.cols {
		n := cp.col.Heading
		if n == nil {
			continue
		}
		at := cp.at + ".heading"
		e, err := runValue(&headingCtx{c: c, s: s, what: "a heading"}, n, at)
		if err != nil {
			return nil, err
		}
		if !e.unset {
			out = append(out, headingPlan{col: cp.col.ID, e: e})
		}
	}
	return out, nil
}

// runValue compiles a formula that is one value for the whole run. A date
// range or a list is no one value a view can show; an amount has no currency
// to print it in.
func runValue(h *headingCtx, n *Node, at string) (expr, error) {
	e, err := compileFormula(h, n, at)
	if err != nil {
		return expr{}, err
	}
	ok := !e.rng && !e.list
	switch e.typ {
	case Text, Date, DateTime, Number, "":
	default:
		ok = false
	}
	if !ok {
		return expr{}, invalid(at, h.what+" is a text, a day or a number")
	}
	return e, nil
}

// openingLabelPlan is one group level's compiled opening label value.
type openingLabelPlan struct {
	level int
	e     expr
	typ   FieldType
}

// openingLabels compiles every opening row's label value; one on an unset
// param is left out, so the row's label has no value.
func (c *compiler) openingLabels(s *stmt) ([]openingLabelPlan, error) {
	var out []openingLabelPlan
	for i, g := range c.p.groups {
		op := g.group.Opening
		if op == nil || op.LabelValue == nil {
			continue
		}
		h := &headingCtx{c: c, s: s, what: "an opening row's label value"}
		e, err := runValue(h, op.LabelValue, g.at+".opening.labelValue")
		if err != nil {
			return nil, err
		}
		typ := e.typ
		if op.LabelBucket != "" {
			// A date-time's day depends on the reader's zone; only a day has
			// one bucket for every reader.
			if e.typ != Date {
				return nil, invalid(g.at+".opening.labelBucket", "a labelBucket needs a labelValue that is a day")
			}
			e.sql, typ = bucketSQL(h, op.LabelBucket, e.sql), bucketType(op.LabelBucket)
		}
		if !e.unset {
			out = append(out, openingLabelPlan{level: i + 1, e: e, typ: typ})
		}
	}
	return out, nil
}

// shownRunValue is a run value as a view shows it, false for a blank one.
func shownRunValue(v any, typ FieldType) (any, FieldType, bool) {
	if v == nil {
		return nil, "", false
	}
	if typ == "" {
		typ = Text
	}
	// Whitespace-only counts as blank too: it draws as an empty header.
	if text, ok := v.(string); ok && typ == Text && strings.TrimSpace(text) == "" {
		return nil, "", false
	}
	if typ == Date {
		// A day reads as YYYY-MM-DD, as a date param's value does.
		if text, ok := v.(string); ok && len(text) >= 10 {
			v = text[:10]
		}
	}
	return v, typ, true
}

// readHeadings computes the run's headings and its opening rows' label
// values in one statement.
func (rn *runner) readHeadings() error {
	s := newStmt(rn.app, rn.c.info)
	plans, err := rn.c.headings(s)
	if err != nil {
		return err
	}
	labels, err := rn.c.openingLabels(s)
	if err != nil || len(plans)+len(labels) == 0 {
		return err
	}
	cells := make([]string, 0, len(plans)+len(labels))
	for i, h := range plans {
		cells = append(cells, fmt.Sprintf("%s AS h%d", h.e.sql, i))
	}
	for i, l := range labels {
		cells = append(cells, fmt.Sprintf("%s AS o%d", l.e.sql, i))
	}
	rows, err := rn.query(s, nil, "SELECT "+strings.Join(cells, ", "))
	if err != nil || len(rows) == 0 {
		return err
	}
	rn.headings = map[string]*ColumnHeading{}
	for i, h := range plans {
		if v, typ, ok := shownRunValue(rows[0][fmt.Sprintf("h%d", i)], h.e.typ); ok {
			rn.headings[h.col] = &ColumnHeading{Value: v, Type: typ}
		}
	}
	rn.openingLabels = map[int]*ColumnHeading{}
	for i, l := range labels {
		if v, typ, ok := shownRunValue(rows[0][fmt.Sprintf("o%d", i)], l.typ); ok {
			rn.openingLabels[l.level] = &ColumnHeading{Value: v, Type: typ}
		}
	}
	return nil
}
```

The heading rules do not change: `runValue` and `shownRunValue` hold the checks that `headings` and `readHeadings` had inline, so both kinds of value share them. The statement binds its values by name (`(s *stmt) bind` gives `{:rpN}`), so the order of the cells is free.

- [ ] **Step 7: Add the row field, the runner field and the dry compile (`run.go`)**

In `type Row struct`, after the `LabelCurrency` field, add:

```go
	// LabelTemplate is set on an opening row whose label names a value of the
	// run (Opening.LabelValue): the label with LabelValueToken where the value
	// goes. LabelType and LabelValue then hold the value, which a view
	// formats and puts in the token's place; Label has it as plain text.
	LabelTemplate string `json:"labelTemplate,omitempty"`
```

In `type runner struct`, after the `headings map[string]*ColumnHeading` field, add:

```go
	// openingLabels are the run's opening label values by group level
	// (readHeadings); a level whose value is blank on this run has none.
	openingLabels map[int]*ColumnHeading
```

In `(c *compiler) compileAll`, after

```go
	if _, err := c.headings(s); err != nil {
		return err
	}
```

add (so `Validate` refuses what a run refuses):

```go
	if _, err := c.openingLabels(s); err != nil {
		return err
	}
```

- [ ] **Step 8: Write the opening row's label (`assemble.go`)**

In `groupWithLabel`, replace

```go
		rn.push(Row{ID: "opening:" + keyString(key), Kind: KindOpening, Depth: depth + 1, Label: opening.Label, Section: id, GroupKey: key, Cells: rn.openingCells(row, opening)}, row)
```

with

```go
		r := Row{ID: "opening:" + keyString(key), Kind: KindOpening, Depth: depth + 1, Label: opening.Label, Section: id, GroupKey: key, Cells: rn.openingCells(row, opening)}
		rn.openingLabel(&r, n, opening)
		rn.push(r, row)
```

Add this method directly above the comment `// groupWithLabel emits a group row; …`:

```go
// openingLabel writes the label of an opening row that names a value of the
// run: the row carries the value typed with the label as its template, and
// Label holds it as plain text. With no value on this run the token leaves
// the label, with the space it stood in.
func (rn *runner) openingLabel(r *Row, n int, op *Opening) {
	if op.LabelValue == nil {
		return
	}
	v := rn.openingLabels[n]
	if v == nil {
		r.Label = strings.Join(strings.Fields(strings.Replace(op.Label, LabelValueToken, "", 1)), " ")
		return
	}
	r.LabelTemplate, r.LabelType, r.LabelValue = op.Label, v.Type, v.Value
	r.Label = strings.Replace(op.Label, LabelValueToken, valueString(v.Value), 1)
}
```

(`assemble.go` already imports `strings`. `(rn *runner) openGroups` assembles once before `readHeadings` and throws those rows away, so the nil map there is correct: a read of a nil map gives nil.)

- [ ] **Step 9: Fill the template in the exporter (`export_format.go`)**

Replace the comment and the first lines of `RowLabel`:

```go
// RowLabel is rowLabelText (format.ts): a typed label formatted, in its
// template's token when the row has one, a subtotal's with "Total " before
// it; else the row's label.
func (f *exportFormatter) RowLabel(row Row) string {
	if row.LabelType == "" || row.LabelValue == nil {
		return row.Label
	}
	text := f.typedLabelText(row.LabelType, row.LabelValue, row.LabelCurrency)
	if row.LabelTemplate != "" {
		return strings.Replace(row.LabelTemplate, LabelValueToken, text, 1)
	}
	if row.Kind == KindSubtotal {
		return "Total " + text
	}
	return text
}
```

- [ ] **Step 10: Run the new tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test ./reports/ -run 'OpeningLabel|HeadingRefusal' -count=1 2>&1 | command grep -m 20 .`
Expected: `ok  	tinycld.org/core/reports`.

- [ ] **Step 11: Write the docs (`docs/packages.md`)**

In the bullet that starts ``- A row's `kind` is `detail`, `group`, `opening`, …`` (lines 1496-1499), add after its last sentence (`… and its `section` is the group row.`):

```markdown
  When the level's `opening` has a `labelValue` that has a value on the run,
  the row also carries `labelTemplate` (the spec's label, with `{value}`),
  `labelType` and `labelValue` (a text, a day, a number, or a bucket key such
  as `"2026-11"` with `labelType` `month`). The view formats the value as it
  formats any typed label and puts it in place of `{value}` ("Balance before
  Nov 2026"); `label` holds the same text with the raw value. With no value on
  the run the row has none of the three, and `label` is the spec's label
  without `{value}`.
```

In the table row of `groups[].opening` (line 1561), change the first cell text `` `{ label, cells? }` `` to `` `{ label, labelValue?, labelBucket?, cells? }` `` and add at the end of the row's text, before the closing ` |`:

```markdown
 `labelValue` is a formula of params and values only (as `columns[].heading`: a text, a day or a number) that the label names: `label` then holds `{value}` exactly once, with other text. `labelBucket` (`day`, `month`, `quarter`, `year`, `fiscalYear`; only on a `labelValue` that is a day, not a date-time) cuts the day to its period, so `{ label: "Balance before {value}", labelValue: { param: "period", bound: "from" }, labelBucket: "month" }` reads "Balance before Nov 2026". A value that is blank on the run (an unset param) leaves `{value}` out: only the token and the spaces around it go, so write the label so that it reads well without it ("Balance before {value}", not "Balance ({value})"). Refused (400): `{value}` in a label with no `labelValue`, a `labelValue` whose label does not hold `{value}` exactly once, or holds no other text (`groups.<i>.opening.label`); a `labelBucket` with no `labelValue`, an unknown one, or one on a value that is not a day (`groups.<i>.opening.labelBucket`); a `labelValue` that reads a row, a column or a total, or is not a text, a day or a number (`groups.<i>.opening.labelValue`).
```

- [ ] **Step 12: Run the whole package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1 2>&1 | command grep -m 30 -E '^ok|FAIL|panic'`
Expected: one line, `ok  	tinycld.org/core/reports` (about 150 s). No `FAIL`.

- [ ] **Step 13: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/openinglabel_test.go core/server/reports/spec.go core/server/reports/bind.go core/server/reports/section_percent.go core/server/reports/heading.go core/server/reports/run.go core/server/reports/assemble.go core/server/reports/export_format.go docs/packages.md
git commit -m "feat(reports): an opening row's label names a value of the run"
```

---

### Task 2: The client reads the label value and shows it in the row label

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/types.ts` (`ResultRow`, near line 144)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/spec.ts` (near lines 114 and 202)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/format.ts` (imports, `rowLabelText` near line 244)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/fixtures/format-cases.json` (the list `"labels"`, line 1869)
- Test: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/__tests__/format.test.ts`, `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/__tests__/spec.test.ts`; `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/__tests__/format-cases.test.ts` (it reads the fixture; do not change it)

**Interfaces:**
- Consumes from task 1 (JSON only): a result row's `labelTemplate` with `labelType` and `labelValue`; the spec's `groups[].opening.labelValue` and `labelBucket`; the token `{value}`. Task 1's `RowLabel` (Go) reads the same fixture cases through `TestExportFormatterRowLabelsMatchTheClient`.
- Consumes (they exist, do not change them): `typedLabelText(type, value, currency, format)` (`format.ts`), `BUCKETS`, `specNodeSchema`, `GROUP_LABEL_TOKEN` (`spec.ts`); in the tests: `labelled`, `PLAIN` (`format.test.ts`), `FEEDINGS`, `parseSpec` (`spec.test.ts`).
- Produces for task 3: `export const LABEL_VALUE_TOKEN = '{value}'` (`spec.ts`); in `specGroupSchema`, `opening.labelValue?: SpecNode` and `opening.labelBucket?: Bucket` (so `SpecGroup['opening']` has them). Also `ResultRow.labelTemplate?: string`, and `rowLabelText(row, format)` fills the template.
- This task does not change `builder/draft.ts`: the builder drops the two fields on save until task 3.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/format.test.ts`, inside `describe('typed labels', …)`, directly before `it('shows a bucketed label with no value as blank', …)`, add:

```ts
    it('puts the typed value of an opening row in its label template', () => {
        const opening = labelled({
            kind: 'opening',
            label: 'Balance before 2026-11',
            labelTemplate: 'Balance before {value}',
            labelType: 'month',
            labelValue: '2026-11',
        })
        expect(rowLabelText(opening, PLAIN)).toBe('Balance before Nov 2026')
        const day = labelled({ ...opening, labelType: 'date', labelValue: '2026-11-01' })
        expect(rowLabelText(day, PLAIN)).toBe('Balance before Nov 1, 2026')
        const middle = labelled({ ...opening, labelTemplate: 'Before {value} (opening)' })
        expect(rowLabelText(middle, PLAIN)).toBe('Before Nov 2026 (opening)')
        // A text value goes in as it is: "$&" and "$1" are no patterns.
        const text = labelled({ ...opening, labelType: 'text', labelValue: 'the $& and $1 audit' })
        expect(rowLabelText(text, PLAIN)).toBe('Balance before the $& and $1 audit')
        // No value on this run: the engine wrote the label without the token.
        const none = labelled({ kind: 'opening', label: 'Balance before' })
        expect(rowLabelText(none, PLAIN)).toBe('Balance before')
    })

```

In `core/reports/lib/__tests__/spec.test.ts`, inside `describe('the parts of version 3', …)`, directly before `it('refuses a pick, a bound or an opening label the engine refuses', …)`, add:

```ts
    it('reads an opening row whose label names a value, and refuses a bucket it does not know', () => {
        const opening = {
            label: 'Balance before {value}',
            labelValue: { param: 'period', bound: 'from' },
            labelBucket: 'month',
        }
        const groups = (bucket: string) => [
            { id: 'g', col: 'species', opening: { ...opening, labelBucket: bucket } },
        ]
        const read = parseSpec({ ...FEEDINGS, version: 4, groups: groups('month') })
        expect(read?.groups?.[0]?.opening).toEqual(opening)
        expect(parseSpec({ ...FEEDINGS, version: 4, groups: groups('week') })).toBeNull()
    })

```

In `core/reports/lib/fixtures/format-cases.json`, in the list `"labels"` (line 1869), add these two cases before the first case (`"name": "a text label"`). The Go test `TestExportFormatterRowLabelsMatchTheClient` and `core/reports/lib/__tests__/format-cases.test.ts` both read this list, so the client and the exporter are pinned on the same rows:

```json
        {
            "name": "an opening row that names a month in its label",
            "row": {
                "id": "r-opening",
                "kind": "opening",
                "depth": 1,
                "label": "Balance before 2026-11",
                "labelTemplate": "Balance before {value}",
                "labelType": "month",
                "labelValue": "2026-11",
                "cells": {}
            },
            "display": {
                "divideBy1000": false,
                "withoutCents": false,
                "negative": "minus",
                "negativeRed": false,
                "hidden": []
            },
            "text": "Balance before Nov 2026"
        },
        {
            "name": "an opening row that names a text with pattern signs",
            "row": {
                "id": "r-opening-text",
                "kind": "opening",
                "depth": 1,
                "label": "Balance before the $& and $1 audit",
                "labelTemplate": "Balance before {value}",
                "labelType": "text",
                "labelValue": "the $& and $1 audit",
                "cells": {}
            },
            "display": {
                "divideBy1000": false,
                "withoutCents": false,
                "negative": "minus",
                "negativeRed": false,
                "hidden": []
            },
            "text": "Balance before the $& and $1 audit"
        },
```

(Each case becomes one test of `format-cases.test.ts`, named `label: <name>`. The Go side already passes these two cases after task 1; step 6 runs it.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format-cases.test.ts 2>&1 | command grep -m 20 -E 'FAIL|Tests |AssertionError'`
Expected: `Tests  4 failed | 213 passed (217)`. The four are the two added tests and the two fixture cases: the format test and the month case read `'Nov 2026'` where they want `'Balance before Nov 2026'`; the text case reads `'the $& and $1 audit'`; the spec test gets `undefined`, because the strict schema refuses `labelValue`.

- [ ] **Step 3: Add the row field (`types.ts`)**

In `interface ResultRow`, after `labelCurrency?: string`, add:

```ts
    /**
     * Set on an opening row whose label names a value of the run (spec
     * `opening.labelValue`): the label with `{value}` where the value goes.
     * `labelType` and `labelValue` hold the value; `label` has it as plain text.
     */
    labelTemplate?: string
```

- [ ] **Step 4: Add the spec fields and the token (`spec.ts`)**

After the line `export const GROUP_LABEL_TOKEN = '{group}'`, add:

```ts
/** Stands for the label value in `groups[].opening.label` and a row's `labelTemplate` (spec.go LabelValueToken). */
export const LABEL_VALUE_TOKEN = '{value}'
```

In `specGroupSchema`, in the `opening` object, after the `label` line, add:

```ts
            // A formula over params and values the label names, where it holds
            // `{value}`; `labelBucket` cuts a day to its month, quarter or year.
            labelValue: specNodeSchema.optional(),
            labelBucket: z.enum(BUCKETS).optional(),
```

(Keep the `label` line as it is in the file.)

- [ ] **Step 5: Fill the template in `rowLabelText` (`format.ts`)**

After the import of `./money`, add:

```ts
import { LABEL_VALUE_TOKEN } from './spec'
```

(`spec.ts` imports only `./types`, so there is no import cycle.)

Replace the doc comment and the first three lines of the body of `rowLabelText`, so that the function reads:

```ts
/**
 * A row's label as the table shows it. The engine types a label that is not text
 * (an amount, a date, a yes/no) and writes `label` as its plain text; a subtotal's
 * label is "Total " and its group's label. An opening row that names a value of the
 * run has the typed value in its template's token.
 */
export function rowLabelText(row: ResultRow, format: NumberFormat): string {
    const { labelType, labelValue } = row
    if (!labelType || labelValue === undefined || labelValue === null) return row.label
    const text = typedLabelText(labelType, labelValue, row.labelCurrency ?? '', format)
    // A function, so a "$&" or "$1" in a text value is not read as a pattern.
    if (row.labelTemplate) return row.labelTemplate.replace(LABEL_VALUE_TOKEN, () => text)
    return row.kind === 'subtotal' ? `Total ${text}` : text
}
```

- [ ] **Step 6: Run the tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format-cases.test.ts core/reports/lib/builder/__tests__/draft.test.ts 2>&1 | command grep -m 20 -E 'FAIL|Tests |Test Files|AssertionError'`
Expected: `Test Files  4 passed (4)` and `Tests  338 passed (338)` (the builder's tests stay green: `draft.ts` reads a spec with the two new keys and leaves them out).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'RowLabelsMatchTheClient' -count=1`
Expected: `ok  	tinycld.org/core/reports` (the exporter of task 1 writes the two fixture labels as the client does).

- [ ] **Step 7: Lint and typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports/lib 2>&1 | command grep -m 10 . ; PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck 2>&1 | command grep -m 20 -E 'error TS|exit'`
Expected: biome prints `Checked … files … No fixes applied.` and no error; the typecheck prints no `error TS` line. When biome reports a format error in a file of this task, run `biome check --write <that file>` and run the check again.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/types.ts core/reports/lib/spec.ts core/reports/lib/format.ts core/reports/lib/fixtures/format-cases.json core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/spec.test.ts
git commit -m "feat(reports): the client shows an opening row's label value"
```

---

### Task 3: The builder keeps the label value and checks the label

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/builder/draft.ts` (imports; near lines 104, 775, 1009, 1334, 1543, 1606, 1611)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/help/reports.md` (line 144)
- Test: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/reports/lib/builder/__tests__/draft.test.ts`
- Test: `/Users/nas/tmp/tinycld/books-purchases/tinycld/tests/e2e/reports-ledger-v3.spec.ts` (the last test, lines 564-566)

**Interfaces:**
- Consumes from task 2: `LABEL_VALUE_TOKEN` (`'{value}'`, exported by `core/reports/lib/spec.ts`); `opening.labelValue?: SpecNode` and `opening.labelBucket?: Bucket` in `specGroupSchema` (so `SpecGroup['opening']` has them).
- Consumes (they exist, do not change them): `BUCKETS`, `GROUP_LABEL_TOKEN` (`spec.ts`); `labelSchema`, `formulaDraftSchema`, `formulaDraft(node, params)`, `specFormula(formula, ctx)`, `specCells(cells, ctx)`, `formulaContext(draft, catalog, { kind: 'heading' })`, `keptParams`, `draftIssues`, `openingIssues` (`draft.ts`); in the unit tests: `V3`, `META`, `CATALOG`, `OPEN_BALANCE`, `draftFromSpec`, `specFromDraft`, `builderSchema`, `emptyGroup`, and the types `SpecNode`, `ReportSpec`, `FormulaDraft` (`draft.test.ts` imports them all today).
- Consumes on the screen (no component changes): the builder form validates on each change (`mode: 'onChange'`, `core/reports/hooks/useReportBuilder.ts`); `builderSchema` gives each issue of `draftIssues` to the form at the issue's path (`draft.ts`, `superRefine`); the **Row label** input is the form field `groups.<i>.opening.label` (`OpeningFields`, `core/reports/components/builder/GroupLevelParts.tsx`), and the form `TextInput` (`core/ui/form/TextInput.tsx`) draws the field's error message as plain text under the input.
- Produces: `OpeningDraft` with `labelValue?: FormulaDraft` and `labelBucket?: Bucket`; a draft issue at `['groups', i, 'opening', 'label']` with one of the three messages of step 4.

- [ ] **Step 1: Write the failing unit tests**

In `core/reports/lib/builder/__tests__/draft.test.ts`, inside `describe('spec version 3 in the builder', …)`, directly before `it('"A formula" on group and total rows writes the column rollup, and the total takes it out', …)`, add:

```ts
    it('keeps an opening label value and its bucket through the draft, and the param it reads', () => {
        const labelValue: SpecNode = { param: 'period', bound: 'from' }
        const opening = {
            label: 'Balance before {value}',
            labelValue,
            labelBucket: 'month' as const,
            cells: { balance: OPEN_BALANCE },
        }
        const spec: ReportSpec = {
            ...V3,
            groups: [{ ...(V3.groups?.[0] ?? { id: 'byTeam', col: 'team' }), opening }],
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(draft.groups[0]?.opening?.labelBucket).toBe('month')
        expect(draft.groups[0]?.opening?.labelValue?.node).toEqual(opening.labelValue)
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(spec))
        expect(draftIssues(draft, CATALOG).filter(issue => issue.path[2] === 'opening')).toEqual([])
        // Nothing but the label value reads the param: it stays.
        const onlyLabel = {
            ...draft,
            filter: emptyGroup(),
            columns: draft.columns
                .map(c => ({ ...c, rollup: null }))
                .filter(c => c.id !== 'balance'),
            groups: draft.groups.map(g => ({
                ...g,
                opening: g.opening ? { ...g.opening, cells: {} } : null,
            })),
        }
        expect(keptParams(onlyLabel).map(p => p.id)).toEqual(['period'])
    })

    it('flags an opening label whose {value} does not fit its label value', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        const from: FormulaDraft = { kind: 'fixed', node: { param: 'period', bound: 'from' } }
        const issueOf = (label: string, labelValue?: FormulaDraft) => {
            const opening = { label, cells: {}, ...(labelValue ? { labelValue } : {}) }
            const labelled = { ...draft, groups: draft.groups.map(g => ({ ...g, opening })) }
            return draftIssues(labelled, CATALOG).find(
                issue => issue.path.join('.') === 'groups.0.opening.label'
            )?.message
        }
        expect(issueOf('Balance before {value}', from)).toBeUndefined()
        expect(issueOf('Balance before')).toBeUndefined()
        expect(issueOf('Balance before', from)).toBe(
            'Write {value} once, where the row’s value goes'
        )
        expect(issueOf('{value} to {value}', from)).toBe(
            'Write {value} once, where the row’s value goes'
        )
        expect(issueOf(' {value} ', from)).toBe('Write some text besides {value}')
        expect(issueOf('Balance before {value}')).toBe(
            'This row has no value to name: take {value} out of the label'
        )
    })

```

(The quote in `row’s` is the typographic apostrophe U+2019, as in the message of step 4.)

- [ ] **Step 2: Write the failing e2e step**

In `tests/e2e/reports-ledger-v3.spec.ts`, in the last test (`'the builder counts a lookup on this row, shows a formula on group and total rows, and starts each group with a row'`), replace

```ts
        await p.getByTestId(o).click()
        await p.getByTestId(`${o}-label`).fill('Brought forward')
```

with

```ts
        await p.getByTestId(o).click()
        // This report gives the row no value to name, so the label takes no {value}.
        const noValue = p.getByText(
            'This row has no value to name: take {value} out of the label',
            { exact: true }
        )
        await p.getByTestId(`${o}-label`).fill('Brought {value}')
        await expect(noValue).toBeVisible()
        await p.getByTestId(`${o}-label`).fill('Brought forward')
        await expect(noValue).toBeHidden()
```

Change nothing else in the test: the report is still saved with the label `Brought forward`, and the lines after it read that label.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/reports/lib/builder/__tests__/draft.test.ts 2>&1 | command grep -m 20 -E 'FAIL|Tests |AssertionError'`
Expected: `Tests  2 failed | 121 passed (123)`: the first added test with `expected undefined to be 'month'`, the second with `expected undefined to be 'Write {value} once, where the row’s v…'`.

Run (one foreground command, about 5 minutes: it builds the web export and seeds the fixtures; when the lane rules give you no time for it, hand back `NEEDS-RUN:` with this command):

```bash
cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e tinycld/tests/e2e/reports-ledger-v3.spec.ts -- -g "the builder counts a lookup"
```

Expected: `Running 1 test`, then 1 failed: `expect(locator).toBeVisible()` on `getByText('This row has no value to name: take {value} out of the label', { exact: true })` finds no element, at the line `await expect(noValue).toBeVisible()`. (The planner did not run e2e. When the run fails at another line, or does not fail, stop and report what it printed; do not change the test to make it fail.)

- [ ] **Step 4: Keep the fields in the builder (`builder/draft.ts`)**

In the import from `'../spec'`, add `LABEL_VALUE_TOKEN,` after `GROUP_LABEL_TOKEN,`.

Replace `openingDraftSchema`:

```ts
export const openingDraftSchema = z.object({
    label: labelSchema,
    cells: z.record(z.string(), formulaDraftSchema),
    // The value the label names at `{value}`, and the period a day is cut to. The
    // builder has no control for them: it keeps them as the spec gave them.
    labelValue: formulaDraftSchema.optional(),
    labelBucket: z.enum(BUCKETS).optional(),
})
```

After the line `const OPENING_NESTED = 'A child row column has no opening cell'`, add:

```ts
const OPENING_TOKEN_ONCE = `Write ${LABEL_VALUE_TOKEN} once, where the row’s value goes`
const OPENING_TOKEN_ONLY = `Write some text besides ${LABEL_VALUE_TOKEN}`
const OPENING_TOKEN_PLAIN = `This row has no value to name: take ${LABEL_VALUE_TOKEN} out of the label`
```

Directly above the comment `// bind.go groupsAndPivot: an opening row shows under a level that opens, …` (above `function openingIssues`), add:

```ts
// bind.go opening: a label that names a value holds the token once, with other
// text (a run with no value shows the label without it); any other label has none.
function openingLabelIssue(opening: OpeningDraft): string | undefined {
    const parts = opening.label.split(LABEL_VALUE_TOKEN)
    if (!opening.labelValue) {
        return parts.length > 1 ? OPENING_TOKEN_PLAIN : undefined
    }
    if (parts.length !== 2) return OPENING_TOKEN_ONCE
    return parts.join('').trim() === '' ? OPENING_TOKEN_ONLY : undefined
}

```

In `openingIssues`, replace

```ts
        if (draft.pivot.on && i === draft.groups.length - 1)
            return [{ path: at, message: OPENING_PIVOT }]
        return Object.keys(group.opening.cells).flatMap((col): Issue[] => {
```

with

```ts
        if (draft.pivot.on && i === draft.groups.length - 1)
            return [{ path: at, message: OPENING_PIVOT }]
        const label = openingLabelIssue(group.opening)
        if (label) return [{ path: [...at, 'label'], message: label }]
        return Object.keys(group.opening.cells).flatMap((col): Issue[] => {
```

In `openingFromSpec`, replace `return { label: opening.label, cells: Object.fromEntries(cells) }` with:

```ts
    return {
        label: opening.label,
        cells: Object.fromEntries(cells),
        ...(opening.labelValue ? { labelValue: formulaDraft(opening.labelValue, params) } : {}),
        ...(opening.labelBucket ? { labelBucket: opening.labelBucket } : {}),
    }
```

In `keptParams`, after the line `for (const cell of Object.values(group.opening?.cells ?? {})) node(cell.node)`, add:

```ts
        node(group.opening?.labelValue?.node)
```

Replace `specOpening`:

```ts
// The keys are written in the order spec.go Opening holds them.
function specOpening(
    opening: OpeningDraft,
    ctx: ReportFormulaContext,
    valueCtx: ReportFormulaContext
) {
    const { label, labelValue, labelBucket, cells } = opening
    return {
        label,
        ...(labelValue ? { labelValue: specFormula(labelValue, valueCtx) } : {}),
        ...(labelBucket ? { labelBucket } : {}),
        ...(Object.keys(cells).length > 0 ? { cells: specCells(cells, ctx) } : {}),
    }
}
```

In `specGroups`, replace its first two lines

```ts
function specGroups(draft: BuilderDraft, catalog: Catalog): SpecGroup[] {
    return draft.groups.map((g, index) => {
```

with

```ts
function specGroups(draft: BuilderDraft, catalog: Catalog): SpecGroup[] {
    // An opening row's label value is one value per run, as a heading is.
    const valueCtx = formulaContext(draft, catalog, { kind: 'heading' })
    return draft.groups.map((g, index) => {
```

and in the same function replace `...(g.opening ? { opening: specOpening(g.opening, ctx) } : {}),` with `...(g.opening ? { opening: specOpening(g.opening, ctx, valueCtx) } : {}),`.

The builder's **Row label** field writes `groups.<i>.opening.label` only (`GroupLevelParts.tsx`), so an edit of the label keeps `labelValue` and `labelBucket`. No component changes.

- [ ] **Step 5: Run the unit tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/reports/lib/__tests__/format.test.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format-cases.test.ts core/reports/lib/builder/__tests__/draft.test.ts 2>&1 | command grep -m 20 -E 'FAIL|Tests |Test Files|AssertionError'`
Expected: `Test Files  4 passed (4)` and `Tests  340 passed (340)`.

- [ ] **Step 6: Write the help text (`core/help/reports.md`)**

In the section `## Opening rows`, add at the end of its paragraph (line 144, after `The last group level of a pivot report cannot have an opening row.`), on the same line:

```markdown
 A report that an app supplies can name a value in the row label, for example the month before the period ("Balance before Nov 2026"). The **Row label** then shows `{value}` where the value goes. Keep `{value}` in the label one time, with other text. When the report has no value to show, the label shows without `{value}`, so write a label that reads well without it. You cannot add `{value}` to the row label of a report that has no such value.
```

- [ ] **Step 7: Lint and typecheck**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check core/reports/lib tests/e2e/reports-ledger-v3.spec.ts 2>&1 | command grep -m 10 . ; PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck 2>&1 | command grep -m 20 -E 'error TS|exit'`
Expected: biome prints `Checked … files … No fixes applied.` and no error; the typecheck prints no `error TS` line. When biome reports a format error in a file of this task, run `biome check --write <that file>` and run the check again.

- [ ] **Step 8: Run the e2e test**

Run the e2e command of step 3 again (the same one command, the same port check).
Expected: `Running 1 test` and `1 passed`. The message shows under **Row label** after `Brought {value}` and goes away after `Brought forward`; the rest of the test reads the saved report as before.

When the message does not show, find the cause in the form path named in **Interfaces** (the draft issue's path must be `groups.0.opening.label`, the name of the input). Do not add a timeout, a wait or a retry.

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports/lib/builder/draft.ts core/reports/lib/builder/__tests__/draft.test.ts tests/e2e/reports-ledger-v3.spec.ts core/help/reports.md
git commit -m "feat(reports): the builder keeps an opening row's label value and checks its label"
```
