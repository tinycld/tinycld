# Report Engine Scale, Part 5: A Lookup Reads One Account's Lines Before a Day by Its Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A General Ledger run with an Accounts filter takes 1 s or less on a book with 500,000 journal lines, with every report's result unchanged.

**Architecture:** A section's beginning balance is a sum lookup over the lines of one account before a day. Its where compares the day through `substr()` and bounds the account column by a range (`linkedEq`), so SQLite searches the index `(account, date)` by the account only and reads the account's whole history. Task 6 adds a bound on the stored day column next to the where (the form `dayBounds` of task 2 writes for the report filter). Task 5 writes each part of such a lookup in two forms inside a `CASE`: where the looked-for value is not blank, the account column is bound by an equality, so the index serves the account and the day; where it is blank (a blank or hidden record, the Total row) the range form stays. No grouped CTE is written.

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The unit's requirements, the baseline and the stored-summary decision: part 1, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale.md`. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`. The measurements and the design facts this part rests on: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part4b.md`, sections "Result" and "Facts for the design of tasks 5 and 6".

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Tasks 6 and 5 are core tasks: they may edit `tinycld/core`. The last step of each changes one Books file, the ledger record.

**Order:** parts 1 to 4 are done (core 298829bd, books 9d9ce3b6). This part: task 6, then task 5 (task 5's plan test needs task 6's bound). Tasks 5d and 7 are the next part (section "What is left").

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. A change to the engine's SQL has a switch in `legacySQL` (`core/server/reports/legacy.go`), one test that proves the switch writes the old SQL, and a cross-check that runs a report with the switch on and off and compares the two results byte for byte (`requireSameAsLegacy`).
- Core names no package: core tests use the fictional zoo only (`zoo.ledger_lines`, `zoo_ledger_lines`); no Books slug, collection or route in `tinycld/core`.
- Each change to SQL is recorded with its plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)", under "Changes".
- The 100k scale run takes more than 4 minutes with its build: an implementer does not start it. It is a `NEEDS-RUN` for the driver.
- The whole core `reports` package takes about 105 s. Run it once, at the end of each task.
- Go code is formatted with `gofmt`. Server logging goes through the package's `log`.
- Change files with the Edit tool only (a hook blocks scripted edits).

## Review Focus

1. A day comparison under `or` or `not` in a lookup's where: a bound there would drop rows the where keeps (a blank day makes the comparison unknown, and `not` of unknown is not the same as `not` of false). Expected: no bound. Test: task 6, `TestALookupsDayComparisonBoundsTheStoredDay`, cases "under or" and "under not".
2. A stored day with a time after it (`2026-07-01 00:00:00.000Z`) on the first or the last day of the period, and a blank day. Expected: counted as its day; a blank day matches no comparison. Test: task 6, `TestALookupsDayBoundsKeepTheEdgeDays`.
3. A lookup on a union source: there is no one stored column. Expected: no bound, the same rows. Test: task 6, `TestALookupOnAUnionGetsNoDayBound`. (A view takes the `col.IsView()` branch of `storedDayCol`, the same test `storedLinkCol` makes; the zoo has no view with a Date field, so that branch has no test.)
4. A looked-for value that is blank: a section of lines with no account, the Total row, a reader who may list no account. The equality form would match no row there. Expected: the range form runs, and the pages are the same. Test: task 5, `TestKnownLinksKeepTheResult` (lines with a blank account, reader `bob`). Not covered by a test: a blank value on the `oneValue` branch (an `outer` link that is blank) and a link `eq` under `or` or `not`; the two texts have the same truth value where the value is not NULL, so the replacement is right there too.
5. A looked-for value that is costly (a nested lookup): the test outside the lookup would compute it again. Expected: only a leaf value gets the second form. Test: the existing `TestALinkLookupComputesTheValueItMatchesOnce` (`linkonce_test.go`) must pass unchanged in task 5. Accepted, speed only, no test: a day bound writes its other side a second time (also when that side is a lookup), and a `col` leaf whose column is an inline lookup is read once more by the `CASE` test.

---

### Task 6: Day bounds in a lookup's where (core)

