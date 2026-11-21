# Report Engine Query Scaling Implementation Plan — part 13 (task 9b: a bound on a union source is written inside each part)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A report that reads a union source (a source with `Parts`) through a filter or a lookup reads each part through an index of the part's own table, so that Transaction Detail with an Accounts filter is no longer refused at 100,000 journal lines.

**Architecture:** SQLite moves no condition on the rows of a `UNION ALL` into its parts, so today a bound on a union reads every row of every part (for a lookup: once for each row of the report). The engine already finds the bounds of a filter (`filterBounds`) and of a lookup's where (`expr.bound`); for a union source it now keeps them on the scope (`scope.partBounds`), written on two alias columns (`__partlink`, `__partday`), and `scope.boundedFrom` writes the union again in the scope's FROM, each part read from its own collection's rule CTE with the bounds it can hold on its own columns. The rows are the same rows: a bound only leaves out rows the filter or the where refuses, and a part that cannot hold a bound reads its rows without it. The switch `legacySQL.noPartBounds` gives the old statement for the same-result tests.

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The human's decision (2026-11-19 and 2026-11-20): the Trial Balance, the Balance Sheet and Transaction Detail are fixed in this unit; at 500,000 journal lines a run filtered to one month takes 1 s or less and a filtered year run 2 s or less. The measure and its 100k baseline: `books/docs/plans/general-ledger-ledger.md`, section "Trial Balance, Balance Sheet and Transaction Detail: the measure and its baseline" (plan part 12). Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`, at ff0d61b4 when this part was written); Books is `books/` (same branch name, d9a695a7). Task 9b is a core task: steps 1 to 12 edit only `tinycld/core/server/reports`. Steps 14 and 15 edit one Books document.

**Built before it was written:** the planner built every code block of this part in a scratch folder and ran it through `go test -overlay` on the tree above: the red run of step 4, the green run of step 10 and the whole package of step 11 gave the results the steps name. The scratch copies are in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part13-scratch/core/` (the six `.go` files and the three test files, as they are after step 9); an implementer may compare with them, and writes the tree from this plan.

**What this part does not fix:** Transaction Detail with no Accounts filter (runs `TD 1` and `TD 2`). With this change its lookups search by entry, but its base rows are bounded by the day alone, and no index of `books_journal_lines` or `books_cash_basis_lines` starts with the day: the plan still shows `SCAN books_journal_lines` and `SCAN books_cash_basis_lines`, and `SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)`. That case is a later task of this unit (9b-3 in the outline); it is not planned here.

## Global Constraints

- Core names no package: every core test, comment and message of this task names the zoo only (`zoo.meals`, `zoo_feedings`, `zoo_treats`, `zoo.documents`). No Books name, slug, collection or route appears under `tinycld/`.
- The rows of a report do not change. Each new SQL form has the switch `legacySQL.noPartBounds`, and `requireSameAsLegacy` compares the result with the switch on and off.
- The bar is the human's and is never lowered: at 500,000 lines `glMonthBar` is 1 s and `glYearBar` is 2 s (median of 5). No task changes a bar, removes a run, or changes the scale book to pass.
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the 1-minute load (`sysctl -n vm.loadavg`) is under 4. An implementer starts no timing run: it is a `NEEDS-RUN` for the driver.
- Go code is formatted with `gofmt`. Comments say why, not what.
- The record of each measure goes in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)".

## Review Focus

1. A part that does not store a key of a bound (no column mapped, a column of another type, a multi-select relation, a view, an unknown collection): it reads its rows without that bound and the other parts keep theirs; a bound never names a column that does not exist. Test: `TestAPartHoldsOnlyTheBoundsItStores` (step 2).
2. A reader whose list rules show one part only, or no part: the bounded union reads each part from its own rule CTE, so the reader gets the same rows as before. Test: `TestPartBoundsKeepTheResult` runs ann (both parts), bob, dan (treats only) and eve (nothing).
3. An `eq` under `or` or `not` holds for rows outside its range: it must bound nothing. Test: `TestAnEqUnderOrOrNotBoundsNoPart`.
4. A period of all dates (no day bound) and a lookup whose where matches a link and a day at once: the bound is one text, so a part that cannot hold one of its keys holds none of it. Tests: `TestPartBoundsKeepTheResult` (params `nil`), `TestAPartHoldsOnlyTheBoundsItStores` (the case "one bound of two keys, one not stored").
5. The realtime topics of a run: a statement that reads a union through its parts must still name each part's collection. Test: `TestPartBoundsKeepTheTopics`.

For the reviewer of the code, not pinned by a test (found while this part was built):
- The union's rule CTE (`r<n> AS (… UNION ALL …)`) stays in the WITH clause when every read of the union is bounded. It is then unread (SQLite does not run it), but its text and params are still sent. Not measured.
- A lookup's link bound on a union is a range only; the equality form of `linkedEq` (`knownLinks`), which lets an index use a day column after the link, is not written for a union.
- On a small table SQLite may pick another index when a part holds two bounds. That is why each zoo test gives each part ONE index (`withMealIndexes(t, z, "link")` or `"day"`); do not add a second index to make a test "more real".

---

### Task 9b: A bound on a union source is written inside each part (core)

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/partbounds_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (the end of `legacySQL`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/stmt.go` (`scope`, lines 280-283; `scope.render`, lines 327-334)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rules.go` (`unionSelect`, lines 124-161)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/formula.go` (`storedLink`, lines 87-91; the plain return of `case "eq", "ne":`, line 874)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookup.go` (`lookupIn`, lines 160-171; `lookupCtx.path`, lines 501-505; `storedDayCol`, lines 591-598)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/bounds.go` (`filterBounds`, `listBound`, `dayBound`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookupbounds_test.go` (lines 190-219, `TestALookupOnAUnionGetsNoDayBound`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/bounds_test.go` (lines 275-276, `TestFilterBoundsLeaveAUnionAlone`)
- Modify (step 14): `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (the end of the file)

**Interfaces:**
- Consumes (existing core code, package `reports`): `q(alias, column string) string` (`stmt.go:253`, gives `[[alias.column]]`); `(s *stmt) rule(collection string) string` (the name `r<n>` of a collection's rule CTE); `(src *Source) unionKeys() []string`; `SourcePart{Collection string; Columns map[string]string; Values map[string]any}`; `expr.bound string` (a bound an `and` joins and passes up); `readsLookedUpRow`, `containsLookup`, `readsLookupColumn` (`lookup.go`); `linkBound`, `dayBounds` (`bounds.go`). Test helpers: `newUnionZoo(t)` (`union_zoo_test.go`), `observeReads(t) *[]readStatement`, `readPool`, `rawStatement` (`statement_test.go`), `queryPlan` (`balance_shape_test.go`), `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)` (`aggonce_test.go`), `requireRowsWhere` (`bounds_test.go`), `addZooSales`, `registerZooSalesSources` (`union_shape_test.go`).
- Produces (for the later tasks of this unit):
  - `legacySQL.noPartBounds bool`
  - consts `partLink = "__partlink"`, `partDay = "__partday"`
  - `scope.partBounds []string`
  - `func (sc *scope) boundedFrom() string`
  - `func partWhere(app core.App, part SourcePart, bounds []string) string`
  - `func unionPartCols(src *Source, i int, keys []string, params dbx.Params) string`
  - `storedLink.part string`
  - `func (l *lookupCtx) unionParts() bool`
  - `func partLinkBound(c fctx, n *Node, args []expr) string`
  - test helpers `withMealIndexes(t *testing.T, z *zoo, kind string)` (kind `"link"` or `"day"`), `unionPlans(t *testing.T, z *zoo, seen []readStatement) string`, the consts `mealsLookupSpec`, `mealsFilterSpec`, the var `mealDays`
  - the output files of the 100k run: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b/st-results.md` and `st-plans.md`

