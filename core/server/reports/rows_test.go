package reports

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// wRows compiles a spec's w for user and returns its rows: __id and each
// path column's value, keyed by column id, sorted by the first column.
func (z *zoo) wRows(t *testing.T, user, raw string, params map[string]string) ([]map[string]any, error) {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, specErr := bind(z.app, spec)
	if specErr != nil {
		return nil, specErr
	}
	given := map[string]json.RawMessage{}
	for k, v := range params {
		given[k] = json.RawMessage(v)
	}
	values, specErr := resolveParams(spec.Params, given, periodContext{today: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), fiscalStart: 1})
	if specErr != nil {
		return nil, specErr
	}
	c := newCompiler(p, z.app, z.as(user), values)
	s := newStmt(z.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	sel := []string{"__id"}
	for idx, cp := range p.cols {
		if cp.path != nil && cp.nest == 0 {
			sel = append(sel, fmt.Sprintf("c%d AS %s", idx, cp.col.ID))
		}
	}
	with, err := s.with(r.render())
	if err != nil {
		return nil, err
	}
	rows, err := z.app.DB().NewQuery(with + "SELECT " + strings.Join(sel, ", ") + " FROM w").Bind(s.params).Rows()
	if err != nil {
		return nil, err
	}
	out, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	first := p.cols[0].col.ID
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i][first]) < fmt.Sprint(out[j][first]) })
	return out, nil
}

// column lists one column of rows.
func column(rows []map[string]any, id string) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r[id]
	}
	return out
}

func (z *zoo) mustW(t *testing.T, user, raw string, params map[string]string) []map[string]any {
	t.Helper()
	rows, err := z.wRows(t, user, raw, params)
	if err != nil {
		t.Fatalf("w: %v", err)
	}
	return rows
}

func TestWHoldsOnlyRowsTheReaderMayList(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}]}`
	if got := column(z.mustW(t, "ann", spec, nil), "note"); !reflect.DeepEqual(got, []any{"f1", "f2", "f3", "f4"}) {
		t.Fatalf("ann's feedings = %v", got)
	}
	if got := column(z.mustW(t, "bob", spec, nil), "note"); !reflect.DeepEqual(got, []any{"f3", "f4", "f5"}) {
		t.Fatalf("bob's feedings = %v", got)
	}
}

func TestAHiddenLinkedRowLeavesEmptyCellsAndKeepsTheRow(t *testing.T) {
	z := newZoo(t)
	// Bob keeps zed, owned by... bob; zed's site is south. The vault is
	// superuser-only, so its link is always hidden.
	rows := z.mustW(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"owner","path":[{"link":"owner"},{"field":"name"}]},
		{"id":"site","path":[{"link":"site"},{"field":"name"}]}]}`, nil)
	if got := column(rows, "name"); !reflect.DeepEqual(got, []any{"Kit", "Leo", "Zed"}) {
		t.Fatalf("names = %v", got)
	}
	// Kit has no site: a blank relation is an empty cell too.
	if got := column(rows, "site"); !reflect.DeepEqual(got, []any{nil, "North", "South"}) {
		t.Fatalf("sites = %v", got)
	}
	// Members may list other members (core's users rule), so owners show.
	if got := column(rows, "owner"); !reflect.DeepEqual(got, []any{"ann", "ann", "bob"}) {
		t.Fatalf("owners = %v", got)
	}
}

