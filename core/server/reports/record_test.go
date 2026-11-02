package reports

import (
	"strings"
	"testing"
)

const byAnimal = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"species","formula":{"record":"g","path":[{"field":"species"}]}},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`

// A group by a link carries the linked record: it labels the row by the
// record's first text field and a record leaf reads its fields on the group,
// on the rows below it, and nowhere outside it.
func TestARecordGroupLabelsItsRowsAndReadsTheRecord(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", byAnimal, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Leo", "Total")
	requireRows(t, res, "species", "zebra", "lion", nil)
	requireRows(t, res, "cost", int64(1000), int64(3500), int64(4500))
	if res.Rows[0].GroupKey[0] != z.id("zed") {
		t.Fatalf("group key = %v", res.Rows[0].GroupKey)
	}
	open := z.run(t, "ann", byAnimal, Options{Expand: [][]any{res.Rows[1].GroupKey}}, nil)
	requireRows(t, open, "species", "zebra", "lion", "lion", "lion", nil)
}

func TestAGroupLabelIsAFormula(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"op":"concat","args":[
			{"record":"g","path":[{"field":"name"}]},{"value":" at "},{"record":"g","path":[{"link":"site"},{"field":"name"}]}]}}],
		"sort":[{"col":"cost"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed at South", "Leo at North", "Total")
}

// A record the reader cannot see is a blank key and a blank label.
func TestARecordTheReaderCannotSeeIsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"vault","path":[{"link":"vault"}]},
		{"id":"vname","formula":{"record":"g","path":[{"field":"name"}]}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"vault"}]}`, Options{}, nil)
	requireRows(t, res, "label", "(none)", "Total")
	requireRows(t, res, "vname", nil, nil)
}

func TestARecordLeafIsRefusedWhereItCannotBeRead(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct {
		spec, at, want string
		status         int
	}{
		"unknown group": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"record":"nope","path":[{"field":"name"}]}}]}`,
			"columns.0.formula", "has no group", 422},
		"not a record group": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"x","formula":{"record":"g","path":[{"field":"name"}]}}],"groups":[{"id":"g","col":"n"}]}`,
			"columns.1.formula", "is not grouped by a link", 400},
		"in the filter": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],"groups":[{"id":"g","col":"a"}],
			"filter":{"op":"eq","args":[{"record":"g","path":[{"field":"name"}]},{"value":"Leo"}]}}`,
			"filter.args.0", "before they are grouped", 400},
		"no path": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},{"id":"x","formula":{"record":"g"}}],"groups":[{"id":"g","col":"a"}]}`,
			"columns.1.formula", "names a field of the record", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}
}

// A label that totals an amount groups by its currency before any level does
// (F5), though no column reads the amount.
func TestALabelThatTotalsAnAmountGroupsByItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}],
		"sort":[{"col":"grams"}]}`, Options{}, nil)
	requireRows(t, res, "label", "1000", "3500", "Total")
}

// Validate checks a group label as it checks a formula column: its size with
// the columns it reads, and its lookups' values.
func TestValidateChecksAGroupLabel(t *testing.T) {
	app := withZooRegistry(t)
	big := strings.TrimSuffix(chainSpec(8, doubling), "}") + `,"groups":[{"id":"g","col":"c0","label":
		{"op":"concat","args":[{"col":"c8"},{"col":"c8"}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, big)), 400, "groups.0.label", "with the formula columns it reads")
	bad := `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],
		"groups":[{"id":"g","col":"a","label":{"op":"lookup","source":"zoo.sites","value":{"nope":1}}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, bad)), 400, "groups.0.label.value", "a lookup's value is a formula")
}

// Under two record groups, each leaf reads its own group's record: the outer
// one on its rows and the inner rows below them, the inner one on its rows
// only.
func TestNestedRecordGroupsReadTheirOwnRecords(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"sname","formula":{"record":"s","path":[{"field":"name"}]}},
		{"id":"species","formula":{"record":"g","path":[{"field":"species"}]}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"site"},{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "South", "North", "Total")
	requireRows(t, res, "sname", "South", "North", nil)
	requireRows(t, res, "species", nil, nil, nil)
	open := z.run(t, "ann", spec, Options{Expand: [][]any{res.Rows[1].GroupKey}}, nil)
	requireRows(t, open, "label", "South", "North", "Leo", "Total")
	requireRows(t, open, "sname", "South", "North", "North", nil)
	requireRows(t, open, "species", nil, nil, "lion", nil)
}

// A lookup reads the row's group record through the row, as it reads a
// column: on the group rows and the rows below them.
func TestALookupReadsTheGroupRecord(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"kg","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"weight"}]},
			"where":{"op":"eq","args":[{"path":[{"field":"species"}]},{"record":"g","path":[{"field":"species"}]}]}}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kg", int64(300), int64(190), nil)
	open := z.run(t, "ann", spec, Options{Expand: [][]any{res.Rows[0].GroupKey}}, nil)
	requireRows(t, open, "kg", int64(300), int64(300), int64(300), int64(190), nil)
}
