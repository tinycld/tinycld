package reports

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// everyV3 uses every part version 3 adds (the plan's "For review" spec): a
// count lookup with outer leaves, a sum lookup with a range bound, the All
// dates preset, an opening row and a rollup.
const everyV3 = `{"version":3,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"allDates"}}],
	"columns":[
		{"id":"animal","label":"Animal","path":[{"link":"animal"}]},
		{"id":"fed","label":"Fed","path":[{"field":"fed"}]},
		{"id":"cost","label":"Cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"sameDay","label":"Same day","formula":{"op":"lookup","source":"zoo.feedings",
			"where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]},
				{"op":"eq","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]}]},
			"value":{"path":[{"field":"id"}]},"pick":"count"}},
		{"id":"balance","label":"Balance","formula":{"op":"add","args":[
			{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
				"where":{"op":"and","args":[
					{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"byAnimal","path":[{"field":"id"}]}]},
					{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}]},
				"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]},
			{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]},
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"byAnimal","col":"animal","open":true,"subtotals":true,
		"opening":{"label":"Before the period","cells":{"balance":{"op":"coalesce","args":[
			{"op":"lookup","source":"zoo.feedings","where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"byAnimal","path":[{"field":"id"}]}]},
				{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}]},
			"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}}}}],
	"sort":[{"col":"fed"}]}`

