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
	app, rec := auditTestAppNoCleanup(t)
	t.Cleanup(app.Cleanup)
	return app, rec
}

// auditTestAppNoCleanup leaves app.Cleanup, which runs the terminate, to the
// test.
func auditTestAppNoCleanup(t *testing.T) (*tests.TestApp, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
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
			logCreate(app, rec, requestInfoOf(nil), "widgets", DefaultLabelExtractor)
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
		logCreate(app, rec, requestInfoOf(nil), "widgets", DefaultLabelExtractor)
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

// A supervised upgrade keeps the mode on until the process exits. The
// terminate must end a parked audit write, which then logs the drop at Error
// and never writes.
func TestAuditRowParkedInReadOnlyIsDroppedAndLoggedAtTerminate(t *testing.T) {
	app, rec := auditTestAppNoCleanup(t)
	readonly.Register(app)
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ran := make(chan struct{}, 1)
	logWhenWritable(app, "widgets", rec.Id, func() { ran <- struct{}{} })
	// The terminate waits for the tail to return, so its log is written by
	// the time Cleanup returns.
	app.Cleanup()

	select {
	case <-ran:
		t.Fatal("an audit write ended by terminate must not run")
	default:
	}
	r, ok := readonlytest.Find(logs(), auditDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the audit row dropped at terminate", auditDroppedMsg)
	}
	if r.Level != slog.LevelError {
		t.Fatalf("want Error, got %v", r.Level)
	}
	if r.Attrs["recordID"] != rec.Id {
		t.Fatalf("the log must name the record, got %v", r.Attrs)
	}
}
