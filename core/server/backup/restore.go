package backup

import (
	"archive/tar"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"filippo.io/age"
	"github.com/pocketbase/pocketbase/core"
	_ "modernc.org/sqlite" // the driver the staged-database integrity check opens with

	"tinycld.org/core/audit"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
	"tinycld.org/core/notify"
)

// Rebuilder turns an archive's lockfile into a binary that carries exactly that
// package set, and ends the process so the supervisor launches the new one. A
// deployment that cannot rebuild itself registers nothing, and a restore of a
// different package set is refused instead.
type Rebuilder func(ctx context.Context, lockfile format.Lockfile) error

var (
	rebuilderMu sync.RWMutex
	rebuilder   Rebuilder
)

func RegisterRebuilder(fn Rebuilder) {
	rebuilderMu.Lock()
	defer rebuilderMu.Unlock()
	rebuilder = fn
}

func HasRebuilder() bool {
	rebuilderMu.RLock()
	defer rebuilderMu.RUnlock()
	return rebuilder != nil
}

// restartFn ends the process so the supervisor relaunches it onto the staged
// data. It is a seam rather than an os.Exit so the package's own tests can
// observe the request instead of killing the test binary.
var (
	restartMu sync.Mutex
	restartFn = func() {}
	restarted bool
)

// SetRestart names the function that ends the process so the supervisor
// relaunches it.
func SetRestart(fn func()) {
	restartMu.Lock()
	defer restartMu.Unlock()
	if fn == nil {
		fn = func() {}
	}
	restartFn = fn
}

func requestRestart() {
	restartMu.Lock()
	fn := restartFn
	restarted = true
	restartMu.Unlock()
	fn()
}

func restartRequested() bool {
	restartMu.Lock()
	defer restartMu.Unlock()
	return restarted
}

// restoring is true from the moment a restore is armed until the process ends.
// Anything that writes to the live database has to stand down once it is set:
// the staged copy is what the next process boots on, so a write landing in
// pb_data after the archive was staged is silently discarded.
var restoring atomic.Bool

// Restoring reports whether this process has armed a restore and is on its way
// out.
func Restoring() bool { return restoring.Load() }

// Source is an archive being read. A local upload is a request body; a remote
// one is a *format.RangeSource, which can also be handed a fresh URL mid-read.
type Source interface {
	io.Reader
	Close() error
}

type RestoreRequest struct {
	Source Source
	// Ranged is the same object as Source when the archive comes from a URL. It
	// is what makes SwapSource possible, so it is separate from Source rather
	// than type-asserted: a caller that does not want a swap does not set it.
	Ranged     *format.RangeSource
	Identity   age.Identity
	Force      bool
	Initiator  string
	Request    *core.RequestEvent
	SourceHost string
}

var ErrNotWaiting = errors.New("backup: no restore is waiting for a source")

var (
	waitingMu sync.Mutex
	waiting   = map[string]*format.RangeSource{}
)

// SwapSource gives a waiting restore a fresh URL for the same object. A
// presigned URL can expire mid-transfer on a large archive, and re-downloading
// from zero can cost more than the whole restore.
func SwapSource(jobID, url string) error {
	waitingMu.Lock()
	src, ok := waiting[jobID]
	waitingMu.Unlock()
	if !ok {
		return ErrNotWaiting
	}
	src.SwapURL(url)
	return nil
}

// StartRestore inserts the ledger row and restores on a goroutine, so an HTTP
// caller gets an id rather than holding a connection open for the transfer.
func StartRestore(app core.App, req RestoreRequest) (string, error) {
	row, job, err := beginRestore(app, req)
	if err != nil {
		return "", err
	}
	go func() { _ = runRestore(app, req, row, job) }()
	return row.Id, nil
}

// Restore is StartRestore without the goroutine: it returns once the archive is
// staged and the rebuild or restart has been asked for.
func Restore(app core.App, req RestoreRequest) (string, error) {
	row, job, err := beginRestore(app, req)
	if err != nil {
		return "", err
	}
	return row.Id, runRestore(app, req, row, job)
}

// beginRestore refuses before it writes: a restore turned away by the interlock
// leaves no ledger row, because nothing was attempted.
func beginRestore(app core.App, req RestoreRequest) (*core.Record, *installjob.Job, error) {
	job := installjob.New("restore", "", "")
	if _, ok := installjob.Claim(job); !ok {
		return nil, nil, ErrBusy
	}
	row := newRow(app, KindRestore, req.Initiator, req.SourceHost)
	if err := app.Save(row); err != nil {
		installjob.Release(job)
		return nil, nil, err
	}
	job.ID = row.Id
	return row, job, nil
}

