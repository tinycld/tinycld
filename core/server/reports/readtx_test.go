package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// runOn loads raw and runs it as user on app (the zoo's app or a txApp).
func (z *zoo) runOn(t *testing.T, app core.App, user, raw string, opts Options) (*Result, error) {
	t.Helper()
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return Run(context.Background(), app, Request{Spec: spec, Loaded: info, Params: map[string]json.RawMessage{}, Options: opts, Auth: z.recs[user]})
}

// cents reads a money cell as a whole number.
func cents(t *testing.T, v any) int64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case nil:
		return 0
	}
	t.Fatalf("cell %#v is not a number", v)
	return 0
}

// sumKind adds the cost cells of every row of kind.
func sumKind(t *testing.T, res *Result, kind string) int64 {
	t.Helper()
	var sum int64
	for _, r := range res.Rows {
		if r.Kind == kind {
			sum += cents(t, r.Cells["cost"])
		}
	}
	return sum
}

// onRead calls write once, before the run's nth read statement (1-based): so
// the statements before it have run and the statements from it on have not.
func onRead(t *testing.T, n int, write func()) *int {
	t.Helper()
	count := 0
	observeRead = func(string, dbx.Params) {
		count++
		if count == n {
			write()
		}
	}
	t.Cleanup(func() { observeRead = nil })
	return &count
}

// A grouped run with its groups open reads a page statement, a Total
// statement and a detail statement. A feeding saved after the first of them
// and before the others is in none of them: the run reads one snapshot, so
// its Total is the sum of its group subtotals and of its detail rows.
func TestARunReadsOneSnapshot(t *testing.T) {
	z := newZoo(t)
	count := onRead(t, 2, func() {
		z.feeding(t, "late", z.recs["leo"], 1000, "2026-03-15", 10)
	})
	res, err := z.runOn(t, z.app, "ann", feedingsBySpecies, Options{ExpandAll: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *count < 3 {
		t.Fatalf("the run read %d statements, want the page, Total and detail statements", *count)
	}
	total := cents(t, lastTotal(t, res).Cells["cost"])
	subtotals := sumKind(t, res, KindSubtotal)
	details := sumKind(t, res, KindDetail)
	if total != subtotals || total != details {
		t.Fatalf("Total %d, group subtotals add to %d, detail rows add to %d: the run read more than one snapshot\nrows %v\ncost %v", total, subtotals, details, rowsOf(res, "id"), rowsOf(res, "cost"))
	}
	if notes := rowsOf(res, "note"); slices.Contains(notes, any("late")) {
		t.Fatalf("the feeding saved during the run is a detail row: %v", notes)
	}
	if total != 4500 {
		t.Fatalf("Total %d, want 4500 (f1–f4, without the feeding saved during the run)", total)
	}
}

// A run given the txApp of a caller's transaction reads that transaction: it
// sees a row the transaction saved and has not committed, and it leaves the
// transaction open for the caller's next write, which commits.
func TestARunInATransactionReadsThatTransaction(t *testing.T) {
	z := newZoo(t)
	var after *core.Record
	err := z.app.RunInTransaction(func(txApp core.App) error {
		feedings, err := txApp.FindCollectionByNameOrId("zoo_feedings")
		if err != nil {
			return err
		}
		inTx := core.NewRecord(feedings)
		inTx.Load(map[string]any{"animal": z.id("leo"), "cost": 1000, "currency": "USD", "fed": "2026-03-15 00:00:00.000Z", "grams": 10, "note": "intx", "due": "2026-03-15"})
		if err := txApp.Save(inTx); err != nil {
			return err
		}
		res, err := z.runOn(t, txApp, "ann", feedingsBySpecies, Options{ExpandAll: true})
		if err != nil {
			return fmt.Errorf("Run: %w", err)
		}
		if notes := rowsOf(res, "note"); !slices.Contains(notes, any("intx")) {
			return fmt.Errorf("the run does not see the transaction's own row: %v", notes)
		}
		if got := cents(t, lastTotal(t, res).Cells["cost"]); got != 5500 {
			return fmt.Errorf("Total %d, want 5500 (f1–f4 and the transaction's row)", got)
		}
		after = core.NewRecord(feedings)
		after.Load(map[string]any{"animal": z.id("leo"), "cost": 1, "currency": "USD", "fed": "2026-03-16 00:00:00.000Z", "grams": 1, "note": "after", "due": "2026-03-16"})
		if err := txApp.Save(after); err != nil {
			return fmt.Errorf("a write after the run, in the same transaction: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := z.app.FindRecordById("zoo_feedings", after.Id); err != nil {
		t.Fatalf("the write after the run did not commit: %v", err)
	}
}

// checkpointBlocked writes a row, then checkpoints the WAL without waiting.
// It reports true when a read transaction still holds a snapshot older than
// that write: the checkpoint cannot copy every frame of the log.
func checkpointBlocked(t *testing.T, z *zoo, key string) bool {
	t.Helper()
	z.feeding(t, key, z.recs["kit"], 1, "2026-03-31", 1)
	var res struct {
		Busy         int `db:"busy"`
		Log          int `db:"log"`
		Checkpointed int `db:"checkpointed"`
	}
	if err := z.app.NonconcurrentDB().NewQuery("PRAGMA wal_checkpoint(PASSIVE)").One(&res); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if res.Log <= 0 {
		t.Fatalf("checkpoint log %d: the test database is not in WAL mode with frames", res.Log)
	}
	return res.Busy != 0 || res.Checkpointed < res.Log
}

// A run that fails after its first statement, by an error or a panic, ends
// its read transaction: no snapshot is left that holds the WAL.
func TestARunThatFailsEndsItsReadTransaction(t *testing.T) {
	z := newZoo(t)

	// The observable works: an open read transaction blocks the checkpoint,
	// and its end frees it.
	db, ok := z.app.ConcurrentDB().(*dbx.DB)
	if !ok {
		t.Fatalf("ConcurrentDB is %T", z.app.ConcurrentDB())
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.NewQuery("SELECT count(*) FROM zoo_feedings").Row(&n); err != nil {
		t.Fatal(err)
	}
	if !checkpointBlocked(t, z, "probe1") {
		t.Fatal("an open read transaction does not block the checkpoint: the observable shows nothing")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if checkpointBlocked(t, z, "probe2") {
		t.Fatal("the checkpoint is blocked with no read transaction open")
	}

	// The statement's own time limit (not the run's context) ends the second
	// statement, so the run returns an error with its transaction's context
	// still live. checkTopics would read that statement again under the spent
	// limit; this test is not about topics.
	t.Run("an error", func(t *testing.T) {
		savedCheck := checkTopics
		checkTopics = nil
		t.Cleanup(func() { checkTopics = savedCheck })
		withLimits(t, func(*limits) {})
		onRead(t, 2, func() { runLimits.timeout = time.Nanosecond })
		if _, err := z.runOn(t, z.app, "ann", feedingsBySpecies, Options{ExpandAll: true}); err == nil {
			t.Fatal("the run did not fail at its second statement")
		}
		if checkpointBlocked(t, z, "afterError") {
			t.Fatal("after a failed run a read transaction still holds the WAL")
		}
	})

	t.Run("a panic", func(t *testing.T) {
		onRead(t, 2, func() { panic("a read panics") })
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("the run did not panic at its second statement")
				}
			}()
			_, _ = z.runOn(t, z.app, "ann", feedingsBySpecies, Options{ExpandAll: true})
		}()
		if checkpointBlocked(t, z, "afterPanic") {
			t.Fatal("after a run that panicked a read transaction still holds the WAL")
		}
	})
}
