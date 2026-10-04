package audit

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/readonly"
	"tinycld.org/core/readonly/readonlytest"
)

func auditTestApp(t *testing.T) (*tests.TestApp, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createAuditLogsCollection(t, app)
	col := core.NewBaseCollection("widgets")
	col.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", "w1")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return app, rec
}

func auditRows(t *testing.T, app core.App, recordID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("audit_logs", "resource_id = {:id}", "", 0, 0, map[string]any{"id": recordID})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestAuditRowWaitsForReadOnlyToEnd(t *testing.T) {
	app, rec := auditTestApp(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		writeWhenWritable(probe, "widgets", rec.Id, func() {
			logCreate(app, rec, nil, "widgets", DefaultLabelExtractor)
		})
	}()
	select {
	case <-waiting:
	case <-done:
		t.Fatal("the audit row was written without waiting for read-only mode to end")
	case <-time.After(5 * time.Second):
		t.Fatal("the audit write never reached the read-only wait")
	}
	if n := auditRows(t, app, rec.Id); n != 0 {
		t.Fatalf("an audit row was written while read-only: %d rows", n)
	}

	readonly.Leave()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the audit row was not written after read-only mode ended")
	}
	if n := auditRows(t, app, rec.Id); n != 1 {
		t.Fatalf("want 1 audit row after read-only mode ended, got %d", n)
	}
}

func TestAuditRowDroppedAndLoggedWhenReadOnlyOutlastsTheWait(t *testing.T) {
	app, rec := auditTestApp(t)
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writeWhenWritable(ctx, "widgets", rec.Id, func() {
		logCreate(app, rec, nil, "widgets", DefaultLabelExtractor)
	})

	if n := auditRows(t, app, rec.Id); n != 0 {
		t.Fatalf("an audit row was written while read-only: %d rows", n)
	}
	r, ok := readonlytest.Find(logs(), auditDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the dropped audit row", auditDroppedMsg)
	}
	if r.Level != slog.LevelError {
		t.Fatalf("a dropped audit row is a compliance gap and must log at Error, got %v", r.Level)
	}
	if r.Attrs["recordID"] != rec.Id || r.Attrs["collection"] != "widgets" {
		t.Fatalf("the log must name the record, got %v", r.Attrs)
	}
}