func runRestore(app core.App, req RestoreRequest, row *core.Record, job *installjob.Job) (err error) {
	id := row.Id
	// The pre-restore backup claims the interlock itself, so ownership passes
	// back and forth. released tracks who holds it, and release is idempotent so
	// the unwind cannot hand it back twice.
	released := false
	release := func() {
		if !released {
			installjob.Release(job)
			released = true
		}
	}
	defer release()
	defer func() {
		if cerr := req.Source.Close(); cerr != nil {
			log.Warn("could not close a restore source", "id", id, "err", cerr)
		}
	}()

	if aerr := audit.Log(app, "restore.started", "backup", id, "restore", req.Request, nil); aerr != nil {
		log.Warn("could not audit the start of a restore", "id", id, "err", aerr)
	}

	// manifest is a pointer so "never read" stays distinguishable from "read and
	// empty": a zero-valued manifest in the ledger reads as a real archive of
	// nothing.
	var manifest *format.Manifest
	defer func() {
		if err == nil {
			// The row stays "running" on purpose. Only the restored process can
			// say the restore worked, because only it boots on the staged data.
			return
		}
		log.Error("restore failed", "id", id, "err", err)
		restoring.Store(false)
		if rerr := os.RemoveAll(pendingDir(app, id)); rerr != nil {
			log.Warn("could not remove a staged restore", "id", id, "err", rerr)
		}
		// The armed marker goes, the pre-restore backup stays: the archive that
		// failed is worthless, but the copy of what this organization had a
		// moment ago is the only way back if anything did get disturbed.
		if rerr := os.Remove(armedPath(app)); rerr != nil && !os.IsNotExist(rerr) {
			log.Warn("could not disarm a failed restore", "id", id, "err", rerr)
		}
		if ferr := finishRow(app, row, "failed", 0, "", err.Error(), manifest); ferr != nil {
			log.Error("could not finalize restore row", "id", id, "err", ferr)
		}
		// Failure is the only outcome this process can announce. Success is
		// announced by the post-boot finalizer, because a restore that worked
		// ends by replacing the process that ran it.
		announceRestore(app, req, row, false, err.Error())
	}()

	// A remote source can expire at any point, including while phase 1 is still
	// reading the manifest, so it is registered for a swap before the first byte
	// is read rather than at phase 5. Registering it later would mean an
	// operator's fresh URL is refused as ErrNotWaiting for exactly the part of
	// the transfer that has not stalled yet.
	if req.Ranged != nil {
		waitingMu.Lock()
		waiting[id] = req.Ranged
		waitingMu.Unlock()
		defer func() {
			waitingMu.Lock()
			delete(waiting, id)
			waitingMu.Unlock()
		}()
		// Deferred after the terminal defer above, so it runs BEFORE it: a
		// watcher still running could otherwise save "waiting_for_source" over
		// the row the restore just finalized.
		watcher := newExpiryWatcher(app, row, req.Ranged)
		defer watcher.stop()
	}

	// Phase 1: the manifest only. Nothing is decided, and nothing is written,
	// until this deployment knows what it is being asked to become.
	reader, err := format.NewReader(req.Source, req.Identity)
	if err != nil {
		return err
	}
	defer reader.Close()
	read, err := reader.ReadManifest()
	if err != nil {
		return err
	}
	manifest = &read
	row.Set("manifest", read)
	if err = app.Save(row); err != nil {
		return err
	}

	// Phase 2: can this deployment run that package set? A refusal here has
	// written nothing to disk, which is what makes it safe to refuse late.
	if !HasRebuilder() && !req.Force {
		installed, ierr := installedPackages(app)
		if ierr != nil {
			return ierr
		}
		if d := compareEmbedded(read, installed); !d.Empty() {
			return &MismatchError{Diff: d}
		}
	}

	// Phase 3: the pre-restore safety copy, keyed to a throwaway identity the
	// owner reads off the row. The identity lives only in the row's metadata, so
	// it is never logged and never leaves the database.
	preID, err := age.GenerateX25519Identity()
	if err != nil {
		return err
	}
	prePath := preBackupPath(app, id)
	if err = os.MkdirAll(filepath.Dir(prePath), 0o700); err != nil {
		return err
	}
	preFile, err := os.Create(prePath)
	if err != nil {
		return err
	}
	release() // the pre-restore backup claims the interlock itself
	if _, err = Run(app, Request{Kind: KindPreRestore, Recipient: preID.Recipient(), Sink: preFile}); err != nil {
		// A pre-restore backup that failed is not a safety copy, and the failure
		// path below keeps whatever is at this path. Leaving the partial file
		// there would offer an operator a way back that cannot be read.
		if rerr := os.Remove(prePath); rerr != nil && !os.IsNotExist(rerr) {
			log.Warn("could not remove a failed pre-restore backup", "id", id, "err", rerr)
		}
		return fmt.Errorf("pre-restore backup: %w", err)
	}
	if _, ok := installjob.Claim(job); !ok {
		return ErrBusy
	}
	released = false
	row.Set("metadata", mergeMeta(row, map[string]any{
		"pre_restore_identity": preID.String(),
		"pre_restore_path":     prePath,
	}))
	if err = app.Save(row); err != nil {
		return err
	}

	// Phase 4: arm. From here the process is on its way out, and the marker is
	// what tells whoever boots next that there is staged data to swap in.
	//
	// The marker is written BEFORE the archive is staged, so it can name a
	// pending directory that is empty or half-written: a process killed during
	// phase 5 leaves exactly that. The boot swap must therefore never trust the
	// marker alone — it has to confirm pending/<id>/data.db is there before it
	// moves anything aside.
	pending := pendingDir(app, id)
	if err = os.MkdirAll(pending, 0o700); err != nil {
		return err
	}
	marker, err := json.Marshal(armed{ID: id, Pending: pending, Pre: prePath, Manifest: read})
	if err != nil {
		return err
	}
	if err = os.WriteFile(armedPath(app), marker, 0o600); err != nil {
		return err
	}
	restoring.Store(true)

	// Phase 5: the rest of the stream, staged and verified against the archive's
	// own checksums before anything is asked to boot on it.
	if err = stage(reader, pending); err != nil {
		return err
	}
	if err = integrityCheck(filepath.Join(pending, format.MemberDB)); err != nil {
		return err
	}
	// The sentinel is the boot swap's only sound evidence that staging finished.
	// It cannot infer that from the members themselves: once the swap starts
	// moving them into pb_data, a staged member's absence from pending means the
	// opposite of what it means before the swap starts.
	if err = os.WriteFile(filepath.Join(pending, stagedSentinel), nil, 0o644); err != nil {
		return err
	}
	row.Set("status", "running")
	if err = app.Save(row); err != nil {
		return err
	}

	// Phase 6: rebuild, or restart onto the package set this binary already has.
	// Force skips the rebuild too: it is the operator saying "this binary, that
	// data", and a rebuild would silently overrule them.
	rebuilderMu.RLock()
	fn := rebuilder
	rebuilderMu.RUnlock()
	if fn != nil && !req.Force {
		release()
		return fn(context.Background(), read.Lockfile)
	}
	requestRestart()
	return nil
}

