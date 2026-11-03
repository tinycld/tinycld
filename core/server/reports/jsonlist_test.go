package reports

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// manyReads is animalsParam with n formula columns that each read the param.
func manyReads(n int) string {
	cols := []string{`{"id":"note","path":[{"field":"note"}]}`}
	for i := range n {
		cols = append(cols, fmt.Sprintf(`{"id":"in%d","formula":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]}}`, i))
	}
	return `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"animals","type":"choice","label":"Animals","multiple":true,
		"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]}}],
	"columns":[` + strings.Join(cols, ",") + `],"sort":[{"col":"note"}]}`
}

// fullList is the most values a choice list holds: Leo, then values that
// match nothing.
func fullList(z *zoo, groups ...string) string {
	items := append([]string{}, groups...)
	items = append(items, `"`+z.id("leo")+`"`)
	for i := 1; i < maxChoiceValues; i++ {
		items = append(items, fmt.Sprintf(`"x%d"`, i))
	}
	return "[" + strings.Join(items, ",") + "]"
}

// A choice list binds one parameter however long it is and however often a
// report reads it: 40 reads of 1,000 values would bind 40,000, past SQLite's
// 32,766 variables.
func TestALongChoiceListReadManyTimesRuns(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", manyReads(40), Options{}, map[string]string{"animals": fullList(z)})
	requireRows(t, res, "note", "f1", "f2", "f3", "f4", nil)
	requireRows(t, res, "in39", true, true, false, false, nil)
}

// A group reference beside a full list of values, read many times.
func TestALongGroupReferenceListReadManyTimesRuns(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", manyReads(40), Options{}, map[string]string{"animals": fullList(z, `{"group":"zebra"}`)})
	requireRows(t, res, "in0", true, true, true, true, nil)
	requireRows(t, res, "in39", true, true, true, true, nil)
}

