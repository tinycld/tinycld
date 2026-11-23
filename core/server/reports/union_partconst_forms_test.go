package reports

import (
	"strings"
	"testing"
)

// The terms of zoo.moves that the forms below are built from: the kind tests,
// and the reads of the crate's and the vet's fields.
const (
	kindIsA    = `{"op":"eq","args":[{"path":[{"field":"kind"}]},{"value":"a"}]}`
	kindIsB    = `{"op":"eq","args":[{"path":[{"field":"kind"}]},{"value":"b"}]}`
	crateName  = `{"path":[{"link":"crate"},{"field":"name"}]}`
	crateGrade = `{"path":[{"link":"crate"},{"field":"grade"}]}`
	vetName    = `{"path":[{"link":"vet"},{"field":"name"}]}`
)

// movesShowing lists every move with one more column, sorted as sort says.
func movesShowing(column, sort string) string {
	return `{"version":4,"source":"zoo.moves",
	"columns":[{"id":"note","path":[{"field":"note"}]}` + column + `],
	"sort":[` + sort + `]}`
}

// movesWhere keeps the moves filter keeps; every row shows its site's name.
func movesWhere(source, filter string) string {
	return `{"version":4,"source":"` + source + `",
	"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"site","path":[{"link":"account"},{"field":"name"}]}],
	"filter":` + filter + `,
	"sort":[{"col":"note"}]}`
}

// The reports of the forms a part constant folds: a value column of an if
// and of a case on the kind, a sort key of an if on the kind that reads a
// link and one that reads none, a filter that tests the kind with ne and
// not(eq), and a filter on zoo.ranked's number constant.
var (
	whoByIf = movesShowing(`,{"id":"who","formula":{"op":"if","args":[`+kindIsA+`,`+crateName+`,`+vetName+`]}}`,
		`{"col":"note"}`)
	whoByCase = movesShowing(`,{"id":"who","formula":{"op":"case","args":[`+kindIsA+`,`+crateName+`,`+kindIsB+`,`+vetName+`,{"value":null}]}}`,
		`{"col":"note"}`)
	sortByWho    = movesShowing(``, `{"formula":{"op":"if","args":[`+kindIsA+`,`+crateGrade+`,`+vetName+`]}},{"col":"note"}`)
	sortBFirst   = movesShowing(``, `{"formula":{"op":"if","args":[`+kindIsA+`,{"value":1},{"value":0}]}},{"col":"note"}`)
	movesWhereNe = movesWhere("zoo.moves", `{"op":"or","args":[
		{"op":"and","args":[{"op":"ne","args":[{"path":[{"field":"kind"}]},{"value":"b"}]},
			{"op":"not","args":[{"op":"in","args":[`+crateGrade+`,{"value":["gold"]}]}]}]},
		{"op":"and","args":[{"op":"not","args":[`+kindIsA+`]},
			{"op":"not","args":[{"op":"isEmpty","args":[`+vetName+`]}]}]}]}`)
	movesWhereRank = movesWhere("zoo.ranked", `{"op":"or","args":[
		{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"rank"}]},{"value":1}]},
			{"op":"not","args":[{"op":"in","args":[`+crateGrade+`,{"value":["gold"]}]}]}]},
		{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"rank"}]},{"value":2}]},
			{"op":"not","args":[{"op":"isEmpty","args":[`+vetName+`]}]}]}]}`)
)