Facts the code depends on (checked on this tree):
- A bound travels up only through `and` (`formula.go`, the existing `expr.bound` path): an `eq` under `or` or `not` gives no bound.
- `stmt.topics` finds the collections a statement reads with the regexp `ruleRef` (`(?:FROM|JOIN) (r\d+) `). The bounded union reads each part as `FROM r<n> p`, so each part's collection is found.
- `lookupIn` puts the bound on the scope AFTER `whereUnknown` is built: the test for an unknown where reads the whole union, as it reads a collection without the day bound.
- The zoo's union `zoo.meals` has the parts `zoo_feedings` (columns `animal`, `fed`) and `zoo_treats` (columns `pet`, `given`); `zoo.documents` has the parts `zoo_orders` and `zoo_tickets`, each with a text column `day`.

- [ ] **Step 1: Add the switch, the two aliases and an empty `partWhere`**

The tests of steps 2 and 3 name these; with them in the tree the tests compile and fail for their behaviour.

In `legacy.go`, add at the end of the `legacySQL` struct, after the field `cellsBeforeThePage bool`:

```go
	// noPartBounds reads a union source through its rule CTE with no bound
	// inside its parts, as before partBounds wrote them.
	noPartBounds bool
```

In `stmt.go`, add after the function `func (sc *scope) render() string { … }` (it ends at line 334):

```go
// partLink and partDay are the aliases a part bound names a union source's
// key under before the part is known: a to-one link's column, and a Date
// field's. partWhere puts each part's own column in their place.
const (
	partLink = "__partlink"
	partDay  = "__partday"
)

// partWhere is the bounds one part of a union holds, joined with AND.
func partWhere(app core.App, part SourcePart, bounds []string) string {
	return ""
}
```

- [ ] **Step 2: Write the new test file**

Create `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/partbounds_test.go`:

```go
package reports

import (
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// withMealIndexes gives each part of zoo.meals one index a bound inside the
// part reads through: on its animal link (kind "link") or on its day (kind
// "day"). One index a part, so the plan does not depend on which of two
// SQLite's planner prefers on a small table.
func withMealIndexes(t *testing.T, z *zoo, kind string) {
	t.Helper()
	columns := map[string]map[string]string{
		"link": {"zoo_feedings": "animal", "zoo_treats": "pet"},
		"day":  {"zoo_feedings": "fed", "zoo_treats": "given"},
	}
	for name, column := range columns[kind] {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		col.AddIndex("idx_"+name+"_"+kind, false, column, "")
		z.save(t, col)
	}
}

// unionPlans is the query plan of every statement a run read, one text.
func unionPlans(t *testing.T, z *zoo, seen []readStatement) string {
	t.Helper()
	db := readPool(t, z)
	var plans []string
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		plans = append(plans, queryPlan(t, db, raw, args)...)
	}
	return strings.Join(plans, "\n")
}

const mealsLookupSpec = `{"version":4,"source":"zoo.pets","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"pet","path":[{"field":"id"}]},
	{"id":"meals","formula":{"op":"lookup","source":"zoo.meals","pick":"sum",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
			{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]}]},
		"value":{"path":[{"field":"cost"}]}}}],
	"sort":[{"col":"name"}]}`

const mealsFilterSpec = `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[{"id":"type","path":[{"field":"kind"}]},{"id":"cost","path":[{"field":"cost"}]}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},"sort":[{"col":"type"}]}`

var mealDays = map[string]string{"days": `{"from":"2026-01-01","to":"2026-02-28"}`}

// A lookup over a union that matches a link reads each part through the index
// on the part's own link column, not every row of the union for each row of
// the report.
func TestALookupOverAUnionSearchesEachPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	seen := observeReads(t)
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	plan := unionPlans(t, z, *seen)
	for _, want := range []string{
		"SEARCH zoo_feedings USING INDEX idx_zoo_feedings_link (animal>? AND animal<?)",
		"SEARCH zoo_treats USING INDEX idx_zoo_treats_link (pet>? AND pet<?)",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("the plan has no %q:\n%s", want, plan)
		}
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, partLink) || strings.Contains(st.sql, partDay) {
			t.Fatalf("a statement keeps a part alias:\n%s", st.sql)
		}
	}
	legacySQL.noPartBounds = true
	t.Cleanup(func() { legacySQL.noPartBounds = false })
	*seen = nil
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	if plan := unionPlans(t, z, *seen); strings.Contains(plan, "idx_zoo_feedings_link") {
		t.Fatalf("with the switch on, the plan still reads a part by its link:\n%s", plan)
	}
}

// A filter on a union's day reads each part through the index on the part's
// own day column: zoo_feedings.fed and zoo_treats.given.
func TestAFilterOnAUnionSearchesEachPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "day")
	seen := observeReads(t)
	z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	plan := unionPlans(t, z, *seen)
	for _, want := range []string{
		"SEARCH zoo_feedings USING INDEX idx_zoo_feedings_day (fed>? AND fed<?)",
		"SEARCH zoo_treats USING INDEX idx_zoo_treats_day (given>? AND given<?)",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("the plan has no %q:\n%s", want, plan)
		}
	}
	// w's WHERE is still the filter alone: the bounds are inside the parts.
	requireRowsWhere(t, *seen, true, "(substr(")
}

// Every reader gets the same rows with the part bounds and without them: a
// reader of both parts, of one part, and of none; with a period and with all
// dates (no day bound).
func TestPartBoundsKeepTheResult(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, params := range []map[string]string{mealDays, nil} {
			requireSameAsLegacy(t, z, user, mealsLookupSpec, Options{}, params, &legacySQL.noPartBounds)
			requireSameAsLegacy(t, z, user, mealsFilterSpec, Options{}, params, &legacySQL.noPartBounds)
		}
	}
}

// A part holds a bound only when its collection stores every key the bound
// names: a link as a single relation, a day as text or a date. A part with no
// column for a key, or a column of another type (a multi-select relation
// holds a JSON array, never an id), reads its rows without that bound; a part
// that is a view (its column may hold NULL) or an unknown collection reads
// its rows without any.
func TestAPartHoldsOnlyTheBoundsItStores(t *testing.T) {
	z := newUnionZoo(t)
	signedIn := rule(`@request.auth.id != ""`)
	boxes := core.NewBaseCollection("zoo_boxes")
	boxes.ListRule, boxes.ViewRule = signedIn, signedIn
	boxes.Fields.Add(
		&core.RelationField{Name: "animals", CollectionId: z.recs["leo"].Collection().Id, MaxSelect: 5},
		&core.DateField{Name: "packed"},
	)
	z.save(t, boxes)
	view := core.NewViewCollection("zoo_treat_days")
	view.ListRule, view.ViewRule = signedIn, signedIn
	view.ViewQuery = `SELECT t.id, t.pet, t.given FROM zoo_treats t`
	z.save(t, view)
	link := q(partLink, "animal") + " >= 'a'"
	day := q(partDay, "fed") + " >= 'b'"
	part := func(collection string, columns map[string]string) SourcePart {
		return SourcePart{Collection: collection, Columns: columns}
	}
	for _, c := range []struct {
		name   string
		part   SourcePart
		bounds []string
		want   string
	}{
		{"both keys stored", part("zoo_treats", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, "[[p.pet]] >= 'a' AND [[p.given]] >= 'b'"},
		{"no column for the day", part("zoo_treats", map[string]string{"animal": "pet"}), []string{link, day}, "[[p.pet]] >= 'a'"},
		{"a day in a number column", part("zoo_treats", map[string]string{"animal": "pet", "fed": "price"}), []string{link, day}, "[[p.pet]] >= 'a'"},
		{"a link in a text column", part("zoo_treats", map[string]string{"animal": "label", "fed": "given"}), []string{link, day}, "[[p.given]] >= 'b'"},
		{"a link in a multi-select relation", part("zoo_boxes", map[string]string{"animal": "animals", "fed": "packed"}), []string{link, day}, "[[p.packed]] >= 'b'"},
		{"a view", part("zoo_treat_days", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, ""},
		{"one bound of two keys, one not stored", part("zoo_treats", map[string]string{"animal": "pet"}), []string{link + " AND " + day}, ""},
		{"an unknown collection", part("zoo_nothing", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, ""},
	} {
		if got := partWhere(z.app, c.part, c.bounds); got != c.want {
			t.Errorf("%s: the part holds %q, want %q", c.name, got, c.want)
		}
	}
}

// An eq under or, or under not, holds for rows outside its range: it bounds
// no part, and the rows are the rows with no bound.
func TestAnEqUnderOrOrNotBoundsNoPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	eq := `{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]}`
	days := `{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]}`
	partBound := regexp.MustCompile(`FROM r\d+ p WHERE `)
	for name, where := range map[string]string{
		"or":  `{"op":"or","args":[` + eq + `,` + days + `]}`,
		"not": `{"op":"not","args":[` + eq + `]}`,
	} {
		spec := strings.Replace(mealsLookupSpec, `{"op":"and","args":[`+"\n\t\t\t"+eq+`,`+"\n\t\t\t"+days+`]}`, where, 1)
		if spec == mealsLookupSpec {
			t.Fatalf("%s: the where was not replaced", name)
		}
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{}, mealDays)
		for _, st := range *seen {
			if partBound.MatchString(st.sql) {
				t.Fatalf("%s: a part is bounded:\n%s", name, st.sql)
			}
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, mealDays, &legacySQL.noPartBounds)
	}
	// The same test finds the bound of the plain and.
	seen := observeReads(t)
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	bounded := false
	for _, st := range *seen {
		bounded = bounded || partBound.MatchString(st.sql)
	}
	if !bounded {
		t.Fatal("the and of the eq and the days bounds no part")
	}
}

// The topics of a run that reads a union through its parts name each part's
// collection, as a read of the union's rule CTE does.
func TestPartBoundsKeepTheTopics(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "day")
	bounded := z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	legacySQL.noPartBounds = true
	t.Cleanup(func() { legacySQL.noPartBounds = false })
	whole := z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	if got, want := strings.Join(bounded.Topics, ","), strings.Join(whole.Topics, ","); got != want {
		t.Fatalf("topics with part bounds %q, without %q", got, want)
	}
	if !strings.Contains(strings.Join(bounded.Topics, ","), "zoo_treats") {
		t.Fatalf("the topics name no part: %v", bounded.Topics)
	}
}
```

