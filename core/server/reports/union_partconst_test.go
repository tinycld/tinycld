package reports

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addZooMoves registers zoo.moves, a union of two parts that each map the
// same keys and set the constant key kind: zoo_moves_a (kind "a") and
// zoo_moves_b (kind "b"). Each move links a site (account), a crate and a
// vet; every target is listed by any signed-in member, so a reader sees every
// target row.
//
//	m1 a crate gold        m5 b vet Vera crate tin
//	m2 a crate tin         m6 b no vet   crate tin
//	m3 a no crate          m7 b vet Vic  no crate
//	m4 a crate gold vet Vera
func addZooMoves(t *testing.T, z *zoo) {
	t.Helper()
	signedIn := rule(`@request.auth.id != ""`)
	target := func(name string) *core.Collection {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(&core.TextField{Name: "name"}, &core.TextField{Name: "grade"})
		z.save(t, col)
		return col
	}
	crates, vets := target("zoo_crates"), target("zoo_vets")
	sites, err := z.app.FindCollectionByNameOrId("zoo_sites")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zoo_moves_a", "zoo_moves_b"} {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.RelationField{Name: "account", CollectionId: sites.Id, MaxSelect: 1},
			&core.RelationField{Name: "crate", CollectionId: crates.Id, MaxSelect: 1},
			&core.RelationField{Name: "vet", CollectionId: vets.Id, MaxSelect: 1},
			&core.TextField{Name: "note"},
			&core.NumberField{Name: "amount"},
		)
		z.save(t, col)
	}
	z.rec(t, "gold", "zoo_crates", map[string]any{"name": "Gold crate", "grade": "gold"})
	z.rec(t, "tin", "zoo_crates", map[string]any{"name": "Tin crate", "grade": "tin"})
	z.rec(t, "vera", "zoo_vets", map[string]any{"name": "Vera"})
	z.rec(t, "vic", "zoo_vets", map[string]any{"name": "Vic"})
	move := func(key, collection, site, crate, vet string, amount int) {
		data := map[string]any{"account": z.id(site), "note": key, "amount": amount}
		if crate != "" {
			data["crate"] = z.id(crate)
		}
		if vet != "" {
			data["vet"] = z.id(vet)
		}
		z.rec(t, key, collection, data)
	}
	move("m1", "zoo_moves_a", "north", "gold", "", 100)
	move("m2", "zoo_moves_a", "south", "tin", "", 200)
	move("m3", "zoo_moves_a", "north", "", "", 300)
	move("m4", "zoo_moves_a", "south", "gold", "vera", 400)
	move("m5", "zoo_moves_b", "north", "tin", "vera", 500)
	move("m6", "zoo_moves_b", "south", "tin", "", 600)
	move("m7", "zoo_moves_b", "north", "", "vic", 700)

	columns := map[string]string{"account": "account", "crate": "crate", "vet": "vet", "note": "note", "amount": "amount"}
	RegisterSource(Source{
		Key: "zoo.crates", Collection: "zoo_crates", Label: "Crates",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text}, {Key: "grade", Label: "Grade", Type: Text}},
	})
	RegisterSource(Source{
		Key: "zoo.vets", Collection: "zoo_vets", Label: "Vets",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text}, {Key: "grade", Label: "Grade", Type: Text}},
	})
	RegisterSource(Source{
		Key: "zoo.moves", Label: "Moves",
		Parts: []SourcePart{
			{Collection: "zoo_moves_a", Columns: columns, Values: map[string]any{"kind": "a"}},
			{Collection: "zoo_moves_b", Columns: columns, Values: map[string]any{"kind": "b"}},
		},
		Fields: []Field{
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
			{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax},
			{Key: "amount", Label: "Amount", Type: Number, Aggregates: SumAll},
		},
		Links: []Link{
			{Key: "account", Label: "Account", Target: "zoo.sites", Cardinality: ToOne, Field: "account"},
			{Key: "crate", Label: "Crate", Target: "zoo.crates", Cardinality: ToOne, Field: "crate"},
			{Key: "vet", Label: "Vet", Target: "zoo.vets", Cardinality: ToOne, Field: "vet"},
		},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
}

// movesByKind keeps a move of kind a whose crate is not gold and a move of
// kind b that has a vet; every row shows its site's name.
const movesByKind = `{"version":4,"source":"zoo.moves",
	"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"site","path":[{"link":"account"},{"field":"name"}]},
		{"id":"amount","path":[{"field":"amount"}]}],
	"filter":{"op":"or","args":[
		{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"kind"}]},{"value":"a"}]},
			{"op":"not","args":[{"op":"in","args":[{"path":[{"link":"crate"},{"field":"grade"}]},{"value":["gold"]}]}]}]},
		{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"kind"}]},{"value":"b"}]},
			{"op":"not","args":[{"op":"isEmpty","args":[{"path":[{"link":"vet"},{"field":"name"}]}]}]}]}]},
	"sort":[{"col":"note"}]}`

// movesWithCrates is movesByKind with a column that reads the crate's name
// on every row, so every part reads the crates.
var movesWithCrates = strings.Replace(movesByKind,
	`{"id":"amount","path":[{"field":"amount"}]}`,
	`{"id":"amount","path":[{"field":"amount"}]},{"id":"crate","path":[{"link":"crate"},{"field":"name"}]}`, 1)

// partOfMove is the part table of zoo.moves a plan arm reads.
var partOfMove = map[string]string{"zoo_moves_a": "a", "zoo_moves_b": "b"}

// tableRead is a plan step that reads a table, and the arm of a compound
// query it lies in (the steps under a LEFT-MOST SUBQUERY or a UNION ALL
// step), or -1 for a step outside every compound query.
type tableRead struct {
	table string
	arm   int
}

// tableReads lists each SCAN or SEARCH step of a plan with the arm it lies
// in, the nearest compound arm among its parents, and names the part that
// each arm reads (partOfMove).
func tableReads(plan []string) (reads []tableRead, armPart map[int]string) {
	armPart = map[int]string{}
	type frame struct {
		indent int
		arm    int
	}
	var stack []frame
	arms := 0
	for _, line := range plan {
		detail := strings.TrimLeft(line, " ")
		indent := len(line) - len(detail)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		arm := -1
		if len(stack) > 0 {
			arm = stack[len(stack)-1].arm
		}
		if strings.HasPrefix(detail, "LEFT-MOST SUBQUERY") || strings.HasPrefix(detail, "UNION ALL") {
			arms++
			arm = arms
		}
		if f := strings.Fields(detail); len(f) > 1 && (f[0] == "SCAN" || f[0] == "SEARCH") {
			reads = append(reads, tableRead{table: f[1], arm: arm})
			if part, ok := partOfMove[f[1]]; ok && arm >= 0 {
				armPart[arm] += part
			}
		}
		stack = append(stack, frame{indent: indent, arm: arm})
	}
	return reads, armPart
}

// partsReading names, over every statement a run read, the parts of zoo.moves
// whose rows read table: "a", "b", or "ab" for a read outside the arms of the
// parts, which serves the rows of both. A read in an arm that is not one
// part's is "?".
func partsReading(t *testing.T, z *zoo, seen []readStatement, table string) map[string]bool {
	t.Helper()
	db := readPool(t, z)
	out := map[string]bool{}
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		reads, armPart := tableReads(queryPlan(t, db, raw, args))
		for _, r := range reads {
			if r.table != table {
				continue
			}
			part := armPart[r.arm]
			switch {
			case r.arm < 0:
				out["a"], out["b"] = true, true
			case part == "a" || part == "b":
				out[part] = true
			default:
				out["?"] = true
			}
		}
	}
	return out
}

