package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

// archiveFor builds a real archive from a source test app so the restore tests
// run against genuine output of Run rather than a hand-assembled stream.
func archiveFor(t *testing.T) ([]byte, age.Identity) {
	t.Helper()
	src := newTestApp(t)
	makeUser(t, src, "owner@example.com", "owner")
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	sink := &closeBuffer{}
	if _, err := Run(src, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: sink}); err != nil {
		t.Fatal(err)
	}
	return sink.Bytes(), id
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// resetRestoreState clears the package-level registry and restart flag so one
// test's rebuilder cannot leak into the next.
func resetRestoreState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		RegisterRebuilder(nil)
		restartMu.Lock()
		restarted = false
		restartMu.Unlock()
		restoring.Store(false)
	})
	RegisterRebuilder(nil)
	restartMu.Lock()
	restarted = false
	restartMu.Unlock()
	restoring.Store(false)
}

// dropPackage removes a package from the target's registry so the archive's
// package set no longer matches this binary's.
func dropPackage(t *testing.T, app core.App, slug string) {
	t.Helper()
	regs, err := app.FindRecordsByFilter("pkg_registry", "slug = {:slug}", "", 0, 0, dbx.Params{"slug": slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(regs) == 0 {
		t.Fatalf("no registry row for %q", slug)
	}
	for _, r := range regs {
		if err := app.Delete(r); err != nil {
			t.Fatal(err)
		}
	}
}

func readArmed(path string, out *armed) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func TestRestoreStagesAndCallsRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	owner := makeUser(t, app, "owner2@example.com", "owner")
	resetRestoreState(t)

	var gotLock format.Lockfile
	RegisterRebuilder(func(_ context.Context, lf format.Lockfile) error { gotLock = lf; return nil })

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Initiator: owner.Id})
	if err != nil {
		t.Fatal(err)
	}
	if gotLock["widgets"] != "@example/widgets@1.0.0" {
		t.Fatalf("rebuilder got %v", gotLock)
	}
	if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), "data.db")); err != nil {
		t.Fatal("data.db not staged")
	}
	if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), "storage", "col1", "rec1", "hello.txt")); err != nil {
		t.Fatal("file not staged")
	}

	var a armed
	if err := readArmed(armedPath(app), &a); err != nil {
		t.Fatalf("not armed: %v", err)
	}
	if a.ID != jobID || a.Pending != pendingDir(app, jobID) || a.Pre != preBackupPath(app, jobID) {
		t.Fatalf("armed %+v", a)
	}
	if a.Manifest.Core == "" {
		t.Fatal("the armed marker must carry the manifest the restored process reports")
	}
	if !Restoring() {
		t.Fatal("Restoring must report true once the restore is armed")
	}

	// The pre-restore backup is readable with the identity stored on the row.
	row, err := app.FindRecordById("backups", jobID)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{}
	if err := row.UnmarshalJSONField("metadata", &meta); err != nil {
		t.Fatal(err)
	}
	rawID, ok := meta["pre_restore_identity"].(string)
	if !ok {
		t.Fatalf("metadata %v has no pre-restore identity", meta)
	}
	preID, err := age.ParseX25519Identity(rawID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(preBackupPath(app, jobID))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, rep, err := format.Inspect(f, preID); err != nil || !rep.OK {
		t.Fatalf("pre-restore backup unreadable: %v", err)
	}

	// The row stays running until the restored process boots and finalizes it.
	if row.GetString("status") != "running" || row.GetString("kind") != "restore" {
		t.Fatalf("row %v", row.PublicExport())
	}
	// The live pb_data is never touched by a restore.
	if _, err := os.Stat(filepath.Join(app.DataDir(), "data.db")); err != nil {
		t.Fatal("live db missing")
	}
	// The restore records its start in the audit log before any work.
	logs, err := app.FindRecordsByFilter("audit_logs", "action = 'restore.started'", "", 0, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit rows %d, err %v", len(logs), err)
	}
}

func TestRestoreRefusesMismatchWithoutRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	dropPackage(t, app, "widgets")

	_, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id})
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("want mismatch, got %v", err)
	}
	if _, err := os.Stat(restoreDir(app)); !os.IsNotExist(err) {
		t.Fatal("a refusal must write nothing to disk")
	}
	if Restoring() {
		t.Fatal("a refused restore must not leave the process marked as restoring")
	}
}

func TestRestoreForceSkipsCheckAndRestarts(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	dropPackage(t, app, "widgets")

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), "data.db")); err != nil {
		t.Fatal("force did not stage")
	}
	if !restartRequested() {
		t.Fatal("force path must request a restart when no rebuilder exists")
	}
}

func TestRestoreEqualSetWithoutRebuilderProceeds(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	if _, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id}); err != nil {
		t.Fatal(err)
	}
	if !restartRequested() {
		t.Fatal("an equal package set with no rebuilder must restart the process itself")
	}
}

// archiveWithBrokenDB is a structurally valid archive whose data.db is not a
// database. It gets past the manifest check, so the restore reaches phase 4 and
// then fails — which is the path that has to leave the pre-restore backup behind.
func archiveWithBrokenDB(t *testing.T) ([]byte, age.Identity) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w, err := format.NewWriter(&buf, id.Recipient(), zstd.SpeedDefault)
	if err != nil {
		t.Fatal(err)
	}
	m := format.Manifest{
		Format: format.FormatV1, Created: time.Now().UTC(), Source: "docker", Kind: "manual",
		Core: "1.2.3", Lockfile: format.Lockfile{"tinycld": "tinycld@1.2.3", "widgets": "@example/widgets@1.0.0"},
		Packages: map[string]string{"widgets": "1.0.0"},
	}
	if err := w.WriteManifest(m); err != nil {
		t.Fatal(err)
	}
	junk := []byte("this is not a database")
	if err := w.WriteFile(format.MemberDB, int64(len(junk)), bytes.NewReader(junk)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), id
}

