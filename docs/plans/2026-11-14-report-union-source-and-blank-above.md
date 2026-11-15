# Report union source (U1) and blank above (G2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core's report engine gains a source that is the union of several collections (`Source.Parts`, U1) and a column setting that totals only up to a group level (`columns[].blankAbove`, G2), edited in the builder, at spec version 4 with no bump.

**Architecture:** A union source's rule CTE is the `UNION ALL` of each part's own list-rule select, projected to the source's keys, so every reader of a source (scopes, joins, to-many totals, exists, record groups, labels, fill, param options, lookups, topics) reads it through the one CTE name it already reads. `blankAbove` binds to a group level; the level compiler reads the column as NULL on every level above it, and the assembler leaves the cell, its `@currency` and its `@label` out of those rows.

**Tech Stack:** Go (PocketBase, dbx, SQLite) in `core/server/reports`; TypeScript (zod, React Native, react-hook-form) in `core/reports`; Playwright e2e over the `salesstub` fixture package.

**Spec:** `/Users/nas/tmp/tinycld/books-reports/books/docs/specs/sales-tax-liability-design.md` §4 (rows U1 and G2, every column) and §5 (the shape of Task 7's fictional report). Checkpoint brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-sales-tax-liability/cp/cp0.md`, "Checkpoint text".

**Verdict:** this change takes `exclusive` for the lane (design gate Q6): U1 and G2 edit the engine files every report unit's checkpoint 0 edits. p5-drill-down's open core PR tinycld#475 edits the same G2 files; this branch is built on core main 58bafaec and never reads #475's branch; the PR that merges second merges `main` and fixes the conflicts at the source.

## Global Constraints

- Repo `/Users/nas/tmp/tinycld/books-reports/tinycld`, branch `books/p5-sales-tax-liability` (from main 58bafaec). Every path in this plan is relative to that repo.
- Core names no feature package: no feature slug, collection or route in any core file. Tests use the fictional `zoo_*` collections and the `salesstub` e2e package only.
- Spec version stays 4 (`CurrentVersion` = 4, `CURRENT_SPEC_VERSION` = 4). No bump, no shim, no reader of an older form (not live).
- Names (from the brief's Produces): Go `reports.SourcePart` (`Collection string`; `Columns map[string]string`, source key → the part's column; `Values map[string]any`, source key → a constant), `reports.Source.Parts []reports.SourcePart` (a union sets `Parts` and leaves `Collection` empty), `reports.Column.BlankAbove string` (JSON `blankAbove`, a group id), refusal code `unknown_group` (422). Client: `blankAbove?: string` on a spec column (`core/reports/lib/spec.ts`), kept by the builder draft. No rename.
- Each task writes its tests first, runs them, and records the red output and its reason before any implementation.
- Agents make no commits and no pushes; the driver commits. Each task's last step lists the files it changed.
- Go tests run from `/Users/nas/tmp/tinycld/books-reports/tinycld/core/server`: `go test ./reports/ -run '<regexp>' -count=1`.
- Vitest runs from `/Users/nas/tmp/tinycld/books-reports/tinycld/core`: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run <path under core>`.
- Biome: 4-space indent, single quotes, ES5 trailing commas, no superfluous semicolons, no `any`, no biome-ignore. Scope `biome --write` to your own files.
- Go logging through `logging.ForPackage`/the package's `log`; no `fmt.Print` in runtime code.
- E2E: ports `E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221`; check them first (`lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7220|7221) ' && { echo PORTS-BUSY; exit 1; }`); run one file only. E2E sets data up through the UI (the stub's landing screen), never by raw PocketBase writes.

## Rulings (this plan's own decisions)

- **R1 — a union row's id is its part record's id**, unprefixed. PocketBase ids are 15 random characters per record, so two parts' ids do not meet; a link to a union joins on that id, as to any source.
- **R2 — parts.** A union has at least two parts, each a distinct collection. CheckSources refuses a part collection whose list rule is null (superusers only), as the spec says; a one-collection source with a null rule stays allowed (unchanged: `zoo_vault`).
- **R3 — keys.** A union's fields name no `Column` (each part maps its own in `Columns`). The CTE projects `id` (each part's own id) and every key in: the fields, the to-one links' `Field`, and every key a part maps or gives a value. So a to-many link from another source to the union names a `BackField` key the parts map. A key a part does not map reads NULL there.
- **R4 — constants.** `Values` are bound parameters (never SQL literals), checked at `RegisterSource`: a string for a text field, an option value for a select, an int/int64/float64 for a number or money field, a bool for a yes/no field; other types are refused.
- **R5 — type agreement.** CheckSources compares, per key, the PocketBase column type of every part that maps it (text vs editor is refused); a relation's target is checked by the link, not here (parts' relations point at different collections by design).
- **R6 — `fuzz_test.go` is not changed.** Its per-seed parity compares one collection with that collection's list API. The union's parity with the list API is pinned per reader (ann, bob, dan, eve) in `union_run_test.go`, and every statement of every test still passes the package's whole-statement topics check (`topics_check_test.go`).
- **R7 — `catalog.go` is not changed.** A union source lists like any other, naming no collection or part; Task 1 pins it with a test.
- **R8 — what blank above means.** A column with `blankAbove: "<g>"` reads NULL on every level above `g`'s (levels.go), and its cell, `@currency` and `@label` are left out of the group, subtotal, layout section and Total rows of those levels (assemble.go). So a rollup, having, sort key, derived column or computed row that reads it there reads blank too. Opening rows keep their own cell formulas.
- **R9 — `core/reports/lib/types.ts` is not changed.** The result gains no field: a cell that is not sent is drawn blank by the view already. (The brief listed the file as Modify.)
- **R10 — one refusal more than the spec lists:** `blankAbove` on a child-row (nest) column is refused (400), as a rollup is: child rows have no totals.
- **R11 — the builder's Totals choice is a `PickMenu`** (like "On group and total rows show"), labelled **Totals**, items **On every row** (id `every`) and **Up to <label of the column the group groups by>** (id `up-<group index>`), so the e2e names an item without knowing a generated group id.
- **R12 — "422 `unknown_group`, at row level for a column and at table level for the spec"** is read as: the problem's key is the column's own path `columns.<i>.blankAbove` (its row) and the whole spec answers 422 through both `Validate` and `Run` (the table). The builder shows a removed group as a field error under the column.
- **R13 — the stub.** `tests/scripts/scaffold-reports-stub.ts` writes only the stub's package files, `collections.ts` and `types.ts`; the migration, Go source and screen are its templates under `tests/scripts/reports-stub/`, which Task 8 edits too.
- **R15 — `join.go` is not changed** (spec §4 U1 lists it): it never reads `Source.Collection`; every join reads the source's CTE through `stmt.sourceRule` (Task 2: `newScope`, `scope.join`), and `lookup.go` and `run.go` are the other readers.
- **R16 — Task 7's Go code names no package:** its test, fixture and comments use neutral words (orders, tickets, lines, sites, animals); only this plan names the report shape it mirrors.
- **R14 — the e2e uses a fresh invited user** (`createInvitedUser`): the stub's sales and returns are owner-only, so the report shows only that test's rows on the shared e2e DB.

## Open questions

None that block. If the driver reads the brief's "row level / table level" otherwise than R12, only Task 3's two `requireSpecError` lines change.

## Review Focus

1. A reader who may list no row of any part (a new user): the union report shows no detail rows and no error, its Total reads 0 in the source's currency. → Task 2 test `TestAUnionReaderWithNoRowsSeesAnEmptyReport`.
2. `blankAbove` naming the outermost group: the first level's rows keep the cell, only the Total leaves it blank. → Task 4 test `TestBlankAboveTheFirstLevelBlanksOnlyTheTotal`.
3. A group removed in the builder while a column still names it: the draft shows a field error under the column instead of saving a spec the engine refuses (422). → Task 5 test "refuses a removed group".
4. A report opened in its default fold state (inner level closed): the outer rows still carry no cell, and the closed inner group rows carry it. → Task 4 test `TestBlankAboveHoldsOnACollapsedRun`.
5. A group by a union's constant select field: rows of both parts group by their constant, and each group totals only its part. → Task 2 test `TestAGroupByAUnionConstantTotalsEachPart`.

---

### Task 1: U1 — union source registry, CheckSources and catalog

**Files:**
- Modify: `core/server/reports/registry.go` (types near line 131, `RegisterSource` 199-213, `validateSource` 215-270; new helpers at the end)
- Modify: `core/server/reports/check.go` (`checkSource` 69-106, `checkLink` 163-190; new helpers)
- Create: `core/server/reports/union_zoo_test.go` (fixture used by Tasks 1 and 2)
- Create: `core/server/reports/union_check_test.go`

**Interfaces:**
- Consumes: `Source`, `Field`, `Link`, `RegisterSource`, `CheckSources`, `Refused`, `lookupSource`, `Catalog`, `newZoo`, `zoo.rec`, `zoo.user`, `zoo.save`, `rule` (zoo_test.go).
- Produces:
  - `type SourcePart struct { Collection string; Columns map[string]string; Values map[string]any }`
  - `Source.Parts []SourcePart`
  - `func (s *Source) collections() []string` — the one collection, or every part's, in part order.
  - `func (s *Source) unionKeys() []string` — sorted keys a union's CTE projects besides `id` (R3).
  - `func checkLink(app core.App, src *Source, l Link) string` (signature changed: no collection argument).
  - Test fixture: `func addZooTreats(t testing.TB, z *zoo)` (collections `zoo_treats`, `zoo_tags`; user `dan`, animal `pip`; records `t1`..`t4`, `tag1`, `tag2`), `func registerZooMeals()` (sources `zoo.meals`, `zoo.pets`, `zoo.tags`), `func newUnionZoo(t *testing.T) *zoo`.

- [ ] **Step 1: Write the fixture**

Create `core/server/reports/union_zoo_test.go`:

```go
package reports

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's union: zoo_treats has the shape of zoo_feedings with other column
// names and another list rule (the animal's owner, where feedings take its
// keepers), so a reader sees feedings by keeping an animal and treats by
// owning one. zoo.meals is the union of both.
//
//	ann owns leo and kit, keeps leo and zed: feedings f1-f4, treats t1 (kit), t3 (leo)
//	bob owns zed and mo, keeps zed and mo:   feedings f3, f4, f5, treat t2 (zed)
//	dan owns pip and keeps nothing:          treat t4 (pip), no feeding
//	eve owns and keeps nothing:              no meal
//
// zoo_tags tags feedings (list: any signed-in user): tag1 → f1, tag2 → f3.
// zoo.pets is zoo_animals with a to-many link into the union; zoo.tags has a
// to-one link into it.
func addZooTreats(t testing.TB, z *zoo) {
	t.Helper()
	animals, err := z.app.FindCollectionByNameOrId("zoo_animals")
	if err != nil {
		t.Fatal(err)
	}
	feedings, err := z.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	treats := core.NewBaseCollection("zoo_treats")
	treats.ListRule = rule(`animal.owner = @request.auth.id`)
	treats.ViewRule = treats.ListRule
	treats.Fields.Add(
		&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
		&core.NumberField{Name: "price"},
		&core.TextField{Name: "currency"},
		&core.DateField{Name: "given"},
		&core.TextField{Name: "label"},
		&core.EditorField{Name: "remark"},
	)
	z.save(t, treats)
	tags := core.NewBaseCollection("zoo_tags")
	tags.ListRule = rule(`@request.auth.id != ""`)
	tags.ViewRule = tags.ListRule
	tags.Fields.Add(
		&core.TextField{Name: "tag"},
		&core.RelationField{Name: "feeding", CollectionId: feedings.Id, MaxSelect: 1},
	)
	z.save(t, tags)

	dan := z.user(t, "dan")
	z.user(t, "eve")
	pip := z.rec(t, "pip", "zoo_animals", map[string]any{"name": "Pip", "species": "parrot", "owner": dan.Id, "weight": 1})
	z.treat(t, "t1", z.id("kit"), 40, "2026-01-07")
	z.treat(t, "t2", z.id("zed"), 60, "2026-02-02")
	z.treat(t, "t3", z.id("leo"), 25, "2026-03-01")
	z.treat(t, "t4", pip.Id, 10, "2026-03-15")
	z.rec(t, "tag1", "zoo_tags", map[string]any{"tag": "tasty", "feeding": z.id("f1")})
	z.rec(t, "tag2", "zoo_tags", map[string]any{"tag": "fresh", "feeding": z.id("f3")})
}

func (z *zoo) treat(t testing.TB, key, animal string, price int, day string) {
	t.Helper()
	z.rec(t, key, "zoo_treats", map[string]any{
		"animal": animal, "price": price, "currency": "USD", "given": day + " 00:00:00.000Z", "label": key,
	})
}

func registerZooMeals() {
	RegisterSource(Source{
		Key: "zoo.meals", Label: "Meals",
		Parts: []SourcePart{
			{
				Collection: "zoo_feedings",
				Columns:    map[string]string{"cost": "cost", "currency": "currency", "grams": "grams", "fed": "fed", "note": "note", "animal": "animal"},
				Values:     map[string]any{"kind": "feeding"},
			},
			{
				Collection: "zoo_treats",
				Columns:    map[string]string{"cost": "price", "currency": "currency", "fed": "given", "note": "label", "animal": "animal"},
				Values:     map[string]any{"kind": "treat"},
			},
		},
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "feeding", Label: "Feeding"}, {Value: "treat", Label: "Treat"}}},
			{Key: "cost", Label: "Cost", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "grams", Label: "Grams", Type: Number, Aggregates: SumAll},
			{Key: "fed", Label: "Fed", Type: Date, Aggregates: MinMax},
		},
		Links:    []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
		Currency: func(core.App, *core.RequestInfo) string { return "USD" },
		// No meals report reads sites, so a run's zoo_sites topic comes from Watch alone.
		Watch: []string{"zoo_sites"},
	})
	RegisterSource(Source{
		Key: "zoo.pets", Collection: "zoo_animals", Label: "Pets",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
		Links:  []Link{{Key: "meals", Label: "Meals", Target: "zoo.meals", Cardinality: ToMany, BackField: "animal"}},
	})
	RegisterSource(Source{
		Key: "zoo.tags", Collection: "zoo_tags", Label: "Tags",
		Fields: []Field{{Key: "tag", Label: "Tag", Type: Text}},
		Links:  []Link{{Key: "meal", Label: "Meal", Target: "zoo.meals", Cardinality: ToOne, Field: "feeding"}},
	})
}

// newUnionZoo is the zoo with its union, checked.
func newUnionZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	return z
}
```

