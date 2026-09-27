package backup

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

func TestVerifyWithoutARestoreRow(t *testing.T) {
	app := newTestApp(t)
	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.IntegrityOK || !rep.OK {
		t.Fatalf("report %+v", rep)
	}
	if len(rep.Collections) != 0 {
		t.Fatalf("nothing to compare, got %+v", rep.Collections)
	}
}

func TestVerifyComparesTheRestoredManifest(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")

	row := newRow(app, KindRestore, "", "")
	row.Set("status", "succeeded")
	row.Set("manifest", format.Manifest{Counts: format.Counts{Collections: map[string]int{"users": 5}, Files: 9}})
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.IntegrityOK {
		t.Fatal("the live database failed its integrity check")
	}
	if rep.OK {
		t.Fatalf("a mismatched count must not report OK: %+v", rep)
	}
	got, ok := rep.Collections["users"]
	if !ok {
		t.Fatalf("collections %+v", rep.Collections)
	}
	if got[0] != 5 || got[1] != 1 {
		t.Fatalf("users %v, want [5 1]", got)
	}
	if rep.Files[0] != 9 || rep.Files[1] != 1 {
		t.Fatalf("files %v, want [9 1]", rep.Files)
	}
}

func TestVerifyAgreesWithTheRealCounts(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")

	// Production order: the archive's manifest is built while the backup runs,
	// and the succeeded restore row is inserted afterwards, by the finalizer.
	// That insert lands in the ledger collection the manifest counted, so a
	// Verify that held the ledger to EQUALITY could never agree.
	m, err := buildManifest(app, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	m.Counts.Files = 1

	row := newRow(app, KindRestore, "", "")
	row.Set("status", "succeeded")
	row.Set("manifest", m)
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("report %+v", rep)
	}
	// The ledger is REPORTED but bounded, not omitted. Omitting it would hide a
	// shortfall in it; the bound is what lets it be over the manifest (the row
	// the finalize just wrote) without being allowed to be under.
	got, ok := rep.Collections[collection]
	if !ok {
		t.Fatalf("the ledger is missing from the report; a bounded collection is still reported: %+v", rep.Collections)
	}
	if got[1] < got[0] {
		t.Fatalf("ledger = %v; the live count must never be BELOW the manifest's", got)
	}
}

// An unknown collection cannot be counted. It must show as a mismatch rather
// than fail the whole report, so the operator sees which one is missing.
func TestVerifyReportsAMissingCollection(t *testing.T) {
	app := newTestApp(t)
	row := newRow(app, KindRestore, "", "")
	row.Set("status", "succeeded")
	row.Set("manifest", format.Manifest{Counts: format.Counts{Collections: map[string]int{"gone": 2}}})
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("a collection the restored data does not have must not report OK")
	}
	if rep.Collections["gone"] != [2]int{2, -1} {
		t.Fatalf("gone %v, want [2 -1]", rep.Collections["gone"])
	}
}

func TestMaintenanceMiddlewareServesWhileIdle(t *testing.T) {
	resetRestoreState(t)
	rec, re := requestEvent("/api/collections/users/records")
	if err := MaintenanceMiddleware()(re); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("an idle server must pass the request on, wrote %q", rec.Body.String())
	}
}

func TestMaintenanceMiddlewareRefusesWhileRestoring(t *testing.T) {
	resetRestoreState(t)
	restoring.Store(true)
	rec, re := requestEvent("/api/collections/users/records")
	if err := MaintenanceMiddleware()(re); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	body := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "restoring" {
		t.Fatalf("body %v", body)
	}
}

func TestMaintenanceMiddlewareKeepsHealthReachable(t *testing.T) {
	resetRestoreState(t)
	restoring.Store(true)
	rec, re := requestEvent("/api/health")
	if err := MaintenanceMiddleware()(re); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("health must stay reachable so a supervisor can probe: %d %q", rec.Code, rec.Body.String())
	}
}

func TestFinalizeRestoreWithoutASwap(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}
	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("an ordinary boot must not write a restore row, got %d", len(rows))
	}
}

