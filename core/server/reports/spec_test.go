package reports

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// Every unknown reference is one shape: a 422 naming the part and a code.
func TestUnknownRefIsA422AtThePart(t *testing.T) {
	err := unknownRef("sort.0.col", "unknown_column", `the report has no column "b"`)
	requireSpecError(t, err, 422, "sort.0.col", `no column "b"`)
	if code := err.Problems["sort.0.col"].Code(); code != "unknown_column" || len(err.Problems) != 1 {
		t.Fatalf("problems = %v", err.Problems)
	}
}

// withZooRegistry registers the zoo's sources and returns an app to bind
// specs for. It has no database: nothing checked it, so it refuses nothing.
func withZooRegistry(t *testing.T) core.App {
	t.Helper()
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	registerZooSources()
	return bareApp(t)
}

// bareApp is an app that is never bootstrapped. bind reads only its store,
// where CheckSources leaves the app's refusals.
func bareApp(t *testing.T) core.App {
	t.Helper()
	return core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
}

func requireSpecError(t *testing.T, err error, status int, at, contains string) {
	t.Helper()
	var se *SpecError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *SpecError", err)
	}
	if se.Status != status {
		t.Fatalf("status = %d, want %d (%v)", se.Status, status, se)
	}
	problem, ok := se.Problems[at]
	if !ok {
		t.Fatalf("no problem at %q: %v", at, se)
	}
	if !strings.Contains(problem.Error(), contains) {
		t.Fatalf("problem at %q = %q, want it to contain %q", at, problem.Error(), contains)
	}
}

const designExample = `{
  "version": 1,
  "source": "zoo.feedings",
  "params": [ { "id": "period", "type": "dateRange", "default": { "preset": "thisFiscalYear" } } ],
  "columns": [
    { "id": "site", "path": [{ "link": "animal" }, { "link": "site" }, { "field": "name" }], "label": "Site" },
    { "id": "total", "path": [{ "field": "cost" }], "aggregate": "sum" },
    { "id": "keepers", "path": [{ "link": "animal" }, { "link": "keepers", "mode": "aggregate" }, { "field": "id" }], "aggregate": "count" },
    { "id": "grams", "path": [{ "field": "grams" }], "aggregate": "sum" },
    { "id": "perGram", "formula": { "op": "div", "args": [ { "col": "total" }, { "col": "grams" } ], "scale": 0 } }
  ],
  "filter": { "op": "and", "args": [
    { "op": "between", "args": [ { "path": [{ "field": "fed" }] }, { "param": "period" } ] },
    { "op": "exists", "link": [{ "link": "animal" }, { "link": "keepers" }],
      "where": { "op": "eq", "args": [ { "path": [{ "link": "user" }, { "field": "name" }] }, { "value": "ann" } ] } }
  ] },
  "sort": [ { "col": "total", "dir": "desc" } ],
  "groups": [ { "id": "bySite", "col": "site", "subtotals": true } ],
  "pivot": { "path": [{ "field": "fed" }], "bucket": "month", "cell": "total", "maxColumns": 50 },
  "layout": null
}`

func TestLoadReadsTheDesignExample(t *testing.T) {
	withZooRegistry(t)
	spec, info, err := Load([]byte(designExample))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Source != "zoo.feedings" || len(spec.Columns) != 5 || spec.Pivot.Cell != "total" {
		t.Fatalf("spec = %+v", spec)
	}
	if string(spec.Filter.Args[1].Where.Args[1].Value) != `"ann"` {
		t.Fatalf("value = %s", spec.Filter.Args[1].Where.Args[1].Value)
	}
	if info.UpgradedFrom != 0 || len(info.Aliased) != 0 {
		t.Fatalf("info = %+v", info)
	}
}

func TestLoadRefusesWhatIsNotASpec(t *testing.T) {
	withZooRegistry(t)
	cases := map[string]struct {
		raw, at, want string
	}{
		"not json":        {`[1]`, "spec", "not a single JSON object"},
		"unknown key":     {`{"version":1,"source":"zoo.feedings","columns":[],"sql":"DROP"}`, "spec", "unknown field"},
		"trailing data":   {`{"version":1,"source":"zoo.feedings","columns":[]} {}`, "spec", "not a single JSON object"},
		"no version":      {`{"source":"zoo.feedings","columns":[]}`, "version", "whole number"},
		"version zero":    {`{"version":0,"columns":[]}`, "version", "whole number"},
		"version 1.5":     {`{"version":1.5,"columns":[]}`, "version", "whole number"},
		"newer version":   {`{"version":2,"columns":[]}`, "version", "newer than this server reads (1)"},
		"wrong leaf type": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":"cost"}]}`, "spec", "cannot unmarshal"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := Load([]byte(c.raw))
			requireSpecError(t, err, 400, c.at, c.want)
		})
	}
}