- [ ] **Step 2: Write the failing tests**

Create `core/server/reports/union_check_test.go`:

```go
package reports

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"
)

func validUnion() Source {
	return Source{
		Key: "zoo.u", Label: "U",
		Parts: []SourcePart{
			{Collection: "zoo_feedings", Columns: map[string]string{"note": "note", "animal": "animal"}, Values: map[string]any{"kind": "feeding"}},
			{Collection: "zoo_treats", Columns: map[string]string{"note": "label", "animal": "animal"}, Values: map[string]any{"kind": "treat"}},
		},
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "feeding", Label: "Feeding"}, {Value: "treat", Label: "Treat"}}},
		},
		Links: []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
	}
}

func TestRegisterSourceRefusesAMalformedUnion(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"collection and parts":       {func(s *Source) { s.Collection = "zoo_feedings" }, "names its collections in its parts"},
		"one part":                   {func(s *Source) { s.Parts = s.Parts[:1] }, "at least two parts"},
		"part with no collection":    {func(s *Source) { s.Parts[1].Collection = "" }, "needs a collection"},
		"a collection twice":         {func(s *Source) { s.Parts[1].Collection = "zoo_feedings" }, "is a part twice"},
		"field with a column":        {func(s *Source) { s.Fields[0].Column = "label" }, "parts map its columns"},
		"part maps id":               {func(s *Source) { s.Parts[0].Columns["id"] = "id" }, `"id" is not a key a part maps`},
		"part maps a blank column":   {func(s *Source) { s.Parts[0].Columns["note"] = "" }, "maps no column"},
		"value of no field":          {func(s *Source) { s.Parts[0].Values["shade"] = "red" }, "is not a field of the source"},
		"value and column":           {func(s *Source) { s.Parts[0].Columns["kind"] = "note" }, "both a column and a value"},
		"value of the wrong type":    {func(s *Source) { s.Parts[0].Values["kind"] = 42 }, "does not fit"},
		"select value not an option": {func(s *Source) { s.Parts[0].Values["kind"] = "snack" }, "does not fit"},
		"field no part reads":        {func(s *Source) { s.Fields = append(s.Fields, Field{Key: "grams", Label: "Grams", Type: Number}) }, `field "grams": no part maps it`},
		"to-one link no part maps": {func(s *Source) {
			s.Links = append(s.Links, Link{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"})
		}, `link "site": no part maps its field "site"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			src := validUnion()
			src.Fields = append([]Field(nil), src.Fields...)
			src.Parts = append([]SourcePart(nil), src.Parts...)
			for i := range src.Parts {
				src.Parts[i].Columns = maps.Clone(src.Parts[i].Columns)
				src.Parts[i].Values = maps.Clone(src.Parts[i].Values)
			}
			c.edit(&src)
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("RegisterSource did not panic")
				}
				if msg, _ := r.(string); !strings.Contains(msg, c.want) {
					t.Fatalf("panic %q does not mention %q", r, c.want)
				}
			}()
			RegisterSource(src)
		})
	}
}

// The registry keeps its own copy of the parts: a caller that changes its maps
// after registering changes nothing.
func TestRegisterSourceCopiesTheParts(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	src := validUnion()
	RegisterSource(src)
	src.Parts[0].Columns["note"] = "changed"
	src.Parts[0].Values["kind"] = "treat"
	got := lookupRegistered("zoo.u").Parts[0]
	if got.Columns["note"] != "note" || got.Values["kind"] != "feeding" {
		t.Fatalf("registered part = %+v", got)
	}
}

func TestCheckSourcesPassesAUnionAndLinksToAndFromIt(t *testing.T) {
	z := newUnionZoo(t)
	for _, key := range []string{"zoo.meals", "zoo.pets", "zoo.tags"} {
		if lookupSource(z.app, key) == nil {
			t.Fatalf("%s is not usable: %v", key, Refused(z.app))
		}
	}
}

