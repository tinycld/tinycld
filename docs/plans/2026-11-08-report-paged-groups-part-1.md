# Report paged group rows and fold list (J1) — Part 1: the engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core's report engine pages the group rows of a grouped report (no 5,000-group cut) and takes a fold list (`options.collapse`).

**Architecture:** See the outline `docs/plans/2026-11-08-report-paged-groups.md` (Architecture, Rulings R1–R9). In short: a *paged grouped* run (grouped, no pivot, no `layout`, no nest) sends a page statement of level rows (each `o<L>` row with its sibling-ordinal path `__p<i>` and its order values `__s<L>_<j>`, filtered by a keyset after the cursor's group path and limited), a Total statement (`SELECT * FROM o0`), and the detail statement over the page's open innermost groups; Go assembles and cuts the page at `limit` counted rows (group, opening, detail, subtotal) and writes the cursor from the last counted row.

**Tech Stack:** Go 1.26, PocketBase, SQLite.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/journal-report-design.md` §4, with the brief `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-journal-report/cp/cp0.md`. Verdict MAJOR; lane purchases does NOT take `exclusive`; the core merge needs the human's approval.

**Verdict: MAJOR.** Lane purchases does NOT take `exclusive`; the core merge needs the human's approval (brief D4).

Every path is relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld`. Go tests: `cd core/server && go test ./reports/ -run '<Regex>' -count=1`. Do not commit (the driver commits); the "Commit" step of each task is the driver's.

## Global Constraints

- "A page holds up to `options.limit` rows (default 200, at most 1,000) counting group, opening, detail and subtotal rows, in document order; a closed group counts as its one row."
- "`having` and `fill` apply before the keyset. The Total row is computed over every row the filter keeps, in a statement of its own, and every page still ends with it."
- "The 5,000 cap on group rows (`rowCap`, `capLevelRows`) goes for grouped reports; it stays for pivots, nests and layout reports."
- "`options.collapse: [groupKey]` (at most 5,000) … `expand` keeps its meaning for levels that are closed by default."
- "A grouped report whose groups fit one page returns the same rows after J1; the golden changes only where it pages, and the reason is written in the test."
- #452 stays: `recordJoins`, `lookupLayers` (keyset and limit on the level's outer select), fill keysets loop over parent rows first, every statement through `rn.query`.
- Core names no package; Go tests use `zoo_*`. Comments say why, not what. No `any` shortcuts in TS; Go keeps the engine's existing style (helpers documented, SQL values bound with `s.bind`, never inlined).
- Rulings R2 (fold semantics), R3 (what a page counts), R4 (`Page.Size` = detail rows), R5 (statements), R6 (cursor), R7 (currency order) of the outline bind every task.

## Review Focus

1. A group added before the cursor or the cursor's own group removed between pages: the next page neither skips nor repeats a row (Task 4 test `TestAGroupAddedOrRemovedBetweenPagesSkipsNoOther`).
2. A blank group key at a seam (`(none)`), which keys match by `IS`: pages in its place once (Task 4 test `TestABlankGroupPagesInItsPlace`).
3. Two currencies with the seam at a currency block's end: the block's Total keeps its place on the page that holds the block's end (Task 5 test `TestAPagedGroupPageKeepsEachCurrencyTotalInItsPlace`).
4. A cursor sent with another fold state, or after a design change: refused at `options.cursor` (Task 4 `TestAGroupCursorFromAnotherFoldStateIsRefused`).
5. A limit of 1 with an opening row: the group row and its opening share the page (Ruling R3; Task 5 test `TestAGroupRowAndItsOpeningShareAPage`).

## Interfaces produced in this part (for Part 2 and the Books checkpoints)

- `reports.Options.Collapse [][]any` — JSON `collapse`, at most `runLimits.foldCap` (5,000) keys; refused at `options.collapse`.
- `page.next` on every paged grouped result that has more rows; a `groupCursor` (base64url JSON `{"k","g","p","m","v"}`).
- No `rowCap` notice for a grouped report without pivot, layout or nest; pivots, layouts and nests keep it.
- `Page.Size` = detail rows on the page (R4).

---

### Task 1: Fold list (J1b, Go)

**Files:**
- Modify: `core/server/reports/run.go` (`Options` lines 20–41; `limits`/`runLimits` lines 196–206; `Run` lines 256–271; `expansion` lines 1298–1311; `parseExpansion` 1313–1355; `where` 1357–1424; `has` 1437–1440)
- Modify: `core/server/reports/fill.go` (`parentKeys` lines 30–52)
- Modify (tests): `core/server/reports/groups_test.go` (`TestExpandTakesAtMostTheRowCapOfKeys`, `TestExpandTakesTheRowCapOfTheDeepestKeys`), `open_test.go`, `fill_test.go`
- Create: `core/server/reports/collapse_test.go`

**Interfaces:**
- Consumes: `keyString`, `sqlJSON`, `numberValue`, `invalid`, `anyOf`, `deepKeys` (groups_test.go), `fillKeyset` (fill_test.go), `z.run`, `z.try`, `requireRows`, `requireSpecError`, `withLimits`.
- Produces: `Options.Collapse [][]any` (`json:"collapse,omitempty"`); `limits.foldCap int` (5000); `parseKeys(at string, keys [][]any) (norm [][]any, encoded [][]string, set map[string]bool, err error)`; `expansion` gains `closed map[string]bool`, `closedKeys [][]any`, `closedEncoded [][]string`; `(e *expansion) has(key []any, n int) bool` is false for a closed key; `(e *expansion) where(c *compiler, s *stmt, depth int, prefix string) string` excludes closed keys of that depth; `(e *expansion) shownUnder(c *compiler, s *stmt, depth int, prefix string) string` (AND of `where` for depths 1..depth, `"1"` at depth 0); `keysIn(s *stmt, cols []string, keys [][]any, encoded [][]string) string`; `openLevels(p *plan, opts Options, e *expansion)`.

- [ ] **Step 1: Write the failing tests**

`core/server/reports/collapse_test.go`:

```go
package reports

import (
	"fmt"
	"slices"
	"testing"
)

// byRegionThenName groups animals by their site's region (open) and then by
// name (closed by default): expand opens a name group, collapse closes a
// region.
const byRegionThenName = `{"version":4,"source":"zoo.animals","columns":[
	{"id":"region","path":[{"link":"site"},{"field":"region"}]},
	{"id":"name","path":[{"field":"name"}]},
	{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
	"groups":[{"id":"r","col":"region","open":true,"subtotals":true},{"id":"n","col":"name","subtotals":true}],
	"sort":[{"col":"name"}]}`

// A fold under a level that is open by default closes exactly that group:
// its row keeps its cells, it has no children and no subtotal, and every
// other group of the level stays open.
func TestCollapseClosesAGroupUnderAnOpenLevel(t *testing.T) {
	z := newZoo(t)
	open := z.run(t, "ann", pagedBySpecies, Options{}, nil)
	requireRows(t, open, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
	res := z.run(t, "ann", pagedBySpecies, Options{Collapse: [][]any{{"lion", "USD"}}}, nil)
	requireRows(t, res, "kind", "group", "group", "opening", "detail", "detail", "subtotal", "total")
	lion := res.Rows[0]
	if lion.Label != "lion" || lion.Expanded || !lion.CanExpand || lion.Cells["cost"] != int64(3500) {
		t.Fatalf("lion = %+v", lion)
	}
}

func TestCollapseClosesAGroupUnderExpandAll(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Collapse: [][]any{{"zebra", "USD"}}}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "total")
	if zebra := res.Rows[4]; zebra.Label != "zebra" || zebra.Expanded || zebra.Cells["cost"] != int64(1000) {
		t.Fatalf("zebra = %+v", zebra)
	}
}

// Ruling R2: a run that sends collapse keeps the open levels open, so expand
// adds a group of a closed level and collapse closes a group of an open one,
// in one run.
func TestCollapseAndExpandTogetherOnTwoLevels(t *testing.T) {
	z := newZoo(t)
	whole := z.run(t, "ann", byRegionThenName, Options{ExpandAll: true}, nil)
	var north, south []any
	// region is a select: its groups read their option labels.
	for _, r := range whole.Rows {
		if r.Kind == "group" && r.Depth == 0 && r.Label == "North" {
			north = r.GroupKey
		}
		if r.Kind == "group" && r.Depth == 0 && r.Label == "South" {
			south = r.GroupKey
		}
	}
	if north == nil || south == nil {
		t.Fatalf("regions = %v", rowsOf(whole, "label"))
	}
	// weight is not money, so the keys carry no currency key.
	leo := append(slices.Clone(north), "Leo")
	res := z.run(t, "ann", byRegionThenName, Options{Expand: [][]any{leo}, Collapse: [][]any{south}}, nil)
	for _, r := range res.Rows {
		switch {
		case r.Kind == "group" && r.Label == "North" && !r.Expanded:
			t.Fatalf("north closed: the open level must stay open when collapse is sent")
		case r.Kind == "group" && r.Label == "South" && r.Expanded:
			t.Fatalf("south open: collapse must close it")
		case r.Kind == "group" && r.Label == "Leo" && !r.Expanded:
			t.Fatalf("Leo closed: expand must open a group of a closed level")
		}
	}
}

func TestCollapseTakesAtMostTheFoldCapOfKeys(t *testing.T) {
	z := newZoo(t)
	z.run(t, "ann", sixLevels, Options{Collapse: deepKeys(runLimits.foldCap)}, nil)
	_, err := z.try(t, "ann", sixLevels, Options{Collapse: deepKeys(runLimits.foldCap + 1)}, nil)
	requireSpecError(t, err, 400, "options.collapse", fmt.Sprintf("at most %d groups", runLimits.foldCap))
}

func TestCollapseRefusesAKeyThatIsNotAGroupKey(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", feedingsBySpecies, Options{Collapse: [][]any{{}}}, nil)
	requireSpecError(t, err, 400, "options.collapse.0", "a group key is a group row's groupKey")
	_, err = z.try(t, "ann", feedingsBySpecies, Options{Collapse: [][]any{{map[string]any{"a": 1}}}}, nil)
	requireSpecError(t, err, 400, "options.collapse.0", "a group key holds text, numbers, true, false or null")
}
```

Add to `open_test.go`:

```go
// Ruling R2: collapse (even empty) keeps the open levels; expand without
// collapse still replaces them (TestOpenIsIgnoredWhenTheRunSendsKeys).
func TestOpenLevelsStayOpenWhenTheRunSendsCollapse(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", pagedBySpecies, Options{Expand: [][]any{}, Collapse: [][]any{}}, nil)
	requireRows(t, res, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
}
```

Add to `fill_test.go` (after `TestFillComputesOnlyUnderExpandedParents`):

```go
// A folded grandparent hides its grandchildren, so their fill records are
// not computed even though their parents' level is open (parentKeys reads
// every ancestor's fold state through shownUnder, not only the parent's).
func TestFillComputesNothingUnderAFoldedAncestor(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	all := fillKeyset(t, z, "ann", threeFills, Options{ExpandAll: true}, 3)
	owner := z.run(t, "ann", threeFills, Options{ExpandAll: true}, nil).Rows[0].GroupKey
	folded := fillKeyset(t, z, "ann", threeFills, Options{ExpandAll: true, Collapse: [][]any{owner}}, 3)
	if folded >= all {
		t.Fatalf("level-3 keyset with the first owner folded = %d, all open = %d", folded, all)
	}
}
```

`threeFills` is the spec of `fillplan_test.go`'s case "three levels of fill" (lines 126–136), moved to a `const threeFills` in `fill_test.go` and used by both.

(`fillKeyset` builds its expansion with `parseExpansion(opts)`; change it to call `openLevels(p, opts, expand)` after `parseExpansion`, as `Run` will, so the helper sees what a run sees.)

Change `groups_test.go`: in `TestExpandTakesAtMostTheRowCapOfKeys` set `l.foldCap = 2` instead of `l.rowCap = 2`; in `TestExpandTakesTheRowCapOfTheDeepestKeys` use `runLimits.foldCap` for `runLimits.rowCap` (three places). Rename them `TestExpandTakesAtMostTheFoldCapOfKeys` and `TestExpandTakesTheFoldCapOfTheDeepestKeys`; their comment says the fold cap bounds a key list (the group-row cap no longer bounds a grouped report).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core/server && go test ./reports/ -run 'Collapse|OpenLevelsStayOpen|FillComputesNothingUnderAFoldedAncestor|ExpandTakes' -count=1`
Expected: FAIL to compile: `unknown field Collapse in struct literal of type Options`, `runLimits.foldCap undefined`, `undefined: openLevels`.

- [ ] **Step 3: Implement**

In `run.go`, `Options`:

```go
	// Expand lists the group keys (a group row's groupKey) whose children
	// the result includes. Groups load lazily: a run returns the first
	// level and the children of each expanded group.
	Expand    [][]any `json:"expand,omitempty"`
	ExpandAll bool    `json:"expandAll,omitempty"`
	// Collapse lists groups the reader closed under a level that is open by
	// default or under ExpandAll. Sent (even empty), it keeps the open levels
	// open beside Expand, so a fold in a report with many open groups never
	// lists them all (J1b, Ruling R2).
	Collapse [][]any `json:"collapse,omitempty"`
```

`limits` gains `foldCap int` with the comment "the most group keys `options.expand` or `options.collapse` lists"; `runLimits` = `limits{timeout: 10 * time.Second, rowCap: 5000, foldCap: 5000, pageSize: 200, maxPage: 1000}`; the `runLimits` comment: "pivot, nest and layout results stop at rowCap rows (a grouped report pages its group rows instead)".

`expansion` gains:

```go
	// closed holds the folded keys (options.collapse) as keyString gives
	// them; closedKeys and closedEncoded are them as keys and encoded hold
	// the expanded ones.
	closed        map[string]bool
	closedKeys    [][]any
	closedEncoded [][]string
```

Replace `parseExpansion` with a shared key reader:

```go
// parseKeys reads a list of group keys (options.expand or options.collapse):
// at most foldCap keys. The keys' values need no cap of their own: a read
// binds them as one JSON parameter (keysIn), a key longer than a group row's
// matches nothing, and the request body's size limit bounds what a caller
// can send.
func parseKeys(at string, keys [][]any) ([][]any, [][]string, map[string]bool, error) {
	if len(keys) > runLimits.foldCap {
		verb := "expands"
		if at == "options.collapse" {
			verb = "folds"
		}
		return nil, nil, nil, invalid(at, fmt.Sprintf("a run %s at most %d groups", verb, runLimits.foldCap))
	}
	norms, encs, set := make([][]any, 0, len(keys)), make([][]string, 0, len(keys)), map[string]bool{}
	for i, key := range keys {
		where := fmt.Sprintf("%s.%d", at, i)
		if len(key) == 0 {
			return nil, nil, nil, invalid(where, "a group key is a group row's groupKey")
		}
		norm := make([]any, len(key))
		encoded := make([]string, len(key))
		for j, v := range key {
			switch x := v.(type) {
			case nil, string, bool:
				norm[j] = x
			case float64:
				if x == float64(int64(x)) {
					norm[j] = int64(x)
				} else {
					norm[j] = x
				}
			case json.Number:
				norm[j] = numberValue(x)
			default:
				return nil, nil, nil, invalid(where, "a group key holds text, numbers, true, false or null")
			}
			text, err := sqlJSON(norm[j])
			if err != nil {
				return nil, nil, nil, invalid(where, "a group key holds text, numbers, true, false or null")
			}
			encoded[j] = text
		}
		norms, encs = append(norms, norm), append(encs, encoded)
		set[keyString(norm)] = true
	}
	return norms, encs, set, nil
}

// parseExpansion reads options.expand and options.collapse.
func parseExpansion(opts Options) (*expansion, error) {
	e := &expansion{all: opts.ExpandAll, open: map[int]bool{}}
	var err error
	if e.keys, e.encoded, e.set, err = parseKeys("options.expand", opts.Expand); err != nil {
		return nil, err
	}
	if e.closedKeys, e.closedEncoded, e.closed, err = parseKeys("options.collapse", opts.Collapse); err != nil {
		return nil, err
	}
	return e, nil
}

// openLevels opens the levels marked open, down to the first level that is
// not (a level under a closed ancestor shows nothing, so it is not fetched),
// unless the run says what opens: expandAll opens every group, and expand
// without collapse lists the open groups (expand: [] opens nothing). A run
// that sends collapse keeps the open levels (Ruling R2).
func openLevels(p *plan, opts Options, e *expansion) {
	if opts.ExpandAll || (opts.Expand != nil && opts.Collapse == nil) {
		return
	}
	for i, g := range p.groups {
		if !g.group.Open {
			break
		}
		e.open[i+1] = true
	}
}
```

In `Run`, replace the inline loop (lines 260–271) with `openLevels(p, req.Options, expand)` and keep its comment there shortened to "No expansion state: the levels marked open expand (openLevels)".

Split `where`'s IN builder out so it serves both lists:

```go
// keysIn matches rows whose cols equal one of keys, value by value as IS
// compares them (the comment formerly on where explains the IN-by-blanks
// split, the json_each binding and COLLATE BINARY; move it here). "0" when
// no key has len(cols) values.
func keysIn(s *stmt, cols []string, keys [][]any, encoded [][]string) string {
	// body: lines 1378–1423 of where today, reading keys/encoded from the
	// arguments instead of e.keys/e.encoded.
}

// where matches rows whose first depth group keys and currency keys are an
// open group's: every group under expandAll or an open level, else an
// expanded key; never a folded key.
func (e *expansion) where(c *compiler, s *stmt, depth int, prefix string) string {
	cols := make([]string, 0, depth+len(c.currencies))
	for i := range depth {
		cols = append(cols, fmt.Sprintf("%sk%d", prefix, i))
	}
	for _, ck := range c.currencies {
		cols = append(cols, prefix+ck)
	}
	open := "1"
	if !e.all && !e.open[depth] {
		open = keysIn(s, cols, e.keys, e.encoded)
	}
	folded := keysIn(s, cols, e.closedKeys, e.closedEncoded)
	if folded == "0" {
		return open
	}
	return "(" + open + " AND NOT " + folded + ")"
}

// shownUnder is true for a row whose every enclosing group is open: the
// groups at depths 1..depth above it. A row under a folded grandparent
// shows nothing even when its parent's level is open.
func (e *expansion) shownUnder(c *compiler, s *stmt, depth int, prefix string) string {
	conds := []string{}
	for d := 1; d <= depth; d++ {
		if w := e.where(c, s, d, prefix); w != "1" {
			conds = append(conds, w)
		}
	}
	if len(conds) == 0 {
		return "1"
	}
	return strings.Join(conds, " AND ")
}

// has is true when the level-n group with key opens.
func (e *expansion) has(key []any, n int) bool {
	if e.closed[keyString(key)] {
		return false
	}
	return e.all || e.open[n] || e.set[keyString(key)]
}
```

`keysIn` returns a term that is never NULL (each IN leg compares only set values; blanks go through `IS NULL`), so `NOT` is safe; say so in its comment.

In `fill.go` `parentKeys`, the parent rows are those whose every ancestor is open: replace `expanded := expand.where(c, s, n-1, "")` with `expanded := expand.shownUnder(c, s, n-1, "")`, and its comment: "the rows of level n-1 that show and are open, its fill rows included. A folded or collapsed row, or one under a folded ancestor, shows no children, so its fill records are never computed."

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -run 'Collapse|Open|Fill|Expand' -count=1`
Expected: PASS. Then the whole package: `cd core/server && go test ./reports/ -count=1` — PASS (no other behavior changed: `where` with no folds returns what it returned).

- [ ] **Step 5: Commit** (driver) — `reports: options.collapse folds groups of open levels (J1b)`

---

### Task 2: Group cursor (Go)

**Files:**
- Modify: `core/server/reports/paging.go` (whole file: `cursor` stays for ungrouped reports; new group cursor beside it; `keysetCond` lines 131–151 becomes a call of `afterSQL`)
- Modify (tests): `core/server/reports/paging_test.go`

**Interfaces:**
- Consumes: `sortKey{name, col string; desc bool; typ FieldType}` (paging.go `valueKind`), `numberValue`, `badCursor`, `keyString`, `expansion` (Task 1).
- Produces:

```go
// orderTerm is one term a level orders its rows by: SQL over the level's
// a<L> row, and its direction. A level's "(x IS NULL)" terms are terms too.
type orderTerm struct {
	sql  string
	desc bool
}

const (
	cursorIn    = "in"    // the page ended inside the group: after its row (and opening), or after detail v of it
	cursorAfter = "after" // the page ended after the group's whole subtree
)

// groupCursor is a paged grouped report's cursor (Ruling R6).
type groupCursor struct {
	Key    string  `json:"k"`
	Group  []any   `json:"g"`           // the path's deepest group key: group keys, then currency keys
	Path   [][]any `json:"p"`           // each path level's order values, level 1 first
	Mode   string  `json:"m"`           // cursorIn or cursorAfter
	Values []any   `json:"v,omitempty"` // detail sort values then the id, inside an innermost group only
}

// groupPos is where a page ended, before it is written as a cursor.
type groupPos struct {
	key  []any
	path [][]any
	mode string
}

func foldPrint(e *expansion) string
func groupFingerprint(levels, shape, fold string, keys []sortKey) string
func levelShape(levels [][]orderTerm) string
func groupCursorOf(fp string, keys []sortKey, at groupPos, detail map[string]any) string
func readGroupCursor(fp string, shape [][]orderTerm, keys []sortKey, groups, currencies int, text string) (groupCursor, error)
func afterSQL(s *stmt, cols []orderTerm, values []any) string
```

- [ ] **Step 1: Write the failing tests** (append to `paging_test.go`)

```go
func testShape() [][]orderTerm {
	return [][]orderTerm{{{sql: "(a.k0 IS NULL)"}, {sql: "a.k0"}}, {{sql: "(x IS NULL)"}, {sql: "x", desc: true}, {sql: "(a.k1 IS NULL)"}, {sql: "a.k1"}}}
}

func TestAGroupCursorReadsBackWhatItWrote(t *testing.T) {
	keys := []sortKey{{name: "s0", col: "note", typ: Text}, {name: "__id"}}
	fp := groupFingerprint("species/;note/", levelShape(testShape()), foldPrint(&expansion{open: map[int]bool{1: true}}), keys)
	at := groupPos{key: []any{"lion", "f1", "USD"}, path: [][]any{{int64(0), "lion"}, {int64(0), int64(12), int64(0), "f1"}}, mode: cursorIn}
	text := groupCursorOf(fp, keys, at, map[string]any{"s0": "f2", "__id": "abc"})
	cur, err := readGroupCursor(fp, testShape(), keys, 2, 1, text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cur.Group, at.key) || !reflect.DeepEqual(cur.Path, at.path) || cur.Mode != cursorIn || !reflect.DeepEqual(cur.Values, []any{"f2", "abc"}) {
		t.Fatalf("cursor = %+v", cur)
	}
	after := groupCursorOf(fp, keys, groupPos{key: []any{"lion", "USD"}, path: at.path[:1], mode: cursorAfter}, nil)
	if cur, err := readGroupCursor(fp, testShape(), keys, 2, 1, after); err != nil || cur.Values != nil || cur.Mode != cursorAfter {
		t.Fatalf("after cursor = %+v, %v", cur, err)
	}
}

// The fingerprint ties a group cursor to the fold state (J1b): a fold, an
// unfold, Expand all or Collapse all is a new run from page 1.
func TestAGroupFingerprintChangesWithTheFoldState(t *testing.T) {
	keys := []sortKey{{name: "__id"}}
	shape := levelShape(testShape())
	base := &expansion{open: map[int]bool{1: true}, set: map[string]bool{}, closed: map[string]bool{}}
	folded := &expansion{open: map[int]bool{1: true}, set: map[string]bool{}, closed: map[string]bool{`["lion","USD"]`: true}}
	all := &expansion{all: true, open: map[int]bool{}, set: map[string]bool{}, closed: map[string]bool{}}
	prints := map[string]bool{}
	for _, e := range []*expansion{base, folded, all} {
		prints[groupFingerprint("species/", shape, foldPrint(e), keys)] = true
	}
	if len(prints) != 3 {
		t.Fatalf("fingerprints = %v, want three", prints)
	}
	// The same folds listed in another order are the same state.
	a := &expansion{open: map[int]bool{}, set: map[string]bool{`["a"]`: true, `["b"]`: true}, closed: map[string]bool{}}
	b := &expansion{open: map[int]bool{}, set: map[string]bool{`["b"]`: true, `["a"]`: true}, closed: map[string]bool{}}
	if foldPrint(a) != foldPrint(b) {
		t.Fatal("the fold print depends on the keys' order")
	}
}

func TestAMalformedGroupCursorIsRefused(t *testing.T) {
	keys := []sortKey{{name: "__id"}}
	fp := groupFingerprint("species/;note/", levelShape(testShape()), "", keys)
	good := func(c map[string]any) string {
		raw, _ := json.Marshal(c)
		return encodeCursor(string(raw))
	}
	cases := map[string]map[string]any{
		"another fingerprint":     {"k": "000000000000", "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "after"},
		"no mode":                 {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}},
		"a mode it never writes":  {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "before"},
		"path longer than group":  {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, 0, "f1"}}, "m": "after"},
		"a level of wrong width":  {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0}}, "m": "after"},
		"values outside a leaf":   {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "in", "v": []any{"x"}},
		"values with mode after":  {"k": fp, "g": []any{"lion", "f1", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, 0, "f1"}}, "m": "after", "v": []any{"x"}},
		"an object in the group":  {"k": fp, "g": []any{map[string]any{}, "USD"}, "p": []any{[]any{0, "lion"}}, "m": "after"},
		"an object in the path":   {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, map[string]any{}}}, "m": "after"},
	}
	for name, c := range cases {
		if _, err := readGroupCursor(fp, testShape(), keys, 2, 1, good(c)); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
}

func TestAfterSQLIsTrueOnlyForARowAfterTheValues(t *testing.T) {
	z := newZoo(t)
	s := newStmt(z.app, nil)
	cols := []orderTerm{{sql: "(x IS NULL)"}, {sql: "x", desc: true}, {sql: "y"}}
	type row struct {
		x    any
		y    string
		want bool
	}
	cases := []struct {
		values []any
		rows   []row
	}{
		{[]any{int64(0), int64(5), "m"}, []row{{int64(6), "a", false}, {int64(5), "a", false}, {int64(5), "m", false}, {int64(5), "z", true}, {int64(4), "a", true}, {nil, "a", true}}},
		// A blank cursor value: only a blank row ties on it; a set row sorts before.
		{[]any{int64(1), nil, "m"}, []row{{int64(3), "z", false}, {nil, "a", false}, {nil, "m", false}, {nil, "z", true}}},
	}
	for _, c := range cases {
		cond := afterSQL(s, cols, c.values)
		for _, r := range c.rows {
			// No coalesce: NOT afterSQL must count a row, so the term is never NULL.
			var after, notAfter any
			q := fmt.Sprintf("SELECT (%s), NOT (%s) FROM (SELECT {:x} AS x, {:y} AS y)", cond, cond)
			params := dbx.Params{"x": r.x, "y": r.y}
			for k, v := range s.params {
				params[k] = v
			}
			if err := z.app.DB().NewQuery(q).Bind(params).Row(&after, &notAfter); err != nil {
				t.Fatal(err)
			}
			if after == nil || notAfter == nil || (after == int64(1)) != r.want {
				t.Errorf("values %v, x=%v y=%v: after = %v, not after = %v, want %v", c.values, r.x, r.y, after, notAfter, r.want)
			}
		}
	}
}
```

(Check the imports `encoding/json`, `fmt`, `reflect`, `github.com/pocketbase/dbx` in `paging_test.go`; add those missing. If `s.bind` names parameters other than `{:name}`, read `stmt.go` `bind` and bind the row's two values the same way the statement binds its own.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core/server && go test ./reports/ -run 'GroupCursor|GroupFingerprint|AfterSQL' -count=1`
Expected: FAIL to compile: `undefined: orderTerm`, `undefined: groupFingerprint`, `undefined: afterSQL`.

- [ ] **Step 3: Implement** (in `paging.go`)

```go
// foldPrint names a run's fold state for a group cursor's fingerprint:
// expandAll, the open levels, and the expanded and folded keys in sorted
// order. A cursor from another fold state is refused (J1a); the client then
// reads from page 1, as it does after any fold.
func foldPrint(e *expansion) string {
	var b strings.Builder
	if e.all {
		b.WriteString("all;")
	}
	levels := make([]int, 0, len(e.open))
	for n, ok := range e.open {
		if ok {
			levels = append(levels, n)
		}
	}
	slices.Sort(levels)
	for _, n := range levels {
		fmt.Fprintf(&b, "open%d;", n)
	}
	for _, part := range []struct {
		tag string
		set map[string]bool
	}{{"+", e.set}, {"-", e.closed}} {
		keys := make([]string, 0, len(part.set))
		for k := range part.set {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(part.tag + k + ";")
		}
	}
	return b.String()
}

// levelShape names each group level's order terms (count and directions), so
// a cursor's path values fit the levels that read them.
func levelShape(levels [][]orderTerm) string {
	var b strings.Builder
	for _, terms := range levels {
		for _, t := range terms {
			b.WriteString(dirOf(t.desc) + ",")
		}
		b.WriteString(";")
	}
	return b.String()
}

// groupFingerprint ties a group cursor to the group levels, each level's
// order shape, the fold state and the detail sort keys (R9d, R6).
func groupFingerprint(levels, shape, fold string, keys []sortKey) string {
	return fingerprint("groups:"+levels+"|shape:"+shape+"|fold:"+fold, keys)
}

// groupCursorOf writes where a page ended; detail is the page's last detail
// row when it ended on one (Mode in).
func groupCursorOf(fp string, keys []sortKey, at groupPos, detail map[string]any) string {
	c := groupCursor{Key: fp, Group: at.key, Path: at.path, Mode: at.mode}
	if detail != nil {
		c.Values = make([]any, len(keys))
		for i, k := range keys {
			c.Values[i] = detail[k.name]
		}
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
```

`fingerprint(levels, keys)` already hashes `levels`; pass the joined text so ungrouped and old-grouped cursors keep their fingerprints.

`readGroupCursor` decodes as `readCursor` does (base64url, `UseNumber`, one value, `io.EOF` after it) and refuses (`badCursor()`) unless: `Key == fp`; `Mode` is `cursorIn` or `cursorAfter`; `len(Group) == d + currencies` with `1 <= d <= groups`; `len(Path) == d` and `len(Path[i]) == len(shape[i])`; every value of `Group` and `Path` is a JSON scalar (`json.Number` → `numberValue`, string, bool, nil); `Values` is empty unless `Mode == cursorIn && d == groups`, and when present it is checked against `keys` exactly as `readCursor` checks `c.Values` (lines 94–113; move that loop into `readValues(keys []sortKey, values []any) error` and call it from both).

`afterSQL` generalises `keysetCond` (lines 131–151) to terms whose SQL is given, where equality is `IS` (a term may be NULL on both sides) and a NULL bound value is never "less" than a row's value:

```go
// afterSQL is true for a row whose terms sort after values, in the terms'
// order and directions, and false (never NULL) otherwise, so NOT afterSQL
// counts a row (the pc count of Task 4). A blank value is matched only by a
// blank (the level's "(x IS NULL)" term before it already put every set
// value first), as keysetCond does. Values are bound, never inlined.
func afterSQL(s *stmt, cols []orderTerm, values []any) string {
	cond := "0"
	for i := len(cols) - 1; i >= 0; i-- {
		if values[i] == nil {
			cond = "(" + cols[i].sql + " IS NULL AND " + cond + ")"
			continue
		}
		v := s.bind(values[i])
		cmp := " > "
		if cols[i].desc {
			cmp = " < "
		}
		// A blank row value against a set value: the "(x IS NULL)" term
		// decided it already; coalesce keeps the term false, not NULL.
		cond = "(coalesce(" + cols[i].sql + cmp + v + ", 0) OR (" + cols[i].sql + " IS " + v + " AND " + cond + "))"
	}
	return cond
}
```

`keysetCond` stays as it is (its NULLs-last rule reads detail columns that carry no `IS NULL` term); add one line to its comment pointing at `afterSQL` for group levels.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -run 'Cursor|GroupFingerprint|AfterSQL|Paging|Page' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** (driver) — `reports: a group cursor carries the group path and the fold state`

---

### Task 3: The Total in a statement of its own

**Files:**
- Modify: `core/server/reports/run.go` (`groupPass` lines 487–520, `groupStatement` lines 697–811, `compileDry` line 1461, `run` line 354)
- Modify: `core/server/reports/fill_test.go` (`fillKeyset` calls `groupStatement`)
- Modify (tests): `core/server/reports/ledger_guard_test.go` (`TestABalanceShapedReportComputesItsBeginningBalanceOncePerStatement`); `core/server/reports/fillplan_test.go` (keyset counts, only if the Total statement changes them; Step 4)
- Create: `core/server/reports/pagedgroups_test.go`

**Interfaces:**
- Consumes: Task 1 `openLevels`.
- Produces: `(rn *runner) pagedGroups() bool` — true for a grouped report with no pivot, no `layout` and no nest (`len(c.p.groups) > 0 && c.p.pivot == nil && len(c.p.layout) == 0 && len(c.p.nest) == 0`); `(c *compiler) groupStatement(s *stmt, r *rowsCTE, expand *expansion, pivot []pivotValue, limit int64, withTotal bool) ([]string, string, error)`; `(rn *runner) totalPass(s *stmt, ctes []string) ([]map[string]any, error)`.

- [ ] **Step 1: Write the failing tests**

`core/server/reports/pagedgroups_test.go`:

```go
package reports

import (
	"strings"
	"testing"
)

// isTotalStatement is the Total statement of a paged grouped run: its body
// ends reading o0 alone (totalPass).
func isTotalStatement(sql string) bool {
	return strings.HasSuffix(strings.TrimSpace(sql), " FROM o0")
}

// J1a: the Total is computed over every row the filter keeps, in a statement
// of its own, so a page of group rows never decides it.
func TestAGroupedRunReadsItsTotalInAStatementOfItsOwn(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	totals := 0
	for _, st := range *seen {
		if isTotalStatement(st.sql) {
			totals++
		}
	}
	if totals != 1 {
		t.Fatalf("%d Total statements, want 1", totals)
	}
	if last := res.Rows[len(res.Rows)-1]; last.Kind != "total" || last.Cells["cost"] != int64(4500) {
		t.Fatalf("total = %+v", last)
	}
}

// A pivot and a layout report still read their Total with their rows: a
// pivot has no pages, and a layout's computed rows read its sections.
func TestAPivotAndALayoutKeepOneGroupStatement(t *testing.T) {
	for name, spec := range map[string]string{"pivot": costBySpeciesAndMonth, "layout": zooStatement} {
		z := newZoo(t)
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{}, nil)
		for _, st := range *seen {
			if isTotalStatement(st.sql) {
				t.Fatalf("%s: a Total statement of its own", name)
			}
		}
	}
}
```

(Set the expected Total `4500` from what `z.run(t, "ann", feedingsBySpecies, …)` totals today — f1 1000 + f2 2500 + f3 700 + f4 300 — and adjust it if ann's rows differ; the test pins that the separate statement gives the same Total. `totalPass`'s body must end in ` FROM o0`, which `isTotalStatement` reads.)

In `ledger_guard_test.go` `TestABalanceShapedReportComputesItsBeginningBalanceOncePerStatement`: classify a statement `"total"` when `isTotalStatement(st.sql)`, else `"group"` when it holds `__level`, else `"detail"`; replace `if kinds != 2` with a check that the kinds seen are exactly `group`, `total`, `detail`, each once: `t.Fatalf("statements %v, want the group, the Total and the detail statement", got)`. The per-statement lookup bounds stay as they are and now also bind the Total statement.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core/server && go test ./reports/ -run 'StatementOfItsOwn|KeepOneGroupStatement|ComputesItsBeginningBalanceOncePerStatement' -count=1`
Expected: FAIL: `0 Total statements, want 1`; the ledger guard: `statements [group detail], want the group, the Total and the detail statement`. (`TestAPivotAndALayoutKeepOneGroupStatement` passes now and pins that the change leaves them alone.)

- [ ] **Step 3: Implement**

`groupStatement` gains `withTotal bool`: when false, the `n == 0` leg (`SELECT * FROM o0`) is not added to `legs` (its CTEs stay in `ctes`: a level that reads the total's frame still has it, and SQLite computes an unread CTE never), and the probe CTE and the probe cells are not added (the probes stay inside `groupStatement`, behind the same switch: `probes := c.probeCells()` and `c.probeCTE()` only when `withTotal`; a paged run has no layout, so there is no computed leg to carry them). Every caller passes `true` except the paged path: `compileDry` (`true`), `fillKeyset` in `fill_test.go` (`true`), `groupPass`. With `withTotal` true the statement is byte-identical to today's (the golden and the guards read it).

```go
// pagedGroups is true for a run whose group rows page (J1a): a grouped
// report with no pivot (a pivot's columns come from every row), no layout (a
// statement's computed rows read every section) and no nest (child rows page
// with their detail rows). The others keep one group statement and its cap.
func (rn *runner) pagedGroups() bool {
	p := rn.c.p
	return len(p.groups) > 0 && p.pivot == nil && len(p.layout) == 0 && len(p.nest) == 0
}

// totalPass reads the Total rows (o0, one per currency) over the group
// statement's CTEs, in a statement of their own: every row the filter keeps,
// whatever page the group rows are on. The condition probes ride on it, as
// it always returns a row unless there are no rows at all.
func (rn *runner) totalPass(s *stmt, ctes []string) ([]map[string]any, error) {
	c := rn.c
	probes := c.probeCells()
	if cte := c.probeCTE(); cte != "" {
		ctes = append(ctes, cte)
	}
	body := "SELECT " + strings.Join(append([]string{"*"}, probes...), ", ") + " FROM o0"
	rows, err := rn.query(s, ctes, body)
	if err != nil {
		return nil, err
	}
	if err := rn.readProbes(s, ctes, rows, c.condProbes); err != nil {
		return nil, err
	}
	return rows, nil
}
```

The body ends in ` FROM o0` (no ORDER BY: Go sorts the Totals by currency in `assemble`), which `isTotalStatement` reads.

In `groupPass`, when `rn.pagedGroups()`: call `groupStatement(…, int64(runLimits.rowCap+1), false)` (no `o0` leg, no probe CTE, no probe cells; `totalPass` adds the probes for the paged path), run it with no `readProbes` call, then `totals, err := rn.totalPass(s, ctes)` with the same `ctes` (the `r.render()` CTE first, as today), and return `rn.capLevelRows(append(totals, rows...))`: the Totals first, as the one statement's `ORDER BY __level` put them, so the cap counts and keeps exactly the rows it did before (`TestGroupRowsStopAtTheRowCap`, `TestGroupRowsStillStopAtTheGroupCap` and `TestTheRowCapNoticeIsGivenOnce` keep passing unchanged in this task). Non-paged reports are unchanged.

The probe cells are read from the first row of the statement that carries them (`readProbes`); with the probes on the Total statement and the level statement free of them, `readProbes` is called once, from `totalPass`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -count=1`
Expected: PASS, the `ledger_scale` golden included (rows unchanged; Ruling R4 keeps `Page.Size`), and the package's topics check (TestMain) has no mismatch for the new statement.

`fillplan_test.go` `TestAFillKeysetLoopsOverItsParentRowsFirst` may fail here, not in Task 6: from this task a paged grouped run sends a Total statement of its own, and where `o0` reads level 1's "rows" sums (`readsSums` → `rt1`), that statement's plan holds level 1's fill keyset too, so a case (for example "ledger, 200 accounts") counts one more keyset. If it fails, count (`go test ./reports/ -run TestAFillKeysetLoopsOverItsParentRowsFirst -v`), set each changed case's `keysets` to what the plans show, with the comment "the page and the Total statement each read f<L> (J1a: the Total has a statement of its own)", and check that every keyset still has `p` outer. Task 6 then only adds its new case and changes a count only where Task 4's or Task 5's page statement changed it.

- [ ] **Step 5: Commit** (driver) — `reports: a grouped report reads its Total in a statement of its own`

---

### Task 4: Paged group rows while no innermost group opens (J1a)

The group keyset, the page cut and the group cursor, for runs that show no detail rows (no innermost group opens). Task 5 removes this gate.

**Files:**
- Modify: `core/server/reports/levels.go` (`level` struct lines 25–80 gains `terms []orderTerm`, `paged bool`; `level()` line 118–122; `order()` lines 227–267 builds terms; `render` lines 343–372)
- Modify: `core/server/reports/run.go` (`runner` struct 211–235; `run` 349–403; `groupStatement` split into `compileLevels` + `unreadCells` + its own legs and render (output unchanged); new `compiledLevel`, `pagedGroupStatement`, `pathCTEs`, `groupKeyset`, `pagedGroupPass`; `capLevelRows` not called on the paged path)
- Modify: `core/server/reports/assemble.go` (`assemble` keeps raw group rows; new `cutPage`; `pageWindow` unchanged, still used by nests)
- Modify (tests): `core/server/reports/pagedgroups_test.go`, `groups_test.go` (`TestGroupRowsStopAtTheRowCap`)

**Interfaces:**
- Consumes: Task 1 `shownUnder`, `has`; Task 2 `orderTerm`, `groupCursor`, `groupPos`, `groupFingerprint`, `levelShape`, `foldPrint`, `groupCursorOf`, `readGroupCursor`, `afterSQL`; Task 3 `pagedGroups`, `totalPass`, `groupStatement(…, withTotal)`.
- Produces:
  - `level.terms []orderTerm` (what `order()` builds; `frame.order` = `orderSQL(terms)`); `orderSQL(terms []orderTerm) []string`.
  - `type compiledLevel struct { lv *level; cells, totals []string; own string }` (today's local `levelRows` type of `groupStatement`, moved to package level); `(c *compiler) compileLevels(s *stmt, r *rowsCTE, expand *expansion, pivot []pivotValue) ([]compiledLevel, error)` (the per-level loop of `groupStatement`, levels 0..G, nothing rendered); `(c *compiler) unreadCells(levels []compiledLevel, computed []unionLeg) map[string]bool` (the `unread` set and its `vcur` exception).
  - `(c *compiler) pagedGroupStatement(s *stmt, levels []compiledLevel, expand *expansion, cur *groupCursor, limit int64) (levelCTEs, pageCTEs []string, body string, err error)` — renders the compiled levels and writes the keyset from the cursor already read: the level rows of one page, no `o0` leg, no probes.
  - `(rn *runner) pagedGroupPass() ([]map[string]any, error)` — compile the levels once, read the cursor from their terms, page statement, then `totalPass`; sets `rn.groupShape [][]orderTerm`.
  - `runner` gains `groupCur *groupCursor`, `groupShape [][]orderTerm`, `rawGroups map[string]map[string]any` (by group row id), `rawDetails map[string]map[string]any` (by detail row id), `groupFP string`.
  - `(rn *runner) cutPage(limit int)` — keeps the page's slice, sets `rn.res.Page`.
  - `(rn *runner) pagedNow() bool` — `pagedGroups()` and (this task only) no innermost group opens.

- [ ] **Step 1: Write the failing tests** (append to `pagedgroups_test.go`)

```go
// byNoteClosed groups feedings by note, every group closed: one group row per
// feeding. bulkFeedings adds n more (notes n00001…).
const byNoteClosed = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"note","subtotals":true}],"sort":[{"col":"note"}]}`

func groupLabels(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Kind == KindGroup {
			out = append(out, r.Label)
		}
	}
	return out
}

func lastTotal(t *testing.T, res *Result) Row {
	t.Helper()
	last := res.Rows[len(res.Rows)-1]
	if last.Kind != KindTotal {
		t.Fatalf("page ends in %+v, want the Total", last)
	}
	return last
}

// J1a: closed groups page in document order with no repeat and no gap, each
// page holds exactly Limit group rows (a closed group counts as its one row),
// and every page ends in the whole report's Total.
func TestClosedGroupsPageThroughEveryGroup(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 7)
	whole := z.run(t, "ann", byNoteClosed, Options{Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 3})
	want := groupLabels(whole.Rows)
	if len(want) != 11 || len(pages) != 4 {
		t.Fatalf("%d groups on %d pages", len(want), len(pages))
	}
	for i, p := range pages {
		if n := len(groupLabels(p.Rows)); i < 3 && n != 3 {
			t.Fatalf("page %d holds %d group rows, want 3", i+1, n)
		}
		if got, want := lastTotal(t, p).Cells["cost"], lastTotal(t, whole).Cells["cost"]; got != want {
			t.Fatalf("page %d total = %v, want %v", i+1, got, want)
		}
		if (p.Page.Next == "") != (i == len(pages)-1) {
			t.Fatalf("page %d next = %q", i+1, p.Page.Next)
		}
	}
	if got := groupLabels(mergedRows(t, pages)); !slices.Equal(got, want) {
		t.Fatalf("paged groups %v\nwant %v", got, want)
	}
}

// Two levels, the inner closed: an open outer group whose inner groups span
// a page boundary goes on on the next page, and its subtotal follows them.
func TestTwoLevelsPageAcrossAnOpenOuterGroup(t *testing.T) {
	z := newZoo(t)
	whole := z.run(t, "ann", byRegionThenName, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 4; limit++ {
		pages := groupedPages(t, z, "ann", byRegionThenName, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
	}
}

// having hides groups before the keyset: a hidden group is never counted and
// never shown, at a seam or anywhere else.
func TestHavingHidesGroupsBeforeTheKeyset(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6)
	spec := strings.Replace(byNoteClosed, `"subtotals":true}`, `"subtotals":true,"having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}}`, 1)
	whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", spec, Options{Limit: 1})
	if got := groupLabels(mergedRows(t, pages)); !slices.Equal(got, groupLabels(whole.Rows)) || len(got) != 4 {
		t.Fatalf("groups %v, want the four feedings over 1 (f1–f4)", got)
	}
}

// fill groups page as base groups do: a fill record with no rows is a group
// row in its place.
func TestFillGroupsPageInTheirPlace(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	raw := animalFill(`{}`, "")
	whole := z.run(t, "ann", raw, Options{Expand: [][]any{}, Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", raw, Options{Expand: [][]any{}, Limit: 1})
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", rowsOf(&Result{Rows: got}, "label"), rowsOf(whole, "label"))
	}
}

// A blank group key ((none)) pages in its place once.
func TestABlankGroupPagesInItsPlace(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"region"}]}`
	whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
	if !slices.Contains(groupLabels(whole.Rows), "(none)") {
		t.Fatalf("no blank group: %v", groupLabels(whole.Rows))
	}
	pages := groupedPages(t, z, "ann", spec, Options{Limit: 1})
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages %v\nwhole %v", rowsOf(&Result{Rows: got}, "label"), rowsOf(whole, "label"))
	}
}

// Keyset on values (R1): a group added before the cursor, or the cursor's own
// group removed, between pages skips no group after the cursor.
func TestAGroupAddedOrRemovedBetweenPagesSkipsNoOther(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 3}, nil)
	z.feeding(t, "a0", z.recs["leo"], 5, "2026-04-02", 1) // note "a0" sorts first
	// f1 is before the cursor; f3 is the cursor's own group.
	if _, err := z.app.DB().NewQuery("DELETE FROM zoo_feedings WHERE note IN ('f1', 'f3')").Execute(); err != nil {
		t.Fatal(err)
	}
	rest := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 3, Cursor: first.Page.Next})
	got := groupLabels(mergedRows(t, append([]*Result{first}, rest...)))
	want := []string{"f1", "f2", "f3", "f4", "n00001", "n00002", "n00003", "n00004"}
	if !slices.Equal(got, want) {
		t.Fatalf("groups %v, want %v", got, want)
	}
}

// J1a, every group closed: page 1 holds exactly N group rows.
func TestEveryGroupClosedPageOneHoldsExactlyLimitGroupRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 20)
	for _, n := range []int{1, 5, 17} {
		res := z.run(t, "ann", byNoteClosed, Options{Limit: n}, nil)
		if got := len(groupLabels(res.Rows)); got != n || res.Page.Next == "" || res.Page.Size != 0 {
			t.Fatalf("limit %d: %d group rows, page %+v", n, got, res.Page)
		}
	}
}

// The group-row cap is gone for a grouped report: 6,004 groups page to their
// end with no rowCap notice.
func TestASixThousandGroupReportReturnsEveryGroup(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6000)
	pages := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 1000})
	labels := []string{}
	for i, p := range pages {
		for _, n := range p.Notices {
			if n.Code == "rowCap" {
				t.Fatalf("page %d says %q", i+1, n.Message)
			}
		}
		labels = append(labels, groupLabels(p.Rows)...)
	}
	if len(labels) != 6004 || len(pages) != 7 || !slices.IsSorted(labels) {
		t.Fatalf("%d groups on %d pages, sorted %v", len(labels), len(pages), slices.IsSorted(labels))
	}
}

// A pivot and a layout report keep the cap and its notice (a statement needs
// every section; a pivot's columns read every row).
func TestAPivotAndALayoutStillStopAtTheRowCap(t *testing.T) {
	for name, spec := range map[string]string{"pivot": costBySpeciesAndMonth, "layout": zooStatement} {
		z := newZoo(t)
		withLimits(t, func(l *limits) { l.rowCap = 1 })
		res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "rowCap" && n.Count == 1 }) {
			t.Fatalf("%s: notices %+v", name, res.Notices)
		}
	}
}

func TestAGroupCursorFromAnotherFoldStateIsRefused(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 2}, nil)
	for name, opts := range map[string]Options{
		"a fold":       {Collapse: [][]any{{"f1", "USD"}}},
		"an expand":    {Expand: [][]any{{"f1", "USD"}}},
		"expand all":   {ExpandAll: true},
		"another sort": {Sort: []ViewSort{{Col: "cost", Dir: "desc"}}},
	} {
		opts.Limit, opts.Cursor = 2, first.Page.Next
		_, err := z.try(t, "ann", byNoteClosed, opts, nil)
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
		_ = name
	}
}

// checkTopics (TestMain) sees the page statement and the Total statement of
// a cursor page; this test makes sure both are sent on a cursor page.
func TestAPagedGroupRunSendsAPageAndATotalStatement(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 2}, nil)
	seen := observeReads(t)
	z.run(t, "ann", byNoteClosed, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	kinds := map[string]int{}
	for _, st := range *seen {
		switch {
		case isTotalStatement(st.sql):
			kinds["total"]++
		case strings.Contains(st.sql, "__p1"):
			kinds["page"]++
		}
	}
	if kinds["total"] != 1 || kinds["page"] != 1 {
		t.Fatalf("statements %v", kinds)
	}
}
```

(`byRegionThenName` is in `collapse_test.go`, Task 1. `"a fold"` needs the run's group keys: feedings by note carry the cost's currency key, so `{"f1","USD"}` is f1's key; read one from `first.Rows[0].GroupKey` if the zoo keys differ. Imports: `reflect`, `slices`, `strings`.)

In `groups_test.go`, `TestGroupRowsStopAtTheRowCap` becomes `TestAGroupedReportHasNoRowCap`: with `rowCap = 2`, `feedingsBySpecies` (two closed groups) shows both groups with no notice, and `costBySpeciesAndMonth` (a pivot) still gives the `rowCap` notice with `Count` 2.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core/server && go test ./reports/ -run 'ClosedGroupsPage|TwoLevelsPage|HavingHides|FillGroupsPage|BlankGroupPages|AddedOrRemoved|ExactlyLimitGroupRows|SixThousandGroup|StillStopAtTheRowCap|AnotherFoldState|APageAndATotal|HasNoRowCap' -count=1`
Expected: FAIL: closed group runs have no `page` (a run with no open innermost group has no detail pass, so `res.Page` is nil → nil dereference or "page 1 next"), the 6,000-group run says `rowCap`, `statements map[total:1]`. `TestAPivotAndALayoutStillStopAtTheRowCap` passes (pin).

- [ ] **Step 3: Implement**

**3a. Order terms (`levels.go`).** In `order()`, build `[]orderTerm` instead of strings: each `"(" + x + " IS NULL)"` is `orderTerm{sql: "(" + x + " IS NULL)"}`, each `x + dirOf(desc)` is `orderTerm{sql: x, desc: desc}`, the label key and the group key ascending, and `[]string{"1"}` stays the empty case as `[]orderTerm{{sql: "1"}}`. Set `lv.terms = terms` and `lv.frame.order = orderSQL(terms)` in `level()`:

```go
// orderSQL writes order terms as ORDER BY items.
func orderSQL(terms []orderTerm) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = t.sql + dirOf(t.desc)
	}
	return out
}
```

This keeps every non-paged statement byte-identical (the golden and the guards read them).

**3b. Order values on `o<L>` (`levels.go` `render`).** When `lv.paged`, add after `"a.__n AS __n", kids+" AS __kids"` one cell per term, `term.sql AS __s<L>_<j>`, and render the CTE `o<L> AS MATERIALIZED (…)` (Ruling R5: the path CTE reads `o<L>` a second time; materialized, each lookup is still computed once). Level 0 is never paged (no terms read).

**3c. The page statement (`run.go`).**

```go
// pagedGroupStatement selects one page of level rows (J1a) in document order:
// the currency blocks in Total order (R7), then each level's sibling ordinal
// down the path (a parent before its children). Its rows are the cursor's
// anchors (the enclosing groups the page starts inside, for their subtotals)
// and the rows after the cursor, at most limit+1 of them past the anchors,
// so the cut (cutPage) knows whether more follow. Every o<L> keeps its
// lookup layers and record joins (#452); the keyset and limit go on the outer
// select. having and fill already shaped o<L>, so they apply before the
// keyset. The levels are compiled (compileLevels) and the cursor read
// before this is called: the cursor's shape is the levels' order terms.
func (c *compiler) pagedGroupStatement(s *stmt, levels []compiledLevel, expand *expansion, cur *groupCursor, limit int64) (levelCTEs, pageCTEs []string, body string, err error)
```

**One order (no level is compiled twice).** Compiling a level binds its parameters on `s` and resets compiler state (`levelTypes`, `sectionCur`, `fillSets`, `sectionMixed`, the probes), so the levels compile exactly once per statement, and the cursor is read between compiling and rendering:

1. Extract from `groupStatement` the loop over `n := 0..len(c.p.groups)` (from the `c.levelTypes, … = …` reset to `levels = append(…)`, `readsSums` included) into `compileLevels`, and the `unread` block (`unreadHelpers(all)` and the `vcur` exception) into `unreadCells`. `groupStatement` then calls `compileLevels`, builds its legs from the result (`SELECT * FROM o<n>` with `expand.where` for n ≥ 2, the `o0` leg when `withTotal`), calls `layoutLegs`, `unreadCells(levels, computed)`, and renders as today. Its output stays byte-identical.
2. `pagedGroupPass` calls `compileLevels(s, r, rn.expand, nil)` (a paged run has no pivot), takes the shape from `levels[1..G].lv.terms`, reads the cursor (3d), and only then calls `pagedGroupStatement(s, levels, rn.expand, rn.groupCur, limit)`.
3. `pagedGroupStatement` sets `lv.paged = true` on levels 1..G, renders every level with `withoutCells(l.cells, c.unreadCells(levels, nil))` (`levelCTEs`, `o0`'s included: `o1` may read its frame, and `totalPass` reads `o0`), then writes the path CTEs, `pc` and the keyset (`pageCTEs`) and the body. Parameters are named (`s.bind` gives `{:rp<n>}`), so the keyset's binds after the levels' binds need no order.

Then, with `G = len(c.p.groups)`, `cks = c.currencies`:

1. Base column names `base` = `__level`, `__row`, `k0..k(G-1)`, each currency key, every cell name of level 1's cells after `withoutCells(…, unread)` (`splitCell`), `__ord`, `__own`, `__label`, `__labelcur`, `__n`, `__kids`. The union selects these explicitly, so the levels' extra `__s` columns line up.
2. Path CTEs (`pathCTEs`): `pp1 AS (SELECT k0, <cks>, __ord AS __p1 FROM o1)`; for L ≥ 2 `pp<L> AS (SELECT o.k0..o.k(L-1), o.<cks>, pp.__p1..pp.__p(L-1), o.__ord AS __p<L> FROM o<L> o JOIN pp<L-1> pp ON o.k0 IS pp.k0 AND … AND o.k(L-2) IS pp.k(L-2) AND o.<ck> IS pp.<ck>)`.
3. Leg L (1..G): `SELECT o.<base…>, pp.__p1..pp.__pL, NULL AS __p(L+1)..NULL AS __pG, <for every level M and term j: o.__sM_j when M == L, else NULL AS __sM_j> FROM o<L> o JOIN pp<L> pp ON <o.k0..k(L-1), cks IS pp's> WHERE <expand.shownUnder(c, s, L-1, "o.")>` — every ancestor open, folds honored (Task 1), so a row under a closed grandparent is never fetched or counted.
4. Keyset (`groupKeyset(s, cur)`, `"1"` without a cursor). With `d = len(cur.Path)`, `P` = `cur.Group` (its first `d` values the group keys, then the currency keys), and `anchors` = `d` for mode `in`, `d-1` for `after`:
   - **(A) anchors:** for each `L` in `1..anchors`: `(__level = L AND k0 IS ? AND … AND k(L-1) IS ? AND <ck> IS ?…)`.
   - **(B) inside (mode `in` only):** `(__level > d AND k0 IS ? … k(d-1) IS ? AND <ck> IS ?)`.
   - **(C0) a later currency block:** `afterSQL(s, currencyTerms, currencyValues)` where `currencyTerms` = for each currency key `{(ck IS NULL)}, {ck}` and `currencyValues` = for each of `P`'s currency values `[isNull ? 1 : 0, value]`. Absent without currency keys.
   - **(Ci) after the level-i path group among its siblings, i = 1..d:** `(<ck> IS ? … AND k0 IS ? … AND k(i-2) IS ? AND __p<i> > (SELECT c<i> FROM pc))`, where the one-row CTE `pc AS (SELECT (SELECT count(*) FROM o<i> x WHERE x.k0 IS ? … x.k(i-2) IS ? AND x.<ck> IS ? AND NOT <afterSQL(s, xTerms(i), cur.Path[i-1])>) AS c<i>, …)` counts the siblings at or before the path group, read from its order **values** (Ruling R1): `xTerms(i)` = level i's terms with `sql` replaced by `x.__s<i>_<j>`. A row's `__p<i>` is NULL above level i, so (Ci) never holds for a shallower row.
   - The keyset is `(A) OR (B) OR (C0) OR (C1) OR … OR (Cd)`.
5. Body: `SELECT * FROM (<legs UNION ALL>) WHERE <keyset> ORDER BY <for each ck: (ck IS NULL), ck>, __p1, …, __pG LIMIT <limit + 1 + anchors>`; return `levelCTEs` (the rendered `o<L>` CTEs) and `pageCTEs` (the path CTEs and `pc`) apart, so `totalPass` gets the level CTEs only.

**3d. The pass and the cut.** `runner` gains `groupCur *groupCursor`, `groupShape [][]orderTerm`, `rawGroups`, `rawDetails`, `groupFP string`.

```go
// pagedNow is true while this run pages its group rows. Task 4 pages only a
// run that shows no detail row (no innermost group opens); Task 5 drops the
// condition.
func (rn *runner) pagedNow() bool {
	if !rn.pagedGroups() {
		return false
	}
	e, depth := rn.expand, len(rn.c.p.groups)
	if e.all || e.open[depth] {
		return false
	}
	return !slices.ContainsFunc(e.keys, func(k []any) bool { return len(k) == depth+len(rn.c.currencies) })
}
```

`pagedGroupPass`, in this order:
1. `s := newStmt`, `r := c.rows(s)`, probes reset and added as in `groupPass` (`c.condProbes, c.probed = nil, nil`, the filter probe, `r.probeWheres()`).
2. `levels, err := c.compileLevels(s, r, rn.expand, nil)`.
3. `shape` = `levels[i].lv.terms` for i = 1..G; `rn.groupShape = shape`.
4. `detailKeys` = the detail order keys (`c.detailSelect(r, "w")`'s second result; compile it once here, the detail pass compiles its own); `rn.groupFP = groupFingerprint(c.groupLevels(), levelShape(shape), foldPrint(rn.expand), detailKeys)`.
5. With a cursor: `cur, err := readGroupCursor(rn.groupFP, shape, detailKeys, len(groups), len(currencies), rn.opts.Cursor)`, `rn.groupCur = &cur`. A cursor that this run cannot read is `badCursor()` (400 `options.cursor`), as for every cursor.
6. `levelCTEs, pageCTEs, body, err := c.pagedGroupStatement(s, levels, rn.expand, rn.groupCur, int64(rn.pageLimit()))`.
7. `rows := rn.query(s, [r.render()] + levelCTEs + pageCTEs, body)`.
8. `totals := rn.totalPass(s, [r.render()] + levelCTEs)` (not the path CTEs or `pc`).
9. Return `append(rows, totals...)`. No `capLevelRows`.

In `run()`: `groupRows, err := rn.groupPass()` becomes `rn.pagedGroupPass()` when `rn.pagedNow()`. The detail branch is skipped on the paged path in this task (no leaf opens). After `rn.assemble(…)`: `if rn.pagedNow() { rn.cutPage(limit) } else { rn.pageWindow() }`, with `limit` = `runLimits.pageSize`, or `min(opts.Limit, runLimits.maxPage)` when given (extract the three lines of `detailPass` into `(rn *runner) pageLimit() int`).

`assemble` stores each group row's raw map: in `groupWithLabel` (or `group`) set `rn.rawGroups[id] = row` when `rn.rawGroups != nil` (the paged pass makes the map).

```go
// cutPage keeps a paged grouped run's page (J1a, R3): the rows from where the
// cursor left off, up to limit counted rows (group, opening, detail and
// subtotal; a Total or computed row is not counted), then the Totals after
// the slice, as every page ends in them. A group row and its opening row
// stay on one page. The last counted row gives page.next when a counted row
// follows it.
func (rn *runner) cutPage(limit int) {
	rows := rn.res.Rows
	start := rn.pageStart(rows)
	end, counted := start, 0
	for end < len(rows) {
		r := rows[end]
		if isEnd(r) {
			end++
			continue
		}
		take := 1
		if r.Kind == KindGroup && end+1 < len(rows) && rows[end+1].Kind == KindOpening && rows[end+1].Section == r.ID {
			take = 2
		}
		if counted+take > limit && counted > 0 {
			break
		}
		counted += take
		end += take
		if counted >= limit {
			break
		}
	}
	more := slices.ContainsFunc(rows[end:], func(r Row) bool { return !isEnd(r) })
	out := slices.Clone(rows[start:end])
	for _, r := range rows[end:] {
		if isEnd(r) {
			out = append(out, r)
		}
	}
	rn.res.Rows = out
	details := 0
	for _, r := range out {
		if r.Kind == KindDetail {
			details++
		}
	}
	// A run that is one page and read no detail rows keeps a nil page, as it
	// had before J1, so a run that fits one page is byte-identical (R4, the
	// ledger golden: its "collapsed" runs fit one page).
	if !more && rn.opts.Cursor == "" && !rn.readDetails {
		return
	}
	rn.res.Page = &Page{Size: details}
	if more {
		rn.res.Page.Next = rn.cursorAt(lastCounted(rows[start:end]))
	}
}
```

`runner.readDetails bool` is set by the detail pass (Task 5); in this task it stays false. Before J1 a grouped run with an open innermost group always ran the detail pass and so always had a page; keep that: `readDetails` is true whenever the detail pass ran.

`pageStart(rows)`: 0 without a cursor. With one: skip the leading Total rows of currency blocks before the cursor's currency (they were on an earlier page; the cursor's own block's Total is kept: it may be next), then, when there are anchors (`in`: the path group; `after`: its parent), start after the deepest anchor's group row and its opening row. A deepest anchor that is not among the rows (removed between pages) logs `log.WarnContext(rn.ctx, "page start group not among the rows", "group", id)` as `pageWindow` does and starts after the leading Totals; the client's merge dedupes.

`lastCounted(rows)` is the last row that is not `isEnd`. `cursorAt(row)` builds a `groupPos` and calls `groupCursorOf(rn.groupFP, detailKeys, pos, detail)`:
- a detail row: `key` = its section group's `GroupKey`, `mode` `in`, detail = `rn.rawDetails[row.ID]` (Task 5 fills it);
- a group row that is expanded (its opening, if any, is on the page by the rule above) or an opening row: `key` = that group's key, `mode` `in`;
- a closed group row, or a subtotal row: `key` = its group's key, `mode` `after`.
`path` = for each level `i` of the key (`1..depth`): the raw row `rn.rawGroups["g:" + keyString(prefix_i)]`, where `prefix_i` is the key's first `i` group values plus its currency values, read as `[row["__s<i>_0"], …]`. Every enclosing group row is in `rawGroups` (anchors or fetched rows), so a missing one is an engine fault: return `""` and `log.ErrorContext(rn.ctx, "page end group path not among the rows", "group", id)` — `Page.Next` empty then ends the reader's list early, which the paging tests catch.

`markCurrencies(groupRows)` reads the totals among `groupRows`; it gets `append(pageRows, totals...)` as `pagedGroupPass` returns them, unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -count=1`
Expected: PASS for the whole package: the new tests, the paging tests of open reports (still on the old path in this task), and the `ledger_scale` golden **unchanged** (the ledger has one level, `balance_shape_test.go:212`; its `collapsed` runs show only closed groups and fit one page, so they keep a nil `page` and the same rows). A golden failure here is a defect of this task, never a reason to update the golden.

- [ ] **Step 5: Commit** (driver) — `reports: closed groups page by a group keyset (J1a)`

---

### Task 5: Paged group rows with open groups (J1a)

**Files:**
- Modify: `core/server/reports/run.go` (`pagedNow` → `pagedGroups` everywhere and `pagedNow` deleted; `run` detail branch; `detailPass` lines 913–982 paged branch; `openGroups` 987–1000; new `pagedDetailAfter`)
- Modify: `core/server/reports/assemble.go` (`detail` stores `rn.rawDetails`)
- Modify (tests): `core/server/reports/paging_test.go` (the grouped tests from line 229 on), `pagedgroups_test.go`, `having_test.go` (`TestHiddenRowsTakeNoPlaceOnAPage`), `run_test.go`, `balance_shape_test.go`, `mixedcondition_test.go`, `groups_test.go` (the existing tests listed in Step 1), `testdata/ledger_scale.golden.json` + `ledger_scale_test.go` (only runs that now page, reason written)

**Interfaces:**
- Consumes: Task 4 everything; `groupedPages`, `mergedRows`, `withF7`, `withEuroLeo`, `pagedBySpecies`, `feedingsBySpecies`, `bulkFeedings`, `requireRows`, `requireOrderedRows`, `requireDetails`.
- Produces: `(rn *runner) pagedDetailAfter(s *stmt, keys []sortKey, leaves [][]any) (string, error)`; every grouped report with no pivot, layout or nest pages its group rows; a cursor inside an open innermost group carries the detail keyset (`v`).

- [ ] **Step 1: Write the failing tests**

Append to `pagedgroups_test.go`:

```go
// Open groups: pages of every limit join into the one-page run, the open
// groups' details span page boundaries, and every page ends in the Total.
func TestOpenGroupsPageAtEveryLimit(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	whole := z.run(t, "ann", pagedBySpecies, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 12; limit++ {
		pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
		for i, p := range pages {
			if lastTotal(t, p).Cells["cost"] != lastTotal(t, whole).Cells["cost"] {
				t.Fatalf("limit %d page %d: total differs", limit, i+1)
			}
		}
	}
}

// Two open levels page as one document.
func TestTwoOpenLevelsPageAsOneDocument(t *testing.T) {
	z := newZoo(t)
	raw := pagedAnimals(2)
	whole := z.run(t, "ann", raw, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 9; limit++ {
		if got := mergedRows(t, groupedPages(t, z, "ann", raw, Options{Limit: limit})); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages differ from the one-page run", limit)
		}
	}
}

// R3: a page counts group, opening, detail and subtotal rows; the group row
// and its opening share a page, also at limit 1.
func TestAGroupRowAndItsOpeningShareAPage(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 1}, nil)
	requireRows(t, first, "kind", "group", "opening", "total")
	second := z.run(t, "ann", pagedBySpecies, Options{Limit: 1, Cursor: first.Page.Next}, nil)
	requireRows(t, second, "kind", "detail", "total")
	third := z.run(t, "ann", pagedBySpecies, Options{Limit: 3}, nil)
	requireRows(t, third, "kind", "group", "opening", "detail", "total")
}

// A viewer sort orders the details inside their groups; a page that ends
// inside a group goes on in the viewer's order.
func TestAViewerSortPagesInsideItsGroups(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	opts := Options{Sort: []ViewSort{{Col: "cost", Dir: "desc"}}}
	opts.Limit = 1000
	whole := z.run(t, "ann", pagedBySpecies, opts, nil)
	opts.Limit = 3
	if got := mergedRows(t, groupedPages(t, z, "ann", pagedBySpecies, opts)); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
	}
}

// Two currencies: each currency's Total keeps its place on the page that
// holds its block's end, and every page ends in the Totals after its slice.
func TestAPagedGroupPageKeepsEachCurrencyTotalInItsPlace(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 100)
	whole := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 1000}, nil)
	for limit := 1; limit <= 8; limit++ {
		pages := groupedPages(t, z, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
	}
}

