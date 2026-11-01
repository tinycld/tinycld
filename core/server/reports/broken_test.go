package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/offboard"
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

// A mark lists at most maxBrokenProblems problems; More counts the rest.
func TestABrokenMarkIsBounded(t *testing.T) {
	cols := make([]string, 0, 60)
	for i := range 60 {
		cols = append(cols, fmt.Sprintf(`{"id":"c%d","path":[{"field":"nope%d"}]}`, i, i))
	}
	z := newZoo(t)
	b := checkSpec(z.app, []byte(`{"version":1,"source":"zoo.feedings","columns":[`+strings.Join(cols, ",")+`]}`))
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

// storedBroken is the broken column exactly as the database holds it.
func storedBroken(t *testing.T, s *savedZoo, key string) string {
	t.Helper()
	var raw sql.NullString
	if err := s.app.DB().NewQuery("SELECT broken FROM reports WHERE id = {:id}").
		Bind(dbx.Params{"id": s.id(key)}).Row(&raw); err != nil {
		t.Fatal(err)
	}
	return raw.String
}

// A spec can quote text of any length in a problem (a decoder error names
// the unknown key). The mark still fits the column's json maxSize, so every
// later save of the report passes PocketBase's size check: here the
// offboard move.
func TestAMarkFitsTheColumnWhateverTheSpecQuotes(t *testing.T) {
	s := newSavedZoo(t)
	s.saved(t, "huge", "ann", "Huge", `{"version":1,"source":"zoo.feedings","`+strings.Repeat("k", 100*1024)+`":1}`, false)
	if _, err := CheckSaved(s.app); err != nil {
		t.Fatal(err)
	}
	raw := storedBroken(t, s, "huge")
	if raw == "" || len(raw) > maxBrokenBytes {
		t.Fatalf("the stored mark is %d bytes, want 1 to %d", len(raw), maxBrokenBytes)
	}
	b := brokenOf(t, s, "huge")
	if b == nil || b.Status != 400 || b.Problems["spec"].Code != "invalid" || len(b.Problems["spec"].Message) > maxProblemMessage {
		t.Fatalf("huge is marked %v; want 400 with one cut spec message", b != nil)
	}
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeReassign, SuccessorUserID: s.id("ed")}, s.id("ad")); err != nil {
		t.Fatalf("offboard with a marked report: %v", err)
	}
	if s.ownerOf(t, "huge") != s.id("ed") {
		t.Fatal("the marked report did not move")
	}
}

// A message is cut on a rune boundary, so the mark stays valid UTF-8.
func TestAProblemMessageIsCutWhole(t *testing.T) {
	msg := cutMessage(strings.Repeat("é", maxProblemMessage))
	if len(msg) > maxProblemMessage || !utf8.ValidString(msg) || !strings.HasSuffix(msg, "é…") {
		t.Fatalf("cut to %d bytes, valid %v: %q", len(msg), utf8.ValidString(msg), msg[len(msg)-8:])
	}
	if short := "a short message"; cutMessage(short) != short {
		t.Fatal("a short message changed")
	}
}

// A mark whose problems together pass the column's limit is stored as one
// summary problem that counts them.
func TestAMarkPastTheColumnIsSummarized(t *testing.T) {
	b := &Broken{Status: 422, Problems: map[string]Problem{}}
	for i := range maxBrokenProblems {
		b.Problems[fmt.Sprintf("columns.%d.%s", i, strings.Repeat("p", 2000))] = Problem{Code: "unknown_field", Message: strings.Repeat("m", maxProblemMessage)}
	}
	b.More = 7
	raw, err := markJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxBrokenBytes {
		t.Fatalf("the mark is %d bytes", len(raw))
	}
	var got Broken
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	p, ok := got.Problems["spec"]
	if got.Status != 422 || len(got.Problems) != 1 || !ok || p.Code != "too_many_problems" || !strings.Contains(p.Message, "57 problems") {
		t.Fatalf("summary = %+v", got)
	}
}

// A check that fails after it wrote some marks still signals the reports
// collection: the marks it wrote are stored.
func TestAFailedCheckSignalsTheMarksItWrote(t *testing.T) {
	s := newSavedZoo(t)
	s.saved(t, "gone", "bob", "Gone", goneSpec, false)
	s.saved(t, "newer", "ann", "Newer", `{"version":99}`, false)
	last := max(s.id("gone"), s.id("newer"))
	if _, err := s.app.DB().NewQuery(fmt.Sprintf(
		"CREATE TRIGGER refuse_mark BEFORE UPDATE OF broken ON reports WHEN NEW.id = '%s' BEGIN SELECT RAISE(ABORT, 'refused'); END", last)).Execute(); err != nil {
		t.Fatal(err)
	}
	touched := 0
	saved := touch
	touch = func(core.App, string) { touched++ }
	t.Cleanup(func() { touch = saved })

	res, err := CheckSaved(s.app)
	if err == nil || res.Written != 1 {
		t.Fatalf("CheckSaved = written %d, err %v; want 1 written and an error", res.Written, err)
	}
	if touched != 1 {
		t.Fatalf("touched %d times, want 1", touched)
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
