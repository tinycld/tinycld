package reports

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// loadSpec loads a spec and fails the test when it is not well-formed JSON.
func loadSpec(t *testing.T, raw string) *Spec {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return spec
}

func TestValidateAcceptsTheDesignExample(t *testing.T) {
	app := withZooRegistry(t)
	if err := Validate(app, loadSpec(t, designExample)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateNamesEveryUnknownReference(t *testing.T) {
	app := withZooRegistry(t)
	RegisterSource(Source{
		Key: "zoo.pens", Collection: "zoo_pens", Label: "Pens",
		Links: []Link{{Key: "yard", Label: "Yard", Target: "zoo.yards", Cardinality: ToOne, Field: "yard"}},
	})
	cases := map[string]struct {
		spec, at, code string
	}{
		"source": {`{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"name"}]}]}`, "source", "unknown_source"},
		"link": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"pen"},{"field":"name"}]}]}`,
			"columns.0.path.0.link", "unknown_link"},
		"field": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"},{"field":"tail"}]}]}`,
			"columns.0.path.1.field", "unknown_field"},
		"link target": {`{"version":1,"source":"zoo.pens","columns":[{"id":"a","path":[{"link":"yard"},{"field":"name"}]}]}`,
			"columns.0.path.0.link", "unknown_source"},
		"sort column": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}]}],"sort":[{"col":"b"}]}`,
			"sort.0.col", "unknown_column"},
		"group column": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}]}],"groups":[{"id":"g","col":"b"}]}`,
			"groups.0.col", "unknown_column"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(app, loadSpec(t, c.spec))
			requireSpecError(t, err, 422, c.at, "")
			if code := err.(*SpecError).Problems[c.at].Code(); code != c.code {
				t.Fatalf("code = %q, want %q", code, c.code)
			}
		})
	}
}

