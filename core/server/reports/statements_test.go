package reports

import "testing"

// zooBalance is a statement with sections, ragged levels and computed rows
// inside a section, on the zoo: felines / grazers / others by a hidden side column; class by site
// region and kind by species, both ragged and hiding empty groups; animals
// with a having; "Earlier years" and "This year" inside the last section with
// a cell per day column; a total of the two later sections; derived Change and
// percent columns.
func zooBalance() string {
	cur := `{"op":"coalesce","args":[{"op":"sum","args":[{"op":"if","args":[{"op":"lte","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]},{"path":[{"field":"cost"}]},{"value":null}]}]},{"value":0}]}`
	pri := `{"op":"coalesce","args":[{"op":"sum","args":[{"op":"if","args":[{"op":"lte","args":[{"path":[{"field":"fed"}]},` + yearBefore + `]},{"path":[{"field":"cost"}]},{"value":null}]}]},{"value":0}]}`
	species := `{"path":[{"link":"animal"},{"field":"species"}]}`
	region := `{"path":[{"link":"animal"},{"link":"site"},{"field":"region"}]}`
	is := func(leaf, v string) string { return `{"op":"eq","args":[` + leaf + `,{"value":"` + v + `"}]}` }
	// earlier and thisYear are Retained earnings and Net income: feedings on or
	// before day in a fiscal year before day's, and in day's.
	byYear := func(day, cmp string) string {
		return `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","where":{"op":"and","args":[
			{"op":"lte","args":[{"path":[{"field":"fed"}]},` + day + `]},
			{"op":"` + cmp + `","args":[{"op":"fiscalYear","args":[{"path":[{"field":"fed"}]}]},{"op":"fiscalYear","args":[` + day + `]}]}]},
			"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}`
	}
	asOf := `{"param":"asOf"}`
	return `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"asOf","type":"asOf","label":"As of"}],
	"columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cur","heading":` + asOf + `,"formula":` + cur + `},
		{"id":"pri","heading":` + yearBefore + `,"formula":` + pri + `},
		{"id":"chg","formula":{"op":"sub","args":[{"col":"cur"},{"col":"pri"}]}},
		{"id":"pct","format":"percent","formula":{"op":"if","args":[{"op":"eq","args":[{"col":"pri"},{"value":0}]},{"value":null},
			{"op":"mul","args":[{"op":"div","scale":4,"args":[{"col":"chg"},{"op":"max","args":[{"col":"pri"},{"op":"sub","args":[{"value":0},{"col":"pri"}]}]}]},{"value":100}]}]}},
		{"id":"side","hidden":true,"formula":{"op":"case","args":[
			{"op":"or","args":[` + is(species, "lion") + `,` + is(species, "cat") + `]},{"value":"felines"},
			{"op":"or","args":[` + is(species, "zebra") + `,` + is(species, "tapir") + `]},{"value":"grazers"},
			{"value":"others"}]}},
		{"id":"cls","hidden":true,"formula":{"op":"case","args":[` + is(region, "north") + `,{"value":1},` + is(region, "south") + `,{"value":2},{"value":null}]}},
		{"id":"knd","hidden":true,"formula":{"op":"case","args":[` + is(species, "lion") + `,{"value":1},` + is(species, "zebra") + `,{"value":2},` + is(species, "tapir") + `,{"value":3},{"value":null}]}}],
	"groups":[
		{"id":"section","col":"side","open":true,"subtotals":true},
		{"id":"class","col":"cls","blank":"skip","hideEmpty":true,"open":true,"subtotals":true,
			"label":{"op":"case","args":[{"op":"eq","args":[{"col":"cls"},{"value":1}]},{"value":"North wing"},{"value":"South wing"}]}},
		{"id":"kind","col":"knd","blank":"skip","hideEmpty":true,"open":true,"subtotals":true,
			"label":{"op":"case","args":[{"op":"eq","args":[{"col":"knd"},{"value":1}]},{"value":"Big cats"},{"op":"eq","args":[{"col":"knd"},{"value":2}]},{"value":"Stripes"},{"value":"Tapirs"}]}},
		{"id":"byAnimal","col":"animal","having":{"op":"or","args":[{"op":"ne","args":[{"col":"cur"},{"value":0}]},{"op":"ne","args":[{"col":"pri"},{"value":0}]}]}}],
	"layout":[
		{"section":"felines","title":"Felines","key":"felines"},
		{"section":"grazers","title":"Grazers","key":"grazers"},
		{"section":"others","title":"Others","key":"others"},
		{"row":"earlier","label":"Earlier years","within":"others","cells":{"cur":` + byYear(asOf, "lt") + `,"pri":` + byYear(yearBefore, "lt") + `}},
		{"row":"thisYear","label":"This year","within":"others","cells":{"cur":` + byYear(asOf, "gte") + `,"pri":` + byYear(yearBefore, "gte") + `}},
		{"row":"later","label":"Grazers and others","formula":{"op":"add","args":[{"subtotal":"grazers"},{"subtotal":"others"}]}}],
	"sort":[{"col":"cls"},{"col":"knd"},{"formula":{"record":"byAnimal","path":[{"field":"name"}]}}]}`
}

