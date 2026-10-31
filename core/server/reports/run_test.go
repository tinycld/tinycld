package reports

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// run loads and runs a spec as user, failing the test on an error.
func (z *zoo) run(t *testing.T, user, raw string, opts Options, params map[string]string) *Result {
	t.Helper()
	res, err := z.try(t, user, raw, opts, params)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func (z *zoo) try(t *testing.T, user, raw string, opts Options, params map[string]string) (*Result, error) {
	t.Helper()
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	given := map[string]json.RawMessage{}
	for k, v := range params {
		given[k] = json.RawMessage(v)
	}
	return Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Params: given, Options: opts, Auth: z.recs[user]})
}

// withLimits changes the run limits for one test.
func withLimits(t *testing.T, edit func(*limits)) {
	t.Helper()
	saved := runLimits
	edit(&runLimits)
	t.Cleanup(func() { runLimits = saved })
}

// rowsOf lists the rows' ids, kinds or labels, or one cell, in order.
func rowsOf(res *Result, what string) []any {
	out := make([]any, len(res.Rows))
	for i, r := range res.Rows {
		switch what {
		case "kind":
			out[i] = r.Kind
		case "label":
			out[i] = r.Label
		default:
			out[i] = r.Cells[what]
		}
	}
	return out
}

func requireRows(t *testing.T, res *Result, what string, want ...any) {
	t.Helper()
	if got := rowsOf(res, what); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v\nwant   %#v", what, got, want)
	}
}

const feedingsByDay = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"active","path":[{"link":"animal"},{"field":"active"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"sort":[{"col":"fed"}]}`

func TestRunDetailRowsAndTheTotal(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	requireRows(t, res, "kind", "detail", "detail", "detail", "detail", "total")
	requireRows(t, res, "note", "f1", "f3", "f2", "f4", nil)
	requireRows(t, res, "fed", "2026-01-05", "2026-01-20", "2026-02-10", "2026-03-03", nil)
	requireRows(t, res, "active", true, true, true, true, nil)
	requireRows(t, res, "cost", int64(1000), int64(700), int64(2500), int64(300), int64(4500))
	requireRows(t, res, "label", "f1", "f3", "f2", "f4", "Total")
	if res.Rows[0].ID != "d:"+z.id("f1") || res.Rows[4].ID != `total:["USD"]` {
		t.Fatalf("ids = %s, %s", res.Rows[0].ID, res.Rows[4].ID)
	}
	bob := z.run(t, "bob", feedingsByDay, Options{}, nil)
	requireRows(t, bob, "note", "f3", "f5", "f4", nil)
	requireRows(t, bob, "cost", int64(700), int64(150), int64(300), int64(1150))
}

func TestRunDescribesItsColumns(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}],"aggregate":"count","format":"plain"},
		{"id":"region","label":"Where","path":[{"link":"site"},{"field":"region"}]},
		{"id":"price","path":[{"field":"price"}],"aggregate":"sum"}],
		"sort":[{"col":"name"}]}`, Options{}, nil)
	want := []ResultColumn{
		{ID: "name", Label: "Name", Type: Text, TotalType: Number, Aggregate: Count, Format: "plain"},
		{ID: "region", Label: "Where", Type: Select, Options: []Option{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}},
		{ID: "price", Label: "Price", Type: Money, Aggregate: Sum, Currency: "USD"},
	}
	if !reflect.DeepEqual(res.Columns, want) {
		t.Fatalf("columns = %+v", res.Columns)
	}
	requireRows(t, res, "name", "Kit", "Leo", "Zed", int64(3))
}

func TestRunTotalsMoneyExactly(t *testing.T) {
	z := newZoo(t)
	// 2^53 + 1 is not a float64; a float sum would lose the 1.
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9007199254740993 WHERE note = 'f1'").Execute(); err != nil {
		t.Fatal(err)
	}
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if got := res.Rows[len(res.Rows)-1].Cells["cost"]; got != int64(9007199254744493) {
		t.Fatalf("total = %#v", got)
	}
}

func TestRunReportsAnAmountTooLarge(t *testing.T) {
	z := newZoo(t)
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9223372036854775807 WHERE note IN ('f1', 'f2')").Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := z.try(t, "ann", feedingsByDay, Options{}, nil); !errors.Is(err, ErrAmountTooLarge) {
		t.Fatalf("err = %v, want ErrAmountTooLarge", err)
	}
}

