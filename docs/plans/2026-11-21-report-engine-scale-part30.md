# Report Engine Scale, Part 30 (task 9e-2a): A Rule That Reads No Row Is Answered Once Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A report statement no longer holds a list rule that gives one answer for every row: core asks that rule one time for each run, in its own small statement, and the statement reads every row of the table or none.

**Architecture:** `ruleSelect` (core `server/reports/rules.go`) already knows a rule that reads no row of its collection (`readsNoRow`) and writes it as `SELECT t.* FROM t WHERE EXISTS (<the rule's joins on a one-row table>)`. SQLite copies a CTE at every reference, so the shipped Balance Sheet prepares that EXISTS 3,172 times (205 ms of a 310 ms prepare). This plan adds `ruleFormOf`, which also returns the probe `SELECT EXISTS (…)` when the EXISTS select does not name the collection's own table. `stmt.with` asks the probe through the run's read transaction (`askRules`, one answer for each distinct probe text and bound values, kept in the runner for that one run) and writes the rule CTE as `SELECT t.* FROM t` (passes) or `SELECT t.* FROM t WHERE 0` (refused). `stmt.ruleReads` keeps the rule in full, so a run's live topics do not change.

**Tech Stack:** Go 1.26, PocketBase (`core.RecordFieldResolver`, `search.FilterData`, dbx), SQLite (modernc). Core module `tinycld.org/core`, package `reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/9e2-investigation.md` (section 2 "Cost drivers" and section 4 "Sketch of 9e-2"), and the human's decision of 2026-11-21 (copied in Global Constraints). The unit's outline: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

## Global Constraints

