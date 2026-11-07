package reports

import (
	"reflect"
	"strings"
	"testing"
)

// A detail-row lookup over zoo.feedings, grouped by animal. WHERE is the
// lookup's where; the other columns are what it may read of the row.
const hoistSpec = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"min","type":"number","label":"Min"}],
	"columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}]},
		{"id":"run","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
		{"id":"kg","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"weight"}]},"pick":"max",
			"where":WHERE}}],
	"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`

const sameSpecies = `{"op":"eq","args":[{"path":[{"field":"species"}]},{"record":"g","path":[{"field":"species"}]}]}`

// compileKgOnADetailRow compiles the kg lookup on a detail row: alone, to
// return what it records it reads of the row, and as the kg column, to
// return the frame it is hoisted into.
func compileKgOnADetailRow(t *testing.T, where string) (*rowReads, *windowFrame) {
	t.Helper()
	app := withZooRegistry(t)
	p, err := bind(app, loadSpec(t, strings.Replace(hoistSpec, "WHERE", where, 1)))
	if err != nil {
		t.Fatal(err)
	}
	c := newCompiler(p, nil, nil, map[string]paramValue{"min": {typ: ParamNumber, value: int64(100)}})
	r, rerr := c.rows(newStmt(nil, nil))
	if rerr != nil {
		t.Fatal(rerr)
	}
	reads := &rowReads{}
	alone := &rowsCtx{r: r, visiting: map[string]bool{}, detail: &windowFrame{checking: true}, reads: reads}
	if _, err := lookupIn(alone, r, p.byID["kg"].col.Formula, "test"); err != nil {
		t.Fatal(err)
	}
	rc := &rowsCtx{r: r, visiting: map[string]bool{}, detail: &windowFrame{}}
	if _, err := rc.col("kg", "test"); err != nil {
		t.Fatal(err)
	}
	return reads, rc.detail
}

// keyedLayers are the group keys of each layer that computes its values once
// per group key.
func keyedLayers(f *windowFrame) [][]string {
	var keys [][]string
	for _, h := range f.hoisted {
		if len(h.keys) > 0 {
			keys = append(keys, h.keys)
		}
	}
	return keys
}

// A lookup that reads a value of the row besides its group record has a
// value per row, so it is never computed once per group, however the value
// is read: a column, an outer path, a running value, or a column whose
// formula reads the row.
func TestALookupThatReadsARowValueIsNotGroupInvariant(t *testing.T) {
	for name, rowValue := range map[string]string{
		"column":        `{"op":"ne","args":[{"path":[{"field":"name"}]},{"col":"note"}]}`,
		"outer path":    `{"op":"ne","args":[{"path":[{"field":"name"}]},{"outer":[{"field":"note"}]}]}`,
		"running value": `{"op":"ne","args":[{"path":[{"field":"weight"}]},{"col":"run"}]}`,
		"inner lookup": `{"op":"ne","args":[{"path":[{"field":"name"}]},{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"name"}]},"pick":"min",
			"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"col":"note"}]}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			where := `{"op":"and","args":[` + sameSpecies + `,` + rowValue + `]}`
			reads, frame := compileKgOnADetailRow(t, where)
			if !reads.other {
				t.Fatalf("the lookup's reads %+v miss its read of a row value", reads)
			}
			if keys := reads.groupKeys(); keys != nil {
				t.Fatalf("group keys = %v, want none", keys)
			}
			if keys := keyedLayers(frame); len(keys) > 0 {
				t.Fatalf("the lookup is computed once per group key %v", keys)
			}
		})
	}
}

// A lookup that reads of the row only its group record, with params and
// constants, has one value for every row of a group: it is computed once per
// group key.
func TestALookupThatReadsOnlyTheGroupRecordAndParamsIsGroupInvariant(t *testing.T) {
	for name, where := range map[string]string{
		"record": sameSpecies,
		"record and param": `{"op":"and","args":[` + sameSpecies + `,
			{"op":"gt","args":[{"path":[{"field":"weight"}]},{"param":"min"}]},
			{"op":"ne","args":[{"path":[{"field":"name"}]},{"value":"Kit"}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			reads, frame := compileKgOnADetailRow(t, where)
			if reads.other {
				t.Fatalf("the lookup's reads %+v name a row value it does not read", reads)
			}
			if keys := reads.groupKeys(); !reflect.DeepEqual(keys, []string{"k0"}) {
				t.Fatalf("group keys = %v, want [k0]", keys)
			}
			if keys := keyedLayers(frame); !reflect.DeepEqual(keys, [][]string{{"k0"}}) {
				t.Fatalf("keyed layers = %v, want one by k0", keys)
			}
		})
	}
}