func TestRestoreBadArchiveFailsClean(t *testing.T) {
	data, id := archiveWithBrokenDB(t)
	app := newTestApp(t)
	resetRestoreState(t)

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id})
	if err == nil {
		t.Fatal("expected failure")
	}
	row, rerr := app.FindRecordById("backups", jobID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if row.GetString("status") != "failed" {
		t.Fatalf("status %q", row.GetString("status"))
	}
	if row.GetString("error") == "" {
		t.Fatal("a failed restore must record why")
	}
	if _, err := os.Stat(pendingDir(app, jobID)); !os.IsNotExist(err) {
		t.Fatal("pending dir left behind")
	}
	if _, err := os.Stat(armedPath(app)); !os.IsNotExist(err) {
		t.Fatal("still armed")
	}
	if Restoring() {
		t.Fatal("a failed restore must clear the restoring flag")
	}
	if _, err := os.Stat(preBackupPath(app, jobID)); err != nil {
		t.Fatal("the pre-restore backup must be kept after a failed restore")
	}
}

// A tampered archive must never reach the rebuilder: the rebuilder ends the
// process, so a rebuild started on an archive that cannot be trusted leaves the
// deployment booting onto whatever partially staged.
func TestRestoreTamperedArchiveNeverReachesTheRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	data[len(data)-10] ^= 0xff
	app := newTestApp(t)
	resetRestoreState(t)
	RegisterRebuilder(func(context.Context, format.Lockfile) error {
		t.Error("rebuilder must not run for a tampered archive")
		return nil
	})

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id})
	if err == nil {
		t.Fatal("expected failure")
	}
	row, rerr := app.FindRecordById("backups", jobID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if row.GetString("status") != "failed" {
		t.Fatalf("status %q", row.GetString("status"))
	}
	if _, err := os.Stat(armedPath(app)); !os.IsNotExist(err) {
		t.Fatal("still armed")
	}
	if Restoring() {
		t.Fatal("a failed restore must clear the restoring flag")
	}
}

// A restore inserts its ledger row before it reads a single byte, so an archive
// that cannot even be opened is still visible in the ledger as a failed run.
func TestRestoreUnreadableArchiveStillLeavesAFailedRow(t *testing.T) {
	app := newTestApp(t)
	resetRestoreState(t)
	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader([]byte("not a backup"))}, Identity: other})
	if err == nil {
		t.Fatal("expected failure")
	}
	if jobID == "" {
		t.Fatal("the ledger row must exist before the archive is read")
	}
	row, rerr := app.FindRecordById("backups", jobID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if row.GetString("status") != "failed" {
		t.Fatalf("status %q", row.GetString("status"))
	}
	if _, err := os.Stat(armedPath(app)); !os.IsNotExist(err) {
		t.Fatal("a restore that never got past the manifest must not be armed")
	}
}

func TestSwapSourceWithoutAWaitingRestore(t *testing.T) {
	if err := SwapSource("nope", "https://example.com/x"); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("want ErrNotWaiting, got %v", err)
	}
}

func TestRestoreWaitsForSourceAndSwaps(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	RegisterRebuilder(func(context.Context, format.Lockfile) error { return nil })

	// The first server delivers half the archive then dies, and rejects every
	// retry as expired, so the restore has to block for a fresh URL.
	var calls int
	expiring := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(data[:len(data)/2])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				c, _, err := hj.Hijack()
				if err == nil {
					_ = c.Close()
				}
			}
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(expiring.Close)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		http.ServeContent(w, r, "b.age", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(good.Close)

	src := format.NewRangeSource(context.Background(), expiring.URL)
	src.SetExpiryWait(30 * time.Second)
	jobID, err := StartRestore(app, RestoreRequest{Source: src, Ranged: src, Identity: id, SourceHost: "x"})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	waited := false
	for time.Now().Before(deadline) {
		row, rerr := app.FindRecordById("backups", jobID)
		if rerr == nil && row.GetString("status") == "waiting_for_source" {
			waited = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !waited {
		t.Fatal("the row never reported waiting_for_source while the URL was expired")
	}
	if err := SwapSource(jobID, good.URL); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(time.Now().Add(20 * time.Second)) {
		if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), "data.db")); err == nil {
			return
		}
		if row, rerr := app.FindRecordById("backups", jobID); rerr == nil && row.GetString("status") == "failed" {
			t.Fatalf("restore failed after the swap: %s", row.GetString("error"))
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("restore did not complete after the swap")
}

// A staged database that is not a valid SQLite file must be refused before the
// process is told to boot on it, or the restart loops on a broken db.
func TestIntegrityCheckRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := integrityCheck(path); err == nil {
		t.Fatal("a garbage file passed the integrity check")
	}
}

func TestIntegrityCheckAcceptsARealSnapshot(t *testing.T) {
	app := newTestApp(t)
	snap := filepath.Join(t.TempDir(), "snap.db")
	if err := vacuumInto(app, snap); err != nil {
		t.Fatal(err)
	}
	if err := integrityCheck(snap); err != nil {
		t.Fatalf("a real snapshot failed the integrity check: %v", err)
	}
}
