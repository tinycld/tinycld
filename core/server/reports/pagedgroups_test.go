package reports

import (
	"fmt"
	"strings"
	"testing"
)

// isTotalStatement is the Total statement of a paged grouped run: its body
// ends reading o0 alone (totalPass).
func isTotalStatement(sql string) bool {
	return strings.HasSuffix(strings.TrimSpace(sql), " FROM o0")
}

// J1a: the Total is computed over every row the filter keeps, in a statement
// of its own, so a page of group rows never decides it.
func TestAGroupedRunReadsItsTotalInAStatementOfItsOwn(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	totals := 0
	for _, st := range *seen {
		if isTotalStatement(st.sql) {
			totals++
		}
	}
	if totals != 1 {
		t.Fatalf("%d Total statements, want 1", totals)
	}
	if last := res.Rows[len(res.Rows)-1]; last.Kind != "total" || last.Cells["cost"] != int64(4500) {
		t.Fatalf("total = %+v", last)
	}
}

// A pivot and a layout report still read their Total with their rows: a
// pivot has no pages, and a layout's computed rows read its sections.
func TestAPivotAndALayoutKeepOneGroupStatement(t *testing.T) {
	for name, spec := range map[string]string{"pivot": fmt.Sprintf(costBySpeciesAndMonth, ""), "layout": fmt.Sprintf(zooStatement, "")} {
		z := newZoo(t)
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{}, nil)
		for _, st := range *seen {
			if isTotalStatement(st.sql) {
				t.Fatalf("%s: a Total statement of its own", name)
			}
		}
	}
}
