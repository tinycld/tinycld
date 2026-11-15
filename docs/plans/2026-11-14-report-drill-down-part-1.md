# Report drills, open links and the drill panel — Part 1 (Tasks 1–8)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The report engine gains `detail.open` (D2) and `columns[].drill` (D1: the target, access, group-row and pivot-cell `@drill` cells), and the client library gains the D1/D2 types and schema, the builder round trip, the `from` / `d` URL keys with prefixed view options, the 1360 px surface rule, the per-scope UI store and `drill.ts`.

**Architecture:** See the outline, `docs/plans/2026-11-14-report-drill-down.md` (Architecture, Names changed, Rulings R1–R12). In short: `open` compiles beside `__label` in the detail select as `__open` and `assemble` keeps only a path under the report's package; a drill binds in `drill.go` against the shipped target's params, its formula values compile as `dv…` / `dvp…` cells on every leg of the group statement, and `assemble` writes them as `<col>@drill` on the drilled level's group and subtotal rows when the reader may view the target. The client encodes a `@drill` cell as the target's `p-` keys and keeps two report views on one route apart by a key prefix and a store scope.

**Tech Stack:** Go 1.26 (PocketBase, SQLite), TypeScript, zod v4, Zustand, vitest.

**Spec:** `/Users/nas/tmp/tinycld/books/books/docs/specs/report-drill-down-design.md` §3, §4, §9, §10. Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-drill-down/cp/cp0.md`.

Paths are relative to `/Users/nas/tmp/tinycld/books/tinycld` (branch `books/p5-drill-down`). Go tests: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run '<Regex>' -count=1`. Vitest, from `/Users/nas/tmp/tinycld/books/tinycld`: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run <file>`. Typecheck, from the same directory: `PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`. Biome on your own files only: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check <files>`.

## Global Constraints

- No spec version bump: every new field is optional at version 4 (`Load` decodes strictly).
- Core names no package: Go tests use the `zoo_*` fixture and the shipped key `zoo.drill_target`; nothing names Books or `salesstub`.
- Refusals go through `b.bad` (bind, 400) and `invalid` (compile); SQL values are bound with `bind`; logs go through the package's `log` (`endpoint.go`), with `WarnContext(rn.ctx, …)`.
- `groupKey` and `pivotBound` are read only where a `levelCtx` has `drill` set (R7); never wrap a context (`inLevel` type-switches on the concrete type).
- Every leg of the group statement has the same cells: a level, a layout leg and the paged statement all carry every `dv…` / `dvp…` name, `NULL` where they do not compile it.
- Client: biome (4 spaces, single quotes), no `any`, no biome-ignore, no `console.*`; comments say why.
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. A bad open path (another package, `..`, `//`, `?`, a scheme, a space, a trailing `/`) gives the row no `open`, and the run logs once, however many rows hold it — Task 1 `TestDetailOpenDropsABadPathAndLogsOncePerRun`.
2. A drill value that is blank (an animal with no birth day) leaves that row's figure with no `@drill`, never one that opens the target unfiltered — Task 4 `TestADrillValueThatIsBlankLeavesTheCellWithNoTarget`.
3. A reader who may not view the target gets no `drill` on the column and no `@drill` cell on any row — Task 3 `TestADrillTargetTheReaderCannotViewGivesNoDrill`, Task 4 `TestNoDrillCellsWithoutAccessToTheTarget`.
4. Pivot bounds are cut to the range: March 10–31, April 1–30, May 1–20 — Task 5 `TestAPivotCellDrillsToItsBucketCutToTheRange`.
5. The main view never reads a `d.` key and a copied link holds no `from`, `d` or `d.` key — Task 7 tests.

---

### Task 1: D2 Go — `detail.open`

**Files:**
- Modify: `core/server/reports/spec.go` (`Detail`, lines 85–89)
- Modify: `core/server/reports/bind.go` (the `spec.Detail` switch, lines 220–226; `formulaSizes`, after line 499)
- Modify: `core/server/reports/detail.go` (`rowsCtx` fields, line 12; `label`, line 342; add `openPath`)
- Modify: `core/server/reports/run.go` (`Row`, after `Parent`, line 165; `runner`, line 235; `detailSelect`, after line 1776)
- Modify: `core/server/reports/assemble.go` (`detail`, lines 795–811)
- Modify: `core/server/reports/alias.go` (line 49), `core/server/reports/lookup.go` (line 92), `core/server/reports/section_percent.go` (line 167)
- Create: `core/server/reports/detailopen_test.go`

**Interfaces:**
- Consumes: `compileFormula`, `rowsCtx`, `asText` (formula.go 630: a relation in a text operator reads `c.label(e)`), `cellSQL`, `log` (endpoint.go 22), `requireSpecError`, `newZoo`, `z.run`, `z.id`.
- Produces: `Detail.Open *Node` (JSON `open`); `Row.Open string` (JSON `open,omitempty`); `rowsCtx.idText bool` (a relation's text is its id); `openPath(slug, value string) (string, bool)`; `runner.openDropped bool`; the detail select's `__open`; refusals `detail` "detail has a label or an open", `detail.open` "a pivot report has no detail rows to open", `detail.open` "open is a text: the path the row opens".

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/detailopen_test.go`:

```go
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// feedingsOpen lists ann's feedings by note (f1, f2 Leo; f3, f4 Zed) with
// the detail open formula given.
const feedingsOpen = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}],"detail":{"open":%s}}`

// countLogs counts, until the test ends, the log records whose message holds
// msg. The package logger follows slog's default (logging.lazyHandler).
func countLogs(t *testing.T, msg string) *atomic.Int32 {
	t.Helper()
	n := &atomic.Int32{}
	prev := slog.Default()
	slog.SetDefault(slog.New(&countHandler{Handler: prev.Handler(), msg: msg, n: n}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return n
}

type countHandler struct {
	slog.Handler
	msg string
	n   *atomic.Int32
}

func (h *countHandler) Handle(ctx context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.msg) {
		h.n.Add(1)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *countHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &countHandler{Handler: h.Handler.WithAttrs(as), msg: h.msg, n: h.n}
}

func (h *countHandler) WithGroup(name string) slog.Handler {
	return &countHandler{Handler: h.Handler.WithGroup(name), msg: h.msg, n: h.n}
}

// opensOf lists each row's open path as "<kind> <open>".
func opensOf(res *Result) []string {
	out := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, r.Kind+" "+r.Open)
	}
	return out
}

func TestADetailRowOpensItsPath(t *testing.T) {
	z := newZoo(t)
	// Inside open a relation reads as its record's id, not its label (R2).
	spec := fmt.Sprintf(feedingsOpen, `{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"link":"animal"}]}]}`)
	res := z.run(t, "ann", spec, Options{}, nil)
	leo, zed := "detail zoo/animals/"+z.id("leo"), "detail zoo/animals/"+z.id("zed")
	if got, want := opensOf(res), []string{leo, leo, zed, zed, "total "}; !slices.Equal(got, want) {
		t.Fatalf("opens = %v, want %v", got, want)
	}
	raw, err := json.Marshal(res.Rows[len(res.Rows)-1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"open"`) {
		t.Fatalf("a row with no open path writes no open key: %s", raw)
	}
}

func TestDetailOpenDropsABadPathAndLogsOncePerRun(t *testing.T) {
	z := newZoo(t)
	for name, prefix := range map[string]string{
		"another package": "other/x/",
		"dots":            "zoo/../",
		"two slashes":     "zoo//",
		"a query":         "zoo/x?q=",
		"a scheme":        "https://zoo/",
		"a space":         "zoo/a b/",
	} {
		t.Run(name, func(t *testing.T) {
			logs := countLogs(t, "open path")
			spec := fmt.Sprintf(feedingsOpen, fmt.Sprintf(`{"op":"concat","args":[{"value":%q},{"path":[{"field":"note"}]}]}`, prefix))
			res := z.run(t, "ann", spec, Options{}, nil)
			if got, want := opensOf(res), []string{"detail ", "detail ", "detail ", "detail ", "total "}; !slices.Equal(got, want) {
				t.Fatalf("opens = %v, want none", got)
			}
			if n := logs.Load(); n != 1 {
				t.Fatalf("logged %d times, want once per run", n)
			}
		})
	}
}

func TestOpenPathKeepsOnlyAPathOfThePackage(t *testing.T) {
	for value, want := range map[string]bool{
		"zoo/animals/abc123": true,
		"zoo/a-b_c/D9":       true,
		"zoo/":               false,
		"zoo/animals/":       false,
		"zoo":                false,
		"zoos/animals/x":     false,
		"zoo/../x":           false,
		"zoo//x":             false,
		"zoo/x?y":            false,
		"zoo/x#y":            false,
		"https://zoo/x":      false,
		"/zoo/x":             false,
		"":                   false,
	} {
		if _, ok := openPath("zoo", value); ok != want {
			t.Errorf("openPath(zoo, %q) = %v, want %v", value, ok, want)
		}
	}
}

func TestNestedChildRowsOpenNothing(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","path":[{"link":"feedings","mode":"nest"},{"field":"note"}]}],
		"sort":[{"col":"name"}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"field":"name"}]}]}}}`, Options{}, nil)
	parents := 0
	for _, r := range res.Rows {
		switch {
		case r.Parent != "" && r.Open != "":
			t.Fatalf("child row %s opens %q", r.ID, r.Open)
		case r.Kind == KindDetail && r.Parent == "":
			parents++
			if r.Open != "zoo/animals/"+r.Label {
				t.Fatalf("row %s opens %q", r.Label, r.Open)
			}
		}
	}
	if parents == 0 {
		t.Fatal("no parent rows")
	}
}

func TestOnlyDetailRowsOpen(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","subtotals":true,"open":true}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/feedings/"},{"path":[{"field":"note"}]}]}}}`, Options{}, nil)
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
			if !strings.HasPrefix(r.Open, "zoo/feedings/f") {
				t.Fatalf("detail row %s opens %q", r.Label, r.Open)
			}
		} else if r.Open != "" {
			t.Fatalf("%s row %s opens %q", r.Kind, r.Label, r.Open)
		}
	}
	if details != 4 {
		t.Fatalf("detail rows = %d, want 4", details)
	}
}

func TestDetailOpenIsRefusedOnAPivotAndWhenNotText(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ spec, at, contains string }{
		"a pivot": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species"}],
			"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"},
			"detail":{"open":{"value":"zoo/x"}}}`, "detail.open", "no detail rows to open"},
		"a number":   {fmt.Sprintf(feedingsOpen, `{"path":[{"field":"grams"}]}`), "detail.open", "open is a text"},
		"a relation": {fmt.Sprintf(feedingsOpen, `{"path":[{"link":"animal"}]}`), "detail.open", "open is a text"},
		"empty":      {`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"detail":{}}`, "detail", "detail has a label or an open"},
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, c.at, c.contains)
		})
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'Open|OnlyDetailRowsOpen|NestedChildRowsOpenNothing' -count=1`
Expected: FAIL to compile (`r.Open undefined`, `openPath undefined`); after the fields exist, `Load` refuses `open` as an unknown field.

- [ ] **Step 3: Add the spec field and the refusals**

`spec.go`, `Detail`:

```go
// Detail shapes detail rows: Label is a formula on a detail row that labels
// it (the first column's value without it); Open is a text formula on a
// detail row, the app path under the report's package that the row opens
// (D2). A relation in a text operator inside Open reads as its record's id.
type Detail struct {
	Label *Node `json:"label,omitempty"`
	Open  *Node `json:"open,omitempty"`
}
```

`bind.go`, replace the `spec.Detail` switch:

```go
	if d := spec.Detail; d != nil {
		if d.Label == nil && d.Open == nil {
			b.bad("detail", "detail has a label or an open")
		}
		if spec.Pivot != nil && d.Label != nil {
			b.bad("detail.label", "a pivot report has no detail rows to label")
		}
		if spec.Pivot != nil && d.Open != nil {
			b.bad("detail.open", "a pivot report has no detail rows to open")
		}
	}
```

`bind.go`, `formulaSizes`, after the `detail.label` check:

```go
	if d := p.spec.Detail; d != nil && d.Open != nil {
		check("detail.open", p.nodeSize(d.Open, memo, map[string]bool{}, true))
	}
```

Every other formula walk visits `open` beside `label`: `alias.go` line 50 `a.node(base, spec.Detail.Open)`; `lookup.go` line 93 `walk(d.Open, "detail.open")`; `section_percent.go` line 168 `walk(s.Detail.Open, "detail.open", false)`.

- [ ] **Step 4: Compile `open` in the detail select**

`detail.go`, `rowsCtx`, after `reads`:

```go
	// idText reads a relation inside a text operator as its record's id, not
	// its label: a detail row's open path holds ids (R2).
	idText bool
```

`detail.go`, `label`:

```go
func (rc *rowsCtx) label(e expr) string {
	if rc.idText {
		return e.sql
	}
	return rc.r.c.textOf(rc.r.s, e)
}
```

`detail.go`, add at the end:

```go
// openPathRe is the characters an open path may hold: no scheme, query,
// fragment, dot or space reaches the router.
var openPathRe = regexp.MustCompile(`^[A-Za-z0-9_/-]+$`)

// openPath keeps a detail row's open value when it is a path under the
// report's package: "<slug>/" then segments that are none of them empty.
func openPath(slug, v string) (string, bool) {
	rest, ok := strings.CutPrefix(v, slug+"/")
	if !ok || rest == "" || !openPathRe.MatchString(v) || strings.Contains(v, "//") || strings.HasSuffix(v, "/") {
		return "", false
	}
	return v, true
}
```

