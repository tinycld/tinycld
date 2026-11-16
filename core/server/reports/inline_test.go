package reports

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// countIsOne holds for a group of exactly one base row.
const countIsOne = `{"op":"eq","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":1}]}`

// inlineBySpecies groups the reader's feedings by species (open, with
// subtotals; %s is the rest of that group's JSON), then by note (closed).
const inlineBySpecies = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"s","col":"species","open":true,"subtotals":true%s},{"id":"n","col":"note"}],
	"sort":[{"col":"species"},{"col":"note"}]}`

// inlineSpec is inlineBySpecies with rest added to the species group.
func inlineSpec(rest string) string { return fmt.Sprintf(inlineBySpecies, rest) }

// inlineOne is inlineBySpecies with the species of one feeding inlined.
var inlineOne = inlineSpec(`,"inline":` + countIsOne)

// inlineZoo lets Ann keep Kit (cat, f6 99) and Mo (monkey, f5 150) too, so
// she reads two species of one feeding each between lion (f1, f2) and zebra
// (f3, f4): cat, lion, monkey, zebra.
func inlineZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": z.id("kit")})
	z.rec(t, "k6", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": z.id("mo")})
	return z
}

// oneRow is the one row of kind with label, failing the test when there is
// none or more than one.
func oneRow(t *testing.T, res *Result, kind, label string) Row {
	t.Helper()
	var found []Row
	for _, r := range res.Rows {
		if r.Kind == kind && r.Label == label {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d %s rows labelled %q in %v", len(found), kind, label, rowsOf(res, "label"))
	}
	return found[0]
}

// hasLabel says a row of the result has label.
func hasLabel(res *Result, label string) bool {
	return slices.ContainsFunc(res.Rows, func(r Row) bool { return r.Label == label })
}

func TestInlineFirstLevelAmongDrawnGroups(t *testing.T) {
	z := inlineZoo(t)
	res := z.run(t, "ann", inlineOne, Options{}, nil)
	requireRows(t, res, "label", "f6", "lion", "f1", "f2", "Total lion", "f5", "zebra", "f3", "f4", "Total zebra", "Total")
	requireRows(t, res, "kind", KindGroup, KindGroup, KindGroup, KindGroup, KindSubtotal, KindGroup, KindGroup, KindGroup, KindGroup, KindSubtotal, KindTotal)
	requireRows(t, res, "depth", 0, 0, 1, 1, 0, 0, 0, 1, 1, 0, 0)
	requireRows(t, res, "level", 2, 1, 2, 2, 1, 2, 1, 2, 2, 1, 0)
	requireRows(t, res, "cost", int64(99), int64(3500), int64(1000), int64(2500), int64(3500), int64(150), int64(1000), int64(700), int64(300), int64(1000), int64(4749))
	if hasLabel(res, "cat") || hasLabel(res, "monkey") || hasLabel(res, "Total cat") || hasLabel(res, "Total monkey") {
		t.Fatalf("an inlined group draws a row or a subtotal: %v", rowsOf(res, "label"))
	}
	lion, zebra := res.Rows[1], res.Rows[6]
	for i, want := range map[int]string{0: "", 1: "", 2: lion.ID, 3: lion.ID, 5: "", 6: "", 7: zebra.ID, 8: zebra.ID} {
		if got := res.Rows[i].Section; got != want {
			t.Fatalf("row %d (%s) section = %q, want %q", i, res.Rows[i].Label, got, want)
		}
	}
	for i, species := range map[int]string{0: "cat", 5: "monkey"} {
		key := res.Rows[i].GroupKey
		if len(key) < 2 || key[0] != species || key[1] != res.Rows[i].Label {
			t.Fatalf("%s's groupKey = %v, want the inlined %s key then its own", res.Rows[i].Label, key, species)
		}
	}
	if !lion.Expanded || !zebra.Expanded {
		t.Fatalf("a drawn group of the open level is closed: lion %+v, zebra %+v", lion, zebra)
	}
}

func TestInlineMiddleLevel(t *testing.T) {
	z := inlineZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"r","col":"region","open":true,"subtotals":true},
			{"id":"s","col":"species","open":true,"subtotals":true%s},
			{"id":"n","col":"note"}],
		"sort":[{"col":"region"},{"col":"species"},{"col":"note"}]}`
	plain := z.run(t, "ann", fmt.Sprintf(spec, ""), Options{}, nil)
	res := z.run(t, "ann", fmt.Sprintf(spec, `,"inline":`+countIsOne), Options{}, nil)

	// The rows of the plain run, less the inlined groups' rows and subtotals,
	// in the same order: the children stand in their group's place.
	var want []any
	for _, r := range plain.Rows {
		if !slices.Contains([]string{"cat", "monkey", "Total cat", "Total monkey"}, r.Label) {
			want = append(want, r.Label)
		}
	}
	requireRows(t, res, "label", want...)

	north := oneRow(t, res, KindGroup, "North")
	f5 := oneRow(t, res, KindGroup, "f5")
	if f5.Depth != 1 || f5.Level != 3 || f5.Section != north.ID {
		t.Fatalf("f5 = depth %d, level %d, section %q; want depth 1, level 3, section %q", f5.Depth, f5.Level, f5.Section, north.ID)
	}
	lion := oneRow(t, res, KindGroup, "lion")
	if lion.Depth != 1 || lion.Section != north.ID {
		t.Fatalf("lion = depth %d, section %q; want a drawn group at depth 1 under North", lion.Depth, lion.Section)
	}
	got := oneRow(t, res, KindSubtotal, "Total North").Cells["cost"]
	if wantCost := oneRow(t, plain, KindSubtotal, "Total North").Cells["cost"]; got != wantCost || got != int64(3650) {
		t.Fatalf("Total North = %v, want %v as without inline (3650)", got, wantCost)
	}
}

