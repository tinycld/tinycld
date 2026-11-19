package reports

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The detail statement of a grouped run joins each detail row to its open
// group (gord, a json_each over one bound list). Measured on darwin/arm64,
// 14 CPUs: with gord read inline the statement's time grows with rows x open
// groups (4,000 open groups 6.7 s, 8,000 about 27 s); with gord MATERIALIZED
// it grows with the rows (16,000 open groups 0.16 s). The limit of 2 s is for
// each statement of the run: the group statement (0.13 s at 16,660 groups)
// and the Total (0.08 s) are under it too. It is 13 times under the one
// reading and over 10 times above the other, so this fails only on the
// quadratic read. A failure is a result: never raise the limit to pass.
func TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 8000)
	withLimits(t, func(l *limits) {
		l.timeout = 2 * time.Second
		l.maxPage = 30000
	})
	res, err := z.try(t, "ann", byNoteClosed, Options{ExpandAll: true, Limit: 30000}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	if details != 8004 {
		t.Fatalf("%d detail rows, want 8004", details)
	}
}

// Both grouped paths write the join: a run that pages its groups, and a
// layout, which does not. Each computes the open groups once.
func TestTheDetailStatementComputesItsOpenGroupsOnce(t *testing.T) {
	for name, spec := range map[string]string{"paged groups": byNoteClosed, "a layout": fmt.Sprintf(zooStatement, "")} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			seen := observeReads(t)
			z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
			found := 0
			for _, st := range *seen {
				if !strings.Contains(st.sql, "JOIN gord ON") {
					continue
				}
				found++
				if !strings.Contains(st.sql, "gord AS MATERIALIZED (SELECT key AS __g") {
					t.Fatalf("the open groups are read inline:\n%s", st.sql)
				}
			}
			if found != 1 {
				t.Fatalf("%d grouped detail statements, want 1", found)
			}
		})
	}
}