(Add `"regexp"` to `detail.go`'s imports.)

`run.go`, `detailSelect`, after the `d.Label` block (line 1776):

```go
	if d := c.p.spec.Detail; d != nil && d.Open != nil {
		e, err := compileFormula(&rowsCtx{r: r, visiting: visiting, detail: frame, idText: true}, d.Open, "detail.open")
		if err != nil {
			return "", nil, err
		}
		if e.typ != Text {
			return "", nil, invalid("detail.open", "open is a text: the path the row opens")
		}
		sel = append(sel, cellSQL(e)+" AS __open")
	}
```

- [ ] **Step 5: Write `Row.Open` in assembly**

`run.go`, `Row`, after `Parent`:

```go
	// Open is the app path a detail row opens (Detail.Open), under the
	// report's package; absent when the report has no open or the row's value
	// is not such a path. Only detail rows have one.
	Open string `json:"open,omitempty"`
```

`run.go`, `runner`, after `assembleErr`:

```go
	// openDropped is set once a detail row's open value was dropped, so a run
	// logs a bad open path once, not once per row.
	openDropped bool
```

`assemble.go`, `detail`, push `Open: rn.openOf(row)` in the `Row{…}` literal, and add:

```go
// openOf is a detail row's open path, or "" when the report has none or the
// value is not a path of the report's package (logged once per run).
func (rn *runner) openOf(row map[string]any) string {
	v, _ := row["__open"].(string)
	if v == "" {
		return ""
	}
	slug, _, _ := strings.Cut(rn.c.p.base.Key, ".")
	if path, ok := openPath(slug, v); ok {
		return path
	}
	if !rn.openDropped {
		rn.openDropped = true
		log.WarnContext(rn.ctx, "a detail row's open path is not a path of its package; the row opens nothing", "source", rn.c.p.base.Key, "value", v)
	}
	return ""
}
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'Open|OnlyDetailRowsOpen|NestedChildRowsOpenNothing|DetailLabel|DetailNeeds' -count=1`
Expected: PASS. If `TestNestedChildRowsOpenNothing` fails because the nest hop's JSON differs, copy the nest column of an existing nest test (`command grep -n '"mode":"nest"' core/server/reports/*_test.go | head -3`); keep the assertions.

- [ ] **Step 7: Run the package's tests**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 2: D1 Go — the drill spec, its leaves and its bind refusals

**Files:**
- Modify: `core/server/reports/spec.go` (`Column`, after `Heading`, line 142; `Node`, after `Bound`, line 181)
- Modify: `core/server/reports/formula.go` (`errNodeKind` and `nodeKinds`, lines 486–499; `compileNode` switch, line 360)
- Modify: `core/server/reports/levels.go` (`levelCtx`, line 836)
- Modify: `core/server/reports/bind.go` (`plan`, line 59; `bind`, after `b.sectionPercents(p)`; `formulaSizes`)
- Modify: `core/server/reports/shipped.go` (add `shippedParamDefs`)
- Modify: `core/server/reports/alias.go`, `lookup.go`, `section_percent.go` (visit drill formulas)
- Create: `core/server/reports/drill.go`
- Create: `core/server/reports/drill_test.go`

**Interfaces:**
- Consumes: `b.bad`, `Load`, `shipped` / `shippedMu`, `ParamDef`, `ParamDate` / `ParamAsOf` / `ParamDateRange` / `ParamChoice`, `plan.byID`, `plan.pivot`, `lc.col`, `lc.bind`.
- Produces: `Column.Drill *Drill` (JSON `drill,omitempty`); `Drill{Report string; Levels map[string]map[string]DrillValue; PivotCells map[string]DrillValue}` (JSON `report`, `levels`, `pivotCells,omitempty`); `DrillValue{Formula *Node; Range *DrillRange; Group *Node; Param string}` with `MarshalJSON` / `UnmarshalJSON` (R1); `DrillRange{From, To *Node}` (JSON `from`, `to`); `Node.GroupKey string` (JSON `groupKey,omitempty`), `Node.PivotBound string` (JSON `pivotBound,omitempty`); `type drillLeaves interface { groupKey(id, at string) (expr, error); pivotBound(bound, at string) (expr, error) }`; `levelCtx.drill *drillLeafState`, `type drillLeafState struct { pivot bool; from, to string }`, `(lc *levelCtx) groupKey`, `(lc *levelCtx) pivotBound`; `shippedParamDefs(key string) ([]ParamDef, bool)`; `drillPlan`, `drillPart{param, form, source, at string; node *Node; dp *drillPlan}`; `plan.drills []*drillPlan`; `(b *binder) drills(p *plan)`; `(s *Spec) drillNodes(visit func(n *Node, at string))`; `paramDef(defs []ParamDef, id string) *ParamDef`; `groupNumber(p *plan, id string) int`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/drill_test.go`:

```go
package reports

import (
	"encoding/json"
	"fmt"
	"testing"
)

// drillTarget is the shipped report a zoo drill opens: the reader's feedings
// by note, filtered by a period, animals (grouped by species), one species,
// a day, an as-of day and a note.
const drillTarget = `{"version":4,"source":"zoo.feedings","params":[
	{"id":"period","type":"dateRange","default":{"preset":"allDates"}},
	{"id":"animals","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]}},
	{"id":"species","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"species"}],"label":[{"field":"species"}]}},
	{"id":"fedOn","type":"date"},
	{"id":"asOf","type":"asOf"},
	{"id":"note","type":"text"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"filter":{"op":"and","args":[
		{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		{"op":"in","args":[{"path":[{"link":"animal"},{"field":"id"}]},{"param":"animals"}]},
		{"op":"eq","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"param":"species"}]},
		{"op":"lte","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]}]}}`

// shipDrillTarget registers the target; with an app it also stores it, owned
// by the first admin: published, or unpublished and shared with no one.
func shipDrillTarget(t *testing.T, s *savedZoo, unpublished bool) {
	t.Helper()
	RegisterShipped(Shipped{Key: "zoo.drill_target", Title: "Feedings by animal", Spec: json.RawMessage(drillTarget), Revision: 1, Unpublished: unpublished})
	if s == nil {
		return
	}
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
}

// drilledFeedingsTmpl groups the reader's feedings by species (open, with
// subtotals), then animal; each %s is one column's drill, in column order:
// species (the first), animal, grams, share (a percent), secret (hidden).
const drilledFeedingsTmpl = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","default":{"preset":"allDates"}},{"id":"label","type":"text"}],
	"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]%s},
		{"id":"animal","path":[{"link":"animal"}]%s},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"%s},
		{"id":"share","formula":{"op":"percentOfTotal","args":[{"col":"grams"}],"scale":1}%s},
		{"id":"secret","path":[{"field":"cost"}],"aggregate":"sum","hidden":true%s}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"g","col":"species","subtotals":true,"open":true},{"id":"a","col":"animal"}]}`

// drilledFeedingsOn is the report with drill on column col only.
func drilledFeedingsOn(col, drill string) string {
	at := map[string]string{col: `,"drill":` + drill}
	return fmt.Sprintf(drilledFeedingsTmpl, at["species"], at["animal"], at["grams"], at["share"], at["secret"])
}

// goodDrill opens the target from both levels.
const goodDrill = `{"report":"zoo.drill_target","levels":{
	"g":{"species":{"groupKey":"g"},"period":{"param":"period"}},
	"a":{"animals":{"groupKey":"a"},"species":{"groupKey":"g"},"period":{"param":"period"}}}}`

// validate loads raw and validates it for the zoo.
func (z *zoo) validate(t *testing.T, raw string) error {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return Validate(z.app, spec)
}

func TestADrillValueKeepsItsFormThroughJSON(t *testing.T) {
	in := `{"report":"zoo.drill_target","levels":{"g":{` +
		`"animals":{"group":{"groupKey":"g"}},` +
		`"fedOn":{"op":"max","args":[{"path":[{"field":"fed"}]}]},` +
		`"period":{"range":{"from":{"pivotBound":"from"},"to":{"param":"period","bound":"to"}}},` +
		`"species":{"param":"label"}}},` +
		`"pivotCells":{"period":{"param":"period"}}}`
	var d Drill
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	g := d.Levels["g"]
	if g["animals"].Group == nil || g["fedOn"].Formula == nil || g["period"].Range == nil || g["species"].Param != "label" {
		t.Fatalf("forms = %+v", g)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("round trip\n got %s\nwant %s", out, in)
	}
}

func TestLoadRefusesADrillValueWithAnUnknownKey(t *testing.T) {
	for name, value := range map[string]string{
		"a range":   `{"range":{"from":{"value":"2026-01-01"},"to":{"value":"2026-01-31"},"x":1}}`,
		"a group":   `{"group":{"groupKey":"g","x":1}}`,
		"a formula": `{"op":"max","args":[],"x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{"period":`+value+`}}}`)
			if _, _, err := Load([]byte(raw)); err == nil {
				t.Fatal("Load accepted an unknown key")
			}
		})
	}
}

func TestAGoodDrillValidates(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	if err := z.validate(t, drilledFeedingsOn("grams", goodDrill)); err != nil {
		t.Fatal(err)
	}
}

func TestADrillIsRefusedWhereItCannotOpen(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	RegisterShipped(Shipped{Key: "other.target", Title: "Other", Spec: json.RawMessage(drillTarget), Revision: 1})
	g := func(values string) string {
		return `{"report":"zoo.drill_target","levels":{"g":{` + values + `}}}`
	}
	for name, c := range map[string]struct{ col, drill, at, contains string }{
		"a key no package ships": {"grams", `{"report":"zoo.nope","levels":{"g":{}}}`, "columns.2.drill.report", "no package ships"},
		"another package":        {"grams", `{"report":"other.target","levels":{"g":{}}}`, "columns.2.drill.report", "own package"},
		"no levels":              {"grams", `{"report":"zoo.drill_target","levels":{}}`, "columns.2.drill.levels", "at least one group level"},
		"a level not a group":    {"grams", `{"report":"zoo.drill_target","levels":{"x":{}}}`, "columns.2.drill.levels.x", "not a group"},
		"an undeclared param":    {"grams", g(`"nope":{"param":"period"}`), "columns.2.drill.levels.g.nope", "has no param nope"},
		"a range into a choice":  {"grams", g(`"species":{"range":{"from":{"value":"2026-01-01"},"to":{"value":"2026-01-31"}}}`), "columns.2.drill.levels.g.species", "does not fit param species"},
		"a group into a single":  {"grams", g(`"species":{"group":{"groupKey":"g"}}`), "columns.2.drill.levels.g.species", "does not fit param species"},
		"a formula into text":    {"grams", g(`"note":{"groupKey":"g"}`), "columns.2.drill.levels.g.note", "does not fit param note"},
		"a copy of another type": {"grams", g(`"period":{"param":"label"}`), "columns.2.drill.levels.g.period", "does not fit param period"},
		"a copy of no param":     {"grams", g(`"note":{"param":"nope"}`), "columns.2.drill.levels.g.note", "no param nope to copy"},
		"a range half":           {"grams", g(`"period":{"range":{"from":{"value":"2026-01-01"}}}`), "columns.2.drill.levels.g.period", "a range has a from and a to"},
		"the first column":       {"species", goodDrill, "columns.0.drill", "first column"},
		"a hidden column":        {"secret", goodDrill, "columns.4.drill", "hidden"},
		"a percent":              {"share", goodDrill, "columns.3.drill", "percent"},
		"pivotCells, no pivot":   {"grams", `{"report":"zoo.drill_target","levels":{"g":{}},"pivotCells":{"period":{"param":"period"}}}`, "columns.2.drill.pivotCells", "no pivot"},
		"pivotBound in a level":  {"grams", g(`"period":{"range":{"from":{"pivotBound":"from"},"to":{"pivotBound":"to"}}}`), "columns.2.drill.levels.g.period.from", "only in a drill's pivotCells"},
		"groupKey below":         {"grams", g(`"animals":{"groupKey":"a"}`), "columns.2.drill.levels.g.animals", "below the level"},
		"groupKey of no group":   {"grams", g(`"species":{"groupKey":"x"}`), "columns.2.drill.levels.g.species", "x is not a group"},
	} {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, z.validate(t, drilledFeedingsOn(c.col, c.drill)), 400, c.at, c.contains)
		})
	}
}

func TestGroupKeyAndPivotBoundAreReadOnlyInADrill(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ leaf, contains string }{
		"groupKey":   {`{"groupKey":"g"}`, "groupKey is read only in a drill value"},
		"pivotBound": {`{"pivotBound":"from"}`, "pivotBound is read only in a drill's pivotCells"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
				"groups":[{"id":"g","col":"note"}],
				"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},` + c.leaf + `]}}`
			requireSpecError(t, z.validate(t, raw), 400, "filter.args.1", c.contains)
		})
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'Drill|GroupKeyAndPivotBound' -count=1`
Expected: FAIL to compile (`undefined: Drill`).

- [ ] **Step 3: Add the spec types**

`spec.go`, `Column`, after `Heading`:

```go
	// Drill opens a shipped report of the same package from this column's
	// figures on group and subtotal rows, filtered to the rows behind each
	// figure (D1).
	Drill *Drill `json:"drill,omitempty"`
```

`spec.go`, `Node`, after `Bound`:

```go
	// GroupKey reads, in a drill value only, the key of a group at or above
	// the drilled level (a bucket's key as text, a link's record id).
	GroupKey string `json:"groupKey,omitempty"`
	// PivotBound reads, in a drill's pivotCells only, the first ("from") or
	// last ("to") day of the pivot cell's bucket, cut to the pivot's range.
	PivotBound string `json:"pivotBound,omitempty"`
```

`spec.go`, after `Detail`:

```go
// Drill is a column's drill: the shipped report it opens (Report, a key of
// the report's own package) and the target's param values per drilled
// group level (Levels, by group id, then by target param id). PivotCells,
// on the pivot's cell column only, are laid over a level's values on its
// pivot cells.
type Drill struct {
	Report     string                           `json:"report"`
	Levels     map[string]map[string]DrillValue `json:"levels"`
	PivotCells map[string]DrillValue            `json:"pivotCells,omitempty"`
}

// DrillValue is one of: a formula on the drilled level, a date range from
// two formulas, a whole group of a choice param's options from a formula,
// or a copy of a param of the report as the run was given it (R1, R3, R4).
type DrillValue struct {
	Formula *Node
	Range   *DrillRange
	Group   *Node
	Param   string
}

type DrillRange struct {
	From *Node `json:"from"`
	To   *Node `json:"to"`
}

func (v DrillValue) MarshalJSON() ([]byte, error) {
	switch {
	case v.Range != nil:
		return json.Marshal(struct {
			Range *DrillRange `json:"range"`
		}{v.Range})
	case v.Group != nil:
		return json.Marshal(struct {
			Group *Node `json:"group"`
		}{v.Group})
	case v.Param != "":
		return json.Marshal(struct {
			Param string `json:"param"`
		}{v.Param})
	}
	return json.Marshal(v.Formula)
}