func TestInlineTotalsUnchanged(t *testing.T) {
	z := inlineZoo(t)
	plain := z.run(t, "ann", inlineSpec(""), Options{}, nil)
	res := z.run(t, "ann", inlineOne, Options{}, nil)
	for _, label := range []string{"Total lion", "Total zebra"} {
		got, want := oneRow(t, res, KindSubtotal, label).Cells["cost"], oneRow(t, plain, KindSubtotal, label).Cells["cost"]
		if got != want {
			t.Fatalf("%s = %v with inline, %v without", label, got, want)
		}
	}
	got, want := lastTotal(t, res).Cells["cost"], lastTotal(t, plain).Cells["cost"]
	if got != want || got != int64(4749) {
		t.Fatalf("Total = %v with inline, %v without; want 4749", got, want)
	}
}

func TestInlineChildrenShowWhateverTheLevelsOpen(t *testing.T) {
	z := inlineZoo(t)
	closed := strings.Replace(inlineOne, `"open":true,`, "", 1)
	base := z.run(t, "ann", closed, Options{}, nil)
	// The first level is not open: the drawn groups stay closed, an inlined
	// group's child shows in its place.
	requireRows(t, base, "label", "f6", "lion", "f5", "zebra", "Total")
	requireRows(t, base, "depth", 0, 0, 0, 0, 0)
	requireRows(t, base, "level", 2, 1, 2, 1, 0)

	cat := []any{"cat", "USD"}
	for name, c := range map[string]struct {
		spec string
		opts Options
	}{
		"collapse, level closed":  {closed, Options{Collapse: [][]any{cat}}},
		"expand, level closed":    {closed, Options{Expand: [][]any{cat}}},
		"collapse, level open":    {inlineOne, Options{Collapse: [][]any{cat}}},
		"expand all and collapse": {inlineOne, Options{ExpandAll: true, Collapse: [][]any{cat}}},
	} {
		t.Run(name, func(t *testing.T) {
			without := Options{ExpandAll: c.opts.ExpandAll}
			if c.opts.Collapse != nil {
				without.Collapse = [][]any{}
			}
			want := z.run(t, "ann", c.spec, without, nil)
			got := z.run(t, "ann", c.spec, c.opts, nil)
			if !reflect.DeepEqual(got.Rows, want.Rows) {
				t.Fatalf("rows\n%v\nwant (the key ignored)\n%v", rowsOf(got, "id"), rowsOf(want, "id"))
			}
		})
	}

	f6 := oneRow(t, base, KindGroup, "f6")
	if !f6.CanExpand || f6.Expanded {
		t.Fatalf("f6 = %+v, want a closed group that can open", f6)
	}
	opened := z.run(t, "ann", closed, Options{Expand: [][]any{f6.GroupKey}}, nil)
	requireRows(t, opened, "label", "f6", "cat", "lion", "f5", "zebra", "Total")
	requireRows(t, opened, "kind", KindGroup, KindDetail, KindGroup, KindGroup, KindGroup, KindTotal)
	requireRows(t, opened, "depth", 0, 1, 0, 0, 0, 0)
	if !opened.Rows[0].Expanded || opened.Rows[1].Section != opened.Rows[0].ID {
		t.Fatalf("f6 = %+v, its detail = %+v", opened.Rows[0], opened.Rows[1])
	}
	folded := z.run(t, "ann", inlineOne, Options{ExpandAll: true, Collapse: [][]any{f6.GroupKey}}, nil)
	if row := oneRow(t, folded, KindGroup, "f6"); row.Expanded || hasLabel(folded, "cat") {
		t.Fatalf("a folded f6 = %+v, rows %v", row, rowsOf(folded, "label"))
	}
}