func TestCheckSourcesRefusesABrokenUnion(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"unknown part collection": {func(s *Source) { s.Parts[1].Collection = "zoo_ghosts" }, `collection "zoo_ghosts" does not exist`},
		"unknown part column":     {func(s *Source) { s.Parts[1].Columns["note"] = "tail" }, `part "zoo_treats": column "tail" does not exist`},
		"unknown column of a key that is no field": {func(s *Source) { s.Parts[1].Columns["extra"] = "tail" },
			`key "extra": part "zoo_treats": column "tail" does not exist`},
		"part column of another type": {func(s *Source) { s.Parts[1].Columns["note"] = "remark" },
			`column "remark" of "zoo_treats" is of type editor, but column "note" of "zoo_feedings" is of type text`},
		"part with no list rule": {func(s *Source) {
			s.Parts[1] = SourcePart{Collection: "zoo_vault", Columns: map[string]string{"note": "name", "animal": "animal"}, Values: map[string]any{"kind": "treat"}}
		}, `part collection "zoo_vault" has no list rule`},
		"to-one link through a relation to elsewhere": {func(s *Source) { s.Links[0].Target = "zoo.sites" },
			`relation "animal" of "zoo_feedings" does not point at "zoo_sites"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			addZooTreats(t, z)
			src := validUnion()
			for i := range src.Parts {
				src.Parts[i].Columns = maps.Clone(src.Parts[i].Columns)
			}
			c.edit(&src)
			RegisterSource(src)
			err := CheckSources(z.app)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("CheckSources = %v, want it to contain %q", err, c.want)
			}
			if lookupSource(z.app, "zoo.u") != nil {
				t.Fatal("a refused union is still usable")
			}
			if lookupSource(z.app, "zoo.feedings") == nil {
				t.Fatal("a sound source was refused")
			}
		})
	}
}

// A to-many link into a union needs its back relation mapped by a part, and a
// source that links to a refused union is refused with it.
func TestCheckSourcesRefusesLinksIntoABrokenUnion(t *testing.T) {
	z := newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	RegisterSource(Source{
		Key: "zoo.owners", Collection: "zoo_animals", Label: "Owners",
		Links: []Link{{Key: "meals", Label: "Meals", Target: "zoo.meals", Cardinality: ToMany, BackField: "owner"}},
	})
	err := CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `no part of "zoo.meals" maps "owner"`) {
		t.Fatalf("CheckSources = %v", err)
	}

	z = newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	broken := *lookupRegistered("zoo.meals")
	broken.Parts = append([]SourcePart(nil), broken.Parts...)
	// A column that does not exist refuses the union and leaves its collections
	// resolvable, so zoo.pets is refused by the cascade (check.go), not by a
	// missing target collection.
	broken.Parts[1].Columns = maps.Clone(broken.Parts[1].Columns)
	broken.Parts[1].Columns["note"] = "tail"
	RegisterSource(broken)
	err = CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `link "meals" targets source "zoo.meals", which is refused`) {
		t.Fatalf("CheckSources = %v", err)
	}
	if lookupSource(z.app, "zoo.pets") != nil {
		t.Fatal("a link to a refused union kept its source")
	}
}

// The catalog offers a union like any source: its fields and links, never its
// collections or parts.
func TestCatalogListsAUnionLikeAnySource(t *testing.T) {
	z := newUnionZoo(t)
	var meals *CatalogSource
	for _, cs := range Catalog(z.app) {
		if cs.Key == "zoo.meals" {
			meals = &cs
		}
	}
	if meals == nil {
		t.Fatal("zoo.meals is not in the catalog")
	}
	var keys []string
	for _, f := range meals.Fields {
		keys = append(keys, f.Key)
	}
	if strings.Join(keys, ",") != "id,note,kind,cost,currency,grams,fed" {
		t.Fatalf("fields = %v", keys)
	}
	if len(meals.Links) != 1 || meals.Links[0].Key != "animal" || meals.Links[0].Target != "zoo.animals" {
		t.Fatalf("links = %+v", meals.Links)
	}
	raw, err := json.Marshal(meals)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"zoo_feedings", "zoo_treats", "parts", "collection"} {
		if strings.Contains(strings.ToLower(string(raw)), word) {
			t.Fatalf("the catalog names %q: %s", word, raw)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'Union|TestRegisterSourceCopiesTheParts|TestCheckSourcesRefusesLinksIntoABrokenUnion' -count=1`
Expected: FAIL to build: `undefined: SourcePart` and `unknown field Parts in struct literal of type Source`. Record this output as the red proof.

- [ ] **Step 4: Implement the registry part**

In `core/server/reports/registry.go`, add after the `Link` type:

```go
// SourcePart is one collection of a union source. The source's rows are the
// rows of every part's collection, each read under that collection's own list
// rule. Columns maps a source key (a field's key, a to-one link's Field, or
// the BackField of a to-many link from another source) to this part's column;
// a key the part does not map reads NULL on its rows. Values gives a field of
// the source a constant on every row of this part (a document kind).
type SourcePart struct {
	Collection string
	Columns    map[string]string
	Values     map[string]any
}
```

In `Source`, after `Collection string`, add:

```go
	// Parts makes the source a union of collections (see SourcePart). A union
	// sets Parts and leaves Collection empty; its fields name no Column (each
	// part maps its own) and its to-one links' Field is a key the parts map.
	Parts []SourcePart
```

In `RegisterSource`, after `src.Aliases = maps.Clone(src.Aliases)`:

```go
	src.Parts = slices.Clone(src.Parts)
	for i := range src.Parts {
		src.Parts[i].Columns = maps.Clone(src.Parts[i].Columns)
		src.Parts[i].Values = maps.Clone(src.Parts[i].Values)
	}
```

In `validateSource`, replace the `if src.Collection == "" || src.Label == ""` block with:

```go
	union := len(src.Parts) > 0
	switch {
	case src.Label == "" || (src.Collection == "" && !union):
		return fmt.Errorf("a source needs a collection and a label")
	case union && src.Collection != "":
		return fmt.Errorf("a union source names its collections in its parts, not in Collection")
	case union && len(src.Parts) < 2:
		return fmt.Errorf("a union source has at least two parts")
	}
```

In the fields loop, before `if f.Column == "" {`:

```go
		if union && f.Column != "" && f.Column != f.Key {
			return fmt.Errorf("field %q: a union source's parts map its columns", f.Key)
		}
```

Before the aliases loop (after the links loop):

```go
	if union {
		if err := validateParts(src); err != nil {
			return err
		}
	}
```

Add at the end of the file:

```go
// validateParts checks a union's parts against its fields and links: every
// part has its own collection, maps keys to columns and gives fields constants
// that fit them, and every field and to-one link is read by at least one part.
func validateParts(src *Source) error {
	fields := map[string]*Field{}
	for i := range src.Fields {
		fields[src.Fields[i].Key] = &src.Fields[i]
	}
	seen := map[string]bool{}
	read := map[string]bool{}
	for i, p := range src.Parts {
		if p.Collection == "" {
			return fmt.Errorf("part %d needs a collection", i)
		}
		if seen[p.Collection] {
			return fmt.Errorf("part %d: collection %q is a part twice", i, p.Collection)
		}
		seen[p.Collection] = true
		for key, column := range p.Columns {
			switch {
			case key == idField.Key || !memberKeyRe.MatchString(key):
				return fmt.Errorf("part %q: %q is not a key a part maps", p.Collection, key)
			case column == "":
				return fmt.Errorf("part %q: key %q maps no column", p.Collection, key)
			}
			read[key] = true
		}
		for key, v := range p.Values {
			f := fields[key]
			switch {
			case f == nil || key == idField.Key:
				return fmt.Errorf("part %q: value %q is not a field of the source", p.Collection, key)
			case p.Columns[key] != "":
				return fmt.Errorf("part %q: %q is both a column and a value", p.Collection, key)
			case !constantFits(f, v):
				return fmt.Errorf("part %q: value %v does not fit the %s field %q", p.Collection, v, f.Type, key)
			}
			read[key] = true
		}
	}
	for _, f := range src.Fields {
		if f.Key != idField.Key && !read[f.Key] {
			return fmt.Errorf("field %q: no part maps it or gives it a value", f.Key)
		}
	}
	for _, l := range src.Links {
		if l.Cardinality == ToOne && !read[l.Field] {
			return fmt.Errorf("link %q: no part maps its field %q", l.Key, l.Field)
		}
	}
	return nil
}

// constantFits is whether a part's constant can be a value of f: text for a
// text field, one of its options for a select, a number for a number or money
// field, true or false for a yes/no field.
func constantFits(f *Field, v any) bool {
	switch f.Type {
	case Text:
		_, ok := v.(string)
		return ok
	case Select:
		s, ok := v.(string)
		return ok && slices.ContainsFunc(f.Options, func(o Option) bool { return o.Value == s })
	case Number, Money:
		switch v.(type) {
		case int, int64, float64:
			return true
		}
	case Bool:
		_, ok := v.(bool)
		return ok
	}
	return false
}

// collections are the collections a source reads: its one collection, or
// each part's, in part order.
func (s *Source) collections() []string {
	if len(s.Parts) == 0 {
		return []string{s.Collection}
	}
	out := make([]string, len(s.Parts))
	for i, p := range s.Parts {
		out[i] = p.Collection
	}
	return out
}

// unionKeys are the keys a union's rule CTE projects besides id, sorted: its
// fields, its to-one links' fields and every key a part maps or gives a value.
func (s *Source) unionKeys() []string {
	set := map[string]bool{}
	for _, f := range s.Fields {
		if f.Key != idField.Key {
			set[f.Key] = true
		}
	}
	for _, l := range s.Links {
		if l.Cardinality == ToOne {
			set[l.Field] = true
		}
	}
	for _, p := range s.Parts {
		for k := range p.Columns {
			set[k] = true
		}
		for k := range p.Values {
			set[k] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}
```

- [ ] **Step 5: Implement the check part**

In `core/server/reports/check.go`:

1. At the top of `checkSource`, add `if len(src.Parts) > 0 { return checkUnionSource(app, src) }`.
2. Replace the first lines of `checkSource` (finding the collection, the name check, the system check) with a call to `partCollection`:

```go
	col, msg := partCollection(app, src.Collection)
	if msg != "" {
		return []string{msg}
	}
```

3. In `checkSource`'s links loop call `checkLink(app, src, l)`, and replace the Watch loop with `problems = append(problems, checkWatch(app, src)...)`.
4. Replace `checkLink` and add the helpers:

```go
// partCollection finds a collection a source reads, by its name, and refuses
// a system collection.
func partCollection(app core.App, name string) (*core.Collection, string) {
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return nil, fmt.Sprintf("collection %q does not exist", name)
	}
	if col.Name != name {
		return nil, fmt.Sprintf("collection %q must be named %q, not by id", name, col.Name)
	}
	if col.System {
		return nil, fmt.Sprintf("collection %q is a system collection", col.Name)
	}
	return col, ""
}

func checkWatch(app core.App, src *Source) []string {
	var problems []string
	for _, w := range src.Watch {
		wc, err := app.FindCollectionByNameOrId(w)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("watched collection %q does not exist", w))
		case wc.Name != w:
			problems = append(problems, fmt.Sprintf("watched collection %q must be named %q, not by id", w, wc.Name))
		case wc.System:
			problems = append(problems, fmt.Sprintf("watched collection %q is a system collection", w))
		}
	}
	return problems
}

// checkUnionSource checks each part of a union as checkSource checks a
// source's one collection, and that the parts agree: every part that maps a
// key maps it to a column of one PocketBase type, so the union's column has
// one type. A part collection must have a list rule (R2).
func checkUnionSource(app core.App, src *Source) []string {
	cols := make([]*core.Collection, len(src.Parts))
	for i, p := range src.Parts {
		col, msg := partCollection(app, p.Collection)
		if msg != "" {
			return []string{msg}
		}
		cols[i] = col
	}
	var problems []string
	for _, f := range src.Fields {
		if f.Key == idField.Key {
			continue
		}
		first, firstType := "", ""
		for i, p := range src.Parts {
			column, ok := p.Columns[f.Key]
			if !ok {
				continue
			}
			mapped := f
			mapped.Column = column
			if msg := checkField(cols[i], mapped); msg != "" {
				problems = append(problems, fmt.Sprintf("field %q: part %q: %s", f.Key, p.Collection, msg))
				continue
			}
			typ := cols[i].Fields.GetByName(column).Type()
			if first == "" {
				first, firstType = fmt.Sprintf("column %q of %q", column, p.Collection), typ
			} else if typ != firstType {
				problems = append(problems, fmt.Sprintf("field %q: column %q of %q is of type %s, but %s is of type %s",
					f.Key, column, p.Collection, typ, first, firstType))
			}
		}
	}
	// A key that is not a field (a link's Field, a BackField of a link from
	// another source) is projected too, so its column must exist.
	isField := map[string]bool{}
	for _, f := range src.Fields {
		isField[f.Key] = true
	}
	for i, p := range src.Parts {
		for _, key := range slices.Sorted(maps.Keys(p.Columns)) {
			column := p.Columns[key]
			if !isField[key] && cols[i].Fields.GetByName(column) == nil {
				problems = append(problems, fmt.Sprintf("key %q: part %q: column %q does not exist", key, p.Collection, column))
			}
		}
	}
	for _, l := range src.Links {
		if msg := checkLink(app, src, l); msg != "" {
			problems = append(problems, fmt.Sprintf("link %q: %s", l.Key, msg))
		}
	}
	problems = append(problems, checkWatch(app, src)...)
	for _, col := range cols {
		if col.ListRule == nil {
			problems = append(problems, fmt.Sprintf("part collection %q has no list rule: only superusers may list it", col.Name))
			continue
		}
		if msg := checkRule(app, col); msg != "" {
			problems = append(problems, msg)
		}
	}
	return problems
}

// ownedColumn is a stored relation column a link follows: its collection and
// column name.
type ownedColumn struct{ collection, column string }

// linkOwners are the stored columns a source's key reads: on a one-collection
// source the column of that name (a link's Field or BackField is a column
// there), on a union each part's mapped column (a part that maps none has
// none).
func linkOwners(src *Source, key string) []ownedColumn {
	if len(src.Parts) == 0 {
		return []ownedColumn{{src.Collection, key}}
	}
	var out []ownedColumn
	for _, p := range src.Parts {
		if column, ok := p.Columns[key]; ok {
			out = append(out, ownedColumn{p.Collection, column})
		}
	}
	return out
}

func sourceCollections(app core.App, src *Source) ([]*core.Collection, string) {
	names := src.collections()
	out := make([]*core.Collection, 0, len(names))
	for _, name := range names {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return nil, fmt.Sprintf("collection %q does not exist", name)
		}
		out = append(out, col)
	}
	return out, ""
}

// checkLink checks the stored relations a link follows: a to-one link's Field
// on this source, a to-many link's BackField on its target, each a single
// visible relation that points at the other side. On a union the relation is
// every part's column for that key, and the other side's collection is any of
// its parts'.
func checkLink(app core.App, src *Source, l Link) string {
	target := sources[l.Target]
	if target == nil {
		return fmt.Sprintf("target source %q is not registered", l.Target)
	}
	targetCols, msg := sourceCollections(app, target)
	if msg != "" {
		return "target " + msg
	}
	srcCols, msg := sourceCollections(app, src)
	if msg != "" {
		return msg
	}
	owners, pointsAt := linkOwners(src, l.Field), targetCols
	if l.Cardinality == ToMany {
		owners, pointsAt = linkOwners(target, l.BackField), srcCols
		if len(owners) == 0 {
			return fmt.Sprintf("no part of %q maps %q", l.Target, l.BackField)
		}
	}
	for _, o := range owners {
		owner, err := app.FindCollectionByNameOrId(o.collection)
		if err != nil {
			return fmt.Sprintf("collection %q does not exist", o.collection)
		}
		if msg := relationCheck(owner, o.column, pointsAt); msg != "" {
			return msg
		}
	}
	return ""
}

func relationCheck(owner *core.Collection, column string, pointsAt []*core.Collection) string {
	rel, ok := owner.Fields.GetByName(column).(*core.RelationField)
	if !ok {
		return fmt.Sprintf("%q is not a relation field of %q", column, owner.Name)
	}
	if rel.IsMultiple() {
		return fmt.Sprintf("relation %q of %q holds many values", column, owner.Name)
	}
	if rel.GetHidden() {
		return fmt.Sprintf("relation %q of %q is hidden from the API", column, owner.Name)
	}
	if !slices.ContainsFunc(pointsAt, func(c *core.Collection) bool { return c.Id == rel.CollectionId }) {
		names := make([]string, len(pointsAt))
		for i, c := range pointsAt {
			names[i] = strconv.Quote(c.Name)
		}
		return fmt.Sprintf("relation %q of %q does not point at %s", column, owner.Name, strings.Join(names, " or "))
	}
	return ""
}
```

Add `"maps"` and `"strconv"` (and `"slices"` / `"strings"` if absent) to check.go's imports. The existing messages (`relation "site" of "zoo_animals" does not point at "users"`, `"note" is not a relation field of "zoo_feedings"`, `target collection %q does not exist`) keep their words.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'Union|TestRegisterSource|TestCheckSources|TestCatalog' -count=1`
Expected: PASS (the old `TestCheckSources*` and `TestRegisterSource*` cases too).

- [ ] **Step 7: Run the whole package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: PASS. A failure is fixed at its source before the task ends.

- [ ] **Step 8: Hand back**

Files changed: `core/server/reports/registry.go`, `core/server/reports/check.go`, `core/server/reports/union_zoo_test.go`, `core/server/reports/union_check_test.go`. No commit (the driver commits).

---

### Task 2: U1 — union runs (rule CTE, links, groups, fill, options, lookups, topics)

**Files:**
- Modify: `core/server/reports/rules.go` (new `unionSelect`)
- Modify: `core/server/reports/stmt.go` (`rule` 141-151, `with` 154-180, `newScope` 262-269, `scope.join` 274-294)
- Modify: `core/server/reports/lookup.go` (`storedLinkCol` 442-456)
- Modify: `core/server/reports/run.go` (Watch topics loop 470-478)
- Create: `core/server/reports/union_run_test.go`

**Interfaces:**
- Consumes: Task 1's `SourcePart`, `Source.Parts`, `(*Source).collections()`, `(*Source).unionKeys()`, `newUnionZoo`, `registerZooMeals`; zoo helpers `z.run`, `z.try`, `z.options`, `z.listIDs`, `z.mux`, `z.id`, `requireRows`, `requireOrderedRows`, `optionLabels`, `rowsOf`; `changesignal.Topic`.
- Produces:
  - `const unionRulePrefix = "union:"` (a collection name never holds a colon)
  - `func (s *stmt) sourceRule(src *Source) string` — the rule CTE name of a source (its collection's, or its union's).
  - `func (s *stmt) ruleRead(name string) (string, dbx.Params, error)` — the select and params of one `s.rules` entry.
  - `func unionSelect(app core.App, src *Source, info *core.RequestInfo) (string, dbx.Params, error)`

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/union_run_test.go`:

```go
package reports

import (
	"slices"
	"testing"

	"tinycld.org/core/changesignal"
)

const mealsReport = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}]}`

// The column of the kind field has the id "type": rowsOf (run_test.go) reads
// "kind" as the row's own kind (detail, group, total), not as a cell.

// A union reads the rows of both parts; a key a part does not map (a treat's
// grams) reads blank, and a part's constant (kind) reads on every row of it.
func TestAUnionRunReadsTheRowsOfEveryPart(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", mealsReport, Options{}, nil)
	requireOrderedRows(t, mealsReport, res, "label", "f1", "f2", "f3", "f4", "t1", "t3", "Total")
	requireRows(t, res, "type", "feeding", "feeding", "feeding", "feeding", "treat", "treat", nil)
	requireRows(t, res, "cost", int64(1000), int64(2500), int64(700), int64(300), int64(40), int64(25), int64(4565))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
	requireRows(t, res, "grams", int64(500), int64(800), int64(300), int64(100), nil, nil, int64(1700))
}

// Each part is read under its own list rule, exactly as the list API lists
// it: feedings by keeping the animal, treats by owning it.
func TestAUnionReadsEachPartUnderItsOwnRule(t *testing.T) {
	z := newUnionZoo(t)
	mux := z.mux(t)
	ids := `{"version":4,"source":"zoo.meals","columns":[{"id":"id","path":[{"field":"id"}]}],"sort":[{"col":"id"}]}`
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		res := z.run(t, user, ids, Options{}, nil)
		var got []string
		for _, r := range res.Rows {
			if r.Kind == KindDetail {
				got = append(got, r.Cells["id"].(string))
			}
		}
		want := append(z.listIDs(t, mux, "zoo_feedings", user, ""), z.listIDs(t, mux, "zoo_treats", user, "")...)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s reads %v, the list API lists %v", user, got, want)
		}
	}
	requireOrderedRows(t, mealsReport, z.run(t, "dan", mealsReport, Options{}, nil), "label", "t4", "Total")
	requireOrderedRows(t, mealsReport, z.run(t, "bob", mealsReport, Options{}, nil), "label", "f3", "f4", "f5", "t2", "Total")
}

// Review Focus 1: a reader who may list no row of either part gets an empty
// report in the source's currency, not an error.
func TestAUnionReaderWithNoRowsSeesAnEmptyReport(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "eve", mealsReport, Options{}, nil)
	requireRows(t, res, "kind", KindTotal)
	requireRows(t, res, "label", "Total")
	requireRows(t, res, "cost", int64(0))
	requireRows(t, res, "cost@currency", "USD")
}

// A run's topics name every part's collection, and the collections the
// source's Watch names (no meals report reads zoo_sites otherwise).
func TestAUnionRunNamesEveryPartsTopic(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", mealsReport, Options{}, nil)
	for _, c := range []string{"zoo_feedings", "zoo_treats", "zoo_sites"} {
		if !slices.Contains(res.Topics, changesignal.Topic(c)) {
			t.Fatalf("topics %v miss %s", res.Topics, c)
		}
	}
}

// Review Focus 5: a group by a part's constant totals each part apart.
func TestAGroupByAUnionConstantTotalsEachPart(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals","columns":[
		{"id":"type","path":[{"field":"kind"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "type", "feeding", "treat", nil)
	requireRows(t, res, "cost", int64(4500), int64(65), int64(4565))
}

// A link from a union to a one-collection source, and a to-many link from a
// one-collection source into a union (each part maps the back relation).
func TestLinksFromAndIntoAUnion(t *testing.T) {
	z := newUnionZoo(t)
	from := `{"version":4,"source":"zoo.meals","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"},{"field":"name"}]}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", from, Options{}, nil)
	requireOrderedRows(t, from, res, "animal", "Leo", "Leo", "Zed", "Zed", "Kit", "Leo", nil)

	into := `{"version":4,"source":"zoo.pets","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"cost","path":[{"link":"meals","mode":"aggregate"},{"field":"cost"}],"aggregate":"sum"}],
		"sort":[{"col":"name"}]}`
	res = z.run(t, "ann", into, Options{}, nil)
	requireOrderedRows(t, into, res, "label", "Kit", "Leo", "Zed", "Total")
	requireRows(t, res, "cost", int64(40), int64(3525), int64(1000), int64(4565))
}