func TestUpgradeRunsEveryStepInOrder(t *testing.T) {
	steps := map[int]upgradeStep{
		1: func(doc map[string]any) error {
			doc["columns"] = doc["cols"]
			delete(doc, "cols")
			return nil
		},
		2: func(doc map[string]any) error {
			doc["source"] = "zoo." + doc["source"].(string)
			return nil
		},
	}
	out, from, err := upgrade([]byte(`{"version":1,"source":"feedings","cols":[]}`), steps, 3)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"version": float64(3), "source": "zoo.feedings", "columns": []any{}}
	if from != 1 || !reflect.DeepEqual(doc, want) {
		t.Fatalf("from %d, doc %v", from, doc)
	}
	if _, _, err := upgrade([]byte(`{"version":1}`), map[int]upgradeStep{}, 2); err == nil {
		t.Fatal("a missing step must fail")
	}
}

func TestLoadResolvesAliasesOnEveryPath(t *testing.T) {
	withZooRegistry(t)
	raw := `{"version":1,"source":"zoo.feedings",
		"columns":[{"id":"w","path":[{"link":"animal"},{"field":"mass"}],"aggregate":"sum"},
		           {"id":"f","formula":{"op":"mul","args":[{"path":[{"link":"animal"},{"field":"mass"}]},{"value":2}]}}],
		"filter":{"op":"exists","link":[{"link":"animal"},{"link":"site"},{"link":"animals"}],
		          "where":{"op":"gt","args":[{"path":[{"field":"mass"}]},{"value":10}]}}}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Columns[0].Path[1].Field != "weight" || spec.Columns[1].Formula.Args[0].Path[1].Field != "weight" {
		t.Fatalf("columns not resolved: %+v", spec.Columns)
	}
	if spec.Filter.Where.Args[0].Path[0].Field != "weight" {
		t.Fatalf("exists where not resolved: %+v", spec.Filter.Where.Args[0].Path)
	}
	want := Alias{Source: "zoo.animals", From: "mass", To: "weight"}
	if len(info.Aliased) != 3 || info.Aliased[0] != want {
		t.Fatalf("aliased = %+v", info.Aliased)
	}
}

func TestLoadKeepsLargeIntegersAcrossAnUpgrade(t *testing.T) {
	withZooRegistry(t)
	// CurrentVersion is 1, so a stand-in step is run through loadWith, which
	// Load calls with upgradeSteps and CurrentVersion.
	steps := map[int]upgradeStep{1: func(doc map[string]any) error { return nil }}
	raw := `{"version":1,"source":"zoo.feedings","columns":[],
		"filter":{"op":"eq","args":[{"path":[{"field":"id"}]},{"value":9007199254740993}]}}`
	spec, info, err := loadWith([]byte(raw), steps, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(spec.Filter.Args[1].Value); got != "9007199254740993" {
		t.Fatalf("value = %s, want 9007199254740993", got)
	}
	if info.UpgradedFrom != 1 {
		t.Fatalf("UpgradedFrom = %d, want 1", info.UpgradedFrom)
	}
}

func TestAliasPathLeavesKeysAloneBehindAnUnknownLink(t *testing.T) {
	withZooRegistry(t)
	base := &Source{
		Key:     "zoo.test",
		Links:   []Link{{Key: "gone", Target: "zoo.nowhere"}},
		Aliases: map[string]string{"mass": "weight"},
	}
	var out []Alias
	path := Path{{Link: "gone"}, {Field: "mass"}}
	if got := aliasPath(base, path, &out); got != nil {
		t.Fatalf("aliasPath = %v, want nil", got)
	}
	if len(out) != 0 || path[1].Field != "mass" {
		t.Fatalf("aliased %+v, path %+v: want the key untouched", out, path)
	}
	spec := &Spec{Source: "zoo.nope", Columns: []Column{{ID: "a", Path: Path{{Field: "mass"}}}}}
	if got := resolveAliases(spec); got != nil || spec.Columns[0].Path[0].Field != "mass" {
		t.Fatalf("unknown source: aliased %+v", got)
	}
}

func TestSpecErrorNamesEveryProblem(t *testing.T) {
	err := invalid("columns.0.id", "an id is required")
	if !strings.Contains(err.Error(), "columns.0.id: an id is required") {
		t.Fatalf("Error() = %q", err.Error())
	}
}
