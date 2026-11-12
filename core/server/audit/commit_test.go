package audit

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestAfterCommit_RunsAtOnceOutsideATransaction(t *testing.T) {
	app, _ := auditTestApp(t)
	ran := false
	afterCommit(app, func() { ran = true })
	if !ran {
		t.Fatal("the write did not run for a request outside a transaction")
	}
}

func TestAfterCommit_WaitsForTheCommit(t *testing.T) {
	app, _ := auditTestApp(t)
	ran := false
	err := app.RunInTransaction(func(tx core.App) error {
		afterCommit(tx, func() { ran = true })
		if ran {
			t.Fatal("the write ran before the transaction committed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("the write did not run after the commit")
	}
}

func TestAfterCommit_DropsTheWriteOnRollback(t *testing.T) {
	app, _ := auditTestApp(t)
	ran := false
	rollback := errors.New("rolled back by the test")
	err := app.RunInTransaction(func(tx core.App) error {
		afterCommit(tx, func() { ran = true })
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("err = %v, want the test's rollback", err)
	}
	if ran {
		t.Fatal("an audit write ran for a transaction that rolled back")
	}
}