- The human's design (2026-11-21), verbatim: "core answers a list rule that reads no row ONCE, in its own small statement (bound values only — CLAUDE.md: bind every value in SQL), and the big statement gets a plain rule CTE (`SELECT t.* FROM t` when the rule passes, `… WHERE 0` when it fails); stmt.ruleReads keeps the full rule so live topics do not change."
- This is an access-control path. The tests must prove: a reader the rule refuses gets no rows (and the same error or empty result as before) for every rule kind that `readsNoRow` accepts; a rule that reads a row is NOT taken out of the big statement; a rule whose answer depends on the auth user is answered for that user only (no answer shared across users or across runs); live topics are unchanged. One test is on the expanded statement size or the prepare count of the Balance-Sheet-shaped zoo report.
- A probe that fails gives an error, never rows.
- Core names no package: no Books name, slug, collection or route in any core file. The fixture is the zoo.
- Every value in SQL is bound. The probe is the rule's own EXISTS select with the rule's own params; this plan writes no new value into SQL text. `WHERE 0` is the human's wording.
- No hosting word in core or Books.
- The implementer starts no scale run and no measure on the 100k book: both are a `NEEDS-RUN` for the driver (step 9).
- Do not push. Commits name no assistant and link no session.
- Paths: core is `/Users/nas/tmp/tinycld/books-purchases/tinycld`, Books is `/Users/nas/tmp/tinycld/books-purchases/books`. Both on branch `books/p5-report-engine-scale`. Tree when this plan was written: core 255a59b8, books c6b0245f (Books moved to 712cb7d5 before the review; the ledger file's last record is the same).

## Review Focus

Each line has its test in Task 1 (the test's name is in brackets).

1. A grant taken away between two runs of one reader: the second run reads nothing, because no answer outlives its run [`TestARuleAnswerBelongsToOneRunAndOneReader`].
2. The probe fails (a database fault, the run time limit): the run fails and sends no statement; the reader is not shown rows [`TestAFailedRuleProbeFailsTheRun`, `TestARuleProbeIsUnderTheRunTimeLimit`]. The probe is asked through the run's own read transaction [`TestARuleAnswerBelongsToOneRunAndOneReader`].
3. A rule that names its own collection (a comparison with every joined row, `@request.body.x:changed` beside a grant, a join of the collection to itself), or a collection whose name is the start of the grant collection's name: such a rule stays in the statement and its rows match the list API [`TestEveryRuleKindReadsTheListAPIsRows`, `TestARuleThatReadsItsRowStaysInTheStatement`, `TestNamesTable`].
4. A reader the rule refuses keeps the topics of the grant collection, so a grant given later reaches the open report [`TestAnAnsweredRuleKeepsTheRunsTopics`].
5. A read outside a run (a param's options) and a union source's parts: each asks for itself, and a refused reader gets no option and no row of the part [`TestParamOptionsUnderAnAnsweredRule`, `TestAUnionPartsRuleIsAnswered`].

## Facts the planner measured (do not find again)

- PocketBase gives a rule's placeholders new random names at every build (`{:fhI2qYjuiCH}` then `{:ff2OQYAI93K}` for the same rule). A cache key made of the probe text as built never matches a second build. `probeKey` numbers the placeholders in the order of the text and adds the bound values.
- The rule SQL holds dbx marks (`[[alias.column]]`, `{{table}}`) beside quoted names. `namesTable` therefore tests the bare name as a whole word.
- The comparison with every joined row writes `` `__mm_<collection>`.`id` = `<collection>`.`id` `` in the EXISTS select; `@request.body.x:changed` writes `[[<collection>.x]]`. Both name the collection, so both keep the EXISTS form. `@request.body.x:isset` is a constant and reads no row.
- Built once by the planner through a Go overlay (the trees were not changed): `go vet .` clean; the whole core `reports` package `ok` in 153 s; `go build ./...` in `core/server` clean; the three Books Go shards (`books/.github/scripts/go-test-shard.sh <n> 3`, with `GOFLAGS=-overlay=…`) `ok` in 129 s, 121 s and 156 s. No existing core or Books test changed.
- Prepare of the shipped Balance Sheet's long statement on the 10k scale book, 20 repetitions of `EXPLAIN`, load 12 (not quiet): before 324 ms / 315 ms (Accrual / Cash; 49,666 opcodes; 5 `__rule` in the text), after 106 ms / 106 ms (36,570 opcodes; 0 `__rule`). A run sends one more statement (the probe): 7 distinct statements became 8.
- The zoo Balance Sheet (`benchBalanceSheet`, 40 lines): 65,339 bytes; 231 copies of a rule CTE in the expanded statement; expanded 282,349 bytes (336,172 with the rule in each copy); 10,161 opcodes (10,478 with the rule). The zoo's grant rule has one join and no `ON`, so its gain is smaller than the shipped report's.
- The planner's files (the four changed core files, the test, the logs, the scratch measure): `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part30-scratch/`.

---

### Task 1 (9e-2a): core answers a rule that reads no row once for each run

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/ruleonce_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rules.go` (imports; `ruleSelect` at line 36; `unionSelect` at line 125)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/stmt.go` (the `stmt` struct at line 20; `ruleRead` at line 155; `with` at line 184)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (the `runner` struct at line 329; `Run` at line 422; `readRows` at line 655; `query` at line 696)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/options.go` (the two `readRows` calls, lines 410 and 478)
- Modify (step 10, after the driver's runs): `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (section "Performance", a new record after "All bars at 500k")

Match each edit by its text, not by its line number: another part of this unit may have moved the lines.

**Interfaces:**
- Consumes (they exist; read them before use): `ruleSelect(app core.App, col *core.Collection, info *core.RequestInfo) (string, dbx.Params, error)`; `type ruleRead struct { sql string; params dbx.Params }` (`stmt.go`); `ruleNames.readsNoRow() bool`; `mergeParams(dst, src dbx.Params) error`; `mapRunError(err error) error` and `runLimits.timeout` (`run.go`); test helpers `newZoo`, `withBalanceZoo`, `balanceSheetZoo`, `benchBalanceSheet`, `measureRun`, `observeReads`, `rawStatement`, `readPool`, `ruleIDs`, `ruleSQL`, `entryLinkSpec`, `z.run`, `z.try`, `z.as`, `z.id`, `z.rec`, `z.save`, `z.user`, `z.mux`, `z.listIDs`, `z.options`, `optionLabels`, `rule`.
- Produces:
  - `type ruleForm struct { ruleRead; probe string; table string }` — the rule in full, and the probe (empty when the rule stays in its statement).
  - `type ruleAsk func(probe string, params dbx.Params) (bool, error)`
  - `func ruleFormOf(app core.App, col *core.Collection, info *core.RequestInfo) (ruleForm, error)`
  - `func (f ruleForm) sent(ask ruleAsk) (ruleRead, error)`
  - `func namesTable(sql, table string) bool`
  - `var ruleAnswer func(ctx context.Context, db dbx.Builder, probe string, params dbx.Params) (bool, error)` — a test wraps it.
  - `func askRules(ctx context.Context, db dbx.Builder, answers map[string]bool) ruleAsk`
  - `func probeKey(probe string, params dbx.Params) string`
  - `func unionForms(app core.App, src *Source, info *core.RequestInfo, ask ruleAsk) (full, sent ruleRead, err error)`; `unionSelect` keeps its signature and returns the rule in full.
  - `stmt.ask ruleAsk`; `func (s *stmt) ruleRead(name string) (full, sent ruleRead, err error)` (the signature changes; `with` is its one caller).
  - `runner.ruleAnswers map[string]bool`
  - `readRows(ctx, app, db, s, ctes, body, topics, answers map[string]bool)` — one new last parameter.
  - `ruleSelect` keeps its signature and its result (the rule in full): `check.go` and the existing tests call it and do not change.
  - Test helpers in `ruleonce_test.go`: `observeProbes(t) *[]ruleProbe`, `sentRule(t, z, collection, user) (string, []string)`, `ruleKindsZoo(t) *zoo`, `detailRows(res) int`, `withList`, `expandedSize`, `ruleCopies`, `opcodes`.

- [ ] **Step 1: Write the failing tests**

Create `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/ruleonce_test.go` with exactly this content:

```go
package reports

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// ruleProbe is one probe a run asked, what it was asked through, and its
// answer.
type ruleProbe struct {
	db     dbx.Builder
	sql    string
	params dbx.Params
	ok     bool
}

// observeProbes collects every rule probe until the test ends.
func observeProbes(t *testing.T) *[]ruleProbe {
	t.Helper()
	var seen []ruleProbe
	saved := ruleAnswer
	ruleAnswer = func(ctx context.Context, db dbx.Builder, probe string, params dbx.Params) (bool, error) {
		ok, err := saved(ctx, db, probe, params)
		seen = append(seen, ruleProbe{db: db, sql: probe, params: params, ok: ok})
		return ok, err
	}
	t.Cleanup(func() { ruleAnswer = saved })
	return &seen
}

// sentRule is the rule CTE of collection as a statement of user holds it
// (stmt.with, with the rules asked as a run asks them), and the ids it reads.
func sentRule(t *testing.T, z *zoo, collection, user string) (string, []string) {
	t.Helper()
	s := newStmt(z.app, z.as(user))
	s.ask = askRules(context.Background(), z.app.DB(), nil)
	name := s.rule(collection)
	with, err := s.with()
	if err != nil {
		t.Fatalf("%s as %s: %v", collection, user, err)
	}
	ids := []string{}
	if err := z.app.DB().NewQuery(with + "SELECT id FROM " + name + " ORDER BY id").Bind(s.params).Column(&ids); err != nil {
		t.Fatalf("%s as %s: %v\n%s", collection, user, err, with)
	}
	return with, ids
}

// ruleKindsZoo adds a collection for each kind of rule readsNoRow accepts
// that the zoo does not hold yet, each with one row, and the user cara, who
// has no grant row. The kinds: a grant in another collection (the ledger
// collections), a field of the reader's own user row (zoo_sites), both in one
// rule (zoo_kind_both), the request alone (zoo_kind_request, zoo_kind_nobody),
// a request header (zoo_kind_header), a comparison with every joined row
// (zoo_kind_every), which reads its row's id, a rule that joins its own
// collection (zoo_kind_self), and "@request.body.x:changed",
// which reads its row's column, alone (zoo_kind_changed) and beside a grant
// (zoo_kind_changed_grant).
func ruleKindsZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	withBalanceZoo(t, z)
	z.user(t, "cara")
	for name, r := range map[string]string{
		"zoo_kind_both":    `@request.auth.role = "member" && @collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true`,
		"zoo_kind_request": `@request.auth.id != ""`,
		"zoo_kind_nobody":  `@request.auth.id = "nobody"`,
		"zoo_kind_header":  `@request.headers.x_zoo = "open"`,
		"zoo_kind_every":   `@request.auth.id != "" && @collection.zoo_ledger_access.user = @request.auth.id`,
		"zoo_kind_self":    `@request.auth.id != "" && @collection.zoo_kind_self:other.note ?= "one"`,
		"zoo_kind_changed": `@request.auth.id != "" && @request.body.note:changed = false`,
		"zoo_kind_changed_grant": `@collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true` +
			` && @request.body.note:changed = false`,
	} {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = rule(r), rule(r)
		col.Fields.Add(&core.TextField{Name: "note"})
		z.save(t, col)
		z.rec(t, name, name, map[string]any{"note": "one"})
	}
	return z
}

// answeredKinds are the collections of ruleKindsZoo whose rule one answer
// serves; keptKinds are those whose rule stays in the statement.
var (
	answeredKinds = []string{"zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines", "zoo_sites", "zoo_kind_both"}
	keptKinds     = []string{
		"zoo_kind_request", "zoo_kind_nobody", "zoo_kind_header", "zoo_kind_every", "zoo_kind_self", "zoo_kind_changed", "zoo_kind_changed_grant",
		"zoo_keepers", "zoo_animals", "zoo_feedings", "users", "zoo_vault",
	}
)

// For every kind of rule readsNoRow accepts, and for the rules that read
// their row, a statement reads the rows the list API gives each reader and
// the rows the rule in full gives: a reader the rule refuses reads none.
func TestEveryRuleKindReadsTheListAPIsRows(t *testing.T) {
	z := ruleKindsZoo(t)
	mux := z.mux(t)
	// bob's grant cannot read; cara has no grant row; ann is made a guest for
	// the second pass, which the role rules refuse.
	check := func(stage string) {
		t.Helper()
		for _, user := range []string{"ann", "bob", "cara"} {
			for _, collection := range append(slices.Clone(answeredKinds), keptKinds...) {
				with, got := sentRule(t, z, collection, user)
				// The list API answers 403 for a null rule, where a report
				// reads no row.
				if collection == "zoo_vault" {
					if len(got) != 0 {
						t.Errorf("%s, as %s: a superuser-only collection gives %v", stage, user, got)
					}
					continue
				}
				if api := z.listIDs(t, mux, collection, user, ""); !reflect.DeepEqual(got, api) {
					t.Errorf("%s, %s as %s: the statement reads %v, the list API gives %v\n%s", stage, collection, user, got, api, with)
				}
				if full := ruleIDs(t, z, collection, user); !reflect.DeepEqual(got, full) {
					t.Errorf("%s, %s as %s: the statement reads %v, the rule in full gives %v", stage, collection, user, got, full)
				}
			}
		}
	}
	check("as built")
	ungranted := []string{
		"zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines", "zoo_kind_both", "zoo_kind_nobody", "zoo_kind_header",
		"zoo_kind_every", "zoo_kind_changed_grant",
	}
	refused := map[string][]string{"bob": ungranted, "cara": ungranted}
	for user, collections := range refused {
		for _, collection := range collections {
			if with, got := sentRule(t, z, collection, user); len(got) != 0 {
				t.Errorf("%s, whom the rule of %s refuses, reads %v\n%s", user, collection, got, with)
			}
		}
	}
	for _, collection := range []string{"zoo_ledger_lines", "zoo_sites", "zoo_kind_both", "zoo_kind_request", "zoo_kind_changed", "zoo_kind_changed_grant"} {
		if _, got := sentRule(t, z, collection, "ann"); len(got) == 0 {
			t.Errorf("ann reads no row of %s: the check proves nothing", collection)
		}
	}
	z.recs["ann"].Set("role", "guest")
	if err := z.app.Save(z.recs["ann"]); err != nil {
		t.Fatal(err)
	}
	check("ann a guest")
	for _, collection := range []string{"zoo_sites", "zoo_kind_both"} {
		if with, got := sentRule(t, z, collection, "ann"); len(got) != 0 {
			t.Errorf("ann, a guest, reads %s %v\n%s", collection, got, with)
		}
	}
}

// plainRule is a rule CTE that holds an answer: every row of its table, or
// none.
var plainRule = regexp.MustCompile("^WITH r0 AS \\(SELECT `(\\w+)`\\.\\* FROM `(\\w+)`( WHERE 0)?\\)\n$")

// A rule one answer serves leaves the statement: its CTE reads every row or
// no row, and the probe that answered it binds every value of the rule.
func TestARuleOneAnswerServesLeavesTheStatement(t *testing.T) {
	z := ruleKindsZoo(t)
	for _, collection := range answeredKinds {
		for user, passes := range map[string]bool{"ann": true, "bob": false} {
			if collection == "zoo_sites" {
				passes = true // both are members
			}
			probes := observeProbes(t)
			with, _ := sentRule(t, z, collection, user)
			m := plainRule.FindStringSubmatch(with)
			if m == nil || m[1] != collection || m[2] != collection {
				t.Fatalf("%s as %s: the rule CTE is not the table's rows:\n%s", collection, user, with)
			}
			if refused := m[3] != ""; refused == passes {
				t.Errorf("%s as %s: refused = %v, want %v:\n%s", collection, user, refused, !passes, with)
			}
			if len(*probes) != 1 || (*probes)[0].ok != passes {
				t.Fatalf("%s as %s: probes %+v, want one that answers %v", collection, user, *probes, passes)
			}
			probe := (*probes)[0]
			if !strings.HasPrefix(probe.sql, "SELECT EXISTS (SELECT (1) FROM (SELECT 1) `__rule` ") {
				t.Errorf("%s: the probe is not the rule on a one-row table:\n%s", collection, probe.sql)
			}
			if strings.Contains(probe.sql, z.id(user)) {
				t.Errorf("%s: the probe holds the reader's id in its text:\n%s", collection, probe.sql)
			}
			bound := false
			for _, v := range probe.params {
				bound = bound || v == z.id(user)
			}
			if !bound {
				t.Errorf("%s: the probe does not bind the reader's id: %v", collection, probe.params)
			}
		}
	}
}

// A rule that reads its row, a rule that joins nothing, and the comparison
// with every joined row (which reads the row's id) are never asked outside
// the statement: the statement holds the rule ruleSelect writes.
func TestARuleThatReadsItsRowStaysInTheStatement(t *testing.T) {
	z := ruleKindsZoo(t)
	names := regexp.MustCompile(`\{:\w+\}|\{\{__sm\w+\}\}|\[\[__sm\w+\.`)
	for _, collection := range keptKinds {
		probes := observeProbes(t)
		with, _ := sentRule(t, z, collection, "ann")
		if len(*probes) != 0 {
			t.Errorf("%s: the rule was asked outside the statement: %+v", collection, *probes)
		}
		// PocketBase names the placeholders anew at each build.
		want := "WITH r0 AS (" + ruleSQL(t, z, collection, "ann") + ")\n"
		if got := names.ReplaceAllString(with, "?"); got != names.ReplaceAllString(want, "?") {
			t.Errorf("%s: the statement holds\n%s\nwant the rule in full\n%s", collection, with, want)
		}
	}
	for collection, form := range map[string]string{
		"zoo_animals": " IN (SELECT", "zoo_feedings": " IN (SELECT", "zoo_kind_every": "WHERE EXISTS (", "zoo_kind_changed_grant": "WHERE EXISTS (",
		"zoo_keepers": "WHERE ", "zoo_vault": "1=0",
	} {
		if with, _ := sentRule(t, z, collection, "ann"); !strings.Contains(with, form) {
			t.Errorf("%s: the statement lost %q:\n%s", collection, form, with)
		}
	}
	// Through a run: zoo_feedings reads its row's animal.
	probes := observeProbes(t)
	seen := observeReads(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}]}`, Options{}, nil)
	if n := detailRows(res); n != 4 {
		t.Fatalf("ann reads %d feedings, want 4", n)
	}
	if len(*probes) != 0 {
		t.Errorf("the run asked a rule that reads its row: %+v", *probes)
	}
	held := false
	for _, st := range *seen {
		held = held || strings.Contains(st.sql, "FROM `zoo_feedings` WHERE +`zoo_feedings`.`id` IN (SELECT")
	}
	if !held {
		t.Error("no statement of the run holds the feedings' rule")
	}
}

func detailRows(res *Result) int {
	n := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			n++
		}
	}
	return n
}

// A run asks each distinct rule one time, for its own reader, and keeps no
// answer for the next run: a grant taken away or given shows in the next run.
func TestARuleAnswerBelongsToOneRunAndOneReader(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	probes := observeProbes(t)
	runAs := func(user string) int {
		t.Helper()
		seen := observeReads(t)
		n := detailRows(z.run(t, user, entryLinkSpec, Options{}, nil))
		if len(*seen) == 0 {
			t.Fatal("the run sent no statement")
		}
		for _, st := range *seen {
			if strings.Contains(st.sql, "__rule") || strings.Contains(st.sql, "zoo_ledger_access") {
				t.Fatalf("a statement of %s's run holds the grant rule:\n%s", user, st.sql)
			}
			if want := user != "ann"; strings.Contains(st.sql, " WHERE 0)") != want && strings.Contains(st.sql, "zoo_ledger_lines") {
				t.Fatalf("a statement of %s's run: refused = %v, want %v:\n%s", user, !want, want, st.sql)
			}
		}
		return n
	}
	// asked is the readers of the probes so far, by the id each one binds.
	asked := func() []string {
		out := make([]string, len(*probes))
		for i, p := range *probes {
			for _, name := range []string{"ann", "bob"} {
				for _, v := range p.params {
					if v == z.id(name) {
						out[i] = name
					}
				}
			}
		}
		return out
	}
	if n := runAs("ann"); n == 0 {
		t.Fatal("ann reads no line")
	}
	// The lines and the entries share the rule's text: one probe for both.
	if got := asked(); !slices.Equal(got, []string{"ann"}) {
		t.Fatalf("ann's run asked %v, want one probe of ann's", got)
	}
	if n := runAs("bob"); n != 0 {
		t.Fatalf("bob, whose grant cannot read, reads %d lines after ann's run", n)
	}
	if n := runAs("ann"); n == 0 {
		t.Fatal("ann reads no line after bob's run")
	}
	if got := asked(); !slices.Equal(got, []string{"ann", "bob", "ann"}) {
		t.Fatalf("three runs asked %v, want each run to ask for its own reader", got)
	}
	if err := z.app.Delete(z.recs["annLedger"]); err != nil {
		t.Fatal(err)
	}
	if n := runAs("bob"); n != 0 {
		t.Fatalf("bob reads %d lines", n)
	}
	// ann's grant is gone: her next run reads nothing. runAs expects her
	// statements to read every row, so this run is checked here.
	if n := detailRows(z.run(t, "ann", entryLinkSpec, Options{}, nil)); n != 0 {
		t.Fatalf("ann reads %d lines after her grant was deleted", n)
	}
	z.recs["bobLedger"].Set("can_read", true)
	if err := z.app.Save(z.recs["bobLedger"]); err != nil {
		t.Fatal(err)
	}
	if n := detailRows(z.run(t, "bob", entryLinkSpec, Options{}, nil)); n == 0 {
		t.Fatal("bob reads no line after his grant was given")
	}
	if got := asked(); !slices.Equal(got, []string{"ann", "bob", "ann", "bob", "ann", "bob"}) {
		t.Fatalf("six runs asked %v", got)
	}
	// Each probe is asked through its run's read transaction, so the answer
	// and the rows are of one state of the database; no two runs share one.
	txs := map[dbx.Builder]bool{}
	for i, p := range *probes {
		tx, ok := p.db.(*dbx.Tx)
		if !ok {
			t.Fatalf("probe %d was asked through %T, want the run's read transaction", i, p.db)
		}
		txs[tx] = true
	}
	if len(txs) != len(*probes) {
		t.Errorf("%d probes were asked through %d transactions, want one for each run", len(*probes), len(txs))
	}
}

// One map of answers never gives one reader another's: the key holds the
// values the probe binds, whatever PocketBase named them.
func TestAProbeKeyHoldsItsValues(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	lines, err := z.app.FindCollectionByNameOrId("zoo_ledger_lines")
	if err != nil {
		t.Fatal(err)
	}
	key := func(user string) string {
		f, err := ruleFormOf(z.app, lines, z.as(user))
		if err != nil || f.probe == "" {
			t.Fatalf("no probe: %v", err)
		}
		return probeKey(f.probe, f.params)
	}
	if key("ann") != key("ann") {
		t.Error("two builds of one rule for one reader have two keys")
	}
	if key("ann") == key("bob") {
		t.Error("ann and bob share a key")
	}
	if a, b := probeKey("SELECT {:a} = {:b}", dbx.Params{"a": "1", "b": 1}), probeKey("SELECT {:x} = {:y}", dbx.Params{"x": 1, "y": "1"}); a == b {
		t.Error("the text 1 and the number 1 share a key")
	}
	// With one shared map, as a run holds: each reader is asked and answered
	// for himself.
	answers := map[string]bool{}
	probes := observeProbes(t)
	for user, want := range map[string]bool{"ann": true, "bob": false} {
		f, err := ruleFormOf(z.app, lines, z.as(user))
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			ok, err := askRules(context.Background(), z.app.DB(), answers)(f.probe, f.params)
			if err != nil || ok != want {
				t.Errorf("%s: answer %v, %v, want %v", user, ok, err, want)
			}
		}
	}
	if len(*probes) != 2 {
		t.Errorf("%d probes for two readers asked two times each, want 2", len(*probes))
	}
}