func TestRunStopsAtTheTimeLimit(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.timeout = time.Nanosecond })
	if _, err := z.try(t, "ann", feedingsByDay, Options{}, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	withLimits(t, func(l *limits) { l.timeout = 10 * time.Second })
	if _, err := z.try(t, "ann", feedingsByDay, Options{}, nil); err != nil {
		t.Fatalf("with the limit lifted: %v", err)
	}
}

func TestRunListsTheTopicsOfEveryCollectionItReads(t *testing.T) {
	z := newZoo(t)
	// The feedings rule reads zoo_keepers; the animals rule reads it too.
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	want := []string{"collection/zoo_animals", "collection/zoo_feedings", "collection/zoo_keepers"}
	if !reflect.DeepEqual(res.Topics, want) {
		t.Fatalf("topics = %v", res.Topics)
	}
	sites := z.run(t, "ann", `{"version":1,"source":"zoo.sites","columns":[{"id":"n","path":[{"field":"name"}]}]}`, Options{}, nil)
	// The sites rule reads the signed-in user's role.
	if !reflect.DeepEqual(sites.Topics, []string{"collection/users", "collection/zoo_sites"}) {
		t.Fatalf("topics = %v", sites.Topics)
	}
}

func TestRunNeedsAReader(t *testing.T) {
	z := newZoo(t)
	spec, _, _ := Load([]byte(feedingsByDay))
	if _, err := Run(context.Background(), z.app, Request{Spec: spec}); !errors.Is(err, ErrNoReader) {
		t.Fatalf("no reader: %v", err)
	}
	superusers, err := z.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), z.app, Request{Spec: spec, Auth: core.NewRecord(superusers)}); !errors.Is(err, ErrNoReader) {
		t.Fatalf("superuser: %v", err)
	}
}

func TestRunReturnsSpecErrors(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"sort":[{"col":"x"}]}`, Options{}, nil)
	requireSpecError(t, err, 422, "sort.0.col", "no column")
	_, err = z.try(t, "ann", feedingsByDay, Options{Today: "2026-13-01"}, nil)
	requireSpecError(t, err, 400, "options.today", "YYYY-MM-DD")
}

func TestRunNoticesRenamedFieldsAndAnUpgrade(t *testing.T) {
	z := newZoo(t)
	spec, info, err := Load([]byte(`{"version":1,"source":"zoo.animals","columns":[{"id":"w","path":[{"field":"mass"}],"aggregate":"sum"}],"sort":[{"col":"w"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	info.UpgradedFrom = 1
	res, err := Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Auth: z.recs["ann"]})
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{}
	for _, n := range res.Notices {
		codes = append(codes, n.Code)
	}
	if !reflect.DeepEqual(codes, []string{"upgraded", "aliased"}) || res.Notices[1].Aliases[0].From != "mass" {
		t.Fatalf("notices = %+v", res.Notices)
	}
	if res.Notices[0].FromVersion != 1 || res.Notices[0].Count != 0 {
		t.Fatalf("upgrade notice = %+v", res.Notices[0])
	}
	requireRows(t, res, "w", int64(4), int64(190), int64(300), int64(494))
}