// requireParts requires the parts whose rows read table to be want.
func requireParts(t *testing.T, z *zoo, seen []readStatement, table string, want ...string) {
	t.Helper()
	got := partsReading(t, z, seen, table)
	ok := len(got) == len(want)
	for _, p := range want {
		ok = ok && got[p]
	}
	if !ok {
		var plans []string
		db := readPool(t, z)
		for _, st := range seen {
			raw, args := rawStatement(t, st.sql, st.params)
			plans = append(plans, strings.Join(queryPlan(t, db, raw, args), "\n"))
		}
		t.Fatalf("the rows of parts %v read %s, want %v:\n%s", got, table, want, strings.Join(plans, "\n\n"))
	}
}

// A filter whose branches each test the constant kind: part a's rows look up
// no vet (the branch of kind b), part b's rows no crate (the branch of kind
// a); the site, which a column reads on every row, is read on both parts. A
// crate a column reads on every row is read on both parts too. With the
// switch on, every part's rows read every link again.
func TestAPartConstantFoldsTheBranchesItMakesFalse(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	seen := observeReads(t)
	res := z.run(t, "ann", movesByKind, Options{}, nil)
	if got := strings.Join(detailNotes(res), ","); got != "m2,m3,m5,m7" {
		t.Fatalf("the moves are %s, want m2,m3,m5,m7", got)
	}
	requireParts(t, z, *seen, "zoo_crates", "a")
	requireParts(t, z, *seen, "zoo_vets", "b")
	requireParts(t, z, *seen, "zoo_sites", "a", "b")

	*seen = nil
	z.run(t, "ann", movesWithCrates, Options{}, nil)
	requireParts(t, z, *seen, "zoo_crates", "a", "b")
	requireParts(t, z, *seen, "zoo_vets", "b")

	legacySQL.noPartConstants = true
	t.Cleanup(func() { legacySQL.noPartConstants = false })
	*seen = nil
	z.run(t, "ann", movesByKind, Options{}, nil)
	requireParts(t, z, *seen, "zoo_vets", "a", "b")
}

// The rows with the constants folded are the rows without: moves of both
// parts in and out of each branch, with and without groups, for a reader of
// every part and a guest, who lists no site.
func TestAPartConstantFoldedKeepsTheRows(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	z.person(t, "gus", "guest")
	grouped := strings.Replace(movesWithCrates, `"sort":[{"col":"note"}]`,
		`"groups":[{"id":"bySite","col":"site","open":true}],"sort":[{"col":"note"}]`, 1)
	for _, spec := range []string{movesByKind, movesWithCrates, grouped} {
		for _, user := range []string{"ann", "gus"} {
			requireSameAsLegacy(t, z, user, spec, Options{ExpandAll: true}, nil, &legacySQL.noPartConstants)
		}
	}
}