// A fold under an open level pages like any closed group.
func TestAFoldedGroupPagesAsOneRow(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	opts := Options{Collapse: [][]any{{"lion", "USD"}}, Limit: 1000}
	whole := z.run(t, "ann", pagedBySpecies, opts, nil)
	opts.Limit = 2
	if got := mergedRows(t, groupedPages(t, z, "ann", pagedBySpecies, opts)); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages differ from the one-page run")
	}
}
```

Rewrite the grouped tests of `paging_test.go` to the J1a contract (a page counts group, opening, detail and subtotal rows; R3, R4):
- `TestAGroupedReportPagesAcrossAGroupBoundary` (`Limit: 2`): page 1 `group, opening, total` (size 0); page 2 `detail f1, detail f2, total`; page 3 `detail f7, subtotal, total`; page 4 `group zebra, opening, total`; page 5 `detail f3, detail f4, total`; page 6 `subtotal, total`. Keep the Total-on-every-page check (`cost 4600`). Rename it `TestAGroupedReportCountsEveryRowKindOnItsPages`; its comment says why the pages changed (J1a).
- `TestARunningSumContinuesOnPageTwo`: `Limit: 4` (group, opening, f1, f2 on page 1); page 2 starts with f7 at 3600.
- `TestAGroupedPageSeamRepeatsNoRow`: unchanged assertions (pages of one row join into the whole; the added f8 after the cursor shows once); `Limit: 2` for the second half keeps it.
- `TestAGroupedPageKeepsEachCurrencyTotalInItsPlace`: replace the per-page row lists by `TestAPagedGroupPageKeepsEachCurrencyTotalInItsPlace` above (delete the old one).
- `TestTheRowCapNoticeIsGivenOnce` (run_test.go:388): a grouped report no longer caps; run it on the pivot `costBySpeciesAndMonth` with `rowCap = 1` (the notice once).
- `TestABalanceShapedReportCutAtAPageSeamGoesOnOnTheNextPage` (balance_shape_test.go:462): at `Limit: 1` page 1 is now Cash's group row and its opening (R3); give it `Limit: 3` so page 1 is the group row, the opening and the 2026-07-01 Rent line, and page 2 starts with 2026-07-02 at −7000 as before; comment: "a page counts group, opening, detail and subtotal rows (J1a)".
- `mixedcondition_test.go:194` asserts per-page notices at `Limit: 1`: rerun it; where the pages now cut elsewhere, set the expected pages to the J1a cut with the reason in a comment, keeping what it pins (a page names only its own rows' mixed columns).
- `TestASixThousandRowGroupedReportPagesToItsEnd`: 6,004 details plus 4 counted group/subtotal rows (lion's and zebra's), `Limit: 1000` → `len(pages) == 7` still holds only if the counted rows fit; assert `details == 6004`, `lionTotals == 1`, no notice, and `len(pages) == (6004 + 4 + 999) / 1000` (group + subtotal rows of lion and zebra counted); write the reason.
- `TestAGroupedPageHoldsAtMostAThousandRows`: count every counted kind on page 1: exactly 1,000; `Page.Size` is the details among them.
- `TestAGroupedCursorIsRefusedAfterASpecChangeOrASortChange`: unchanged cases; `"its group is shut"` is now refused by the fold print (same 400).
- `TestHiddenRowsTakeNoPlaceOnAPage` (having_test.go:67): `pageSize = 3` (a page now counts the group row: zebra's group, f3, f4 are 3 rows; lion's group, f1, f2 would be 3 too, so the guard still fails when a hidden group takes place); assertions unchanged (rows `group zebra, f3, f4, Total`, `res.Page.Next == ""`); comment: "a page counts group, opening, detail and subtotal rows (J1a)".
- `TestGroupRowsStillStopAtTheGroupCap`: becomes `TestAnOpenGroupedReportHasNoGroupCap`: with `rowCap = 3`, `byNote` `ExpandAll` shows all four groups and their details, no `rowCap` notice.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core/server && go test ./reports/ -run 'OpenGroupsPage|TwoOpenLevels|ShareAPage|ViewerSortPages|CurrencyTotalInItsPlace|FoldedGroupPages|CountsEveryRowKind|RunningSumContinues|SixThousandRow|AtMostAThousandRows|NoGroupCap' -count=1`
Expected: FAIL: open runs still take the detail-only paging (Task 4's gate), so page 1 at `Limit: 1` is `group, opening, detail, total` (the detail pass's one row plus the whole group statement), and `TestAnOpenGroupedReportHasNoGroupCap` sees `rowCap`.