// A version 2 spec loads unchanged as version 3: the step is the identity,
// so the run says nothing about an upgrade and a save stores version 3.
func TestAV2SpecLoadsSilentlyAsVersion3(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":2,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],"groups":[{"id":"g","col":"animal","open":true}]}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != CurrentVersion || info.UpgradedFrom != 0 {
		t.Fatalf("version %d, upgradedFrom %d", spec.Version, info.UpgradedFrom)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	_, stored, err := loadStored([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(stored, &doc); err != nil || doc["version"] != float64(CurrentVersion) {
		t.Fatalf("stored = %s", stored)
	}
}

// A version 1 spec takes both identity steps (1→2, 2→3): it loads as
// version 3, the run says nothing about an upgrade, and a save stores
// version 3.
func TestAV1SpecLoadsSilentlyAsVersion3(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":1,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],"groups":[{"id":"g","col":"animal","open":true}]}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != CurrentVersion || info.UpgradedFrom != 0 {
		t.Fatalf("version %d, upgradedFrom %d", spec.Version, info.UpgradedFrom)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	_, stored, err := loadStored([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(stored, &doc); err != nil || doc["version"] != float64(CurrentVersion) {
		t.Fatalf("stored = %s", stored)
	}
}

func TestAVersion5SpecIsRefused(t *testing.T) {
	_, _, err := Load([]byte(`{"version":5,"source":"zoo.feedings","columns":[]}`))
	requireSpecError(t, err, 400, "version", "version 5 is newer than this server reads (4)")
}

// Every v3 field decodes strictly and is stored as posted; a run's options
// carry the viewer sort.
func TestASpecWithEveryV3FieldLoadsAndIsStoredUnchanged(t *testing.T) {
	withZooRegistry(t)
	spec, info, stored := mustLoad(t, everyV3)
	if info.UpgradedFrom != 0 || len(info.Aliased) != 0 {
		t.Fatalf("info = %+v", info)
	}
	want := jsonDoc(t, []byte(everyV3)).(map[string]any)
	want["version"] = json.Number("4")
	if !reflect.DeepEqual(want, jsonDoc(t, stored)) {
		t.Fatalf("stored spec differs from the posted one:\n%s", stored)
	}
	for _, key := range []string{`"outer":[`, `"bound":"from"`, `"pick":"sum"`, `"pick":"count"`, `"opening":{`, `"rollup":{`, `"preset":"allDates"`} {
		if !strings.Contains(string(stored), key) {
			t.Fatalf("stored spec lost %s: %s", key, stored)
		}
	}
	if o := spec.Groups[0].Opening; o == nil || o.Label != "Before the period" || o.Cells["balance"] == nil {
		t.Fatalf("opening = %+v", o)
	}
	if spec.Columns[4].Rollup == nil || spec.Columns[3].Formula.Where.Args[0].Args[1].Outer[0].Link != "animal" {
		t.Fatalf("columns = %+v", spec.Columns)
	}
	var opts Options
	if err := json.Unmarshal([]byte(`{"sort":[{"col":"cost","dir":"desc"}]}`), &opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opts.Sort, []ViewSort{{Col: "cost", Dir: "desc"}}) {
		t.Fatalf("sort = %+v", opts.Sort)
	}
	if KindOpening != "opening" {
		t.Fatalf("KindOpening = %q", KindOpening)
	}
}

// An aliased key in an outer path, a rollup or an opening cell is replaced
// with its current key, and the formula-size guard counts the new slots.
func TestV3SlotsResolveAliasesAndCountSize(t *testing.T) {
	app := withZooRegistry(t)
	spec, info, _ := mustLoad(t, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"heavier","formula":{"op":"lookup","source":"zoo.animals","pick":"count",
			"where":{"op":"gt","args":[{"path":[{"field":"mass"}]},{"outer":[{"link":"animal"},{"field":"mass"}]}]},
			"value":{"path":[{"field":"name"}]}}},
		{"id":"weight","formula":{"value":null},
			"rollup":{"op":"max","args":[{"path":[{"link":"animal"},{"field":"mass"}]}]}}],
		"groups":[{"id":"g","col":"animal","opening":{"label":"Before","cells":{"weight":{"record":"g","path":[{"field":"mass"}]}}}}]}`)
	for name, got := range map[string]string{
		"lookup where": spec.Columns[1].Formula.Where.Args[0].Path[0].Field,
		"outer":        spec.Columns[1].Formula.Where.Args[1].Outer[1].Field,
		"rollup":       spec.Columns[2].Rollup.Args[0].Path[1].Field,
		"opening cell": spec.Groups[0].Opening.Cells["weight"].Path[0].Field,
	} {
		if got != "weight" {
			t.Errorf("%s = %q, want weight", name, got)
		}
	}
	if len(info.Aliased) != 4 {
		t.Fatalf("aliased %d keys: %+v", len(info.Aliased), info.Aliased)
	}

	big := `{"op":"add","args":[` + strings.TrimSuffix(strings.Repeat(`{"value":1},`, 1000), ",") + `]}`
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","rollup":`+big+`}],
		"groups":[{"id":"g","col":"animal","opening":{"label":"Before","cells":{"cost":`+big+`}}}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "at most 32 levels and 1000 nodes")
	requireSpecError(t, err, 400, "groups.0.opening.cells.cost", "at most 32 levels and 1000 nodes")
}

// v3Refusal is one spec a v3 part refuses: the status, the part's path and
// a piece of the message.
type v3Refusal struct {
	spec   string
	status int
	at     string
	want   string
}

// requireV3Refusal loads the spec and, when it loads, validates it: either
// step may refuse it.
func requireV3Refusal(t *testing.T, app core.App, c v3Refusal) {
	t.Helper()
	spec, _, err := Load([]byte(c.spec))
	if err == nil {
		err = Validate(app, spec)
	}
	requireSpecError(t, err, c.status, c.at, c.want)
}

// Every v3 part is checked where it is: Tasks 2–8 add their rows to cases.
func TestV3PartsAreValidated(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]v3Refusal{
		// F2: the outer leaf.
		"an outer leaf with a value": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"outer":[{"field":"note"}],"value":1}}]}`,
			400, "columns.0.formula", "one of op, col, path, outer"},
		"an outer field the report's source does not have": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"outer":[{"field":"nope"}]}]},
			"value":{"path":[{"field":"weight"}]}}}]}`, 422, "columns.0.formula.where.args.1.outer.0.field", `has no field "nope"`},
		// F9: every new part decodes strictly.
		"an unknown key in an opening": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],
			"groups":[{"id":"g","col":"a","opening":{"label":"x","lable":"y"}}]}`, 400, "spec", `unknown field "lable"`},
		"an unknown key in a rollup": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}],"aggregate":"sum",
			"rollup":{"op":"sum","sql":"x"}}]}`, 400, "spec", `unknown field "sql"`},
		"a bound that is not text": {`{"version":3,"source":"zoo.feedings","params":[{"id":"p","type":"dateRange"}],
			"columns":[{"id":"x","formula":{"param":"p","bound":1}}]}`, 400, "spec", "cannot unmarshal"},
		// F1: sum and count lookups.
		"a pick that is not min, max, sum or count": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","value":{"path":[{"field":"weight"}]},"pick":"avg"}}]}`, 400, "columns.0.formula.pick", "pick is min, max, sum or count"},
		"a sum of days": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","value":{"path":[{"field":"born"}]},"pick":"sum"}}]}`, 400, "columns.0.formula.value", "pick sum adds numbers or amounts"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { requireV3Refusal(t, app, c) })
	}
}