// fillUnderInline groups the reader's feedings by species (%s is the rest of
// that group's JSON), then by animal, filling each species with its animals
// that have no feeding.
const fillUnderInline = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"s","col":"species","subtotals":true%s},
		{"id":"a","col":"animal","fill":{"where":{"op":"eq","args":[{"path":[{"field":"species"}]},{"col":"species"}]}}}],
	"sort":[{"col":"species"},{"formula":{"record":"a","path":[{"field":"name"}]}}]}`

// shownRow is what a reader sees of a row, less its place in the tree.
type shownRow struct {
	Kind, Label         string
	CanExpand, Expanded bool
	Cost                any
}

// shownRows are the rows of res as a reader sees them, less those labelled
// one of drop.
func shownRows(res *Result, drop ...string) []shownRow {
	var out []shownRow
	for _, r := range res.Rows {
		if !slices.Contains(drop, r.Label) {
			out = append(out, shownRow{r.Kind, r.Label, r.CanExpand, r.Expanded, r.Cells["cost"]})
		}
	}
	return out
}

// A fill on the level below an inline level adds the same records as without
// inline: under an inlined group in its place, under a drawn group as before,
// and a closed drawn group opens on what its fill adds.
func TestInlineFillBelowAnInlinedLevel(t *testing.T) {
	z := inlineZoo(t)
	withEmptyAnimals(t, z)
	z.rec(t, "tom", "zoo_animals", map[string]any{"name": "Tom", "species": "cat", "owner": z.id("ann"), "active": true})
	inlined := []string{"cat", "monkey", "Total cat", "Total monkey"}
	for name, open := range map[string]string{"level closed": "", "level open": `,"open":true`} {
		t.Run(name, func(t *testing.T) {
			plainSpec := fmt.Sprintf(fillUnderInline, open)
			// Without inline, cat and monkey open, as an inlined group always is:
			// on a closed level the reader opens them.
			plain := z.run(t, "ann", plainSpec, Options{}, nil)
			if open == "" {
				catAndMonkey := [][]any{oneRow(t, plain, KindGroup, "cat").GroupKey, oneRow(t, plain, KindGroup, "monkey").GroupKey}
				plain = z.run(t, "ann", plainSpec, Options{Expand: catAndMonkey}, nil)
			}
			res := z.run(t, "ann", fmt.Sprintf(fillUnderInline, open+`,"inline":`+countIsOne), Options{}, nil)

			if got, want := shownRows(res), shownRows(plain, inlined...); !reflect.DeepEqual(got, want) {
				t.Fatalf("rows with inline\n%+v\nwant (without inline, less the inlined groups)\n%+v", got, want)
			}
			if tom := oneRow(t, res, KindGroup, "Tom"); tom.Depth != 0 || tom.Level != 2 {
				t.Fatalf("the fill record Tom of the inlined cat group = depth %d, level %d; want 0 and 2", tom.Depth, tom.Level)
			}
			for _, sibling := range []string{"lion", "zebra"} {
				got, want := oneRow(t, res, KindGroup, sibling), oneRow(t, plain, KindGroup, sibling)
				if got.CanExpand != want.CanExpand || !got.CanExpand || got.Expanded != (open != "") {
					t.Fatalf("%s = canExpand %v, expanded %v; without inline canExpand %v", sibling, got.CanExpand, got.Expanded, want.CanExpand)
				}
			}
			if open != "" && !hasLabel(res, "Lia") {
				t.Fatalf("the fill record Lia is not under the open lion group: %v", rowsOf(res, "label"))
			}
		})
	}
}

func TestInlinePagesAcrossAnInlinedGroup(t *testing.T) {
	z := inlineZoo(t)
	for _, expandAll := range []bool{false, true} {
		whole := z.run(t, "ann", inlineOne, Options{ExpandAll: expandAll, Limit: 1000}, nil)
		for limit := 1; limit <= 8; limit++ {
			pages := groupedPages(t, z, "ann", inlineOne, Options{ExpandAll: expandAll, Limit: limit})
			if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
				t.Fatalf("expandAll %v, limit %d: pages\n%v\nwhole\n%v", expandAll, limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
			}
			requireCut(t, pages, whole, limit)
		}
	}
	// The inlined cat group is not a row of the page: a page of 1 holds its
	// child f6, a page of 2 f6 and lion.
	one := z.run(t, "ann", inlineOne, Options{Limit: 1}, nil)
	requireRows(t, one, "label", "f6", "Total")
	two := z.run(t, "ann", inlineOne, Options{Limit: 2}, nil)
	requireRows(t, two, "label", "f6", "lion", "Total")

	// A cursor from the spec before it gained inline is not one this report
	// gave.
	before := z.run(t, "ann", inlineSpec(""), Options{Limit: 2}, nil)
	if before.Page == nil || before.Page.Next == "" {
		t.Fatalf("page = %+v, want a next cursor", before.Page)
	}
	_, err := z.try(t, "ann", inlineOne, Options{Limit: 2, Cursor: before.Page.Next}, nil)
	requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
}

func TestInlineWithHavingAndHideEmpty(t *testing.T) {
	z := inlineZoo(t)
	isLion := `{"op":"eq","args":[{"col":"species"},{"value":"lion"}]}`
	costOverOn := func(n int) string {
		return fmt.Sprintf(`{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":%d}]}`, n)
	}
	twoLevels := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","open":true,"subtotals":true%s},{"id":"n","col":"note","having":%s}],
		"sort":[{"col":"species"},{"col":"note"}]}`

	// Lion is inlined; its f1 (1000) fails the note level's having.
	res := z.run(t, "ann", fmt.Sprintf(twoLevels, `,"inline":`+isLion, costOverOn(1500)), Options{}, nil)
	if hasLabel(res, "f1") || hasLabel(res, "lion") || hasLabel(res, "Total lion") {
		t.Fatalf("rows %v: a hidden child or the inlined group shows", rowsOf(res, "label"))
	}
	if f2 := oneRow(t, res, KindGroup, "f2"); f2.Depth != 0 || f2.Level != 2 {
		t.Fatalf("f2 = depth %d, level %d; want 0 and 2", f2.Depth, f2.Level)
	}

	// Zebra is inlined and both its feedings fail the having: it shows
	// nothing, as the hidden cat and monkey groups (hideEmpty) do.
	isZebra := `{"op":"eq","args":[{"col":"species"},{"value":"zebra"}]}`
	res = z.run(t, "ann", fmt.Sprintf(twoLevels, `,"hideEmpty":true,"inline":`+isZebra, costOverOn(900)), Options{}, nil)
	requireRows(t, res, "label", "lion", "f1", "f2", "Total lion", "Total")
}

func TestInlineReadsWhatHavingReads(t *testing.T) {
	z := inlineZoo(t)
	levelOne := func(res *Result) []string {
		var out []string
		for _, r := range res.Rows {
			if r.Kind == KindGroup && r.Level == 1 {
				out = append(out, r.Label)
			}
		}
		slices.Sort(out)
		return out
	}

	// A level total.
	res := z.run(t, "ann", inlineSpec(`,"inline":{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":200}]}`), Options{}, nil)
	if got := levelOne(res); !slices.Equal(got, []string{"lion", "zebra"}) {
		t.Fatalf("a level total: drawn groups %v, want lion and zebra", got)
	}

	// A record field of the level's group: Kit (4) and Mo (12) weigh under 100.
	byAnimal := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"a","col":"animal","open":true,"subtotals":true,
			"inline":{"op":"lt","args":[{"record":"a","path":[{"field":"weight"}]},{"value":100}]}},{"id":"n","col":"note"}],
		"sort":[{"col":"note"}]}`
	res = z.run(t, "ann", byAnimal, Options{}, nil)
	if got := levelOne(res); !slices.Equal(got, []string{"Leo", "Zed"}) {
		t.Fatalf("a record field: drawn groups %v, want Leo and Zed", got)
	}
	for _, note := range []string{"f5", "f6"} {
		if r := oneRow(t, res, KindGroup, note); r.Depth != 0 || r.Level != 2 {
			t.Fatalf("a record field: %s = depth %d, level %d", note, r.Depth, r.Level)
		}
	}

	// A param; unset, the condition matches every group, as a having's does.
	withParam := strings.Replace(inlineSpec(`,"inline":{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"max"}]}`),
		`"source":"zoo.feedings",`, `"source":"zoo.feedings","params":[{"id":"max","type":"number"}],`, 1)
	res = z.run(t, "ann", withParam, Options{}, map[string]string{"max": "200"})
	if got := levelOne(res); !slices.Equal(got, []string{"lion", "zebra"}) {
		t.Fatalf("a param: drawn groups %v, want lion and zebra", got)
	}
	res = z.run(t, "ann", withParam, Options{}, nil)
	requireRows(t, res, "label", "f6", "f1", "f2", "f5", "f3", "f4", "Total")
	requireRows(t, res, "depth", 0, 0, 0, 0, 0, 0, 0)
	requireRows(t, res, "level", 2, 2, 2, 2, 2, 2, 0)

	// A sum lookup that met two currencies: the notice names the inline
	// condition by its group.
	withEuroFeeding(t, z)
	count := `{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]}`
	orMixed := `{"op":"or","args":[` + count + `,
		{"op":"ne","args":[{"op":"coalesce","args":[` + leoCosts + `,{"value":0}]},{"value":0}]}]}`
	mixed := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","inline":` + orMixed + `},{"id":"n","col":"note"}],
		"sort":[{"col":"note"}]}`
	res = z.run(t, "ann", mixed, Options{}, nil)
	n := conditionNotice(res, "groups.0.inline")
	if n == nil {
		t.Fatalf("no notice names the inline condition: %+v", res.Notices)
	}
	if !strings.Contains(n.Message, `inline condition of the "animal" group`) || !strings.Contains(n.Message, "reads a looked-up total that mixes currencies") {
		t.Fatalf("message = %q", n.Message)
	}
}