// addZooRanked registers zoo.ranked, zoo.moves with the number constant rank:
// 1 on part a, 2 on part b.
func addZooRanked(t *testing.T, z *zoo) {
	t.Helper()
	columns := map[string]string{"account": "account", "crate": "crate", "vet": "vet", "note": "note", "amount": "amount"}
	RegisterSource(Source{
		Key: "zoo.ranked", Label: "Ranked moves",
		Parts: []SourcePart{
			{Collection: "zoo_moves_a", Columns: columns, Values: map[string]any{"rank": 1}},
			{Collection: "zoo_moves_b", Columns: columns, Values: map[string]any{"rank": 2}},
		},
		Fields: []Field{
			{Key: "rank", Label: "Rank", Type: Number},
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

// A value column whose branch the kind decides, as an if and as a case: part
// a's rows read no vet (the branch of kind b), part b's rows no crate.
func TestAPartConstantFoldsAValueTerm(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	want := map[string]any{
		"m1": "Gold crate", "m2": "Tin crate", "m3": nil, "m4": "Gold crate",
		"m5": "Vera", "m6": nil, "m7": "Vic",
	}
	for name, spec := range map[string]string{"if": whoByIf, "case": whoByCase} {
		t.Run(name, func(t *testing.T) {
			seen := observeReads(t)
			got := cellsByNote(z.run(t, "ann", spec, Options{}, nil), "who")
			for note, w := range want {
				if got[note] != w {
					t.Errorf("who of %s is %#v, want %#v", note, got[note], w)
				}
			}
			requireParts(t, z, *seen, "zoo_crates", "a")
			requireParts(t, z, *seen, "zoo_vets", "b")
		})
	}
}

// A sort key whose branch the kind decides: part a's rows read no vet, part
// b's rows no crate. A sort key of the kind alone (an if of 1 and 0) puts the
// moves of kind b first.
func TestAPartConstantFoldsASortTerm(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	if got := strings.Join(detailNotes(z.run(t, "ann", sortBFirst, Options{}, nil)), ","); got != "m5,m6,m7,m1,m2,m3,m4" {
		t.Fatalf("the moves of kind b first are %s, want m5,m6,m7,m1,m2,m3,m4", got)
	}
	seen := observeReads(t)
	if rows := detailNotes(z.run(t, "ann", sortByWho, Options{}, nil)); len(rows) != 7 {
		t.Fatalf("the moves by who are %v, want the 7 moves", rows)
	}
	requireParts(t, z, *seen, "zoo_crates", "a")
	requireParts(t, z, *seen, "zoo_vets", "b")
}

// A filter that tests the kind with ne and with not(eq) folds as eq does:
// part a's rows look up no vet, part b's rows no crate; the site, read by a
// column on every row, is read on both parts.
func TestAPartConstantFoldsANotEqualTest(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	seen := observeReads(t)
	if got := strings.Join(detailNotes(z.run(t, "ann", movesWhereNe, Options{}, nil)), ","); got != "m2,m3,m5,m7" {
		t.Fatalf("the moves are %s, want m2,m3,m5,m7", got)
	}
	requireParts(t, z, *seen, "zoo_sites", "a", "b")
	requireParts(t, z, *seen, "zoo_crates", "a")
	requireParts(t, z, *seen, "zoo_vets", "b")
}

// A number constant folds as a text one: on zoo.ranked, part a's rows look up
// no vet, part b's rows no crate.
func TestAPartConstantOfANumberFolds(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	addZooRanked(t, z)
	seen := observeReads(t)
	if got := strings.Join(detailNotes(z.run(t, "ann", movesWhereRank, Options{}, nil)), ","); got != "m2,m3,m5,m7" {
		t.Fatalf("the moves are %s, want m2,m3,m5,m7", got)
	}
	requireParts(t, z, *seen, "zoo_crates", "a")
	requireParts(t, z, *seen, "zoo_vets", "b")
}

// The rows of every form above are the rows with the switch on, for a reader
// of every part and a guest, who lists no site.
func TestAPartConstantFoldedFormKeepsTheRows(t *testing.T) {
	z := newZoo(t)
	addZooMoves(t, z)
	addZooRanked(t, z)
	z.person(t, "gus", "guest")
	for _, spec := range []string{whoByIf, whoByCase, sortByWho, sortBFirst, movesWhereNe, movesWhereRank} {
		for _, user := range []string{"ann", "gus"} {
			requireSameAsLegacy(t, z, user, spec, Options{}, nil, &legacySQL.noPartConstants)
		}
	}
}