The tabs inside `mealsLookupSpec` matter: `TestAnEqUnderOrOrNotBoundsNoPart` replaces the where by its exact text (a line break and three tabs before the `eq` and before the `between`), and fails with "the where was not replaced" when they differ.

- [ ] **Step 3: Change the two tests that pin the old behaviour**

In `lookupbounds_test.go`, replace the whole of `TestALookupOnAUnionGetsNoDayBound` with its comment (lines 190-219) by:

```go
// A union source has no one stored column for its day (each part maps its
// own): a lookup on it bounds the day inside each part, on the part's own
// column, never on the union's rows, and counts the same documents as with no
// bound.
func TestALookupOnAUnionBoundsTheDayInsideEachPart(t *testing.T) {
	z := newZoo(t)
	addZooSales(t, z)
	registerZooSalesSources()
	spec := `{"version":4,"source":"zoo.sites",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.documents","pick":"count","value":{"path":[{"field":"id"}]},
				"where":{"op":"lt","args":[{"path":[{"field":"day"}]},{"param":"period","bound":"from"}]}}}],
		"sort":[{"col":"name"}]}`
	params := map[string]string{"period": `{"from":"2026-10-04","to":"2026-10-31"}`}
	// bounds runs the report and lists the alias of every bound on a stored
	// day in the statements it read.
	bound := regexp.MustCompile(`\[\[(\w+)\.day\]\] (>=|<=) `)
	bounds := func() []string {
		seen := observeReads(t)
		res := z.run(t, "ann", spec, Options{Today: "2026-10-15"}, params)
		// o1 (10-02) and k1 (10-03) are before the period; o2 and o3 are in it.
		for _, r := range res.Rows {
			if r.Cells["n"] != int64(2) {
				t.Fatalf("%v: %v documents before the period, want 2", r.Cells["name"], r.Cells["n"])
			}
		}
		if len(res.Rows) == 0 {
			t.Fatal("no rows")
		}
		var aliases []string
		for _, st := range *seen {
			for _, m := range bound.FindAllStringSubmatch(st.sql, -1) {
				aliases = append(aliases, m[1])
			}
		}
		return aliases
	}
	// Each statement that holds the lookup bounds both parts (zoo_orders and
	// zoo_tickets both store the day), and only inside a part (alias p).
	got := bounds()
	if len(got) < 2 || slices.ContainsFunc(got, func(alias string) bool { return alias != "p" }) {
		t.Fatalf("bounds on a stored day are on %v, want two or more, each inside a part (p)", got)
	}
	legacySQL.noPartBounds = true
	t.Cleanup(func() { legacySQL.noPartBounds = false })
	if got := bounds(); len(got) != 0 {
		t.Fatalf("with the switch on, a lookup on a union bounds a stored day on %v", got)
	}
}
```

(`regexp` and `slices` are already imported by `lookupbounds_test.go`. The built code gives four bounds on this run, `[p p p p]`; the test asks for two or more, each inside a part, so that it does not pin how many statements hold the lookup.)

In `bounds_test.go`, replace lines 275-276:

```go
// A union has no one stored column to bound: w's WHERE is the filter alone.
func TestFilterBoundsLeaveAUnionAlone(t *testing.T) {
```

by (the body of the test does not change):

```go
// A union has no one stored column to bound in w's WHERE, which stays the
// filter alone: its bounds are inside the parts (partbounds_test.go).
func TestFilterBoundsStayOutOfAUnionsWhere(t *testing.T) {
```