func TestInlineRefusals(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ spec, want string }{
		"the last level": {`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"n","inline":` + countIsOne + `}]}`,
			"the last level has no child rows to show in the group's place"},
		"a pivot's last level": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species","inline":` + countIsOne + `}],
			"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`,
			"the last level has no child rows to show in the group's place"},
		"a bucketed level": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"fed","path":[{"field":"fed"}]},{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"fed","bucket":"month","inline":` + countIsOne + `},{"id":"h","col":"n"}]}`,
			"a bucketed level always shows its groups"},
		"a layout's first group": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species","inline":` + countIsOne + `},{"id":"a","col":"animal"}],
			"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`,
			"a layout's sections always show"},
		"not a condition": {`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"s","path":[{"link":"animal"},{"field":"species"}]}],
			"groups":[{"id":"g","col":"s","inline":{"value":3}},{"id":"h","col":"n"}]}`,
			"inline is a condition"},
		"one row's value": {`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"s","path":[{"link":"animal"},{"field":"species"}]}],
			"groups":[{"id":"g","col":"s","inline":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}},{"id":"h","col":"n"}]}`,
			"inline reads a group's totals, keys and records, not a value of one row"},
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, "groups.0.inline", c.want)
			_, err = z.try(t, "ann", c.spec, Options{}, nil)
			requireSpecError(t, err, 400, "groups.0.inline", c.want)
		})
	}
}