// A probe that fails shows no rows: the run fails, and no later statement of
// the run is sent.
func TestAFailedRuleProbeFailsTheRun(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	fault := errors.New("the probe failed")
	saved := ruleAnswer
	ruleAnswer = func(context.Context, dbx.Builder, string, dbx.Params) (bool, error) { return false, fault }
	t.Cleanup(func() { ruleAnswer = saved })
	seen := observeReads(t)
	res, err := z.try(t, "ann", entryLinkSpec, Options{}, nil)
	if !errors.Is(err, fault) || res != nil {
		t.Fatalf("Run = %+v, %v, want the probe's error and no result", res, err)
	}
	if len(*seen) != 0 {
		t.Fatalf("the run sent %d statements after the failed probe", len(*seen))
	}
}

// A probe is under the run time limit: one that does not answer in time ends
// the run with ErrTimeout, and no statement is sent.
func TestARuleProbeIsUnderTheRunTimeLimit(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	limit, saved := runLimits.timeout, ruleAnswer
	runLimits.timeout = 20 * time.Millisecond
	ruleAnswer = func(ctx context.Context, _ dbx.Builder, _ string, _ dbx.Params) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(10 * time.Second):
			return true, nil
		}
	}
	t.Cleanup(func() { runLimits.timeout, ruleAnswer = limit, saved })
	seen := observeReads(t)
	res, err := z.try(t, "ann", entryLinkSpec, Options{}, nil)
	if !errors.Is(err, ErrTimeout) || res != nil {
		t.Fatalf("Run = %+v, %v, want ErrTimeout and no result", res, err)
	}
	if len(*seen) != 0 {
		t.Fatalf("the run sent %d statements after the probe ran out of time", len(*seen))
	}
}