func TestValidateRefusesAnInvalidSpec(t *testing.T) {
	app := withZooRegistry(t)
	cols := func(c string) string {
		return `{"version":1,"source":"zoo.feedings","columns":[` + c + `]}`
	}
	cases := map[string]struct {
		spec, at, want string
	}{
		"no columns":  {cols(``), "columns", "1 to 100 columns"},
		"bad id":      {cols(`{"id":"1a","path":[{"field":"cost"}]}`), "columns.0.id", "an id of letters"},
		"repeated id": {cols(`{"id":"a","path":[{"field":"cost"}]},{"id":"a","path":[{"field":"fed"}]}`), "columns.1.id", "used twice"},
		"path and formula": {cols(`{"id":"a","path":[{"field":"cost"}],"formula":{"value":1}}`), "columns.0",
			"a path or a formula"},
		"sum across to-many": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings"},{"field":"cost"}],"aggregate":"sum"}]}`,
			"columns.0.path.0", "a sum across a to-many link would double-count"},
		"to-many without mode": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings"},{"field":"cost"}]}]}`,
			"columns.0.path.0", "needs mode aggregate or nest"},
		"total without aggregate": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"aggregate"},{"field":"cost"}]}]}`,
			"columns.0.aggregate", "needs an aggregate"},
		"two to-many totals": {`{"version":1,"source":"zoo.sites","columns":[{"id":"a","path":[{"link":"animals","mode":"aggregate"},{"link":"feedings","mode":"aggregate"},{"field":"cost"}],"aggregate":"sum"}]}`,
			"columns.0.path.1", "a total crosses one to-many link"},
		"mode on to-one": {cols(`{"id":"a","path":[{"link":"animal","mode":"nest"},{"field":"name"}]}`), "columns.0.path.0",
			"mode is for a to-many link"},
		"too many links": {cols(`{"id":"a","path":[{"link":"animal"},{"link":"feedings","mode":"nest"},{"link":"animal"},{"link":"feedings","mode":"nest"},{"link":"animal"},{"field":"name"}]}`),
			"columns.0.path.4", "at most 4 links"},
		"field not last": {cols(`{"id":"a","path":[{"field":"cost"},{"link":"animal"}]}`), "columns.0.path.0", "the last step"},
		"no field":       {cols(`{"id":"a","path":[{"link":"animal"}]}`), "columns.0.path", "ends with a field"},
		"total of child rows": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}],"aggregate":"sum"}]}`,
			"columns.0.aggregate", "child rows are not totalled"},
		"required nest": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"nest","with":"required"},{"field":"cost"}]}]}`,
			"columns.0.path.0", "with does not apply"},
		"two nest chains": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}]},{"id":"b","path":[{"link":"keepers","mode":"nest"},{"link":"user"},{"field":"name"}]}]}`,
			"columns.1.path", "nests one chain of child rows"},
		"aggregate not offered": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"field":"species"}],"aggregate":"sum"}]}`,
			"columns.0.aggregate", "Species does not offer sum"},
		"distinct count across to-many": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"aggregate"},{"field":"id"}],"aggregate":"countDistinct"}]}`,
			"columns.0.aggregate", "cannot be totalled"},
		"to-many total of mixed currencies": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"link":"feedings","mode":"aggregate"},{"field":"cost"}],"aggregate":"sum"}]}`,
			"columns.0.aggregate", "different currencies"},
		"avg without scale": {cols(`{"id":"a","path":[{"field":"cost"}],"aggregate":"avg"}`), "columns.0.scale", "needs a scale"},
		"scale without avg": {cols(`{"id":"a","path":[{"field":"cost"}],"aggregate":"sum","scale":2}`), "columns.0.scale", "only an avg column"},
		"bucket of text": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"a","bucket":"month"}]}`,
			"groups.0.bucket", "only a date column"},
		"pivot without groups": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}],"aggregate":"sum"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"a"}}`,
			"pivot", "needs at least one group"},
		"pivot cell without aggregate": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}]},{"id":"n","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"n"}],"pivot":{"path":[{"field":"fed"}],"cell":"a"}}`,
			"pivot.cell", "a column with an aggregate"},
		"pivot across to-many": {`{"version":1,"source":"zoo.animals","columns":[{"id":"a","path":[{"field":"weight"}],"aggregate":"sum"},{"id":"n","path":[{"field":"name"}]}],"groups":[{"id":"g","col":"n"}],"pivot":{"path":[{"link":"feedings"},{"field":"fed"}],"cell":"a"}}`,
			"pivot.path.0", "reads a to-many link only through exists"},
		"layout without groups": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}],"aggregate":"sum"}],"layout":[{"section":"s","title":"S","key":"x"}]}`,
			"layout", "needs a group"},
		"layout item of both kinds": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"a"}],"layout":[{"section":"s","row":"r","title":"S","key":"x"}]}`,
			"layout.0", "a section or a computed row"},
		"section without key": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"a"}],"layout":[{"section":"s","title":"S"}]}`,
			"layout.0", "the key of a first-level group"},
		"version": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}]}]}`, "version", "load the spec first"},
		"param":   {`{"version":1,"source":"zoo.feedings","params":[{"id":"p","type":"color"}],"columns":[{"id":"a","path":[{"field":"cost"}]}]}`, "params.0.type", "unknown param type"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Decoded directly, not loaded: Load itself refuses a newer
			// version, and Validate must refuse a spec that skipped Load.
			var spec Spec
			if err := json.Unmarshal([]byte(c.spec), &spec); err != nil {
				t.Fatalf("decode: %v", err)
			}
			requireSpecError(t, Validate(app, &spec), 400, c.at, c.want)
		})
	}
}

