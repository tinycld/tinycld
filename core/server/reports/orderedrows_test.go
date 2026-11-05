package reports

import (
	"fmt"
	"strings"
	"testing"
)

// failRecorder is a testing.TB that records a failure instead of stopping
// the test, so a test can check that a helper fails.
type failRecorder struct {
	testing.TB
	failed bool
	msg    string
}

func (f *failRecorder) Helper() {}

func (f *failRecorder) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}

func TestRequireOrderedRowsNeedsASort(t *testing.T) {
	res := &Result{Rows: []Row{
		{Kind: KindDetail, Cells: map[string]any{"note": "f1"}},
		{Kind: KindDetail, Cells: map[string]any{"note": "f2"}},
	}}
	const cols = `"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}]`
	for _, c := range []struct {
		name, spec string
		want       []any
		fails      string
	}{
		{"no sort", `{` + cols + `}`, []any{"f1", "f2"}, "record-id order"},
		{"an empty sort", `{` + cols + `,"sort":[]}`, []any{"f1", "f2"}, "record-id order"},
		{"a sort", `{` + cols + `,"sort":[{"col":"note"}]}`, []any{"f1", "f2"}, ""},
		{"a sort, rows out of order", `{` + cols + `,"sort":[{"col":"note"}]}`, []any{"f2", "f1"}, "note ="},
		{"no sort, rows out of order", `{` + cols + `}`, []any{"f2", "f1"}, "record-id order"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := &failRecorder{}
			requireOrderedRows(rec, c.spec, res, "note", c.want...)
			if rec.failed != (c.fails != "") || !strings.Contains(rec.msg, c.fails) {
				t.Fatalf("failed = %v, message %q; want a failure saying %q", rec.failed, rec.msg, c.fails)
			}
		})
	}
}