// Leo links to a vault row that only a superuser may list. The join reads the
// rule CTE, so the cell is empty and Leo stays; a join over the table would
// show "Gold".
func TestAToOneLinkToASuperuserOnlyCollectionGivesEmptyCellsAndKeepsTheParent(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"gold","path":[{"link":"vault"},{"field":"name"}]}]}`
	rows := z.mustW(t, "ann", spec, nil)
	if got := column(rows, "name"); !reflect.DeepEqual(got, []any{"Kit", "Leo", "Zed"}) {
		t.Fatalf("names = %v", got)
	}
	if got := column(rows, "gold"); !reflect.DeepEqual(got, []any{nil, nil, nil}) {
		t.Fatalf("vault names = %v", got)
	}
	required := strings.Replace(spec, `{"link":"vault"}`, `{"link":"vault","with":"required"}`, 1)
	if got := z.mustW(t, "ann", required, nil); len(got) != 0 {
		t.Fatalf("a required hidden link keeps rows: %v", got)
	}
}

func TestRequiredLinkDropsRowsWithoutIt(t *testing.T) {
	z := newZoo(t)
	rows := z.mustW(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"site","path":[{"link":"site","with":"required"},{"field":"name"}]}]}`, nil)
	if got := column(rows, "name"); !reflect.DeepEqual(got, []any{"Leo", "Zed"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestAToManyTotalCountsOnlyVisibleLinkedRows(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","path":[{"link":"feedings","mode":"aggregate"},{"field":"id"}],"aggregate":"count"},
		{"id":"cost","path":[{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"sum"},
		{"id":"last","path":[{"link":"feedings","mode":"aggregate"},{"field":"fed"}],"aggregate":"max"}]}`
	rows := z.mustW(t, "ann", spec, nil)
	// Ann sees kit but keeps no feeding of kit: f6 is hidden from her.
	want := map[string][]any{
		"name": {"Kit", "Leo", "Zed"},
		"fed":  {int64(0), int64(2), int64(2)},
		"cost": {int64(0), int64(1300), int64(400)},
		"last": {nil, "2026-02-10", "2026-03-03"},
	}
	for id, w := range want {
		if got := column(rows, id); !reflect.DeepEqual(got, w) {
			t.Errorf("%s = %#v, want %#v", id, got, w)
		}
	}
}

func TestFilterExistsAndNotExists(t *testing.T) {
	z := newZoo(t)
	base := `{"version":1,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}],"filter":%s}`
	cases := map[string]struct {
		filter string
		want   []any
	}{
		"has an animal that is a zebra": {`{"op":"exists","link":[{"link":"animals"}],"where":{"op":"eq","args":[{"path":[{"field":"species"}]},{"value":"zebra"}]}}`, []any{"South"}},
		"has no visible animal":         {`{"op":"notExists","link":[{"link":"animals"}]}`, []any{}},
		"has a fed animal": {`{"op":"exists","link":[{"link":"animals"},{"link":"feedings"}],
			"where":{"op":"gt","args":[{"path":[{"field":"grams"}]},{"value":400}]}}`, []any{"North"}},
		"or of exists": {`{"op":"or","args":[{"op":"exists","link":[{"link":"animals"}],"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Zed"}]}},
			{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]}]}`, []any{"North", "South"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rows := z.mustW(t, "ann", fmt.Sprintf(base, c.filter), nil)
			got := column(rows, "name")
			if len(got) == 0 {
				got = []any{}
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("sites = %v, want %v", got, c.want)
			}
		})
	}
}

func TestExistsLinkRowsTheReaderCannotSeeDoNotMatch(t *testing.T) {
	z := newZoo(t)
	// Mo lives in the north, but ann cannot list mo.
	rows := z.mustW(t, "ann", `{"version":1,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"exists","link":[{"link":"animals"}],"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Mo"}]}}}`, nil)
	if len(rows) != 0 {
		t.Fatalf("rows = %v", rows)
	}
}

func TestAFilterOnAnUnsetParamMatchesEveryRow(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings",
		"params":[{"id":"species","type":"text"},{"id":"period","type":"dateRange"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"param":"species"}]},
			{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]}]}}`
	if got := column(z.mustW(t, "ann", spec, nil), "note"); len(got) != 4 {
		t.Fatalf("unset params: %v", got)
	}
	got := column(z.mustW(t, "ann", spec, map[string]string{"species": `"lion"`, "period": `{"from":"2026-02-01","to":"2026-02-28"}`}), "note")
	if !reflect.DeepEqual(got, []any{"f2"}) {
		t.Fatalf("set params: %v", got)
	}
}

