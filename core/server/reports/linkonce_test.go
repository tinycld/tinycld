package reports

import (
	"strings"
	"testing"
)

// A lookup whose where matches a link to a value that reads nothing of the
// looked-up row computes that value once per row, though the equality and
// the bound on the stored relation column read it three times: here the
// value is a nested lookup (the row's own id, picked by name), and its max
// is in the detail statement once.
func TestALinkLookupComputesTheValueItMatchesOnce(t *testing.T) {
	z := newZoo(t)
	withPlainFeedingsRule(t, z)
	seen := observeReads(t)
	ownID := `{"op":"lookup","source":"zoo.animals","pick":"max","value":{"path":[{"field":"id"}]},
		"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"outer":[{"field":"name"}]}]}}`
	res := z.run(t, "ann", `{"version":3,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","formula":{"op":"lookup","source":"zoo.feedings","pick":"count","value":{"path":[{"field":"id"}]},
			"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},`+ownID+`]}}}],
		"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "fed", int64(1), int64(2), int64(2), nil)
	details := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "__level") {
			continue
		}
		details++
		if n := strings.Count(st.sql, "max("); n != 1 {
			t.Fatalf("the detail statement computes the matched value %d times:\n%s", n, st.sql)
		}
	}
	if details == 0 {
		t.Fatal("no detail statement was read")
	}
	reads := perRowReadsOf(t, z, *seen, "zoo_feedings")
	if len(reads) == 0 {
		t.Fatal("no subquery read the feedings for each row")
	}
	for _, step := range reads {
		if !indexSearch.MatchString(step) {
			t.Fatalf("the lookup reads the feedings by %q", step)
		}
	}
}
