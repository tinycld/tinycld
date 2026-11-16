package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestTxOnBeforeCommit(t *testing.T) {
	t.Parallel()

	newDemo2 := func(t *testing.T, app core.App, title string) *core.Record {
		t.Helper()
		collection, err := app.FindCollectionByNameOrId("demo2")
		if err != nil {
			t.Fatal(err)
		}
		record := core.NewRecord(collection)
		record.Set("title", title)
		return record
	}
	countTitle := func(t *testing.T, app core.App, title string) int {
		t.Helper()
		records, err := app.FindAllRecords("demo2", dbx.HashExp{"title": title})
		if err != nil {
			t.Fatal(err)
		}
		return len(records)
	}

	t.Run("runs once, in the transaction, before OnComplete", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		var order []string
		err := app.RunInTransaction(func(txApp core.App) error {
			txApp.TxInfo().OnComplete(func(error) error {
				order = append(order, "complete")
				return nil
			})
			// A nested call joins the outer transaction: its callback waits
			// for the outer function to return.
			err := txApp.RunInTransaction(func(inner core.App) error {
				inner.TxInfo().OnBeforeCommit(func(tx core.App) error {
					order = append(order, "before")
					if !tx.IsTransactional() {
						t.Error("Expected the callback's app to be transactional")
					}
					return tx.Save(newDemo2(t, tx, "before_commit_write"))
				})
				return nil
			})
			order = append(order, "body")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(order, ","); got != "body,before,complete" {
			t.Fatalf("Expected order body,before,complete, got %s", got)
		}
		if n := countTitle(t, app, "before_commit_write"); n != 1 {
			t.Fatalf("Expected the callback's write to commit once, got %d rows", n)
		}
	})

	t.Run("a callback registered by a callback runs", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		calls := 0
		err := app.RunInTransaction(func(txApp core.App) error {
			txApp.TxInfo().OnBeforeCommit(func(tx core.App) error {
				calls++
				tx.TxInfo().OnBeforeCommit(func(core.App) error {
					calls++
					return nil
				})
				return nil
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("Expected 2 callback calls, got %d", calls)
		}
	})

	t.Run("an error rolls back and stops the later callbacks", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		refused := errors.New("refused before commit")
		laterRan := false
		err := app.RunInTransaction(func(txApp core.App) error {
			if err := txApp.Save(newDemo2(t, txApp, "rolled_back_write")); err != nil {
				return err
			}
			txApp.TxInfo().OnBeforeCommit(func(core.App) error { return refused })
			txApp.TxInfo().OnBeforeCommit(func(core.App) error {
				laterRan = true
				return nil
			})
			return nil
		})
		if !errors.Is(err, refused) {
			t.Fatalf("Expected the callback's error, got %v", err)
		}
		if laterRan {
			t.Fatal("Expected no callback after the one that failed")
		}
		if n := countTitle(t, app, "rolled_back_write"); n != 0 {
			t.Fatalf("Expected the write to roll back, got %d rows", n)
		}
	})

	t.Run("does not run when the transaction's function fails", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		failed := errors.New("body failed")
		ran := false
		err := app.RunInTransaction(func(txApp core.App) error {
			txApp.TxInfo().OnBeforeCommit(func(core.App) error {
				ran = true
				return nil
			})
			return failed
		})
		if !errors.Is(err, failed) {
			t.Fatalf("Expected the function's error, got %v", err)
		}
		if ran {
			t.Fatal("Expected no callback after a failed function")
		}
	})
}