**Why:** the lookup's where reads the day as `NULLIF(substr(col, 1, 10), '')`, which no index serves. Alone this task changes no plan line of the General Ledger (the account is still bound by a range, so SQLite cannot use `date` after it; measured: 870 to 865 ms). It is the base of task 5.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/formula.go` (the struct `expr`, after `storedLink`; in `operatorExpr` the cases `"lt", "lte", "gt", "gte"`, `"between"` and `"and", "or"`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookup.go` (imports, `lookupIn`, `(*lookupCtx).path`, two new functions before `oneValue`)
- Create (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookupbounds_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (step 8)

**Interfaces:**
- Consumes (all in the tree; read each definition before use): `dayBounds(col, lo, hi string) string` (`bounds.go:164`; `""` leaves a side open), `readsLookedUpRow(n *Node) bool` (`lookup.go`), `mirrored` (`zone.go`), `q(alias, column string) string`, `(*compiler).dry()`, `log`. Test helpers: `newZoo(t)`, `withBalanceZoo(t, z)`, `glJuly` (`balance_shape_test.go`), `observeReads(t) *[]readStatement` (`statement_test.go`), `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)` (`aggonce_test.go`), `addZooSales(t, z)`, `registerZooSalesSources()` (`union_shape_test.go`), `z.run`, `z.rec`, `z.id`.
- Produces: `legacySQL.noLookupDayBounds bool`; `expr.day storedDay` and `expr.bound string`; `type storedDay struct{ col, sql string }`; `(l *lookupCtx) storedDayCol(f *Field) string`; `dayBound(n *Node, args []expr) string`. Rule: a comparison `lt/lte/gt/gte/between` in a lookup's where, between a Date field of the looked-up row itself (no hops; a text or date column of a collection that is not a view and has no union parts) and a Date value that reads nothing of that row, carries a bound; `and` passes its terms' bounds up; `lookupIn` writes the where's bound after the where (`(<where>) AND <bound>`). Test helpers for task 5: none.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/lookupbounds_test.go`:

```go
package reports

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The pieces of a lookup's where over the ledger lines: the line's account is
// the report row's, its day, and the two days of the period param.
const (
	lbLink = `{"op":"eq","args":[{"path":[{"link":"account"}]},{"outer":[{"field":"id"}]}]}`
	lbDay  = `{"path":[{"field":"date"}]}`
	lbFrom = `{"param":"period","bound":"from"}`
	lbTo   = `{"param":"period","bound":"to"}`
)

// lookupBoundsSpec lists the ledger accounts, each with the count of the
// ledger lines its where holds for.
func lookupBoundsSpec(where string) string {
	return `{"version":4,"source":"zoo.ledger_accounts",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"name","path":[{"field":"full_name"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.ledger_lines","pick":"count","value":{"path":[{"field":"id"}]},"where":` + where + `}}],
		"sort":[{"col":"name"}]}`
}

// storedDayBound finds a bound on the stored day column of the ledger lines:
// the bare column compared (the where itself reads the day through substr).
var storedDayBound = regexp.MustCompile(`\[\[t\d+\.date\]\] (>=|<=) `)

// dayBoundsOf lists the sides (">=", "<=") of the bounds on the lines' stored
// day in the statements seen, each once, sorted.
func dayBoundsOf(seen []readStatement) []string {
	var sides []string
	for _, st := range seen {
		for _, m := range storedDayBound.FindAllStringSubmatch(st.sql, -1) {
			if !slices.Contains(sides, m[1]) {
				sides = append(sides, m[1])
			}
		}
	}
	slices.Sort(sides)
	return sides
}

// withEdgeDays adds lines on Cash whose stored day is blank, or a day with a
// time after it on the last day before the period, its first and its last.
func withEdgeDays(t *testing.T, z *zoo) {
	t.Helper()
	for key, day := range map[string]string{
		"edgeBlank": "", "edgeBefore": "2026-06-30 23:59:59.000Z",
		"edgeFirst": "2026-07-01 00:00:00.000Z", "edgeLast": "2026-07-31 12:00:00.000Z",
	} {
		z.rec(t, key, "zoo_ledger_lines", map[string]any{
			"entry": z.id("J0"), "account": z.id("cash"), "date": day, "debit": 100, "currency": "USD", "posted": true, "position": 9,
		})
	}
}

// A comparison of the looked-up row's stored day with a day that reads
// nothing of that row bounds the stored column, when the where cannot hold
// without it: as the where itself or under its ands. Under or and not, and
// between two days of the row, there is no bound. The result is the same with
// and without the bounds, for a period and for All dates.
func TestALookupsDayComparisonBoundsTheStoredDay(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withEdgeDays(t, z)
	and := func(terms ...string) string { return `{"op":"and","args":[` + strings.Join(terms, ",") + `]}` }
	op := func(name string, args ...string) string {
		return `{"op":"` + name + `","args":[` + strings.Join(args, ",") + `]}`
	}
	cases := []struct {
		name, where string
		want        []string
	}{
		{"before a day", and(lbLink, op("lt", lbDay, lbFrom)), []string{"<="}},
		{"the day on the right", and(lbLink, op("gt", lbFrom, lbDay)), []string{"<="}},
		{"from a day", and(lbLink, op("gte", lbDay, lbFrom)), []string{">="}},
		{"month to date", and(lbLink, op("gte", lbDay, op("startOfMonth", lbTo)), op("lt", lbDay, lbTo)), []string{"<=", ">="}},
		{"a date range", and(lbLink, op("between", lbDay, `{"param":"period"}`)), []string{"<=", ">="}},
		{"between two days", and(lbLink, op("between", lbDay, lbFrom, lbTo)), []string{"<=", ">="}},
		{"an and inside the and", and(lbLink, and(op("gte", lbDay, lbFrom), op("lte", lbDay, lbTo))), []string{"<=", ">="}},
		{"the where itself", op("lt", lbDay, lbFrom), []string{"<="}},
		{"under or", and(lbLink, op("or", op("lt", lbDay, lbFrom), op("eq", `{"path":[{"field":"posted"}]}`, `{"value":false}`))), nil},
		{"under not", and(lbLink, op("not", op("gte", lbDay, lbFrom))), nil},
		{"two days of the row", and(lbLink, op("lte", lbDay, lbDay)), nil},
	}
	july := map[string]string{"period": glJuly}
	t.Cleanup(func() { legacySQL.noLookupDayBounds = false })
	for _, c := range cases {
		spec := lookupBoundsSpec(c.where)
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{Today: "2026-07-15"}, july)
		if got := dayBoundsOf(*seen); !slices.Equal(got, c.want) {
			t.Errorf("%s: bounds on the stored day %v, want %v", c.name, got, c.want)
		}
		// The switch writes the SQL of before: no bound on the stored day.
		legacySQL.noLookupDayBounds = true
		*seen = nil
		z.run(t, "ann", spec, Options{Today: "2026-07-15"}, july)
		legacySQL.noLookupDayBounds = false
		if got := dayBoundsOf(*seen); len(got) != 0 {
			t.Errorf("%s: with the switch on, bounds on the stored day %v, want none", c.name, got)
		}
		for _, params := range []map[string]string{july, {"period": `{"preset":"allDates"}`}} {
			requireSameAsLegacy(t, z, "ann", spec, Options{Today: "2026-07-15"}, params, &legacySQL.noLookupDayBounds)
		}
	}
}

// A union source has no one stored column for its day (each part maps its
// own, and a part may map none): a lookup on it gets no bound and counts the
// same documents.
func TestALookupOnAUnionGetsNoDayBound(t *testing.T) {
	z := newZoo(t)
	addZooSales(t, z)
	registerZooSalesSources()
	seen := observeReads(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.sites",
		"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}}],
		"columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"n","formula":{"op":"lookup","source":"zoo.documents","pick":"count","value":{"path":[{"field":"id"}]},
				"where":{"op":"lt","args":[{"path":[{"field":"day"}]},{"param":"period","bound":"from"}]}}}],
		"sort":[{"col":"name"}]}`, Options{Today: "2026-10-15"}, map[string]string{"period": `{"from":"2026-10-04","to":"2026-10-31"}`})
	// o1 (10-02) and k1 (10-03) are before the period; o2 and o3 are in it.
	for _, r := range res.Rows {
		if r.Cells["n"] != int64(2) {
			t.Fatalf("%v: %v documents before the period, want 2", r.Cells["name"], r.Cells["n"])
		}
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows")
	}
	bound := regexp.MustCompile(`\.day\]\] (>=|<=) `)
	for _, st := range *seen {
		if bound.MatchString(st.sql) {
			t.Fatalf("a lookup on a union bounds a stored day:\n%s", st.sql)
		}
	}
}

// The counts themselves, on the days at the edges of the period: a day with a
// time after it counts as its day, and a blank day matches no comparison.
func TestALookupsDayBoundsKeepTheEdgeDays(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withEdgeDays(t, z)
	count := func(where string) any {
		t.Helper()
		spec := lookupBoundsSpec(`{"op":"and","args":[` + lbLink + `,` + where + `]}`)
		res := z.run(t, "ann", spec, Options{Today: "2026-07-15"}, map[string]string{"period": glJuly})
		for _, r := range res.Rows {
			if r.Cells["name"] == "Cash" {
				return r.Cells["n"]
			}
		}
		t.Fatal("no Cash row")
		return nil
	}
	// Cash: j1a 07-01, j2a 07-02, j3a 07-03, and the four edge lines.
	for _, c := range []struct {
		name, where string
		want        int64
	}{
		{"before the first day", `{"op":"lt","args":[` + lbDay + `,` + lbFrom + `]}`, 1},
		{"up to the first day", `{"op":"lte","args":[` + lbDay + `,` + lbFrom + `]}`, 3},
		{"after the first day", `{"op":"gt","args":[` + lbDay + `,` + lbFrom + `]}`, 3},
		{"from the last day", `{"op":"gte","args":[` + lbDay + `,` + lbTo + `]}`, 1},
		{"in the period", `{"op":"between","args":[` + lbDay + `,{"param":"period"}]}`, 5},
	} {
		if got := count(c.where); got != c.want {
			t.Errorf("%s: %v lines, want %d", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:
`go test -run 'TestALookupsDay|TestALookupOnAUnionGetsNoDayBound' -count=1 ./reports/`

Expected: the build fails with `legacySQL.noLookupDayBounds undefined`. A build failure is not yet a behavioral red, so add the field alone (step 3) and run again.

- [ ] **Step 3: Add the switch**

In `core/server/reports/legacy.go`, after the field `inlineDetailRows bool`:

```go
	// noLookupDayBounds leaves the bounds on a looked-up source's stored day
	// column out of a lookup's WHERE, as before dayBound wrote them.
	noLookupDayBounds bool
```

Run the command of step 2 again. Expected: FAIL, with eight lines of `TestALookupsDayComparisonBoundsTheStoredDay` of the form `before a day: bounds on the stored day [], want [<=]` (one for each case that wants a bound). `TestALookupsDayBoundsKeepTheEdgeDays` and `TestALookupOnAUnionGetsNoDayBound` PASS: they pin what must not change.

- [ ] **Step 4: Carry the stored day and the bound on `expr`**

In `core/server/reports/formula.go`, in the struct `expr`, after the field `link storedLink`:

```go
	// day is, in a lookup's where, the stored column a Date field of the
	// looked-up row itself is read from; a comparison of it with a day bounds
	// that column too (dayBound).
	day storedDay
	// bound is, on a condition in a lookup's where, bounds on stored columns of
	// the looked-up row that hold on every row the condition holds for. Only
	// and passes it up (a row an and holds for passes each of its terms), and
	// only lookupIn reads it, on the where itself. "" for any other value.
	bound string
```

After the type `storedLink`:

```go
// storedDay is the stored column (col) a Date field of a lookup's own row is
// read from as sql; a copy of the expr whose sql changed no longer reads it.
type storedDay struct{ col, sql string }
```

In `operatorExpr`, case `"lt", "lte", "gt", "gte"`: the last `return` gets the bound, and the case `"between"` too. Replace

```go
		return expr{sql: "(" + args[0].sql + " " + comparisons[n.Op] + " " + args[1].sql + ")", typ: Bool}, nil
	case "between":
		return between(c, args, at)
```

with

```go
		return expr{sql: "(" + args[0].sql + " " + comparisons[n.Op] + " " + args[1].sql + ")", typ: Bool, bound: dayBound(n, args)}, nil
	case "between":
		e, err := between(c, args, at)
		if err != nil {
			return expr{}, err
		}
		e.bound = dayBound(n, args)
		return e, nil
```

In the case `"and", "or"`, replace the lines from `parts := make(…)` through the last `return` of the case with:

```go
		parts := make([]string, 0, len(args))
		var bounds []string
		for i, a := range args {
			if a.typ != Bool && a.typ != "" {
				return expr{}, invalid(fmt.Sprintf("%s.args.%d", at, i), n.Op+" needs conditions")
			}
			if !a.unset {
				parts = append(parts, a.sql)
			}
			// A row an and holds for passes each of its terms, so it is inside
			// each term's bounds. An or passes none up.
			if n.Op == "and" && !a.unset && a.bound != "" {
				bounds = append(bounds, a.bound)
			}
		}
		if len(parts) == 0 {
			return unsetCondition, nil
		}
		return expr{sql: "(" + strings.Join(parts, " "+strings.ToUpper(n.Op)+" ") + ")", typ: Bool, bound: strings.Join(bounds, " AND ")}, nil
```

Every other operator builds a new `expr`, so a bound never passes through `or`, `not`, `if` or `coalesce` (read `operatorExpr` to confirm before you rely on it; `operator` returns `unsetCondition` for a condition on an unset param, which drops the bound too).

- [ ] **Step 5: Find the stored day, write the bound**

In `core/server/reports/lookup.go`:

Add the import `"github.com/pocketbase/pocketbase/core"` (as `bounds.go` has it).

In `(*lookupCtx).path`, before the line `switch cur, fixed := amountCurrencyOf(l.sc, hops, bound); {`:

```go
	if l.inWhere && len(hops) == 0 && !bound.endsAtLink && bound.field != nil && bound.field.Type == Date {
		if col := l.storedDayCol(bound.field); col != "" {
			e.day = storedDay{col: col, sql: e.sql}
		}
	}
```

Before the comment of `oneValue`, add:

```go
// storedDayCol is the stored column of a Date field of the lookup scope's own
// row, or "" where a bound on it is not safe: a view's or a union's column
// (which may hold NULL, or no day), or a column that is not text or a
// PocketBase date. filterBounds makes the same test of the report's base.
func (l *lookupCtx) storedDayCol(f *Field) string {
	// A dry compile (Validate) has no database to read the collection from.
	if legacySQL.noLookupDayBounds || l.r.c.dry() || len(l.src.Parts) > 0 {
		return ""
	}
	col, err := l.r.c.p.app.FindCachedCollectionByNameOrId(l.src.Collection)
	if err != nil {
		// Without the bound the lookup is still right, only slower.
		log.Warn("lookup day column not bounded", "collection", l.src.Collection, "err", err)
		return ""
	}
	if col.IsView() {
		return ""
	}
	field := col.Fields.GetByName(f.Column)
	if field == nil || !slices.Contains([]string{core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate}, field.Type()) {
		return ""
	}
	return q(l.sc.alias, f.Column)
}

// dayBound is the bound a comparison in a lookup's where implies on a stored
// day column of the looked-up row: for lt, lte, gt or gte of such a day with a
// day that reads nothing of that row (a param's day, a bound of a range, a
// formula over them, a value of the report's row), and for between of it with
// a date range param or with two such days. "" for any other comparison. The
// comparison reads the day through substr(), which no index serves; the bound
// names the column itself (dayBounds). It holds on every row the comparison
// holds for: where the other day is blank the comparison holds for no row,
// and neither does the bound.
func dayBound(n *Node, args []expr) string {
	stored := func(e expr) bool { return e.day.col != "" && e.day.sql == e.sql }
	free := func(i int) bool {
		e := args[i]
		return e.typ == Date && !e.rng && !e.unset && !e.all && !readsLookedUpRow(n.Args[i])
	}
	if n.Op == "between" {
		switch {
		case len(args) < 2 || !stored(args[0]):
		case len(args) == 2 && args[1].rng && !args[1].all && !args[1].unset:
			return dayBounds(args[0].day.col, args[1].lo, args[1].hi)
		case len(args) == 3 && free(1) && free(2):
			return dayBounds(args[0].day.col, args[1].sql, args[2].sql)
		}
		return ""
	}
	for i := range 2 {
		if !stored(args[i]) || !free(1-i) {
			continue
		}
		op := n.Op
		if i == 1 {
			op = mirrored[op]
		}
		if op == "lt" || op == "lte" {
			return dayBounds(args[i].day.col, "", args[1-i].sql)
		}
		return dayBounds(args[i].day.col, args[1-i].sql, "")
	}
	return ""
}
```

In `lookupIn`, replace

```go
		if !w.unset {
			where = "(" + w.sql + ")"
		}
```

with

```go
		if !w.unset {
			where = "(" + w.sql + ")"
			// The bounds name the stored day column itself, which an index on
			// it serves; they leave out only rows the where refuses.
			if w.bound != "" {
				where += " AND " + w.bound
			}
		}
```

`where` goes into every part of the lookup (`pickedLookup`); `whereUnknown` reads `w.sql` and stays as it is.

- [ ] **Step 6: Run the tests to see them pass, then the whole package, and commit core**

Run, in `core/server`:
`go vet ./reports/ && go test -run 'TestALookupsDay|TestALookupOnAUnionGetsNoDayBound' -count=1 ./reports/`
Expected: `ok  	tinycld.org/core/reports` (about 1 s).

Then once: `gofmt -l ./reports/ && go test -count=1 ./reports/`
Expected: `gofmt` prints nothing; `ok  	tinycld.org/core/reports` in about 105 s. This is the first run of the whole package with task 6 alone: the planner ran it with tasks 6 and 5 built together, and task 6 alone on its own tests. When a test that counts SQL text fails here, hand back `STOPPED: <test name and message>`; do not change a count in this task.

Commit only when your prompt permits commits (lane rule 7); if it does not, name the four files in your report and the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/lookupbounds_test.go
git commit -m "perf(reports): a day comparison in a lookup's where bounds the stored day"
```

- [ ] **Step 7: Hand the 100k run to the driver**

Do not start it. Hand back:

`NEEDS-RUN: mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task6 && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task6 go test -run 'TestGLScale$/100k$' -timeout 20m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task6.log 2>&1`

What is needed from it: `scale-task6/gl-results.md` (the five medians) and `scale-task6/gl-plans.md`. The implementer's work ends here. Step 8 is done by the agent that receives the run's files.

Expected: no median moves by more than the run's variation (about 25%); the plan lines of `books_journal_lines` are the same as in `scale-task5c/gl-plans.md`. When a run with an Accounts filter (3, 4, 5) is more than 25% above its last record, hand back a second `NEEDS-RUN` of the same command (output to `scale-task6-b`); only when both are above, hand back `STOPPED: day bounds in a lookup's where slow run <n>` with the medians. Do not change the rule to pass.

- [ ] **Step 8: Record the change in the Books ledger, and commit Books (the agent that receives the run)**

Read `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` from the heading `#### Task 5c` to the end of the section, and append after the record of task 5c, in the same form (read from the run's files, never typed from this plan):

```markdown
#### Task 6: a day comparison in a lookup's where bounds the stored day (core `server/reports/lookup.go`, <date>)

A lookup's where compared a line's day through `substr()`. A comparison of the day with a day of the period now also bounds the stored column (`date >= lo AND date <= (hi || x'ff')`), next to the where. Alone it changes no plan line: the account is still bound by a range, so SQLite does not use `date` after it (task 5).

| run, 100k lines | before | after |
| --- | --- | --- |
| 1 all accounts, month | 583 ms | <median> |
| 2 all accounts, year | 1,601 ms | <median> |
| 3 busy account, month | 240 ms | <median> |
| 4 small account, month | 122 ms | <median> |
| 5 three accounts, year | 824 ms | <median> |

Plan, the detail statement of run 5, reads of `books_journal_lines`: <the plan lines and their counts, before and after>.
```

Where each `<…>` comes from:
- `<median>`: `scale-task6/gl-results.md`, the row of that run, the column `median of 5`, written in ms as the earlier records have it.
- The plan lines: `scale-task6/gl-plans.md` and `scale-task5c/gl-plans.md`, under the heading `## 100k lines, run 5 three accounts, year`, the statement with 201 rows (`### statement <n>: <time>, <rows> rows, <chars> chars`). List them with `command grep -n -E '^## 100k lines, run 5|^### statement|books_journal_lines' <file>` and count each distinct line under that statement.
- The "before" column is the task 5c record's "after" column: copy it from the ledger, and correct the figures above when they differ.

Commit only when your prompt permits commits; if it does not, the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(books): record the lookup day bounds at 100k lines"
```

---

### Task 5: A lookup's link bound is an equality where the looked-for value is known (core)

**Why:** `linkedEq` bounds the stored link column by a range (`col >= COALESCE(V, '') AND col <= COALESCE(V, x'ff')`) so that a blank V (a blank or hidden record, the Total row) reads the rows whose link is blank or hidden. Under a range SQLite uses only the link column of the index `(account, date)`; with `col = V` it uses the day bound of task 6 too: `SEARCH … (account=? AND date<?)`. Measured by text replacement at 100k lines, before task 5c: with task 6, statement 6 of run 5 goes from 870 to 800 ms and of run 3 from 208 to about 170 ms. `col = V` is wrong where V is blank, and no single indexable form covers both, so each part of the lookup is written in both forms inside a `CASE` on V. The grouped CTE of the outline is not built.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/formula.go` (the type `storedLink`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookup.go` (imports, `lookupIn`, the struct `lookupCtx`, `(*lookupCtx).path`, `linkedEq`, a new type and function before `whereProber`)
- Modify (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/ledger_guard_test.go:44-65` and `:106-108` (three text counts)
- Create (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/lookupeq_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (step 8)

**Interfaces:**
- Consumes: task 6 (`expr.bound`, the day bound in `where`); `linkedEq(n *Node, args []expr) string`, `pickedLookup(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr, at string) (expr, error)`, `readsLookedUpRow`, `(*scope).oneValue` (`lookup.go`); `hoistLookupParts` (`detail.go:477`: the parts of a lookup a statement computes once are `unguarded` or `sql`, `mixed`, `cur.sql`, `shown`). Test helpers: `withLedgerScale`, `withLedgerLines`, `runLedger`, `ledgerRuns`, `ledgerPages`, `ledgerCanonical`, `ledgerID`, `ledgerPeriod`, `z.recWithID` (`ledger_scale_test.go`), `ledgerAccounts` (`bounds_test.go`), `observeReads`, `rawStatement`, `readPool` (`statement_test.go`), `queryPlan`, `balanceSpec` (`balance_shape_test.go`), `requireSameAsLegacy`.
- Produces: `legacySQL.rangeLinkBounds bool`; `type linkEq struct{ rng, eq, v string }`; `storedLink.eqs *[]linkEq`; `lookupCtx.eqs *[]linkEq`; `knownLinks(rng expr, eqs []linkEq, where string, pick func(where string) (expr, error)) (expr, error)`; test helper `knownLinkSearches(t *testing.T, z *zoo, seen []readStatement) (total, least int)`. Rule: an `eq` of a one-hop link path of the looked-up row with a leaf that reads nothing of that row (a group record, a column, an outer path, a param, a constant) gives the lookup a second form; a value that is a formula or a lookup keeps the range alone. `expr.scan` (the layout's one-read form) stays the range form.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/lookupeq_test.go`:

```go
package reports

import (
	"strings"
	"testing"
)

// knownLinks writes each part of a lookup as a CASE over its two forms, tests
// only the values whose bound the where holds, and leaves a lookup with no
// link bound as it is.
func TestKnownLinksWritesEachPartInTwoForms(t *testing.T) {
	form := func(w string) expr {
		return expr{
			sql: "(CASE WHEN M(" + w + ") THEN NULL ELSE S(" + w + ") END)", unguarded: "S(" + w + ")", mixed: "M(" + w + ")",
			typ: Money, cur: currency{sql: "C(" + w + ")", id: "lk"}, scan: "SCAN " + w,
		}
	}
	eqs := []linkEq{{rng: "R1", eq: "E1", v: "a.rc1"}, {rng: "gone", eq: "X", v: "a.rc9"}, {rng: "R2", eq: "E2", v: "a.rc2"}}
	got, err := knownLinks(form("R1 AND R2 AND d"), eqs, "R1 AND R2 AND d", func(where string) (expr, error) {
		return form(where), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	either := func(part string) string {
		return "(CASE WHEN a.rc1 IS NOT NULL AND a.rc2 IS NOT NULL THEN " + part + "(E1 AND E2 AND d) ELSE " + part + "(R1 AND R2 AND d) END)"
	}
	for _, c := range []struct{ name, got, want string }{
		{"the value before the check", got.unguarded, either("S")},
		{"the check", got.mixed, either("M")},
		{"the currency", got.cur.sql, either("C")},
		{"the shown form", got.shown, ""},
		{"the one-read form", got.scan, "SCAN R1 AND R2 AND d"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
	if !strings.HasPrefix(got.sql, "(CASE WHEN a.rc1 IS NOT NULL AND a.rc2 IS NOT NULL THEN (CASE WHEN M(E1 AND E2 AND d) THEN NULL") {
		t.Errorf("the value: %q", got.sql)
	}
	if got.typ != Money || got.cur.id != "lk" {
		t.Errorf("the type and the currency's name changed: %v, %q", got.typ, got.cur.id)
	}
	plain := expr{sql: "S", typ: Number}
	same, err := knownLinks(plain, nil, "d", func(string) (expr, error) {
		t.Fatal("a lookup with no link bound was built twice")
		return expr{}, nil
	})
	if err != nil || same.sql != "S" {
		t.Fatalf("a lookup with no link bound: %q, %v", same.sql, err)
	}
}

// knownLinkSearches counts, over the statements seen, the plan steps that
// read the ledger lines of one account before a day (the equality form with
// its day bound) and the least such count of a statement.
func knownLinkSearches(t *testing.T, z *zoo, seen []readStatement) (total, least int) {
	t.Helper()
	db := readPool(t, z)
	least = -1
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		n := 0
		for _, step := range queryPlan(t, db, raw, args) {
			if strings.Contains(step, "zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date<?)") {
				n++
			}
		}
		total += n
		if least < 0 || n < least {
			least = n
		}
	}
	return total, least
}

// The beginning balance of a section is a sum lookup of one account's lines
// before the period. Each statement of the run reads it for a known account
// by the account and the day, so by the lines before the period only; with
// the switch on, no statement does (the range on the account leaves the day
// out of the search).
func TestALookupOnAKnownLinkSearchesTheLinkAndTheDay(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	opts := Options{Today: "2025-07-15", ExpandAll: true}
	seen := observeReads(t)
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if len(*seen) == 0 {
		t.Fatal("no statement was read")
	}
	// The value, the two-currency check and the currency: three reads.
	if _, least := knownLinkSearches(t, z, *seen); least < 3 {
		t.Fatalf("a statement reads the lines by account and day %d times, want 3 or more", least)
	}
	legacySQL.rangeLinkBounds = true
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	*seen = nil
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if total, _ := knownLinkSearches(t, z, *seen); total != 0 {
		t.Fatalf("with the switch on, %d reads by account and day, want none", total)
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, ".account]] = ") {
			t.Fatalf("with the switch on, a statement bounds the account by an equality:\n%s", st.sql)
		}
	}
}

// Every run of the ledger fixture returns the same pages with the two forms
// and with the range alone: on a plain and a mixed book, with lines whose
// account is blank (their section's record is blank, as the Total row's is,
// so they take the range form), and for a reader who may list no account.
func TestKnownLinksKeepTheResult(t *testing.T) {
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for i, day := range []string{"2025-01-10", "2025-04-10", "2025-04-11"} {
			z.recWithID(t, ledgerID("blnk", i), "zoo_ledger_lines", map[string]any{
				"entry": ledgerID("entr", i), "account": "", "date": day, "debit": 700 + i, "currency": "USD", "position": 8, "posted": true,
			})
		}
		for _, r := range ledgerRuns() {
			legacySQL.rangeLinkBounds = true
			before := ledgerCanonical(t, z, ledgerPages(t, z, r))
			legacySQL.rangeLinkBounds = false
			if after := ledgerCanonical(t, z, ledgerPages(t, z, r)); after != before {
				t.Fatalf("%s (mixed %v): the pages changed\nbefore: %s\nafter:  %s", r.name, mixed, before, after)
			}
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15", ExpandAll: true},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.rangeLinkBounds)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:
`go test -run 'TestKnownLinks|TestALookupOnAKnownLink' -count=1 ./reports/`

Expected: the build fails (`undefined: linkEq`, `undefined: knownLinks`, `legacySQL.rangeLinkBounds undefined`). A build failure is not yet a behavioral red, so add the names alone (step 3) and run again.

- [ ] **Step 3: Add the switch, the type and a `knownLinks` that changes nothing**

In `core/server/reports/legacy.go`, after the field `noLookupDayBounds bool`:

```go
	// rangeLinkBounds writes a lookup whose where matches a link once, with
	// the link column bounded by a range, as before knownLinks added the form
	// with an equality.
	rangeLinkBounds bool
```

In `core/server/reports/lookup.go`, before the comment of `whereProber`, add the type and, for now, a function that returns the lookup as it is:

```go
// linkEq is one bound linkedEq wrote on a stored link column in a lookup's
// where, for a looked-for value V that reads nothing of the looked-up row:
// rng is the bound as a range, as the where holds it (no other text of the
// where is the same: it names the lookup's own scope and V); eq is the bound
// as an equality, right where V is not blank; v is V as the report's row
// reads it.
type linkEq struct{ rng, eq, v string }

func knownLinks(rng expr, eqs []linkEq, where string, pick func(where string) (expr, error)) (expr, error) {
	return rng, nil
}
```

Run the command of step 2 again. Expected: FAIL, with
- `TestKnownLinksWritesEachPartInTwoForms`: `the value before the check: "S(R1 AND R2 AND d)", want "(CASE WHEN a.rc1 IS NOT NULL AND a.rc2 IS NOT NULL THEN S(E1 AND E2 AND d) ELSE S(R1 AND R2 AND d) END)"`, and the same for the check, the currency and the value;
- `TestALookupOnAKnownLinkSearchesTheLinkAndTheDay`: `a statement reads the lines by account and day 0 times, want 3 or more`;
- `TestKnownLinksKeepTheResult`: PASS (the switch changes nothing yet).

- [ ] **Step 4: Collect the link bounds of a where**

In `core/server/reports/formula.go`, replace the type `storedLink` and its comment's last sentence:

```go
// storedLink is the stored relation column (col) a link path reads its
// value (sql) through, in the lookup scope sc; a copy of the expr whose sql
// changed no longer reads through it. eqs collects the bounds eq writes on
// col for the lookup the where belongs to (linkEq), nil when it keeps none.
type storedLink struct {
	col, sql string
	sc       *scope
	eqs      *[]linkEq
}
```

In `core/server/reports/lookup.go`:

In the struct `lookupCtx`, after `inWhere bool`:

```go
	// eqs collects, in a where, the link bounds eq writes for a value that
	// reads nothing of the looked-up row (linkEq); nil keeps none.
	eqs *[]linkEq
```

In `(*lookupCtx).path`, the line that sets `e.link` becomes:

```go
			e.link = storedLink{col: col, sql: e.sql, sc: l.sc, eqs: l.eqs}
```

(`(*lookupCtx).exists` builds its inner contexts without `eqs`: a link bound inside an `exists` keeps the range alone.)

In `linkedEq`, replace the five lines from `v := args[1-i].sql` through the `return fmt.Sprintf(…)` with:

```go
		v := args[1-i].sql
		free := !readsLookedUpRow(n.Args[1-i])
		if free {
			v = a.link.sc.oneValue(v)
		}
		rng := fmt.Sprintf("(%s IS %s AND %s >= COALESCE(%s, '') AND %s <= COALESCE(%s, x'ff'))", a.sql, v, a.link.col, v, a.link.col, v)
		// The same bound as an equality, for the rows where the value is known
		// (knownLinks). A value that reads the looked-up row cannot be tested
		// outside the lookup, so its bound stays a range. A group record reads
		// the report's row, though readsLookedUpRow counts its path as a read
		// of the looked-up row and the range reads it in place. Only a leaf
		// (a record, a column, an outer path, a param, a constant) gets the
		// second form: the test outside the lookup reads the value once more,
		// which a formula or a lookup would compute again.
		if other := n.Args[1-i]; a.link.eqs != nil && other.Op == "" && (free || other.Record != "") {
			*a.link.eqs = append(*a.link.eqs, linkEq{
				rng: rng, v: args[1-i].sql,
				eq: fmt.Sprintf("(%s IS %s AND %s = %s)", a.sql, v, a.link.col, v),
			})
		}
		return rng
```

The range text `linkedEq` returns is the same as before, so the where is unchanged.

- [ ] **Step 5: Write the two forms**

In `core/server/reports/lookup.go`, add the import `"strings"`.

Replace the `knownLinks` of step 3 with:

```go
// knownLinks gives a lookup whose where bounds a link column by a range a
// second form, for the rows where every looked-for value is known. A range
// (col >= V AND col <= V) lets SQLite use only the link column of an index;
// an equality lets it use the columns after it too (a day bound, dayBound),
// so a lookup of one account's lines before a day reads those lines and not
// the account's whole history. The equality is wrong where V is blank (a
// group whose record is blank or hidden, the Total row): there the range
// matches every row, and the where's own comparison keeps those whose link is
// blank or hidden. So each part of the lookup becomes
//
//	CASE WHEN <every V> IS NOT NULL THEN <the part, equalities> ELSE <the part, ranges> END
//
// and SQLite computes only the form a row takes. rng is the lookup over where
// as it is; pick builds the lookup over another where. scan stays the range
// form: the layout computes it once in a CTE, with no row to test V on.
func knownLinks(rng expr, eqs []linkEq, where string, pick func(where string) (expr, error)) (expr, error) {
	var known []string
	for _, l := range eqs {
		if !strings.Contains(where, l.rng) {
			continue
		}
		where = strings.ReplaceAll(where, l.rng, l.eq)
		known = append(known, l.v+" IS NOT NULL")
	}
	if len(known) == 0 {
		return rng, nil
	}
	eq, err := pick(where)
	if err != nil {
		return expr{}, err
	}
	cond := strings.Join(known, " AND ")
	either := func(eq, rng string) string {
		if eq == "" || eq == rng {
			return rng
		}
		return fmt.Sprintf("(CASE WHEN %s THEN %s ELSE %s END)", cond, eq, rng)
	}
	out := rng
	out.sql, out.unguarded = either(eq.sql, rng.sql), either(eq.unguarded, rng.unguarded)
	out.mixed, out.shown = either(eq.mixed, rng.mixed), either(eq.shown, rng.shown)
	out.cur.sql = either(eq.cur.sql, rng.cur.sql)
	return out, nil
}
```

In `lookupIn`, replace

```go
	where, unknown := "1", ""
	if n.Where != nil {
		w, err := compileWhere(&lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}, n.Where, at)
```

with

```go
	where, unknown := "1", ""
	var eqs []linkEq
	if n.Where != nil {
		wc := &lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}
		if !legacySQL.rangeLinkBounds {
			wc.eqs = &eqs
		}
		w, err := compileWhere(wc, n.Where, at)
```

and replace

```go
	e, err := pickedLookup(outer, r, n, v, sc, where, out, at)
	if err != nil || unknown == "" || n.Pick == "count" {
```

with

```go
	e, err := pickedLookup(outer, r, n, v, sc, where, out, at)
	if err == nil {
		e, err = knownLinks(e, eqs, where, func(where string) (expr, error) {
			return pickedLookup(outer, r, n, v, sc, where, out, at)
		})
	}
	if err != nil || unknown == "" || n.Pick == "count" {
```

The lines after it (the where that was unknown: `e.guarded(…)`) read the merged `e.mixed` and `e.unguarded` and stay as they are. Both forms read one scope (`sc.render()`), so the one-row table of a value (`oneValue`) is written once in each form's FROM and serves both.

- [ ] **Step 6: Move the three text counts of the guard test**

`ledger_guard_test.go` counts SQL text. The lookup that matches the account is now written two times and a row runs one form, so three counts move. In `TestABalanceShapedReportComputesItsBeginningBalanceOncePerStatement`, replace

```go
		// Two lookups, on two levels (the total and the accounts) or in two
		// selects (the shown accounts and the lines).
		if n := strings.Count(st.sql, "(SELECT sum("); n > 4 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 4 (2 lookups, once per level)", kind, n)
		}
```

with

```go
		// Two lookups, on two levels (the total and the accounts) or in two
		// selects (the shown accounts and the lines). The lookup that matches
		// the account is written in two forms (knownLinks), and a row runs one
		// of them: 3 sums a level.
		if n := strings.Count(st.sql, "(SELECT sum("); n > 6 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 6 (2 lookups, one of them in two forms, once per level)", kind, n)
		}
```

and

```go
		// currency), all computed once per account.
		if byLine > 9 {
			t.Errorf("the %s statement reads a group record by %d subqueries on a line, want at most 9", kind, byLine)
		}
```

with

```go
		// currency), all computed once per account. With the two forms of
		// that lookup (knownLinks) each part also reads the account 2 times in
		// its equality form and once in the test that picks the form: 18 in
		// the text, of which a row runs 9 (a known account) or 12 (a blank one).
		if byLine > 18 {
			t.Errorf("the %s statement reads a group record by %d subqueries on a line, want at most 18", kind, byLine)
		}
```

In `TestACursorPageComputesItsBeginningBalanceOncePerStatement`, replace

```go
		if n := strings.Count(st.sql, "(SELECT sum("); n > 4 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 4", kind, n)
		}
```

with

```go
		if n := strings.Count(st.sql, "(SELECT sum("); n > 6 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 6", kind, n)
		}
```

No other test changes. `TestALinkLookupComputesTheValueItMatchesOnce` (`linkonce_test.go`) must pass as it is: its looked-for value is a lookup, which gets no second form. When it fails, the leaf rule of step 4 is wrong; do not change the test.

- [ ] **Step 7: Run the tests to see them pass, then the whole package, and commit core**

Run, in `core/server`:
`go vet ./reports/ && go test -run 'TestKnownLinks|TestALookupOnAKnownLink|TestABalanceShapedReportComputes|TestACursorPageComputes|TestALinkLookupComputes|TestALookupOnATwoHopLink' -count=1 ./reports/`
Expected: `ok  	tinycld.org/core/reports` (about 8 s).

Then once: `gofmt -l ./reports/ && go test -count=1 ./reports/`
Expected: `gofmt` prints nothing; `ok  	tinycld.org/core/reports` in about 105 s (the planner's run of this code: 105.7 s, no failure).

Commit only when your prompt permits commits; if it does not, name the five files in your report and the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/lookupeq_test.go core/server/reports/ledger_guard_test.go
git commit -m "perf(reports): a lookup on a known link searches the link and the day"
```

- [ ] **Step 8: Hand the 100k run to the driver, then record it in the Books ledger**

Do not start the run. Hand back:

`NEEDS-RUN: mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5 && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5 go test -run 'TestGLScale$/100k$' -timeout 20m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5.log 2>&1`

What is needed from it: `scale-task5/gl-results.md` and `scale-task5/gl-plans.md`. The implementer's work ends here; the rest of this step is done by the agent that receives the run's files.

Expected: in the detail statement of runs 3 and 5, plan lines `SEARCH … idx_books_jlines_account_date (account=? AND date>? AND date<?)` for the month-to-date lookup (next to the `(account>? AND account<?)` lines of the range form, which stay in the plan and run only for a blank account); runs 3 and 5 lower by about 5 to 20%. When a run with an Accounts filter is more than 25% above its last record, hand back a second `NEEDS-RUN` (output to `scale-task5-b`); only when both are above, hand back `STOPPED: the two forms of a link lookup slow run <n>` with the medians and that run's statement times. When the `(account=? AND date…)` lines of the lookup are missing, hand back `STOPPED: the equality form is not in the plan` with the plan lines of run 5's detail statement. Do not change the rule to pass.

Then read `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` from the heading `#### Task 6` to the end of the section, and append after the record of task 6, in the same form (read from the run's files, never typed from this plan):

```markdown
#### Task 5: a lookup on a known link searches the link and the day (core `server/reports/lookup.go`, <date>)

A lookup's where bounded the account column by a range, so that a blank account (the Total row, a section with no account) reads the lines with no account; under a range SQLite did not use the day. Each part of the lookup is now written in two forms: where the account is known, `account = V` with the day bound of task 6; where it is blank, the range as before. A row runs one form.

| run, 100k lines | before | after |
| --- | --- | --- |
| 1 all accounts, month | <task 6 after> | <median> |
| 2 all accounts, year | <task 6 after> | <median> |
| 3 busy account, month | <task 6 after> | <median> |
| 4 small account, month | <task 6 after> | <median> |
| 5 three accounts, year | <task 6 after> | <median> |

Plan, the detail statement of run 5, reads of `books_journal_lines`: <the plan lines and their counts, before and after>. The detail statement: run 5 <time before> to <time after>, run 3 <time before> to <time after>.
```

Where each `<…>` comes from:
- `<task 6 after>`: the "after" column of the task 6 record in the ledger.
- `<median>`: `scale-task5/gl-results.md`, the row of that run, the column `median of 5`, in ms.
- The plan lines and the statement times: `scale-task5/gl-plans.md` (after) and `scale-task6/gl-plans.md` (before), under `## 100k lines, run 5 three accounts, year` and `## 100k lines, run 3 busy account, month`, the statement with the most rows (`### statement <n>: <time>, <rows> rows, <chars> chars`). List them with `command grep -n -E '^## 100k lines, run (3|5)|^### statement|books_journal_lines' <file>`.

Commit only when your prompt permits commits; if it does not, the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(books): record the two forms of a link lookup at 100k lines"
```

---

## What is left (the next plan part)

- **5d. core · A detail row's lookups are computed for the page's rows only.** Not designed: its planner first times the parts of the final detail select (hand-off note `handoff-backend-planner-part4b.md`, section "First thing to measure"). After 5c the detail statement is 651.9 ms of run 5's 824 ms at 100k; tasks 6 and 5 take about 8% of it, so run 5 does not reach the bar at 500k without 5d.
- **7. books · Scale run and record.** The 500k `NEEDS-RUN`, the plan pins in `TestGLScaleBook`, the ledger record; `STOPPED` when a filtered run is over 1 s.
- Then the human's two decisions (large reports do not live-refresh; the Trial Balance, the Balance Sheet and the Transaction Detail reach the same bar).

Facts for the next planner (built by this part's planner, 2026-11-19, on core 298829bd):

- Tasks 6 and 5 were built together in an overlay: every new test passes, the whole core `reports` package passes (105.7 s) with the three counts of step 6 moved, and the Books General Ledger tests (`go test -run 'GeneralLedger|TestGL' .` in `books/server`, 10 s) pass against it. Task 6 alone was run on its own tests only. Scratch: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part5-scratch/` (`formula.go`, `lookup.go`, `legacy.go`, `ledger_guard_test.go`, the two new test files, `overlay.json`).
- The zoo plan with both tasks, the level statement of the ledger fixture: `SEARCH zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date<?)` three times, each followed by the range form's `(account>? AND account<?)`. No time was measured.
- `readsLookedUpRow` is true for a group record leaf with a path (`{"record":…,"path":[…]}`: `n.Path != nil`), so `linkedEq` reads a record three times in place and never through `oneValue`. Task 5 names the record leaf beside `free`; it does not change `readsLookedUpRow`.
- A lookup inside a layout keeps the range form (`expr.scan`); a layout report (the Balance Sheet) gains nothing from task 5.
