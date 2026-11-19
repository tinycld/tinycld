package reports

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// requireSameAsLegacy runs a report with one change of the performance work
// off (*off) and then on, and requires the same result, byte for byte.
func requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool) {
	t.Helper()
	t.Cleanup(func() { *off = false })
	*off = true
	before, err := json.Marshal(z.run(t, user, spec, opts, params))
	if err != nil {
		t.Fatal(err)
	}
	*off = false
	after, err := json.Marshal(z.run(t, user, spec, opts, params))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("the result changed\nbefore: %s\nafter:  %s", before, after)
	}
}

// blankZeroMeals shows a kind's cost, blank when it is 0: the formula reads
// one sum two times.
const blankZeroMeals = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"paid","formula":{"op":"if","args":[
		{"op":"eq","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]},
		{"value":null},
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
	"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`

// levelAggregates finds a<L>, the aggregates of a level, in a statement.
var levelAggregates = regexp.MustCompile(`(?m)^a(\d+) AS \((.*)$`)

// requireInEachLevel requires text n times in every a<L> of every statement.
func requireInEachLevel(t *testing.T, seen []readStatement, text string, n int) {
	t.Helper()
	levels := 0
	for _, st := range seen {
		for _, m := range levelAggregates.FindAllStringSubmatch(st.sql, -1) {
			levels++
			if got := strings.Count(m[2], text); got != n {
				t.Errorf("a%s has %q %d times, want %d:\n%s", m[1], text, got, n, m[2])
			}
		}
	}
	if levels == 0 {
		t.Fatal("no statement has a level's aggregates")
	}
}

func TestAnAggregateALevelReadsTwiceIsComputedOnce(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", blankZeroMeals, Options{}, nil)
	requireRows(t, res, "paid", int64(4500), int64(65), int64(4565))
	requireInEachLevel(t, *seen, "sum(", 1)
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.repeatAggregates = true
	t.Cleanup(func() { legacySQL.repeatAggregates = false })
	*seen = nil
	z.run(t, "ann", blankZeroMeals, Options{}, nil)
	requireInEachLevel(t, *seen, "sum(", 2)
}

// A sum of a lookup whose where is unknown (lookupwheremixed_test.go), read
// two times: the notice names the where at both reads.
func TestAnAggregateReadTwiceNamesAnUnknownWhereAtEachRead(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"fed","formula":{"op":"if","args":[
			{"op":"eq","args":[{"op":"sum","args":[` + unknownWhere + `]},{"value":0}]},
			{"value":null},
			{"op":"sum","args":[` + unknownWhere + `]}]}}],"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "columns.2.formula.args.0.args.0.args.0.where", "columns.2.formula.args.2.args.0.where")
	requireSameAsLegacy(t, z, "bob", spec, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
}

func TestAnAggregateComputedOnceKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		requireSameAsLegacy(t, z, user, blankZeroMeals, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
	}
}

// differingMeals reads one argument through two operators and one operator
// at two scales: none of the four is another's value.
const differingMeals = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"most","formula":{"op":"max","args":[{"path":[{"field":"grams"}]}]}},
	{"id":"all","formula":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}},
	{"id":"mean0","formula":{"op":"avg","scale":0,"args":[{"path":[{"field":"grams"}]}]}},
	{"id":"mean2","formula":{"op":"avg","scale":2,"args":[{"path":[{"field":"grams"}]}]}}],
	"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`

func TestAggregatesThatDifferStayApart(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", differingMeals, Options{}, nil)
	// ann's feedings weigh 500, 800, 300 and 100 grams; a treat has none.
	for id, want := range map[string]string{"most": "[800 <nil> 800]", "all": "[1700 0 1700]", "mean0": "[425 <nil> 425]", "mean2": "[425 <nil> 425]"} {
		if got := fmt.Sprint(rowsOf(res, id)); got != want {
			t.Errorf("%s is %s, want %s", id, got, want)
		}
	}
	// One average for each scale, one max: the two averages are not merged.
	requireInEachLevel(t, *seen, "tc_div(", 2)
	requireInEachLevel(t, *seen, "max(", 1)
	requireSameAsLegacy(t, z, "ann", differingMeals, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
}

// mealsOfPet sums a pet's meals from the union: a lookup inside an aggregate.
const mealsOfPet = `{"op":"lookup","source":"zoo.meals","pick":"sum",
	"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
	"value":{"path":[{"field":"cost"}]}}`

const lookupTwice = `{"version":4,"source":"zoo.pets","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"pet","path":[{"field":"id"}]},
	{"id":"fed","formula":{"op":"if","args":[
		{"op":"eq","args":[{"op":"sum","args":[` + mealsOfPet + `]},{"value":0}]},
		{"value":null},
		{"op":"sum","args":[` + mealsOfPet + `]}]}}],
	"groups":[{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`

func TestAnAggregateOfALookupReadTwiceKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", lookupTwice, Options{}, nil)
	requireRows(t, res, "fed", int64(40), int64(3525), int64(1000), int64(4565))
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		requireSameAsLegacy(t, z, user, lookupTwice, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
	}
}

// A column a lookup's value reads is a column the aggregate reads: the memo
// must not hide a self-read from the compile that refuses it.
func TestColumnsReadSeesAColumnALookupsValueReads(t *testing.T) {
	p := &plan{byID: map[string]*colPlan{}, lookups: map[*Node]decodedLookup{}}
	look := &Node{Op: "lookup", Value: json.RawMessage(`{"col":"k"}`)}
	read := map[string]bool{}
	p.columnsRead(&Node{Op: "sum", Args: []*Node{look}}, read)
	if !read["k"] {
		t.Fatalf("columnsRead = %v, want k", read)
	}
}