// An unset param makes its condition match every row, and so does any
// condition built only of such conditions: not and notExists included. A
// cleared chip filters nothing, however the filter wraps it.
func TestAnUnsetParamUnderNotOrNotExistsMatchesEveryRow(t *testing.T) {
	z := newZoo(t)
	feedings := `{"version":1,"source":"zoo.feedings","params":[{"id":"t","type":"text"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],"filter":%s}`
	every := []any{"f1", "f2", "f3", "f4"}
	cases := map[string]struct {
		filter     string
		unset, set []any
	}{
		"not": {`{"op":"not","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"param":"t"}]}]}`,
			every, []any{"f2", "f3", "f4"}},
		"and of unset and not": {`{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"param":"t"}]},
			{"op":"not","args":[{"op":"contains","args":[{"path":[{"field":"note"}]},{"param":"t"}]}]}]}`,
			every, []any{}},
		// The unset condition drops out; the set one still filters.
		"or keeps a set condition": {`{"op":"or","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"param":"t"}]},
			{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f3"}]}]}`,
			[]any{"f3"}, []any{"f1", "f3"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec := fmt.Sprintf(feedings, c.filter)
			for _, run := range []struct {
				params map[string]string
				want   []any
			}{{nil, c.unset}, {map[string]string{"t": `"f1"`}, c.set}} {
				got := column(z.mustW(t, "ann", spec, run.params), "note")
				if !reflect.DeepEqual(got, run.want) {
					t.Fatalf("params %v: %v, want %v", run.params, got, run.want)
				}
			}
		})
	}
	// Kit has no feeding ann may see: with the param set, only kit has no
	// such feeding; unset, the where filters nothing and every animal stays.
	animals := `{"version":1,"source":"zoo.animals","params":[{"id":"t","type":"text"}],
		"columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"notExists","link":[{"link":"feedings"}],"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"param":"t"}]}}}`
	if got := column(z.mustW(t, "ann", animals, nil), "name"); !reflect.DeepEqual(got, []any{"Kit", "Leo", "Zed"}) {
		t.Fatalf("notExists, unset: %v", got)
	}
	if got := column(z.mustW(t, "ann", animals, map[string]string{"t": `"f1"`}), "name"); !reflect.DeepEqual(got, []any{"Kit", "Zed"}) {
		t.Fatalf("notExists, set: %v", got)
	}
}

// Validate checks a condition on a param as Run does: the param being unset
// in a dry compile does not skip the argument count or the types.
func TestValidateChecksAConditionOnAnUnsetParam(t *testing.T) {
	app := withZooRegistry(t)
	spec := func(filter string) string {
		return `{"version":1,"source":"zoo.feedings","params":[{"id":"n","type":"number"}],
			"columns":[{"id":"note","path":[{"field":"note"}]}],"filter":` + filter + `}`
	}
	err := Validate(app, loadSpec(t, spec(`{"op":"gt","args":[{"path":[{"field":"note"}]},{"param":"n"}]}`)))
	requireSpecError(t, err, 400, "filter", "compares numbers, dates or text")
	err = Validate(app, loadSpec(t, spec(`{"op":"eq","args":[{"param":"n"}]}`)))
	requireSpecError(t, err, 400, "filter", "takes 2 values")
	err = Validate(app, loadSpec(t, spec(`{"op":"not","args":[{"op":"between","args":[{"path":[{"field":"note"}]},{"param":"n"},{"value":true},{"value":1}]}]}`)))
	requireSpecError(t, err, 400, "filter.args.0", "between takes")
}