- [ ] **Step 4: Run the tests to see them fail**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -run 'TestALookupOverAUnionSearchesEachPart|TestAFilterOnAUnionSearchesEachPart|TestPartBoundsKeep|TestAPartHoldsOnlyTheBoundsItStores|TestAnEqUnderOrOrNotBoundsNoPart|TestALookupOnAUnionBoundsTheDayInsideEachPart|TestFilterBoundsStayOutOfAUnionsWhere' -count=1 -v .
```

Expected: FAIL, with these five failures (the planner's run):

```
lookupbounds_test.go:232: bounds on a stored day are on [], want two or more, each inside a part (p)
--- FAIL: TestALookupOnAUnionBoundsTheDayInsideEachPart
partbounds_test.go:74: the plan has no "SEARCH zoo_feedings USING INDEX idx_zoo_feedings_link (animal>? AND animal<?)":
--- FAIL: TestALookupOverAUnionSearchesEachPart
partbounds_test.go:104: the plan has no "SEARCH zoo_feedings USING INDEX idx_zoo_feedings_day (fed>? AND fed<?)":
--- FAIL: TestAFilterOnAUnionSearchesEachPart
partbounds_test.go:166: both keys stored: the part holds "", want "[[p.pet]] >= 'a' AND [[p.given]] >= 'b'"
partbounds_test.go:166: no column for the day: the part holds "", want "[[p.pet]] >= 'a'"
partbounds_test.go:166: a day in a number column: the part holds "", want "[[p.pet]] >= 'a'"
partbounds_test.go:166: a link in a text column: the part holds "", want "[[p.given]] >= 'b'"
partbounds_test.go:166: a link in a multi-select relation: the part holds "", want "[[p.packed]] >= 'b'"
--- FAIL: TestAPartHoldsOnlyTheBoundsItStores
partbounds_test.go:204: the and of the eq and the days bounds no part
--- FAIL: TestAnEqUnderOrOrNotBoundsNoPart
```

(The cases of `TestAPartHoldsOnlyTheBoundsItStores` that want no bound, "a view", "one bound of two keys, one not stored" and "an unknown collection", pass against the empty `partWhere`; they guard the code of step 6.)

`TestPartBoundsKeepTheResult`, `TestPartBoundsKeepTheTopics` and `TestFilterBoundsStayOutOfAUnionsWhere` PASS here: they guard that the change keeps the rows, the topics and w's WHERE, and they must pass before and after.

- [ ] **Step 5: `rules.go` — take the select list of a part out of `unionSelect`**

`boundedFrom` (step 6) writes the same select list. In `unionSelect`, delete the line

```go
	stem := "u_" + strings.ReplaceAll(src.Key, ".", "_")
```

and replace, inside its loop, the block from `cols := make([]string, 0, len(keys)+1)` to the line `selects = append(selects, "SELECT "+strings.Join(cols, ", ")+" FROM ("+ruleSQL+") p")` (both included) by this one line:

```go
		selects = append(selects, "SELECT "+unionPartCols(src, i, keys, params)+" FROM ("+ruleSQL+") p")
```

Add after `unionSelect`:

```go
// unionPartCols is the select list of part i of a union source, read as p:
// the row's id, then each of the source's keys. A constant is bound in params.
func unionPartCols(src *Source, i int, keys []string, params dbx.Params) string {
	part := src.Parts[i]
	stem := "u_" + strings.ReplaceAll(src.Key, ".", "_")
	cols := make([]string, 0, len(keys)+1)
	cols = append(cols, "[[p.id]] AS [[id]]")
	for _, key := range keys {
		column, mapped := part.Columns[key]
		value, constant := part.Values[key]
		switch {
		case mapped:
			cols = append(cols, fmt.Sprintf("[[p.%s]] AS [[%s]]", column, key))
		case constant:
			name := fmt.Sprintf("%s_%d_%s", stem, i, key)
			params[name] = value
			cols = append(cols, fmt.Sprintf("{:%s} AS [[%s]]", name, key))
		default:
			cols = append(cols, fmt.Sprintf("NULL AS [[%s]]", key))
		}
	}
	return strings.Join(cols, ", ")
}
```

(The body is the deleted block, unchanged. `part` in the loop of `unionSelect` is still used by the lines above the deleted block.)

- [ ] **Step 6: `stmt.go` — the scope keeps the bounds and writes the bounded union**

In the `scope` struct, add after the field `tms   []*tmSub`:

```go
	// partBounds are the bounds a union scope's rows lie inside, each written
	// on partLink and partDay columns (boundedFrom).
	partBounds []string
```

In `scope.render`, replace

```go
	parts := append([]string{"FROM " + sc.from}, sc.sql...)
```

by

```go
	parts := append([]string{"FROM " + sc.boundedFrom()}, sc.sql...)
```

Add after the consts `partLink` and `partDay` of step 1:

```go
var partColRef = regexp.MustCompile(`\[\[__part(link|day)\.([A-Za-z0-9_]+)\]\]`)