// A lookup sums one amount across both parts: one source, one currency.
func TestALookupSumsAcrossTheParts(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.pets","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"pet","path":[{"field":"id"}]},
		{"id":"meals","formula":{"op":"lookup","source":"zoo.meals","pick":"sum",
			"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
			"value":{"path":[{"field":"cost"}]}}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Kit", "Leo", "Zed", "Total")
	requireRows(t, res, "meals", int64(40), int64(3525), int64(1000), nil)
	requireRows(t, res, "meals@currency", "USD", "USD", "USD", nil)
}

// A record group on a link into a union is labelled by the union record, and
// its fill adds the union records with no rows.
func TestARecordGroupOnAUnionLinkAndItsFill(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.tags","columns":[
		{"id":"meal","path":[{"link":"meal"}],"labelField":"note"},
		{"id":"tags","path":[{"field":"tag"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"meal"%s}]}`
	res := z.run(t, "ann", fmt.Sprintf(spec, ""), Options{}, nil)
	requireRows(t, res, "label", "f1", "f3", "Total")
	requireRows(t, res, "tags", int64(1), int64(1), int64(2))

	filled := z.run(t, "ann", fmt.Sprintf(spec, `,"fill":{}`), Options{}, nil)
	requireRows(t, filled, "label", "f1", "f2", "f3", "f4", "t1", "t3", "Total")
}

// A source-backed param over a union lists the union's records the reader
// may see, from both parts.
func TestAParamListsItsOptionsFromAUnion(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals",
		"params":[{"id":"meal","type":"choice","multiple":true,
			"optionsFrom":{"source":"zoo.meals","value":[{"field":"id"}],"label":[{"field":"note"}]}}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"meal"}]}}`
	labels := func(user string) []string {
		got := optionLabels(z.options(t, user, spec, OptionsRequest{Param: "meal"}))
		slices.Sort(got)
		return got
	}
	if got := labels("ann"); !slices.Equal(got, []string{"f1", "f2", "f3", "f4", "t1", "t3"}) {
		t.Fatalf("ann's options = %v", got)
	}
	if got := labels("dan"); !slices.Equal(got, []string{"t4"}) {
		t.Fatalf("dan's options = %v", got)
	}
}
```

Add `"fmt"` to the imports (used by `TestARecordGroupOnAUnionLinkAndItsFill`).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'Union|TestLinksFromAndIntoAUnion|TestALookupSumsAcrossTheParts|TestAParamListsItsOptionsFromAUnion' -count=1`
Expected: FAIL: each run fails with `reports: collection "": ...` (the scope reads the rule of the union's empty Collection). Record it as the red proof.

- [ ] **Step 3: Implement the union rule CTE**

In `core/server/reports/rules.go`, add (imports `strings`):

```go
// unionSelect is a union source's rule CTE: each part's rule select
// (ruleSelect, the part's own list rule), projected to the source's keys and
// joined by UNION ALL. A key a part does not map reads NULL there; a part's
// constants are bound, under names that hold the source key, so two unions in
// one statement never bind one name twice.
func unionSelect(app core.App, src *Source, info *core.RequestInfo) (string, dbx.Params, error) {
	keys := src.unionKeys()
	stem := "u_" + strings.ReplaceAll(src.Key, ".", "_")
	params := dbx.Params{}
	selects := make([]string, 0, len(src.Parts))
	for i, part := range src.Parts {
		col, err := app.FindCachedCollectionByNameOrId(part.Collection)
		if err != nil {
			return "", nil, fmt.Errorf("reports: collection %q: %w", part.Collection, err)
		}
		ruleSQL, ruleParams, err := ruleSelect(app, col, info)
		if err != nil {
			return "", nil, err
		}
		if err := mergeParams(params, ruleParams); err != nil {
			return "", nil, err
		}
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
		selects = append(selects, "SELECT "+strings.Join(cols, ", ")+" FROM ("+ruleSQL+") p")
	}
	return strings.Join(selects, "\nUNION ALL\n"), params, nil
}
```

- [ ] **Step 4: Read the union's CTE wherever a source is read**

In `core/server/reports/stmt.go`:

```go
// unionRulePrefix marks a union source's entry in stmt.rules: "union:<key>".
// A collection name never holds a colon.
const unionRulePrefix = "union:"

// sourceRule names the rule CTE a source's rows are read from: its
// collection's, or its union's.
func (s *stmt) sourceRule(src *Source) string {
	if len(src.Parts) > 0 {
		return s.rule(unionRulePrefix + src.Key)
	}
	return s.rule(src.Collection)
}

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

In `with`, replace the body of `if s.app != nil { ... }` with:

```go
		if s.app != nil {
			ruleSQL, params, err := s.ruleRead(name)
			if err != nil {
				return "", err
			}
			if err := mergeParams(s.params, params); err != nil {
				return "", err
			}
			sql = ruleSQL
			s.ruleReads[s.ruleOf[name]] = ruleRead{sql: ruleSQL, params: params}
		}
```

In `newScope`, `from: s.sourceRule(src) + " " + alias,`. In `scope.join`, `kind, sc.s.sourceRule(h.to), next, q(next, "id"), q(alias, h.link.Field)))`. Update the `rule` doc comment: "rule names the CTE that holds the rows of a `s.rules` entry (a collection, or `union:<source key>`) the reader may list."

- [ ] **Step 5: Lookup bound and Watch topics**

In `core/server/reports/lookup.go` `storedLinkCol`, after `if len(hops) != 1 { return "" }`:

```go
	// A union's link column is NULL on a part that does not map it, as a
	// view's may be.
	if len(hops[0].from.Parts) > 0 {
		return ""
	}
```

In `core/server/reports/run.go`, replace the Watch loop with:

```go
	for _, src := range Sources(rn.app) {
		reads := slices.ContainsFunc(src.collections(), func(c string) bool {
			return rn.topics[changesignal.Topic(c)]
		})
		if reads {
			for _, w := range src.Watch {
				rn.topics[changesignal.Topic(w)] = true
			}
		}
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'Union|TestLinksFromAndIntoAUnion|TestALookupSumsAcrossTheParts|TestAParamListsItsOptionsFromAUnion' -count=1`
Expected: PASS, and no "topics ... miss" lines from the package's `TestMain` topics check.

- [ ] **Step 7: Run the whole package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: PASS (including `FuzzRuleParity`'s seed corpus and `TestMain`'s topics check).

- [ ] **Step 8: Hand back**

Files changed: `core/server/reports/rules.go`, `stmt.go`, `lookup.go`, `run.go`, `union_run_test.go`. No commit.

---

### Task 3: G2 — `columns[].blankAbove` in the spec and its refusals

**Files:**
- Modify: `core/server/reports/spec.go` (`Column`, after `Heading`, line 142)
- Modify: `core/server/reports/bind.go` (`colPlan` 105-112, `bind` 197-233, new `blankAbove` after `groupsAndPivot`)
- Create: `core/server/reports/blankabove_bind_test.go`

**Interfaces:**
- Consumes: `Load`, `Validate`, `withZooRegistry`, `requireSpecError`, `newZoo`, `z.try`.
- Produces: `Column.BlankAbove string` (`json:"blankAbove,omitempty"`); `colPlan.blankLevel int` (1 + the index of the named group in `spec.Groups`, 0 when unset or refused); `func (b *binder) blankAbove(p *plan)`; refusal code `unknown_group` (422) at `columns.<i>.blankAbove`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/blankabove_bind_test.go`:

```go
package reports

import (
	"errors"
	"fmt"
	"testing"
)

// blankAboveSpec is a feedings report grouped by species, then animal; %s is
// the cost column's extra keys, %s more keys of the spec.
const blankAboveSpec = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"%s},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
	{"id":"animal","path":[{"link":"animal"}],"hidden":true}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true},{"id":"byAnimal","col":"animal","subtotals":true}]%s}`

func TestBlankAboveIsAcceptedOnAGroupedMoneyColumn(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(fmt.Sprintf(blankAboveSpec, `,"blankAbove":"byAnimal"`, "")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if spec.Columns[1].BlankAbove != "byAnimal" {
		t.Fatalf("blankAbove = %q", spec.Columns[1].BlankAbove)
	}
	if err := Validate(app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestBlankAboveRefusals(t *testing.T) {
	app := withZooRegistry(t)
	pivot := `,"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}`
	for _, c := range []struct {
		name, col, rest, at, want string
		status                    int
	}{
		{"unknown group", `,"blankAbove":"nope"`, "", "columns.1.blankAbove", `the report has no group "nope"`, 422},
		{"with a rollup", `,"blankAbove":"byAnimal","rollup":{"value":0}`, "", "columns.1.blankAbove", "a rollup already replaces", 400},
		{"with a rows total", `,"blankAbove":"byAnimal","total":"rows"`, "", "columns.1.blankAbove", "rows shown", 400},
		{"on a pivot's cell column", `,"blankAbove":"byAnimal"`, pivot, "columns.1.blankAbove", "pivot's cell column", 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(blankAboveSpec, c.col, c.rest)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}

	grouped := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}],"blankAbove":"g"}],
		"groups":[{"id":"g","col":"species"}]}`
	spec, _, err := Load([]byte(grouped))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.1.blankAbove", "grouped by")

	nested := `{"version":4,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"species","path":[{"field":"species"}]},
		{"id":"cost","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}],"blankAbove":"g"}],
		"groups":[{"id":"g","col":"species"}]}`
	spec, _, err = Load([]byte(nested))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.2.blankAbove", "child row column")
}

// R12: an unknown group is the column's own 422 problem, coded unknown_group,
// and the whole spec answers 422 through Run as through Validate.
func TestBlankAboveOnAnUnknownGroupIsA422ThroughRun(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", fmt.Sprintf(blankAboveSpec, `,"blankAbove":"nope"`, ""), Options{}, nil)
	requireSpecError(t, err, 422, "columns.1.blankAbove", `no group "nope"`)
	var se *SpecError
	if !errors.As(err, &se) || se.Problems["columns.1.blankAbove"].Code() != "unknown_group" {
		t.Fatalf("problem = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'BlankAbove' -count=1`
Expected: FAIL: the spec field does not exist yet, so `spec.Columns[1].BlankAbove` does not compile (`has no field or method BlankAbove`). Record it.

- [ ] **Step 3: Add the field**

In `core/server/reports/spec.go` `Column`, after `Heading`:

```go
	// BlankAbove names a group: the column totals on that group's level and the
	// levels inside it, and leaves its cell blank on the group and subtotal
	// rows of outer levels, layout sections and the Total (no currency cell
	// either). For a figure the outer levels would count twice.
	BlankAbove string `json:"blankAbove,omitempty"`
```

- [ ] **Step 4: Bind it**

In `core/server/reports/bind.go`, add to `colPlan` after `scale int`:

```go
	// blankLevel is the level (1 for the first group) the column totals up to
	// (Column.BlankAbove); 0 for every level.
	blankLevel int
```

In `bind`, after `b.groupsAndPivot(p)`, add `b.blankAbove(p)`. Add after `groupsAndPivot`:

```go
// blankAbove binds each column's blankAbove to the level of its group.
// Refused where the column has no totals of its own to stop at a level (a
// rollup, a total of the rows shown, a pivot's cell column, child rows),
// where it shows the group itself (a column a group groups by), and for a
// group the report does not have (422 unknown_group).
func (b *binder) blankAbove(p *plan) {
	spec := p.spec
	for _, cp := range p.cols {
		c := cp.col
		if c.BlankAbove == "" {
			continue
		}
		at := cp.at + ".blankAbove"
		gi := slices.IndexFunc(spec.Groups, func(g Group) bool { return g.ID == c.BlankAbove })
		switch {
		case gi < 0:
			b.missing(at, "unknown_group", fmt.Sprintf("the report has no group %q", c.BlankAbove))
		case c.Rollup != nil:
			b.bad(at, "a rollup already replaces the column's totals: it does not go with blankAbove")
		case c.Total == TotalRows:
			b.bad(at, "a total of the rows shown adds the rows of every level: it does not go with blankAbove")
		case spec.Pivot != nil && spec.Pivot.Cell == c.ID:
			b.bad(at, "a pivot's cell column is totalled by its pivot columns")
		case cp.nest > 0:
			b.bad(at, "a child row column has no totals to leave blank")
		case slices.ContainsFunc(spec.Groups, func(g Group) bool { return g.Col == c.ID }):
			b.bad(at, "a column a group is grouped by shows the group on its total rows")
		default:
			cp.blankLevel = gi + 1
		}
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'BlankAbove' -count=1`
Expected: PASS.

- [ ] **Step 6: Run the whole package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 7: Hand back**

Files changed: `core/server/reports/spec.go`, `bind.go`, `blankabove_bind_test.go`. No commit.

---

### Task 4: G2 — blank cells above the level (levels and assemble)

**Files:**
- Modify: `core/server/reports/levels.go` (`levelCtx.col`, 850-862)
- Modify: `core/server/reports/assemble.go` (`cells`, 842-881)
- Create: `core/server/reports/blankabove_test.go`

**Interfaces:**
- Consumes: Task 3's `colPlan.blankLevel`, `blankAboveSpec`; `newZoo`, `z.run`, `Row.Kind`, `Row.Level`, `Row.Cells`, `KindGroup`, `KindSubtotal`, `KindDetail`, `KindTotal`, `expr.withoutValue()`, `toInt`.
- Produces: on rows whose level is above `blankLevel` (group, subtotal and layout section rows of outer levels, and the Total, `__level` 0), the column has no cell, no `<id>@currency` and no `<id>@label`; `{col}` of it reads NULL on those levels.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/blankabove_test.go`:

```go
package reports

import (
	"fmt"
	"strings"
	"testing"
)

// ann sees leo (lion: f1 1000, f2 2500) and zed (zebra: f3 700, f4 300).
const blankAboveRun = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
	{"id":"animal","path":[{"link":"animal"}],"hidden":true},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"%s"},
	{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true},{"id":"byAnimal","col":"animal","subtotals":true}],
	"sort":[{"col":"note"}]}`

// outer is a row of the levels above level: its group and subtotal rows, and
// the Total.
func outer(r Row, level int) bool {
	return r.Kind == KindTotal || ((r.Kind == KindGroup || r.Kind == KindSubtotal) && r.Level < level)
}

func requireNoCell(t *testing.T, r Row, id string) {
	t.Helper()
	for _, k := range []string{id, id + "@currency", id + "@label"} {
		if _, ok := r.Cells[k]; ok {
			t.Fatalf("row %s (%s) carries %s = %v", r.ID, r.Kind, k, r.Cells[k])
		}
	}
}

func TestBlankAboveLeavesTheOuterLevelsAndTheTotalBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(blankAboveRun, "byAnimal"), Options{ExpandAll: true}, nil)
	var subtotals, details []any
	for _, r := range res.Rows {
		switch {
		case outer(r, 2):
			requireNoCell(t, r, "cost")
			// A {col} read of it there reads blank too (levels.go).
			if r.Cells["twice"] != nil {
				t.Fatalf("row %s: twice = %v", r.ID, r.Cells["twice"])
			}
			if r.Kind != KindGroup && r.Cells["grams"] == nil {
				t.Fatalf("row %s lost its grams", r.ID)
			}
		case r.Kind == KindSubtotal:
			subtotals = append(subtotals, r.Cells["cost"], r.Cells["cost@currency"], r.Cells["twice"])
		case r.Kind == KindDetail:
			details = append(details, r.Cells["cost"])
		}
	}
	if fmt.Sprint(subtotals) != fmt.Sprint([]any{int64(3500), "USD", int64(7000), int64(1000), "USD", int64(2000)}) {
		t.Fatalf("animal subtotals = %v", subtotals)
	}
	if fmt.Sprint(details) != fmt.Sprint([]any{int64(1000), int64(2500), int64(700), int64(300)}) {
		t.Fatalf("details = %v", details)
	}
	total := res.Rows[len(res.Rows)-1]
	if total.Kind != KindTotal || total.Cells["grams"] != int64(1700) {
		t.Fatalf("total = %+v", total)
	}
}

// Review Focus 4: in the default fold state the closed animal rows carry the
// figure and the species rows above them do not.
func TestBlankAboveHoldsOnACollapsedRun(t *testing.T) {
	z := newZoo(t)
	spec := fmt.Sprintf(blankAboveRun, "byAnimal")
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kind", KindGroup, KindGroup, KindTotal)
	for _, r := range res.Rows {
		requireNoCell(t, r, "cost")
	}
	requireRows(t, res, "grams", int64(1300), int64(400), int64(1700))

	// Species open, animals closed by default: the closed animal rows carry the
	// figure, the species rows above them and the Total do not.
	open := strings.Replace(spec, `"col":"species","subtotals":true`, `"col":"species","subtotals":true,"open":true`, 1)
	res = z.run(t, "ann", open, Options{}, nil)
	var animals []any
	for _, r := range res.Rows {
		if outer(r, 2) {
			requireNoCell(t, r, "cost")
			continue
		}
		if r.Kind == KindGroup && r.Level == 2 {
			animals = append(animals, r.Cells["cost"], r.Cells["cost@currency"])
		}
	}
	if fmt.Sprint(animals) != fmt.Sprint([]any{int64(3500), "USD", int64(1000), "USD"}) {
		t.Fatalf("closed animal rows = %v", animals)
	}
}

// Review Focus 2: on the first group only the Total leaves the cell blank.
func TestBlankAboveTheFirstLevelBlanksOnlyTheTotal(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(blankAboveRun, "bySpecies"), Options{}, nil)
	requireRows(t, res, "cost", int64(3500), int64(1000), nil)
	requireRows(t, res, "cost@currency", "USD", "USD", nil)
}

// On a layout report the sections are the first level: their rows leave the
// cell blank, the levels inside keep it.
func TestBlankAboveLeavesLayoutSectionsBlank(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	var inner []any
	for _, r := range res.Rows {
		if r.Level == 1 {
			requireNoCell(t, r, "cost")
			continue
		}
		if r.Kind == KindSubtotal {
			inner = append(inner, r.Cells["cost"])
		}
	}
	if fmt.Sprint(inner) != fmt.Sprint([]any{int64(3500), int64(1000)}) {
		t.Fatalf("animal subtotals = %v (rows %v)", inner, rowsOf(res, "id"))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestBlankAbove(Leaves|Holds|TheFirst)' -count=1`
