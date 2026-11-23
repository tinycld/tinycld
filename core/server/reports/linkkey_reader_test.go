package reports

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// visitKeyOptions is a visits report whose Visits param lists each visit by
// its note, valued by the key of its link, in groups of the note.
func visitKeyOptions(link string) string {
	return `{"version":4,"source":"zoo.visits",
	"params":[{"id":"visits","type":"choice","label":"Visits","multiple":true,
		"optionsFrom":{"source":"zoo.visits","value":[{"link":"` + link + `"},{"field":"id"}],"label":[{"field":"note"}],"group":[{"field":"note"}]}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}],
	"sort":[{"col":"note"}]}`
}

// keyOptionValues maps each option's label to its value.
func keyOptionValues(res *ParamOptions) map[string]string {
	out := map[string]string{}
	for _, o := range res.Options {
		out[o.Label] = o.Value
	}
	return out
}

// sameOptionsAsLegacy reads a param's options with the key read through the
// join (the switch on) and without, and requires the same options and groups.
func sameOptionsAsLegacy(t *testing.T, z *zoo, user, spec string) {
	t.Helper()
	t.Cleanup(func() { legacySQL.joinLinkKeys = false })
	legacySQL.joinLinkKeys = true
	before, err := json.Marshal(z.options(t, user, spec, OptionsRequest{Param: "visits"}))
	if err != nil {
		t.Fatal(err)
	}
	legacySQL.joinLinkKeys = false
	after, err := json.Marshal(z.options(t, user, spec, OptionsRequest{Param: "visits"}))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("the options changed\nbefore: %s\nafter:  %s", before, after)
	}
}

// Param options run for a reader, so their statements read a link's key as
// a run's do: the site key, behind a rule one answer serves, from the row's
// own column with no join to the sites; the animal key, behind a rule of the
// row, through the join, so ann never gets mo's key (v2 links mo). The
// options are the options the join gave.
func TestParamOptionsReadALinkKeyWithoutAJoin(t *testing.T) {
	z := newZoo(t)
	addZooVisits(t, z)
	z.person(t, "gus", "guest")
	for _, user := range []string{"ann", "gus"} {
		sameOptionsAsLegacy(t, z, user, visitKeyOptions("site"))
		sameOptionsAsLegacy(t, z, user, visitKeyOptions("animal"))
	}

	seen := observeReads(t)
	values := keyOptionValues(z.options(t, "ann", visitKeyOptions("animal"), OptionsRequest{Param: "visits"}))
	if values["v1"] != z.id("leo") || values["v2"] == z.id("mo") {
		t.Errorf("ann's animal key options are %v: want leo's key on v1 and never mo's", values)
	}
	if !joinsTo(t, *seen, "zoo_animals") {
		t.Errorf("the animal key options, behind a rule of the row, are read with no join to the animals:\n%s", (*seen)[0].sql)
	}

	*seen = nil
	values = keyOptionValues(z.options(t, "ann", visitKeyOptions("site"), OptionsRequest{Param: "visits"}))
	if values["v1"] != z.id("north") || values["v2"] != z.id("south") {
		t.Errorf("ann's site key options are %v, want north's key on v1 and south's on v2", values)
	}
	if joinsTo(t, *seen, "zoo_sites") {
		t.Errorf("the site key options join the sites:\n%s", (*seen)[0].sql)
	}
}