// chainSpec is grams as c0, then n formula columns: c<i> is step with PREV
// standing for c<i-1>.
func chainSpec(n int, step string) string {
	cols := []string{`{"id":"c0","path":[{"field":"grams"}]}`}
	for i := 1; i <= n; i++ {
		formula := strings.ReplaceAll(step, "PREV", fmt.Sprintf("c%d", i-1))
		cols = append(cols, fmt.Sprintf(`{"id":"c%d","formula":%s}`, i, formula))
	}
	return `{"version":1,"source":"zoo.feedings","columns":[` + strings.Join(cols, ",") + `]}`
}

const doubling = `{"op":"add","args":[{"col":"PREV"},{"col":"PREV"}]}`

// Each column reads the one before twice, so its formula doubles once the
// columns it reads are inlined: c9 has 1,023 nodes. Validate refuses it from
// the columns' sizes, before anything compiles. With 99 columns the last one
// has 2^100 nodes: the count must not overflow into a small number.
func TestValidateRefusesAFormulaChainThatDoubles(t *testing.T) {
	app := withZooRegistry(t)
	for _, n := range []int{27, 99} {
		err := Validate(app, loadSpec(t, chainSpec(n, doubling)))
		requireSpecError(t, err, 400, "columns.9", "with the formula columns it reads")
		requireSpecError(t, err, 400, fmt.Sprintf("columns.%d", n), "with the formula columns it reads")
		if _, ok := err.(*SpecError).Problems["columns.8"]; ok {
			t.Fatalf("n=%d: c8 (511 nodes) was refused: %v", n, err)
		}
	}
}

// Each column adds one level to the one before, so c33 is 33 levels deep
// once inlined; SQLite would refuse the statement at run time.
func TestValidateRefusesADeepFormulaChain(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, chainSpec(40, `{"op":"add","args":[{"col":"PREV"},{"value":1}]}`)))
	requireSpecError(t, err, 400, "columns.33", "with the formula columns it reads")
	if _, ok := err.(*SpecError).Problems["columns.32"]; ok {
		t.Fatalf("c32 (32 levels) was refused: %v", err)
	}
}

// A filter is compiled with the columns it reads inlined too.
func TestValidateRefusesAFilterTooLargeWithTheColumnsItReads(t *testing.T) {
	app := withZooRegistry(t)
	raw := strings.TrimSuffix(chainSpec(8, doubling), "}") + `,"filter":{"op":"and","args":[
		{"op":"gt","args":[{"col":"c8"},{"value":0}]},{"op":"gt","args":[{"col":"c8"},{"value":0}]}]}}`
	requireSpecError(t, Validate(app, loadSpec(t, raw)), 400, "filter", "with the formula columns it reads")
}

// Run binds the spec the same way: the doubling chain is a 400, not a
// statement that takes hours to build.
func TestRunRefusesAFormulaChainThatDoubles(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", chainSpec(27, doubling), Options{}, nil)
	requireSpecError(t, err, 400, "columns.9", "with the formula columns it reads")
}

func TestBindRecordsTheNestChain(t *testing.T) {
	app := withZooRegistry(t)
	spec := loadSpec(t, `{"version":1,"source":"zoo.sites","columns":[
		{"id":"site","path":[{"field":"name"}]},
		{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"name"}]},
		{"id":"fed","path":[{"link":"animals","mode":"nest"},{"link":"feedings","mode":"nest"},{"field":"fed"}]},
		{"id":"owner","path":[{"link":"animals","mode":"nest"},{"link":"owner"},{"field":"name"}]}]}`)
	p, err := bind(app, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.nest) != 2 {
		t.Fatalf("%d nest levels, want 2", len(p.nest))
	}
	ids := func(l *nestLevel) []string {
		var out []string
		for _, cp := range l.cols {
			out = append(out, cp.col.ID)
		}
		return out
	}
	if got := ids(p.nest[0]); len(got) != 2 || got[0] != "animal" || got[1] != "owner" {
		t.Fatalf("level 1 columns = %v", got)
	}
	if got := ids(p.nest[1]); len(got) != 1 || got[0] != "fed" {
		t.Fatalf("level 2 columns = %v", got)
	}
}