// boundedFrom is the scope's FROM. A union scope with part bounds reads its
// own union in place of the union's rule CTE: each part is read from its
// collection's rule CTE with the bounds it can hold in its WHERE, on the
// part's own columns. SQLite moves no condition on the union's rows into its
// parts, so a bound outside the union reads every row of every part; inside a
// part, an index of the part's table serves it. The rows are the rule CTE's
// rows inside the bounds: the columns and the list rules are the same.
func (sc *scope) boundedFrom() string {
	if len(sc.partBounds) == 0 || len(sc.src.Parts) == 0 || sc.s.app == nil || legacySQL.noPartBounds {
		return sc.from
	}
	wheres := make([]string, len(sc.src.Parts))
	bounded := false
	for i, part := range sc.src.Parts {
		wheres[i] = partWhere(sc.s.app, part, sc.partBounds)
		bounded = bounded || wheres[i] != ""
	}
	if !bounded {
		return sc.from
	}
	keys := sc.src.unionKeys()
	selects := make([]string, len(sc.src.Parts))
	for i, part := range sc.src.Parts {
		selects[i] = "SELECT " + unionPartCols(sc.src, i, keys, sc.s.params) + " FROM " + sc.s.rule(part.Collection) + " p"
		if wheres[i] != "" {
			selects[i] += " WHERE " + wheres[i]
		}
	}
	return "(" + strings.Join(selects, "\nUNION ALL\n") + ") " + sc.alias
}
```

Replace the empty `partWhere` of step 1 (its comment too) by:

```go
// partWhere is the bounds one part of a union holds, joined with AND: each
// bound whose every key the part maps to a column its collection stores (a
// single relation for a link, text or a PocketBase date for a day; never a
// view's column, which may hold NULL). A part that cannot hold a bound reads
// its rows without it: the rows stay right, only slower.
func partWhere(app core.App, part SourcePart, bounds []string) string {
	col, err := app.FindCachedCollectionByNameOrId(part.Collection)
	if err != nil || col.IsView() {
		return ""
	}
	var held []string
	for _, bound := range bounds {
		ok := true
		sql := partColRef.ReplaceAllStringFunc(bound, func(ref string) string {
			m := partColRef.FindStringSubmatch(ref)
			kinds := []string{core.FieldTypeRelation}
			if m[1] == "day" {
				kinds = []string{core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate}
			}
			field := col.Fields.GetByName(part.Columns[m[2]])
			if field == nil || !slices.Contains(kinds, field.Type()) {
				ok = false
				return ref
			}
			// A multi-select relation holds a JSON array, never an id.
			if rel, isRel := field.(*core.RelationField); isRel && rel.IsMultiple() {
				ok = false
				return ref
			}
			return q("p", field.GetName())
		})
		if ok {
			held = append(held, sql)
		}
	}
	return strings.Join(held, " AND ")
}
```

(`stmt.go` already imports `regexp`, `slices`, `strings` and the PocketBase `core`.)

- [ ] **Step 7: `formula.go` — an `eq` on a union's link gives a bound**

In the `storedLink` struct, add after the field `eqs      *[]linkEq`:

```go
	// part is the link's column of a union source, under partLink: no one
	// stored column, so eq bounds each part of the union instead (partLinkBound).
	part string
```

In `case "eq", "ne":` (line 861), replace its plain return (line 874)

```go
		return expr{sql: "(" + args[0].sql + " IS " + args[1].sql + ")", typ: Bool}, nil
```

by

```go
		return expr{sql: "(" + args[0].sql + " IS " + args[1].sql + ")", typ: Bool, bound: partLinkBound(c, n, args)}, nil
```

- [ ] **Step 8: `lookup.go` — a lookup over a union puts its bound on the scope**

(a) In `lookupIn` (lines 160-171), replace

```go
			if w.bound != "" {
				where += " AND " + w.bound
			}
		}
		out.agg, out.win = out.agg || w.agg, out.win || w.win
		if w.condMixed != "" {
			unknown = whereUnknown(outer, sc, w, at+".where")
		}
	}
```

by

```go
			if w.bound != "" && len(src.Parts) == 0 {
				where += " AND " + w.bound
			}
		}
		out.agg, out.win = out.agg || w.agg, out.win || w.win
		if w.condMixed != "" {
			unknown = whereUnknown(outer, sc, w, at+".where")
		}
		// A union has no one stored column: its bounds go inside each part
		// (boundedFrom). The test for an unknown where above reads the whole
		// union, as it reads a collection without the bounds.
		if !w.unset && w.bound != "" && len(src.Parts) > 0 {
			sc.partBounds = append(sc.partBounds, w.bound)
		}
	}
```

(b) In `lookupCtx.path` (lines 501-505), replace

```go
		if col := l.storedLinkCol(hops); col != "" {
			e.link = storedLink{col: col, sql: e.sql, sc: l.sc, eqs: l.eqs}
		}
```

by

```go
		if col := l.storedLinkCol(hops); col != "" {
			e.link = storedLink{col: col, sql: e.sql, sc: l.sc, eqs: l.eqs}
		} else if l.unionParts() && len(hops) == 1 {
			e.link = storedLink{part: q(partLink, hops[0].link.Field), sql: e.sql}
		}
```

(c) Add before the comment of `storedDayCol` (line 591):

```go
// unionParts is true where a lookup's where may bound the parts of its union
// source: the path starts at the looked-up row itself (no prefix), and the
// compile has a database to read each part's collection from.
func (l *lookupCtx) unionParts() bool {
	return len(l.src.Parts) > 0 && len(l.prefix) == 0 && l.sc.src == l.src && !l.r.c.dry() && !legacySQL.noPartBounds
}