// inlineExportSpec is inlineOne without the open flag: an export opens every
// group itself.
var inlineExportSpec = strings.Replace(inlineOne, `"open":true,`, "", 1)

// inlineExportLabels are the first cells of an export of inlineExportSpec,
// every group open: no line and no total for cat or monkey. A detail row is
// labelled by its first column (species).
var inlineExportLabels = []string{"f6", "cat", "lion", "f1", "lion", "f2", "lion", "Total lion", "f5", "monkey", "zebra", "f3", "zebra", "f4", "zebra", "Total zebra", "Total"}

func inlineSavedZoo(t *testing.T) (*savedZoo, *core.Record) {
	t.Helper()
	s := newSavedZoo(t)
	s.rec(t, "k5", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": s.id("kit")})
	s.rec(t, "k6", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": s.id("mo")})
	return s, s.saved(t, "inline", "ann", "Feedings Inline", inlineExportSpec, false)
}

func TestInlineExport(t *testing.T) {
	s, rep := inlineSavedZoo(t)
	open := s.run(t, "ann", inlineExportSpec, Options{ExpandAll: true}, nil)
	requireRows(t, open, "label", toAny(inlineExportLabels)...)

	csv := csvRecords(t, s.buildCSV(t, "ann", rep.Id, `{"today":"2026-11-16"}`).Bytes)
	var csvLabels []string
	for _, r := range csv[1:] {
		csvLabels = append(csvLabels, r[0])
	}
	if !slices.Equal(csvLabels, inlineExportLabels) {
		t.Fatalf("CSV labels\n%q\nwant\n%q", csvLabels, inlineExportLabels)
	}

	// The sheet indents a label by its row's depth: the inlined child f6 is
	// at depth 0.
	sheet := sheetOf(t, buildExportAs(t, s, "ann", rep.Id, "xlsx", `{}`, `{"today":"2026-11-16","timeZone":"UTC"}`).Bytes)
	var sheetLabels []string
	for row := 7; row <= len(sheet.Cells); row++ {
		sheetLabels = append(sheetLabels, strings.TrimLeft(sheetString(t, sheet, row, 1), " "))
	}
	if !slices.Equal(sheetLabels, inlineExportLabels) {
		t.Fatalf("Excel labels\n%q\nwant\n%q", sheetLabels, inlineExportLabels)
	}
	if got := sheetString(t, sheet, 7, 1); got != "f6" {
		t.Fatalf("Excel row 7 = %q, want f6 at depth 0", got)
	}

	// The PDF keeps the reader's folds: with every group open it lists the
	// same rows as the CSV and the sheet, in the same order.
	hermeticExportFonts(t)
	text, _ := pdfTextOf(t, buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"expandAll":true,"today":"2026-11-16"}`).Bytes)
	if got := pdfFirstCells(text, "Species"); !slices.Equal(got, inlineExportLabels) {
		t.Fatalf("PDF labels\n%q\nwant\n%q\nin\n%s", got, inlineExportLabels, text)
	}
}

// pdfFirstCells is the label of each table line of a PDF's text: the lines
// after the heading line (its first word heading, repeated on each page) and
// before the page footer. A label is the line's first word, or for a total
// line every word before its figure ("Total lion").
func pdfFirstCells(text, heading string) []string {
	var out []string
	inTable := false
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0:
		case fields[0] == heading:
			inTable = true
		case !inTable || strings.Contains(line, "Page "):
			inTable = false
		case fields[0] == "Total":
			out = append(out, strings.Join(fields[:len(fields)-1], " "))
		default:
			out = append(out, fields[0])
		}
	}
	return out
}

func toAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func TestInlineDrillKeepsTheGroupKey(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	isZebra := `{"op":"eq","args":[{"col":"species"},{"value":"zebra"}]}`
	spec := strings.Replace(drilledFeedingsOn("grams", goodDrill),
		`"open":true},{"id":"a"`, `"open":true,"inline":`+isZebra+`},{"id":"a"`, 1)
	if !strings.Contains(spec, `"inline"`) {
		t.Fatal("the drilled report's groups moved")
	}
	res := s.run(t, "ann", spec, Options{}, nil)
	if hasLabel(res, "zebra") {
		t.Fatalf("the inlined zebra group draws a row: %v", rowsOf(res, "label"))
	}
	zed := oneRow(t, res, KindGroup, "Zed")
	leo := oneRow(t, res, KindGroup, "Leo")
	if zed.Depth != 0 || leo.Depth != 1 {
		t.Fatalf("Zed depth %d, Leo depth %d; want 0 and 1", zed.Depth, leo.Depth)
	}
	wantZed := fmt.Sprintf(`{"animals":[%q],"period":{"preset":"allDates"},"species":"zebra"}`, s.id("zed"))
	if got := drillJSON(t, zed, "grams"); got != wantZed {
		t.Fatalf("Zed drill = %s, want %s", got, wantZed)
	}
	wantLeo := fmt.Sprintf(`{"animals":[%q],"period":{"preset":"allDates"},"species":"lion"}`, s.id("leo"))
	if got := drillJSON(t, leo, "grams"); got != wantLeo {
		t.Fatalf("Leo drill = %s, want %s", got, wantLeo)
	}
}

// withClientZoo adds a fictional receivables book: clients with an optional
// parent client and an inactive flag, and lines on them (or on no client).
//
//	Acme                      100            no sub-clients
//	Harbor            200     own lines, and two sub-clients:
//	  Harbor Kitchen   60
//	  Harbor Bar       30     inactive
//	Mill               40, -40  nets to 0
//	Zeta              150     inactive, no sub-clients
//	(no client)        10, 5
func withClientZoo(t *testing.T, z *zoo) {
	t.Helper()
	signedIn := rule(`@request.auth.id != ""`)
	clients := core.NewBaseCollection("zoo_clients")
	clients.ListRule, clients.ViewRule = signedIn, signedIn
	clients.Fields.Add(&core.TextField{Name: "name"}, &core.BoolField{Name: "inactive"})
	z.save(t, clients)
	clients.Fields.Add(&core.RelationField{Name: "parent", CollectionId: clients.Id, MaxSelect: 1})
	z.save(t, clients)
	lines := core.NewBaseCollection("zoo_client_lines")
	lines.ListRule, lines.ViewRule = signedIn, signedIn
	lines.Fields.Add(
		&core.RelationField{Name: "client", CollectionId: clients.Id, MaxSelect: 1},
		&core.TextField{Name: "date"},
		&core.TextField{Name: "memo"},
		&core.NumberField{Name: "debit"},
		&core.NumberField{Name: "credit"},
	)
	z.save(t, lines)

	for _, c := range []struct {
		key, name, parent string
		inactive          bool
	}{
		{"acme", "Acme", "", false},
		{"harbor", "Harbor", "", false},
		{"kitchen", "Harbor Kitchen", "harbor", false},
		{"bar", "Harbor Bar", "harbor", true},
		{"mill", "Mill", "", false},
		{"zeta", "Zeta", "", true},
	} {
		data := map[string]any{"name": c.name, "inactive": c.inactive}
		if c.parent != "" {
			data["parent"] = z.id(c.parent)
		}
		z.rec(t, c.key, "zoo_clients", data)
	}
	for i, l := range []struct {
		client        string
		debit, credit int
	}{
		{"acme", 100, 0}, {"harbor", 200, 0}, {"kitchen", 60, 0}, {"bar", 30, 0},
		{"mill", 40, 0}, {"mill", 0, 40}, {"zeta", 150, 0}, {"", 10, 0}, {"", 5, 0},
	} {
		data := map[string]any{"date": fmt.Sprintf("2026-07-%02d", i+1), "memo": fmt.Sprintf("line %d", i+1), "debit": l.debit, "credit": l.credit}
		if l.client != "" {
			data["client"] = z.id(l.client)
		}
		z.rec(t, fmt.Sprintf("cl%d", i+1), "zoo_client_lines", data)
	}

	RegisterSource(Source{
		Key: "zoo.clients", Collection: "zoo_clients", Label: "Clients",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "inactive", Label: "Inactive", Type: Bool},
		},
		Links: []Link{{Key: "parent", Label: "Parent", Target: "zoo.clients", Cardinality: ToOne, Field: "parent"}},
	})
	RegisterSource(Source{
		Key: "zoo.client_lines", Collection: "zoo_client_lines", Label: "Client lines",
		Fields: []Field{
			{Key: "date", Label: "Date", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "memo", Label: "Memo", Type: Text},
			{Key: "debit", Label: "Debit", Type: Number, Aggregates: SumAll},
			{Key: "credit", Label: "Credit", Type: Number, Aggregates: SumAll},
		},
		Links: []Link{{Key: "client", Label: "Client", Target: "zoo.clients", Cardinality: ToOne, Field: "client"}},
	})
}

// netOf is a line's debit minus its credit.
const netOf = `{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}`

// nameOr is a group record's name, or "No client" for the blank key.
func nameOr(group string) string {
	return `{"op":"coalesce","args":[{"record":"` + group + `","path":[{"field":"name"}]},{"value":"No client"}]}`
}

// clientBalance is a family-balance shape on the client zoo: base rows linked
// to a record with an optional parent link and a yes/no field, grouped by the
// family (the parent, or the record itself), then by the record.
var clientBalance = `{"version":4,"source":"zoo.client_lines",
	"params":[
		{"id":"rows","type":"choice","default":"nonzero","options":[{"value":"nonzero","label":"Non-zero"},{"value":"activity","label":"With activity"}]},
		{"id":"order","type":"choice","default":"name","options":[{"value":"name","label":"Name"},{"value":"balance","label":"Largest balance"}]}],
	"columns":[
		{"id":"name","label":"Client","path":[{"link":"client"}]},
		{"id":"balance","label":"Balance","formula":{"op":"sum","args":[` + netOf + `]}},
		{"id":"topKey","label":"Family","hidden":true,"formula":{"op":"coalesce","args":[{"path":[{"link":"client"},{"link":"parent"}]},{"path":[{"link":"client"}]}]}},
		{"id":"clientKey","label":"Client","hidden":true,"path":[{"link":"client"}]}],
	"groups":[
		{"id":"byTop","col":"topKey","open":true,"subtotals":true,"hideEmpty":true,"label":` + nameOr("byTop") + `,
			"inline":{"op":"eq","args":[{"op":"sum","args":[{"op":"if","args":[
				{"op":"isEmpty","args":[{"path":[{"link":"client"},{"link":"parent"}]}]},{"value":0},{"value":1}]}]},{"value":0}]}},
		{"id":"byClient","col":"clientKey",
			"label":{"op":"concat","args":[` + nameOr("byClient") + `,
				{"op":"if","args":[{"op":"eq","args":[{"record":"byClient","path":[{"field":"inactive"}]},{"value":true}]},{"value":" (inactive)"},{"value":""}]}]},
			"having":{"op":"or","args":[{"op":"eq","args":[{"param":"rows"},{"value":"activity"}]},
				{"op":"ne","args":[{"op":"sum","args":[` + netOf + `]},{"value":0}]}]},
			"muted":{"op":"eq","args":[{"record":"byClient","path":[{"field":"inactive"}]},{"value":true}]}}],
	"sort":[
		{"formula":{"op":"if","args":[{"op":"isEmpty","args":[{"record":"byClient","path":[{"link":"parent"},{"field":"name"}]}]},{"value":0},{"value":1}]}},
		{"formula":{"op":"if","args":[{"op":"eq","args":[{"param":"order"},{"value":"balance"}]},{"op":"sum","args":[` + netOf + `]},{"value":0}]},"dir":"desc"},
		{"formula":{"op":"if","args":[{"op":"isEmpty","args":[{"record":"byTop","path":[{"field":"name"}]}]},{"value":1},{"value":0}]}},
		{"formula":{"op":"lower","args":[{"record":"byTop","path":[{"field":"name"}]}]}},
		{"formula":{"op":"lower","args":[{"record":"byClient","path":[{"field":"name"}]}]}},
		{"formula":{"path":[{"field":"date"}]}}],
	"detail":{"label":{"op":"concat","args":[{"path":[{"field":"date"}]},{"value":" · "},{"path":[{"field":"memo"}]}]}}}`

func TestInlineAndMutedFamilyShape(t *testing.T) {
	z := newZoo(t)
	withClientZoo(t, z)

	res := z.run(t, "ann", clientBalance, Options{}, nil)
	requireRows(t, res, "label", "Acme", "Harbor", "Harbor", "Harbor Bar (inactive)", "Harbor Kitchen", "Total Harbor", "Zeta (inactive)", "No client", "Total")
	requireRows(t, res, "kind", KindGroup, KindGroup, KindGroup, KindGroup, KindGroup, KindSubtotal, KindGroup, KindGroup, KindTotal)
	requireRows(t, res, "depth", 0, 0, 1, 1, 1, 0, 0, 0, 0)
	requireRows(t, res, "level", 2, 1, 2, 2, 2, 1, 2, 2, 0)
	requireRows(t, res, "balance", int64(100), int64(290), int64(200), int64(30), int64(60), int64(290), int64(150), int64(15), int64(555))
	var muted []any
	for _, r := range res.Rows {
		muted = append(muted, r.Muted)
	}
	if want := []any{false, false, false, true, false, false, true, false, false}; !reflect.DeepEqual(muted, want) {
		t.Fatalf("muted = %v, want %v", muted, want)
	}
	// The Total adds every depth-0 row and family subtotal.
	var sum int64
	for _, r := range res.Rows {
		if r.Depth == 0 && (r.Kind == KindSubtotal || (r.Kind == KindGroup && r.Level == 2)) {
			sum += toInt(r.Cells["balance"])
		}
	}
	if total := toInt(lastTotal(t, res).Cells["balance"]); total != sum || total != 555 {
		t.Fatalf("Total = %d, the depth-0 rows and family subtotals add to %d; want 555", total, sum)
	}

	// Largest balance first: the family by its total; inside it the parent's
	// own row first, then its sub-clients by balance; No client last.
	byBalance := z.run(t, "ann", clientBalance, Options{}, map[string]string{"order": `"balance"`})
	requireRows(t, byBalance, "label", "Harbor", "Harbor", "Harbor Kitchen", "Harbor Bar (inactive)", "Total Harbor", "Zeta (inactive)", "Acme", "No client", "Total")

	// With activity, Mill (nets to 0) is one plain row too.
	activity := z.run(t, "ann", clientBalance, Options{}, map[string]string{"rows": `"activity"`})
	if mill := oneRow(t, activity, KindGroup, "Mill"); mill.Depth != 0 || mill.Level != 2 || toInt(mill.Cells["balance"]) != 0 {
		t.Fatalf("Mill = %+v, want a plain row of 0", mill)
	}

	// A plain row opens into its lines.
	acme := res.Rows[0]
	opened := z.run(t, "ann", clientBalance, Options{Expand: [][]any{acme.GroupKey}}, nil)
	line := opened.Rows[1]
	if line.Kind != KindDetail || line.Depth != 1 || line.Section != acme.ID || line.Label != "2026-07-01 · line 1" {
		t.Fatalf("Acme's first row = %+v, want its line", line)
	}
}