func TestRunResolvesPresetsWithTheReadersDay(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings",
		"params":[{"id":"period","type":"dateRange","default":{"preset":"thisMonth"}}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		"sort":[{"col":"note"}]}`
	requireRows(t, z.run(t, "ann", spec, Options{Today: "2026-01-15"}, nil), "note", "f1", "f3", nil)
	requireRows(t, z.run(t, "ann", spec, Options{Today: "2026-02-15"}, nil), "note", "f2", nil)
	requireRows(t, z.run(t, "ann", spec, Options{Today: "2026-02-15"}, map[string]string{"period": `{"preset":"lastMonth"}`}), "note", "f1", "f3", nil)
}

func TestRunFormulasOnDetailRowsAndTotals(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"double","formula":{"op":"mul","args":[{"col":"cost"},{"value":2}]},"aggregate":"sum"},
		{"id":"perGram","formula":{"op":"div","args":[{"col":"cost"},{"col":"grams"}],"scale":0}},
		{"id":"largest","formula":{"op":"max","args":[{"path":[{"field":"cost"}]}]}}],
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "double", int64(2000), int64(5000), int64(1400), int64(600), int64(9000))
	// A formula with no aggregate is computed again from each row's
	// values: the total's is 4500 / 1700, not a sum of the rows'.
	requireRows(t, res, "perGram", int64(2), int64(3), int64(2), int64(3), int64(3))
	// An aggregate in a formula is the aggregate of one detail row.
	requireRows(t, res, "largest", int64(1000), int64(2500), int64(700), int64(300), int64(2500))
}

// A Bool column's count is a number wherever it is totalled: a group, a
// subtotal or the total, with the groups collapsed or open. Its detail cells
// stay true or false.
func TestACountOfABoolIsANumberOnEveryTotalRow(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{
		Key: "zoo.flags", Collection: "zoo_animals", Label: "Flags",
		Fields: []Field{
			{Key: "species", Label: "Species", Type: Text},
			{Key: "active", Label: "Active", Type: Bool, Aggregates: CountOnly},
		},
	})
	spec := `{"version":1,"source":"zoo.flags","columns":[
		{"id":"species","path":[{"field":"species"}]},
		{"id":"active","path":[{"field":"active"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"species","subtotals":true}],"sort":[{"col":"species"}]}`
	// bob keeps mo (inactive) and zed (active).
	collapsed := z.run(t, "bob", spec, Options{}, nil)
	requireRows(t, collapsed, "kind", "group", "group", "total")
	requireRows(t, collapsed, "active", int64(1), int64(1), int64(2))

	open := z.run(t, "bob", spec, Options{ExpandAll: true}, nil)
	requireRows(t, open, "kind", "group", "detail", "subtotal", "group", "detail", "subtotal", "total")
	requireRows(t, open, "active", int64(1), false, int64(1), int64(1), true, int64(1), int64(2))
	if !reflect.DeepEqual(collapsed.Columns, open.Columns) {
		t.Fatalf("columns differ:\n%+v\n%+v", collapsed.Columns, open.Columns)
	}
}

// The columns' types come from the spec, so a collapsed run (no detail
// statement) describes them as an expanded run does.
func TestColumnTypesDoNotDependOnWhetherRowsLoaded(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}],"aggregate":"count"},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"share","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":1}},
		{"id":"notes","formula":{"path":[{"field":"note"}]},"aggregate":"count"}],
		"groups":[{"id":"g","col":"species"}]}`
	collapsed := z.run(t, "ann", spec, Options{}, nil)
	open := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	if !reflect.DeepEqual(collapsed.Columns, open.Columns) {
		t.Fatalf("columns differ:\n%+v\n%+v", collapsed.Columns, open.Columns)
	}
	want := []ResultColumn{
		{ID: "species", Label: "Species", Type: Text, TotalType: Number, Aggregate: Count},
		{ID: "cost", Label: "Cost", Type: Money, CurrencyCell: "cost@currency", Aggregate: Sum},
		{ID: "share", Label: "share", Type: Percent},
		{ID: "notes", Label: "notes", Type: Text, TotalType: Number, Aggregate: Count},
	}
	if !reflect.DeepEqual(collapsed.Columns, want) {
		t.Fatalf("columns = %+v", collapsed.Columns)
	}
}

func TestValidateCompilesTheSpec(t *testing.T) {
	z := newZoo(t)
	validate := func(raw string) error {
		spec, _, err := Load([]byte(raw))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return Validate(z.app, spec)
	}
	const col = `{"id":"n","path":[{"field":"note"}]}`
	// An unknown field or link inside a filter or a formula names it (422).
	err := validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `],
		"filter":{"op":"gt","args":[{"path":[{"field":"nope"}]},{"value":1}]}}`)
	requireSpecError(t, err, 422, "filter.args.0.path.0.field", "nope")
	err = validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `],
		"filter":{"op":"gt","args":[{"path":[{"link":"nope"},{"field":"x"}]},{"value":1}]}}`)
	requireSpecError(t, err, 422, "filter.args.0.path.0.link", "nope")
	err = validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `,
		{"id":"f","formula":{"op":"add","args":[{"path":[{"field":"nope"}]},{"value":1}]}}]}`)
	requireSpecError(t, err, 422, "columns.1.formula.args.0.path.0.field", "nope")
	err = validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `,
		{"id":"f","formula":{"op":"add","args":[{"path":[{"link":"animal"},{"link":"nope"},{"field":"x"}]},{"value":1}]}}]}`)
	requireSpecError(t, err, 422, "columns.1.formula.args.0.path.1.link", "nope")
	// A formula leaf across a to-many link would repeat the row.
	err = validate(`{"version":1,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]},
		{"id":"f","formula":{"op":"add","args":[{"path":[{"link":"feedings"},{"field":"grams"}]},{"value":1}]}}]}`)
	requireSpecError(t, err, 400, "columns.1.formula.args.0.path.0", "to-many")
	// A formula that cannot compile fails here, not at a run.
	err = validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `,
		{"id":"f","formula":{"op":"runningSum","args":[{"col":"f"}]}}]}`)
	if err == nil || !strings.Contains(err.Error(), "reads itself") {
		t.Fatalf("a formula reading itself: %v", err)
	}
	// A path in a formula with no aggregate is blank on total rows, not an
	// error.
	err = validate(`{"version":1,"source":"zoo.feedings","columns":[` + col + `,
		{"id":"f","formula":{"op":"mul","args":[{"path":[{"field":"cost"}]},{"value":2}]}}]}`)
	if err != nil {
		t.Fatalf("a path in a formula: %v", err)
	}
}