// UnmarshalJSON reads a value's form by its one key (R1): range, group, or
// param as a string; anything else is a formula. Every part decodes
// strictly, as Load decodes the spec: a custom decoder does not inherit
// DisallowUnknownFields.
func (v *DrillValue) UnmarshalJSON(raw []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return err
	}
	if len(keys) == 1 {
		switch {
		case keys["range"] != nil:
			return strictJSON(keys["range"], &v.Range)
		case keys["group"] != nil:
			return strictJSON(keys["group"], &v.Group)
		case keys["param"] != nil:
			if err := json.Unmarshal(keys["param"], &v.Param); err == nil {
				return nil
			}
		}
	}
	return strictJSON(raw, &v.Formula)
}

func strictJSON(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
```

(Add `"bytes"` to `spec.go`'s imports if it lacks it.)

- [ ] **Step 4: Make the leaves nodes of their own kind**

`formula.go`: `errNodeKind` becomes `"a formula node is one of op, col, path, outer, param, value, subtotal, record, groupKey or pivotBound"`; in `nodeKinds` add `n.GroupKey != "", n.PivotBound != ""` to the list of kinds. In `compileNode`'s `switch` (line 360), first case:

```go
	case n.GroupKey != "" || n.PivotBound != "":
		return compileDrillLeaf(c, n, at)
```

`levels.go`, `levelCtx`, after `rows`:

```go
	// drill is set while a drill value compiles on this level: only then are
	// groupKey and pivotBound read (R7).
	drill *drillLeafState
```

- [ ] **Step 5: Create `drill.go` with the leaves, the shipped params and the bind**

`shipped.go`, after `isShipped`:

```go
// shippedParamDefs is the params of the shipped report key, read from its
// spec; false when no package ships key or its spec does not load.
func shippedParamDefs(key string) ([]ParamDef, bool) {
	shippedMu.Lock()
	s, ok := shipped[key]
	shippedMu.Unlock()
	if !ok {
		return nil, false
	}
	spec, _, err := Load(s.Spec)
	if err != nil {
		return nil, false
	}
	return spec.Params, true
}
```

`bind.go`, `plan`, after `percents`:

```go
	// drills binds each drilled column, in column order (drill.go).
	drills []*drillPlan
```

`bind.go`, `bind`: call `b.drills(p)` right after `b.sectionPercents(p)` (the pivot and the groups are bound by then). `bind.go`, `formulaSizes`, at the end:

```go
	p.spec.drillNodes(func(n *Node, at string) {
		if n != nil {
			check(at, p.nodeSize(n, memo, map[string]bool{}, true))
		}
	})
```

`alias.go`, after the `Detail` block: `spec.drillNodes(func(n *Node, _ string) { a.node(base, n) })`. `lookup.go`, at the end of `lookups`: `p.spec.drillNodes(walk)`. `section_percent.go`, after the `Detail` block: `s.drillNodes(func(n *Node, at string) { walk(n, at, false) })`.

Create `core/server/reports/drill.go`:

```go
package reports

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Drill value forms, as a drillPart writes them.
const (
	drillFormula = "formula"
	drillFrom    = "from"
	drillTo      = "to"
	drillGroup   = "group"
	drillParam   = "param"
)

const (
	errGroupKeyOutside   = "groupKey is read only in a drill value"
	errPivotBoundOutside = "pivotBound is read only in a drill's pivotCells"
)

// drillPlan is a drilled column bound to its target: the parts each drilled
// level writes, and the parts its pivot cells lay over them.
type drillPlan struct {
	col    *colPlan
	idx    int // the column's index, which names its cells
	drill  *Drill
	target []ParamDef
	// order is the drilled levels (1 for the first group), ascending.
	order  []int
	levels map[int][]drillPart
	pivot  []drillPart
}

// drillPart is one value a drill writes: param is the target's param, form
// how it is written, node the formula (nil for a copy), source the param a
// copy reads, at where its refusals point, and dp the drill it belongs to.
type drillPart struct {
	param, form, source, at string
	node                    *Node
	dp                      *drillPlan
}

// drillLeaves is a context where a drill value's leaves read: a levelCtx.
type drillLeaves interface {
	groupKey(id, at string) (expr, error)
	pivotBound(bound, at string) (expr, error)
}

// drillLeafState is what a drill value compiles against: for a pivot cell
// part, the bounds of its bucket.
type drillLeafState struct {
	pivot    bool
	from, to string
}

func compileDrillLeaf(c fctx, n *Node, at string) (expr, error) {
	d, ok := c.(drillLeaves)
	switch {
	case !ok && n.GroupKey != "":
		return expr{}, invalid(at, errGroupKeyOutside)
	case !ok:
		return expr{}, invalid(at, errPivotBoundOutside)
	case n.GroupKey != "":
		return d.groupKey(n.GroupKey, at)
	}
	return d.pivotBound(n.PivotBound, at)
}

// groupKey is group id's key on this level's rows: a bucket's key as text,
// else the group column as the level reads it (a link's record id).
func (lc *levelCtx) groupKey(id, at string) (expr, error) {
	if lc.drill == nil {
		return expr{}, invalid(at, errGroupKeyOutside)
	}
	i := groupNumber(lc.lv.c.p, id) - 1
	if i < 0 || i >= lc.lv.n {
		return expr{}, invalid(at, fmt.Sprintf("groupKey %s is not a group at or above this level", id))
	}
	g := lc.lv.c.p.groups[i]
	if g.group.Bucket != "" {
		return expr{sql: fmt.Sprintf("a.k%d", i), typ: Text}, nil
	}
	return lc.col(g.col.col.ID, at)
}

// pivotBound is the first or last day of the pivot cell's bucket.
func (lc *levelCtx) pivotBound(bound, at string) (expr, error) {
	if lc.drill == nil || !lc.drill.pivot {
		return expr{}, invalid(at, errPivotBoundOutside)
	}
	day := lc.drill.from
	if bound == "to" {
		day = lc.drill.to
	}
	return expr{sql: lc.bind(day), typ: Date}, nil
}

// groupNumber is group id's level (1 for the first group), 0 for none.
func groupNumber(p *plan, id string) int {
	for i, g := range p.spec.Groups {
		if g.ID == id {
			return i + 1
		}
	}
	return 0
}

func paramDef(defs []ParamDef, id string) *ParamDef {
	for i := range defs {
		if defs[i].ID == id {
			return &defs[i]
		}
	}
	return nil
}

// isPercentColumn is a column that shows a percent: no rows of its own sit
// behind it to open.
func isPercentColumn(c *Column) bool {
	if c.Format == "percent" {
		return true
	}
	return c.Formula != nil && slices.Contains([]string{"percentOfParent", "percentOfTotal", "percentOfSection"}, c.Formula.Op)
}

// drills binds every drilled column against its target's params; every
// refusal is a 400 at the drill's spec path.
func (b *binder) drills(p *plan) {
	slug, _, _ := strings.Cut(p.spec.Source, ".")
	for i := range p.spec.Columns {
		c := &p.spec.Columns[i]
		if c.Drill == nil {
			continue
		}
		at := fmt.Sprintf("columns.%d.drill", i)
		switch {
		case i == 0:
			b.bad(at, "the first column labels the rows, so it opens no report")
			continue
		case c.Hidden:
			b.bad(at, "a hidden column shows no figure to open")
			continue
		case isPercentColumn(c):
			b.bad(at, "a percent has no rows of its own to open")
			continue
		}
		target, ok := shippedParamDefs(c.Drill.Report)
		if !ok {
			b.bad(at+".report", fmt.Sprintf("no package ships the report %q", c.Drill.Report))
			continue
		}
		if pkg, _, _ := strings.Cut(c.Drill.Report, "."); pkg != slug {
			b.bad(at+".report", "a drill opens a report of the report's own package")
			continue
		}
		dp := &drillPlan{col: p.byID[c.ID], idx: i, drill: c.Drill, target: target, levels: map[int][]drillPart{}}
		b.drillLevels(p, dp, at)
		b.drillPivot(p, dp, at)
		p.drills = append(p.drills, dp)
	}
}

func (b *binder) drillLevels(p *plan, dp *drillPlan, at string) {
	if len(dp.drill.Levels) == 0 {
		b.bad(at+".levels", "a drill opens from at least one group level")
		return
	}
	for _, g := range slices.Sorted(maps.Keys(dp.drill.Levels)) {
		lat := at + ".levels." + g
		n := groupNumber(p, g)
		if n == 0 {
			b.bad(lat, fmt.Sprintf("%s is not a group of the report", g))
			continue
		}
		dp.order = append(dp.order, n)
		dp.levels[n] = b.drillParts(p, dp, dp.drill.Levels[g], lat, n, false)
	}
	slices.Sort(dp.order)
}

func (b *binder) drillPivot(p *plan, dp *drillPlan, at string) {
	if len(dp.drill.PivotCells) == 0 {
		return
	}
	pat := at + ".pivotCells"
	switch {
	case p.pivot == nil:
		b.bad(pat, "the report has no pivot")
	case p.pivot.cell != dp.col:
		b.bad(pat, "pivotCells goes on the pivot's cell column")
	case len(dp.order) > 0:
		// Laid over every drilled level: a groupKey reads a group at or above
		// the highest one.
		dp.pivot = b.drillParts(p, dp, dp.drill.PivotCells, pat, dp.order[0], true)
	}
}

// drillParts checks each value against its target param and splits it into
// the parts it writes.
func (b *binder) drillParts(p *plan, dp *drillPlan, values map[string]DrillValue, at string, level int, pivot bool) []drillPart {
	var parts []drillPart
	for _, id := range slices.Sorted(maps.Keys(values)) {
		v, vat := values[id], at+"."+id
		def := paramDef(dp.target, id)
		if def == nil {
			b.bad(vat, fmt.Sprintf("the report %s has no param %s", dp.drill.Report, id))
			continue
		}
		if msg := drillForm(p, def, v); msg != "" {
			b.bad(vat, msg)
			continue
		}
		for _, part := range valueParts(id, v, vat) {
			part.dp = dp
			b.drillLeafRefs(p, part.node, part.at, level, pivot)
			parts = append(parts, part)
		}
	}
	return parts
}

// drillForm refuses a value form that cannot fit its target param (R3); a
// formula's type is checked when it compiles (drillValueFits).
func drillForm(p *plan, def *ParamDef, v DrillValue) string {
	var fits bool
	switch {
	case v.Range != nil:
		if v.Range.From == nil || v.Range.To == nil {
			return "a range has a from and a to"
		}
		fits = def.Type == ParamDateRange
	case v.Group != nil:
		fits = def.Type == ParamChoice && def.Multiple && def.OptionsFrom != nil && len(def.OptionsFrom.Group) > 0
	case v.Param != "":
		src := paramDef(p.spec.Params, v.Param)
		if src == nil {
			return fmt.Sprintf("the report has no param %s to copy", v.Param)
		}
		fits = src.Type == def.Type && src.Multiple == def.Multiple
	case v.Formula != nil:
		fits = def.Type == ParamDate || def.Type == ParamAsOf || def.Type == ParamChoice
	default:
		return "a drill value is a formula, a range, a group or a param"
	}
	if !fits {
		return fmt.Sprintf("this value does not fit param %s (%s)", def.ID, def.Type)
	}
	return ""
}

func valueParts(id string, v DrillValue, at string) []drillPart {
	switch {
	case v.Range != nil:
		return []drillPart{
			{param: id, form: drillFrom, node: v.Range.From, at: at + ".from"},
			{param: id, form: drillTo, node: v.Range.To, at: at + ".to"},
		}
	case v.Group != nil:
		return []drillPart{{param: id, form: drillGroup, node: v.Group, at: at + ".group"}}
	case v.Param != "":
		return []drillPart{{param: id, form: drillParam, source: v.Param, at: at}}
	}
	return []drillPart{{param: id, form: drillFormula, node: v.Formula, at: at}}
}

// drillLeafRefs refuses a groupKey of no group or of a level below the
// drilled one, and a pivotBound outside pivotCells or without a bucketed
// pivot.
func (b *binder) drillLeafRefs(p *plan, node *Node, at string, level int, pivot bool) {
	walkDrillNode(node, at, func(n *Node, at string) {
		if n.GroupKey != "" {
			switch g := groupNumber(p, n.GroupKey); {
			case g == 0:
				b.bad(at, fmt.Sprintf("%s is not a group of the report", n.GroupKey))
			case g > level:
				b.bad(at, fmt.Sprintf("groupKey %s is below the level the drill opens from", n.GroupKey))
			}
		}
		if n.PivotBound != "" {
			switch {
			case !pivot:
				b.bad(at, errPivotBoundOutside)
			case n.PivotBound != "from" && n.PivotBound != "to":
				b.bad(at, `pivotBound is "from" or "to"`)
			case p.pivot == nil || p.pivot.pivot.Bucket == "":
				b.bad(at, "pivotBound needs a pivot by a bucket of a date")
			}
		}
	})
}

func walkDrillNode(n *Node, at string, visit func(*Node, string)) {
	if n == nil {
		return
	}
	visit(n, at)
	walkDrillNode(n.Where, at+".where", visit)
	for i, a := range n.Args {
		walkDrillNode(a, fmt.Sprintf("%s.args.%d", at, i), visit)
	}
}

// drillNodes visits every formula of the spec's drills with its spec path,
// so each formula walk (aliases, lookups, sizes, section percents) reaches
// them.
func (s *Spec) drillNodes(visit func(n *Node, at string)) {
	for i := range s.Columns {
		d := s.Columns[i].Drill
		if d == nil {
			continue
		}
		at := fmt.Sprintf("columns.%d.drill", i)
		for _, g := range slices.Sorted(maps.Keys(d.Levels)) {
			drillValueNodes(d.Levels[g], at+".levels."+g, visit)
		}
		drillValueNodes(d.PivotCells, at+".pivotCells", visit)
	}
}

func drillValueNodes(values map[string]DrillValue, at string, visit func(*Node, string)) {
	for _, id := range slices.Sorted(maps.Keys(values)) {
		v, vat := values[id], at+"."+id
		switch {
		case v.Range != nil:
			visit(v.Range.From, vat+".from")
			visit(v.Range.To, vat+".to")
		case v.Group != nil:
			visit(v.Group, vat+".group")
		case v.Formula != nil:
			visit(v.Formula, vat)
		}
	}
}
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'Drill|GroupKeyAndPivotBound' -count=1`
Expected: PASS. (No drill value compiles yet: Task 4 compiles them.) If the walkers in `alias.go` / `lookup.go` reject a `nil` node, guard with `if n != nil` in the closure.

- [ ] **Step 7: Run the package's tests**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 3: D1 Go — the target and the reader's access

**Files:**
- Modify: `core/server/reports/shipped.go` (add `drillTargets`)
- Modify: `core/server/reports/run.go` (`ResultColumn`, after `Heading`, line 116; `DrillTarget`; `runner`; `run`, line 398)
- Modify: `core/server/reports/assemble.go` (`columns`, line 42)
- Modify: `core/server/reports/drill_test.go`

**Interfaces:**
- Consumes: `plan.drills` (Task 2), `FindViewableRecord` (reader.go 17), `reportsCollection`, `newSavedZoo`, `shippedRow`, `shipDrillTarget` (Task 2).
- Produces: `type DrillTarget struct { ReportID string \`json:"reportId"\`; Title string \`json:"title"\` }`; `ResultColumn.Drill *DrillTarget` (JSON `drill,omitempty`); `drillTargets(app core.App, info *core.RequestInfo, keys []string) (map[string]*DrillTarget, error)`; `runner.drillTargets map[string]*DrillTarget` (by column id; absent: no access); `(rn *runner) resolveDrills() error`.

- [ ] **Step 1: Write the failing tests**

Add to `core/server/reports/drill_test.go`:

```go
// columnDrill is the drill of the result column id, nil when it has none.
func columnDrill(t *testing.T, res *Result, id string) *DrillTarget {
	t.Helper()
	for _, c := range res.Columns {
		if c.ID == id {
			return c.Drill
		}
	}
	t.Fatalf("no column %s", id)
	return nil
}

func TestADrilledColumnNamesTheTargetTheReaderMayView(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	want := DrillTarget{ReportID: shippedRow(t, s, "zoo.drill_target").Id, Title: "Feedings by animal"}
	if got := columnDrill(t, res, "grams"); got == nil || *got != want {
		t.Fatalf("drill = %+v, want %+v", got, want)
	}
	if got := columnDrill(t, res, "animal"); got != nil {
		t.Fatalf("an undrilled column has drill %+v", got)
	}
	raw, err := json.Marshal(res.Columns)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), fmt.Sprintf(`"drill":{"reportId":%q,"title":"Feedings by animal"}`, want.ReportID)) {
		t.Fatalf("columns JSON: %s", raw)
	}
}

func TestADrillTargetTheReaderCannotViewGivesNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	// Unpublished and shared with no one: only its owner, the admin, sees it.
	shipDrillTarget(t, s, true)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	if got := columnDrill(t, res, "grams"); got != nil {
		t.Fatalf("drill = %+v for a reader who cannot view the target", got)
	}
}

func TestADrillTargetNotStoredYetGivesNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	// Registered, not reconciled: no reports row exists yet.
	shipDrillTarget(t, nil, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	if got := columnDrill(t, res, "grams"); got != nil {
		t.Fatalf("drill = %+v before the target is stored", got)
	}
}
```

(Add `"strings"` to the test file's imports.)

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'DrilledColumnNames|DrillTarget' -count=1`
Expected: FAIL to compile (`c.Drill undefined` on `ResultColumn`).

- [ ] **Step 3: Implement**

`run.go`, `ResultColumn`, after `Heading`:

```go
	// Drill is the report the column's figures open (Column.Drill), absent
	// when the column has none or the reader may not view it; the rows'
	// <id>@drill cells then are absent too.
	Drill *DrillTarget `json:"drill,omitempty"`
```

`run.go`, after `ColumnHeading`:

```go
// DrillTarget is the report a drilled column opens, as the reader sees it.
type DrillTarget struct {
	ReportID string `json:"reportId"`
	Title    string `json:"title"`
}
```

`run.go`, `runner`, after `openDropped`:

```go
	// drillTargets is each drilled column's target the reader may view, by
	// column id; a column the reader may not open is absent.
	drillTargets map[string]*DrillTarget
```

`run.go`, `run`, first statement:

```go
	if err := rn.resolveDrills(); err != nil {
		return err
	}
```

`shipped.go`, at the end:

```go
// drillTargets finds the stored report of each shipped key and keeps those
// the reader may view (R8). A key with no stored report yet, or one the
// reader may not view, maps to nil; any other read error fails the run.
func drillTargets(app core.App, info *core.RequestInfo, keys []string) (map[string]*DrillTarget, error) {
	out := map[string]*DrillTarget{}
	for _, key := range keys {
		if _, seen := out[key]; seen {
			continue
		}
		out[key] = nil
		rec, err := app.FindFirstRecordByFilter(reportsCollection, "shipped_key = {:k}", dbx.Params{"k": key})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, err := FindViewableRecord(app, info, reportsCollection, rec.Id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out[key] = &DrillTarget{ReportID: rec.Id, Title: rec.GetString("title")}
	}
	return out, nil
}
```

`drill.go`, at the end:

```go
// resolveDrills reads, once per run, which drilled columns' targets the
// reader may open.
func (rn *runner) resolveDrills() error {
	drills := rn.c.p.drills
	if len(drills) == 0 {
		return nil
	}
	keys := make([]string, 0, len(drills))
	for _, dp := range drills {
		keys = append(keys, dp.drill.Report)
	}
	targets, err := drillTargets(rn.app, rn.c.info, keys)
	if err != nil {
		return err
	}
	rn.drillTargets = map[string]*DrillTarget{}
	for _, dp := range drills {
		if t := targets[dp.drill.Report]; t != nil {
			rn.drillTargets[dp.col.col.ID] = t
		}
	}
	return nil
}
```

`assemble.go`, `columns`, after `rc.Heading = …`: `rc.Drill = rn.drillTargets[cp.col.ID]`.

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'DrilledColumnNames|DrillTarget|Shipped' -count=1`
Expected: PASS.

- [ ] **Step 5: Hand the diff to the driver** (no commit).

---

### Task 4: D1 Go — `@drill` cells on group and subtotal rows

**Files:**
- Modify: `core/server/reports/rows.go` (`compiler`, add `given`)
- Modify: `core/server/reports/run.go` (`readerCompiler`, line 363; `compileLevels`, lines 1208–1215)
- Modify: `core/server/reports/layout.go` (computed legs, after line 224)
- Modify: `core/server/reports/assemble.go` (`groupWithLabel`, lines 751–783)
- Modify: `core/server/reports/drill.go`
- Modify: `core/server/reports/drill_test.go`

**Interfaces:**
- Consumes: `plan.drills`, `drillPart`, `levelCtx.drill`, `lv.readsRowValue`, `lv.unread`, `cellSQL`, `textOf` (options.go 537), `isDay`, `runner.drillTargets` (Task 3).
- Produces: `compiler.given map[string]json.RawMessage` (the run's posted params); `(c *compiler) givenParam(id string) json.RawMessage`; `(p *plan) eachDrillCell(pivotCount int, cell func(name string, dp *drillPlan, n, j int, part drillPart) error) error`; `(lv *level) drillCells(pivot []pivotValue) ([]string, error)`; `blankDrillCells(p *plan, pivotCount int) []string`; `drillValueFits(def *ParamDef, form string, typ FieldType) bool`; `type DrillParams map[string]json.RawMessage`; `(rn *runner) drillCellsOf(row map[string]any, n int, out map[string]any)`; cell `<col id>@drill` on the drilled level's group and subtotal rows.

- [ ] **Step 1: Write the failing tests**

Add to `core/server/reports/drill_test.go`:

```go
// drillsOf lists every row as "<label> <its col@drill JSON>" ("" for none).
func drillsOf(t *testing.T, res *Result, col string) []string {
	t.Helper()
	out := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, r.Label+" "+drillJSON(t, r, col))
	}
	return out
}

func drillJSON(t *testing.T, r Row, col string) string {
	t.Helper()
	v, ok := r.Cells[col+"@drill"]
	if !ok {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// valueForms drills from both levels with every formula form: a single
// choice and a list item from groupKey, a day and an as-of day from a
// level's dates, a range of them, and a copied param.
const valueForms = `{"report":"zoo.drill_target","levels":{
	"g":{"species":{"groupKey":"g"},"period":{"param":"period"},
		"fedOn":{"op":"max","args":[{"path":[{"field":"fed"}]}]},
		"asOf":{"op":"min","args":[{"path":[{"field":"fed"}]}]}},
	"a":{"animals":{"groupKey":"a"},"species":{"groupKey":"g"},
		"period":{"range":{"from":{"op":"min","args":[{"path":[{"field":"fed"}]}]},"to":{"op":"max","args":[{"path":[{"field":"fed"}]}]}}}}}}`

func TestAGroupRowDrillsWithEveryValueForm(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", valueForms), Options{Today: "2026-06-30"},
		map[string]string{"period": `{"preset":"thisFiscalYearToDate"}`})
	lion := `{"asOf":"2026-01-05","fedOn":"2026-02-10","period":{"preset":"thisFiscalYearToDate"},"species":"lion"}`
	zebra := `{"asOf":"2026-01-20","fedOn":"2026-03-03","period":{"preset":"thisFiscalYearToDate"},"species":"zebra"}`
	leo := fmt.Sprintf(`{"animals":[%q],"period":{"from":"2026-01-05","to":"2026-02-10"},"species":"lion"}`, s.id("leo"))
	zed := fmt.Sprintf(`{"animals":[%q],"period":{"from":"2026-01-20","to":"2026-03-03"},"species":"zebra"}`, s.id("zed"))
	want := []string{"lion " + lion, "Leo " + leo, "Total lion " + lion, "zebra " + zebra, "Zed " + zed, "Total zebra " + zebra, "Total "}
	if got := drillsOf(t, res, "grams"); !slices.Equal(got, want) {
		t.Fatalf("drills\n got %q\nwant %q", got, want)
	}
	if got := drillsOf(t, res, "animal"); slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "{") }) {
		t.Fatalf("an undrilled column has drill cells: %q", got)
	}
}

func TestADrillGroupPickWritesTheGroupReference(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{"animals":{"group":{"groupKey":"g"}}}}}`), Options{}, nil)
	if got := drillJSON(t, res.Rows[0], "grams"); got != `{"animals":[{"group":"lion"}]}` {
		t.Fatalf("lion drill = %s", got)
	}
}

func TestADrillCopiesTheParamAsTheRunWasGivenIt(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	drill := `{"report":"zoo.drill_target","levels":{"g":{"period":{"param":"period"}}}}`
	for name, c := range map[string]struct {
		params map[string]string
		want   string
	}{
		"a preset":  {map[string]string{"period": `{"preset":"thisFiscalYearToDate"}`}, `{"period":{"preset":"thisFiscalYearToDate"}}`},
		"a custom":  {map[string]string{"period": `{"from":"2026-01-01","to":"2026-01-31"}`}, `{"period":{"from":"2026-01-01","to":"2026-01-31"}}`},
		"all dates": {map[string]string{"period": `{"preset":"allDates"}`}, `{"period":{"preset":"allDates"}}`},
		"unposted":  {nil, `{"period":{"preset":"allDates"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			res := s.run(t, "ann", drilledFeedingsOn("grams", drill), Options{Today: "2026-06-30"}, c.params)
			if got := drillJSON(t, res.Rows[0], "grams"); got != c.want {
				t.Fatalf("drill = %s, want %s", got, c.want)
			}
		})
	}
}

func TestADrillValueThatIsBlankLeavesTheCellWithNoTarget(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	// Nub has no birth day: its species row's fedOn is blank.
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "newt", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.feeding(t, "f7", nub, 10, "2026-02-01", 50)
	drill := `{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"},
		"fedOn":{"op":"max","args":[{"path":[{"link":"animal"},{"field":"born"}]}]}}}}`
	res := s.run(t, "ann", drilledFeedingsOn("grams", drill), Options{}, nil)
	species := map[string]bool{"lion": true, "zebra": true, "newt": true}
	for _, r := range res.Rows {
		// Only level g drills; the open animal rows below it never do.
		if r.Kind != KindGroup || !species[r.Label] {
			continue
		}
		got := drillJSON(t, r, "grams")
		if (r.Label == "newt") != (got == "") {
			t.Fatalf("%s drill = %q", r.Label, got)
		}
	}
}

func TestOnlyTheDrilledLevelsRowsDrill(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"}}}}}],
		"groups":[{"id":"g","col":"species","subtotals":true,"open":true}]}`, Options{}, nil)
	drilled := 0
	for _, r := range res.Rows {
		got := drillJSON(t, r, "grams")
		switch {
		case (r.Kind == KindGroup || r.Kind == KindSubtotal) && got != "":
			drilled++
		case r.Kind != KindGroup && r.Kind != KindSubtotal && got != "":
			t.Fatalf("%s row %s drills: %s", r.Kind, r.Label, got)
		}
	}
	if drilled != 4 {
		t.Fatalf("drilled group and subtotal rows = %d, want 4", drilled)
	}
}

func TestNoDrillCellsWithoutAccessToTheTarget(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, true)
	res := s.run(t, "ann", drilledFeedingsOn("grams", valueForms), Options{}, nil)
	for _, r := range res.Rows {
		if got := drillJSON(t, r, "grams"); got != "" {
			t.Fatalf("row %s drills %s for a reader who cannot view the target", r.Label, got)
		}
	}
}

