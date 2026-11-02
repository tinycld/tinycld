package reports

import "testing"

// withEmptyAnimals adds a site and two of ann's animals with no feeding: Lia
// (lion, at North) and Eli (elk, at East).
func withEmptyAnimals(t *testing.T, z *zoo) {
	t.Helper()
	east := z.rec(t, "east", "zoo_sites", map[string]any{"name": "East", "region": "north"})
	z.rec(t, "lia", "zoo_animals", map[string]any{"name": "Lia", "species": "lion", "site": z.id("north"), "owner": z.id("ann"), "active": true})
	z.rec(t, "eli", "zoo_animals", map[string]any{"name": "Eli", "species": "elk", "site": east.Id, "owner": z.id("ann"), "active": true})
}

func animalFill(fill, filter string) string {
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"animal","fill":` + fill + `}],"sort":[{"col":"cost"},{"formula":{"record":"g","path":[{"field":"name"}]}}]`
	if filter != "" {
		spec += `,"filter":` + filter
	}
	return spec + "}"
}

// Fill adds the target's records that have no rows, under the reader's rules:
// ann sees eli, kit and lia (active, no feeding she may read); mo is bob's.
func TestFillAddsRecordsWithNoRows(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", animalFill(`{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`, ""), Options{}, nil)
	requireRows(t, res, "label", "Eli", "Kit", "Lia", "Zed", "Leo", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(0), int64(1000), int64(3500), int64(4500))
	requireRows(t, res, "n", int64(0), int64(0), int64(0), int64(2), int64(2), int64(4))
	if res.Rows[0].CanExpand || !res.Rows[3].CanExpand {
		t.Fatalf("a fill row with no deeper fill has nothing to open: %+v", res.Rows[:4])
	}
}

// With no rows at all the first level's fill records sit under the no-currency total.
func TestFillWithNoRowsSitsUnderOneTotal(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", animalFill(`{}`, `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]}`), Options{}, nil)
	if last := res.Rows[len(res.Rows)-1]; last.ID != "total:[null]" || len(res.Rows) != 6 {
		t.Fatalf("rows = %+v", res.Rows)
	}
}

const sitesThenAnimals = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[
		{"id":"s","col":"site","fill":{}},
		{"id":"a","col":"animal","fill":{"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]},
			{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]}]}}}],
	"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}},{"formula":{"record":"a","path":[{"field":"name"}]}}]}`

// A deeper fill adds, under each parent, the records its where ties to that
// parent; a parent that is itself a fill record opens on them.
func TestFillOnADeeperLevelSitsUnderEachParent(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	// Open the sites, not the animals, so no feeding rows show.
	sites := [][]any{{z.id("east"), "USD"}, {z.id("north"), "USD"}, {z.id("south"), "USD"}}
	res := z.run(t, "ann", sitesThenAnimals, Options{Expand: sites}, nil)
	requireRows(t, res, "label", "East", "Eli", "North", "Leo", "Lia", "South", "Zed", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(3500), int64(3500), int64(0), int64(1000), int64(1000), int64(4500))
	if !res.Rows[0].CanExpand || res.Rows[1].CanExpand {
		t.Fatalf("east opens on its fill record, which opens on nothing: %+v", res.Rows[:2])
	}
}

// R6: a fill record appears under every currency the report shows; leo has
// no EUR feeding, so it is a fill record under the EUR total.
func TestFillSitsUnderEveryCurrency(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("zed"), "cost": 200, "currency": "EUR", "fed": "2026-04-01 00:00:00.000Z", "note": "f7"})
	res := z.run(t, "ann", animalFill(`{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`, ""), Options{}, nil)
	requireRows(t, res, "label", "Eli", "Kit", "Leo", "Lia", "Zed", "Total", "Eli", "Kit", "Lia", "Zed", "Leo", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(0), int64(0), int64(200), int64(200), int64(0), int64(0), int64(0), int64(1000), int64(3500), int64(4500))
	requireRows(t, res, "cost@currency", "EUR", "EUR", "EUR", "EUR", "EUR", "EUR", "USD", "USD", "USD", "USD", "USD", "USD")
}

// A fill record's totals are those of no rows, whatever they read: a to-many
// count is 0 (not blank), a count of a literal is 0, a min is blank.
func TestFillRowsTotalNoRows(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"keepers","path":[{"link":"animal"},{"link":"keepers","mode":"aggregate"},{"field":"id"}],"aggregate":"count"},
		{"id":"ones","formula":{"value":1},"aggregate":"count"},
		{"id":"first","path":[{"field":"fed"}],"aggregate":"min"}],
		"groups":[{"id":"g","col":"animal","fill":{"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Lia"}]}}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "keepers", int64(1), int64(0), int64(1), int64(2))
	requireRows(t, res, "ones", int64(2), int64(0), int64(2), int64(4))
	requireRows(t, res, "first", "2026-01-05", nil, "2026-01-20", "2026-01-05")
}

// A fill's where reads an enclosing group's record and params; a where on an
// unset param fills in every record the reader may list.
func TestFillWhereReadsTheParentRecordAndParams(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	spec := `{"version":2,"source":"zoo.feedings","params":[{"id":"sp","type":"text"}],"columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[
			{"id":"s","col":"site"},
			{"id":"a","col":"animal","fill":{"where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"site"},{"field":"name"}]},{"record":"s","path":[{"field":"name"}]}]},
				{"op":"eq","args":[{"path":[{"field":"species"}]},{"param":"sp"}]}]}}}],
		"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}},{"formula":{"record":"a","path":[{"field":"name"}]}}]}`
	sites := Options{Expand: [][]any{{z.id("north"), "USD"}, {z.id("south"), "USD"}}}
	all := z.run(t, "ann", spec, sites, nil)
	requireRows(t, all, "label", "North", "Leo", "Lia", "South", "Zed", "Total")
	zebras := z.run(t, "ann", spec, sites, map[string]string{"sp": `"zebra"`})
	requireRows(t, zebras, "label", "North", "Leo", "South", "Zed", "Total")
}

// Under a bucketed group the parent's key is the bucket's text.
func TestFillWhereReadsABucketKey(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"fed","path":[{"field":"fed"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[
			{"id":"m","col":"fed","bucket":"month"},
			{"id":"a","col":"animal","fill":{"where":{"op":"eq","args":[{"col":"fed"},{"value":"2026-03"}]}}}],
		"sort":[{"formula":{"record":"a","path":[{"field":"name"}]}}]}`,
		Options{Expand: [][]any{{"2026-01", "USD"}, {"2026-02", "USD"}, {"2026-03", "USD"}}}, nil)
	requireRows(t, res, "label", "2026-01", "Leo", "Zed", "2026-02", "Leo", "2026-03", "Eli", "Kit", "Leo", "Lia", "Zed", "Total")
}

func TestFillIsRefusedWhereItCannotWork(t *testing.T) {
	app := withZooRegistry(t)
	notRecord := `{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"n","fill":{}}]}`
	ownColumn := `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"a","fill":{"where":{"op":"gt","args":[{"col":"c"},{"value":0}]}}}]}`
	for _, c := range []struct{ spec, at, want string }{
		{notRecord, "groups.0.fill", "only a group by a link"},
		{ownColumn, "groups.0.fill.where.args.0", "the keys of the groups above it"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