func TestAFormulaPathIsBlankOnGroupAndTotalRows(t *testing.T) {
	z := newZoo(t)
	const columns = `"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"double","formula":{"op":"mul","args":[{"path":[{"field":"cost"}]},{"value":2}]}},
		{"id":"same","formula":{"path":[{"field":"cost"}]}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"perGram","formula":{"op":"div","args":[{"col":"cost"},{"col":"grams"}],"scale":0}}]`
	grouped := z.run(t, "ann", `{"version":1,"source":"zoo.feedings",`+columns+`,
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, grouped, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	// A formula that reads a row's value is blank as a whole on group and
	// total rows, not computed from a blank (which would show 0).
	requireRows(t, grouped, "same", nil, int64(1000), int64(2500), nil, int64(700), int64(300), nil)
	requireRows(t, grouped, "double", nil, int64(2000), int64(5000), nil, int64(1400), int64(600), nil)
	// A formula over totals is still computed for each group.
	requireRows(t, grouped, "perGram", int64(3), int64(2), int64(3), int64(3), int64(2), int64(3), int64(3))
	flat := z.run(t, "ann", `{"version":1,"source":"zoo.feedings",`+columns+`,"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, flat, "same", int64(1000), int64(2500), int64(700), int64(300), nil)

	_, err := z.try(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"bad","formula":{"op":"mul","args":[{"path":[{"field":"nope"}]},{"value":2}]}}]}`, Options{}, nil)
	requireSpecError(t, err, 422, "columns.0.formula.args.0.path.0.field", "nope")
}

func TestTheRowCapNoticeIsGivenOnce(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 1 })
	// Both the group statement (3 rows) and the detail statement (4) pass the cap.
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true}, nil)
	n := 0
	for _, notice := range res.Notices {
		if notice.Code == "rowCap" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// Each cap notice is given once per run, keyed by its code.
func TestNoticeOnceKeepsTheFirstOfACode(t *testing.T) {
	rn := &runner{res: &Result{}}
	rn.noticeOnce(Notice{Code: "rowCap", Count: 1})
	rn.noticeOnce(Notice{Code: "pivotCap", LeftOut: 2})
	rn.noticeOnce(Notice{Code: "rowCap", Count: 3})
	want := []Notice{{Code: "rowCap", Count: 1}, {Code: "pivotCap", LeftOut: 2}}
	if !reflect.DeepEqual(rn.res.Notices, want) {
		t.Fatalf("notices = %+v", rn.res.Notices)
	}
}

func TestGroupKeysCompareByType(t *testing.T) {
	z := newZoo(t)
	if _, err := z.app.DB().NewQuery("UPDATE zoo_animals SET species = '1' WHERE name = 'Zed'").Execute(); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{Expand: [][]any{{"1"}}}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "total")
	text, err := parseExpansion(Options{Expand: [][]any{{"1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !text.has([]any{"1"}) || text.has([]any{int64(1)}) {
		t.Fatal("the text \"1\" and the number 1 are different group keys")
	}
}