Expected: FAIL: `row ... (group) carries cost = 3500` (Task 3 binds the level but nothing reads it yet). Record it.

- [ ] **Step 3: Read the column as blank above its level**

In `core/server/reports/levels.go` `levelCtx.col`, after the `cp == nil` check:

```go
	// Above its blankAbove level the column has no value: its own cell, and
	// every formula of the level that reads it (a rollup, a having, a sort
	// key, a derived column), read blank there.
	if lc.lv.n < cp.blankLevel {
		e, err := lc.recompute(cp, at)
		if err != nil {
			return expr{}, err
		}
		return e.withoutValue(), nil
	}
```

- [ ] **Step 4: Leave the cell out of those rows**

In `core/server/reports/assemble.go` `cells`, change `_, grouped := row["__level"]` to `level, grouped := row["__level"]`, and at the top of the columns loop, after the `Hidden` check:

```go
		// A column blank above its level sends no cell, currency or label on the
		// rows of outer levels and the Total (blankAbove): a layout's computed
		// rows (one level below the last group) keep theirs.
		if grouped && toInt(level) < int64(cp.blankLevel) {
			continue
		}
```

In the same function's `percents` loop, skip a percent column the same way: `if grouped && toInt(level) < int64(sp.col.blankLevel) { continue }`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'BlankAbove' -count=1`
Expected: PASS.

- [ ] **Step 6: Run the whole package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 7: Hand back**

Files changed: `core/server/reports/levels.go`, `assemble.go`, `blankabove_test.go`. No commit.

---

### Task 5: Builder — the column's Totals choice ("On every row" / "Up to <group>")

**Files:**
- Modify: `core/reports/lib/spec.ts` (`specColumnSchema`, 122-146)
- Modify: `core/reports/lib/builder/draft.ts` (`columnFields` 145-169, refusals near `rollupIssues` 728-751, `draftIssues` 774-795, `columnDraft` 939-959, `specColumns` 1125-1147)
- Modify: `core/reports/components/builder/ColumnsSection.tsx` (`ColumnRow` 99-177; new `ColumnBlankAbove` after `ColumnRollup` 268-317)
- Test: `core/reports/lib/builder/__tests__/draft.test.ts` (new `describe` block at the end; imports at 11-45)

