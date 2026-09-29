package core_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// A hook that swaps e.App for its own transaction app and does not restore it
// must not leak that finished transaction into the after-success hooks.
// Otherwise every after hook (realtime access checks among them) queries
// through a committed transaction and fails.
func TestTransactionFromInnerHookWithoutAppRestore(t *testing.T) {
	t.Parallel()

	leakTx := func(e *core.RecordEvent) error {
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			return e.Next()
		})
	}

	checkApp := func(t *testing.T) func(e *core.RecordEvent) error {
		return func(e *core.RecordEvent) error {
			if e.App.IsTransactional() {
				t.Error("Expected e.App to be non-transactional")
			}
			if _, err := e.App.FindFirstRecordByFilter("demo2", "1=1"); err != nil {
				t.Errorf("Failed to perform a db query after tx success: %v", err)
			}
			return e.Next()
		}
	}

	t.Run("create", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		app.OnRecordCreate("demo2").BindFunc(leakTx)
		app.OnRecordAfterCreateSuccess("demo2").BindFunc(checkApp(t))

		collection, err := app.FindCollectionByNameOrId("demo2")
		if err != nil {
			t.Fatal(err)
		}
		record := core.NewRecord(collection)
		record.Set("title", "test_inner_tx_no_restore")
		if err := app.Save(record); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if app.EventCalls["OnRecordAfterCreateSuccess"] != 1 {
			t.Fatalf("Expected 1 OnRecordAfterCreateSuccess call, got %d", app.EventCalls["OnRecordAfterCreateSuccess"])
		}
	})

	t.Run("update", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		app.OnRecordUpdate("demo2").BindFunc(leakTx)
		app.OnRecordAfterUpdateSuccess("demo2").BindFunc(checkApp(t))

		record, err := app.FindFirstRecordByFilter("demo2", "1=1")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.Save(record); err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		if app.EventCalls["OnRecordAfterUpdateSuccess"] != 1 {
			t.Fatalf("Expected 1 OnRecordAfterUpdateSuccess call, got %d", app.EventCalls["OnRecordAfterUpdateSuccess"])
		}
	})

	t.Run("delete", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		app.OnRecordDelete("demo2").BindFunc(leakTx)
		app.OnRecordAfterDeleteSuccess("demo2").BindFunc(checkApp(t))

		record, err := app.FindFirstRecordByFilter("demo2", "1=1")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.Delete(record); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if app.EventCalls["OnRecordAfterDeleteSuccess"] != 1 {
			t.Fatalf("Expected 1 OnRecordAfterDeleteSuccess call, got %d", app.EventCalls["OnRecordAfterDeleteSuccess"])
		}
	})
}