// fourLevels groups feedings four deep; the last level is a link that fills
// in every animal the reader may list, so a run reads the expanded keys of
// level 3 twice: for the level's rows and for its fill's parents.
const fourLevels = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g0","col":"species"},{"id":"g1","col":"name"},{"id":"g2","col":"note"},
		{"id":"g3","col":"animal","fill":{}}]}`

// Thousands of expanded keys bind one parameter per read: 5,000 keys of
// four values, read twice, would bind 40,000.
func TestThousandsOfExpandedKeysRun(t *testing.T) {
	z := newZoo(t)
	open := [][]any{{"lion", "USD"}, {"lion", "Leo", "USD"}, {"lion", "Leo", "f1", "USD"}}
	want := z.run(t, "ann", fourLevels, Options{Expand: open}, nil)
	keys := append([][]any{}, open...)
	for i := len(keys); i < runLimits.rowCap; i++ {
		keys = append(keys, []any{"lion", "Leo", fmt.Sprintf("x%d", i), "USD"})
	}
	got := z.run(t, "ann", fourLevels, Options{Expand: keys}, nil)
	if !reflect.DeepEqual(got.Rows, want.Rows) {
		t.Fatalf("rows = %+v\nwant   %+v", got.Rows, want.Rows)
	}
	// The fill adds every animal Ann may list under f1: Kit, Leo and Zed.
	requireRows(t, got, "label", "lion", "Leo", "f1", "Kit", "Leo", "Zed", "f2", "zebra", "Total")
}

// An expanded key matches as SQLite's IS compares the key with the group
// value, which keeps its type: the number 500 opens the group 500, the text
// "800" does not open the number group 800, and a blank key opens a blank
// group, level by level.
func TestExpandedKeysOfMixedTypes(t *testing.T) {
	z := newZoo(t)
	byGrams := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"grams","path":[{"field":"grams"}]},{"id":"note","path":[{"field":"note"}]}],
		"groups":[{"id":"g","col":"grams"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", byGrams, Options{Expand: [][]any{{float64(500)}, {"800"}, {300.5}, {true}}}, nil)
	requireRows(t, res, "note", nil, nil, nil, "f1", nil, nil)

	byRegion := `{"version":1,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"species","path":[{"field":"species"}]},
		{"id":"name","path":[{"field":"name"}]}],
		"groups":[{"id":"g0","col":"region"},{"id":"g1","col":"species"}],"sort":[{"col":"name"}]}`
	res = z.run(t, "ann", byRegion, Options{Expand: [][]any{{nil}, {nil, "cat"}, {"north"}, {"north", "lion"}, {"south", "zebra"}}}, nil)
	// South is not expanded, so its zebra key opens nothing.
	requireRows(t, res, "label", "North", "lion", "North", "South", "(none)", "cat", "(none)", "Total")
	requireRows(t, res, "name", nil, nil, "Leo", nil, nil, nil, "Kit", nil)
}

// Keys with more patterns of blanks than SQLite parses ORs in a chain (1,000)
// still compile, and each pattern matches only its own rows: 2,048 keys over
// a group key and ten currency keys, one per pattern.
func TestExpandedKeysWithManyPatternsOfBlanks(t *testing.T) {
	z := newZoo(t)
	cols := []string{"k0"}
	c := &compiler{}
	for i := range 10 {
		c.currencies = append(c.currencies, fmt.Sprintf("ck%d", i))
	}
	cols = append(cols, c.currencies...)
	keys := make([][]any, 0, 1<<len(cols))
	for mask := range 1 << len(cols) {
		key := make([]any, len(cols))
		for j := range cols {
			if mask&(1<<j) == 0 {
				key[j] = "v"
			}
		}
		keys = append(keys, key)
	}
	e, err := parseExpansion(Options{Expand: keys})
	if err != nil {
		t.Fatal(err)
	}
	s := newStmt(z.app, nil)
	where := e.where(c, s, 1, "")
	all := strings.TrimSuffix(strings.Repeat("'v', ", len(cols)), ", ")
	none := strings.TrimSuffix(strings.Repeat("NULL, ", len(cols)), ", ")
	mixed := "NULL, " + strings.TrimSuffix(strings.Repeat("'v', ", len(cols)-1), ", ")
	other := "'w', " + strings.TrimSuffix(strings.Repeat("'v', ", len(cols)-1), ", ")
	sql := fmt.Sprintf(`WITH t(%s) AS (VALUES (%s), (%s), (%s), (%s)) SELECT count(*) AS n FROM t WHERE %s`,
		strings.Join(cols, ", "), all, none, mixed, other, where)
	var row struct {
		N int `db:"n"`
	}
	if err := z.app.DB().NewQuery(sql).Bind(s.params).One(&row); err != nil {
		t.Fatal(err)
	}
	if row.N != 3 {
		t.Fatalf("matched %d rows, want 3", row.N)
	}
}

// A select's options bind as one parameter per read, however many there
// are: 20,003 options would bind 40,006 values in a CASE. A value is looked
// up whole, whatever characters it holds, and the first option with a value
// names it, as CASE takes the first WHEN that matches.
func TestASelectWithManyOptionsShowsItsLabels(t *testing.T) {
	z := newZoo(t)
	odd := `a"b.c[0] \ é`
	menu := core.NewBaseCollection("zoo_menu")
	menu.ListRule = rule(`@request.auth.id != ""`)
	menu.Fields.Add(&core.TextField{Name: "note"}, &core.SelectField{Name: "kind", Values: []string{"apple", "zebra", odd}, MaxSelect: 1})
	z.save(t, menu)
	z.rec(t, "m1", "zoo_menu", map[string]any{"note": "m1", "kind": "apple"})
	z.rec(t, "m2", "zoo_menu", map[string]any{"note": "m2", "kind": "zebra"})
	z.rec(t, "m3", "zoo_menu", map[string]any{"note": "m3", "kind": odd})
	options := []Option{{Value: "apple", Label: "Zest"}}
	for i := range 20000 {
		options = append(options, Option{Value: fmt.Sprintf("x%d", i), Label: fmt.Sprintf("X %d", i)})
	}
	options = append(options, Option{Value: "zebra", Label: "Apricot"}, Option{Value: odd, Label: "Odd"}, Option{Value: "apple", Label: "Later"})
	RegisterSource(Source{
		Key: "zoo.menu", Collection: "zoo_menu", Label: "Menu",
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text, Aggregates: CountOnly},
			{Key: "kind", Label: "Kind", Type: Select, Options: options},
		},
	})
	grouped := z.run(t, "ann", `{"version":2,"source":"zoo.menu","columns":[{"id":"kind","path":[{"field":"kind"}]},
		{"id":"n","path":[{"field":"note"}],"aggregate":"count"}],"groups":[{"id":"g","col":"kind"}]}`, Options{}, nil)
	requireRows(t, grouped, "label", "Apricot", "Odd", "Zest", "Total")
	shown := z.run(t, "ann", `{"version":2,"source":"zoo.menu","columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"low","formula":{"op":"lower","args":[{"path":[{"field":"kind"}]}]}}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, shown, "low", "zest", "apricot", "odd", nil)
}

// A list compares as a list of bound values does: the left side's affinity
// applies to each value, so a number matches a text field that holds its
// digits, and text matches a number field.
func TestAListComparesAsBoundValues(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5, "currency": "USD", "grams": 7, "note": "500"})
	spec := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"byNote","formula":{"op":"in","args":[{"path":[{"field":"note"}]},{"value":[500,"f1"]}]}},
		{"id":"byGrams","formula":{"op":"in","args":[{"path":[{"field":"grams"}]},{"value":["800","7.0",1e400]}]}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "note", "500", "f1", "f2", "f3", "f4", nil)
	requireRows(t, res, "byNote", true, true, false, false, false, nil)
	requireRows(t, res, "byGrams", true, false, true, false, false, nil)
}

// sqlJSON writes each value as JSON that SQLite reads back as the value a
// bound parameter holds: the same type and, for a number, the same bits.
func TestJSONValueReadsBackAsTheBoundValue(t *testing.T) {
	z := newZoo(t)
	values := []any{nil, true, false, "", "f1", `quote " and \ and ' and ]`, "日本", "a\x00b",
		int64(0), int64(-7), int64(math.MaxInt64), int64(math.MinInt64),
		0.5, -0.1, 500.0, 1e21, 1e-7, 5e-324, math.MaxFloat64, math.Inf(1), math.Inf(-1), math.NaN()}
	for _, v := range values {
		text, err := sqlJSON(v)
		if err != nil {
			t.Fatalf("sqlJSON(%v): %v", v, err)
		}
		var row struct {
			Same bool   `db:"same"`
			Type string `db:"type"`
			Want string `db:"want"`
		}
		err = z.app.DB().NewQuery(`SELECT json_extract({:json}, '$[0]') IS {:v} AS same,
			typeof(json_extract({:json}, '$[0]')) AS type, typeof({:v}) AS want`).
			Bind(dbx.Params{"json": "[" + text + "]", "v": v}).One(&row)
		if err != nil {
			t.Fatalf("%v: %v", v, err)
		}
		if !row.Same || row.Type != row.Want {
			t.Errorf("%#v as %s reads back as %s (same %v), bound as %s", v, text, row.Type, row.Same, row.Want)
		}
	}
	if _, err := sqlJSON([]int{1}); err == nil {
		t.Fatal("sqlJSON took a slice")
	}
}