// A dry compile gives a param its default: Validate compiles what a run with
// no posted value compiles.
func TestValidateCompilesAParamWithItsDefault(t *testing.T) {
	app := withZooRegistry(t)
	p, err := bind(app, loadSpec(t, `{"version":1,"source":"zoo.feedings",
		"params":[{"id":"n","type":"number","default":5},{"id":"t","type":"text"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c := newCompiler(p, nil, nil, nil)
	if n := c.params["n"]; n.unset || n.value != int64(5) {
		t.Fatalf("n = %+v, want its default 5", n)
	}
	if !c.params["t"].unset {
		t.Fatalf("t = %+v, want unset", c.params["t"])
	}
}

func TestAFilterReadsAFormulaColumnAndAToManyTotal(t *testing.T) {
	z := newZoo(t)
	rows := z.mustW(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","path":[{"link":"feedings","mode":"aggregate"},{"field":"id"}],"aggregate":"count"},
		{"id":"heavy","formula":{"op":"gt","args":[{"path":[{"field":"weight"}]},{"value":100}]}}],
		"filter":{"op":"and","args":[{"col":"heavy"},{"op":"gt","args":[{"col":"fed"},{"value":0}]}]}}`, nil)
	if got := column(rows, "name"); !reflect.DeepEqual(got, []any{"Leo", "Zed"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestRowCompileErrors(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct {
		spec, at, want string
		status         int
	}{
		"to-many value in a filter": {`{"version":1,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]}],
			"filter":{"op":"gt","args":[{"path":[{"link":"feedings"},{"field":"grams"}]},{"value":1}]}}`,
			"filter.args.0.path.0", "reads a to-many link only through exists", 400},
		"total in a filter": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
			"filter":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}}`,
			"filter.args.0", "use it in a column, not in a filter", 400},
		"filter not a condition": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
			"filter":{"path":[{"field":"cost"}]}}`, "filter", "the filter is a condition", 400},
		"unknown param": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
			"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"param":"who"}]}}`, "filter.args.1", "no param", 422},
		"formula reads itself": {`{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},
			{"id":"a","formula":{"op":"add","args":[{"col":"b"},{"value":1}]}},{"id":"b","formula":{"op":"add","args":[{"col":"a"},{"value":1}]}}],
			"filter":{"op":"gt","args":[{"col":"a"},{"value":1}]}}`, "columns.2.formula.args.0", "reads itself", 400},
		"where reads a column": {`{"version":1,"source":"zoo.sites","columns":[{"id":"n","path":[{"field":"name"}]}],
			"filter":{"op":"exists","link":[{"link":"animals"}],"where":{"op":"eq","args":[{"col":"n"},{"value":"x"}]}}}`,
			"filter.where.args.0", "not report columns", 400},
		"unknown exists link": {`{"version":1,"source":"zoo.sites","columns":[{"id":"n","path":[{"field":"name"}]}],
			"filter":{"op":"exists","link":[{"link":"pens"}]}}`, "filter.link.0.link", "no link", 422},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.wRows(t, "ann", c.spec, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
		})
	}
}