**Interfaces:**
- Consumes: Task 3's JSON `blankAbove` and its refusals; `BuilderDraft`, `ColumnDraft`, `draftFromSpec`, `specFromDraft`, `builderSchema`, `nestDepth`, `PickMenu`/`PickItem` (`@tinycld/core/ui/pick-menu`), `FieldError`, test fixtures `CATALOG`, `SALES_KEY`, `META`.
- Produces:
  - `blankAbove: z.string().optional()` on `specColumnSchema` (so `SpecColumn.blankAbove?: string`)
  - `blankAbove: z.string().optional()` on the column draft (`ColumnDraft.blankAbove?: string`)
  - `export type BlankAboveDraft = Pick<BuilderDraft, 'groups' | 'columns' | 'pivot'>`
  - `export function offersBlankAbove(draft: BlankAboveDraft, index: number): boolean`
  - `export function blankAboveChoices(draft: BlankAboveDraft): { id: string; label: string }[]` — one per group, `label` = `Up to <label of the column the group groups by>` (the group id when that column is gone)
  - `export function withBlankAbove(columns: ColumnDraft[], index: number, group: string | undefined): ColumnDraft[]`
  - Component `ColumnBlankAbove`: PickMenu testID `report-builder-column-<i>-blank-above`, items `…-every` and `…-up-<group index>`; field error testID `report-builder-column-<i>-blank-above-error`.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/builder/__tests__/draft.test.ts`, and add `blankAboveChoices`, `offersBlankAbove`, `withBlankAbove` to the `../draft` import list:

```ts
// G2: a column totals only up to a group level; the outer levels and the Total
// leave it blank (spec.go Column.BlankAbove).
describe('a column totalled up to a group', () => {
    const UP_TO_TEAM: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [
            { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
            { id: 'status', label: 'Status', path: [{ field: 'status' }] },
            { id: 'team', label: 'Team', path: [{ link: 'team' }] },
            {
                id: 'amount',
                label: 'Amount',
                path: [{ field: 'amount' }],
                aggregate: 'sum',
                blankAbove: 'byTeam',
            },
        ],
        groups: [
            { id: 'byStatus', col: 'status' },
            { id: 'byTeam', col: 'team' },
        ],
    }
    const issuePaths = (draft: BuilderDraft) => {
        const parsed = builderSchema(CATALOG).safeParse(draft)
        return parsed.success ? [] : parsed.error.issues.map(issue => issue.path.join('.'))
    }

    it('reads blankAbove from a spec and writes it back; a spec without it stays without', () => {
        const draft = draftFromSpec(UP_TO_TEAM, META, CATALOG)
        expect(draft.columns[3]?.blankAbove).toBe('byTeam')
        expect(issuePaths(draft)).toEqual([])
        expect(specFromDraft(draft, CATALOG)).toEqual(UP_TO_TEAM)
        const amount = { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' as const }
        const without = { ...UP_TO_TEAM, columns: [...UP_TO_TEAM.columns.slice(0, 3), amount] }
        const saved = specFromDraft(draftFromSpec(without, META, CATALOG), CATALOG)
        expect(saved.columns[3]).not.toHaveProperty('blankAbove')
    })

    it('parses blankAbove on a spec column, and a column without it', () => {
        expect(specColumnSchema.parse(UP_TO_TEAM.columns[3]).blankAbove).toBe('byTeam')
        expect(specColumnSchema.parse(UP_TO_TEAM.columns[0])).not.toHaveProperty('blankAbove')
    })

    it('writes "Up to <group>" as the group id and "On every row" as no key', () => {
        const draft = draftFromSpec(UP_TO_TEAM, META, CATALOG)
        expect(blankAboveChoices(draft)).toEqual([
            { id: 'byStatus', label: 'Up to Status' },
            { id: 'byTeam', label: 'Up to Team' },
        ])
        const everyRow = { ...draft, columns: withBlankAbove(draft.columns, 3, undefined) }
        expect(specFromDraft(everyRow, CATALOG).columns[3]).not.toHaveProperty('blankAbove')
        const upToStatus = { ...draft, columns: withBlankAbove(draft.columns, 3, 'byStatus') }
        expect(specFromDraft(upToStatus, CATALOG).columns[3]?.blankAbove).toBe('byStatus')
    })

    it('is offered only on a grouped report, never with a rollup, a rows total or on a pivot cell', () => {
        const draft = draftFromSpec(UP_TO_TEAM, META, CATALOG)
        expect(offersBlankAbove(draft, 3)).toBe(true)
        expect(offersBlankAbove({ ...draft, groups: [] }, 3)).toBe(false)
        const set = (patch: Partial<ColumnDraft>) => ({
            ...draft,
            columns: draft.columns.map((c, i) => (i === 3 ? ({ ...c, ...patch } as ColumnDraft) : c)),
        })
        expect(offersBlankAbove(set({ total: 'rows' }), 3)).toBe(false)
        expect(
            offersBlankAbove(set({ rollup: { kind: 'editable', node: { value: 0 } } }), 3)
        ).toBe(false)
        expect(
            offersBlankAbove({ ...draft, pivot: { ...draft.pivot, on: true, cell: 'amount' } }, 3)
        ).toBe(false)
        // A column a group groups by shows the group on its total rows.
        expect(offersBlankAbove(draft, 2)).toBe(false)
    })

    it('refuses what the engine refuses, a removed group included (Review Focus 3)', () => {
        const draft = draftFromSpec(UP_TO_TEAM, META, CATALOG)
        expect(issuePaths({ ...draft, groups: draft.groups.slice(0, 1) })).toContain(
            'columns.3.blankAbove'
        )
        const rows = {
            ...draft,
            columns: draft.columns.map((c, i) => (i === 3 ? { ...c, total: 'rows' as const } : c)),
        }
        expect(issuePaths(rows)).toContain('columns.3.blankAbove')
    })
})
```

Add `specColumnSchema` to the `../../spec` import, and `ColumnDraft` / `BuilderDraft` / `ReportSpec` types where the file does not import them already (search the file's import lines first; use the names it uses).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run reports/lib/builder/__tests__/draft.test.ts -t 'a column totalled up to a group'`
Expected: FAIL: `blankAboveChoices is not a function` (no export) and the round trip drops `blankAbove` (strict schema: `Unrecognized key: "blankAbove"`). Record it.

- [ ] **Step 3: The spec schema**

In `core/reports/lib/spec.ts` `specColumnSchema`, after `heading`:

```ts
    // A group id: the column totals on that group's level and the levels inside it;
    // the outer levels' group and subtotal rows and the Total leave it blank.
    blankAbove: z.string().optional(),
```

- [ ] **Step 4: The draft**

In `core/reports/lib/builder/draft.ts`:

1. `columnFields`, after `hidden`:

```ts
    /** The group the column totals up to; absent: every row (G2). */
    blankAbove: z.string().optional(),
```

2. `columnDraft`'s `common`, after the `hidden` spread: `...(column.blankAbove ? { blankAbove: column.blankAbove } : {}),`
3. `specColumns`, after `if (column.hidden) out.hidden = true`: `if (column.blankAbove) out.blankAbove = column.blankAbove`
4. After `rollupIssues`, add:

```ts
export type BlankAboveDraft = Pick<BuilderDraft, 'groups' | 'columns' | 'pivot'>

const BLANK_ABOVE_GONE = 'This group level was removed: choose another, or On every row'
const BLANK_ABOVE_ROLLUP = 'A formula on group and total rows already replaces the totals'
const BLANK_ABOVE_ROWS = 'Totals that add the rows shown add every level: they cannot stop at a group'
const BLANK_ABOVE_PIVOT = "A pivot's cell column is totalled by its pivot columns"
const BLANK_ABOVE_NESTED = 'A child row column has no totals'
const BLANK_ABOVE_GROUPED = 'A column a group is grouped by shows the group on its total rows'

// bind.go blankAbove: a column with totals of its own, on a grouped report.
function blankAboveRefusal(draft: BlankAboveDraft, index: number) {
    const column = draft.columns[index]
    if (!column) return undefined
    if (column.rollup) return BLANK_ABOVE_ROLLUP
    if (column.total === 'rows') return BLANK_ABOVE_ROWS
    if (draft.pivot.on && draft.pivot.cell === column.id) return BLANK_ABOVE_PIVOT
    if (nestDepth(column) > 0) return BLANK_ABOVE_NESTED
    if (draft.groups.some(g => g.col === column.id)) return BLANK_ABOVE_GROUPED
    return undefined
}

/** "Totals" is offered on a grouped report, on a column whose totals can stop at a level. */
export function offersBlankAbove(draft: BlankAboveDraft, index: number): boolean {
    if (draft.groups.length === 0 || !draft.columns[index]) return false
    return blankAboveRefusal(draft, index) === undefined
}

/** One "Up to <group>" choice per group level, named by the column it groups by. */
export function blankAboveChoices(draft: BlankAboveDraft): { id: string; label: string }[] {
    const labels = new Map(draft.columns.map(column => [column.id, column.label]))
    return draft.groups.map(g => ({ id: g.id, label: `Up to ${labels.get(g.col) ?? g.id}` }))
}

/** The columns with column `index` totalled up to `group`, or on every row for undefined. */
export function withBlankAbove(
    columns: ColumnDraft[],
    index: number,
    group: string | undefined
): ColumnDraft[] {
    // An undefined blankAbove is "every row": specColumns writes the key only when set.
    return columns.map((column, i) => (i === index ? { ...column, blankAbove: group } : column))
}

function blankAboveIssues(draft: BuilderDraft): Issue[] {
    const groups = new Set(draft.groups.map(g => g.id))
    return draft.columns.flatMap((column, i): Issue[] => {
        if (!column.blankAbove) return []
        const path = ['columns', i, 'blankAbove']
        if (!groups.has(column.blankAbove)) return [{ path, message: BLANK_ABOVE_GONE }]
        const message = blankAboveRefusal(draft, i)
        return message ? [{ path, message }] : []
    })
}
```

5. In `draftIssues`, add `...blankAboveIssues(draft),` after `...rollupIssues(draft),`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS (the whole file).

- [ ] **Step 6: The builder control**

In `core/reports/components/builder/ColumnsSection.tsx`, import `blankAboveChoices`, `offersBlankAbove`, `withBlankAbove` and the `BlankAboveDraft` type from the draft module the file already imports from, and add after `ColumnRollup`:

```tsx
const EVERY_ROW = 'On every row'

// The Totals menu's items: every row, then one "Up to <group>" per level, by index
// (R11), the current choice selected.
function blankAboveItems(
    draft: BlankAboveDraft,
    current: string | undefined,
    choose: (group: string | undefined) => void
): PickItem[] {
    const levels = blankAboveChoices(draft).map(
        (choice, i): PickItem => ({
            id: `up-${i}`,
            label: choice.label,
            isSelected: current === choice.id,
            onSelect: () => choose(choice.id),
        })
    )
    const every: PickItem = {
        id: 'every',
        label: EVERY_ROW,
        isSelected: current === undefined,
        onSelect: () => choose(undefined),
    }
    return [every, ...levels]
}

/**
 * "Totals" (G2): the column's total on every row, or only up to a group level, so the
 * outer levels and the Total leave it blank (a figure they would count twice). Offered
 * on a grouped report, never with a formula on group and total rows, Add the rows
 * shown, a pivot's cell column or a column a group groups by; a choice set before
 * that stays in view, with its refusal, so it can be set back.
 */
function ColumnBlankAbove({ builder, columns, index }: Omit<ColumnPartProps, 'source'>) {
    const control = builder.form.control
    const groups = useWatch({ control, name: 'groups' })
    const pivot = useWatch({ control, name: 'pivot' })
    const column = columns[index]
    const draft = { groups, columns, pivot }
    if (!column) return null
    if (column.blankAbove === undefined && !offersBlankAbove(draft, index)) return null
    const testID = `report-builder-column-${index}-blank-above`
    const state = builder.form.getFieldState(`columns.${index}.blankAbove`, builder.form.formState)
    const choose = (group: string | undefined) =>
        builder.setColumns(withBlankAbove(columns, index, group))
    const items = blankAboveItems(draft, column.blankAbove, choose)
    const shown = items.find(item => item.isSelected)?.label ?? EVERY_ROW
    return (
        <View className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-xs text-muted-foreground">Totals</Text>
                <PickMenu
                    label={shown}
                    purpose={`Totals of ${column.label}`}
                    items={items}
                    testID={testID}
                />
            </View>
            <FieldError message={state.error?.message} testID={`${testID}-error`} />
        </View>
    )
}
```

In `ColumnRow`, after `<ColumnRollup … />`, add `<ColumnBlankAbove builder={builder} columns={columns} index={index} />`.

- [ ] **Step 7: Check types and lint**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core && PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check reports/lib/spec.ts reports/lib/builder/draft.ts reports/lib/builder/__tests__/draft.test.ts reports/components/builder/ColumnsSection.tsx`
Expected: no errors. Fix any at the source.

- [ ] **Step 8: Hand back**

Files changed: `core/reports/lib/spec.ts`, `core/reports/lib/builder/draft.ts`, `core/reports/lib/builder/__tests__/draft.test.ts`, `core/reports/components/builder/ColumnsSection.tsx`. No commit.

---

### Task 6: Docs — help and package docs

**Files:**
- Modify: `core/help/reports.md` (section `## Totals`, lines 49-64)
- Modify: `docs/packages.md` ("Report sources", after the code block ending line 1172; the "Fields added in version 4" table, lines 1421-1442)
- Test: `core/reports/lib/__tests__/statements-docs.test.ts`

**Interfaces:**
- Consumes: Tasks 1-5's names (`Source.Parts`, `SourcePart{Collection, Columns, Values}`, `columns[].blankAbove`, `unknown_group`, the builder's **Totals** / **On every row** / **Up to**).
- Produces: help text the e2e and users read; docs the Books checkpoints read.

- [ ] **Step 1: Write the failing tests**

Add to `core/reports/lib/__tests__/statements-docs.test.ts`, inside `describe('statement docs', …)`:

```ts
    it('names the Totals choice in the help', () => {
        const help = read('core/help/reports.md')
        expect(help).toContain('**Totals**')
        expect(help).toContain('**On every row**')
        expect(help).toContain('**Up to')
    })

    it('shows a union source in Report sources and blankAbove in the version 4 table', () => {
        const docs = read('docs/packages.md')
        const sources = docs.slice(docs.indexOf('### Report sources'))
        expect(sources).toContain('Parts: []reports.SourcePart{')
        expect(sources).toContain('Values:')
        const start = docs.indexOf('**Fields added in version 4.**')
        const table = docs.slice(start, docs.indexOf('\n\n', docs.indexOf('| Field |', start)))
        expect(table).toContain('`columns[].blankAbove`')
        expect(table).toContain('`unknown_group`')
    })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run reports/lib/__tests__/statements-docs.test.ts`
Expected: FAIL: `expected … to contain '**Totals**'` and `'Parts: []reports.SourcePart{'`. Record it.

- [ ] **Step 3: Write the help**

In `core/help/reports.md`, at the end of `## Totals` (after the paragraph that ends "…because the Total row is not inside a group."), add:

```markdown
To show a column's total only up to a group level, open the column in **Columns** and set **Totals** to **Up to** and the level, for example **Up to Team**. That level's rows, the levels inside it and the opened rows show the column. The group and total rows of the levels above it, and the Total row, leave the cell empty. Use it for a figure that the levels above would count twice: when one row counts under two groups of a level, the total of the level above adds it two times. **On every row** (the default) shows the column's total on every row. This choice shows only when the report has a group level. It does not show while **On group and total rows show** is **A formula** or **Total** is **Add the rows shown**, for a pivot's cell column, or for a column that a group level groups by.
```

- [ ] **Step 4: Write the package docs**

In `docs/packages.md`, after the `RegisterSource` code block of "### Report sources" (before the bullet "- Every link declares its cardinality."), add:

````markdown
**Union sources.** A source can read several collections of one shape as one
source: set `Parts` and leave `Collection` empty. Each part is a collection,
read under its own list rule; `Columns` maps a source key to that part's
column, and `Values` gives a field a constant on every row of the part. A key
a part does not map reads blank there.

```go
reports.RegisterSource(reports.Source{
    Key:   "zoo.meals",
    Label: "Meals",
    Parts: []reports.SourcePart{
        {Collection: "zoo_feedings", Columns: map[string]string{"cost": "cost", "currency": "currency", "animal": "animal"}, Values: map[string]any{"kind": "feeding"}},
        {Collection: "zoo_treats", Columns: map[string]string{"cost": "price", "currency": "currency", "animal": "animal"}, Values: map[string]any{"kind": "treat"}},
    },
    Fields: []reports.Field{
        {Key: "kind", Label: "Kind", Type: reports.Select, Options: []reports.Option{{Value: "feeding", Label: "Feeding"}, {Value: "treat", Label: "Treat"}}},
        {Key: "cost", Label: "Cost", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
        {Key: "currency", Label: "Currency", Type: reports.Text},
    },
    Links: []reports.Link{
        {Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: reports.ToOne, Field: "animal"},
    },
})
```

- A union has at least two parts, each its own collection with a list rule
  (`CheckSources` refuses a part whose list rule is superusers only). Its
  fields name no `Column`; every field and every to-one link's `Field` is
  mapped (or, for a field, given a value) by at least one part.
- Every part that maps a key maps it to a column of the same PocketBase type.
  A constant is text for a text field, an option's value for a select, a
  number for a number or money field, `true` or `false` for a yes/no field.
