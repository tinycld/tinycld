package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

// archiveFor builds a real archive from a source test app so the restore tests
// run against genuine output of Run rather than a hand-assembled stream.
//
// The manual-run ceiling is counted in a package-level slice, so every archive a
// test builds spends one of a real deployment's ten daily runs. Fixture setup is
// not a person clicking "Back up now", so the counter is cleared first: without
// this the suite fails once it holds ten tests that build an archive, whichever
// one happens to run eleventh.
func archiveFor(t *testing.T) ([]byte, age.Identity) {
	t.Helper()
	resetDailyLimitForTesting()
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

// The restore hands its interlock claim to the rebuilder instead of releasing it
// and letting the rebuilder take a fresh one. That window was real: by phase 6 a
// restore has already spent a pre-restore backup and staged the entire archive,
// so an install claiming the interlock in between made the rebuilder fail with
// ErrBusy and threw all of that away.
//
// Two things are asserted, and both matter. The job the rebuilder receives is the
// one the restore claimed (same pointer, and it is the CURRENT holder while the
// rebuilder runs), so nothing could have claimed the interlock in between. And it
// is released exactly once afterwards.
func TestRestoreHandsItsClaimToTheRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)

	var gotJob *installjob.Job
	var heldDuringRebuild *installjob.Job
	RegisterRebuilder(func(_ context.Context, job *installjob.Job, _ format.Lockfile) error {
		gotJob = job
		heldDuringRebuild = installjob.Current()
		installjob.Release(job)
		return nil
	})

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id})
	if err != nil {
		t.Fatal(err)
	}
	if gotJob == nil {
		t.Fatal("the rebuilder was never called")
	}
	if heldDuringRebuild != gotJob {
		t.Fatalf("the interlock holder during the rebuild was %v, want the job the "+
			"rebuilder was handed — a release/re-claim window would let an install "+
			"in and cost a staged archive", heldDuringRebuild)
	}
	// The claim is keyed to the restore's own ledger row, so a busy response
	// elsewhere names the restore rather than an anonymous job.
	if gotJob.ID != jobID {
		t.Fatalf("the rebuilder's job id is %q, want the restore's row id %q", gotJob.ID, jobID)
	}
	if gotJob.Action != "restore" {
		t.Fatalf("the rebuilder's job action is %q, want restore", gotJob.Action)
	}
	// Released exactly once: the rebuilder released it, and runRestore's own
	// unwind must not have taken it back or released a second time.
	if installjob.Running() {
		t.Fatal("the interlock is still held after the rebuilder released it")
	}
}

// The interlock is process-wide, so a rebuilder that fails WITHOUT releasing
// would wedge the whole deployment: no backup, restore or package job could ever
// claim it again. The handover contract says the rebuilder releases, but the cost
// of one implementer forgetting is total, so runRestore releases on the error
// return too. Release compares identity, so the belt and braces cannot evict a
// later holder.
func TestRestoreReleasesTheClaimWhenTheRebuilderFailsWithoutIt(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)

	boom := errors.New("rebuild exploded")
	RegisterRebuilder(func(_ context.Context, _ *installjob.Job, _ format.Lockfile) error {
		return boom // deliberately does NOT release
	})

	if _, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id}); !errors.Is(err, boom) {
		t.Fatalf("Restore err = %v, want the rebuilder's error", err)
	}
	if installjob.Running() {
		t.Fatal("the interlock is still held after a rebuilder failed without " +
			"releasing it — nothing could ever claim it again")
	}
}

func TestRestoreStagesAndCallsRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	owner := makeUser(t, app, "owner2@example.com", "owner")
	resetRestoreState(t)

	var gotLock format.Lockfile
	RegisterRebuilder(func(_ context.Context, job *installjob.Job, lf format.Lockfile) error {
		gotLock = lf
		installjob.Release(job)
		return nil
	})

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
	RegisterRebuilder(func(_ context.Context, job *installjob.Job, _ format.Lockfile) error {
		defer installjob.Release(job)
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
	RegisterRebuilder(func(_ context.Context, job *installjob.Job, _ format.Lockfile) error {
		installjob.Release(job)
		return nil
	})

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
	staged := time.Now().Add(20 * time.Second)
	for time.Now().Before(staged) {
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

// archiveWithSymlinkMember hand-builds the age → zstd → tar pipeline, because
// format.Writer only ever emits regular files and the point of this fixture is a
// member that is not one. A symlink named storage/x passes the name check — the
// name lands inside the staging directory — so only the type check can refuse it.
func archiveWithSymlinkMember(t *testing.T) ([]byte, age.Identity) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	ageW, err := age.Encrypt(&buf, id.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	zw, err := zstd.NewWriter(ageW, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(zw)

	writeReg := func(name string, body []byte) string {
		hdr := &tar.Header{
			Name: name, Mode: 0o600, Size: int64(len(body)),
			ModTime: time.Now().UTC(), Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}

	manifest, err := json.MarshalIndent(format.Manifest{
		Format: format.FormatV1, Created: time.Now().UTC(), Source: "docker", Kind: "manual",
		Core: "1.2.3", Lockfile: format.Lockfile{"tinycld": "tinycld@1.2.3"},
		Packages: map[string]string{"widgets": "1.0.0"},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestSum := writeReg(format.MemberManifest, manifest)

	// The member under test: a symlink that points at somewhere it has no
	// business reaching.
	if err := tw.WriteHeader(&tar.Header{
		Name: format.StoragePrefix + "x", Mode: 0o777, Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd", ModTime: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	// The symlink IS listed in checksums.txt, with the hash of the empty body a
	// symlink member carries. Without the type check the archive therefore
	// verifies cleanly and the entry is judged only on its name — so this fixture
	// proves the type check is the only thing refusing it, not the checksums.
	empty := sha256.Sum256(nil)
	sums := manifestSum + "  " + format.MemberManifest + "\n" +
		hex.EncodeToString(empty[:]) + "  " + format.StoragePrefix + "x\n"
	writeReg(format.MemberChecksums, []byte(sums))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ageW.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), id
}

// A backup contains regular files and nothing else. An archive carrying a
// symlink is either a different format or an attempt to make a later write land
// outside the staging directory, so staging must refuse it outright.
func TestRestoreRefusesANonRegularMember(t *testing.T) {
	data, id := archiveWithSymlinkMember(t)
	app := newTestApp(t)
	resetRestoreState(t)

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id})
	if err == nil {
		t.Fatal("a symlink member was accepted")
	}
	if !errors.Is(err, format.ErrFormat) {
		t.Fatalf("want ErrFormat, got %v", err)
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error does not name the reason: %v", err)
	}
	// Nothing was followed: the staging directory holds no such entry, by any
	// kind, and the link target was never touched.
	if _, err := os.Lstat(filepath.Join(pendingDir(app, jobID), "storage", "x")); !os.IsNotExist(err) {
		t.Fatalf("the symlink member reached the staging directory: %v", err)
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

// Force is the operator saying "this binary, that data". A rebuild would
// silently overrule them, so Force restarts onto the current package set even
// when a rebuilder is registered.
func TestRestoreForceSkipsTheRebuilder(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	RegisterRebuilder(func(_ context.Context, job *installjob.Job, _ format.Lockfile) error {
		defer installjob.Release(job)
		t.Error("Force must not call the rebuilder")
		return nil
	})

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), "data.db")); err != nil {
		t.Fatal("force did not stage")
	}
	if !restartRequested() {
		t.Fatal("Force must restart the process itself rather than rebuild")
	}
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

// The staged tree becomes pb_data, so its permissions must be the ones
// PocketBase itself writes. Staging at 0o600/0o700 would leave a restored
// deployment with a storage tree no other process or user could read.
func TestRestoreStagesWithPocketBasePermissions(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return true })

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	pending := pendingDir(app, jobID)

	fi, err := os.Stat(filepath.Join(pending, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Fatalf("data.db mode %o, want 644", got)
	}
	fi, err = os.Stat(filepath.Join(pending, "storage", "col1", "rec1", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Fatalf("stored file mode %o, want 644", got)
	}
	fi, err = os.Stat(filepath.Join(pending, "storage", "col1", "rec1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o755 {
		t.Fatalf("storage dir mode %o, want 755", got)
	}
}

// Phase 5 marks a staging directory complete only after the archive verified and
// the staged database passed its integrity check. The boot swap relies on that
// sentinel to tell an unfinished stage from a swap already under way.
func TestRestoreMarksAStagingDirectoryComplete(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return true })

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: id, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir(app, jobID), stagedSentinel)); err != nil {
		t.Fatalf("a fully staged directory carries no sentinel: %v", err)
	}
}

// A staging directory whose archive failed verification must NOT be marked
// complete, or the next boot would swap in data nothing checked.
func TestRestoreLeavesAFailedStageUnmarked(t *testing.T) {
	data, id := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return true })

	// Corrupt the tail so the archive's own checksums fail at Verify, after the
	// members are already on disk.
	tampered := make([]byte, len(data))
	copy(tampered, data)
	tampered[len(tampered)-1] ^= 0xff

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(tampered)}, Identity: id, Force: true})
	if err == nil {
		t.Fatal("a tampered archive must not stage successfully")
	}
	if _, serr := os.Stat(filepath.Join(pendingDir(app, jobID), stagedSentinel)); !os.IsNotExist(serr) {
		t.Fatal("a failed stage was marked complete")
	}
}