// partLinkBound is the bound of eq(link, V) in the where of a lookup over a
// union source, on the link's partLink column: the range linkedEq writes on a
// stored column (V gives the column V, a blank V every column), "" for any
// other eq. V reads nothing of the looked-up row and holds no lookup: the
// bound is written inside each part of the union, which reads the report's
// row but no table beside the union (so V is not computed once in oneValue),
// and repeats V's SQL.
func partLinkBound(c fctx, n *Node, args []expr) string {
	for i, a := range args[:2] {
		other := n.Args[1-i]
		if a.link.part == "" || a.link.sql != a.sql || readsLookedUpRow(other) || containsLookup(other) || readsLookupColumn(c, other, map[string]bool{}) {
			continue
		}
		v := args[1-i].sql
		return fmt.Sprintf("%s >= COALESCE(%s, '') AND %s <= COALESCE(%s, x'ff')", a.link.part, v, a.link.part, v)
	}
	return ""
}
```

(d) Replace the comment of `storedDayCol` and the start of its body:

```go
// storedDayCol is the stored column of a Date field of the lookup scope's own
// row, or "" where a bound on it is not safe: a view's or a union's column
// (which may hold NULL, or no day), or a column that is not text or a
// PocketBase date. filterBounds makes the same test of the report's base.
func (l *lookupCtx) storedDayCol(f *Field) string {
```

by

```go
// storedDayCol is the stored column of a Date field of the lookup scope's own
// row, or "" where a bound on it is not safe: a view's column (which may hold
// NULL, or no day), or a column that is not text or a PocketBase date.
// filterBounds makes the same test of the report's base. A union's day is its
// partDay column: each part tests its own column (partWhere).
func (l *lookupCtx) storedDayCol(f *Field) string {
	if l.unionParts() && !legacySQL.noLookupDayBounds {
		return q(partDay, f.Column)
	}
```

The lines of the body after it do not change (the next one is the comment `// A dry compile (Validate) has no database to read the collection from.`).

- [ ] **Step 9: `bounds.go` — a filter on a union puts its bounds on the root scope**

Replace the last paragraph of the comment of `filterBounds`:

```go
// Only a term of the filter's top-level and (or the filter itself) gives a
// bound: a row the filter keeps passes every such term. The base must be a
// collection that is not a view and has no union parts, as storedLinkCol
// requires of a lookup's source.
```

by

```go
// Only a term of the filter's top-level and (or the filter itself) gives a
// bound: a row the filter keeps passes every such term. A base collection
// must not be a view, as storedLinkCol requires of a lookup's source. A union
// base has no one stored column: its bounds go on the root scope, which
// writes them inside each part (boundedFrom), and w's WHERE gets none.
```

Replace the body of `filterBounds` by:

```go
func (c *compiler) filterBounds(r *rowsCTE) (string, error) {
	f := c.p.spec.Filter
	// A dry compile (Validate) has no database to read the collection from.
	if f == nil || legacySQL.noFilterBounds || c.dry() {
		return "", nil
	}
	union := len(c.p.base.Parts) > 0
	if union && legacySQL.noPartBounds {
		return "", nil
	}
	// stored is the stored column a bound names for column name of the base
	// row, "" when the base does not keep it as one of kinds: a bound compares
	// the stored value as text. A union's column is a partLink or partDay
	// column, which each part tests for itself (partWhere).
	stored := func(name string, kinds ...string) string {
		if kinds[0] == core.FieldTypeRelation {
			return q(partLink, name)
		}
		return q(partDay, name)
	}
	if !union {
		col, err := c.app.FindCachedCollectionByNameOrId(c.p.base.Collection)
		if err != nil {
			// Without the bounds the rows are still right, only slower.
			log.Warn("filter not bounded", "collection", c.p.base.Collection, "err", err)
			return "", nil
		}
		if col.IsView() {
			return "", nil
		}
		stored = func(name string, kinds ...string) string {
			field := col.Fields.GetByName(name)
			if field == nil || !slices.Contains(kinds, field.Type()) {
				return ""
			}
			// A multi-select relation holds a JSON array, never an id.
			if rel, isRel := field.(*core.RelationField); isRel && rel.IsMultiple() {
				return ""
			}
			return q(r.root.alias, name)
		}
	}
	terms := []*Node{f}
	if f.Op == "and" {
		terms = f.Args
	}
	var bounds []string
	for i, term := range terms {
		if term == nil || len(term.Args) != 2 {
			continue
		}
		at := "filter"
		if f.Op == "and" {
			at = fmt.Sprintf("filter.args.%d", i)
		}
		var bound string
		var err error
		switch term.Op {
		case "in":
			bound, err = r.listBound(term, at, stored)
		case "between", "lt", "lte", "gt", "gte":
			bound, err = r.dayBound(term, at, stored)
		}
		if err != nil {
			return "", err
		}
		if bound != "" {
			bounds = append(bounds, bound)
		}
	}
	if union {
		// Each term is one bound, so a part that cannot hold one still holds
		// the others.
		r.root.partBounds = bounds
		return "", nil
	}
	return strings.Join(bounds, " AND "), nil
}
```

`stored` now returns the column text in place of a bool. In `listBound`, change the signature and three lines:

```go
func (r *rowsCTE) listBound(term *Node, at string, stored func(string, ...string) string) (string, error) {
```

replace

```go
	if !stored(p.hops[0].link.Field, core.FieldTypeRelation) {
		return "", nil
	}
```

by

```go
	col := stored(p.hops[0].link.Field, core.FieldTypeRelation)
	if col == "" {
		return "", nil
	}
```

and its last line `return linkBound(q(r.root.alias, p.hops[0].link.Field), list.sql), nil` by

```go
	return linkBound(col, list.sql), nil
```

In `dayBound`, change the signature:

```go
func (r *rowsCTE) dayBound(term *Node, at string, stored func(string, ...string) string) (string, error) {
```

replace

```go
	if !stored(p.field.Column, core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate) {
		return "", nil
	}
```

by

```go
	col := stored(p.field.Column, core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate)
	if col == "" {
		return "", nil
	}
```

and delete the later line `col := q(r.root.alias, p.field.Column)` (the `switch` under it reads the `col` above).

- [ ] **Step 10: Run the tests of this task, the vet and the format check**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && gofmt -l . ; go vet . && go test -run 'TestALookupOverAUnionSearchesEachPart|TestAFilterOnAUnionSearchesEachPart|TestPartBoundsKeep|TestAPartHoldsOnlyTheBoundsItStores|TestAnEqUnderOrOrNotBoundsNoPart|TestALookupOnAUnionBoundsTheDayInsideEachPart|TestFilterBoundsStayOutOfAUnionsWhere' -count=1 -v .
```

Expected: `gofmt -l` prints nothing, `go vet` prints nothing, the eight tests PASS (`ok  	tinycld.org/core/reports`).

- [ ] **Step 11: Run the package**

The change is in the FROM of every scope, so every test of the engine is a test of it. Run (about 2 minutes):

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 .
```

Expected: `ok  	tinycld.org/core/reports` (the planner's run: 113 s). A failure is a defect of this change: find its cause and correct it; do not change a test to pass, and do not run again to see if it passes.

- [ ] **Step 12: Commit (core)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && git add core/server/reports/partbounds_test.go core/server/reports/legacy.go core/server/reports/stmt.go core/server/reports/rules.go core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/bounds.go core/server/reports/lookupbounds_test.go core/server/reports/bounds_test.go && git commit -m "perf(reports): a bound on a union source is written inside each part"
```

- [ ] **Step 13: Hand the 100k run to the driver**

Do not start it. Hand back, as the task's result:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
Needed from it: the files scale-task9b/st-results.md and scale-task9b/st-plans.md, and the PASS or FAIL line of the test.
```

The run needs the commit of step 12 in the tree (Books reads core through the workspace). No bar is held at 100k, so the test passes when each run with a bar shows a group row; a run core refuses is recorded as refused.

- [ ] **Step 14: Record the result in the ledger (Books)**

With the files of step 13, append to the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`. Write the heading and the first two paragraphs as they are here:

```markdown
#### Task 9b: a bound on a union source is written inside each part (core `server/reports/stmt.go`, 2026-11-20)

The Balance Sheet and Transaction Detail read the union source `books.basis_lines` (parts `books_journal_lines` and `books_cash_basis_lines`). Before: a scope read the union's rule CTE (`FROM r<n> t<n>`) and the bounds of the filter and of a lookup's where were conditions on the union's rows. SQLite moves no condition into the parts of a `UNION ALL`, so each read was a `SCAN` of both tables, and a lookup read them once for each row of the report. After: the scope writes the union itself, each part from its own collection's rule CTE with the bounds on the part's own columns: `FROM (SELECT … FROM r<a> p WHERE p.account IN (…) AND p.date >= ? AND p.date <= (? || x'ff') UNION ALL SELECT … FROM r<b> p WHERE …) t<n>`. A part that does not store a key of a bound reads its rows without that bound. The switch `legacySQL.noPartBounds` gives the old statement; `partbounds_test.go` holds the plans and the same-result tests.

Not fixed by this task: Transaction Detail with no Accounts filter (TD 1, TD 2). Its lookups search by entry now, but its base rows have a day bound alone, and no index of the two tables starts with the day: the plan still shows `SCAN books_journal_lines` and `SCAN books_cash_basis_lines`, and `SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)`. A later task of this unit handles it.
```

Then add, under them:

1. The line `At 100,000 lines (median of 5, core <the commit of step 12>, books d9a695a7):` with the short hash of the core commit (`git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD`).
2. A table with the header `| run | baseline (task 9a) | after task 9b | group rows | rows |` and one row for each of the eleven runs, in the order of `st-results.md`. The baseline column is this text, one for each run in order: `744ms`, `480ms`, `565ms`, `1.389s`, `1.708s`, `refused`, `refused`, `refused`, `312ms`, `refused`, `refused` (TB 1, TB 2, TB 3, BS 1, BS 2, TD 1 to TD 6). The three other columns are the columns `median of 5`, `group rows` and `rows` of `scale-task9b/st-results.md`, copied as they are; a refused run is written `refused after <time>`.
3. The heading line `Reads of the line tables in statement 6 (the page of detail rows) of TD 5 three accounts, year, and of TD 2 all accounts, year (scale-task9b/st-plans.md):` and under it, in one fenced block for each of the two runs, every line of that statement's plan in `st-plans.md` that names `books_journal_lines`, `books_cash_basis_lines` or `AUTOMATIC`. On the small book the planner saw, for TD 5: `SEARCH books_journal_lines USING INDEX idx_books_jlines_account_date (account=? AND date>? AND date<?)`, the same on `idx_books_cash_basis_lines_account_date`, and the lookups `SEARCH … USING INDEX idx_books_jlines_entry (entry>? AND entry<?)` and `idx_books_cash_basis_lines_entry`, with no `MATERIALIZE r0`; for TD 2 the two `SCAN` lines and the `AUTOMATIC` line named above. Write what the 100k files show, also when it differs from this.
4. One sentence that names every run that is still refused at 100k, or `No run is refused at 100k.`

Change no code in this step, whatever the run shows: a run with an Accounts filter (TD 3 to TD 6) that is still refused, or a Balance Sheet that is slower than its baseline, goes in the record as measured and in the hand-back of the task, with the path of `st-plans.md`. The next plan part chooses the next change from it.

- [ ] **Step 15: Commit the record (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs: the three statements at 100k after the union part bounds"
```

---

## After this part

The later tasks of the unit (the outline, `plan-outline.md`) are planned from the record of step 14: Transaction Detail with no Accounts filter (the `SCAN` by day and the automatic currency index), then the 500k measure against the bars. They are not in this part.
