# Report Column Fit, Spec v4 — Part 1 (Tasks 1–7) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The report definition owns column fit: spec version 4 adds `display.oneLine.minWidth`, `display.columns` (CSS-like per-column `width`, `minWidth`, `maxWidth`, `whiteSpace`, `lineClamp`, `textOverflow`, `flexGrow`) and a per-layout `minWidth`; the engine checks them and refuses any setting that could cut a figure; version 4 is the only spec version.

**Architecture:** Go checks the definition only (`core/server/reports/fit.go`, `display.go`); the client measures, places and fits (`core/reports/lib`, Part 2). The placement and cut rules that lived twice (Go `display.go` and TS `table-lines.ts`) live only in TS after Task 3. There is no upgrade path: after Task 7, `Load` reads version 4 alone, and every fixture says version 4.

**Tech Stack:** Go (PocketBase fork, ozzo-validation), TypeScript (zod v4, vitest), Playwright.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-design.md` (§3 definition fields, §3.3 figures never cut, §3.4 Go validation) with the binding decisions `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-decisions.md` (option B; no TanStack; Go/TS duplicate removed; marker; answers 1–6; **no backward compatibility**). Outline of all parts: `docs/plans/2026-11-08-report-column-fit-v4-outline.md`. Part 2 (`docs/plans/2026-11-08-report-column-fit-v4-part-2.md`, Tasks 8–14) covers the TS schema, measure, fit, marker, wrap, builder, e2e and help. Style: `docs/plans/2026-11-06-report-engine-v3.md`, `docs/plans/2026-11-07-report-layout-fit-part-1.md`.

All paths are relative to the core repo `/Users/nas/tmp/tinycld/books-purchases/tinycld` unless absolute. Line numbers are at main `f018575f`.

---

## For review

### No backward compatibility (human, 2026-11-08)

"We do not care about backward compat, this is not live." So: one spec version (4); no v3 → v4 upgrade; no compatibility shims; fixtures move to `"version":4` directly. To keep every task green, Task 1 raises `CurrentVersion` to 4 with the old identity steps still in place, Tasks 4–6 move the test fixtures, and Task 7 deletes the upgrade machinery (`upgradeSteps`, `upgrade`, `loadWith`, `sameExceptVersion`, `LoadInfo.UpgradedFrom`, `Notice.FromVersion`, the `upgraded` notice, TS `fromVersion`) and makes `Load` refuse every version but 4.

Defaults: no default changes in Part 1. Go accepts every setting the design lists with the decisions' limits (`textOverflow` takes `ellipsis` only, decision 5; no per-width column settings, decision 6).

### Tasks 4–6 break the 8-file rule on purpose

Each changes about 20 test files, but every change is one `Edit` with `replace_all` of a literal (`"version":3` → `"version":4`) and nothing else. A reviewer checks one grep. Splitting them further would add review cost without risk reduction.

### Tests this part breaks (and where they are fixed)

| Assertion | Broken by | Fixed in |
|---|---|---|
| `core/server/reports/saved_test.go:65` (posts `{"version":4}` to get a refusal) | T1 | T1 (posts 5) |
| `core/server/reports/spec_test.go:107` ("newer than this server reads (3)") | T1 | T1; rewritten T7 |
| `core/server/reports/v2_test.go:80-82`, `:283`, `:295-296` (stored/loaded version 3) | T1 | T1; upgrade tests deleted T7 |
| `core/server/reports/v3_test.go:53-54`, `:65`, `:81-82`, `:93`, `:98-100`, `:111-112` | T1 | T1; upgrade tests deleted T7 |
| `core/reports/lib/__tests__/spec.test.ts:70-75` (refuses version 4) | T1 | T1; T7 |
| `core/reports/lib/builder/__tests__/draft.test.ts:1500` (`saved.version` is 3) | T1 | T1 |
| `core/server/reports/v2_test.go:136-185` rows "no room on line 1", "overlaps"; `TestDisplayRefusesAPinwheel` `:185-197`; `TestPlaceCells*` `:199-229`; `display_cases_test.go` | T3 | T3 (rows move to the accepted list; tests deleted) |
| `core/reports/lib/__tests__/table-lines.test.ts:405` (`lines(360, [refused])` equals `auto`; the `line: 9` layout now gets `notice: ARRANGEMENT_NOTICE`) | T3 | T3 |
| `core/server/reports/bind_test.go:114` row `"version"` (decodes a non-current version without `Load`; a blind replace makes it current) | T4 | T4 (set to `"version":5`) |
| `core/server/reports/spec_test.go:91` and `:181-182` (read `info.UpgradedFrom`: compile error) | T7 | T7 |
| `core/server/reports/spec_test.go:95-140` (`TestLoadRefusesWhatIsNotASpec` messages, `TestUpgradeRunsEveryStepInOrder`), `v2_test.go:13-24` (`TestAChangingStepIsNoticed`), `run_test.go:210-232` (`TestRunNoticesRenamedFieldsAndAnUpgrade`), `balance_shape_test.go:312-313`, `tests/e2e/reports-shipped.spec.ts:51-56` | T7 | T7 |
| Books `books/server/report_trial_balance_test.go:852` golden `"version":3`; `books/server/report_general_ledger_test.go:306-307` (`spec.Version != 3`) | T1 | Books follow-up |
| Books `books/server/report_trial_balance_test.go:281-282`, `report_general_ledger_test.go:306-307` (`info.UpgradedFrom` no longer exists: compile error); `books/server/report_sources_test.go:493` (`"version":2`, refused) | T7 | Books follow-up |

No core e2e assertion is broken by Part 1 except `reports-shipped.spec.ts:51-56` (T7). Grep of `tests/e2e/reports-*.spec.ts` for placement messages finds only `reports-wrap.spec.ts:951` (the builder's own check, client-side; unchanged). Books e2e (`general-ledger.spec.ts`, `trial-balance.spec.ts`) read no spec version and no engine placement error; Part 1 changes nothing they draw.

---

## Global Constraints

- One spec version: `reports.CurrentVersion = 4`, `CURRENT_SPEC_VERSION = 4`. After Task 7 no code reads another version.
- Length grammar (design §3.4), one parser: `^(\d{1,4}(\.\d+)?)(ch|px|%)$` plus keywords per field. Ranges: ch 1–120, px 1–4000, % 1–100. `display.oneLine.minWidth` and `display.layouts[].minWidth`: px only.
- `width`: length. `minWidth`: `auto` | `min-content` | `max-content` | length. `maxWidth`: `none` | length. `whiteSpace`: `nowrap` | `normal`. `lineClamp`: 1–4, only with `whiteSpace: normal`. `textOverflow`: `ellipsis` only (decision 5). `flexGrow`: integer 0–10.
- A figure is never cut (decision 2): on a column whose result type (or total type) is number, money, percent, date, datetime or bool, and on a first (label) column whose label is typed date, datetime, number or money, Go refuses `width`, `maxWidth`, `whiteSpace`, `lineClamp`, `textOverflow`, and any `minWidth` but `auto`/`max-content`. `flexGrow` is free.
- `display.columns` keys are columns of the spec (422 `unknown_column` otherwise). Strict decode refuses unknown keys.
- Go validates the definition only; placement, overlap and pinwheel are the client's (`core/reports/lib/table-lines.ts placeCells`).
- Never `any`, never `biome-ignore`, never `console.*`. Read files with Read; edit with Edit (hooks block `sed -i` and scripted replaces).
- Commands, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`:
  - Go: `cd core/server && go test -tags no_ui -count=1 -run '<regexp>' ./reports/` (the whole package takes about 40 s: `go test -tags no_ui -count=1 ./reports/`)
  - unit: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`
  - types: `pnpm exec tinycld-pkg typecheck`
  - lint: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check <files>`
  - e2e (one file): check ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`), then `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts`
- The driver commits; each task's last step lists the files to stage.

## Review Focus

1. **A setting with an empty value** (`"minWidth": ""`, `"oneLine": {}`). Expect: an empty column setting reads as absent (omitempty); an empty `oneLine` is refused ("minWidth is a length in px"), not taken as "any width". Test: Task 1 "oneLine with no width".
2. **A figure hidden behind a formula or a total** (a text path with a `sum` aggregate on its group rows; a formula that gives money; a first column labelled by a date field). Expect: refused like a plain money column. Test: Task 2 cases "a summed number", "a formula giving money", "a label typed as a date".
3. **minWidth larger than maxWidth in different units** (`30ch` vs `200px`). Expect: accepted (CSS: min wins at draw time); same unit and min > max is refused. Test: Task 1 "min above max" and "min and max in different units".
4. **A spec saved through the CLI/API with overlapping cells** (Go no longer refuses it). Expect: it saves; the view does not crash, lays the rows out automatically, and says why. Test: Task 3 Go "leaves placement to the client" and TS "says so when an arrangement cannot be placed".
5. **A stored or posted spec of version 1–3 after Task 7.** Expect: a clear 400 "version must be 4" on load (a saved one opens Broken with that problem), never a silent upgrade or a 500. Test: Task 7 `TestLoadReadsOnlyVersion4`.

---

### Task 1: Go spec v4 format and definition checks

**Files:**
- Modify: `core/server/reports/spec.go:17-36` (version, step), `:55-67` (`Display`, `DisplayLayout`)
- Create: `core/server/reports/fit.go`
- Modify: `core/server/reports/display.go:18-40` (call `columnFits`; layout `minWidth`)
- Create: `core/server/reports/v4_test.go`
- Modify (version fixes): `core/server/reports/saved_test.go:65`, `spec_test.go:107`, `v2_test.go:80`, `:283` area, `:295`, `v3_test.go:53,65,81,93,98-100,111`
- Modify: `core/reports/lib/spec.ts:9`, `core/reports/lib/__tests__/spec.test.ts:70-75`, `core/reports/lib/builder/__tests__/draft.test.ts:1500`

**Interfaces:**
- Consumes: `binder.bad`, `binder.column(p, id, at)` (`bind.go:149,698`), `requireSpecError` (`spec_test.go:39`), `withZooRegistry` (`spec_test.go:24`), `mustLoad`, `jsonDoc` (`v2_test.go:87,102`).
- Produces: `const CurrentVersion = 4`; `upgradeSteps[3]` identity (deleted in Task 7); `type Display struct { OneLine *DisplayOneLine; Columns map[string]ColumnFit; Layouts []DisplayLayout }` (JSON `oneLine`, `columns`, `layouts`, all omitempty); `type DisplayOneLine struct { MinWidth string }` (JSON `minWidth`); `DisplayLayout.MinWidth string` (JSON `minWidth,omitempty`); `type ColumnFit struct { Width, MinWidth, MaxWidth, WhiteSpace string; LineClamp int; TextOverflow string; FlexGrow *int }` (JSON camelCase, all omitempty); in `fit.go`: `type lengthRule struct{ name string; keywords, units []string }`, `widthRule`, `minWidthRule`, `maxWidthRule`, `pxRule`, `fitLength(s string) (float64, string, bool)`, `fitLengthProblem(s string, rule lengthRule) string`, `(b *binder) columnFits(p *plan)`, `(b *binder) columnFit(f ColumnFit, at string)`; in `v4_test.go`: `const everyV4`. TS: `CURRENT_SPEC_VERSION = 4`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/v4_test.go`:

```go
package reports

import (
	"fmt"
	"reflect"
	"testing"
)

// everyV4 uses every part version 4 adds: the one-line gate, every column
// fit setting, and a layout's own gate.
const everyV4 = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","label":"Animal","path":[{"link":"animal"}]},
	{"id":"note","label":"Note","path":[{"field":"note"}]},
	{"id":"cost","label":"Cost","path":[{"field":"cost"}]}],
	"display":{"oneLine":{"minWidth":"1040px"},
		"columns":{
			"note":{"width":"20ch","minWidth":"12ch","maxWidth":"60ch","whiteSpace":"normal","lineClamp":2,"textOverflow":"ellipsis","flexGrow":2},
			"cost":{"minWidth":"max-content","flexGrow":0}},
		"layouts":[{"width":"tablet","minWidth":"600px","tracks":2,"cells":[
			{"col":"animal","line":1,"span":2},{"col":"note","line":2},{"col":"cost","line":2}]}]}}`

func TestASpecWithEveryV4FieldLoadsAndIsStoredUnchanged(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, stored := mustLoad(t, everyV4)
	if !reflect.DeepEqual(jsonDoc(t, []byte(everyV4)), jsonDoc(t, stored)) {
		t.Fatalf("stored spec differs from the posted one:\n%s", stored)
	}
	if err := Validate(app, spec); err != nil {
		t.Fatal(err)
	}
	d := spec.Display
	note := d.Columns["note"]
	if d.OneLine.MinWidth != "1040px" || d.Layouts[0].MinWidth != "600px" || note.LineClamp != 2 || note.FlexGrow == nil || *note.FlexGrow != 2 {
		t.Fatalf("display = %+v", d)
	}
	// A flexGrow of 0 is a setting (no share of the spare room), not "absent".
	if g := d.Columns["cost"].FlexGrow; g == nil || *g != 0 {
		t.Fatalf("cost flexGrow = %v", g)
	}
}

func TestColumnFitIsChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":4,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"display":%s}`
	note := func(fit string) string { return `{"columns":{"note":` + fit + `}}` }
	for name, c := range map[string]struct {
		display, at, want string
		status            int
	}{
		"a length with no unit":         {note(`{"minWidth":"12"}`), "display.columns.note.minWidth", "minWidth is auto, min-content, max-content or a length in ch, px or %", 400},
		"a keyword width":               {note(`{"width":"auto"}`), "display.columns.note.width", "width is a length in ch, px or %", 400},
		"maxWidth auto":                 {note(`{"maxWidth":"auto"}`), "display.columns.note.maxWidth", "maxWidth is none or a length in ch, px or %", 400},
		"ch above 120":                  {note(`{"minWidth":"121ch"}`), "display.columns.note.minWidth", "a length in ch is 1 to 120", 400},
		"px above 4000":                 {note(`{"width":"4001px"}`), "display.columns.note.width", "a length in px is 1 to 4000", 400},
		"zero percent":                  {note(`{"maxWidth":"0%"}`), "display.columns.note.maxWidth", "a length in % is 1 to 100", 400},
		"a negative length":             {note(`{"minWidth":"-8ch"}`), "display.columns.note.minWidth", "minWidth is auto", 400},
		"min above max":                 {note(`{"minWidth":"30ch","maxWidth":"20ch"}`), "display.columns.note.minWidth", "minWidth is at most maxWidth", 400},
		"whiteSpace pre":                {note(`{"whiteSpace":"pre"}`), "display.columns.note.whiteSpace", "whiteSpace is nowrap or normal", 400},
		"lineClamp without normal":      {note(`{"lineClamp":2}`), "display.columns.note.lineClamp", "lineClamp goes with whiteSpace normal", 400},
		"lineClamp 5":                   {note(`{"whiteSpace":"normal","lineClamp":5}`), "display.columns.note.lineClamp", "lineClamp is 1 to 4", 400},
		"clip is not offered":           {note(`{"textOverflow":"clip"}`), "display.columns.note.textOverflow", "textOverflow is ellipsis", 400},
		"flexGrow 11":                   {note(`{"flexGrow":11}`), "display.columns.note.flexGrow", "flexGrow is 0 to 10", 400},
		"flexGrow below 0":              {note(`{"flexGrow":-1}`), "display.columns.note.flexGrow", "flexGrow is 0 to 10", 400},
		"an unknown column":             {`{"columns":{"zz":{"flexGrow":1}}}`, "display.columns.zz", "the report has no column", 422},
		"oneLine in ch":                 {`{"oneLine":{"minWidth":"80ch"}}`, "display.oneLine.minWidth", "minWidth is a length in px", 400},
		"oneLine with no width":         {`{"oneLine":{}}`, "display.oneLine.minWidth", "minWidth is a length in px", 400},
		"a layout gate in percent":      {`{"layouts":[{"width":"tablet","minWidth":"50%","tracks":1,"cells":[{"col":"note","line":1}]}]}`, "display.layouts.0.minWidth", "minWidth is a length in px", 400},
		"a layout gate above 4000 px":   {`{"layouts":[{"width":"tablet","minWidth":"5000px","tracks":1,"cells":[{"col":"note","line":1}]}]}`, "display.layouts.0.minWidth", "a length in px is 1 to 4000", 400},
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(base, c.display)))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}
	for name, display := range map[string]string{
		"keywords":                       note(`{"minWidth":"min-content","maxWidth":"none"}`),
		"a percent width":                note(`{"width":"25%"}`),
		"a decimal length":               note(`{"minWidth":"7.5ch"}`),
		"min and max in different units": note(`{"minWidth":"30ch","maxWidth":"200px"}`),
		"an empty fit":                   note(`{}`),
		"wrap with the default clamp":    note(`{"whiteSpace":"normal"}`),
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(base, display)))
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(app, spec); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Strict decode: a fit key the engine does not know is refused by Load.
	_, _, err := Load([]byte(fmt.Sprintf(base, note(`{"min":"8ch"}`))))
	requireSpecError(t, err, 400, "spec", "unknown field")
}
```

Version fixes in the existing tests (each was written for `CurrentVersion = 3`):

- `saved_test.go:65`: `{"version":4}` → `{"version":5}` (a version from the future is still refused).
- `spec_test.go:107`: `"newer version":   {`{"version":5,"columns":[]}`, "version", "newer than this server reads (4)"},`
- `v2_test.go:80`: `want["version"] = json.Number("4")`; `:295`: `spec.Version != CurrentVersion`; in `TestALookupValueWithoutAnAliasIsKept` (`:276-288`) post `"version":4` at `:280` (it compares no version; it fails because the 3 → 4 identity upgrade re-encodes and compacts the raw value: `v2_test.go:283: value = {"path":[{"field":"name"}]}`).
- `v3_test.go`: `:53` and `:81` `spec.Version != CurrentVersion`; `:65` and `:93` `doc["version"] != float64(CurrentVersion)`; `:98-100` becomes `TestAVersion5SpecIsRefused` with `{"version":5,…}` and `"version 5 is newer than this server reads (4)"`; `:111` compares with the posted doc whose version is set to 4 first:

```go
	want := jsonDoc(t, []byte(everyV3)).(map[string]any)
	want["version"] = json.Number("4")
	if !reflect.DeepEqual(want, jsonDoc(t, stored)) {
```

TS: in `core/reports/lib/__tests__/spec.test.ts:70-75` replace the test with:

```ts
    it('reads a version 1 to 4 spec and refuses version 5', () => {
        expect(parseSpec({ ...FEEDINGS, version: 1 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 4 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 5 })).toBeNull()
        expect(CURRENT_SPEC_VERSION).toBe(4)
    })
```

and in `core/reports/lib/builder/__tests__/draft.test.ts:1500`: `expect(saved.version).toBe(CURRENT_SPEC_VERSION)`.

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'V4|ColumnFit|Version|LoadRefuses|ASpecIsCheckedAndSavedLoaded|EveryV2|EveryV3|WithoutAnAlias' ./reports/`
Expected: FAIL to build: `d.OneLine undefined`, `d.Columns undefined`, `d.Layouts[0].MinWidth undefined`.

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts`
Expected: FAIL: `parseSpec({ ...FEEDINGS, version: 4 })` gives "expected null not to be null".

- [ ] **Step 3: Implement the format**

In `spec.go` replace `:17-36` with:

```go
// CurrentVersion is the spec version this engine reads and writes.
const CurrentVersion = 4

// upgradeSteps[v] turns a version v document into version v+1. The steps
// are identities and go with the one-version change (column fit plan,
// Task 7).
var upgradeSteps = map[int]upgradeStep{
	1: func(map[string]any) error { return nil },
	2: func(map[string]any) error { return nil },
	3: func(map[string]any) error { return nil },
}
```

Replace `Display` and `DisplayLayout` (`:55-67`) with:

```go
// Display is how the report table lays its columns out (E14): the width from
// which one line is tried, each column's fit, and the arrangements per width.
// The engine checks it and otherwise leaves it to the client
// (core/reports/lib/column-measure.ts, table-lines.ts).
type Display struct {
	OneLine *DisplayOneLine      `json:"oneLine,omitempty"`
	Columns map[string]ColumnFit `json:"columns,omitempty"`
	Layouts []DisplayLayout      `json:"layouts,omitempty"`
}

// DisplayOneLine gates the one-line layout: it is tried only where the rows
// are at least MinWidth (px) wide. The columns must still fit.
type DisplayOneLine struct {
	MinWidth string `json:"minWidth"`
}

// ColumnFit is a column's CSS-like fit (column-fit-design.md §3.1). Lengths
// are the text room: the client adds the cell's padding.
type ColumnFit struct {
	Width        string `json:"width,omitempty"`
	MinWidth     string `json:"minWidth,omitempty"`
	MaxWidth     string `json:"maxWidth,omitempty"`
	WhiteSpace   string `json:"whiteSpace,omitempty"`
	LineClamp    int    `json:"lineClamp,omitempty"`
	TextOverflow string `json:"textOverflow,omitempty"`
	// FlexGrow is a pointer: 0 (no share of the spare room) is a setting.
	FlexGrow *int `json:"flexGrow,omitempty"`
}

// DisplayLayout places columns on lines of Tracks equal tracks at one width
// (mobile, tablet or desktop), tried only where the rows are at least
// MinWidth (px) wide. A column it does not list is not shown there.
type DisplayLayout struct {
	Width    string        `json:"width"`
	MinWidth string        `json:"minWidth,omitempty"`
	Tracks   int           `json:"tracks"`
	Cells    []DisplayCell `json:"cells"`
}
```

Create `core/server/reports/fit.go`:

```go
package reports

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Column fit (spec v4, column-fit-design.md §3): the engine checks the
// settings; the client measures and lays the columns out.

const (
	maxLineClamp = 4
	maxFlexGrow  = 10
)

var fitLengthPattern = regexp.MustCompile(`^(\d{1,4}(?:\.\d+)?)(ch|px|%)$`)

// fitUnitRange is each unit's least and greatest value.
var fitUnitRange = map[string][2]float64{"ch": {1, 120}, "px": {1, 4000}, "%": {1, 100}}

// lengthRule is what one setting takes: its keywords and a length in its units.
type lengthRule struct {
	name     string
	keywords []string
	units    []string
}

var (
	allUnits     = []string{"ch", "px", "%"}
	widthRule    = lengthRule{name: "width", units: allUnits}
	minWidthRule = lengthRule{name: "minWidth", keywords: []string{"auto", "min-content", "max-content"}, units: allUnits}
	maxWidthRule = lengthRule{name: "maxWidth", keywords: []string{"none"}, units: allUnits}
	pxRule       = lengthRule{name: "minWidth", units: []string{"px"}}
)

// fitLength reads a length: its value and unit; false when s is not one.
func fitLength(s string) (float64, string, bool) {
	m := fitLengthPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	return v, m[2], err == nil
}

// orList is "a, b or c".
func orList(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// fitLengthProblem is why rule refuses s, or "" when it takes it.
func fitLengthProblem(s string, rule lengthRule) string {
	if slices.Contains(rule.keywords, s) {
		return ""
	}
	v, unit, ok := fitLength(s)
	if !ok || !slices.Contains(rule.units, unit) {
		takes := append(slices.Clone(rule.keywords), "a length in "+orList(rule.units))
		return rule.name + " is " + orList(takes)
	}
	if r := fitUnitRange[unit]; v < r[0] || v > r[1] {
		return fmt.Sprintf("a length in %s is %g to %g", unit, r[0], r[1])
	}
	return ""
}

// columnFits checks display.oneLine and display.columns. Whether a setting
// could cut a figure needs the columns' types, so fitRefusals checks that
// after the dry compile.
func (b *binder) columnFits(p *plan) {
	d := p.spec.Display
	if d.OneLine != nil {
		if msg := fitLengthProblem(d.OneLine.MinWidth, pxRule); msg != "" {
			b.bad("display.oneLine.minWidth", msg)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.Columns)) {
		at := "display.columns." + id
		if b.column(p, id, at) != nil {
			b.columnFit(d.Columns[id], at)
		}
	}
}

func (b *binder) columnFit(f ColumnFit, at string) {
	for _, s := range []struct {
		value string
		rule  lengthRule
	}{{f.Width, widthRule}, {f.MinWidth, minWidthRule}, {f.MaxWidth, maxWidthRule}} {
		if s.value == "" {
			continue
		}
		if msg := fitLengthProblem(s.value, s.rule); msg != "" {
			b.bad(at+"."+s.rule.name, msg)
		}
	}
	// Only lengths of one unit compare here; across units the client's CSS
	// rule (the min wins) decides at draw time.
	if lo, loUnit, ok := fitLength(f.MinWidth); ok {
		if hi, hiUnit, ok := fitLength(f.MaxWidth); ok && loUnit == hiUnit && lo > hi {
			b.bad(at+".minWidth", "minWidth is at most maxWidth")
		}
	}
	if f.WhiteSpace != "" && f.WhiteSpace != "nowrap" && f.WhiteSpace != "normal" {
		b.bad(at+".whiteSpace", "whiteSpace is nowrap or normal")
	}
	switch {
	case f.LineClamp == 0:
	case f.LineClamp < 1 || f.LineClamp > maxLineClamp:
		b.bad(at+".lineClamp", fmt.Sprintf("lineClamp is 1 to %d", maxLineClamp))
	case f.WhiteSpace != "normal":
		b.bad(at+".lineClamp", "lineClamp goes with whiteSpace normal")
	}
	if f.TextOverflow != "" && f.TextOverflow != "ellipsis" {
		b.bad(at+".textOverflow", "textOverflow is ellipsis")
	}
	if f.FlexGrow != nil && (*f.FlexGrow < 0 || *f.FlexGrow > maxFlexGrow) {
		b.bad(at+".flexGrow", fmt.Sprintf("flexGrow is 0 to %d", maxFlexGrow))
	}
}
```

The test strings above must match these messages exactly: `minWidthRule` gives "minWidth is auto, min-content, max-content or a length in ch, px or %", `widthRule` "width is a length in ch, px or %", `maxWidthRule` "maxWidth is none or a length in ch, px or %", `pxRule` "minWidth is a length in px".

In `display.go`, in `(b *binder) display`, call the fit checks before the layout count check (`:19-22` become):

```go
	d := p.spec.Display
	if d == nil {
		return
	}
	b.columnFits(p)
	if len(d.Layouts) > len(displayWidths) {
```

and after the width switch (`:36`, `seen[l.Width] = true`) add:

```go
		if l.MinWidth != "" {
			if msg := fitLengthProblem(l.MinWidth, pxRule); msg != "" {
				b.bad(at+".minWidth", msg)
			}
		}
```

In `core/reports/lib/spec.ts:9`: `export const CURRENT_SPEC_VERSION = 4`.

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (whole package; the version fixes above are every failure a version bump gives, checked on 2026-11-08 with a `-overlay` run).

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -w reports/v4_test.go reports/fit.go && gofmt -l reports/ && go vet -tags no_ui ./reports/`
Expected: no output (the map keys of `TestColumnFitIsChecked` above are padded wider than gofmt pads them; `gofmt -w` aligns them).

- [ ] **Step 5: Stage**

```bash
git add core/server/reports/spec.go core/server/reports/fit.go core/server/reports/display.go core/server/reports/v4_test.go core/server/reports/saved_test.go core/server/reports/spec_test.go core/server/reports/v2_test.go core/server/reports/v3_test.go core/reports/lib/spec.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts
```
Commit message: `reports: spec v4 column fit settings and their checks`

---

### Task 2: Go — a figure is never cut

**Files:**
- Modify: `core/server/reports/assemble.go:14-36` (use `resultType`)
- Modify: `core/server/reports/fit.go` (add `resultType`, `neverCut`, `fitRefusals`)
- Modify: `core/server/reports/run.go:1434-1462` (`compileDry` calls `fitRefusals`)
- Test: `core/server/reports/v4_test.go`

**Interfaces:**
- Consumes: Task 1 `ColumnFit`, `Display.Columns`; compiler maps `detailTypes`, `levelTypes`, `labelTypes`, `detailLabel *labelPlan` (`rows.go:41-76`, `labelPlan.typ` `rows.go:103-107`); `aggregateType` (`formula.go:1189`); `c.p.cols` (spec order, pivot cell columns last).
- Produces: `func (c *compiler) resultType(cp *colPlan) (typ, total FieldType)` (what `ResultColumn.Type`/`TotalType` say); `func (c *compiler) neverCut(cp *colPlan, first bool) bool`; `func fitRefusals(c *compiler) error` (nil or a 400 `*SpecError` keyed `display.columns.<id>.<setting>`); `const neverCutMessage = "a figure is never cut: it takes only flexGrow and minWidth auto or max-content"`.

Why after the compile: a column's type is known only once its formula, aggregate or label is compiled (`assemble.go:14-36` reads the compiler's maps). `compileDry` runs on every `Validate`, so a save, a shipped report and the builder's check all refuse the same specs.

The rule mirrors the client's: `isFigureColumn` (`core/reports/lib/column-measure.ts:41-54`: number, money, percent, date, datetime, bool, or such a `totalType`) and the typed label (`TYPED_LABELS`, `:176`: date, datetime, number, money). The first column is the label column: it is refused when it is a figure, when its record label is typed, or when the detail label formula is typed.

- [ ] **Step 1: Write the failing test**

Append to `core/server/reports/v4_test.go`:

```go
func TestAFigureIsNeverCut(t *testing.T) {
	app := withZooRegistry(t)
	spec := func(first, fits string) string {
		return `{"version":4,"source":"zoo.feedings","columns":[` + first + `,
			{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}]},
			{"id":"fed","path":[{"field":"fed"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
			{"id":"double","formula":{"op":"mul","args":[{"col":"cost"},{"value":2}]}}],
			"groups":[{"id":"g","col":"animal"}],"display":{"columns":` + fits + `}}`
	}
	const animal = `{"id":"animal","path":[{"link":"animal"}]}`
	for name, c := range map[string]struct{ raw, at string }{
		"money with a min":           {spec(animal, `{"cost":{"minWidth":"8ch"}}`), "display.columns.cost.minWidth"},
		"money with a width":         {spec(animal, `{"cost":{"width":"100px"}}`), "display.columns.cost.width"},
		"a date that wraps":          {spec(animal, `{"fed":{"whiteSpace":"normal"}}`), "display.columns.fed.whiteSpace"},
		"a summed number with a max": {spec(animal, `{"grams":{"maxWidth":"10ch"}}`), "display.columns.grams.maxWidth"},
		"money with an ellipsis":     {spec(animal, `{"cost":{"textOverflow":"ellipsis"}}`), "display.columns.cost.textOverflow"},
		"a formula giving money":     {spec(animal, `{"double":{"minWidth":"min-content"}}`), "display.columns.double.minWidth"},
		"a label typed as a date":    {spec(`{"id":"animal","path":[{"link":"animal"}],"labelField":"born"}`, `{"animal":{"width":"20ch"}}`), "display.columns.animal.width"},
	} {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, Validate(app, loadSpec(t, c.raw)), 400, c.at, neverCutMessage)
		})
	}
	// flexGrow and a whole-value min are free on a figure; text may narrow and wrap.
	ok := spec(animal, `{"cost":{"minWidth":"max-content","flexGrow":2},"fed":{"minWidth":"auto"},
		"note":{"minWidth":"5ch","whiteSpace":"normal","lineClamp":2},"animal":{"minWidth":"5ch","whiteSpace":"normal","lineClamp":2}}`)
	if err := Validate(app, loadSpec(t, ok)); err != nil {
		t.Fatal(err)
	}
}
```

(`loadSpec` is `bind_test.go:11`. If `mul` of a money column and a number is not the engine's operator name, use the multiply operator `formula.go` names; the column must give money.)

- [ ] **Step 2: Run the test to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'AFigureIsNeverCut' ./reports/`
Expected: FAIL to build: `undefined: neverCutMessage`. With the constant alone: every refusal case fails with `err = <nil>`.

- [ ] **Step 3: Implement**

In `fit.go`, add:

```go
const neverCutMessage = "a figure is never cut: it takes only flexGrow and minWidth auto or max-content"

// figureTypes are the cells shown whole (column-measure.ts FIGURE_TYPES), and
// typedLabelTypes the labels shown whole (column-measure.ts TYPED_LABELS).
var (
	figureTypes     = []FieldType{Number, Money, Percent, Date, DateTime, Bool}
	typedLabelTypes = []FieldType{Date, DateTime, Number, Money}
)

// resultType is a column's type and, when its group cells differ, their type:
// what ResultColumn.Type and TotalType say.
func (c *compiler) resultType(cp *colPlan) (typ, total FieldType) {
	typ = c.detailTypes[cp.col.ID]
	if typ == "" {
		typ = c.levelTypes[cp.col.ID]
	}
	if cp.path != nil {
		typ = cp.path.field.Type
		if cp.tm >= 0 {
			typ = aggregateType(cp.col.Aggregate, cp.path.field.Type, cp.scale)
		}
	}
	if t := c.levelTypes[cp.col.ID]; t != "" && t != typ && cp.col.Aggregate != "" {
		total = t
	}
	return typ, total
}

// neverCut is true for a column whose cells the client shows whole: a figure,
// or (first, the label column) a label typed as a date or an amount.
func (c *compiler) neverCut(cp *colPlan, first bool) bool {
	typ, total := c.resultType(cp)
	if slices.Contains(figureTypes, typ) || slices.Contains(figureTypes, total) {
		return true
	}
	if !first {
		return false
	}
	if t, ok := c.labelTypes[cp.col.ID]; ok && slices.Contains(typedLabelTypes, t) {
		return true
	}
	return c.detailLabel != nil && slices.Contains(typedLabelTypes, c.detailLabel.typ)
}

// fitRefusals refuses a fit that could cut a figure (decision 2): on such a
// column only flexGrow and a whole-value minWidth are taken.
func fitRefusals(c *compiler) error {
	d := c.p.spec.Display
	if d == nil || len(d.Columns) == 0 {
		return nil
	}
	problems := map[string]validation.Error{}
	// The label column is the spec's first column; c.p.cols moves a pivot's
	// cell column to the end, so its order is not the spec's.
	label := c.p.spec.Columns[0].ID
	for _, cp := range c.p.cols {
		f, ok := d.Columns[cp.col.ID]
		if !ok || !c.neverCut(cp, cp.col.ID == label) {
			continue
		}
		at := "display.columns." + cp.col.ID + "."
		for _, s := range []struct {
			name  string
			isSet bool
		}{
			{"width", f.Width != ""},
			{"minWidth", f.MinWidth != "" && f.MinWidth != "auto" && f.MinWidth != "max-content"},
			{"maxWidth", f.MaxWidth != ""},
			{"whiteSpace", f.WhiteSpace != ""},
			{"lineClamp", f.LineClamp != 0},
			{"textOverflow", f.TextOverflow != ""},
		} {
			if s.isSet {
				problems[at+s.name] = validation.NewError("invalid", neverCutMessage)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &SpecError{Status: 400, Problems: problems}
}
```

Add `validation "github.com/pocketbase/ozzo-validation/v4"` to `fit.go`'s imports.

In `assemble.go`, `(rn *runner) columns` (`:18-36`): replace the type lines with the helper, keeping the select options and label parts:

```go
		rc := ResultColumn{ID: cp.col.ID, Label: labelOf(cp), Aggregate: cp.col.Aggregate, Format: cp.col.Format}
		rc.Type, rc.TotalType = c.resultType(cp)
		if cp.path != nil && cp.path.field.Type == Select {
			rc.Options = cp.path.field.Options
		} else if cp.path == nil && rc.Type == Select {
			rc.Options = c.formulaOptions[cp.col.ID]
		}
		if typ, ok := c.labelTypes[cp.col.ID]; ok {
```

(read `:18-36` first; the options branch must behave exactly as before: path columns read their field's options, formula columns read `formulaOptions` when the type is select.)

In `run.go` `compileDry`, after the `groupStatement` call (`:1452-1454`) add:

```go
	if err := fitRefusals(c); err != nil {
		return err
	}
```

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (the `resultType` refactor is covered by every run test that reads `ResultColumn.Type`).

- [ ] **Step 5: Stage**

```bash
git add core/server/reports/fit.go core/server/reports/assemble.go core/server/reports/run.go core/server/reports/v4_test.go
```
Commit message: `reports: refuse a column fit that could cut a figure`

---

### Task 3: Placement leaves Go

**Files:**
- Modify: `core/server/reports/display.go:62-208` (per-cell range checks stay; placement goes)
- Delete: `core/server/reports/display_cases_test.go`
- Modify: `core/server/reports/v2_test.go:136-225`
- Test: `core/server/reports/v4_test.go`
- Modify: `core/reports/lib/table-lines.ts:28-31,84-88,106,119-123,283-292`
- Modify: `core/reports/lib/builder/draft.ts:516-520` (comment)
- Test: `core/reports/lib/__tests__/table-lines.test.ts:206-217` and a new case

**Interfaces:**
- Consumes: TS `placeCells`, `authoredLines`, `tableLines` (`table-lines.ts:124,284,475`).
- Produces: Go `func cellRangeProblem(tracks int, c DisplayCell) string`; Go no longer has `placed`, `placeCells`, `piece`, `uncuttable`, `cutDown`, `pinwheelMessage`, `prefixProblem`. TS `export const ARRANGEMENT_NOTICE = "This width's arrangement cannot be laid out, so the rows are laid out automatically here."`; an authored layout `placeCells` refuses gives `{ kind: 'refused', notice: ARRANGEMENT_NOTICE }`.

Decision: "Go validates the definition only; placement lives in TS." A cell's numbers (line 1–8, lines, span ≤ tracks, start inside the line) are the definition and stay in Go. Where cells land (first free tracks, overlap, "no room", the pinwheel) is placement: TS alone. The builder already refuses a bad arrangement before a save (`draft.ts:544`, `reports-wrap.spec.ts:951`); a spec posted by the CLI or API with an overlap now saves, and the view lays the rows out automatically and says why (the notice band, the same path as `PIVOT_NOTICE`). `core/reports/lib/fixtures/display-cells.json` stays as the TS test table.

- [ ] **Step 1: Write the failing tests**

Append to `core/server/reports/v4_test.go`:

```go
// Where cells land is the client's (core/reports/lib/table-lines.ts
// placeCells): the engine checks each cell's numbers only, so a full line, an
// overlap and a pinwheel save.
func TestTheEngineLeavesPlacementToTheClient(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":4,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]},{"id":"b","path":[{"field":"cost"}]},
		{"id":"c","path":[{"field":"grams"}]},{"id":"d","path":[{"field":"fed"}]},{"id":"e","path":[{"field":"currency"}]}],"display":{"layouts":[%s]}}`
	for name, layout := range map[string]string{
		"no room":  `{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"span":2},{"col":"b","line":1}]}`,
		"overlap":  `{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"lines":2},{"col":"b","line":2,"start":1}]}`,
		"pinwheel": `{"width":"mobile","tracks":3,"cells":[{"col":"a","line":1,"start":1,"span":2},{"col":"b","line":1,"start":3,"lines":2},{"col":"c","line":3,"start":2,"span":2},{"col":"d","line":2,"start":1,"lines":2},{"col":"e","line":2,"start":2}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := Validate(app, loadSpec(t, fmt.Sprintf(base, layout))); err != nil {
				t.Fatal(err)
			}
		})
	}
}
```

In `v2_test.go` `TestDisplayLayoutsAreChecked`: delete the rows `"no room on line 1"` (`:147`) and `"overlaps"` (`:148`); every other row stays (their keys are unchanged: `display.layouts.0.cells.<n>`). Delete `TestDisplayRefusesAPinwheel` (`:185-197`), `TestPlaceCellsFlowsIntoFreeTracks` (`:199-210`) and `TestPlaceCellsCutsAnyLayoutButAPinwheel` (`:212-229`). Delete `display_cases_test.go`.

In `core/reports/lib/__tests__/table-lines.test.ts`, change the comments at `:206-210` (incl. "Keep equal to the count in the Go test", whose Go test is deleted) to `// The client alone places cells (the engine checks each cell's numbers only).`; at `:405` the refused `line: 9` layout now says why: `expect(lines(360, [refused])).toStrictEqual({ ...auto, notice: ARRANGEMENT_NOTICE })`; import `ARRANGEMENT_NOTICE` from `../table-lines`, and add next to the pivot notice test (`:648-661`):

```ts
    it('lays out automatically, and says so, when an arrangement cannot be placed', () => {
        // Σmin 404 > 300: one line does not fit.
        const auto = tableLines({ columns: COLS, width: 300, layouts: [], measures: COLS_MEASURES })
        const overlap: DisplayLayout = {
            width: 'mobile',
            tracks: 2,
            cells: [
                { col: 'account', line: 1, span: 2 },
                { col: 'number', line: 1 },
            ],
        }
        const result = tableLines({ columns: COLS, width: 300, layouts: [overlap], measures: COLS_MEASURES })
        // The literal: a missing export reads as undefined and would pass a toEqual.
        expect(result.notice).toBe(
            "This width's arrangement cannot be laid out, so the rows are laid out automatically here."
        )
        expect(result).toStrictEqual({ ...auto, notice: ARRANGEMENT_NOTICE })
    })
```

(Import `DisplayLayout` from `../spec` if the file does not yet.)

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LeavesPlacement|DisplayLayoutsAreChecked' ./reports/`
Expected: FAIL: `no room`, `overlap` and `pinwheel` give a 400 (`no room on line 1`, `the cell overlaps another`, `can't be laid out`).

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: FAIL: `result.notice` is undefined, not the notice text; the `:405` case gets no `notice`.

- [ ] **Step 3: Implement**

`display.go`: in the cell loop of `display` (`:52-61`) add the range check, and delete the `placeCells` call (`:62-64`):

```go
		for j, cell := range l.Cells {
			cat := fmt.Sprintf("%s.cells.%d", at, j)
			if b.column(p, cell.Col, cat+".col") != nil && cols[cell.Col] {
				b.bad(cat+".col", "a column is placed once per layout")
			}
			cols[cell.Col] = true
			if cell.Justify != "" && cell.Justify != "start" && cell.Justify != "center" && cell.Justify != "end" {
				b.bad(cat+".justify", "justify is start, center or end")
			}
			if msg := cellRangeProblem(l.Tracks, cell); msg != "" {
				b.bad(cat, msg)
			}
		}
```

Replace everything from `// placed is where a display cell sits` (`:68`) to the end of the file with:

```go
// cellRangeProblem is why a cell's line, lines, span or start is out of
// range, or "". Where the cells land, and whether they overlap or cut down by
// straight lines, is the client's (core/reports/lib/table-lines.ts
// placeCells); the engine checks the definition only.
func cellRangeProblem(tracks int, c DisplayCell) string {
	span, lines := max(c.Span, 1), max(c.Lines, 1)
	switch {
	case c.Line < 1 || c.Line > maxDisplayLines:
		return fmt.Sprintf("line is 1 to %d", maxDisplayLines)
	case c.Lines < 0 || lines > maxDisplayLines:
		return fmt.Sprintf("lines is 1 to %d", maxDisplayLines)
	case c.Line+lines-1 > maxDisplayLines:
		return fmt.Sprintf("a cell ends by line %d", maxDisplayLines)
	case c.Span < 0 || span > tracks:
		return fmt.Sprintf("span is 1 to %d", tracks)
	case c.Start < 0 || c.Start > tracks || (c.Start > 0 && c.Start+span-1 > tracks):
		return "the cell does not fit its line"
	}
	return ""
}
```

Remove the now-unused imports (`errors`, `validation`) from `display.go`; `grep -n 'prefixProblem\|placeCells\|pinwheelMessage' core/server/reports/*.go` must print nothing.

`table-lines.ts`:
- After `PIVOT_NOTICE` (`:30-31`) add:

```ts
export const ARRANGEMENT_NOTICE =
    "This width's arrangement cannot be laid out, so the rows are laid out automatically here."
```

- `cutDown` doc (`:84-88`): `the first cut found. The client alone places cells; the engine checks each cell's numbers only.`
- `cellProblem` comment (`:106`): `// A cell's first refusal before placement (the engine checks the same numbers).`
- `placeCells` doc (`:119-123`): `The placement rules: a cell with a start sits there; one without takes the first tracks of its line free on every line it spans; then the cells must cut down by straight lines. The one copy: the engine leaves placement to the client.`
- `authoredLines` (`:285-286`):

```ts
    const arranged = placeCells(layout.tracks, layout.cells)
    // A spec from the CLI or the API may hold an arrangement the builder would
    // refuse; the engine does not place cells, so the view says why it skips it.
    if ('problem' in arranged) return { kind: 'refused', notice: ARRANGEMENT_NOTICE }
```

`draft.ts:516-520` comment: `// What the builder refuses in each display layout: the engine's checks in its order (core/server/reports/display.go display: a second layout for a width; bad tracks, no cells or more cells than a report has columns stop the layout's checks; each cell whose column the report lacks or another cell placed), then the first cell placeCells refuses (the client alone places cells).`

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/ && gofmt -l reports/`
Expected: PASS, no gofmt output.

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS. If another existing case gives an authored layout with a placement problem and expects no `notice`, add `notice: ARRANGEMENT_NOTICE` to its expectation and say so in the report.

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts`
Expected: PASS (the builder's own refusal at `:951` is client-side; the pivot notice test keeps one `report-notice-layout`).

- [ ] **Step 5: Stage**

```bash
git add core/server/reports/display.go core/server/reports/display_cases_test.go core/server/reports/v2_test.go core/server/reports/v4_test.go core/reports/lib/table-lines.ts core/reports/lib/builder/draft.ts core/reports/lib/__tests__/table-lines.test.ts
```
Commit message: `reports: placement lives in the client only`

---

### Task 4: Fixtures to version 4 (Go tests a–h)

**Files (modify; each only its version literals):**
`core/server/reports/alias_test.go`, `amountparam_test.go`, `apps_test.go`, `balance_shape_test.go`, `bind_test.go`, `blankcurrency_test.go`, `broken_test.go`, `currency_reader_test.go`, `detailblank_test.go`, `detaillabel_test.go`, `endpoint_saved_test.go`, `endpoint_test.go`, `fieldcurrency_test.go`, `fill_test.go`, `formula_run_test.go`, `fuzz_test.go`, `grouporder_test.go`, `groups_test.go`, `having_test.go`, `heading_test.go`

**Interfaces:**
- Consumes: Task 1 (`CurrentVersion = 4`; old versions still load until Task 7).
- Produces: no `"version":1`, `"version":2` or `"version":3` (any spacing) in these files.

No new test: the red is the grep, and Task 7 makes any missed literal a failing load.

- [ ] **Step 1: List the literals (the red)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && grep -cE '"version" ?: ?[123]\b' alias_test.go amountparam_test.go apps_test.go balance_shape_test.go bind_test.go blankcurrency_test.go broken_test.go currency_reader_test.go detailblank_test.go detaillabel_test.go endpoint_saved_test.go endpoint_test.go fieldcurrency_test.go fill_test.go formula_run_test.go fuzz_test.go grouporder_test.go groups_test.go having_test.go heading_test.go`
Expected: 144 in all.

- [ ] **Step 2: Replace**

For each file, for each distinct spelling the grep shows (`"version":1`, `"version":2`, `"version":3`, `"version": 3`, …), one `Edit` with `replace_all: true` to `"version":4`. Keep `"version":99` (a future version on purpose, `broken_test.go:46,221`). Exception: `bind_test.go:114`, row `"version"`, decodes a spec without `Load` and expects Validate's "load the spec first"; it needs a version other than the current one: set it to `"version":5` first, then do the `replace_all`. Read the lines around each literal in `fuzz_test.go` and `alias_test.go` first: if a test's point is the old version itself (not just a fixture), stop and report it instead of replacing.

- [ ] **Step 3: Check**

Run the grep of Step 1 again. Expected: every count 0.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

- [ ] **Step 4: Stage**

```bash
git add core/server/reports/alias_test.go core/server/reports/amountparam_test.go core/server/reports/apps_test.go core/server/reports/balance_shape_test.go core/server/reports/bind_test.go core/server/reports/blankcurrency_test.go core/server/reports/broken_test.go core/server/reports/currency_reader_test.go core/server/reports/detailblank_test.go core/server/reports/detaillabel_test.go core/server/reports/endpoint_saved_test.go core/server/reports/endpoint_test.go core/server/reports/fieldcurrency_test.go core/server/reports/fill_test.go core/server/reports/formula_run_test.go core/server/reports/fuzz_test.go core/server/reports/grouporder_test.go core/server/reports/groups_test.go core/server/reports/having_test.go core/server/reports/heading_test.go
```
Commit message: `reports: test fixtures at spec version 4 (a–h)`

---

### Task 5: Fixtures to version 4 (Go tests j–p)

**Files (modify; each only its version literals):**
`core/server/reports/jsonlist_test.go`, `labellength_test.go`, `layout_test.go`, `levellookuponce_test.go`, `linkonce_test.go`, `lookup_test.go`, `lookuphoist_test.go`, `lookupwheremixed_test.go`, `mixedbool_test.go`, `mixedcondition_test.go`, `mixedsort_test.go`, `nest_test.go`, `noticeparts_test.go`, `open_test.go`, `opening_test.go`, `options_test.go`, `orderedrows_test.go`, `paging_test.go`, `params_test.go`, `pivot_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces: no version 1–3 literal in these files.

- [ ] **Step 1: List the literals (the red)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && grep -cE '"version" ?: ?[123]\b' jsonlist_test.go labellength_test.go layout_test.go levellookuponce_test.go linkonce_test.go lookup_test.go lookuphoist_test.go lookupwheremixed_test.go mixedbool_test.go mixedcondition_test.go mixedsort_test.go nest_test.go noticeparts_test.go open_test.go opening_test.go options_test.go orderedrows_test.go paging_test.go params_test.go pivot_test.go`
Expected: about 111 in all.

- [ ] **Step 2: Replace**

As Task 4 Step 2. One special line: `lookup_test.go:256` wraps its spec in `strings.Replace(chainSpec(20, step), `"version":1`, `"version":2`, 1)`. After the replace both literals are `"version":4` and the call does nothing: write `Validate(app, loadSpec(t, chainSpec(20, step)))` instead (drop `strings` from the imports if it becomes unused).

- [ ] **Step 3: Check**

Run the grep again (all 0) and `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/` (PASS).

- [ ] **Step 4: Stage**

```bash
git add core/server/reports/jsonlist_test.go core/server/reports/labellength_test.go core/server/reports/layout_test.go core/server/reports/levellookuponce_test.go core/server/reports/linkonce_test.go core/server/reports/lookup_test.go core/server/reports/lookuphoist_test.go core/server/reports/lookupwheremixed_test.go core/server/reports/mixedbool_test.go core/server/reports/mixedcondition_test.go core/server/reports/mixedsort_test.go core/server/reports/nest_test.go core/server/reports/noticeparts_test.go core/server/reports/open_test.go core/server/reports/opening_test.go core/server/reports/options_test.go core/server/reports/orderedrows_test.go core/server/reports/paging_test.go core/server/reports/params_test.go core/server/reports/pivot_test.go
```
Commit message: `reports: test fixtures at spec version 4 (j–p)`

---

### Task 6: Fixtures to version 4 (Go tests r–v)

**Files (modify; each only its version literals):**
`core/server/reports/record_test.go`, `relation_test.go`, `result_extras_test.go`, `rollup_test.go`, `rows_test.go`, `rowstotal_test.go`, `run_test.go`, `saved_test.go`, `selectlabel_test.go`, `shipped_test.go`, `sortformula_test.go`, `statement_test.go`, `textday_test.go`, `valuecurrency_test.go`, `viewsort_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces: no version 1–3 literal in these files.

- [ ] **Step 1: List the literals (the red)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && grep -cE '"version" ?: ?[123]\b' record_test.go relation_test.go result_extras_test.go rollup_test.go rows_test.go rowstotal_test.go run_test.go saved_test.go selectlabel_test.go shipped_test.go sortformula_test.go statement_test.go textday_test.go valuecurrency_test.go viewsort_test.go`
Expected: about 211 in all.

- [ ] **Step 2: Replace**

As Task 4 Step 2. Special lines:
- `sortformula_test.go:131`: the `strings.Replace(…, `"version":1`, `"version":2`, 1)` around `chainSpec(8, doubling)` does nothing after the replace; drop the `Replace` and keep its argument.
- `saved_test.go:97` (`if strings.Contains(body, `"version":1`)`) becomes `"version":4` with the replace: it checks that a refused future spec (`"version":99`) is not rewritten to the current version, which is the right check.
- `run_test.go:212` (`TestRunNoticesRenamedFieldsAndAnUpgrade`): replace its literal too; Task 7 rewrites the test.

- [ ] **Step 3: Check**

Run the grep again (all 0) and `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/` (PASS).

- [ ] **Step 4: Stage**

```bash
git add core/server/reports/record_test.go core/server/reports/relation_test.go core/server/reports/result_extras_test.go core/server/reports/rollup_test.go core/server/reports/rows_test.go core/server/reports/rowstotal_test.go core/server/reports/run_test.go core/server/reports/saved_test.go core/server/reports/selectlabel_test.go core/server/reports/shipped_test.go core/server/reports/sortformula_test.go core/server/reports/statement_test.go core/server/reports/textday_test.go core/server/reports/valuecurrency_test.go core/server/reports/viewsort_test.go
```
Commit message: `reports: test fixtures at spec version 4 (r–v)`

---

### Task 7: One spec version

**Files:**
- Modify: `core/server/reports/spec.go:17-36,292-422` (`Load` reads version 4 only; upgrade machinery deleted)
- Modify: `core/server/reports/run.go:162-180,327-334` (`Notice.FromVersion`, `upgraded` notice)
- Modify: `core/server/reports/spec_test.go:95-140`, `v2_test.go` (`:12-24` and every remaining version test), `v3_test.go` (upgrade tests), `run_test.go:210-232`, `balance_shape_test.go:312-313`
- Modify: `core/reports/lib/spec.ts:205-207`, `core/reports/lib/types.ts:115-116`, `core/reports/components/ReportNotices.tsx:101` (comment)
- Modify: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/__tests__/run-client.test.ts:133`, `core/reports/lib/builder/__tests__/draft.test.ts:298,327,1486-1500`
- Modify: `tests/e2e/reports-shipped.spec.ts:51-56`, `tests/e2e/reports-share.spec.ts:222`, `tests/scripts/reports-stub/server/register.go.txt:61,84,115`

**Interfaces:**
- Consumes: Tasks 1, 4–6.
- Produces: `func Load(raw []byte) (*Spec, *LoadInfo, error)` refusing every version but `CurrentVersion` with 400 `version` "version must be 4"; `type LoadInfo struct { Aliased []Alias }`; `Notice` without `FromVersion`; no `upgradeSteps`, `upgradeStep`, `upgrade`, `loadWith`, `sameExceptVersion`. TS: `reportSpecSchema.version` = `z.literal(CURRENT_SPEC_VERSION)`; `ResultNotice` (types.ts) without `fromVersion`.

Default change: none. Behaviour change (decision, no backward compatibility): a stored report of version 1–3 opens Broken with "version must be 4" (`broken.go` already marks a spec `Load` refuses, `broken_test.go:46-60`).

- [ ] **Step 1: Write the failing tests**

In `spec_test.go`, replace `TestLoadRefusesWhatIsNotASpec`'s version rows (`:103-107`) and its `"version":1` fixtures, and delete `TestUpgradeRunsEveryStepInOrder` (`:117-140`):

```go
		"not json":        {`[1]`, "spec", "not a single JSON object"},
		"unknown key":     {`{"version":4,"source":"zoo.feedings","columns":[],"sql":"DROP"}`, "spec", "unknown field"},
		"trailing data":   {`{"version":4,"source":"zoo.feedings","columns":[]} {}`, "spec", "not a single JSON object"},
		"no version":      {`{"source":"zoo.feedings","columns":[]}`, "version", "version must be 4"},
		"version zero":    {`{"version":0,"columns":[]}`, "version", "version must be 4"},
		"version 1.5":     {`{"version":1.5,"columns":[]}`, "version", "version must be 4"},
		"an older version": {`{"version":3,"source":"zoo.feedings","columns":[]}`, "version", "version must be 4"},
		"newer version":   {`{"version":5,"columns":[]}`, "version", "version must be 4"},
		"wrong leaf type": {`{"version":4,"source":"zoo.feedings","columns":[{"id":"a","path":"cost"}]}`, "spec", "cannot unmarshal"},
```

Add to `v4_test.go`:

```go
// One spec version: an older or newer one is refused, never upgraded.
func TestLoadReadsOnlyVersion4(t *testing.T) {
	for _, v := range []int{1, 2, 3, 5} {
		_, _, err := Load([]byte(fmt.Sprintf(`{"version":%d,"source":"zoo.feedings","columns":[]}`, v)))
		requireSpecError(t, err, 400, "version", "version must be 4")
	}
	spec, info, err := Load([]byte(`{"version":4,"source":"zoo.feedings","columns":[]}`))
	if err != nil || spec.Version != 4 || len(info.Aliased) != 0 {
		t.Fatalf("spec %+v, info %+v, err %v", spec, info, err)
	}
}
```

Delete the tests that exist only for the upgrade: `v2_test.go` `TestAChangingStepIsNoticed` (`:13-24`) and the v1 → v2 alias upgrade test whose name ends `ThroughTheUpgrade` (`:290-303`); in `spec_test.go` drop the `UpgradedFrom` check at `:91`, and rename `TestLoadKeepsLargeIntegersAcrossAnUpgrade` to `TestLoadKeepsLargeIntegers`, dropping its `UpgradedFrom` check at `:181-182` (its point is now that the decode keeps the value); `v3_test.go` `TestAV2SpecLoadsSilentlyAsVersion3`, `TestAV1SpecLoadsSilentlyAsVersion3`, `TestAVersion5SpecIsRefused` (now `TestLoadReadsOnlyVersion4`). Move the remaining `"version":1/2/3` literals in `v2_test.go`, `v3_test.go` (`everyV3`), `spec_test.go` to `"version":4`, and drop the `want["version"] = json.Number("4")` lines Task 1 added (the posted doc is version 4 now, so the plain comparison holds). In `v3_test.go:108` and `balance_shape_test.go:312-313` drop the `UpgradedFrom` checks (keep `spec.Version != CurrentVersion`).

`run_test.go:210-232`: rename to `TestRunNoticesRenamedFields`, delete `info.UpgradedFrom = 1`, and expect `[]string{"aliased"}` with `res.Notices[0].Aliases[0].From != "mass"`; delete the `FromVersion` check.

TS, `spec.test.ts`: `FEEDINGS` (`:11`) `version: 4`; `:60` `version: 4`; replace the `:70-75` test with

```ts
    it('reads version 4 only', () => {
        expect(parseSpec({ ...FEEDINGS, version: 4 })).not.toBeNull()
        for (const version of [1, 2, 3, 5]) expect(parseSpec({ ...FEEDINGS, version })).toBeNull()
        expect(CURRENT_SPEC_VERSION).toBe(4)
    })
```

and every other `version: 2` / `version: 3` in the file → `version: 4` (rename `describe('spec version 3'` to `describe('the parts of version 3'` — the parts stay). `run-client.test.ts:133` and `draft.test.ts:298,327` `version: 4`; `draft.test.ts:1486-1500` reads a version 4 spec and expects `CURRENT_SPEC_VERSION`.

E2E: `tests/e2e/reports-shipped.spec.ts:51-56`: delete the "A version 1 shipped report runs with no upgraded notice" comment and the `report-notice-upgraded` assertion; keep the wait for `report-table` or `report-empty`. `tests/e2e/reports-share.spec.ts:222` `version: 4` (the seeded spec is still Broken by its missing source). `tests/scripts/reports-stub/server/register.go.txt:61,84,115`: `"version": 4`.

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LoadReadsOnlyVersion4|LoadRefuses|RunNotices' ./reports/`
Expected: FAIL: `TestLoadReadsOnlyVersion4` — versions 1–3 load (upgraded) and version 5 says "newer than this server reads (4)", not "version must be 4"; the `TestLoadRefusesWhatIsNotASpec` version rows fail the same way.

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts`
Expected: FAIL: version 1 parses.

- [ ] **Step 3: Implement**

`spec.go`: replace `:17-36` with

```go
// CurrentVersion is the one spec version this engine reads and writes.
const CurrentVersion = 4
```

delete `upgradeStep`, `upgrade`, `sameExceptVersion` and `loadWith`; `LoadInfo` becomes

```go
// LoadInfo says what Load changed: every alias it resolved.
type LoadInfo struct {
	Aliased []Alias
}
```

and `Load`:

```go
// Load reads a stored or posted spec: one JSON object of version
// CurrentVersion, decoded strictly (an unknown key is an error), with every
// aliased key replaced by its current key. A later bind step checks the
// result. A spec saved again is saved as Load returns it.
func Load(raw []byte) (*Spec, *LoadInfo, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, invalid("spec", "the spec is not a single JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, invalid("spec", "the spec is not a single JSON object")
	}
	if n, ok := doc["version"].(json.Number); !ok || n.String() != strconv.Itoa(CurrentVersion) {
		return nil, nil, invalid("version", fmt.Sprintf("version must be %d", CurrentVersion))
	}
	var spec Spec
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&spec); err != nil {
		return nil, nil, invalid("spec", "the spec is not valid: "+err.Error())
	}
	return &spec, &LoadInfo{Aliased: resolveAliases(&spec)}, nil
}
```

Fix the imports (`strconv` in; `reflect` out if unused). `grep -n 'UpgradedFrom\|upgradeStep\|loadWith\|sameExceptVersion' core/server -r` must print nothing.

`run.go`: delete `FromVersion` (`:178-179`) and the `upgraded` branch of `loadNotices` (`:328-330`).

`spec.ts:205-207`:

```ts
    // One spec version: a spec of another version is unreadable here.
    version: z.literal(CURRENT_SPEC_VERSION),
```

`types.ts:115-116`: delete `fromVersion`. `ReportNotices.tsx:101`: drop "an upgraded" from the comment. Grep `upgraded` in `core/reports`, `core/help`, `docs/packages.md`: remove any text about the upgrade notice (`docs/packages.md`'s "Spec version 3" paragraph is rewritten in Part 2, Task 14; leave it for that task).

- [ ] **Step 4: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/ ./coreserver/ && gofmt -l reports/ && go vet -tags no_ui ./reports/`
Expected: PASS, no output from gofmt/vet. (`coreserver` holds `composition_parity_test.go` and shipped wiring.)

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && grep -rnE '"version" ?: ?[123]\b|version: [123]\b' core/server/reports core/reports tests/e2e tests/scripts/reports-stub | grep -v node_modules`
Expected: nothing.

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports` then `pnpm exec tinycld-pkg typecheck`
Expected: PASS.

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-shipped.spec.ts` and `… -- reports-share.spec.ts`
Expected: PASS. First regenerate the local `salesstub/` from `register.go.txt` (install and `packages:generate` do not; CI does it at `.github/workflows/ci.yml:250`): from `/Users/nas/tmp/tinycld/books-purchases` run `pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts`, with no e2e run in progress; then `grep -n '"version"' salesstub/server/register.go` shows only 4.

- [ ] **Step 5: Stage**

```bash
git add core/server/reports/spec.go core/server/reports/run.go core/server/reports/spec_test.go core/server/reports/v2_test.go core/server/reports/v3_test.go core/server/reports/v4_test.go core/server/reports/run_test.go core/server/reports/balance_shape_test.go core/reports/lib/spec.ts core/reports/lib/types.ts core/reports/components/ReportNotices.tsx core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/run-client.test.ts core/reports/lib/builder/__tests__/draft.test.ts tests/e2e/reports-shipped.spec.ts tests/e2e/reports-share.spec.ts tests/scripts/reports-stub/server/register.go.txt
```
Commit message: `reports: one spec version (4); no upgrade path`

---

## Books follow-up (not core tasks; after the core merge, in the Books lane)

Settings from the design (§3.7) and the decisions (1: GL at 1024 is tablet; 3: TB account wraps):

- `books/server/report_general_ledger.go:115-157`: add to the spec's `Display`
  ```go
  Display: &reports.Display{
      OneLine: &reports.DisplayOneLine{MinWidth: "1040px"},
      Columns: map[string]reports.ColumnFit{
          "type":        {MinWidth: "12ch"},        // "Journal entry" whole; "Void reversal of JE-10" gets "…"
          "number":      {MinWidth: "max-content"}, // entry numbers never cut
          "description": {FlexGrow: ptr(2)},
      },
      Layouts: generalLedgerLayouts(), // tablet gains MinWidth: "600px"
  }
  ```
  (`FlexGrow` is `*int`; a local `func ptr(n int) *int` or an inline var.)
- `books/server/report_trial_balance.go:72-88`:
  ```go
  Columns: map[string]reports.ColumnFit{
      "account": {MinWidth: "5ch", WhiteSpace: "normal", LineClamp: 2},
      "number":  {MinWidth: "max-content"},
  },
  ```
- Books Go tests the core change breaks: `books/server/report_trial_balance_test.go:852` golden (`"version":4` and the new `display`), `:281-282` (drop `info.UpgradedFrom`); `books/server/report_general_ledger_test.go:306-307` (`spec.Version != reports.CurrentVersion`, drop `UpgradedFrom`); `books/server/report_sources_test.go:493` (`"version":4`).
- `books/tests/e2e/general-ledger.spec.ts:921-966` (1024 → tablet): fix the premise comment `:942-944` (no Books sidebar on `/a/reports`: the rows are window − 64, 960 at 1024); after every `setViewportSize` call `const layout = await waitForTableFit(page)` (core `tests/e2e/helpers.ts`, Part 2 Task 11) and assert `layout` is `'tablet'` at 900 and 1024 and `'one-line'` at 1280, before polling geometry; keep `cut: []`.
- `books/tests/e2e/general-ledger.spec.ts:861-919` (phone): `waitForTableFit` after the resize; assert `'mobile'`.
- `books/tests/e2e/trial-balance.spec.ts:553-613` per `tb-phone-diagnosis.md` §3: rename to "on a phone the debit sits beside the account, the credit wraps under it, and only the chip row scrolls sideways"; replace `wrapped` with `debitBeside` (`|debit.top − label.top| ≤ 1`, `debit.left ≥ label.right − 1`) and `creditWrapped` (`credit.top ≥ label.bottom − 1`), keep rows with a debit or a credit cell and require at least one of each; `insideTable` checks `debit ?? credit`; `tableFits` skips scrollers inside `[data-testid="report-controls"]`; call `waitForTableFit` after the resize. With the TB settings above the account label may wrap to 2 lines: the label checks read `label.top`/`label.bottom`, which hold.
- The Books e2e changes need Part 2 (Tasks 9–12: settings read, marker, wrap); the Books Go changes need only Part 1.

## Open questions and risks

Open questions (not covered by the decisions): none for Part 1.

Risks:
- Between the Part 1 merge and the Books follow-up, the Books Go tests fail (Task 1: version goldens; Task 7: `UpgradedFrom` compile error). Land the Books Go change right after the core merge, on the same branch name.
- A stored report of version 1–3 in any developer database opens Broken after Task 7 (accepted: not live). Shipped reports are rebuilt from code at start-up, so only user-saved ones are hit.
- Phone: a 7-digit bold total makes the GL mobile line 4 about 372 px > 370, so that row falls to automatic lines (Part 2 measures; the Books follow-up may need `flexGrow` or a narrower `type`).
- `textOverflow` has one legal value (`ellipsis`, decision 5), so the setting is a no-op today; it is kept so a later `clip` needs no format change.
