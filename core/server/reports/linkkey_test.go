package reports

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addZooVisits registers zoo.visits, a plain source on zoo_visits, and
// zoo.stops, a union of zoo_visits and zoo_calls. A visit links a site and an
// animal; a call links an animal only, so zoo.stops' calls read the site key
// blank. Sites are listed by every member, whatever the row (a rule one
// answer serves); animals by their owner or keeper (a rule of the row), so
// ann sees leo, zed and kit, never mo.
//
//	v1 north leo   v2 south mo   v3 no site, no animal
//	c1 zed         c2 no animal
func addZooVisits(t *testing.T, z *zoo) {
	t.Helper()
	find := func(name string) *core.Collection {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return col
	}
	sites, animals := find("zoo_sites"), find("zoo_animals")
	signedIn := rule(`@request.auth.id != ""`)
	visits := core.NewBaseCollection("zoo_visits")
	visits.ListRule, visits.ViewRule = signedIn, signedIn
	visits.Fields.Add(
		&core.RelationField{Name: "site", CollectionId: sites.Id, MaxSelect: 1},
		&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
		&core.TextField{Name: "note"},
	)
	z.save(t, visits)
	calls := core.NewBaseCollection("zoo_calls")
	calls.ListRule, calls.ViewRule = signedIn, signedIn
	calls.Fields.Add(
		&core.RelationField{Name: "pet", CollectionId: animals.Id, MaxSelect: 1},
		&core.TextField{Name: "label"},
	)
	z.save(t, calls)
	z.rec(t, "v1", "zoo_visits", map[string]any{"site": z.id("north"), "animal": z.id("leo"), "note": "v1"})
	z.rec(t, "v2", "zoo_visits", map[string]any{"site": z.id("south"), "animal": z.id("mo"), "note": "v2"})
	z.rec(t, "v3", "zoo_visits", map[string]any{"note": "v3"})
	z.rec(t, "c1", "zoo_calls", map[string]any{"pet": z.id("zed"), "label": "c1"})
	z.rec(t, "c2", "zoo_calls", map[string]any{"label": "c2"})

	fields := []Field{{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax}}
	links := []Link{
		{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"},
		{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"},
	}
	RegisterSource(Source{Key: "zoo.visits", Collection: "zoo_visits", Label: "Visits", Fields: fields, Links: links})
	RegisterSource(Source{
		Key: "zoo.stops", Label: "Stops",
		Parts: []SourcePart{
			{Collection: "zoo_visits", Columns: map[string]string{"site": "site", "animal": "animal", "note": "note"}},
			{Collection: "zoo_calls", Columns: map[string]string{"animal": "pet", "note": "label"}},
		},
		Fields: fields, Links: links,
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
}

// keyReport lists a source's rows with the key of one link and isEmpty of
// it, and keeps the rows whose key is set or blank as filter says.
func keyReport(source, link, filter string) string {
	key := `{"path":[{"link":"` + link + `"},{"field":"id"}]}`
	return `{"version":4,"source":"` + source + `",
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"key","path":[{"link":"` + link + `"},{"field":"id"}]},
		{"id":"blank","formula":{"op":"isEmpty","args":[` + key + `]}}],
	"filter":` + strings.ReplaceAll(filter, "KEY", key) + `,
	"sort":[{"col":"note"}]}`
}

// The filters of a key report: rows whose key is blank, set, or either.
const (
	keyBlank  = `{"op":"isEmpty","args":[KEY]}`
	keySet    = `{"op":"not","args":[{"op":"isEmpty","args":[KEY]}]}`
	keyEither = `{"op":"or","args":[` + keyBlank + `,` + keySet + `]}`
)

// nameReport keeps the rows whose linked row has a name: a read of a field
// of the target, not of its key.
func nameReport(source, link string) string {
	return `{"version":4,"source":"` + source + `",
	"columns":[{"id":"note","path":[{"field":"note"}]}],
	"filter":{"op":"not","args":[{"op":"isEmpty","args":[{"path":[{"link":"` + link + `"},{"field":"name"}]}]}]},
	"sort":[{"col":"note"}]}`
}

// joinsTo is true when a statement joins the rule CTE of table (a JOIN or
// LEFT JOIN of r<n>), as scope.join writes a link. A read of the CTE by FROM
// (a label's own subquery) is not a join.
func joinsTo(t *testing.T, seen []readStatement, table string) bool {
	t.Helper()
	for _, st := range seen {
		raw, _ := rawStatement(t, st.sql, st.params)
		for _, m := range ruleCTE.FindAllStringSubmatch(raw, -1) {
			if m[2] == table && strings.Contains(raw, "JOIN "+m[1]+" ") {
				return true
			}
		}
	}
	return false
}

// A report that reads only a link's key, on a target whose rule passes for
// the reader (sites, a rule one answer serves), reads the row's own link
// column: no statement joins the target's rule CTE. The same report reading
// the target's name keeps the join. A plain source, and a union whose second
// part does not map the link. With the switch on, the key is read through the
// join again.
func TestALinkKeyIsReadWithoutAJoin(t *testing.T) {
	z := newZoo(t)
	addZooVisits(t, z)
	for _, src := range []string{"zoo.visits", "zoo.stops"} {
		t.Run(src, func(t *testing.T) {
			for name, filter := range map[string]string{"blank": keyBlank, "set": keySet, "either": keyEither} {
				seen := observeReads(t)
				z.run(t, "ann", keyReport(src, "site", filter), Options{}, nil)
				if joinsTo(t, *seen, "zoo_sites") {
					t.Errorf("%s: a report that reads only the site key joins the sites:\n%s", name, (*seen)[0].sql)
				}
			}
			seen := observeReads(t)
			z.run(t, "ann", nameReport(src, "site"), Options{}, nil)
			if !joinsTo(t, *seen, "zoo_sites") {
				t.Errorf("a report that reads the site's name does not join the sites:\n%s", (*seen)[0].sql)
			}
			legacySQL.joinLinkKeys = true
			t.Cleanup(func() { legacySQL.joinLinkKeys = false })
			seen = observeReads(t)
			z.run(t, "ann", keyReport(src, "site", keyEither), Options{}, nil)
			legacySQL.joinLinkKeys = false
			if !joinsTo(t, *seen, "zoo_sites") {
				t.Errorf("with the switch on, the site key is not read through the join:\n%s", (*seen)[0].sql)
			}
		})
	}
}

// The key read from the row's own column is the key the join read: set on a
// visit, blank on a visit with no site and on a call (a part that does not
// map the site), for a reader of every site and for a reader of none.
func TestALinkKeyWithoutAJoinKeepsTheRows(t *testing.T) {
	z := newZoo(t)
	addZooVisits(t, z)
	z.person(t, "gus", "guest")
	res := z.run(t, "ann", keyReport("zoo.stops", "site", keyEither), Options{}, nil)
	want := map[string]any{"v1": z.id("north"), "v2": z.id("south"), "v3": nil, "c1": nil, "c2": nil}
	keys := cellsByNote(res, "key")
	blanks := cellsByNote(res, "blank")
	for note, key := range want {
		if keys[note] != key {
			t.Errorf("the site key of %s is %#v, want %#v", note, keys[note], key)
		}
		if blanks[note] != (key == nil) {
			t.Errorf("isEmpty of the site key of %s is %#v, want %v", note, blanks[note], key == nil)
		}
	}
	for _, src := range []string{"zoo.visits", "zoo.stops"} {
		for _, filter := range []string{keyBlank, keySet, keyEither} {
			for _, user := range []string{"ann", "gus"} {
				requireSameAsLegacy(t, z, user, keyReport(src, "site", filter), Options{}, nil, &legacySQL.joinLinkKeys)
			}
		}
	}
}

// A reader never reads the key of a linked row his rules hide. Animals are
// hidden by a rule of the row: ann's visit v2 links mo, whom she cannot see,
// so its animal key is blank and the statement keeps the join. A guest's
// site rule (one answer for every row) fails: every site key is blank.
func TestALinkKeyKeepsTheRulesOfItsTarget(t *testing.T) {
	z := newZoo(t)
	addZooVisits(t, z)
	z.person(t, "gus", "guest")
	for _, src := range []string{"zoo.visits", "zoo.stops"} {
		t.Run(src, func(t *testing.T) {
			seen := observeReads(t)
			res := z.run(t, "ann", keyReport(src, "animal", keyEither), Options{}, nil)
			want := map[string]any{"v1": z.id("leo"), "v2": nil, "v3": nil}
			if src == "zoo.stops" {
				want["c1"], want["c2"] = z.id("zed"), nil
			}
			keys, blanks := cellsByNote(res, "key"), cellsByNote(res, "blank")
			for note, key := range want {
				if keys[note] != key {
					t.Errorf("ann reads the animal key of %s as %#v, want %#v", note, keys[note], key)
				}
				if blanks[note] != (key == nil) {
					t.Errorf("ann reads isEmpty of the animal key of %s as %#v, want %v", note, blanks[note], key == nil)
				}
			}
			if !joinsTo(t, *seen, "zoo_animals") {
				t.Errorf("the animal key, behind a rule of the row, is read with no join to the animals:\n%s", (*seen)[0].sql)
			}

			res = z.run(t, "gus", keyReport(src, "site", keyEither), Options{}, nil)
			keys, blanks = cellsByNote(res, "key"), cellsByNote(res, "blank")
			if len(keys) == 0 {
				t.Fatal("the guest reads no row")
			}
			for note, key := range keys {
				if key != nil || blanks[note] != true {
					t.Errorf("the guest, who may list no site, reads the site key of %s as %#v (isEmpty %#v)", note, key, blanks[note])
				}
			}
			if rows := detailNotes(z.run(t, "gus", keyReport(src, "site", keySet), Options{}, nil)); len(rows) != 0 {
				t.Errorf("the guest finds rows with a site key set: %v", rows)
			}
		})
	}
}
