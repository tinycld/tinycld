package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

const goneSpec = `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`

func brokenOf(t *testing.T, s *savedZoo, key string) *Broken {
	t.Helper()
	rec, err := s.app.FindRecordById(reportsCollection, s.id(key))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := rec.Get("broken").(types.JSONRaw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b Broken
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	return &b
}

// CheckSaved marks each report that cannot run with why, names its owner,
// clears the mark when the report is fixed, and writes only what changed.
// It never moves `updated`.
func TestCheckSavedMarksBrokenReports(t *testing.T) {
	s := newSavedZoo(t)
	s.saved(t, "gone", "bob", "Gone", goneSpec, false)
	s.saved(t, "newer", "ann", "Newer", `{"version":99}`, false)
	before, _ := s.app.FindRecordById(reportsCollection, s.id("gone"))

	res, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 5 || len(res.Broken) != 2 || res.Written != 2 {
		t.Fatalf("checked %d, broken %d, written %d; want 5, 2, 2", res.Checked, len(res.Broken), res.Written)
	}
	gone := brokenOf(t, s, "gone")
	if gone == nil || gone.Status != 422 || gone.Problems["source"].Code != "unknown_source" {
		t.Fatalf("gone = %+v", gone)
	}
	if b := brokenOf(t, s, "newer"); b == nil || b.Status != 400 || b.Problems["version"].Code != "invalid" {
		t.Fatalf("newer = %+v", b)
	}
	for _, b := range res.Broken {
		if b.ID == s.id("gone") && (b.Owner == nil || b.Owner.Id != s.id("bob") || b.Title != "Gone") {
			t.Fatalf("gone's owner = %v", b.Owner)
		}
	}
	after, _ := s.app.FindRecordById(reportsCollection, s.id("gone"))
	if before.GetString("updated") != after.GetString("updated") {
		t.Fatal("marking a report moved updated")
	}

	again, err := CheckSaved(s.app)
	if err != nil || again.Written != 0 || len(again.Broken) != 2 {
		t.Fatalf("second check: %+v %v", again, err)
	}

	// A source that comes back clears the mark.
	RegisterSource(Source{Key: "zoo.gone", Collection: "zoo_feedings", Label: "Back",
		Fields: []Field{{Key: "cost", Label: "Cost", Type: Number, Aggregates: SumAll}}})
	fixed, err := CheckSaved(s.app)
	if err != nil || fixed.Written != 1 || brokenOf(t, s, "gone") != nil {
		t.Fatalf("after the source came back: %+v %v, gone = %+v", fixed, err, brokenOf(t, s, "gone"))
	}
}

// An app without core's report migration has no saved reports to check.
func TestCheckSavedWithoutTheCollection(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if res, err := CheckSaved(app); err != nil || res.Checked != 0 {
		t.Fatalf("CheckSaved = %+v, %v", res, err)
	}
}

// CheckSaved reads the reports checkBatch at a time: the number of reads
// follows the number of reports, never the size of one statement.
func TestCheckSavedReadsInBatches(t *testing.T) {
	s := newSavedZoo(t)
	saved := checkBatch
	checkBatch = 2
	t.Cleanup(func() { checkBatch = saved })
	s.saved(t, "x4", "ann", "x4", savedSpec, false)
	s.saved(t, "x5", "ann", "x5", savedSpec, false)
	res, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	// 5 reports in reads of 2: 2, 2, 1.
	if res.Checked != 5 || res.Batches != 3 {
		t.Fatalf("checked %d in %d reads, want 5 in 3", res.Checked, res.Batches)
	}
	s.saved(t, "x6", "ann", "x6", savedSpec, false)
	// 6 in reads of 2: 2, 2, 2, then an empty read ends it.
	if res, _ := CheckSaved(s.app); res.Checked != 6 || res.Batches != 4 {
		t.Fatalf("checked %d in %d reads, want 6 in 4", res.Checked, res.Batches)
	}
}

// The broken column holds at most maxBrokenProblems problems; More counts
// the rest, so the 64 KB column always fits.
func TestABrokenMarkIsBounded(t *testing.T) {
	cols := make([]string, 0, 60)
	for i := range 60 {
		cols = append(cols, fmt.Sprintf(`{"id":"c%d","path":[{"field":"nope%d"}]}`, i, i))
	}
	newZoo(t) // registers the zoo sources
	b := checkSpec([]byte(`{"version":1,"source":"zoo.feedings","columns":[` + strings.Join(cols, ",") + `]}`))
	if b == nil {
		t.Fatal("a spec with 60 bad columns has no mark")
	}
	if len(b.Problems) != maxBrokenProblems || b.More != 10 {
		t.Fatalf("problems %d, more %d", len(b.Problems), b.More)
	}
	raw, _ := json.Marshal(b)
	if len(raw) > 65536 {
		t.Fatalf("the mark is %d bytes", len(raw))
	}
}

type messagesHandler struct{ records *[]slog.Record }

func (h messagesHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h messagesHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h messagesHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h messagesHandler) WithGroup(string) slog.Handler      { return h }

// At start, dev logs each broken report with its owner; production logs one
// line with the count and the command that lists them.
func TestBrokenReportsAreLoggedAtStart(t *testing.T) {
	var records []slog.Record
	saved := log
	log = slog.New(messagesHandler{&records})
	t.Cleanup(func() { log = saved })
	res := &CheckResult{Checked: 3, Broken: []BrokenReport{
		{ID: "r1", Title: "One", Broken: Broken{Status: 422}},
		{ID: "r2", Title: "Two", Broken: Broken{Status: 400}},
	}}
	logBrokenAtStart(res, true)
	if len(records) != 2 || records[0].Level != slog.LevelWarn || records[0].Message != "saved report is broken" {
		t.Fatalf("dev logged %v", records)
	}
	records = nil
	logBrokenAtStart(res, false)
	if len(records) != 1 || records[0].Level != slog.LevelInfo || !strings.Contains(records[0].Message, "tinycld reports validate") {
		t.Fatalf("production logged %v", records)
	}
	records = nil
	logBrokenAtStart(&CheckResult{Checked: 3}, true)
	if len(records) != 0 {
		t.Fatalf("nothing broken logged %v", records)
	}
}