func TestAMultipleChoiceParamInAFilter(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings",
		"params":[{"id":"species","type":"choice","multiple":true,"options":[{"value":"lion","label":"Lion"},{"value":"zebra","label":"Zebra"}]}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"in","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"param":"species"}]}}`
	cases := map[string]struct {
		params map[string]string
		want   []any
	}{
		"one choice":         {map[string]string{"species": `["lion"]`}, []any{"f1", "f2"}},
		"two choices":        {map[string]string{"species": `["zebra","lion"]`}, []any{"f1", "f2", "f3", "f4"}},
		"unset":              {nil, []any{"f1", "f2", "f3", "f4"}},
		"empty list":         {map[string]string{"species": `[]`}, []any{"f1", "f2", "f3", "f4"}},
		"posted null":        {map[string]string{"species": `null`}, []any{"f1", "f2", "f3", "f4"}},
		"choice with no row": {map[string]string{"species": `["zebra"]`}, []any{"f3", "f4"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := column(z.mustW(t, "ann", spec, c.params), "note"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("notes = %v, want %v", got, c.want)
			}
		})
	}
}

// A formula column is a group column: its key is computed in w.
func TestAFormulaColumnIsAGroupKey(t *testing.T) {
	z := newZoo(t)
	spec, _, err := Load([]byte(`{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"heavy","formula":{"op":"gt","args":[{"path":[{"field":"weight"}]},{"value":100}]}}],
		"groups":[{"id":"g","col":"heavy"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p, specErr := bind(z.app, spec)
	if specErr != nil {
		t.Fatal(specErr)
	}
	c := newCompiler(p, z.app, z.as("ann"), nil)
	s := newStmt(z.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		t.Fatal(err)
	}
	with, err := s.with(r.render())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := z.app.DB().NewQuery(with + "SELECT k0 FROM w ORDER BY k0, __id").Bind(s.params).Rows()
	if err != nil {
		t.Fatal(err)
	}
	out, err := scanRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got := column(out, "k0"); !reflect.DeepEqual(got, []any{int64(0), int64(1), int64(1)}) {
		t.Fatalf("keys = %v", got)
	}
}

func TestAFormulaOperandAcrossAToManyLinkIsRefused(t *testing.T) {
	z := newZoo(t)
	_, err := z.wRows(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"n","path":[{"field":"name"}]},
		{"id":"x","formula":{"op":"gt","args":[{"path":[{"link":"feedings"},{"field":"grams"}]},{"value":1}]}}],
		"filter":{"col":"x"}}`, nil)
	requireSpecError(t, err, 400, "columns.1.formula.args.0.path.0", "reads a to-many link only through exists")
}

func TestAnUnknownReferenceInAFilterIsRefused(t *testing.T) {
	z := newZoo(t)
	base := `{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":%s},{"value":"x"}]}}`
	cases := map[string]struct{ path, at, want string }{
		"field": {`[{"field":"nope"}]`, "filter.args.0.path.0.field", "no field"},
		"link":  {`[{"link":"pens"},{"field":"name"}]`, "filter.args.0.path.0.link", "no link"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.wRows(t, "ann", fmt.Sprintf(base, c.path), nil)
			requireSpecError(t, err, 422, c.at, c.want)
		})
	}
}

func TestAFilterPathFollowsFourLinksAndNotFive(t *testing.T) {
	z := newZoo(t)
	base := `{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
		"filter":{"op":"isEmpty","args":[{"path":[%s]}]}}`
	four := `{"link":"animal"},{"link":"vault"},{"link":"animal"},{"link":"owner"},{"field":"name"}`
	// The vault is hidden from ann, so the end of the chain is empty on every row.
	rows, err := z.wRows(t, "ann", fmt.Sprintf(base, four), nil)
	if err != nil {
		t.Fatalf("4 links and a field: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %v", rows)
	}
	five := `{"link":"animal"},{"link":"vault"},{"link":"animal"},{"link":"vault"},{"link":"animal"},{"field":"species"}`
	_, err = z.wRows(t, "ann", fmt.Sprintf(base, five), nil)
	requireSpecError(t, err, 400, "filter.args.0.path.4", "at most 4 links")
}

// A visible linked row whose where field is blank: exists is false, so
// notExists is true, as not(exists) is.
func TestExistsWithABlankWhereFieldIsFalseNotNull(t *testing.T) {
	z := newZoo(t)
	blank := z.rec(t, "blank", "zoo_sites", map[string]any{"name": "Blank"})
	z.rec(t, "ivy", "zoo_animals", map[string]any{"name": "Ivy", "species": "ibis", "site": blank.Id, "owner": z.id("ann")})
	where := `{"op":"gt","args":[{"path":[{"field":"region"}]},{"value":"a"}]}`
	link := `[{"link":"site"}]`
	cases := map[string]struct {
		filter string
		want   []any
	}{
		"exists":     {`{"op":"exists","link":` + link + `,"where":` + where + `}`, []any{"Leo", "Zed"}},
		"notExists":  {`{"op":"notExists","link":` + link + `,"where":` + where + `}`, []any{"Ivy", "Kit"}},
		"not exists": {`{"op":"not","args":[{"op":"exists","link":` + link + `,"where":` + where + `}]}`, []any{"Ivy", "Kit"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rows := z.mustW(t, "ann", `{"version":1,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]}],"filter":`+c.filter+`}`, nil)
			if got := column(rows, "name"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("animals = %v, want %v", got, c.want)
			}
		})
	}
}

func TestWithDoesNotApplyToAnExistsLink(t *testing.T) {
	z := newZoo(t)
	base := `{"version":1,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]}],"filter":%s}`
	cases := map[string]string{
		"notExists": `{"op":"notExists","link":[{"link":"site","with":"required"}]}`,
		"or":        `{"op":"or","args":[{"op":"exists","link":[{"link":"site","with":"required"}]},{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Kit"}]}]}`,
	}
	for name, filter := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.wRows(t, "ann", fmt.Sprintf(base, filter), nil)
			requireSpecError(t, err, 400, map[string]string{"notExists": "filter.link.0", "or": "filter.args.0.link.0"}[name], "with does not apply to an exists link")
		})
	}
}