func TestFinalizeRestoreRecordsTheRestoredArchive(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	restoring.Store(true)

	prev := previousDir(app, "r1")
	if err := os.MkdirAll(prev, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prev, "data.db"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	pre := preBackupPath(app, "r1")
	if err := os.MkdirAll(filepath.Dir(pre), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre, []byte("pre"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := format.Manifest{Core: "1.2.3", Counts: format.Counts{Collections: map[string]int{"users": 4}, Files: 7}}
	raw, err := json.Marshal(armed{ID: "r1", Pending: pendingDir(app, "r1"), Pre: pre, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swappedPath(app), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}

	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("succeeded restore rows %d", len(rows))
	}
	var got format.Manifest
	if err := rows[0].UnmarshalJSONField("manifest", &got); err != nil {
		t.Fatal(err)
	}
	if got.Core != "1.2.3" || got.Counts.Collections["users"] != 4 {
		t.Fatalf("manifest %+v — Verify has nothing to compare without it", got)
	}
	if rows[0].GetDateTime("finished").IsZero() {
		t.Fatal("the finalized row has no finish time")
	}

	if _, err := os.Stat(prev); !os.IsNotExist(err) {
		t.Fatal("the previous data was not dropped")
	}
	if _, err := os.Stat(pre); !os.IsNotExist(err) {
		t.Fatal("the pre-restore backup was not dropped")
	}
	if _, err := os.Stat(swappedPath(app)); !os.IsNotExist(err) {
		t.Fatal("the swapped marker was not cleared")
	}
	if Restoring() {
		t.Fatal("maintenance mode must end once the restore is finalized")
	}

	notifs, err := app.FindRecordsByFilter("notifications", "type = 'core.restore.succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs) == 0 {
		t.Fatal("nobody was told the restore worked")
	}
	logs, err := app.FindRecordsByFilter("audit_logs", "action = 'restore.succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit rows %d", len(logs))
	}

	// Verify now has a manifest to compare against.
	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Collections["users"][0] != 4 {
		t.Fatalf("verify %+v", rep)
	}
	// This fixture's manifest counts only "users", so the collections the finalize
	// wrote into are not in it and are not reported — the loop reports what the
	// MANIFEST counted, nothing more. That they are above their own backup-time
	// counts, and bounded rather than skipped, is asserted where a manifest
	// actually carries them: TestVerifyReportsOKAfterAFinalizedRestore and
	// TestVerifyReportsAShortfallInASelfWrittenCollection.
	if _, ok := rep.Collections["users"]; !ok {
		t.Errorf("users is missing from the report, so nothing was compared at all: %+v", rep.Collections)
	}
}

// A restore of data that genuinely matches its manifest must report OK.
//
// It could not, and the failure was total rather than cosmetic. Verify excluded
// only the backup ledger, but announceRestore also writes a notification and an
// audit entry as the LAST act of the restore — so those two collections are always
// at least one row ahead of the manifest, and one mismatch clears rep.OK. A
// restore drill reads exactly that field to decide whether a backup is
// restorable, so every drill of a perfectly good backup failed and the one signal
// that says "this org can be recovered" was permanently false. The hosted DR e2e
// is what surfaced it.
//
// The manifest here is built from the live data BEFORE the finalize, which is
// production's order: the archive is written by the backup, and the rows the
// restore adds land afterwards.
func TestVerifyReportsOKAfterAFinalizedRestore(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	restoring.Store(true)

	manifest, err := buildManifest(app, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	// One file, to match what newTestApp's storage holds; the count itself is not
	// what this test is about.
	manifest.Counts.Files = 1

	raw, err := json.Marshal(armed{ID: "r1", Pending: pendingDir(app, "r1"), Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(swappedPath(app)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swappedPath(app), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}

	// The finalize really did write into all three, or this test would pass for
	// the wrong reason — on a build where nothing is written, an exclusion is
	// untested.
	for _, q := range []struct{ collection, filter string }{
		{collection, "kind = 'restore' && status = 'succeeded'"},
		{"notifications", "type = 'core.restore.succeeded'"},
		{"audit_logs", "action = 'restore.succeeded'"},
	} {
		rows, err := app.FindRecordsByFilter(q.collection, q.filter, "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatalf("the finalize wrote no %s row, so this test does not exercise the exclusion", q.collection)
		}
	}

	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("a restore whose data matches its manifest must report OK; a drill reads this field: %+v", rep)
	}
}

// requestEvent builds a RequestEvent with no next handler: Next() is then a
// no-op that writes nothing, so an empty response body means the middleware
// passed the request on rather than answering it.
func requestEvent(path string) (*httptest.ResponseRecorder, *core.RequestEvent) {
	rec := httptest.NewRecorder()
	re := &core.RequestEvent{}
	re.Request = httptest.NewRequest(http.MethodGet, path, nil)
	re.Response = rec
	return rec, re
}

// The whole chain on real output: an archive built by Run, staged and armed by
// Restore, then swapped in by the boot-time helper. FinalizeRestore itself is
// asserted separately — it needs a process booted on the swapped data, and this
// app still holds the replaced database open.
func TestApplyPendingRestoreCompletesARealRestore(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return true })

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	// LedgerPath is overridden in tests, so the boot helper — which derives its
	// paths from the data dir alone — is pointed at the same state directory.
	dataDir := filepath.Join(ledgerPathOverride, "pb_data")
	if err := os.Rename(app.DataDir(), dataDir); err != nil {
		t.Fatal(err)
	}

	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	// The archive's own storage file is what the restored data dir now carries.
	if _, err := os.Stat(filepath.Join(dataDir, "storage", "col1", "rec1", "hello.txt")); err != nil {
		t.Fatalf("the archive's files were not swapped in: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "data.db")); err != nil {
		t.Fatalf("the archive's database was not swapped in: %v", err)
	}
	if _, err := os.Stat(previousDir(app, jobID)); err != nil {
		t.Fatalf("the previous data was not kept: %v", err)
	}
	if _, err := os.Stat(pendingDir(app, jobID)); !os.IsNotExist(err) {
		t.Fatal("the staging directory was not consumed")
	}

	var a armed
	if err := readArmed(swappedPath(app), &a); err != nil {
		t.Fatalf("no swapped marker: %v", err)
	}
	if a.ID != jobID || a.Manifest.Core == "" {
		t.Fatalf("the marker the finalizer reads is incomplete: %+v", a)
	}
	if !Restoring() {
		t.Fatal("the swapped-in process serves 503 until it finalizes")
	}
}

// FinalizeRestore can die between saving the row and clearing the marker. The
// next boot sees the marker again and must not insert a second succeeded row or
// notify a second time.
func TestFinalizeRestoreIsIdempotent(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	restoring.Store(true)

	marker, err := json.Marshal(armed{
		ID:       "r1",
		Pending:  pendingDir(app, "r1"),
		Manifest: format.Manifest{Core: "1.2.3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	write := func() {
		if err := os.MkdirAll(filepath.Dir(swappedPath(app)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(swappedPath(app), marker, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write()
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}
	// The marker is back, as a crash before its removal would leave it.
	write()
	restoring.Store(true)
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}

	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("succeeded restore rows %d, want 1", len(rows))
	}
	notifs, err := app.FindRecordsByFilter("notifications", "type = 'core.restore.succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications %d, want 1", len(notifs))
	}
	if _, err := os.Stat(swappedPath(app)); !os.IsNotExist(err) {
		t.Fatal("the second pass left the marker behind")
	}
	if Restoring() {
		t.Fatal("the second pass must still end maintenance mode")
	}
}

// A restore of a DIFFERENT archive after an earlier one must still be recorded:
// idempotence keys on the job id, not on "any succeeded restore exists".
func TestFinalizeRestoreRecordsASecondRestore(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")

	for _, id := range []string{"r1", "r2"} {
		raw, err := json.Marshal(armed{ID: id, Pending: pendingDir(app, id)})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(swappedPath(app)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(swappedPath(app), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		restoring.Store(true)
		if err := FinalizeRestore(app); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'succeeded'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("succeeded restore rows %d, want 2", len(rows))
	}
}

// The serve-time finalizer is the first point at which a database exists, so it
// is what turns a rollback marker into something an operator can see.
func TestFinalizeRestoreReportsARollback(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")

	manifest := format.Manifest{Core: "1.2.3"}
	raw, err := json.Marshal(rolledBack{
		Armed:    armed{ID: "r1", Manifest: manifest},
		Reason:   rollbackReason,
		RolledAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rolledBackDir(app), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(rolledBackDir(app), "r1.json")
	if err := os.WriteFile(marker, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}

	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'failed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("failed restore rows %d, want 1", len(rows))
	}
	if msg := rows[0].GetString("error"); msg != rollbackReason {
		t.Fatalf("error %q", msg)
	}
	var meta map[string]any
	if err := rows[0].UnmarshalJSONField("metadata", &meta); err != nil {
		t.Fatal(err)
	}
	if meta["restored_from_job"] != "r1" {
		t.Fatalf("metadata %+v", meta)
	}
	notifs, err := app.FindRecordsByFilter("notifications", "type = 'core.restore.failed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs) == 0 {
		t.Fatal("administrators were not told the restore was rolled back")
	}
	audits, err := app.FindRecordsByFilter("audit_logs", "action = 'restore.failed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) == 0 {
		t.Fatal("the rollback was not audited")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the marker must go once it is recorded")
	}
}

// The marker's removal is the last step, so a crash after the insert brings it
// back. A second pass must not write a second row.
func TestFinalizeRestoreReportsARollbackOnlyOnce(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	write := func() {
		raw, err := json.Marshal(rolledBack{Armed: armed{ID: "r1"}, Reason: rollbackReason})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(rolledBackDir(app), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rolledBackDir(app), "r1.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}
	write()
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}
	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'failed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("failed restore rows %d, want 1", len(rows))
	}
}

// A collection the restore writes into is held to a LOWER BOUND, not skipped.
//
// The bound is what keeps real loss visible. Those collections are always at
// least one row ahead of the manifest through the restore's own doing, so equality
// is unachievable — but rows can still be MISSING from them, and a short audit log
// or notification history is loss on exactly the collections an operator would
// later go to for evidence of what happened. Skipping them reported that as fine.
func TestVerifyBoundsTheCollectionsARestoreWritesInto(t *testing.T) {
	for _, tc := range []struct {
		name       string
		collection string
		want, live int
		ok         bool
	}{
		// Ahead of the manifest: the restore's own rows. Fine.
		{"ledger ahead", collection, 3, 4, true},
		{"notifications ahead", "notifications", 2, 5, true},
		{"audit ahead", "audit_logs", 1, 2, true},
		// Exactly equal is also within the bound.
		{"audit equal", "audit_logs", 4, 4, true},
		// SHORT of the manifest: rows that did not come across. Not fine — this
		// is the case a skip reported as healthy.
		{"audit short", "audit_logs", 4, 3, false},
		{"notifications short", "notifications", 2, 0, false},
		{"ledger short", collection, 3, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := countAgrees(tc.collection, tc.want, tc.live); got != tc.ok {
				t.Fatalf("countAgrees(%q, want=%d, live=%d) = %v, want %v",
					tc.collection, tc.want, tc.live, got, tc.ok)
			}
		})
	}

	// An ordinary collection is still equality in both directions: the restore
	// replaced the whole database, so a row either way is data that disagrees.
	if countAgrees("users", 4, 5) {
		t.Error("an ordinary collection ABOVE the manifest must not pass; the restore replaced the whole database")
	}
	if countAgrees("users", 4, 3) {
		t.Error("an ordinary collection below the manifest must not pass")
	}

	// -1 is the "could not be counted" marker and must fail every test,
	// including the bound: unreadable is a stronger signal than wrong, not a
	// weaker one. Without this, an uncountable self-written collection would
	// pass any manifest count of 0.
	for _, name := range []string{"users", "audit_logs", collection} {
		if countAgrees(name, 0, -1) {
			t.Errorf("an uncountable %s passed; -1 means unreadable, which is worse than a wrong number", name)
		}
	}
}

// The bound has to hold end to end, not just in the helper: a real finalized
// restore whose audit log came back SHORT must not report ok.
func TestVerifyReportsAShortfallInASelfWrittenCollection(t *testing.T) {
	resetRestoreState(t)
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")

	// A manifest claiming more audit rows than the live data will ever hold. The
	// finalize adds one of its own, so the live count lands well under 50 — which
	// is precisely the shortfall the bound exists to catch.
	manifest, err := buildManifest(app, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Counts.Collections["audit_logs"] = 50
	manifest.Counts.Files = 1

	raw, err := json.Marshal(armed{ID: "r1", Pending: pendingDir(app, "r1"), Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(swappedPath(app)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swappedPath(app), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	restoring.Store(true)
	if err := FinalizeRestore(app); err != nil {
		t.Fatal(err)
	}

	rep, err := Verify(app)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("a restore that lost audit rows must not report OK: %+v", rep)
	}
	got, ok := rep.Collections["audit_logs"]
	if !ok {
		t.Fatalf("audit_logs is missing from the report; a bounded collection must still be REPORTED: %+v", rep.Collections)
	}
	if got[0] != 50 || got[1] >= 50 {
		t.Fatalf("audit_logs = %v, want [50, <50] — the shortfall is the finding", got)
	}
}