// addZooRounds registers zoo.rounds on zoo_rounds, whose rows link a pen and
// a vip. Pens are listed by any signed-in user: a rule that reads no row and
// joins nothing. Vips are listed by an admin only: a rule that reads no row
// and joins the reader's record.
//
//	r1 pen p1 vip w1   r2 pen p2   r3 no pen, no vip
func addZooRounds(t *testing.T, z *zoo) {
	t.Helper()
	target := func(name, listRule string) *core.Collection {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = rule(listRule), rule(listRule)
		col.Fields.Add(&core.TextField{Name: "name"})
		z.save(t, col)
		return col
	}
	pens := target("zoo_pens", `@request.auth.id != ""`)
	vips := target("zoo_vips", `@request.auth.role = "admin"`)
	rounds := core.NewBaseCollection("zoo_rounds")
	signedIn := rule(`@request.auth.id != ""`)
	rounds.ListRule, rounds.ViewRule = signedIn, signedIn
	rounds.Fields.Add(
		&core.RelationField{Name: "pen", CollectionId: pens.Id, MaxSelect: 1},
		&core.RelationField{Name: "vip", CollectionId: vips.Id, MaxSelect: 1},
		&core.TextField{Name: "note"},
	)
	z.save(t, rounds)
	z.rec(t, "p1", "zoo_pens", map[string]any{"name": "Pen one"})
	z.rec(t, "p2", "zoo_pens", map[string]any{"name": "Pen two"})
	z.rec(t, "w1", "zoo_vips", map[string]any{"name": "Vip one"})
	z.rec(t, "r1", "zoo_rounds", map[string]any{"pen": z.id("p1"), "vip": z.id("w1"), "note": "r1"})
	z.rec(t, "r2", "zoo_rounds", map[string]any{"pen": z.id("p2"), "note": "r2"})
	z.rec(t, "r3", "zoo_rounds", map[string]any{"note": "r3"})

	named := []Field{{Key: "name", Label: "Name", Type: Text}}
	RegisterSource(Source{Key: "zoo.pens", Collection: "zoo_pens", Label: "Pens", Fields: named})
	RegisterSource(Source{Key: "zoo.vips", Collection: "zoo_vips", Label: "Vips", Fields: named})
	RegisterSource(Source{
		Key: "zoo.rounds", Collection: "zoo_rounds", Label: "Rounds",
		Fields: []Field{{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax}},
		Links: []Link{
			{Key: "pen", Label: "Pen", Target: "zoo.pens", Cardinality: ToOne, Field: "pen"},
			{Key: "vip", Label: "Vip", Target: "zoo.vips", Cardinality: ToOne, Field: "vip"},
		},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
}

// requireKeys requires the key and blank cells of each round to be want's.
func requireKeys(t *testing.T, res *Result, who string, want map[string]any) {
	t.Helper()
	keys, blanks := cellsByNote(res, "key"), cellsByNote(res, "blank")
	for note, key := range want {
		if keys[note] != key {
			t.Errorf("%s reads the key of %s as %#v, want %#v", who, note, keys[note], key)
		}
		if blanks[note] != (key == nil) {
			t.Errorf("%s reads isEmpty of the key of %s as %#v, want %v", who, note, blanks[note], key == nil)
		}
	}
}

// A target whose rule reads no row and joins nothing (@request.auth.id !=
// "") gives one answer for every row: a signed-in reader sees every pen, so
// the pen key is read from the row's own column with no join to the pens.
// The keys are the keys the join gave.
func TestALinkKeyBehindARuleThatJoinsNothingIsReadWithoutAJoin(t *testing.T) {
	z := newZoo(t)
	addZooRounds(t, z)
	z.person(t, "gus", "guest")
	for _, filter := range []string{keyBlank, keySet, keyEither} {
		for _, user := range []string{"ann", "gus"} {
			requireSameAsLegacy(t, z, user, keyReport("zoo.rounds", "pen", filter), Options{}, nil, &legacySQL.joinLinkKeys)
		}
	}
	seen := observeReads(t)
	res := z.run(t, "ann", keyReport("zoo.rounds", "pen", keyEither), Options{}, nil)
	requireKeys(t, res, "ann", map[string]any{"r1": z.id("p1"), "r2": z.id("p2"), "r3": nil})
	if joinsTo(t, *seen, "zoo_pens") {
		t.Errorf("the pen key, behind a rule that reads no row, joins the pens:\n%s", (*seen)[0].sql)
	}
}

// A target whose rule one answer serves and that answer fails for the reader
// (@request.auth.role = "admin", read by a member) shows him no key: every
// vip key is blank and no row has one set. An admin reads the vip key. The
// keys are the keys the join gave.
func TestALinkKeyBehindAFailedOneAnswerRuleIsBlank(t *testing.T) {
	z := newZoo(t)
	addZooRounds(t, z)
	z.person(t, "ada", "admin")
	for _, filter := range []string{keyBlank, keySet, keyEither} {
		for _, user := range []string{"ann", "ada"} {
			requireSameAsLegacy(t, z, user, keyReport("zoo.rounds", "vip", filter), Options{}, nil, &legacySQL.joinLinkKeys)
		}
	}
	res := z.run(t, "ann", keyReport("zoo.rounds", "vip", keyEither), Options{}, nil)
	requireKeys(t, res, "ann", map[string]any{"r1": nil, "r2": nil, "r3": nil})
	if rows := detailNotes(z.run(t, "ann", keyReport("zoo.rounds", "vip", keySet), Options{}, nil)); len(rows) != 0 {
		t.Errorf("ann, who may list no vip, finds rows with a vip key set: %v", rows)
	}
	res = z.run(t, "ada", keyReport("zoo.rounds", "vip", keyEither), Options{}, nil)
	requireKeys(t, res, "ada", map[string]any{"r1": z.id("w1"), "r2": nil, "r3": nil})
}