// The topics of a run are those of the rule in full, for a reader the rule
// passes and for one it refuses: a refused reader's report still follows the
// grant collection, so a grant given later reaches it.
func TestAnAnsweredRuleKeepsTheRunsTopics(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	topicsOf := func(collections ...string) []string {
		out := make([]string, len(collections))
		for i, c := range collections {
			out[i] = changesignal.Topic(c)
		}
		slices.Sort(out)
		return out
	}
	// fullTopics is what the rules in full read, found as before the rules
	// were answered outside the statement: by explaining each rule select.
	fullTopics := func(user string, collections ...string) []string {
		var out []string
		for _, c := range collections {
			col, err := z.app.FindCollectionByNameOrId(c)
			if err != nil {
				t.Fatal(err)
			}
			sql, params, err := ruleSelect(z.app, col, z.as(user))
			if err != nil {
				t.Fatal(err)
			}
			read, err := changesignal.TopicsRead(context.Background(), z.app, sql, params)
			if err != nil {
				t.Fatal(err)
			}
			for _, topic := range read {
				if !slices.Contains(out, topic) {
					out = append(out, topic)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	sitesSpec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}]}`
	for _, user := range []string{"ann", "bob"} {
		got := z.run(t, user, entryLinkSpec, Options{}, nil).Topics
		if want := topicsOf("zoo_ledger_access", "zoo_ledger_entries", "zoo_ledger_lines"); !slices.Equal(got, want) {
			t.Errorf("%s: topics %v, want %v", user, got, want)
		}
		if want := fullTopics(user, "zoo_ledger_lines", "zoo_ledger_entries"); !slices.Equal(got, want) {
			t.Errorf("%s: topics %v, the rules in full read %v", user, got, want)
		}
		got = z.run(t, user, sitesSpec, Options{}, nil).Topics
		if want := topicsOf("users", "zoo_sites"); !slices.Equal(got, want) {
			t.Errorf("%s: sites topics %v, want %v", user, got, want)
		}
		if want := fullTopics(user, "zoo_sites"); !slices.Equal(got, want) {
			t.Errorf("%s: sites topics %v, the rule in full reads %v", user, got, want)
		}
	}
}

// A union's rule CTE holds the answer of a part whose rule one answer serves
// and the rule in full of a part whose rule reads its row; its topics are
// those of each part's rule in full.
func TestAUnionPartsRuleIsAnswered(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	RegisterSource(Source{
		Key: "zoo.entries_and_animals", Label: "Entries and animals",
		Parts: []SourcePart{
			{Collection: "zoo_ledger_entries", Columns: map[string]string{"name": "number"}},
			{Collection: "zoo_animals", Columns: map[string]string{"name": "name"}},
		},
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	if lookupSource(z.app, "zoo.entries_and_animals") == nil {
		t.Fatalf("the union is refused: %v", Refused(z.app)["zoo.entries_and_animals"])
	}
	spec := `{"version":4,"source":"zoo.entries_and_animals","columns":[{"id":"name","path":[{"field":"name"}]}]}`
	// ann: four entries, and the animals she owns or keeps (Leo, Kit, Zed).
	// bob: no entry (his grant cannot read), and Zed and Mo.
	for user, want := range map[string]int{"ann": 7, "bob": 2} {
		probes := observeProbes(t)
		seen := observeReads(t)
		res := z.run(t, user, spec, Options{}, nil)
		if n := detailRows(res); n != want {
			t.Errorf("%s reads %d rows of the union, want %d", user, n, want)
		}
		if len(*probes) != 1 {
			t.Errorf("%s: %d probes, want 1 (the entries' grant rule)", user, len(*probes))
		}
		for _, st := range *seen {
			if strings.Contains(st.sql, "__rule") {
				t.Errorf("%s: a statement holds the grant rule:\n%s", user, st.sql)
			}
			if !strings.Contains(st.sql, "FROM `zoo_animals` WHERE +`zoo_animals`.`id` IN (SELECT") {
				t.Errorf("%s: a statement lost the animals' rule, which reads its row:\n%s", user, st.sql)
			}
		}
		for _, c := range []string{"zoo_ledger_access", "zoo_ledger_entries", "zoo_animals", "zoo_keepers"} {
			if !slices.Contains(res.Topics, changesignal.Topic(c)) {
				t.Errorf("%s: topics %v lack %s", user, res.Topics, c)
			}
		}
	}
}

// A rule is asked outside its statement only when its filter does not name
// the collection's own table: the name as a whole word, in any of the forms
// dbx and PocketBase write it. A longer name that holds it is another table.
func TestNamesTable(t *testing.T) {
	for sql, want := range map[string]bool{
		"LEFT JOIN `zoo_ledger_access` `__collection_alias_me` WHERE [[__collection_alias_me.user]] = {:p}": false,
		"LEFT JOIN `zoo_ledger` `__collection_zoo_ledger`":                                                  true,
		"WHERE `__mm_zoo_ledger`.`id` = `zoo_ledger`.`id`":                                                  true,
		"WHERE [[zoo_ledger.note]] IS NOT NULL":                                                             true,
		"FROM {{zoo_ledger}}":                                                                               true,
		"WHERE zoo_ledger.id = 1":                                                                           true,
		"WHERE [[__collection_zoo_ledger.note]] = {:zoo_ledger_p}":                                          false,
	} {
		if got := namesTable(sql, "zoo_ledger"); got != want {
			t.Errorf("namesTable(%q) = %v, want %v", sql, got, want)
		}
	}
}

// grantedOptions lists the ledger lines of the picked accounts; its options
// are the ledger accounts, read under the grant rule.
const grantedOptions = `{"version":4,"source":"zoo.ledger_lines",
	"params":[{"id":"accounts","type":"choice","label":"Accounts","multiple":true,
		"optionsFrom":{"source":"zoo.ledger_accounts","value":[{"field":"id"}],"label":[{"field":"full_name"}]}}],
	"columns":[{"id":"pos","path":[{"field":"position"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"account"}]},{"param":"accounts"}]}}`

// A param's options are read outside a run, each statement asking for
// itself: a reader the grant rule refuses gets no option.
func TestParamOptionsUnderAnAnsweredRule(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	probes := observeProbes(t)
	seen := observeReads(t)
	if got := optionLabels(z.options(t, "ann", grantedOptions, OptionsRequest{Param: "accounts"})); len(got) != 8 || got[0] != "Bank" {
		t.Fatalf("ann's options = %v, want the 8 accounts from Bank", got)
	}
	if got := optionLabels(z.options(t, "bob", grantedOptions, OptionsRequest{Param: "accounts"})); len(got) != 0 {
		t.Fatalf("bob, whose grant cannot read, gets the options %v", got)
	}
	if got := optionLabels(z.options(t, "bob", grantedOptions, OptionsRequest{Param: "accounts", Values: []string{z.id("cash")}})); len(got) != 0 {
		t.Fatalf("bob gets the option of an id he names: %v", got)
	}
	if len(*seen) == 0 || len(*probes) == 0 {
		t.Fatalf("%d statements, %d probes: nothing was checked", len(*seen), len(*probes))
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, "__rule") {
			t.Errorf("an options statement holds the grant rule:\n%s", st.sql)
		}
	}
	answers := map[bool]int{}
	for _, p := range *probes {
		answers[p.ok]++
	}
	if answers[true] == 0 || answers[false] == 0 {
		t.Errorf("probe answers %v, want ann's true and bob's false", answers)
	}
}

// withList splits "WITH a AS (..),\nb AS MATERIALIZED (..)\n<body>" into its
// CTE bodies by name and the statement's body.
func withList(t *testing.T, sql string) (map[string]string, string) {
	t.Helper()
	rest, ok := strings.CutPrefix(sql, "WITH ")
	if !ok {
		t.Fatalf("the statement has no WITH:\n%.200s", sql)
	}
	head := regexp.MustCompile(`^(\w+) AS (?:MATERIALIZED )?\(`)
	bodies := map[string]string{}
	for {
		m := head.FindStringSubmatch(rest)
		if m == nil {
			t.Fatalf("no CTE at:\n%.200s", rest)
		}
		rest = rest[len(m[0]):]
		depth, quoted, end := 1, false, -1
		for i := 0; i < len(rest) && end < 0; i++ {
			switch c := rest[i]; {
			case c == '\'':
				quoted = !quoted
			case quoted:
			case c == '(':
				depth++
			case c == ')':
				if depth--; depth == 0 {
					end = i
				}
			}
		}
		if end < 0 {
			t.Fatalf("CTE %s does not close", m[1])
		}
		bodies[m[1]] = rest[:end]
		rest = rest[end+1:]
		if next, more := strings.CutPrefix(rest, ",\n"); more {
			rest = next
			continue
		}
		return bodies, strings.TrimPrefix(rest, "\n")
	}
}

var cteRead = regexp.MustCompile(`\b(?:FROM|JOIN) (\w+)\b`)

// expandedSize is the size of a statement with each read of a CTE (FROM or
// JOIN) replaced by the CTE's own expanded text: what SQLite prepares, since
// it copies a CTE at every reference.
func expandedSize(bodies map[string]string, text string, memo map[string]int) int {
	size := len(text)
	for _, m := range cteRead.FindAllStringSubmatch(text, -1) {
		body, ok := bodies[m[1]]
		if !ok {
			continue
		}
		n, done := memo[m[1]]
		if !done {
			n = expandedSize(bodies, body, memo)
			memo[m[1]] = n
		}
		size += n
	}
	return size
}

// ruleCopies is how many copies of a rule CTE (r<n>) the expanded statement
// holds: each read of a rule CTE, counted once for each copy of the CTE that
// reads it.
func ruleCopies(bodies map[string]string, text string, memo map[string]int) int {
	copies := 0
	for _, m := range cteRead.FindAllStringSubmatch(text, -1) {
		body, ok := bodies[m[1]]
		if !ok {
			continue
		}
		if ruleName.MatchString(m[1]) {
			copies++
			continue
		}
		n, done := memo[m[1]]
		if !done {
			n = ruleCopies(bodies, body, memo)
			memo[m[1]] = n
		}
		copies += n
	}
	return copies
}

var ruleName = regexp.MustCompile(`^r\d+$`)

// opcodes is the length of the program SQLite prepares for a statement.
func opcodes(t *testing.T, z *zoo, sql string, params dbx.Params) int {
	t.Helper()
	raw, args := rawStatement(t, sql, params)
	rows, err := readPool(t, z).Query("EXPLAIN "+raw, args...)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// SQLite copies a CTE at every reference, so the Balance-Sheet-shaped layout
// prepares its rule CTEs hundreds of times. With the grant rule answered by
// one probe, each copy is the table's rows and holds no join: the statement
// SQLite prepares is smaller, in expanded text and in program, than the same
// statement with the rule in each copy.
func TestABalanceSheetShapedStatementHoldsNoRule(t *testing.T) {
	z := balanceSheetZoo(t, 40)
	probes := observeProbes(t)
	m := measureRun(t, z, benchBalanceSheet())
	if len(*probes) != 1 {
		t.Fatalf("the run asked %d probes, want 1: its collections share one rule", len(*probes))
	}
	if strings.Contains(m.sql, "__rule") || strings.Contains(m.sql, "zoo_ledger_access") {
		t.Fatalf("the group statement holds the grant rule:\n%.2000s", m.sql)
	}
	bodies, body := withList(t, m.sql)
	table := regexp.MustCompile("^SELECT `(\\w+)`\\.\\* FROM `(\\w+)`$")
	// held is the statement as it was sent before: the rule in full in each
	// rule CTE.
	held, heldSQL, heldParams := map[string]string{}, m.sql, dbx.Params{}
	if err := mergeParams(heldParams, m.params); err != nil {
		t.Fatal(err)
	}
	rules := 0
	for name, text := range bodies {
		held[name] = text
		if !ruleName.MatchString(name) {
			continue
		}
		tm := table.FindStringSubmatch(text)
		if tm == nil || tm[1] != tm[2] {
			t.Fatalf("rule CTE %s is not its table's rows: %s", name, text)
		}
		col, err := z.app.FindCollectionByNameOrId(tm[1])
		if err != nil {
			t.Fatal(err)
		}
		full, params, err := ruleSelect(z.app, col, z.as("ann"))
		if err != nil {
			t.Fatal(err)
		}
		if err := mergeParams(heldParams, params); err != nil {
			t.Fatal(err)
		}
		rules++
		held[name] = full
		heldSQL = strings.Replace(heldSQL, name+" AS ("+text+")", name+" AS ("+full+")", 1)
	}
	if rules < 2 || !strings.Contains(heldSQL, "__rule") {
		t.Fatalf("%d rule CTEs put back, want the lines and the accounts", rules)
	}
	copies := ruleCopies(bodies, body, map[string]int{})
	if copies < 100 {
		t.Fatalf("the expanded statement holds %d copies of a rule CTE: the fixture no longer shows the cost", copies)
	}
	sentSize, heldSize := expandedSize(bodies, body, map[string]int{}), expandedSize(held, body, map[string]int{})
	sentOps, heldOps := opcodes(t, z, m.sql, m.params), opcodes(t, z, heldSQL, heldParams)
	t.Logf("statement %d bytes; %d copies of a rule CTE; expanded %d bytes (%d with the rule in each copy); %d opcodes (%d with the rule)",
		len(m.sql), copies, sentSize, heldSize, sentOps, heldOps)
	if saved := heldSize - sentSize; saved < copies*100 {
		t.Errorf("the expanded statement is %d bytes smaller than with the rule in each of %d copies, want at least 100 bytes for each", saved, copies)
	}
	if sentOps >= heldOps {
		t.Errorf("the statement prepares to %d opcodes, with the rule in each copy %d: want fewer", sentOps, heldOps)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -run 'TestEveryRuleKind' -count=1 .
```

Expected: FAIL at the build, with `undefined: ruleAnswer`, `s.ask undefined (type *stmt has no field or method ask)` and `undefined: askRules`.

- [ ] **Step 3: `rules.go` — the rule's forms, the probe and its answer**

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rules.go`, replace the import block with:

```go
import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)
```

Keep the doc comment of `ruleSelect` and add these lines at its end (after "a plain condition in place."):

```go
//
// ruleSelect is the rule in full. A statement of a run holds a rule that one
// answer serves as every row or no row (ruleFormOf, ruleForm.sent).
```

Replace the whole function `ruleSelect` (from `func ruleSelect(` to its closing brace, just before the comment `// ruleNames is PocketBase's rule resolver`) with:

```go
func ruleSelect(app core.App, col *core.Collection, info *core.RequestInfo) (string, dbx.Params, error) {
	f, err := ruleFormOf(app, col, info)
	if err != nil {
		return "", nil, err
	}
	return f.sql, f.params, nil
}

// ruleForm is a collection's rule select in full (ruleSelect) and, for a rule
// that one answer serves, the statement that gives the answer.
type ruleForm struct {
	ruleRead
	// probe is "SELECT EXISTS (<the rule on a one-row table>)", bound by the
	// rule's params: 1 when the reader's rule passes. It is set only for a
	// rule that reads no row, joined, and does not name the collection's own
	// table; every other rule has none and stays in its statement.
	probe string
	// table is the collection's table, quoted.
	table string
}

// ruleAsk answers a rule's probe for the statement being written: true when
// the reader's rule passes.
type ruleAsk func(probe string, params dbx.Params) (bool, error)

// sent is the rule select a statement holds. A rule with a probe is asked one
// time, outside the statement, and the statement reads every row of the table
// or none: SQLite copies a CTE at each reference, so a rule left in a
// statement that reads its CTE thousands of times is prepared thousands of
// times. With no ask (a caller that only builds), or for a rule with no
// probe, the statement holds the rule in full. An ask that fails gives no
// select: a reader is never shown rows on a failed answer.
func (f ruleForm) sent(ask ruleAsk) (ruleRead, error) {
	if f.probe == "" || ask == nil {
		return f.ruleRead, nil
	}
	ok, err := ask(f.probe, f.params)
	if err != nil {
		return ruleRead{}, err
	}
	sql := fmt.Sprintf("SELECT %s.* FROM %s", f.table, f.table)
	if !ok {
		sql += " WHERE 0"
	}
	return ruleRead{sql: sql, params: dbx.Params{}}, nil
}

// ruleFormOf builds the list rule of col for the reader of info.
func ruleFormOf(app core.App, col *core.Collection, info *core.RequestInfo) (ruleForm, error) {
	q := app.DB().Select("{{" + col.Name + "}}.*").From(col.Name)
	switch {
	case col.ListRule == nil:
		q.AndWhere(dbx.NewExp("1=0"))
	case *col.ListRule != "":
		resolver := &ruleNames{RecordFieldResolver: core.NewRecordFieldResolver(app, col, info, true)}
		expr, err := search.FilterData(*col.ListRule).BuildExpr(resolver)
		if err != nil {
			return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
		if resolver.readsNoRow() {
			// The joins of the rule hang on a one-row table in place of the
			// collection, and EXISTS keeps a row once however many joined rows
			// meet the rule, as DISTINCT did.
			inner := app.DB().Select("(1)").From("(SELECT 1) __rule").AndWhere(expr)
			if err := resolver.UpdateQuery(inner); err != nil {
				return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
			}
			if inner.Info().Distinct {
				inner.Distinct(false)
				built := inner.Build()
				table := app.DB().QuoteSimpleTableName(col.Name)
				form := ruleForm{ruleRead: ruleRead{
					sql:    fmt.Sprintf("SELECT %s.* FROM %s WHERE EXISTS (%s)", table, table, built.SQL()),
					params: built.Params(),
				}}
				// The two shapes of readsNoRow that still read the row name
				// the collection's table in the filter; they keep the EXISTS
				// form, which SQLite runs for each row.
				if !namesTable(built.SQL(), col.Name) {
					form.probe = "SELECT EXISTS (" + built.SQL() + ")"
					form.table = table
				}
				return form, nil
			}
		}
		// A rule that reads no row and joined nothing reaches here after the
		// first UpdateQuery, which added no join, so nothing is added twice.
		q.AndWhere(expr)
		if err := resolver.UpdateQuery(q); err != nil {
			return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
	}
	built := q.Build()
	if !q.Info().Distinct {
		return ruleForm{ruleRead: ruleRead{sql: built.SQL(), params: built.Params()}}, nil
	}
	// A rule that joined is a DISTINCT select, which SQLite cannot read in
	// place: it copies the rows first, and a subquery run for every report
	// row then reads that copy whole, with no index of the table. The ids
	// the rule allows, read once, keep the same rows (id is the key) and
	// leave the table itself to be read through its indexes. The unary +
	// keeps SQLite from driving a read by that id list, which would visit
	// every allowed row where an index of the table finds the few it needs.
	table := app.DB().QuoteSimpleTableName(col.Name)
	id := app.DB().QuoteSimpleColumnName("id")
	return ruleForm{ruleRead: ruleRead{
		sql:    fmt.Sprintf("SELECT %s.* FROM %s WHERE +%s.%s IN (SELECT %s FROM (%s))", table, table, table, id, id, built.SQL()),
		params: built.Params(),
	}}, nil
}

// namesTable reports whether sql names the table as a whole word, quoted or
// not. A longer name that holds it (zoo_ledger in zoo_ledger_access) is not
// the table.
func namesTable(sql, table string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(table) + `\b`).MatchString(sql)
}

// ruleAnswer runs a rule's probe through db and returns its answer; a test
// wraps it to see what a run asks.
var ruleAnswer = func(ctx context.Context, db dbx.Builder, probe string, params dbx.Params) (bool, error) {
	var ok bool
	err := db.NewQuery(probe).Bind(params).WithContext(ctx).Row(&ok)
	return ok, err
}

// askRules is the ruleAsk of the statements of one run: each distinct probe
// is run one time through db, under the run time limit, and answers holds
// what it gave. The answers belong to the one run that made the map (one
// reader, one read transaction): no run reads another's. With no map, each
// statement asks for itself.
func askRules(ctx context.Context, db dbx.Builder, answers map[string]bool) ruleAsk {
	if answers == nil {
		answers = map[string]bool{}
	}
	return func(probe string, params dbx.Params) (bool, error) {
		key := probeKey(probe, params)
		if ok, asked := answers[key]; asked {
			return ok, nil
		}
		ctx, cancel := context.WithTimeout(ctx, runLimits.timeout)
		defer cancel()
		ok, err := ruleAnswer(ctx, db, probe, params)
		if err != nil {
			return false, mapRunError(err)
		}
		answers[key] = ok
		return ok, nil
	}
}

var probeParam = regexp.MustCompile(`\{:(\w+)\}`)

// probeKey names a probe by its text and the values it binds, in the order
// the text reads them. PocketBase gives a rule's placeholders new names at
// every build, so the names are numbered here: the collections of one package
// that share a rule text then share one answer, and two readers never do (the
// reader's id is one of the values).
func probeKey(probe string, params dbx.Params) string {
	index := map[string]int{}
	var values []any
	text := probeParam.ReplaceAllStringFunc(probe, func(tok string) string {
		name := tok[2 : len(tok)-1]
		i, ok := index[name]
		if !ok {
			i = len(index)
			index[name] = i
			values = append(values, params[name])
		}
		return "{:" + strconv.Itoa(i) + "}"
	})
	return text + "\x00" + fmt.Sprintf("%#v", values)
}

```

Replace the whole of `unionSelect` and its doc comment (from `// unionSelect is a union source's rule CTE` to the closing brace, just before the comment `// unionPartCols is the select list`) with:

```go
// unionSelect is a union source's rule CTE in full: each part's rule select
// (ruleSelect, the part's own list rule), projected to the source's keys and
// joined by UNION ALL. A key a part does not map reads NULL there; a part's
// constants are bound, under names that hold the source key, so two unions in
// one statement never bind one name twice.
func unionSelect(app core.App, src *Source, info *core.RequestInfo) (string, dbx.Params, error) {
	full, _, err := unionForms(app, src, info, nil)
	return full.sql, full.params, err
}

// unionForms is a union source's rule CTE in full (unionSelect) and as a
// statement holds it: each part's rule as ruleForm.sent writes it for ask.
func unionForms(app core.App, src *Source, info *core.RequestInfo, ask ruleAsk) (full, sent ruleRead, err error) {
	keys := src.unionKeys()
	full.params, sent.params = dbx.Params{}, dbx.Params{}
	fulls := make([]string, 0, len(src.Parts))
	sents := make([]string, 0, len(src.Parts))
	for i, part := range src.Parts {
		col, err := app.FindCachedCollectionByNameOrId(part.Collection)
		if err != nil {
			return ruleRead{}, ruleRead{}, fmt.Errorf("reports: collection %q: %w", part.Collection, err)
		}
		form, err := ruleFormOf(app, col, info)
		if err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		partSent, err := form.sent(ask)
		if err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		if err := mergeParams(full.params, form.params); err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		if err := mergeParams(sent.params, partSent.params); err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		fulls = append(fulls, "SELECT "+unionPartCols(src, i, keys, full.params)+" FROM ("+form.sql+") p")
		sents = append(sents, "SELECT "+unionPartCols(src, i, keys, sent.params)+" FROM ("+partSent.sql+") p")
	}
	full.sql = strings.Join(fulls, "\nUNION ALL\n")
	sent.sql = strings.Join(sents, "\nUNION ALL\n")
	return full, sent, nil
}

```

`ruleNames`, `readsNoRow`, `unionPartCols`, `mergeParams` and `listRequestInfo` do not change.

- [ ] **Step 4: `stmt.go` — the statement asks, and keeps the rule in full for its topics**

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/stmt.go`, in the `stmt` struct replace

```go
	// ruleReads holds each rule CTE's own select and params, by CTE name, as
	// with rendered them, so a run finds the topics a rule reads (topics).
	ruleReads map[string]ruleRead
	n         int
}
```

with

```go
	// ruleReads holds each rule CTE's select in full and its params, by CTE
	// name, so a run finds the topics a rule reads (topics).
	ruleReads map[string]ruleRead
	// ask answers the rules that one answer serves (ruleForm.sent); readRows
	// sets it. Without it with writes every rule in full.
	ask ruleAsk
	n   int
}
```

Replace the method `ruleRead` and its comment

```go
// ruleRead is the select and params of one entry of s.rules.
func (s *stmt) ruleRead(name string) (string, dbx.Params, error) {
	if key, union := strings.CutPrefix(name, unionRulePrefix); union {
		src := lookupRegistered(key)
		if src == nil {
			return "", nil, fmt.Errorf("reports: union source %q is not registered", key)
		}
		return unionSelect(s.app, src, s.info)
	}
	col, err := s.app.FindCachedCollectionByNameOrId(name)
	if err != nil {
		return "", nil, fmt.Errorf("reports: collection %q: %w", name, err)
	}
	return ruleSelect(s.app, col, s.info)
}
```

with

```go
// ruleRead is the select and params of one entry of s.rules: in full, and as
// the statement holds it (ruleForm.sent).
func (s *stmt) ruleRead(name string) (full, sent ruleRead, err error) {
	if key, union := strings.CutPrefix(name, unionRulePrefix); union {
		src := lookupRegistered(key)
		if src == nil {
			return ruleRead{}, ruleRead{}, fmt.Errorf("reports: union source %q is not registered", key)
		}
		return unionForms(s.app, src, s.info, s.ask)
	}
	col, err := s.app.FindCachedCollectionByNameOrId(name)
	if err != nil {
		return ruleRead{}, ruleRead{}, fmt.Errorf("reports: collection %q: %w", name, err)
	}
	form, err := ruleFormOf(s.app, col, s.info)
	if err != nil {
		return ruleRead{}, ruleRead{}, err
	}
	sent, err = form.sent(s.ask)
	if err != nil {
		return ruleRead{}, ruleRead{}, err
	}
	return form.ruleRead, sent, nil
}
```

In `with`, replace

```go
			ruleSQL, params, err := s.ruleRead(name)
			if err != nil {
				return "", err
			}
			if err := mergeParams(s.params, params); err != nil {
				return "", err
			}
			sql = ruleSQL
			s.ruleReads[s.ruleOf[name]] = ruleRead{sql: ruleSQL, params: params}