func TestAnAmountCopyKeepsItsCents(t *testing.T) {
	for raw, want := range map[string]string{
		`12000`: `{"amount":12000,"currency":""}`,
		`{"amount":12000,"currency":"USD"}`: `{"amount":12000,"currency":"USD"}`,
		`null`: `null`,
	} {
		if got := string(amountJSON(json.RawMessage(raw))); got != want {
			t.Errorf("amountJSON(%s) = %s, want %s", raw, got, want)
		}
	}
}

func TestAFormulaOfTheWrongTypeIsRefused(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	for name, c := range map[string]struct{ values, at string }{
		"text into a day":     {`"fedOn":{"groupKey":"g"}`, "columns.2.drill.levels.g.fedOn"},
		"a number to choose":  {`"species":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}`, "columns.2.drill.levels.g.species"},
		"text into a range":   {`"period":{"range":{"from":{"groupKey":"g"},"to":{"groupKey":"g"}}}`, "columns.2.drill.levels.g.period.from"},
		"a number to a group": {`"animals":{"group":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}`, "columns.2.drill.levels.g.animals.group"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{`+c.values+`}}}`)
			requireSpecError(t, z.validate(t, raw), 400, c.at, "does not fit")
		})
	}
}
```

(Add `"slices"` to the test file's imports. `s.run`, `s.id`, `s.rec`, `s.feeding` come from the embedded `*zoo`.)

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'GroupRowDrills|GroupPick|CopiesTheParam|ThatIsBlank|OnlyTheDrilled|NoDrillCells|WrongType' -count=1`
Expected: FAIL: rows have no `grams@drill` cells; `Validate` accepts the misfits.

- [ ] **Step 3: Keep the posted params**

`rows.go`, `compiler`, after `params`:

```go
	// given is the run's params as posted, which a drill's param copy writes
	// unchanged (R4); nil in a dry compile.
	given map[string]json.RawMessage
```

`run.go`, `readerCompiler`, replace the last line:

```go
	c := newCompilerWithFiscal(p, app, info, params, fiscal)
	c.given = given
	return c, nil
```

- [ ] **Step 4: Compile the drill cells on every leg**

`drill.go`, add:

```go
// eachDrillCell calls cell for each drill cell in the union's order: its
// name, the drilled level it is compiled on, and its part (j is -1 for a
// level part). A copy writes a posted value, so it has no cell.
func (p *plan) eachDrillCell(pivotCount int, cell func(name string, dp *drillPlan, n, j int, part drillPart) error) error {
	for _, dp := range p.drills {
		for _, n := range dp.order {
			for k, part := range dp.levels[n] {
				if part.form == drillParam {
					continue
				}
				if err := cell(fmt.Sprintf("dv%d_%d_%d", dp.idx, n, k), dp, n, -1, part); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// drillCells are the level's drill cells: compiled on the drilled level,
// NULL on every other, so each leg of the group statement has the same.
func (lv *level) drillCells(pivot []pivotValue) ([]string, error) {
	var out []string
	err := lv.c.p.eachDrillCell(len(pivot), func(name string, dp *drillPlan, n, j int, part drillPart) error {
		sql := "NULL"
		if n == lv.n {
			var err error
			if sql, err = lv.drillPart(dp, part, &drillLeafState{}); err != nil {
				return err
			}
		}
		out = append(out, sql+" AS "+name)
		return nil
	})
	return out, err
}

// drillPart compiles one drill value on this level, refusing a type that
// cannot fit its target param (R3). A value that reads one row's value has
// none on a group row, as an opening cell has none.
func (lv *level) drillPart(dp *drillPlan, part drillPart, st *drillLeafState) (string, error) {
	wheres := len(lv.wheres)
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting, rows: lv.frame, drill: st}, part.node, part.at)
	if err != nil {
		return "", err
	}
	def := paramDef(dp.target, part.param)
	if !drillValueFits(def, part.form, e.typ) {
		return "", invalid(part.at, fmt.Sprintf("this value does not fit param %s (%s): it is %s", def.ID, def.Type, e.typ))
	}
	if lv.readsRowValue(part.node, false, map[string]bool{}) {
		lv.unread(e, wheres)
		return "NULL", nil
	}
	return cellSQL(e), nil
}

func drillValueFits(def *ParamDef, form string, typ FieldType) bool {
	isDate := typ == Date || typ == DateTime
	isChoice := typ == Text || typ == Select || typ == Relation
	switch form {
	case drillFrom, drillTo:
		return isDate
	case drillGroup:
		return isChoice
	}
	switch def.Type {
	case ParamDate, ParamAsOf:
		return isDate
	case ParamChoice:
		return isChoice
	}
	return false
}

// blankDrillCells are every drill cell, NULL: a computed layout row's.
func blankDrillCells(p *plan, pivotCount int) []string {
	var out []string
	// The callback never fails, so neither does the walk.
	_ = p.eachDrillCell(pivotCount, func(name string, _ *drillPlan, _, _ int, _ drillPart) error {
		out = append(out, "NULL AS "+name)
		return nil
	})
	return out
}
```

`run.go`, `compileLevels`, after the `openingCells` call:

```go
		drill, err := lv.drillCells(pivot)
		if err != nil {
			return nil, err
		}
```

and append `drill...` after `opening...` in the `cells = append(…)` line, with the comment updated: "… then the opening cells, then the drill cells follow the pivot cells, as in every leg of the union."

`layout.go`, after the blank opening cells loop (line 224): `cells = append(cells, blankDrillCells(c.p, lc.pivotCount)...)`.

The helper-cell rule of `unreadCells` (`helperCell`, run.go 1272) does not match `dv…`, so the cells stay in every leg; no change there.

- [ ] **Step 5: Write `<col>@drill` in assembly**

`drill.go`, add (with `"encoding/json"` in the imports):

```go
// DrillParams is a <col>@drill cell: the target's param values by param
// id, each in the run request's JSON form.
type DrillParams map[string]json.RawMessage

// givenParam is a source param as the run was given it (R4): its posted
// JSON, else its default, else null (the target clears it).
func (c *compiler) givenParam(id string) json.RawMessage {
	d := paramDef(c.p.spec.Params, id)
	raw, ok := c.given[id]
	if !ok && d != nil {
		raw, ok = d.Default, d.Default != nil
	}
	if !ok {
		return json.RawMessage("null")
	}
	if d != nil && d.Type == ParamAmount {
		return amountJSON(raw)
	}
	return raw
}

// amountJSON writes an amount posted as bare cents in its object form: the
// URL reads a bare number as units, not cents, so a copy must not keep it.
func amountJSON(raw json.RawMessage) json.RawMessage {
	var cents int64
	if json.Unmarshal(raw, &cents) != nil {
		return raw
	}
	return jsonOf(map[string]any{"amount": cents, "currency": ""})
}

// drillCellsOf writes the <col>@drill cells of a group or subtotal row of
// level n, for each column drilled from that level whose target the reader
// may view.
func (rn *runner) drillCellsOf(row map[string]any, n int, out map[string]any) {
	for _, dp := range rn.c.p.drills {
		parts, ok := dp.levels[n]
		if !ok || rn.drillTargets[dp.col.col.ID] == nil {
			continue
		}
		values, ok := rn.drillParams(parts, func(k int) any { return row[fmt.Sprintf("dv%d_%d_%d", dp.idx, n, k)] })
		if !ok {
			continue
		}
		out[dp.col.col.ID+"@drill"] = values
		// Task 5 adds the pivot cells here.
	}
}

// drillParams writes parts' values; false when one is blank (R5): a NULL,
// an empty text, a day that is not a day, or a range that ends before it
// starts.
func (rn *runner) drillParams(parts []drillPart, value func(k int) any) (DrillParams, bool) {
	out := DrillParams{}
	ranges := map[string]*[2]string{}
	for k, part := range parts {
		if part.form == drillParam {
			out[part.param] = rn.c.givenParam(part.source)
			continue
		}
		v := value(k)
		if v == nil {
			return nil, false
		}
		text := textOf(v)
		if text == "" {
			return nil, false
		}
		def := paramDef(part.dp.target, part.param)
		switch {
		case part.form == drillFrom || part.form == drillTo:
			day, ok := dayOf(text)
			if !ok {
				return nil, false
			}
			if ranges[part.param] == nil {
				ranges[part.param] = &[2]string{}
			}
			ranges[part.param][map[string]int{drillFrom: 0, drillTo: 1}[part.form]] = day
		case part.form == drillGroup:
			out[part.param] = jsonOf([]map[string]string{{"group": text}})
		case def.Type == ParamDate || def.Type == ParamAsOf:
			day, ok := dayOf(text)
			if !ok {
				return nil, false
			}
			out[part.param] = jsonOf(day)
		case def.Multiple:
			out[part.param] = jsonOf([]string{text})
		default:
			out[part.param] = jsonOf(text)
		}
	}
	for id, r := range ranges {
		if r[0] > r[1] {
			return nil, false
		}
		out[id] = jsonOf(map[string]string{"from": r[0], "to": r[1]})
	}
	return out, true
}

// dayOf is a stored date's day (its first 10 characters).
func dayOf(s string) (string, bool) {
	if len(s) < 10 || !isDay(s[:10]) {
		return "", false
	}
	return s[:10], true
}

// jsonOf marshals a text, a list of texts or a map of texts, which cannot
// fail.
func jsonOf(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
```

`assemble.go`, `groupWithLabel`: build the group row's cells once and add the drill cells; the subtotal reads the same row:

```go
	cells := rn.cells(row)
	rn.drillCellsOf(row, n, cells)
```

Use `Cells: cells` in the group `Row{…}`; for the subtotal, `subCells := rn.cells(row)` then `rn.drillCellsOf(row, n, subCells)` and `Cells: subCells` (each row owns its map: a client that edits one row's cells must not change the other's).

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'Drill|GroupKeyAndPivotBound|GroupPick|CopiesTheParam|ThatIsBlank|OnlyTheDrilled|NoDrillCells|WrongType' -count=1`
Expected: PASS. If `TestAGroupRowDrillsWithEveryValueForm`'s labels differ only in the subtotal's label text, read one with `rowsOf(res, "label")` and fix the test's labels, not the engine.

- [ ] **Step 7: Run the package's tests**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -count=1`
Expected: PASS (layout, paged and pivot reports keep their cells: the drill cells exist only for drilled columns).

- [ ] **Step 8: Hand the diff to the driver** (no commit).

---

### Task 5: D1 Go — pivot cells, and the drill in the rule parity fuzz

**Files:**
- Modify: `core/server/reports/pivot.go` (add `bucketDays`)
- Modify: `core/server/reports/drill.go` (`eachDrillCell`, `drillCells`, `drillCellsOf`)
- Modify: `core/server/reports/assemble.go` (`pivotColumns`, line 63)
- Modify: `core/server/reports/fuzz_test.go` (`FuzzRuleParity`, lines 203–223)
- Create: `core/server/reports/drillpivot_test.go`

**Interfaces:**
- Consumes: `pivotValue`, `rn.pivotValues`, `c.params[range]` (`from`, `to`), `c.fiscal`, `drillLeafState` (Task 2), `drillParams` (Task 4), `drillTarget`, `shipDrillTarget` (Task 2).
- Produces: `bucketDays(bucket string, key any, fiscalStart int) (first, last string, ok bool)`; cells `dvp<col idx>_<level>_<j>_<k>`; `<cell id>.p<j>@drill` (the level's values with `pivotCells` laid over) and the cell column's `<cell id>@drill` (the level's values alone); pivot `ResultColumn.Drill`; `(z *zoo) drillParity(t *testing.T, user string)`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/drillpivot_test.go`:

```go
package reports

import (
	"testing"
)

// drilledByMonth pivots ann's grams by month of fed over the period's
// months; a pivot cell drills to its month cut to the period.
const drilledByMonth = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"},"period":{"param":"period"}}},
			"pivotCells":{"period":{"range":{"from":{"pivotBound":"from"},"to":{"pivotBound":"to"}}}}}}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams","range":"period"}}`

func TestAPivotCellDrillsToItsBucketCutToTheRange(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledByMonth, Options{}, map[string]string{"period": `{"from":"2026-03-10","to":"2026-05-20"}`})
	lion := res.Rows[0]
	if lion.Label != "lion" {
		t.Fatalf("first row = %s", lion.Label)
	}
	for col, want := range map[string]string{
		"grams.p0": `{"period":{"from":"2026-03-10","to":"2026-03-31"},"species":"lion"}`,
		"grams.p1": `{"period":{"from":"2026-04-01","to":"2026-04-30"},"species":"lion"}`,
		"grams.p2": `{"period":{"from":"2026-05-01","to":"2026-05-20"},"species":"lion"}`,
		// The cell column (the pivot's total) drills with the level's values alone.
		"grams": `{"period":{"from":"2026-03-10","to":"2026-05-20"},"species":"lion"}`,
	} {
		if got := drillJSON(t, lion, col); got != want {
			t.Errorf("%s drill = %s, want %s", col, got, want)
		}
	}
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Drill == nil {
			t.Fatalf("pivot column %s has no drill", c.ID)
		}
	}
}

func TestAPivotCellOfABlankDateHasNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "lion", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.rec(t, "f8", "zoo_feedings", map[string]any{"animal": nub.Id, "grams": 5, "cost": 1, "currency": "USD", "note": "f8"})
	// No range: the columns are the months that have rows, and "(none)".
	res := s.run(t, "ann", drilledByMonth, Options{}, nil)
	for _, c := range res.Columns {
		if c.Pivot == nil || c.Pivot.Key != nil {
			continue
		}
		for _, r := range res.Rows {
			if got := drillJSON(t, r, c.ID); got != "" {
				t.Fatalf("the blank date's column drills on %s: %s", r.Label, got)
			}
		}
		return
	}
	t.Fatal("no blank date column")
}

func TestBucketDays(t *testing.T) {
	for _, c := range []struct {
		bucket      string
		key         any
		fiscal      int
		first, last string
		ok          bool
	}{
		{"day", "2026-03-10", 1, "2026-03-10", "2026-03-10", true},
		{"month", "2026-03", 1, "2026-03-01", "2026-03-31", true},
		{"month", "2028-02", 1, "2028-02-01", "2028-02-29", true},
		{"quarter", "2026-Q2", 1, "2026-04-01", "2026-06-30", true},
		{"year", "2026", 1, "2026-01-01", "2026-12-31", true},
		{"fiscalYear", int64(2026), 1, "2026-01-01", "2026-12-31", true},
		{"fiscalYear", int64(2027), 4, "2026-04-01", "2027-03-31", true},
		{"month", nil, 1, "", "", false},
		{"month", "March", 1, "", "", false},
		{"quarter", "2026-Q5", 1, "", "", false},
		{"fiscalYear", "2026", 1, "", "", false},
	} {
		first, last, ok := bucketDays(c.bucket, c.key, c.fiscal)
		if first != c.first || last != c.last || ok != c.ok {
			t.Errorf("bucketDays(%s, %v, %d) = %s %s %v, want %s %s %v", c.bucket, c.key, c.fiscal, first, last, ok, c.first, c.last, c.ok)
		}
	}
}

func TestPivotCellsAreRefusedOffTheCellColumn(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	raw := `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"}}},"pivotCells":{"period":{"param":"period"}}}}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"}}`
	requireSpecError(t, z.validate(t, raw), 400, "columns.2.drill.pivotCells", "the pivot's cell column")
}
```

`fuzz_test.go`, in `FuzzRuleParity` after `mux := z.mux(f)`:

```go
	// A published drill target, owned by an admin, every reader may open.
	z.person(f, "drill_owner", "admin")
	RegisterShipped(Shipped{Key: "zoo.drill_target", Title: "Feedings by animal", Spec: json.RawMessage(drillTarget), Revision: 1})
	if err := ReconcileShipped(z.app); err != nil {
		f.Fatal(err)
	}
```

and in the fuzz body, after `z.periodPivotParity(…)`: `z.drillParity(t, user)`. Add to `fuzz_test.go`:

```go
// drilledBySpecies totals the reader's grams by species; a row drills to
// the target filtered to its species.
const drilledBySpecies = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"}}}}}],
	"groups":[{"id":"g","col":"species"}]}`

// drillParity: each species row's grams equals the total of the drill
// target run, as the same reader, with that row's @drill params; a row with
// a species always has them.
func (z *zoo) drillParity(t *testing.T, user string) {
	t.Helper()
	res := z.run(t, user, drilledBySpecies, Options{}, nil)
	for _, row := range res.Rows {
		if row.Kind != KindGroup {
			continue
		}
		params, ok := row.Cells["grams@drill"].(DrillParams)
		blank := len(row.GroupKey) == 0 || row.GroupKey[0] == nil || row.GroupKey[0] == ""
		// A guest may not view a published report (the reports read rule), so
		// carol's rows must have no drill; every other reader's must.
		if user == "carol" {
			if ok {
				t.Fatalf("carol, a guest, drills on %s", row.Label)
			}
			continue
		}
		if !ok {
			if !blank {
				t.Fatalf("%s: species %s has no drill", user, row.Label)
			}
			continue
		}
		given := map[string]string{}
		for id, raw := range params {
			given[id] = string(raw)
		}
		target := z.run(t, user, drillTarget, Options{}, given)
		total := target.Rows[len(target.Rows)-1]
		if total.Kind != KindTotal || fmt.Sprint(total.Cells["grams"]) != fmt.Sprint(row.Cells["grams"]) {
			t.Fatalf("%s: species %s grams %v, its drill's total %v (%s)", user, row.Label, row.Cells["grams"], total.Cells["grams"], total.Kind)
		}
	}
}
```

(Add `"encoding/json"` to `fuzz_test.go`'s imports if it lacks it.)

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'PivotCell|BucketDays|FuzzRuleParity' -count=1`
Expected: FAIL: `bucketDays` undefined. After Step 4, if `TestPivotCellsAreRefusedOffTheCellColumn` fails because another pivot rule refuses a second aggregate column first (other `at` or message), remove the `drill` from the fixture, confirm `Validate` accepts it, and fix the fixture (not the drill rule) until only the drill refusal remains.

- [ ] **Step 3: Implement `bucketDays`**

`pivot.go`, after `bucketKey`:

```go
// bucketDays is the first and last day of the bucket whose key is key (in
// bucketKey's forms; a fiscal year is its end year), false for a key that is
// no bucket (a blank date's).
func bucketDays(bucket string, key any, fiscalStart int) (first, last string, ok bool) {
	text, _ := key.(string)
	var start, end time.Time
	switch bucket {
	case "day":
		d, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return "", "", false
		}
		start, end = d, d
	case "month":
		d, err := time.Parse("2006-01", text)
		if err != nil {
			return "", "", false
		}
		start, end = d, d.AddDate(0, 1, -1)
	case "quarter":
		var y, q int
		if n, err := fmt.Sscanf(text, "%4d-Q%d", &y, &q); err != nil || n != 2 || q < 1 || q > 4 {
			return "", "", false
		}
		start = time.Date(y, time.Month(3*q-2), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 3, -1)
	case "year":
		y, err := strconv.Atoi(text)
		if err != nil || len(text) != 4 {
			return "", "", false
		}
		start = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(1, 0, -1)
	case "fiscalYear":
		y, isYear := key.(int64)
		if !isYear {
			return "", "", false
		}
		startYear, month := int(y), max(fiscalStart, 1)
		if month > 1 {
			startYear--
		}
		start = time.Date(startYear, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(1, 0, -1)
	default:
		return "", "", false
	}
	return start.Format(time.DateOnly), end.Format(time.DateOnly), true
}
```

(Add `"strconv"` to `pivot.go`'s imports.)

- [ ] **Step 4: Compile the pivot parts**

`drill.go`, `eachDrillCell`: inside `for _, n := range dp.order`, after the level parts:

```go
			for j := 0; j < pivotCount && len(dp.pivot) > 0; j++ {
				for k, part := range dp.pivot {
					if part.form == drillParam {
						continue
					}
					if err := cell(fmt.Sprintf("dvp%d_%d_%d_%d", dp.idx, n, j, k), dp, n, j, part); err != nil {
						return err
					}
				}
			}
```

`drill.go`, `drillCells`: compile a pivot part with its bucket's bounds:

```go
		if n == lv.n {
			st := &drillLeafState{}
			blank := false
			if j >= 0 {
				st.pivot = true
				st.from, st.to, blank = lv.pivotBounds(pivot[j])
			}
			s, err := lv.drillPart(dp, part, st)
			if err != nil {
				return err
			}
			if !blank {
				sql = s
			}
		}
```

and add:

```go
// placeholderDay stands for the bounds of a pivot value that is no bucket
// (a dry compile's, a blank date's): the part still compiles, so Validate
// refuses a misfit, and its cell is NULL (R6).
const placeholderDay = "0001-01-01"

// pivotBounds is pivot value v's first and last day, cut to the pivot's
// range; blank when v is no bucket.
func (lv *level) pivotBounds(v pivotValue) (from, to string, blank bool) {
	pv := lv.c.p.pivot
	from, to, ok := bucketDays(pv.pivot.Bucket, v.key, lv.c.fiscal)
	if !ok {
		return placeholderDay, placeholderDay, true
	}
	if pv.pivot.Range != "" {
		r := lv.c.params[pv.pivot.Range]
		if r.from != "" && r.from > from {
			from = r.from
		}
		if r.to != "" && r.to < to {
			to = r.to
		}
	}
	return from, to, false
}
```

`drill.go`, `drillCellsOf`: replace the `// Task 5 adds the pivot cells here.` line (after `continue` on a blank level value, so a blank level value leaves every cell of the row with no target) with:

```go
		// Each pivot cell drills with the level's values and its pivot
		// values laid over them; the cell column, the pivot's total, with the
		// level's values alone.
		for j := range rn.pivotValues {
			if len(dp.pivot) == 0 {
				break
			}
			over, ok := rn.drillParams(dp.pivot, func(k int) any { return row[fmt.Sprintf("dvp%d_%d_%d_%d", dp.idx, n, j, k)] })
			if !ok {
				continue
			}
			merged := maps.Clone(values)
			maps.Copy(merged, over)
			out[fmt.Sprintf("%s.p%d@drill", dp.col.col.ID, j)] = merged
		}
```

(Add `"maps"` to `drill.go`'s imports if Task 2 did not.)

`assemble.go`, `pivotColumns`: in the `ResultColumn{…}` literal add `Drill: rn.drillTargets[cell.col.ID],`.

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'PivotCell|BucketDays|Drill|PeriodPivot' -count=1`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -run 'FuzzRuleParity' -count=1`
Expected: PASS over the 60 seeds. If `ReconcileShipped` fails because the fuzz zoo's app lacks the reports collection, read `rlstest.NewAssembledApp` and the start of `newSavedZoo` and add only what `ReconcileShipped` needs (do not call `Register`, which binds routes the fuzz mux already has).

- [ ] **Step 6: Run the package's tests**

Run: `cd /Users/nas/tmp/tinycld/books/tinycld/core/server && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 7: Hand the diff to the driver** (no commit).

---

### Task 6: Client — result types, spec schema and the builder round trip

**Files:**
- Modify: `core/reports/lib/types.ts` (`ResultColumn`, after `heading`, line 89; `ResultRow`, after `parent`, line 104)
- Modify: `core/reports/lib/spec.ts` (`SpecNode`, line 51; `specNodeSchema`, line 71; `specColumnSchema`, line 145; `reportSpecSchema.detail`, line 284)
- Modify: `core/reports/lib/builder/draft.ts` (`columnFields`, line 168; `draftShape`, line 264; `columnDraft`, line 958; `draftFromSpec`, line 1047; `specColumns`, line 1144; `specFromDraft`, lines 1422–1425; builder issues, line 791)
- Modify: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/builder/__tests__/draft.test.ts`

**Interfaces:**
- Consumes: `parseSpec`, `draftFromSpec`, `specFromDraft`, `CATALOG`, `SALES_KEY` (catalog fixture), `META` (draft.test.ts).
- Produces: `DrillTarget { reportId: string; title: string }`, `type DrillParams = Record<string, unknown>`, `ResultColumn.drill?: DrillTarget`, `ResultRow.open?: string` (types.ts); `SpecNode.groupKey?: string`, `SpecNode.pivotBound?: 'from' | 'to'`, `specDrillValueSchema`, `specDrillSchema`, `type SpecDrill`, `SpecColumn.drill?: SpecDrill`, `ReportSpec.detail: { label?, open? }` (spec.ts); `ColumnDraft.drill?: SpecDrill`, `BuilderDraft.detailOpen?: SpecNode` (draft.ts), each written back as it was read.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/spec.test.ts`:

```ts
describe('drills and opened rows (D1, D2)', () => {
    const drill = {
        report: 'zoo.drill_target',
        levels: {
            g: {
                species: { groupKey: 'g' },
                period: { param: 'period' },
                animals: { group: { groupKey: 'g' } },
                fedOn: { op: 'max', args: [{ path: [{ field: 'fed' }] }] },
            },
        },
        pivotCells: { period: { range: { from: { pivotBound: 'from' }, to: { pivotBound: 'to' } } } },
    }
    const withDrill = (value: unknown) => ({
        ...FEEDINGS,
        columns: FEEDINGS.columns.map(c => (c.id === 'cost' ? { ...c, drill: value } : c)),
    })

    it('parses a column drill in every value form', () => {
        expect(parseSpec(withDrill(drill))).toEqual(withDrill(drill))
    })

    it('refuses a drill value with an unknown key', () => {
        const bad = {
            ...drill,
            pivotCells: { period: { range: { from: { value: 1 }, to: { value: 2 }, x: 1 } } },
        }
        expect(parseSpec(withDrill(bad))).toBeNull()
    })

    it('refuses a pivotBound that is not from or to', () => {
        const bad = { report: 'zoo.drill_target', levels: { g: { fedOn: { pivotBound: 'middle' } } } }
        expect(parseSpec(withDrill(bad))).toBeNull()
    })

    it('parses detail.open with or without a label', () => {
        const open = { op: 'concat', args: [{ value: 'zoo/feedings/' }, { path: [{ field: 'note' }] }] }
        expect(parseSpec({ ...FEEDINGS, detail: { open } })?.detail).toEqual({ open })
        expect(parseSpec({ ...FEEDINGS, detail: { label: { col: 'note' }, open } })?.detail).toEqual({
            label: { col: 'note' },
            open,
        })
    })

    it('keeps parsing a spec without them', () => {
        expect(parseSpec(FEEDINGS)).toEqual(FEEDINGS)
    })
})
```

Append to `core/reports/lib/builder/__tests__/draft.test.ts`:

```ts
describe('a drill and an open row in the builder (D1, D2)', () => {
    const DRILLED: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        params: [{ id: 'period', type: 'dateRange', label: 'Day', default: { preset: 'thisMonth' } }],
        columns: [
            { id: 'status', label: 'Status', path: [{ field: 'status' }] },
            {
                id: 'amount',
                label: 'Amount',
                path: [{ field: 'amount' }],
                aggregate: 'sum',
                drill: {
                    report: 'sales.by_status',
                    levels: { byStatus: { status: { groupKey: 'byStatus' }, period: { param: 'period' } } },
                },
            },
        ],
        filter: { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
        groups: [{ id: 'byStatus', col: 'status', subtotals: true }],
        detail: { open: { op: 'concat', args: [{ value: 'sales/sale/' }, { path: [{ field: 'status' }] }] } },
    }

    it('saves the drill and the open path as they were read', () => {
        // toEqual, not a JSON text compare: the key order of specFromDraft's columns is not the spec's.
        const draft = draftFromSpec(DRILLED, META, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(DRILLED)
    })

    it('keeps a label beside the open path', () => {
        const spec: ReportSpec = { ...DRILLED, detail: { label: { col: 'status' }, open: DRILLED.detail?.open } }
        expect(specFromDraft(draftFromSpec(spec, META, CATALOG), CATALOG).detail).toEqual(spec.detail)
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: FAIL: `parseSpec` returns null for a column with `drill`; the builder drops `drill` and `open`.

- [ ] **Step 3: Implement the types and the schema**

`types.ts`, after `CellValue`:

```ts
/** The report a drilled column's figures open (run.go DrillTarget). */
export interface DrillTarget {
    reportId: string
    title: string
}

/**
 * A `<col>@drill` cell: the target's param values by param id, each in the run
 * request's JSON form. Read it only through `drillCellParams` (drill.ts): cells
 * stay typed as `CellValue`, which every other cell is (R12).
 */
export type DrillParams = Record<string, unknown>
```

`ResultColumn`, after `heading`:

```ts
    /** The report this column's figures open; absent when it has none or the reader may not view it. */
    drill?: DrillTarget
```

`ResultRow`, after `parent`:

```ts
    /** The app path a detail row opens (spec `detail.open`). */
    open?: string
```

`spec.ts`, `SpecNode`, after `bound`:

```ts
    /** In a drill value only: the key of a group at or above the drilled level. */
    groupKey?: string
    /** In a drill's pivotCells only: the first or last day of the pivot cell's bucket. */
    pivotBound?: 'from' | 'to'
```

and in `specNodeSchema` add `groupKey: z.string().optional(), pivotBound: z.enum(['from', 'to']).optional(),`.

`spec.ts`, before `specColumnSchema`:

```ts
/**
 * A drill value (spec.go DrillValue, R1): a range from two formulas, a whole
 * group of a choice param's options, a copy of a param of the report, or a
 * formula. A bare `{ param }` is always the copy, as the engine reads it.
 */
export const specDrillValueSchema = z.union([
    z.strictObject({ range: z.strictObject({ from: specNodeSchema, to: specNodeSchema }) }),
    z.strictObject({ group: specNodeSchema }),
    z.strictObject({ param: z.string() }),
    specNodeSchema,
])

/** A column's drill: the shipped report it opens and the target's values per group level. */
export const specDrillSchema = z.strictObject({
    report: z.string(),
    levels: z.record(z.string(), z.record(z.string(), specDrillValueSchema)),
    pivotCells: z.record(z.string(), specDrillValueSchema).optional(),
})
export type SpecDrill = z.infer<typeof specDrillSchema>
```

`specColumnSchema`, after `heading`:

```ts
    // The shipped report the column's figures open, filtered to each figure's rows.
    drill: specDrillSchema.optional(),
```

`reportSpecSchema.detail`: `z.strictObject({ label: specNodeSchema.optional(), open: specNodeSchema.optional() }).nullish()`.

- [ ] **Step 4: Keep them in the builder**

`draft.ts`, `columnFields`, after `hidden`:

```ts
    /** The column's drill, kept as the spec says it: the builder shows it, never edits it (Q10). */
    drill: specDrillSchema.optional(),
```

`draftShape`, after `detailLabel`:

```ts
    /**
     * The path an opened row opens (spec detail.open). Kept as the raw spec node, not a
     * formula draft as detailLabel is: the builder shows it and never edits it (Q10), so
     * a draft round trip could only change it.
     */
    detailOpen: specNodeSchema.optional(),
```

`columnDraft`, in `common` after `hidden`: `...(column.drill ? { drill: column.drill } : {}),`.

`draftFromSpec`, after `detailLabel`: `...(spec.detail?.open ? { detailOpen: spec.detail.open } : {}),`.

`specColumns`, after `hidden`: `if (column.drill) out.drill = column.drill`.

`specFromDraft`, replace the detail block:

```ts
    if (draft.detailLabel || draft.detailOpen) {
        const ctx = formulaContext(draft, catalog, { kind: 'detail' })
        spec.detail = {
            ...(draft.detailLabel ? { label: specFormula(draft.detailLabel, ctx) } : {}),
            ...(draft.detailOpen ? { open: draft.detailOpen } : {}),
        }
    } else if (draft.explicit.detail) spec.detail = {}
```

Builder issues, beside the `PIVOT_DETAIL` issue (line 791): a pivot report has no rows to open, as the engine refuses (bind.go):

```ts
    if (draft.pivot.on && draft.detailOpen)
        issues.push({ path: ['detailOpen'], message: 'A pivot report has no rows to open' })
```

Import `specDrillSchema` and `specNodeSchema` from `'../spec'` where `draft.ts` imports the spec types.

- [ ] **Step 5: Run the tests and the typecheck**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 6: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/lib/types.ts core/reports/lib/spec.ts core/reports/lib/builder/draft.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: no errors.

- [ ] **Step 7: Hand the diff to the driver** (no commit).

---

### Task 7: Client — the `from` and `d` keys, and prefixed view options

**Files:**
- Modify: `core/reports/lib/view-url.ts` (constants, line 33; `parseViewOptions`, lines 134–152; `serializeViewOptions`, lines 160–180)
- Modify: `core/reports/lib/__tests__/view-url.test.ts`

**Interfaces:**
- Consumes: `paramKey`, `encodeParamValue`, `decodeParamValue`, `rangeValue` (params.ts).
- Produces: `FROM_KEY = 'from'`, `DRILL_KEY = 'd'`, `DRILL_PREFIX = 'd.'`; `parseViewOptions(defs, search, prefix = '')`; `serializeViewOptions(defs, options, prefix = '')`; `clearedDrillKeys(search: SearchParams): Record<string, undefined>`; `reportLink` unchanged, pinned to write no `from` / `d` / `d.` key.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/view-url.test.ts` (import `clearedDrillKeys`, `DRILL_KEY`, `DRILL_PREFIX`, `FROM_KEY`, `parseViewOptions`, `reportLink`, `serializeViewOptions` from `'../view-url'` and `rangeValue` from `'../params'`, beside the file's imports):

```ts
describe('two report views on one route (D4)', () => {
    const DEFS: ParamDef[] = [{ id: 'period', type: 'dateRange', default: { preset: 'thisMonth' } }]
    const SEARCH = {
        'p-period': 'lastMonth',
        s: 'amount:asc',
        [DRILL_KEY]: 'r2',
        'd.p-period': '2026-03-01..2026-03-31',
        'd.s': 'amount:desc',
        'd.hide': 'cost',
        [FROM_KEY]: 'r0',
    }

    it('names its keys', () => {
        expect([FROM_KEY, DRILL_KEY, DRILL_PREFIX]).toEqual(['from', 'd', 'd.'])
    })

    it('reads only the prefixed keys under the prefix', () => {
        const drill = parseViewOptions(DEFS, SEARCH, DRILL_PREFIX)
        expect(drill.params.period).toEqual(rangeValue('2026-03-01', '2026-03-31'))
        expect(drill.sort).toEqual({ col: 'amount', dir: 'desc' })
        expect(drill.display.hidden).toEqual(['cost'])
    })

    it('never reads a prefixed key without the prefix', () => {
        const main = parseViewOptions(DEFS, SEARCH)
        expect(main.params.period).toEqual({ kind: 'preset', preset: 'lastMonth' })
        expect(main.sort).toEqual({ col: 'amount', dir: 'asc' })
        expect(main.display.hidden).toEqual([])
    })

    it('writes only prefixed keys under the prefix', () => {
        const written = serializeViewOptions(DEFS, parseViewOptions(DEFS, SEARCH, DRILL_PREFIX), DRILL_PREFIX)
        expect(Object.keys(written).every(key => key.startsWith(DRILL_PREFIX))).toBe(true)
        expect(written['d.p-period']).toBe('2026-03-01..2026-03-31')
        expect(written['d.s']).toBe('amount:desc')
    })

    it('clears every drill key and leaves the others', () => {
        // toStrictEqual: toEqual ignores undefined keys, so it would pass a clear that clears nothing.
        expect(clearedDrillKeys(SEARCH)).toStrictEqual({
            d: undefined,
            'd.p-period': undefined,
            'd.s': undefined,
            'd.hide': undefined,
        })
    })

    it('copies a link with no from, d or d. key', () => {
        const link = reportLink('https://x.test', 'r1', DEFS, parseViewOptions(DEFS, SEARCH))
        const query = new URL(link).searchParams
        expect([...query.keys()].filter(key => key === FROM_KEY || key === DRILL_KEY || key.startsWith(DRILL_PREFIX))).toEqual([])
        expect(query.get('p-period')).toBe('lastMonth')
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/view-url.test.ts`
Expected: FAIL: `DRILL_KEY` is not exported.

- [ ] **Step 3: Implement**

`view-url.ts`, after `SORT_SEPARATOR`:

```ts
/** The report a drilled report was opened from (D3); never part of a copied link. */
export const FROM_KEY = 'from'
/** The report the drill panel shows (D4); its view options carry `DRILL_PREFIX`. */
export const DRILL_KEY = 'd'
export const DRILL_PREFIX = 'd.'
```

`parseViewOptions(defs, search, prefix = '')`: read every key as `` `${prefix}${…}` ``: `` search[`${prefix}${paramKey(def.id)}`] ``, `` search[`${prefix}div1000`] ``, `nocents`, `negative`, `red`, `hide`, and `` search[`${prefix}${SORT_KEY}`] ``. Add the doc line: "With a prefix it reads only that view's keys (the drill panel's, `DRILL_PREFIX`), so two views on one route never read each other's."

`serializeViewOptions(defs, options, prefix = '')`: build `out` as today, then return it with every key prefixed:

```ts
    return Object.fromEntries(Object.entries(out).map(([key, value]) => [`${prefix}${key}`, value]))
```

Add, after `serializeViewOptions`:

```ts
/** Every drill key of the route, cleared: router.setParams removes an undefined key. */
export function clearedDrillKeys(search: SearchParams): Record<string, undefined> {
    const out: Record<string, undefined> = {}
    for (const key of Object.keys(search)) {
        if (key === DRILL_KEY || key.startsWith(DRILL_PREFIX)) out[key] = undefined
    }
    return out
}
```

`reportLink` stays as it is: it writes `serializeViewOptions(defs, options)` with no prefix, so no `from` or `d` key can reach a copied link (R10); add that sentence to its doc comment.

- [ ] **Step 4: Run the tests and the typecheck**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/reports/lib/__tests__/view-url.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 5: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/reports/lib/view-url.ts core/reports/lib/__tests__/view-url.test.ts`
Expected: no errors.

- [ ] **Step 6: Hand the diff to the driver** (no commit).

---

### Task 8: Client — the surface rule, the per-scope store and `drill.ts`

**Files:**
- Create: `core/lib/detail-surface.ts`, `core/lib/use-detail-surface.ts`, `core/lib/__tests__/detail-surface.test.ts`
- Modify: `core/reports/stores/report-ui-store.ts`
- Create: `core/reports/stores/__tests__/report-ui-store.test.ts`
- Modify: `core/reports/hooks/useReportView.ts` (lines 173, 177, 331, 336, 337)
- Create: `core/reports/lib/drill.ts`, `core/reports/lib/__tests__/drill.test.ts`

**Interfaces:**
- Consumes: `useWindowSizeStore` (`@tinycld/core/lib/stores/window-size-store`), `useOrgHref` (`@tinycld/core/lib/org-routes`), `useRouter` / `useLocalSearchParams` (expo-router; vitest maps it to `tests/expo-router-stub.ts`), `textFromCents` (money.ts), `paramKey`, `FROM_KEY`, `DRILL_KEY`, `DRILL_PREFIX`, `clearedDrillKeys`, `SearchParams` (Task 7), `DrillParams`, `ResultColumn`, `ResultRow` (Task 6), `parseViewOptions`, `valueFromJson`.
- Produces: `@tinycld/core/lib/detail-surface` (`DETAIL_PANEL_WIDTH = 480`, `DETAIL_PANEL_MIN_WINDOW = 1360`, `type DetailSurface = 'panel' | 'page'`, `detailSurfaceFor(windowWidth: number): DetailSurface`); `@tinycld/core/lib/use-detail-surface` (`useDetailSurface(): DetailSurface`); `type ReportUIScope = 'main' | 'drill'`; store `scopes: Record<ReportUIScope, { surface: ReportSurface | null; isCustomizeOpen: boolean }>` with `openSurface(surface, scope = 'main')`, `closeSurface(scope = 'main')`, `closeSurfaceOf(kind, scope = 'main')`, `openCustomize(scope = 'main')`, `closeCustomize(scope = 'main')`, `startVisit()` resetting both; `drill.ts`: `encodeDrillValue(value: unknown): string | undefined`, `drillCellParams(column, row): DrillParams | null`, `drillHref(column, row, sourceId): string | null`, `drillPanelParams(column, row, current: SearchParams = {}): Record<string, string | undefined> | null`, `isDrillShown(column, row, search: SearchParams): boolean`, `useOpenDrill({ sourceId, hasPanel }): (column: ResultColumn, row: ResultRow) => void`.

- [ ] **Step 1: Write the failing tests**

Create `core/lib/__tests__/detail-surface.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { DETAIL_PANEL_MIN_WINDOW, DETAIL_PANEL_WIDTH, detailSurfaceFor } from '../detail-surface'

describe('detailSurfaceFor', () => {
    it('shows a record beside the list from 1360 px, as a page below', () => {
        expect(detailSurfaceFor(1359)).toBe('page')
        expect(detailSurfaceFor(1360)).toBe('panel')
        expect([DETAIL_PANEL_MIN_WINDOW, DETAIL_PANEL_WIDTH]).toEqual([1360, 480])
    })
})
```

Create `core/reports/stores/__tests__/report-ui-store.test.ts`:

```ts
import { beforeEach, describe, expect, it } from 'vitest'
import { useReportUIStore } from '../report-ui-store'

const state = () => useReportUIStore.getState()

describe('the report UI store per view', () => {
    beforeEach(() => state().startVisit())

    it('opens a surface in the drill view and leaves the main view alone', () => {
        state().openSurface({ kind: 'param', paramId: 'period' }, 'drill')
        state().openCustomize('drill')
        expect(state().scopes.drill).toEqual({ surface: { kind: 'param', paramId: 'period' }, isCustomizeOpen: true })
        expect(state().scopes.main).toEqual({ surface: null, isCustomizeOpen: false })
    })

    it('opens a surface in the main view and leaves the drill view alone', () => {
        state().openSurface({ kind: 'sort' })
        state().openCustomize()
        expect(state().scopes.main).toEqual({ surface: { kind: 'sort' }, isCustomizeOpen: true })
        expect(state().scopes.drill).toEqual({ surface: null, isCustomizeOpen: false })
    })

    it('closes a surface of a kind only in its own view', () => {
        state().openSurface({ kind: 'sort' })
        state().openSurface({ kind: 'sort' }, 'drill')
        state().closeSurfaceOf('sort', 'drill')
        expect(state().scopes.main.surface).toEqual({ kind: 'sort' })
        expect(state().scopes.drill.surface).toBeNull()
    })

    it('starts a visit with both views closed', () => {
        state().openCustomize()
        state().openSurface({ kind: 'add-filter' }, 'drill')
        state().startVisit()
        expect(state().scopes).toEqual({
            main: { surface: null, isCustomizeOpen: false },
            drill: { surface: null, isCustomizeOpen: false },
        })
    })
})
```

Create `core/reports/lib/__tests__/drill.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { drillCellParams, drillHref, drillPanelParams, encodeDrillValue, isDrillShown } from '../drill'
import { valueFromJson } from '../params'
import type { ParamDef } from '../spec'
import type { DrillParams, ResultColumn, ResultRow } from '../types'
import { parseViewOptions } from '../view-url'

const TARGET: ParamDef[] = [
    { id: 'period', type: 'dateRange' },
    {
        id: 'customers',
        type: 'choice',
        multiple: true,
        optionsFrom: {
            source: 'zoo.sales',
            value: [{ field: 'customer' }],
            label: [{ field: 'customer' }],
            group: [{ field: 'status' }],
        },
    },
    {
        id: 'status',
        type: 'choice',
        options: [
            { value: 'open', label: 'Open' },
            { value: 'paid', label: 'Paid' },
        ],
    },
]

const COLUMN: ResultColumn = {
    id: 'amount',
    label: 'Amount',
    type: 'money',
    drill: { reportId: 'r2', title: 'Sales by customer' },
}

// A @drill cell is not a CellValue (R12): the test builds the cells as the
// server sends them.
function rowWith(drill: DrillParams | undefined): ResultRow {
    const cells: Record<string, unknown> = { amount: 1500 }
    if (drill) cells['amount@drill'] = drill
    return { id: 'g:open', kind: 'group', depth: 0, label: 'Open', cells: cells as ResultRow['cells'] }
}

function searchOf(href: string): Record<string, string> {
    return Object.fromEntries(new URLSearchParams(href.split('?')[1]))
}

const def = (id: string) => TARGET.find(d => d.id === id) as ParamDef

describe('drillHref', () => {
    it('writes each value as the target reads it back, with the source report', () => {
        const drill = {
            period: { from: '2026-03-01', to: '2026-03-31' },
            customers: ['acme'],
            status: 'paid',
        }
        const href = drillHref(COLUMN, rowWith(drill), 'r1')
        expect(href?.startsWith('reports/r2?')).toBe(true)
        const search = searchOf(href ?? '')
        expect(search.from).toBe('r1')
        expect(parseViewOptions(TARGET, search).params).toEqual({
            period: valueFromJson(def('period'), drill.period),
            customers: valueFromJson(def('customers'), ['acme']),
            status: valueFromJson(def('status'), 'paid'),
        })
    })

    it('writes a preset and a group pick', () => {
        const drill = { period: { preset: 'thisFiscalYearToDate' }, customers: [{ group: 'open' }] }
        const search = searchOf(drillHref(COLUMN, rowWith(drill), 'r1') ?? '')
        expect(parseViewOptions(TARGET, search).params).toEqual({
            period: { kind: 'preset', preset: 'thisFiscalYearToDate' },
            customers: valueFromJson(def('customers'), [{ group: 'open' }]),
        })
    })

    it('leaves out a cleared value, so the target takes its default', () => {
        const search = searchOf(drillHref(COLUMN, rowWith({ period: null, status: 'open' }), 'r1') ?? '')
        expect(Object.keys(search).sort()).toEqual(['from', 'p-status'])
    })

    it('gives no href without a @drill cell or a column drill', () => {
        expect(drillHref(COLUMN, rowWith(undefined), 'r1')).toBeNull()
        expect(drillHref({ ...COLUMN, drill: undefined }, rowWith({ status: 'open' }), 'r1')).toBeNull()
        expect(drillCellParams(COLUMN, rowWith(undefined))).toBeNull()
    })
})

describe('encodeDrillValue', () => {
    it('writes each JSON form as the URL writes a param', () => {
        expect(encodeDrillValue('paid')).toBe('paid')
        expect(encodeDrillValue(12)).toBe('12')
        // An amount with no currency comes as an object (Go amountJSON), never bare cents.
        expect(encodeDrillValue({ amount: 12000, currency: '' })).toBe('120.00')
        expect(encodeDrillValue({ preset: 'lastMonth' })).toBe('lastMonth')
        expect(encodeDrillValue({ from: '2026-01-01', to: '2026-01-31' })).toBe('2026-01-01..2026-01-31')
        expect(encodeDrillValue(['a', 'b'])).toBe('["a","b"]')
        expect(encodeDrillValue({ amount: 150000, currency: 'USD' })).toBe('1500.00 USD')
        expect(encodeDrillValue(null)).toBeUndefined()
    })
})

describe('drillPanelParams', () => {
    const drill = { period: { from: '2026-03-01', to: '2026-03-31' }, status: 'paid' }

    it('writes d and the d.-prefixed params, and clears an earlier drill', () => {
        const current = { 'p-period': 'lastMonth', d: 'r9', 'd.p-customers': '["x"]', 'd.s': 'amount:asc' }
        expect(drillPanelParams(COLUMN, rowWith(drill), current)).toStrictEqual({
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
            'd.p-customers': undefined,
            'd.s': undefined,
        })
    })

    it('gives nothing for a figure with no drill', () => {
        expect(drillPanelParams(COLUMN, rowWith(undefined))).toBeNull()
    })

    it('knows the figure whose drill the panel shows', () => {
        const shown = { 'p-period': 'lastMonth', d: 'r2', 'd.p-period': '2026-03-01..2026-03-31', 'd.p-status': 'paid' }
        expect(isDrillShown(COLUMN, rowWith(drill), shown)).toBe(true)
        expect(isDrillShown(COLUMN, rowWith({ ...drill, status: 'open' }), shown)).toBe(false)
        expect(isDrillShown(COLUMN, rowWith(drill), { ...shown, 'd.p-customers': '["x"]' })).toBe(false)
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/lib/__tests__/detail-surface.test.ts core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/__tests__/drill.test.ts`
Expected: FAIL: the modules `../detail-surface` and `../drill` do not exist; `scopes` is undefined.

- [ ] **Step 3: The surface rule**

Create `core/lib/detail-surface.ts`:

```ts
export const DETAIL_PANEL_WIDTH = 480
// Below this window width a 480 px panel leaves the list too narrow, so the record is a page.
export const DETAIL_PANEL_MIN_WINDOW = 1360

export type DetailSurface = 'panel' | 'page'

export function detailSurfaceFor(windowWidth: number): DetailSurface {
    return windowWidth >= DETAIL_PANEL_MIN_WINDOW ? 'panel' : 'page'
}
```

Create `core/lib/use-detail-surface.ts`:

```ts
import { type DetailSurface, detailSurfaceFor } from './detail-surface'
import { useWindowSizeStore } from './stores/window-size-store'

// A primitive selector, so a reader re-renders only when the surface flips.
export function useDetailSurface(): DetailSurface {
    return useWindowSizeStore(s => detailSurfaceFor(s.width))
}
```

(Books keeps its own copy until its checkpoint 1 imports these; core names no package, so nothing here changes Books.)

- [ ] **Step 4: The per-scope store**

`report-ui-store.ts`: replace `surface` / `isCustomizeOpen` and their actions:

```ts
/** The view a surface belongs to: the report, or the report its drill panel shows (D4). */
export type ReportUIScope = 'main' | 'drill'

interface ScopeState {
    surface: ReportSurface | null
    isCustomizeOpen: boolean
}

const CLOSED: ScopeState = { surface: null, isCustomizeOpen: false }
```

In `ReportUIState`, replace the two fields and five actions with:

```ts
    // Each view keeps its own popover and Customize, so a chip of one id in the
    // panel and in the report never opens the other's.
    scopes: Record<ReportUIScope, ScopeState>
    openSurface: (surface: ReportSurface, scope?: ReportUIScope) => void
    closeSurface: (scope?: ReportUIScope) => void
    // Closes the surface only while it is still of this kind: a menu closes itself
    // after an item opened the next surface, and that close must not take the new one.
    closeSurfaceOf: (kind: ReportSurface['kind'], scope?: ReportUIScope) => void
    openCustomize: (scope?: ReportUIScope) => void
    closeCustomize: (scope?: ReportUIScope) => void
```

and in the store:

```ts
const withScope = (state: ReportUIState, scope: ReportUIScope, patch: Partial<ScopeState>) => ({
    scopes: { ...state.scopes, [scope]: { ...state.scopes[scope], ...patch } },
})
```

```ts
    scopes: { main: CLOSED, drill: CLOSED },
    openSurface: (surface, scope = 'main') => set(state => withScope(state, scope, { surface })),
    closeSurface: (scope = 'main') => set(state => withScope(state, scope, { surface: null })),
    closeSurfaceOf: (kind, scope = 'main') =>
        set(state => (state.scopes[scope].surface?.kind === kind ? withScope(state, scope, { surface: null }) : {})),
    openCustomize: (scope = 'main') => set(state => withScope(state, scope, { isCustomizeOpen: true })),
    closeCustomize: (scope = 'main') => set(state => withScope(state, scope, { isCustomizeOpen: false })),
    startVisit: () => set({ scopes: { main: CLOSED, drill: CLOSED }, builderPicker: null }),
```

(`withScope` is declared above `useReportUIStore`; `ReportUIState` is declared before it.)

`useReportView.ts`, line 173: `useReportUIStore(s => s.scopes.main.surface)`; line 177: `useReportUIStore(s => s.scopes.main.isCustomizeOpen) && isFocused`. (Task 13 replaces `'main'` with the view's `uiScope`.)

`useReportView.ts`, lines 331, 336 and 337: the model hands the actions to press handlers (`CloseButton onPress`, `toolbarButton`), and a press event would then arrive as `scope` and write the key `[object Object]`. Wrap each one: `onCloseSurface: () => closeSurface('main')`, `onOpenCustomize: () => openCustomize('main')`, `onCloseCustomize: () => closeCustomize('main')`. Find every other place that passes a store action straight to a handler and wrap it the same way: `command grep -rn 'openSurface\|closeSurface\|openCustomize\|closeCustomize' core/reports --include='*.ts*' | command grep -v __tests__`. Add to `report-ui-store.test.ts`:

```ts
    it('ignores a press event passed where a scope goes', () => {
        const press = { nativeEvent: {} } as unknown as 'main'
        state().openCustomize(press)
        expect(Object.keys(state().scopes).sort()).toEqual(['drill', 'main'])
    })
```

and make each action read its scope through `const scopeOf = (scope: unknown): ReportUIScope => (scope === 'drill' ? 'drill' : 'main')`, so a stray argument opens the main view rather than a third key.

- [ ] **Step 5: `drill.ts`**

Create `core/reports/lib/drill.ts`:

```ts
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useReportUIStore } from '../stores/report-ui-store'
import { textFromCents } from './money'
import type { DrillParams, ResultColumn, ResultRow } from './types'
import { clearedDrillKeys, DRILL_KEY, DRILL_PREFIX, FROM_KEY, paramKey, type SearchParams } from './view-url'

// A drilled figure opens its column's target report filtered by the figure's
// `@drill` cell: the target's params in the run request's JSON form. The figure
// has no target defs, so each value is written as encodeParamValue writes the
// same value (R9); the tests read every form back through parseViewOptions.

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** A param's JSON value as the URL writes it; undefined leaves the key out (the target's default). */
export function encodeDrillValue(value: unknown): string | undefined {
    if (typeof value === 'string') return value
    if (typeof value === 'number') return String(value)
    if (Array.isArray(value)) return JSON.stringify(value)
    if (!isRecord(value)) return undefined
    if (typeof value.preset === 'string') return value.preset
    if (typeof value.from === 'string' && typeof value.to === 'string') return `${value.from}..${value.to}`
    if (typeof value.amount === 'number') {
        const text = textFromCents(value.amount)
        return typeof value.currency === 'string' && value.currency !== '' ? `${text} ${value.currency}` : text
    }
    return undefined
}

/** The figure's drill params, or null when its column has no drill or the row no `@drill` cell. */
export function drillCellParams(column: ResultColumn, row: ResultRow): DrillParams | null {
    if (!column.drill) return null
    const cell: unknown = row.cells[`${column.id}@drill`]
    return isRecord(cell) ? cell : null
}

function drillKeys(params: DrillParams, prefix: string): Record<string, string> {
    const out: Record<string, string> = {}
    for (const [id, value] of Object.entries(params)) {
        const text = encodeDrillValue(value)
        if (text !== undefined) out[`${prefix}${paramKey(id)}`] = text
    }
    return out
}

/** The full report's screen path for a figure, with the source report (D3); wrap it with orgHref. */
export function drillHref(column: ResultColumn, row: ResultRow, sourceId: string): string | null {
    const params = drillCellParams(column, row)
    if (!column.drill || !params) return null
    const query = new URLSearchParams({ ...drillKeys(params, ''), [FROM_KEY]: sourceId })
    return `reports/${column.drill.reportId}?${query.toString()}`
}

/** The route params that show a figure's drill in the panel; every older drill key is cleared. */
export function drillPanelParams(
    column: ResultColumn,
    row: ResultRow,
    current: SearchParams = {}
): Record<string, string | undefined> | null {
    const params = drillCellParams(column, row)
    if (!column.drill || !params) return null
    return { ...clearedDrillKeys(current), [DRILL_KEY]: column.drill.reportId, ...drillKeys(params, DRILL_PREFIX) }
}

function firstOf(value: string | string[] | undefined): string | undefined {
    return Array.isArray(value) ? value[0] : value
}

/** Whether the panel shows exactly this figure's drill (the figure is then marked). */
export function isDrillShown(column: ResultColumn, row: ResultRow, search: SearchParams): boolean {
    const wanted = drillPanelParams(column, row)
    if (!wanted) return false
    // Only the panel's report and params count: a change of the panel's sort keeps the mark.
    const shown = Object.keys(search).filter(
        key => (key === DRILL_KEY || key.startsWith(`${DRILL_PREFIX}${paramKey('')}`)) && search[key] !== undefined
    )
    return shown.length === Object.keys(wanted).length && shown.every(key => firstOf(search[key]) === wanted[key])
}

/**
 * A figure press: beside the report when the screen hosts a panel and the window
 * is wide enough (Customize closes, one side surface at a time), else the full
 * report as its own screen.
 */
export function useOpenDrill({ sourceId, hasPanel }: { sourceId: string; hasPanel: boolean }) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const closeCustomize = useReportUIStore(s => s.closeCustomize)
    return (column: ResultColumn, row: ResultRow) => {
        if (hasPanel && surface === 'panel') {
            const next = drillPanelParams(column, row, search)
            if (!next) return
            router.setParams(next)
            closeCustomize('main')
            return
        }
        const href = drillHref(column, row, sourceId)
        if (href) router.push(orgHref(href))
    }
}
```

If `orgHref` does not keep a query string in its path argument, split the path and pass the query as its params object (`orgHref('reports/[id]', { id, ...query })`, see `org-routes.ts` `useOrgHref` usage); keep `drillHref`'s return value as tested.

- [ ] **Step 6: Run the tests and the typecheck**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/vitest run core/lib/__tests__/detail-surface.test.ts core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/__tests__/drill.test.ts core/reports/lib/__tests__/view-url.test.ts`
Expected: PASS. If the money format of `textFromCents(150000)` is not `1500.00`, read `money.test.ts` and fix the test's expected text, not `encodeDrillValue` (it must match `encodeParamValue`).

Run: `cd /Users/nas/tmp/tinycld/books/tinycld && PATH=/Users/nas/tmp/tinycld/books/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: no errors (every store caller compiles: actions keep their call shape, the two selectors moved to `scopes.main`).

- [ ] **Step 7: Biome**

Run: `/Users/nas/tmp/tinycld/books/node_modules/.bin/biome check core/lib/detail-surface.ts core/lib/use-detail-surface.ts core/lib/__tests__/detail-surface.test.ts core/reports/stores/report-ui-store.ts core/reports/stores/__tests__/report-ui-store.test.ts core/reports/hooks/useReportView.ts core/reports/lib/drill.ts core/reports/lib/__tests__/drill.test.ts`
Expected: no errors.

- [ ] **Step 8: Hand the diff to the driver** (no commit).