func balanceZoo(t *testing.T) *zoo {
	z := newZoo(t)
	ann := z.id("ann")
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": ann, "animal": z.id("kit")})
	z.rec(t, "k6", "zoo_keepers", map[string]any{"user": ann, "animal": z.id("mo")})
	dud := z.rec(t, "dud", "zoo_animals", map[string]any{"name": "Dud", "species": "lion", "site": z.id("north"), "owner": ann})
	tap := z.rec(t, "tap", "zoo_animals", map[string]any{"name": "Tap", "species": "tapir", "site": z.id("south"), "owner": ann})
	z.rec(t, "k7", "zoo_keepers", map[string]any{"user": ann, "animal": dud.Id})
	z.rec(t, "k8", "zoo_keepers", map[string]any{"user": ann, "animal": tap.Id})
	z.feeding(t, "d1", dud, 400, "2026-01-10", 10)
	z.feeding(t, "d2", dud, -400, "2026-03-01", 10)
	z.feeding(t, "t1", tap, 200, "2026-01-08", 10)
	z.feeding(t, "t2", tap, -200, "2026-01-09", 10)
	return z
}

type figures struct{ cur, pri int64 }

func TestAStatementWithSectionsRaggedLevelsAndRowsInsideASection(t *testing.T) {
	z := balanceZoo(t)
	// The year rows' hand sums assume fiscal years that start in January.
	if m := fiscalStartOf(lookupSource(z.app, "zoo.feedings"), z.app, nil); m != 1 {
		t.Fatalf("fiscal start month %d, want 1", m)
	}
	res := z.run(t, "ann", zooBalance(), Options{}, map[string]string{"asOf": `"2027-02-15"`})
	byLabel := map[string]Row{}
	var labels []any
	for _, r := range res.Rows {
		byLabel[r.Label] = r
		labels = append(labels, r.Label)
	}
	requireRows(t, res, "label",
		"Felines", "North wing", "Big cats", "Dud", "Leo", "Total Big cats", "Total North wing", "Kit", "Total Felines",
		"Grazers", "South wing", "Stripes", "Zed", "Total Stripes", "Total South wing", "Total Grazers",
		"Others", "North wing", "Mo", "Total North wing", "Earlier years", "This year", "Total Others",
		"Grazers and others")
	get := func(label string) figures {
		r, ok := byLabel[label]
		if !ok {
			t.Fatalf("no row %q in %v", label, labels)
		}
		cur, _ := r.Cells["cur"].(int64)
		pri, _ := r.Cells["pri"].(int64)
		return figures{cur, pri}
	}
	// Earlier years: every feeding by 2027-02-15 is in fiscal 2026 (< 2027):
	// 4749; by 2026-02-15 none is before 2026: 0. This year: 0 and 4750.
	if f := get("Earlier years"); f != (figures{4749, 0}) {
		t.Fatalf("earlier years = %+v", f)
	}
	if f := get("This year"); f != (figures{0, 4750}) {
		t.Fatalf("this year = %+v", f)
	}
	// The last section's total with its within rows equals the hand sums in
	// both day columns: Mo 150/150 + 4749/0 + 0/4750.
	if f := get("Total Others"); f != (figures{4899, 4900}) {
		t.Fatalf("total others = %+v", f)
	}
	if f := get("Total Felines"); f != (figures{3599, 3900}) {
		t.Fatalf("total felines = %+v", f)
	}
	if f := get("Total Grazers"); f != (figures{1000, 700}) {
		t.Fatalf("total grazers = %+v", f)
	}
	// The last row is the sum of the two later sections.
	last := res.Rows[len(res.Rows)-1]
	if f := get(last.Label); f != (figures{get("Total Grazers").cur + get("Total Others").cur, get("Total Grazers").pri + get("Total Others").pri}) {
		t.Fatalf("last row %q = %+v", last.Label, f)
	}
	// Change on every row with figures is its own Current − Prior; percent is
	// blank where Prior is 0.
	for _, r := range res.Rows {
		cur, hasCur := r.Cells["cur"].(int64)
		pri, hasPri := r.Cells["pri"].(int64)
		if !hasCur || !hasPri {
			continue
		}
		if r.Cells["chg"] != cur-pri {
			t.Fatalf("%s: change %v, want %d", r.Label, r.Cells["chg"], cur-pri)
		}
		if pri == 0 && r.Cells["pct"] != nil {
			t.Fatalf("%s: percent %v on a zero prior", r.Label, r.Cells["pct"])
		}
	}
	// Tapirs (Tap nets 0 on both days, so the having hides it) is absent with
	// its subtotal; Kit (no site) sits directly under Felines.
	for _, gone := range []string{"Tapirs", "Total Tapirs", "Tap"} {
		if _, ok := byLabel[gone]; ok {
			t.Fatalf("%q shows", gone)
		}
	}
	if kit := byLabel["Kit"]; kit.Depth != 1 || kit.Level != 4 {
		t.Fatalf("Kit at depth %d level %d", kit.Depth, kit.Level)
	}
	// No hidden key column reaches the result.
	for _, c := range res.Columns {
		if c.ID == "side" || c.ID == "cls" || c.ID == "knd" {
			t.Fatalf("hidden column %q in the result", c.ID)
		}
	}
}