```

with

```go
			full, sent, err := s.ruleRead(name)
			if err != nil {
				return "", err
			}
			if err := mergeParams(s.params, sent.params); err != nil {
				return "", err
			}
			sql = sent.sql
			// The rule in full, not as sent: a run's topics name what the
			// rule reads (the grant collection, the reader's user row) also
			// when the statement holds only the rule's answer.
			s.ruleReads[s.ruleOf[name]] = full
```

`stmt.go` still imports `dbx` (the `params` field and `ruleRead` type use it).

- [ ] **Step 5: `run.go` and `options.go` — a run holds its answers**

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go`, in the `runner` struct, after the field

```go
	ruleTopics map[string][]string
```

add

```go
	// ruleAnswers holds the answer of each rule probe the run asked
	// (askRules), so its statements ask each rule once.
	ruleAnswers map[string]bool
```

In `Run`, where the runner is built, replace `ruleTopics: map[string][]string{},` with

```go
ruleTopics: map[string][]string{}, ruleAnswers: map[string]bool{},
```

Replace the end of the comment of `readRows`, its signature and its first line

```go
// statement reads.
func readRows(ctx context.Context, app core.App, db dbx.Builder, s *stmt, ctes []string, body string, topics map[string][]string) ([]map[string]any, []string, error) {
	ctes = materializeShared(ctes, body)
```