// The restore path reads through a RangeSource, whose dial failure carries the
// presigned source URL. The failure defer copies it into the row's error column.
func TestFailedRestoreRecordsNoSignedURLInTheLedger(t *testing.T) {
	app := newTestApp(t)
	resetRestoreState(t)
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	source := "https://127.0.0.1:1/bucket/backup.age?X-Amz-Signature=DEADBEEF"
	src := format.NewRangeSource(context.Background(), source)
	jobID, rerr := Restore(app, RestoreRequest{
		Source: src, Ranged: src, Identity: id, SourceHost: HostOnly(source),
	})
	if rerr == nil {
		t.Fatal("a restore from a closed port must fail")
	}
	row, ferr := app.FindRecordById("backups", jobID)
	if ferr != nil {
		t.Fatal(ferr)
	}
	assertNoSignature(t, row.GetString("error"))
}

// The restore side of the same hazard: a source that sends its headers and then
// stops sending parks the reader inside the transport, where the retry budget
// cannot reach it, holding the interlock with it.
func TestRestoreGivesUpOnAStalledSourceAndReleasesTheInterlock(t *testing.T) {
	prev := format.StallDeadline
	format.StallDeadline = 150 * time.Millisecond
	t.Cleanup(func() { format.StallDeadline = prev })

	data, identity := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		// Enough for age's header, then silence.
		_, _ = w.Write(data[:64])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	src := format.NewRangeSource(context.Background(), srv.URL+"/x")
	jobID, err := Restore(app, RestoreRequest{Source: src, Ranged: src, Identity: identity})
	if err == nil {
		t.Fatal("a stalled source must fail the restore")
	}
	if installjob.Running() {
		t.Fatal("a stalled restore must release the interlock")
	}
	row, rerr := app.FindRecordById("backups", jobID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := row.GetString("status"); got != "failed" {
		t.Fatalf("status %q", got)
	}
}

// A composition with no supervisor cannot end the process. A restore that
// believed otherwise left `restoring` set, so every request afterwards met the
// maintenance 503 with nothing coming to clear it — the whole deployment wedged
// behind a restore that could not complete.
func TestRestoreThatCannotRestartLeavesTheDeploymentServing(t *testing.T) {
	data, identity := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return false })

	jobID, err := Restore(app, RestoreRequest{
		Source: readCloser{bytes.NewReader(data)}, Identity: identity, Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if Restoring() {
		t.Fatal("a restore nothing will restart must not leave the deployment behind the 503")
	}
	row, rerr := app.FindRecordById("backups", jobID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	// The row stays running: the restore IS still owed, and only the process that
	// boots on the staged data can say it worked.
	if got := row.GetString("status"); got != "running" {
		t.Fatalf("status %q, want running", got)
	}
	var meta map[string]any
	if err := row.UnmarshalJSONField("metadata", &meta); err != nil {
		t.Fatal(err)
	}
	if meta["awaiting_restart"] != true {
		t.Fatalf("metadata %+v — the panel has no way to say a restart is owed", meta)
	}
	// The staged data is still there for the restart to pick up.
	if _, serr := os.Stat(armedPath(app)); serr != nil {
		t.Fatalf("the armed marker must survive: %v", serr)
	}
}

// The restart that WILL happen leaves the restore in maintenance mode: the
// staged copy is what boots next, so a write landing here would be discarded.
func TestRestoreThatWillRestartStaysInMaintenanceMode(t *testing.T) {
	data, identity := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	SetRestart(func() bool { return true })

	if _, err := Restore(app, RestoreRequest{
		Source: readCloser{bytes.NewReader(data)}, Identity: identity, Force: true,
	}); err != nil {
		t.Fatal(err)
	}
	if !Restoring() {
		t.Fatal("a restore on its way out must keep serving 503")
	}
}