// expiryWatcher reports a stalled remote source on the ledger row, so an
// operator watching the panel can tell "the URL expired, paste a fresh one"
// apart from "this restore is stuck". It owns its goroutine's lifetime rather
// than reading the row's status to decide when to stop: the row is saved from
// the restore's own goroutine, and a watcher reading it would race.
type expiryWatcher struct {
	stopCh chan struct{}
	done   chan struct{}
}

const (
	expiryPollInterval = 500 * time.Millisecond
	// Four quiet polls before the row is flipped: a transfer can pause for a
	// moment without the URL having expired, and a panel that flickers
	// "waiting for source" teaches operators to ignore it.
	expiryStallPolls = 4
)

func newExpiryWatcher(app core.App, row *core.Record, src *format.RangeSource) *expiryWatcher {
	w := &expiryWatcher{stopCh: make(chan struct{}), done: make(chan struct{})}
	// The watcher never touches row: saving it from here would race the restore
	// goroutine's own saves. It writes through a fresh read of the same id.
	id := row.Id
	go func() {
		defer close(w.done)
		t := time.NewTicker(expiryPollInterval)
		defer t.Stop()
		last := src.Offset()
		stalled := 0
		blocked := false
		for {
			select {
			case <-w.stopCh:
				return
			case <-t.C:
			}
			cur := src.Offset()
			if cur != last {
				stalled = 0
				if blocked {
					blocked = false
					setRestoreStatus(app, id, "running", nil)
				}
			} else {
				stalled++
			}
			last = cur
			if !blocked && stalled >= expiryStallPolls && src.Blocked() {
				blocked = true
				setRestoreStatus(app, id, "waiting_for_source", map[string]any{"resume_offset": cur})
			}
		}
	}()
	return w
}

func (w *expiryWatcher) stop() {
	close(w.stopCh)
	<-w.done
}