with

```go
// statement reads. A rule that one answer serves is asked through db before
// the statement is written (askRules); answers is a run's map of them, nil
// for a read that is no part of a run.
func readRows(ctx context.Context, app core.App, db dbx.Builder, s *stmt, ctes []string, body string, topics map[string][]string, answers map[string]bool) ([]map[string]any, []string, error) {
	ctes = materializeShared(ctes, body)
	s.ask = askRules(ctx, db, answers)
```

The probe gets its own time limit inside `askRules`. Do not move the `context.WithTimeout` line of `readRows`: `readtx_test.go` changes `runLimits.timeout` from `observeRead`, which runs before it.

In `(rn *runner) query`, replace `body, rn.ruleTopics)` with `body, rn.ruleTopics, rn.ruleAnswers)`.

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/options.go`, in both calls of `readRows`, replace `s, nil, body, nil)` with `s, nil, body, nil, nil)`.

- [ ] **Step 6: Run the new tests to see them pass**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && gofmt -l . && go vet . && go test -run 'TestEveryRuleKind|TestARuleOneAnswer|TestARuleThatReadsItsRowStays|TestARuleAnswerBelongs|TestAProbeKey|TestAFailedRuleProbe|TestARuleProbeIsUnder|TestAnAnsweredRule|TestAUnionPartsRule|TestABalanceSheetShapedStatementHoldsNoRule|TestNamesTable|TestParamOptionsUnderAnAnsweredRule' -count=1 -v .
```

