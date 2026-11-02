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
	// Opened, with its subtotal: every row under the hidden record reads it
	// blank too.
	sub := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"vault","path":[{"link":"vault"}]},
		{"id":"vname","formula":{"record":"g","path":[{"field":"name"}]}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"vault","subtotals":true}]}`, Options{Expand: [][]any{res.Rows[0].GroupKey}}, nil)
	requireRows(t, sub, "kind", "group", "detail", "detail", "detail", "subtotal", "total")
	requireRows(t, sub, "label", "(none)", "(none)", "(none)", "(none)", "Total (none)", "Total")
	requireRows(t, sub, "vname", nil, nil, nil, nil, nil, nil)
}

// A visible record read through a link to a row the reader cannot see reads
// blank: leo's vault is hidden, zed has none.
func TestARecordFieldPastAHiddenLinkIsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"vault","formula":{"record":"g","path":[{"link":"vault"},{"field":"name"}]}},
		{"id":"site","formula":{"record":"g","path":[{"link":"site"},{"field":"name"}]}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Leo", "Total")
	requireRows(t, res, "site", "South", "North", nil)
	requireRows(t, res, "vault", nil, nil, nil)
}

// Two records with the same label are two groups. With no sort key a record
// group reads in label order, then by record id.
func TestRecordGroupsReadInLabelOrder(t *testing.T) {
	z := newZoo(t)
	twin := z.rec(t, "leo2", "zoo_animals", map[string]any{"name": "Leo", "species": "lion", "owner": z.id("ann"), "weight": 150})
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": twin.Id})
	z.feeding(t, "f7", twin, 50, "2026-04-01", 10)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Leo", "Zed", "Total")
	first, second := z.id("leo"), z.id("leo2")
	if second < first {
		first, second = second, first
	}
	if res.Rows[0].GroupKey[0] != first || res.Rows[1].GroupKey[0] != second {
		t.Fatalf("Leo keys = %v, %v; want %s then %s", res.Rows[0].GroupKey, res.Rows[1].GroupKey, first, second)
	}
}

// An outer group's rows hold many records of an inner group, so its label
// cannot read one, directly or through a column it reads.
func TestAnOuterLabelCannotReadAnInnerRecord(t *testing.T) {
	app := withZooRegistry(t)
	const cols = `"columns":[{"id":"site","path":[{"link":"animal"},{"link":"site"}]},{"id":"animal","path":[{"link":"animal"}]},
		{"id":"species","formula":{"record":"a","path":[{"field":"species"}]}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}]`
	cases := map[string]struct{ label, at string }{
		"directly":         {`{"record":"a","path":[{"field":"name"}]}`, "groups.0.label"},
		"in a concat":      {`{"op":"concat","args":[{"value":"x"},{"record":"a","path":[{"field":"name"}]}]}`, "groups.0.label.args.1"},
		"through a column": {`{"col":"species"}`, "groups.0.label"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":2,"source":"zoo.feedings",` + cols + `,"groups":[{"id":"s","col":"site","label":` + c.label + `},{"id":"a","col":"animal"}]}`
			requireSpecError(t, Validate(app, loadSpec(t, raw)), 400, c.at, "which is grouped inside it")
		})
	}
	// Inside a total, each base row has its record: the label may total them.
	total := `{"version":2,"source":"zoo.feedings",` + cols + `,"groups":[{"id":"s","col":"site","label":
		{"op":"countDistinct","args":[{"record":"a","path":[{"field":"species"}]}]}},{"id":"a","col":"animal"}]}`
	if err := Validate(app, loadSpec(t, total)); err != nil {
		t.Fatalf("a total of inner records: %v", err)
	}
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
	// The label is an amount, so it is sent typed with its currency (R9).
	for i, want := range []int64{1000, 3500} {
		row := res.Rows[i]
		if row.LabelType != Money || row.LabelValue != want || row.LabelCurrency != "USD" {
			t.Fatalf("row %d label = %v %v %q", i, row.LabelType, row.LabelValue, row.LabelCurrency)
		}
	}
	if total := res.Rows[2]; total.LabelType != "" || total.LabelValue != nil {
		t.Fatalf("total label = %+v", total)
	}
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