// setRestoreStatus moves a restore's row between running and waiting_for_source
// without holding the record the restore goroutine is saving.
func setRestoreStatus(app core.App, id, status string, extra map[string]any) {
	row, err := app.FindRecordById(collection, id)
	if err != nil {
		log.Warn("could not read a restore row to report its source state", "id", id, "err", err)
		return
	}
	// A row that already reached a terminal status is not moved back: the
	// restore has ended and the watcher is only a moment behind it.
	if s := row.GetString("status"); s != "running" && s != "waiting_for_source" {
		return
	}
	row.Set("status", status)
	if len(extra) > 0 {
		row.Set("metadata", mergeMeta(row, extra))
	}
	if err := app.Save(row); err != nil {
		log.Warn("could not record a restore's source state", "id", id, "err", err)
	}
}

// mergeMeta keeps whatever is already in the metadata column. The pre-restore
// identity lives there, and overwriting the map would lose the only copy.
func mergeMeta(row *core.Record, extra map[string]any) map[string]any {
	out := map[string]any{}
	existing := map[string]any{}
	if err := row.UnmarshalJSONField("metadata", &existing); err != nil {
		log.Warn("could not read a restore row's metadata", "id", row.Id, "err", err)
	}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// stage writes every member into dir. It accepts only the members a backup
// contains: an archive naming anything else is either a different format or an
// attempt to write outside the staging directory.
func stage(r *format.Reader, dir string) error {
	for {
		hdr, body, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// A backup writes regular files and nothing else. Refusing every other
		// type here is the check that matters: a symlink member named
		// storage/x pointing at /etc or at the live pb_data would otherwise be
		// judged solely on where its own name lands, and a later write through
		// that name would leave the staging directory entirely.
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("%w: member %q is not a regular file", format.ErrFormat, hdr.Name)
		}
		if hdr.Name != format.MemberDB && !strings.HasPrefix(hdr.Name, format.StoragePrefix) {
			return fmt.Errorf("%w: unexpected member %q", format.ErrFormat, hdr.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
			return fmt.Errorf("%w: member %q escapes the staging directory", format.ErrFormat, hdr.Name)
		}
		if err := writeMember(target, body); err != nil {
			return err
		}
	}
	return r.Verify()
}

// writeMember stages one member with the permissions PocketBase itself writes
// under pb_data. The staged tree BECOMES pb_data, so staging it tighter would
// leave a restored deployment with a database and a storage tree no other
// process or user on the host could read.
func writeMember(target string, body io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// integrityCheck refuses a staged database SQLite cannot read. Without it a
// truncated or corrupt archive gets swapped in and the process restart-loops on
// a database that will never open, with the live copy already moved aside.
func integrityCheck(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("backup: open the staged database: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Warn("could not close the staged database", "path", path, "err", cerr)
		}
	}()
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("backup: check the staged database: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Warn("could not close an integrity-check result", "path", path, "err", cerr)
		}
	}()
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("backup: check the staged database: %w", err)
		}
		problems = append(problems, line)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backup: check the staged database: %w", err)
	}
	if len(problems) == 1 && problems[0] == "ok" {
		return nil
	}
	return fmt.Errorf("backup: the staged database failed its integrity check: %s", strings.Join(problems, "; "))
}

// announceRestore tells the people who can act, and records the outcome. Neither
// failure fails the restore: by the time this runs the decision is already made.
//
// Only the failure branch is reachable from this file. A restore that got as far
// as arming ends by replacing the process, so this process is never the one that
// can say it worked — the post-boot finalizer calls this with ok=true once it has
// booted on the staged data.
func announceRestore(app core.App, req RestoreRequest, row *core.Record, ok bool, errMsg string) {
	typ, title, body, action := "core.restore.succeeded", "Restore completed",
		"This organization was restored from a backup.", "restore.succeeded"
	if !ok {
		typ, title, body, action = "core.restore.failed", "Restore failed",
			"Restoring from a backup failed: "+errMsg, "restore.failed"
	}
	if _, err := notify.Administrators(app, notify.NotifyParams{
		Type: typ, Package: "core", Title: title, Body: body, URL: "/settings/backups",
	}); err != nil {
		log.Warn("could not notify administrators about a restore", "err", err)
	}
	meta := map[string]any{}
	if host := row.GetString("target_host"); host != "" {
		meta["source_host"] = host
	}
	if err := audit.Log(app, action, "backup", row.Id, "restore", req.Request, meta); err != nil {
		log.Warn("could not audit a restore", "err", err)
	}
}