Expected: `gofmt -l` prints nothing, `go vet` prints nothing, twelve `--- PASS` lines and `ok`. `TestABalanceSheetShapedStatementHoldsNoRule` logs a line close to: `statement 65339 bytes; 231 copies of a rule CTE; expanded 282349 bytes (336172 with the rule in each copy); 10161 opcodes (10478 with the rule)`.

- [ ] **Step 7: Prove that the access tests catch a wrong change**

Each mutation is made on a COPY of the file, read through `go test -overlay`; the repository file is never changed (the Edit tool on the copy, no `git checkout`). For each: copy the file to your scratchpad, make the one change, write `{"Replace":{"<repo path of the file>":"<the copy>"}}` to a JSON file, run the command, and record the failing test names in your report. A mutation that fails no test is a finding: stop and report it. A build failure (`[build failed]`) is not a caught mutation: the mutation must compile.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -overlay <the json> -run 'TestEveryRuleKind|TestARuleOneAnswer|TestARuleThatReadsItsRowStays|TestARuleAnswerBelongs|TestAProbeKey|TestAFailedRuleProbe|TestARuleProbeIsUnder|TestAnAnsweredRule|TestAUnionPartsRule|TestParamOptionsUnderAnAnsweredRule' -count=1 .
```

| Mutation (one for each run) | The planner's forecast of the tests that fail (not run by the planner) |
|---|---|
| `rules.go`, `sent`: change `sql += " WHERE 0"` to `sql += " WHERE 1"` | `TestEveryRuleKindReadsTheListAPIsRows`, `TestARuleOneAnswerServesLeavesTheStatement`, `TestARuleAnswerBelongsToOneRunAndOneReader`, `TestAUnionPartsRuleIsAnswered`, `TestParamOptionsUnderAnAnsweredRule` |
| `run.go`, `readRows`: change `s.ask = askRules(ctx, db, answers)` to `s.ask = askRules(ctx, app.DB(), answers)` | `TestARuleAnswerBelongsToOneRunAndOneReader` (the probe is not asked through a transaction) |
| `rules.go`, `askRules`: change `context.WithTimeout(ctx, runLimits.timeout)` to `context.WithCancel(ctx)` | `TestARuleProbeIsUnderTheRunTimeLimit` (after 10 s) |
| `rules.go`, `askRules`: change `return false, mapRunError(err)` to `return false, err` | `TestARuleProbeIsUnderTheRunTimeLimit` |
| `rules.go`, `ruleFormOf`: change `if !namesTable(built.SQL(), col.Name) {` to `if true {` | `TestEveryRuleKindReadsTheListAPIsRows`, `TestARuleThatReadsItsRowStaysInTheStatement` (the probe of `zoo_kind_every` names a table the probe does not hold: an SQL error) |
| `rules.go`, `probeKey`: change the last line to `return text` | `TestAProbeKeyHoldsItsValues` |
| `rules.go`, `askRules`: delete the line `answers[key] = ok` | `TestAProbeKeyHoldsItsValues`, `TestARuleAnswerBelongsToOneRunAndOneReader` (more probes than runs) |
| `stmt.go`, `with`: add the line `full = sent` before `s.ruleReads[s.ruleOf[name]] = full` | `TestAnAnsweredRuleKeepsTheRunsTopics`, `TestAUnionPartsRuleIsAnswered` |
| `rules.go`, `askRules`: change `return false, mapRunError(err)` to `return true, nil` | `TestAFailedRuleProbeFailsTheRun` |

- [ ] **Step 8: Run the whole package, build the module, commit**

Run (about 150 s for the tests):

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go build ./... && cd reports && go vet . && go test -count=1 .
```

Expected: no output from the build and the vet, then `ok  	tinycld.org/core/reports`. The package's `TestMain` (`topics_check_test.go`) fails the package when a statement's topics miss a collection the whole statement reads: `ok` also proves that no statement lost a topic.

Then check that core names no package and commit:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && ! command grep -n -i -E 'books|hosting|tenant|saas' core/server/reports/ruleonce_test.go core/server/reports/rules.go core/server/reports/stmt.go && git add core/server/reports/ruleonce_test.go core/server/reports/rules.go core/server/reports/stmt.go core/server/reports/run.go core/server/reports/options.go && git commit -m "perf(reports): a list rule that reads no row is answered once for each run"
```

Expected: the grep prints nothing; one commit of five files. When the grep prints a line, no commit is made: remove the word and run the command again. Do not push.

- [ ] **Step 9: Hand the two measures to the driver**

Hand back these two lines exactly, and stop. Step 10 is for the agent the driver starts with the logs. Both need a quiet machine (`pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4).

```
NEEDS-RUN: mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2 && sysctl -n vm.loadavg > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2/load-prepare.txt && cd /Users/nas/tmp/tinycld/books-purchases/books/server && ZZ_PREPARE=1 go test -overlay /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part30-scratch/overlay-measure.json -run 'TestZZPrepare$' -count=1 -timeout 30m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2/prepare.log 2>&1
NEEDS-RUN: sysctl -n vm.loadavg > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2/load-scale.txt && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e2.log 2>&1
```

The first is the prepare measure of the shipped Balance Sheet on the 100k scale book: `EXPLAIN` of each long statement of the two Balance Sheet runs, read to its end, 20 times, as the investigation did. `overlay-measure.json` adds one scratch test file (`part30-scratch/zz_prepare_test.go`, never committed) to the Books test package and changes no other file, so it measures the committed core.

Needed from the first (`command grep 'statement' …/scale-task9e2/prepare.log`): two lines, `BS 1 accrual, as of statement <i> of <n>: <chars> chars, <opcodes> opcodes, <k> __rule; prepare median <ms> (min …, max …)` and the same for `BS 2 cash, as of`. Expected: `0 __rule`, about 36,570 opcodes, a median near 105 ms. Before this task (planner, 10k book, load 12): 324 ms and 315 ms, 49,666 opcodes, `5 __rule`. A median over 150 ms on a quiet machine, or a `__rule` count that is not 0: `STOPPED: the Balance Sheet statement still prepares in <ms>` with the two lines.

Needed from the second: PASS; `scale-task9e2/st-results.md` (every median; no run refused; the group rows and rows of each run as in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b3/st-results.md`, the last 100k round) and `scale-task9e2/st-plans.md`. A run over its bar, a refused run, or a changed row count: `STOPPED: run <name> is <ms> at 100k` (or the changed count) with the statement times; the bar is never lowered.

- [ ] **Step 10: The ledger record (Books), from the driver's logs**

In `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`, at the end of the file (after the record "All bars at 500k (2026-11-21)"), add the record below. Fill each `<…>` from the file named beside it; write no figure that is not in those files.

```markdown
#### Task 9e-2a: a list rule that reads no row is answered once for each run (core `server/reports/rules.go`, 2026-11-21)

The cause (`9e2-investigation.md`): SQLite copies a CTE at every reference, so the prepare cost of a statement follows its expanded text. The Balance Sheet statement expands to 4.03 MB and holds 3,172 copies of a rule CTE; each copy carried the grant rule as `EXISTS (… LEFT JOIN users … LEFT JOIN books_access …)`, about 205 ms of a 310 ms prepare. The rule reads no row of its collection: its answer is the same for every row.

The change (core): `ruleFormOf` returns the rule in full and, for a joined rule that reads no row and does not name its collection's table, the probe `SELECT EXISTS (…)`. A run asks each distinct probe one time through its read transaction (`askRules`; the key is the probe's text and bound values, the answers live in the runner of that one run) and the statement's rule CTE is `SELECT t.* FROM t`, or `SELECT t.* FROM t WHERE 0` for a refused reader. `stmt.ruleReads` keeps the rule in full, so a run's topics are the same. A rule that reads its row, the comparison with every joined row and `:changed` stay in the statement. A probe that fails fails the run.

The tests (core `ruleonce_test.go`): `TestEveryRuleKindReadsTheListAPIsRows`, `TestARuleOneAnswerServesLeavesTheStatement`, `TestARuleThatReadsItsRowStaysInTheStatement`, `TestARuleAnswerBelongsToOneRunAndOneReader`, `TestAProbeKeyHoldsItsValues`, `TestAFailedRuleProbeFailsTheRun`, `TestARuleProbeIsUnderTheRunTimeLimit`, `TestAnAnsweredRuleKeepsTheRunsTopics`, `TestAUnionPartsRuleIsAnswered`, `TestNamesTable`, `TestParamOptionsUnderAnAnsweredRule`, `TestABalanceSheetShapedStatementHoldsNoRule`.

Code: core <the commit of step 8, `git -C /Users/nas/tmp/tinycld/books-purchases/tinycld log --oneline -1 -- core/server/reports/rules.go`>.

Prepare of the long Balance Sheet statement (`EXPLAIN` read to its end, 20 times, the 100k book; `scale-task9e2/prepare.log`):

| run | before (10k book, load 12) | after: median (min to max) | opcodes before | opcodes after |
|---|---|---|---|---|
| BS 1 accrual, as of | 324 ms | <median (min to max) of the BS 1 line> | 49,666 | <opcodes of the BS 1 line> |
| BS 2 cash, as of | 315 ms | <median (min to max) of the BS 2 line> | 49,665 | <opcodes of the BS 2 line> |

The 100k round (`scale-task9e2/st-results.md`; before = `scale-task9b3/st-results.md`, the last 100k round, which did not hold tasks 28a and 29):

| lines | run | before | after |
|---|---|---|---|
<one row for each of the eleven runs of `st-results.md`: `| 100000 | <run> | <median in scale-task9b3> | <median in scale-task9e2> |`>

<One sentence: PASS or not, whether a run is refused, whether the rows and group rows are the same, and the 1-minute load at the start of each of the two runs (the first number in `scale-task9e2/load-prepare.txt` and in `scale-task9e2/load-scale.txt`).>

Not done here (each a later, larger step, from the investigation): one select for the currencies of all computed rows of a layout (about 100 ms to 66 ms on the copy); the `__probes` reads (66 ms to 56 ms); a staged run with the shared CTEs in temp tables (the floor, about 20 ms). The prepare cost of the other large statements (General Ledger, Transaction Detail, Trial Balance, Journal, Profit and Loss) is not measured yet: the human asked for that benchmark after this task.
```

Then commit in Books:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs(books): ledger record of task 9e-2a (a rule that reads no row is answered once)"
```

Do not push.
