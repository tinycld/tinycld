package reports

import (
	"context"
	"testing"
)

// A page's cursor holds the order values of the group its last counted row is
// in and of each group around it, read from their statement rows. Were one of
// those rows missing (an engine fault), an empty page.next would end the
// report early and look complete: the cut fails the run instead (review B F2).
// No report reaches this path, so the runner is built by hand: two closed
// group rows, a page of one, and no statement row for the first group.
func TestACutWhoseCursorGroupRowIsMissingFailsTheRun(t *testing.T) {
	rn := &runner{
		ctx:       context.Background(),
		c:         &compiler{},
		rawGroups: map[string]map[string]any{},
		res: &Result{Rows: []Row{
			{ID: `g:["a"]`, Kind: KindGroup, GroupKey: []any{"a"}},
			{ID: `g:["b"]`, Kind: KindGroup, GroupKey: []any{"b"}},
			{ID: "total", Kind: KindTotal},
		}},
	}
	if err := rn.cutPage(1); err == nil {
		t.Fatalf("the cut gave page %+v, want an error", rn.res.Page)
	}
}