- A union reads like any source: its links, links to it (a to-one relation
  that points at one of its part collections; a to-many link whose
  `BackField` each part maps), record groups, labels, fill, param options,
  lookups (a sum across the parts is one source, one currency) and change
  topics (every part's collection). A row's id is its part record's id.
````

In the "Fields added in version 4" table, add a row after `columns[].hidden`:

```markdown
| `columns[].blankAbove` | The id of a group: the column totals on that group's level, the levels inside it and the detail rows; the group and subtotal rows of the levels above it, the layout sections when the group is not the first, and the Total leave the cell out (no `<col>@currency` cell either). A `{col}` read of it on those levels reads blank. For a figure the levels above would count twice. Refused on a pivot's cell column, with `rollup`, with `total: "rows"`, on a child-row column, on a column a group groups by, and for a group the report does not have (422 `unknown_group`, at `columns.<i>.blankAbove`). |
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run reports/lib/__tests__/statements-docs.test.ts`
Expected: PASS.

- [ ] **Step 6: Hand back**

Files changed: `core/help/reports.md`, `docs/packages.md`, `core/reports/lib/__tests__/statements-docs.test.ts`. No commit.

---

### Task 7: A fictional report of the Sales Tax Liability's shape, run on the merged engine

**Files:**
- Create: `core/server/reports/union_shape_test.go`

**Interfaces:**
- Consumes: everything of Tasks 1-4 (`SourcePart`, `Source.Parts`, `blankAbove`); `newZoo`, `z.rec`, `z.save`, `z.id`, `z.run`, `rule`, `requireOrderedRows`; sources `zoo.people`, `zoo.sites`, `zoo.animals` (zoo_test.go).
- Produces: `func addZooSales(t testing.TB, z *zoo)`, `func registerZooSalesSources()`, `TestATwoUnionReportTotalsOnlyTheUnblankedColumnAboveTheInnerLevel` (process rule: the last Go task runs the merged engine on the report's shape).

Shape (spec §5, fictional names): `zoo.lines` is a union of two line collections, each line linked to `zoo.documents`, a union of two document collections (part columns `order` / `ticket`); groups by two links, the outer (`bySite`, as the agency) open, the inner `byAnimal` (as the rate); three money columns with `blankAbove: "byAnimal"` and `tax` without; a detail label `concat` over document fields with an `if` on a missing customer; filter `document.status = "posted"` through a required link.

- [ ] **Step 1: Write the test (fixture and assertion)**

Create `core/server/reports/union_shape_test.go`:

```go
package reports

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional book of two unions: orders and tickets
// (two document collections), each with its lines (one line per document and
// rate), a line's site standing for the agency and its animal for the rate.
//
//	o1 order  2026-10-02 posted INV-1 (ann): north/leo 10000·10000·290, north/zed 10000·10000·100, south/kit 10000·8000·400
//	k1 ticket 2026-10-03 posted SR-1 (no customer): north/leo 2000·2000·58
//	o2 order  2026-10-05 posted INV-2 (ann): north/leo 5000·0·0 (nothing taxable)
//	o3 order  2026-10-06 void   INV-3 (ann): north/leo 9999·9999·999 (drops out)
//
// (gross · taxable · tax, in cents). North's two rates both carry o1, so its
// sales summed above the rate would count o1 twice.
func addZooSales(t testing.TB, z *zoo) {
	t.Helper()
	find := func(name string) *core.Collection {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return col
	}
	users, sites, animals := find("users"), find("zoo_sites"), find("zoo_animals")
	signedIn := rule(`@request.auth.id != ""`)
	document := func(name string) *core.Collection {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.TextField{Name: "day"},
			&core.SelectField{Name: "status", Values: []string{"posted", "void"}, MaxSelect: 1},
			&core.TextField{Name: "number"},
			&core.NumberField{Name: "sequence"},
			&core.NumberField{Name: "subtotal"},
			&core.TextField{Name: "currency"},
			&core.RelationField{Name: "customer", CollectionId: users.Id, MaxSelect: 1},
		)
		z.save(t, col)
		return col
	}
	line := func(name, docField string, doc *core.Collection) {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.RelationField{Name: docField, CollectionId: doc.Id, MaxSelect: 1},
			&core.RelationField{Name: "site", CollectionId: sites.Id, MaxSelect: 1},
			&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
			&core.NumberField{Name: "gross"},
			&core.NumberField{Name: "taxable"},
			&core.NumberField{Name: "amount"},
			&core.TextField{Name: "currency"},
		)
		z.save(t, col)
	}
	line("zoo_order_lines", "order", document("zoo_orders"))
	line("zoo_ticket_lines", "ticket", document("zoo_tickets"))

	doc := func(key, collection, day, status, number string, seq int, customer string) {
		z.rec(t, key, collection, map[string]any{"day": day, "status": status, "number": number, "sequence": seq, "currency": "USD", "customer": customer})
	}
	ann := z.id("ann")
	doc("o1", "zoo_orders", "2026-10-02", "posted", "INV-1", 1, ann)
	doc("o2", "zoo_orders", "2026-10-05", "posted", "INV-2", 2, ann)
	doc("o3", "zoo_orders", "2026-10-06", "void", "INV-3", 3, ann)
	doc("k1", "zoo_tickets", "2026-10-03", "posted", "SR-1", 1, "")
	ln := func(collection, docField, docKey, site, animal string, gross, taxable, amount int) {
		z.rec(t, docKey+site+animal, collection, map[string]any{
			docField: z.id(docKey), "site": z.id(site), "animal": z.id(animal),
			"gross": gross, "taxable": taxable, "amount": amount, "currency": "USD",
		})
	}
	ln("zoo_order_lines", "order", "o1", "north", "leo", 10000, 10000, 290)
	ln("zoo_order_lines", "order", "o1", "north", "zed", 10000, 10000, 100)
	ln("zoo_order_lines", "order", "o1", "south", "kit", 10000, 8000, 400)
	ln("zoo_ticket_lines", "ticket", "k1", "north", "leo", 2000, 2000, 58)
	ln("zoo_order_lines", "order", "o2", "north", "leo", 5000, 0, 0)
	ln("zoo_order_lines", "order", "o3", "north", "leo", 9999, 9999, 999)
}

func registerZooSalesSources() {
	docColumns := map[string]string{"day": "day", "status": "status", "number": "number", "sequence": "sequence", "subtotal": "subtotal", "currency": "currency", "customer": "customer"}
	RegisterSource(Source{
		Key: "zoo.documents", Label: "Documents",
		Parts: []SourcePart{
			{Collection: "zoo_orders", Columns: docColumns, Values: map[string]any{"kind": "order"}},
			{Collection: "zoo_tickets", Columns: docColumns, Values: map[string]any{"kind": "ticket"}},
		},
		Fields: []Field{
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "order", Label: "Order"}, {Value: "ticket", Label: "Ticket"}}},
			{Key: "day", Label: "Day", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "status", Label: "Status", Type: Select, Options: []Option{{Value: "posted", Label: "Posted"}, {Value: "void", Label: "Void"}}},
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "sequence", Label: "Sequence", Type: Number, Aggregates: MinMax},
			{Key: "subtotal", Label: "Subtotal", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
		},
		Links: []Link{{Key: "customer", Label: "Customer", Target: "zoo.people", Cardinality: ToOne, Field: "customer"}},
	})
	lineColumns := func(docField string) map[string]string {
		return map[string]string{"document": docField, "site": "site", "animal": "animal", "gross": "gross", "taxable": "taxable", "amount": "amount", "currency": "currency"}
	}
	RegisterSource(Source{
		Key: "zoo.lines", Label: "Lines",
		Parts: []SourcePart{
			{Collection: "zoo_order_lines", Columns: lineColumns("order")},
			{Collection: "zoo_ticket_lines", Columns: lineColumns("ticket")},
		},
		Fields: []Field{
			{Key: "gross", Label: "Gross", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "taxable", Label: "Taxable", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "amount", Label: "Tax", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
		},
		Links: []Link{
			{Key: "document", Label: "Document", Target: "zoo.documents", Cardinality: ToOne, Field: "document"},
			{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"},
			{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"},
		},
	})
}

const salesTaxShape = `{"version":4,"source":"zoo.lines",
	"columns":[
		{"id":"name","path":[{"link":"document"},{"field":"number"}]},
		{"id":"siteKey","path":[{"link":"site"}],"hidden":true},
		{"id":"animalKey","path":[{"link":"animal"}],"hidden":true},
		{"id":"gross","path":[{"field":"gross"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"nontaxable","formula":{"op":"sub","args":[{"col":"gross"},{"col":"taxable"}]},"blankAbove":"byAnimal"},
		{"id":"taxable","path":[{"field":"taxable"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"tax","path":[{"field":"amount"}],"aggregate":"sum"}],
	"filter":{"op":"eq","args":[{"path":[{"link":"document","with":"required"},{"field":"status"}]},{"value":"posted"}]},
	"groups":[{"id":"bySite","col":"siteKey","open":true,"subtotals":true},{"id":"byAnimal","col":"animalKey"}],
	"detail":{"label":{"op":"concat","args":[
		{"path":[{"link":"document"},{"field":"day"}]},
		{"value":" · "},
		{"op":"if","args":[{"op":"eq","args":[{"path":[{"link":"document"},{"field":"kind"}]},{"value":"order"}]},{"value":"Order"},{"value":"Ticket"}]},
		{"value":" "},
		{"path":[{"link":"document"},{"field":"number"}]},
		{"op":"if","args":[
			{"op":"isEmpty","args":[{"path":[{"link":"document"},{"link":"customer"},{"field":"name"}]}]},
			{"value":""},
			{"op":"concat","args":[{"value":" · "},{"path":[{"link":"document"},{"link":"customer"},{"field":"name"}]}]}]}]}},
	"sort":[{"formula":{"path":[{"link":"document"},{"field":"day"}]}},{"formula":{"path":[{"link":"document"},{"field":"sequence"}]}}]}`

func TestATwoUnionReportTotalsOnlyTheUnblankedColumnAboveTheInnerLevel(t *testing.T) {
	z := newZoo(t)
	addZooSales(t, z)
	registerZooSalesSources()
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	res := z.run(t, "ann", salesTaxShape, Options{ExpandAll: true}, nil)

	money := []string{"gross", "nontaxable", "taxable", "tax"}
	figures := func(r Row) string {
		out := make([]any, len(money))
		for i, id := range money {
			out[i] = r.Cells[id]
		}
		return fmt.Sprint(out)
	}
	var rates, sites []string
	var underLeo []string
	innerTax, allInnerTax := int64(0), int64(0)
	for _, r := range res.Rows {
		switch {
		case r.Kind == KindGroup && r.Level == 2:
			rates = append(rates, r.Label+" "+figures(r))
			innerTax += r.Cells["tax"].(int64)
		case r.Kind == KindDetail && len(rates) == 1:
			underLeo = append(underLeo, r.Label)
		case r.Kind == KindSubtotal || r.Kind == KindTotal:
			// Above the rate only the tax shows, and it adds the rate rows above it.
			for _, id := range []string{"gross", "nontaxable", "taxable"} {
				requireNoCell(t, r, id)
			}
			want := innerTax
			if r.Kind == KindTotal {
				want = allInnerTax
			} else {
				allInnerTax += innerTax
				innerTax = 0
			}
			if r.Cells["tax"] != want {
				t.Fatalf("%s tax = %v, want the rate rows' %d", r.Label, r.Cells["tax"], want)
			}
			sites = append(sites, r.Label+" "+fmt.Sprint(r.Cells["tax"]))
		}
	}
	wantRates := []string{
		"Leo [17000 5000 12000 348]",
		"Zed [10000 0 10000 100]",
		"Kit [10000 2000 8000 400]",
	}
	if fmt.Sprint(rates) != fmt.Sprint(wantRates) {
		t.Fatalf("rate rows = %v\nwant      %v", rates, wantRates)
	}
	if fmt.Sprint(sites) != fmt.Sprint([]string{"Total North 448", "Total South 400", "Total 848"}) {
		t.Fatalf("totals = %v", sites)
	}
	wantLeo := []string{"2026-10-02 · Order INV-1 · ann", "2026-10-03 · Ticket SR-1", "2026-10-05 · Order INV-2 · ann"}
	if fmt.Sprint(underLeo) != fmt.Sprint(wantLeo) {
		t.Fatalf("Leo's documents = %v\nwant              %v", underLeo, wantLeo)
	}
}
```

Notes for the implementer: `requireNoCell` is defined in `blankabove_test.go` (Task 4). A closed or open level-2 group row without subtotals keeps its figures (no subtotal rows on `byAnimal`). If a row label or a cell type differs from the expectation (for example the record group's label, or an `int64` vs `float64` cell), read the run's rows (`rowsOf(res, "label")`) and fix the engine if it contradicts spec §3/§5 (an agency total with a sales figure, an `@currency` cell above the rate, a document counted twice); change an expectation only where it was wrong about the fixture's own numbers, and say so in the hand-back.

- [ ] **Step 2: Run the test**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go test ./reports/ -run 'TestATwoUnionReport' -count=1`
Expected: PASS on the engine of Tasks 1-4. This task's red proof: run the same test with Task 4's `assemble.go` change taken out in a scratch overlay (`go test -overlay`, scratch copy under the session scratchpad, never editing repo source) and record the FAIL `row s:... carries gross`; then run it as is (PASS). If it fails on the real engine, the failure is a core defect: fix it at its source in this task, with a test of its own in the file of the task whose code it is.

- [ ] **Step 3: Run the whole package**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld/core/server && go vet ./reports/ && go test ./reports/ -count=1`
Expected: PASS.

- [ ] **Step 4: Hand back**

Files changed: `core/server/reports/union_shape_test.go` (and any engine fix, named). No commit.

---

### Task 8: E2E — a union source and blank above in the view and the builder (desktop and phone)

**Files:**
- Modify: `tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt` (new collection after `salesstub_sales`, line 59; down, 61-64)
- Modify: `tests/scripts/reports-stub/server/register.go.txt` (new source after `salesstub.teams`, line 53)
- Modify: `tests/scripts/reports-stub/screens/index.tsx.txt` (schema near line 36, new `ReturnForm` after `ManySalesForm` line 328, `ReportStubIndex` 330-341)
- Modify: `tests/scripts/scaffold-reports-stub.ts` (`writeSources`, 129-158; header comment 9-14)
- Modify: `tests/e2e/reports-helpers.ts` (constants near line 24; `addReturn` after `addSales`, line 121)
- Create: `tests/e2e/reports-union-blank-above.spec.ts`

**Interfaces:**
- Consumes: Tasks 1-5 (union source, `blankAbove`, the builder's Totals menu `report-builder-column-<i>-blank-above` with items `-every` / `-up-<group index>`); helpers `createInvitedUser`, `skipWithoutReportsStub` (`tests/e2e/helpers.ts`), `openReportStub`, `addTeam`, `addSale`, `openReportsHub`, `startReport`, `addColumn`, `chooseOption`, `saveNewReport`, `uniqueName`, `cellOf` (reports-helpers / report-view-helpers).
- Produces: collection `salesstub_returns` (owner, team, customer, amount, currency, day); source `salesstub.activity` (union of `salesstub_sales` and `salesstub_returns`: kind Sale/Return, customer, amount, currency, day; link team); screen test ids `salesstub-return-team`, `-customer`, `-amount`, `-day`, `-create`, `-saved`; helpers `export const ACTIVITY_SOURCE = 'salesstub.activity'`, `export interface StubReturn { team: string; customer: string; amount: number; day: string }`, `export async function addReturn(page: Page, ret: StubReturn)`.

- [ ] **Step 1: Write the failing e2e spec**

Create `tests/e2e/reports-union-blank-above.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import { cellOf } from './report-view-helpers'
import {
    ACTIVITY_SOURCE,
    addColumn,
    addReturn,
    addSale,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The stub's union source (salesstub.activity: sales and returns, each with its
 * kind) and a column totalled up to a group (blankAbove). A fresh invited user owns
 * the rows, so the report holds only this test's sales and returns (R14).
 */
type Shot = { id: string; amount: string }

// Every drawn row's id and amount cell, in one snapshot ('' where the cell is empty or
// absent).
function amounts(page: Page): Promise<Shot[]> {
    return page.getByTestId('report-table').evaluate(table =>
        [...table.querySelectorAll('[data-testid^="report-row-"]')].map(row => {
            const id = row.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
            const cell = table.querySelector(`[data-testid="report-cell-amount-${CSS.escape(id)}"]`)
            return { id, amount: cell?.textContent?.trim() ?? '' }
        })
    )
}

const RECORD_ID = /^[a-z0-9]{15}$/

// A subtotal row's key holds its groups' keys then its currency: a team's total holds
// the team's record id, a kind's total does not.
function totals(shots: Shot[]) {
    const subtotals = shots.filter(s => s.id.startsWith('s:'))
    const isTeam = (s: Shot) =>
        (JSON.parse(s.id.slice(2)) as unknown[]).some(v => typeof v === 'string' && RECORD_ID.test(v))
    return {
        team: subtotals.filter(isTeam).map(s => s.amount.replace('$', '')).sort(),
        kind: subtotals.filter(s => !isTeam(s)).map(s => s.amount),
        total: shots.filter(s => s.id.startsWith('total')).map(s => s.amount),
    }
}

test.describe('reports: union source and blank above', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('sales and returns in one report; a total up to the team leaves the kinds and the Total blank, also on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptunion')
        try {
            const east = uniqueName('East')
            const west = uniqueName('West')
            const sold = uniqueName('Sold')
            const soldWest = uniqueName('Sold west')
            const returned = uniqueName('Returned')
            await openReportStub(p)
            await addTeam(p, east)
            await addTeam(p, west, 'south')
            await addSale(p, { team: east, customer: sold, amount: 1000, cost: 0, day: '2026-07-02' })
            await addSale(p, { team: west, customer: soldWest, amount: 400, cost: 0, day: '2026-07-02' })
            await addReturn(p, { team: west, customer: returned, amount: 250, day: '2026-07-03' })

            // Columns customer (0), kind (1), team (2), amount summed (3); grouped by
            // kind, then team, both open, with subtotals.
            await openReportsHub(p)
            await startReport(p, ACTIVITY_SOURCE)
            await addColumn(p, ['customer'])
            await addColumn(p, ['kind'])
            await p.getByTestId('report-builder-add-column').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-record-team')
                .click()
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'kind')
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-1-col', 'team')
            await p.getByTestId('report-builder-group-1-open').click()

            // One report lists the rows of both collections, each with its kind.
            await expect(await cellOf(p, 'kind', sold)).toHaveText('Sale')
            await expect(await cellOf(p, 'kind', soldWest)).toHaveText('Sale')
            await expect(await cellOf(p, 'kind', returned)).toHaveText('Return')
            await expect.poll(async () => totals(await amounts(p)).kind.every(a => a !== '')).toBe(true)

            // Totals up to the team: the team totals keep the figure; the kinds' totals
            // and the Total show none.
            await chooseOption(p, 'report-builder-column-3-blank-above', 'up-1')
            await expect(p.getByTestId('report-builder-column-3-blank-above')).toHaveText(/Up to Team/)
            await expect.poll(async () => totals(await amounts(p))).toEqual({
                team: ['10.00', '2.50', '4.00'],
                kind: ['', ''],
                total: [''],
            })

            await saveNewReport(p, uniqueName('Union up to team'))
            await p.setViewportSize({ width: 390, height: 844 })
            const table = p.getByTestId('report-table')
            await expect
                .poll(() => table.evaluate(el => el.scrollWidth - el.clientWidth))
                .toBe(0)
            await expect
                .poll(() => p.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth))
                .toBe(0)
            await expect.poll(async () => {
                const t = totals(await amounts(p))
                return [...t.kind, ...t.total]
            }).toEqual(['', '', ''])
        } finally {
            await close()
        }
    })
})
```

Notes: the `report-builder-group-<i>-open` and `-col` test ids are the builder's (`GroupSortSection.tsx`); confirm each id against the component before the first run and correct the spec, not the component, where a name differs. If the kind group label or a cell writes the select's stored value (`sale`) where the source's option label (`Sale`) belongs, that is a core defect: fix it in core.

- [ ] **Step 2: Run the spec to verify it fails**

Check the ports, then run:
`cd /Users/nas/tmp/tinycld/books-reports/tinycld && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7220|7221) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e -- reports-union-blank-above.spec.ts`
Expected: FAIL at the first `addReturn` (`salesstub-return-team` not found) — the stub has no returns yet. If the run takes more than about 4 minutes, hand back `NEEDS-RUN: <that command>` with "the red reason of reports-union-blank-above.spec.ts".

- [ ] **Step 3: The returns collection**

In `tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt`, after `app.save(sales)`:

```js
        const returns = new Collection({
            id: 'pbc_salesstub_returns',
            name: 'salesstub_returns',
            type: 'base',
            system: false,
            listRule: OWN,
            viewRule: OWN,
            createRule: OWN,
            updateRule: OWN,
            deleteRule: OWN,
            fields: [
                { id: 'salesstub_returns_owner', name: 'owner', type: 'relation', collectionId: users.id, required: true, maxSelect: 1, cascadeDelete: false },
                { id: 'salesstub_returns_team', name: 'team', type: 'relation', collectionId: teams.id, maxSelect: 1, cascadeDelete: false },
                { id: 'salesstub_returns_customer', name: 'customer', type: 'text', max: 200 },
                { id: 'salesstub_returns_amount', name: 'amount', type: 'number', onlyInt: true },
                { id: 'salesstub_returns_currency', name: 'currency', type: 'text', max: 3 },
                { id: 'salesstub_returns_day', name: 'day', type: 'date' },
                ...stamps('salesstub_returns'),
            ],
        })
        app.save(returns)
```

In the down function, first line: `app.delete(app.findCollectionByNameOrId('salesstub_returns'))`. Update the header comment: "a sale or a return belongs to its owner alone".

- [ ] **Step 4: The union source**

In `tests/scripts/reports-stub/server/register.go.txt`, after the `salesstub.teams` `RegisterSource` call:

```go
	// Sales and returns as one source (a union, U1): each row keeps its kind.
	activityColumns := map[string]string{"customer": "customer", "amount": "amount", "currency": "currency", "day": "day", "team": "team"}
	reports.RegisterSource(reports.Source{
		Key: "salesstub.activity", Label: "Stub activity",
		Parts: []reports.SourcePart{
			{Collection: "salesstub_sales", Columns: activityColumns, Values: map[string]any{"kind": "sale"}},
			{Collection: "salesstub_returns", Columns: activityColumns, Values: map[string]any{"kind": "return"}},
		},
		Fields: []reports.Field{
			{Key: "customer", Label: "Customer", Type: reports.Text, Aggregates: reports.CountOnly},
			{Key: "kind", Label: "Kind", Type: reports.Select, Options: []reports.Option{
				{Value: "sale", Label: "Sale"}, {Value: "return", Label: "Return"},
			}},
			{Key: "amount", Label: "Amount", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: reports.Text},
			{Key: "day", Label: "Day", Type: reports.Date, Aggregates: reports.MinMax},
		},
		Links: []reports.Link{
			{Key: "team", Label: "Team", Target: "salesstub.teams", Cardinality: reports.ToOne, Field: "team"},
		},
		Heading: stubHeading,
	})
```

Update the package comment: "three report sources … one of them a union of two collections".

- [ ] **Step 5: The return form**

In `tests/scripts/reports-stub/screens/index.tsx.txt`, after `type ManySalesValues …`:

```tsx
const returnSchema = z.object({
    team: z.string().min(1, 'Choose a team'),
    customer: z.string().trim().min(1).max(200),
    amount: z.number().int(),
    day: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Enter a date as YYYY-MM-DD'),
})
type ReturnValues = z.infer<typeof returnSchema>
```

After `ManySalesForm`:

```tsx
function ReturnForm() {
    const { user } = useAuth()
    const [returns] = useStore('salesstub_returns')
    const [saved, setSaved] = useState('')
    const teamOptions = useTeamOptions()
    const form = useForm<ReturnValues>({
        resolver: zodResolver(returnSchema),
        mode: 'onChange',
        defaultValues: { team: '', customer: '', amount: 0, day: '' },
    })
    const create = useMutation({
        mutationFn: function* (values: ReturnValues) {
            yield returns.insert({
                id: newRecordId(),
                owner: user.id,
                team: values.team,
                customer: values.customer,
                amount: values.amount,
                currency: 'USD',
                day: `${values.day} 00:00:00.000Z`,
            })
        },
        onSuccess: (_data, values) => {
            setSaved(`Saved ${values.customer}`)
            form.reset({ ...values, customer: '', amount: 0 })
        },
    })
    const onCreate = () => {
        void form.handleSubmit(values => create.mutate(values))()
    }
    return (
        <View className="gap-2">
            <Text className="text-base font-semibold text-foreground">New return</Text>
            <SelectInput
                name="team"
                control={form.control}
                label="Team"
                options={teamOptions}
                testID="salesstub-return-team"
            />
            <TextInput
                name="customer"
                control={form.control}
                label="Customer"
                testID="salesstub-return-customer"
            />
            <NumberInput
                name="amount"
                control={form.control}
                label="Amount (cents)"
                testID="salesstub-return-amount"
            />
            <TextInput
                name="day"
                control={form.control}
                label="Day"
                placeholder="YYYY-MM-DD"
                testID="salesstub-return-day"
            />
            <Button onPress={onCreate} isDisabled={create.isPending} testID="salesstub-return-create">
                <ButtonText>Add return</ButtonText>
            </Button>
            <SavedNote text={saved} testID="salesstub-return-saved" />
        </View>
    )
}
```

In `ReportStubIndex`, after `<ManySalesForm />`, add `<ReturnForm />`. Update the screen's top comment: "adds teams, sales and returns through the UI".

- [ ] **Step 6: The stub's collections and the helpers**

In `tests/scripts/scaffold-reports-stub.ts` `writeSources`: in `collections.ts` add

```ts
    const salesstub_returns = newCollection('salesstub_returns', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    return { salesstub_teams, salesstub_sales, salesstub_returns }
```

and in `types.ts` `Pick<Schema, 'salesstub_teams' | 'salesstub_sales' | 'salesstub_returns'>`. In the header comment, "registers three sources … one a union of sales and returns".

In `tests/e2e/reports-helpers.ts`, after `TEAMS_SOURCE`: `export const ACTIVITY_SOURCE = 'salesstub.activity'`; after `addSales`:

```ts
export interface StubReturn {
    team: string
    customer: string
    /** Cents. */
    amount: number
    day: string
}

export async function addReturn(page: Page, ret: StubReturn) {
    await page.getByTestId('salesstub-return-team').click()
    await page.getByRole('menuitem', { name: ret.team, exact: true }).click()
    await page.getByTestId('salesstub-return-customer').fill(ret.customer)
    await page.getByTestId('salesstub-return-amount').fill(String(ret.amount))
    await page.getByTestId('salesstub-return-day').fill(ret.day)
    await page.getByTestId('salesstub-return-create').click()
    await expect(page.getByTestId('salesstub-return-saved')).toHaveText(`Saved ${ret.customer}`)
}
```

Then regenerate (nothing else may be running e2e): `cd /Users/nas/tmp/tinycld/books-reports && pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts` (writes the stub from the templates and runs `packages:generate`, which regenerates `pbSchema.ts` with `salesstub_returns`).

- [ ] **Step 7: Run the spec to verify it passes**

Check the ports, then:
`cd /Users/nas/tmp/tinycld/books-reports/tinycld && E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e -- reports-union-blank-above.spec.ts`
Expected: PASS. A failure is diagnosed from the run's output and `tinycld/tmp/e2e-serve-7220.log` (search it with `command grep -E`), and fixed at its source; no timeout bumps, no retries. Hand back `NEEDS-RUN` for a run over about 4 minutes.

- [ ] **Step 8: Lint and the stub's unit test**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check tests/e2e/reports-union-blank-above.spec.ts tests/e2e/reports-helpers.ts tests/scripts/scaffold-reports-stub.ts && /Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run tests/unit/scaffold-reports-stub.test.ts`
Expected: no errors, PASS.

- [ ] **Step 9: Hand back**

Files changed: the six files above. No commit.

---

## Self-review

- Spec coverage: U1 (Parts, Columns, Values, NULL for unmapped, per-part list rule, UNION ALL projected to keys, joins, links to and from, record groups, labels, fill, param options, lookups, topics, CheckSources per part with types and list rule) → Tasks 1-2; catalog → Task 1; G2 (spec field, the cell on the group level and inside, blank on outer group/subtotal rows, layout sections and Total, no `@currency`; refusals incl. 422 `unknown_group`) → Tasks 3-4; builder (Q10) → Task 5; help and packages.md → Task 6; the last Go task of the shape of spec §5 → Task 7; e2e on `salesstub` (desktop and phone) → Task 8.
- Every "Tests it must add" item has a test in a task: U1 list → Task 1 (`TestCheckSourcesRefusesABrokenUnion`, link/catalog tests) and Task 2 (rows, NULL, Values, own rule, links both ways, record group with label, fill, options, lookup sum, topics); G2 list → Tasks 3-4; last task → Task 7; Vitest → Tasks 5-6; Docs → Task 6; E2E → Task 8.
- Names are those of the brief's Produces; no rename.