- [ ] **Step 3: Implement**

1. Delete `pagedNow`; every use reads `rn.pagedGroups()`. Remove the paged branch of `groupPass` (Task 3): every paged run now goes through `pagedGroupPass`, so `groupPass` runs only when `!rn.pagedGroups()` and always calls `groupStatement(…, true)` and `capLevelRows` as before J1. Keep `totalPass` (`pagedGroupPass` uses it). `groupStatement`'s `withTotal` then has no caller that passes `false` (`pagedGroupPass` builds its statement with `pagedGroupStatement`), so remove the parameter and its two switches, and the `true` argument in `compileDry` and `fill_test.go` `fillKeyset`; the non-paged statement stays byte-identical.
2. `run()`: on the paged path, `leaves := rn.openGroups(groupRows)` (the page's rows: anchors and fetched rows) — but in mode `after` the path group itself was not fetched, so it is never a leaf; the leaves are the page's open innermost groups in document order. Run the detail pass when `len(leaves) > 0` and set `rn.readDetails = true`. A cursor whose mode is `in` with detail values names a leaf that is `leaves[0]` when it still exists (it is the deepest anchor); when it was removed between pages it is not, and the page goes on from the group keyset (`pagedDetailAfter`), never a refusal: a refusal would show the next-page error row (outline R8) whose Retry fails every time.
   Also update the `Options` comment at run.go:26–28: "Cursor is the previous page's page.next; Limit is the page size (at most maxPage). A grouped report without pivot, layout or nest pages its group, opening, detail and subtotal rows in document order (J1a); a pivot, a layout and a nest read their group rows whole."
3. `detailPass` paged branch: the same statement (leaf order `gord`, `leafJoin`, `LIMIT limit+1`), with `where, err = rn.pagedDetailAfter(s, order, leaves)`, no `rn.page`, and no `rn.res.Page` (cutPage sets it). Keep each detail's raw row: in `assemble.go` `detail`, set `rn.rawDetails[id] = row` when the map exists.

```go
// pagedDetailAfter selects the detail rows of a paged page: every row of the
// page's open innermost groups, or, when the cursor stopped after a detail
// row of the first of them, that group's rows after it (F7's keyset) and the
// other groups' rows.
func (rn *runner) pagedDetailAfter(s *stmt, keys []sortKey, leaves [][]any) (string, error) {
	cur := rn.groupCur
	if cur == nil || cur.Values == nil {
		return "1", nil
	}
	// The cursor's group is gone (removed between pages) or no longer a
	// leaf: the group keyset already starts after it, so every row of the
	// page's leaves follows. A fold change never gets here: the fold print
	// refused the cursor.
	if len(leaves) == 0 || keyString(leaves[0]) != keyString(cur.Group) {
		return "1", nil
	}
	g := s.bind(int64(0))
	return fmt.Sprintf("(d.__g > %s OR (d.__g = %s AND %s))", g, g, keysetCond(s, keys, cur.Values)), nil
}
```

The detail keys of the cursor fingerprint (Task 4 `rn.groupFP`) are the ones this pass orders by; read them from the same `detailSelect` result so the two never differ.
4. `cutPage` already handles detail rows (`cursorAt` reads `rn.rawDetails`). Check the window start for mode `in` with detail values: the page starts after the leaf's group row and opening, and the detail pass already starts after the cursor's row.
5. Golden: run `cd core/server && LEDGER_GOLDEN_DUMP=<scratchpad>/before go test ./reports/ -run TestABalanceShapedReportOverManyAccountsKeepsItsOutput -count=1` on the Task 4 tree before this step's code, and again with `<scratchpad>/after` after it; diff the two directories. Only runs that page (`pages`, `all`, `accounts`, `someOpen`, `viewSort`, `allDates`, and `default` where it is longer than 200 rows) may differ, and only by where pages cut and their cursors; a run on one page must be identical. Then `LEDGER_GOLDEN_UPDATE=1 go test ./reports/ -run TestABalanceShapedReportOverManyAccountsKeepsItsOutput -count=1` and, above `ledgerRuns()`, write: "Since J1a (core plan 2026-11-08) a page counts group, opening, detail and subtotal rows, so runs with a limit cut their pages elsewhere; a run that fits one page keeps its output (diffed before the update)."

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -count=1`
Expected: PASS, the fuzz seeds included (`FuzzRuleParity` runs its 40 seeds as tests; `pagedGroupParity` compares merged pages with the one-page run).

- [ ] **Step 5: Commit** (driver) — `reports: open grouped reports page group, opening, detail and subtotal rows (J1a)`

---

### Task 6: #452 pins and rule parity on the paged statements

The tests of this task pin behavior Tasks 3–5 built. Each one's red is shown by a mutation, run through `go test -overlay` on a scratchpad copy (memory p5-trial-balance: never edit repo source to show a red).

**Files:**
- Modify (tests): `core/server/reports/ledger_guard_test.go`, `fillplan_test.go`, `balance_shape_test.go`, `fuzz_test.go`

**Interfaces:**
- Consumes: `observeReads`, `recordSubquery`, `isTotalStatement` (Task 3), `runLedger`, `ledgerPeriod`, `withLedgerScale`, `fillOuterLoops`, `parentLoop`, `runBalances`, `balanceSpec`, `mergedRows`, `groupedPages`, `pagedAnimals`, `pagedShape`, `z.listExpanded`.
- Produces: no new names.

- [ ] **Step 1: Write the tests**

`ledger_guard_test.go`: a new test beside the first one, on a cursor page:

```go
// J1a: a cursor page's level statement keeps the #452 shape (each lookup
// once per group row, the group record joined once per level), and so does
// the Total statement of its own.
func TestACursorPageComputesItsBeginningBalanceOncePerStatement(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 20)
	first, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15", Limit: 7})
	if err != nil || first.Page == nil || first.Page.Next == "" {
		t.Fatalf("first page: %+v, %v", first.Page, err)
	}
	seen := observeReads(t)
	if _, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15", Limit: 7, Cursor: first.Page.Next}); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, st := range *seen {
		kind := "detail"
		switch {
		case isTotalStatement(st.sql):
			kind = "total"
		case strings.Contains(st.sql, "__p1"):
			kind = "page"
		}
		kinds[kind]++
		if n := strings.Count(st.sql, "(SELECT sum("); n > 4 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 4", kind, n)
		}
		for _, m := range recordSubquery.FindAllStringSubmatch(st.sql, -1) {
			if m[1] == "a" {
				t.Errorf("the %s statement reads a group record by a subquery on a level row", kind)
			}
		}
	}
	if kinds["page"] != 1 || kinds["total"] != 1 {
		t.Fatalf("statements %v, want one page and one Total statement", kinds)
	}
}
```

(If the `(SELECT sum(` count of a page statement is higher because `o<L>` is read by its leg and its path CTE, that is the R5 failure the test exists for: `o<L>` must be `AS MATERIALIZED`. The count is of SQL text, so it does not change with materialization; add a plan check as `fillplan_test.go` does if the text count alone cannot see a lookup computed twice: `queryPlan(t, db, st.sql, args)` must show each `o<L>` as `MATERIALIZE` once.)

`fillplan_test.go`: a case in `TestAFillKeysetLoopsOverItsParentRowsFirst`'s table:

```go
{"a cursor page that starts inside fill groups", 2, func(t *testing.T, z *zoo) {
	withEmptyAnimals(t, z)
	opts := Options{ExpandAll: true, Limit: 2}
	first := z.run(t, "ann", sitesThenAnimals, opts, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatal("no second page")
	}
	opts.Cursor = first.Page.Next
	z.run(t, "ann", sitesThenAnimals, opts, nil)
}},
```

and change `fillOuterLoops` callers' expected `keysets` only where they still differ from what Task 3 set (Task 3 Step 4 already moved the counts the Total statement changes; here only the page statement of Tasks 4–5 can change one): a CTE a statement does not read is not in its plan, so count first (`go test -run TestAFillKeysetLoopsOverItsParentRowsFirst -v`) and set each case's count to what the plans show, with a comment per changed case: "the page and the Total statement each read f<L> (J1a: the Total has a statement of its own)". Every keyset still has `p` outer.

`balance_shape_test.go`: a test beside `TestABalanceShapedReportCutAtAPageSeamGoesOnOnTheNextPage`:

```go
// J1a: a seam between two account groups (the page ends on a subtotal) goes
// on with the next account's group row; the pages join into the one-page run.
func TestABalanceShapedReportCutBetweenGroupsGoesOnOnTheNextPage(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	params := map[string]string{"period": glJuly, "rows": `"all"`}
	whole := runBalances(t, z, params)
	end := slices.IndexFunc(whole.Rows, func(r Row) bool { return r.Kind == KindSubtotal })
	if end < 0 {
		t.Fatal("no subtotal")
	}
	opts := Options{Today: "2026-07-15", Limit: end + 1 - countEnds(whole.Rows[:end+1])}
	first := z.run(t, "ann", balanceSpec(), opts, params)
	if last := lastCountedRow(first.Rows); last.Kind != KindSubtotal {
		t.Fatalf("page 1 ends in %+v, want the first account's subtotal", last)
	}
	opts.Cursor = first.Page.Next
	second := z.run(t, "ann", balanceSpec(), opts, params)
	if second.Rows[0].Kind != KindGroup || second.Rows[0].ID == first.Rows[0].ID {
		t.Fatalf("page 2 starts with %+v, want the next account's group row", second.Rows[0])
	}
	pages := []*Result{first}
	for o := opts; ; {
		res := z.run(t, "ann", balanceSpec(), o, params)
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		o.Cursor = res.Page.Next
	}
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("merged pages differ from the one-page run")
	}
}

func countEnds(rows []Row) int {
	n := 0
	for _, r := range rows {
		if isEnd(r) {
			n++
		}
	}
	return n
}

func lastCountedRow(rows []Row) Row {
	for i := len(rows) - 1; i >= 0; i-- {
		if !isEnd(rows[i]) {
			return rows[i]
		}
	}
	return Row{}
}
```

`fuzz_test.go`: `pagedShapes` gains `{1, "fold"}` and `{2, "fold"}` (8 → 10 shapes; the comment's "40 seeds run each one five times" becomes "four times"); `pagedGroupParity` gets a `"fold"` case: no `Expand`, and `opts.Collapse = [][]any{}` plus a random half of the open run's group rows' keys (`z.run(t, user, raw, Options{Limit: 1000}, nil)`). After the merged-equals-whole check, add reader parity of the details: under `"all"`, the merged detail rows' ids (`d:<id>`), sorted, equal the sorted ids `z.listItems(t, mux, "zoo_feedings", user)` lists for the reader — the rows the rule lets the reader see (merged rows come in group order, so compare as sorted lists). `pagedGroupParity` takes `mux http.Handler` for this (pass it from `FuzzRuleParity`).

- [ ] **Step 2: Show each test bites (red by mutation)**

For each mutation, copy the named file at its current state to `<scratchpad>/mut/<file>`, apply the one-line change there, write `<scratchpad>/mut/overlay.json` as `{"Replace": {"<abs path of the repo file>": "<abs path of the copy>"}}`, and run `cd core/server && go test -overlay <scratchpad>/mut/overlay.json ./reports/ -run '<Test>' -count=1`:
- `levels.go` `render`: drop `MATERIALIZED` from `o%d AS MATERIALIZED` → `TestACursorPageComputesItsBeginningBalanceOncePerStatement` FAIL (each `o<L>` planned twice), or, if SQLite still materializes it, record that the guard is the plan check and say so in the task report.
- `fill.go` `parentKeys`: `shownUnder` → `"1"` (every parent) → the new fillplan case FAIL (keyset count or outer loop).
- `assemble.go` `cutPage`: start at 0 always → `TestABalanceShapedReportCutBetweenGroupsGoesOnOnTheNextPage` FAIL (row on two pages).
- `run.go` `groupKeyset`: drop (B) → the fuzz `fold`/`some` shapes FAIL (`mergedRows`: a group's children missing).
Expected: each FAIL as named. Then without the overlay:

- [ ] **Step 3: Run the tests to verify they pass**

Run: `cd core/server && go test ./reports/ -count=1`
Expected: PASS. A failure here is a Task 3–5 defect: fix it at its source in the engine, not in the test.

- [ ] **Step 4: Commit** (driver) — `reports: pin the #452 shapes and rule parity on paged group statements`

---

## Books follow-up from this part

None in this part beyond the outline's "Books plan changes" (R2, R4, `RunOptions.collapse`). The Books Go tests that call `reports.Run` on a grouped report with a `Limit` (the GL paging tests) count pages with the four row kinds after Task 5; that is the brief's Do step 4, after the merge.
