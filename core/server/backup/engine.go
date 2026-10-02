// Package backup runs one organization's backup: a consistent snapshot of the
// database plus every stored file, handed to a repository — by default an
// encrypted archive streamed to a sink.
//
// Every run inserts its ledger row BEFORE doing any work and always ends in a
// terminal status. "When was this last backed up" is then a read of the ledger
// rather than a guess, and a run killed by a restart shows as interrupted
// instead of running forever.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"filippo.io/age"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/audit"
	"tinycld.org/core/backup/archive"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
	"tinycld.org/core/installjob"
	"tinycld.org/core/logging"
	"tinycld.org/core/notify"
)

var log = logging.ForPackage("backup")

type Kind string

const (
	KindManual     Kind = "manual"
	KindScheduled  Kind = "scheduled"
	KindPreRestore Kind = "pre_restore"
	KindRestore    Kind = "restore"
)

var (
	ErrBusy      = errors.New("backup: another job is running")
	ErrRateLimit = errors.New("backup: daily manual backup limit reached")
)

// Request is one run. Neither the recipient nor anything derived from a signed
// target URL is ever logged or stored: TargetHost is a hostname, which is why
// the caller passes it rather than the URL.
//
// Repo is where the backup goes. When it is nil the run writes an archive to
// Sink, encrypted to Recipient — the shape every caller had before
// repositories existed.
type Request struct {
	Kind       Kind
	Repo       repo.Repository
	Recipient  age.Recipient
	Sink       io.WriteCloser
	Initiator  string // users id, or "" for a run nobody asked for
	Callback   string // optional URL; the finished row is POSTed to it as JSON
	TargetHost string // hostname for the ledger; "" for a stream
	Request    *core.RequestEvent
}

var (
	sourceMu sync.RWMutex
	source   = "docker"
)

// SetSource names the deployment shape recorded in every manifest, so a
// restore can tell what it is reading before it starts.
func SetSource(s string) {
	sourceMu.Lock()
	defer sourceMu.Unlock()
	source = s
}

func currentSource() string {
	sourceMu.RLock()
	defer sourceMu.RUnlock()
	return source
}

// Start inserts the ledger row and runs the backup on a goroutine. It fails
// fast on the interlock and the ceiling so a caller gets 409/429 instead of a
// row that fails a moment later.
//
// The run is tracked against app from before it claims anything until after
// its last write, so StopAll can hold the app open until it is done.
func Start(app core.App, req Request) (string, error) {
	finished, err := trackRun(app)
	if err != nil {
		return "", err
	}
	row, job, err := begin(app, req)
	if err != nil {
		finished()
		return "", err
	}
	go func() {
		defer finished()
		_ = run(app, req, row, job)
	}()
	return row.Id, nil
}

// Run is Start without the goroutine: it returns when the sink is closed.
func Run(app core.App, req Request) (string, error) {
	finished, err := trackRun(app)
	if err != nil {
		return "", err
	}
	defer finished()
	row, job, err := begin(app, req)
	if err != nil {
		return "", err
	}
	return row.Id, run(app, req, row, job)
}

// begin refuses before it writes: a run turned away by the ceiling or the
// interlock leaves no ledger row, because nothing was attempted.
func begin(app core.App, req Request) (*core.Record, *installjob.Job, error) {
	job := installjob.New("backup", "", "")
	if _, ok := installjob.Claim(job); !ok {
		return nil, nil, ErrBusy
	}
	// The ceiling is checked AFTER the interlock, because allowManual records
	// the slot as it checks: a run turned away as busy did no work and must not
	// spend one.
	if req.Kind == KindManual && !allowManual(app) {
		installjob.Release(job)
		return nil, nil, ErrRateLimit
	}
	row := newRow(app, req.Kind, req.Initiator, req.TargetHost)
	if err := app.Save(row); err != nil {
		installjob.Release(job)
		return nil, nil, err
	}
	job.ID = row.Id
	return row, job, nil
}

func run(app core.App, req Request, row *core.Record, job *installjob.Job) (err error) {
	defer installjob.Release(job)

	r := req.Repo
	if r == nil {
		r = &archive.Repository{Recipient: req.Recipient, Target: archive.ToSink(req.Sink, "")}
	}
	var (
		result repo.PutResult
		// sent is what the repository reports it has sent so far; a failed run
		// records it, so an administrator is not told nothing was written when
		// something was.
		sent   atomic.Int64
		putRan bool
		// nil until built: a zero-valued manifest in the ledger reads as a real
		// backup of nothing, which is worse than no manifest at all.
		manifest *format.Manifest
	)
	// Named-return err: this closure is the single place a run becomes
	// terminal, whichever return below got here.
	defer func() {
		// A panic leaves err nil, so without this the row would finalize as
		// succeeded with zero bytes and administrators would be told a backup
		// they do not have is fine. The run reports the panic as its error
		// rather than re-panicking: the archive is lost either way, and a
		// crashed process cannot finish the ledger row.
		if p := recover(); p != nil {
			err = fmt.Errorf("backup: panic: %v", p)
			log.Error("backup panicked", "id", row.Id, "kind", req.Kind, "panic", p,
				"stack", string(debug.Stack()))
		}
		// Once Put runs, the repository owns the sink and closes it on every
		// path. Before that the run still owns a caller's sink: an HTTP PUT
		// sink holds a pipe and the goroutine reading it, so leaving it open
		// leaks both for the life of the process.
		if !putRan && req.Sink != nil {
			if cerr := req.Sink.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
		status, errMsg := "succeeded", ""
		if err != nil {
			status, errMsg = "failed", err.Error()
			result.Bytes = sent.Load()
			log.Error("backup failed", "id", row.Id, "kind", req.Kind, "err", err)
		}
		survive(row.Id, "finalize", func() {
			if ferr := finishRow(app, row, status, result, errMsg, manifest, r.Kind()); ferr != nil {
				log.Error("could not finalize backup row", "id", row.Id, "err", ferr)
			}
		})
		// The same lifetime as the transfer: a shutdown has to reach the
		// network calls here too, or StopAll waits on a peer that never answers.
		ctx, cancel := format.Lifetime(context.Background())
		defer cancel()
		survive(row.Id, "announce", func() { announce(ctx, app, req, row, status, errMsg) })
		survive(row.Id, "callback", func() { postCallback(ctx, req.Callback, row) })
	}()

	// Refused before the snapshot: a VACUUM INTO that runs out of space leaves
	// a truncated file and a SQLite error an operator cannot act on.
	if err = os.MkdirAll(tmpDir(app), 0o700); err != nil {
		return err
	}
	if err = requireFreeSpace(tmpDir(app), liveDatabaseBytes(app)); err != nil {
		return err
	}
	snap, err := snapshot.FromDataDir(snapshotOptions(app, req.Kind))
	if err != nil {
		return err
	}
	defer func() {
		if rerr := snap.Release(); rerr != nil {
			log.Warn("could not release a backup snapshot", "id", row.Id, "err", rerr)
		}
	}()
	manifest = &snap.Manifest

	progress := newProgress(app, row, sent.Load)
	// stop() runs before the terminal defer, so the ticker cannot save a stale
	// byte count over the final one.
	defer progress.stop()

	// A repository's client owns its sockets, and one whose server goes silent
	// blocks forever: the run would keep the interlock and keep renewing the
	// delete hold, so every storage delete is journaled for the life of the
	// process. The watchdog ends a Put whose count stops moving, and a shutdown
	// ends it too.
	ctx, watchdog := format.Watch(context.Background())
	defer watchdog.Stop()
	putRan = true
	result, err = r.Put(ctx, snap, func(n int64) {
		if sent.Swap(n) != n {
			watchdog.Progressed()
		}
	})
	return watchdog.Err(err)
}

// snapshotOptions takes the live app's snapshot through its own writer
// connection (see vacuumInto) and its own filesystem, so S3 storage works
// in-process.
func snapshotOptions(app core.App, kind Kind) snapshot.Options {
	return snapshot.Options{
		DataDir:  app.DataDir(),
		TmpDir:   tmpDir(app),
		Holder:   "app",
		Kind:     string(kind),
		Source:   currentSource(),
		Instance: app.Settings().Meta.AppURL,
		Vacuum:   func(dest string) error { return vacuumInto(app, dest) },
		Files:    func() ([]snapshot.StoredFile, func() error, error) { return appFiles(app) },
	}
}

func appFiles(app core.App) ([]snapshot.StoredFile, func() error, error) {
	fs, err := app.NewFilesystem()
	if err != nil {
		return nil, nil, err
	}
	objs, err := fs.List("")
	if err != nil {
		_ = fs.Close()
		return nil, nil, err
	}
	out := make([]snapshot.StoredFile, 0, len(objs))
	for _, o := range objs {
		key := o.Key
		out = append(out, snapshot.StoredFile{
			Key:  key,
			Size: o.Size,
			Open: func() (io.ReadCloser, error) { return fs.GetReader(key) },
		})
	}
	return out, fs.Close, nil
}

// announce tells the people who can act, and records the run in the audit log.
// Neither failure fails the backup: the archive is already written.
func announce(ctx context.Context, app core.App, req Request, row *core.Record, status, errMsg string) {
	if req.Kind == KindPreRestore {
		return // part of a restore; the restore announces itself
	}
	title, body, typ := "Backup completed", "A backup of this organization finished successfully.", "core.backup.succeeded"
	action := "backup.created"
	if status != "succeeded" {
		title, body, typ = "Backup failed", "A backup of this organization failed: "+errMsg, "core.backup.failed"
		action = "backup.failed"
	}
	if _, err := notify.AdministratorsContext(ctx, app, notify.NotifyParams{Type: typ, Package: "core", Title: title, Body: body, URL: "/settings/backups"}); err != nil {
		log.Warn("could not notify administrators about a backup", "err", err)
	}
	meta := map[string]any{"bytes": row.GetInt("bytes"), "kind": string(req.Kind)}
	if host := row.GetString("target_host"); host != "" {
		meta["target_host"] = host
	}
	if err := audit.Log(app, action, "backup", row.Id, string(req.Kind), req.Request, meta); err != nil {
		log.Warn("could not audit a backup", "err", err)
	}
}

// callbackBound caps one callback request, from dial to the last byte of the
// response body. The transport bounds only the wait for headers, and a run is
// still holding the job interlock while it posts, so a receiver that answers and
// then never finishes its body would block every backup, restore and package
// install. A receiver only has to acknowledge a small JSON row.
const callbackBound = 30 * time.Second

// callbackTimeout is callbackBound, held in a var only so a test can shorten
// it; nothing else writes it.
var callbackTimeout = callbackBound

// postCallback hands the finished row to whoever asked to be told. The row is
// exported publicly, so a callback never carries a passphrase or a recipient.
func postCallback(ctx context.Context, url string, row *core.Record) {
	if url == "" {
		return
	}
	body, err := json.Marshal(row.PublicExport())
	if err != nil {
		log.Warn("could not encode a backup row for its callback", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callbackTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Warn("backup callback failed", "host", HostOnly(url), "err", format.RedactURLError(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := format.NoRedirectClient().Do(req)
	if err != nil {
		// The host attribute alone is not enough: Go's *url.Error prints the
		// WHOLE callback URL, and a callback URL carries a token as often as a
		// target URL carries a signature. Every log call becomes a Sentry
		// breadcrumb, so the unredacted error was a credential in Sentry.
		log.Warn("backup callback failed", "host", HostOnly(url), "err", format.RedactURLError(err))
		return
	}
	_, cerr := io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if cerr != nil {
		log.Warn("backup callback did not finish its response", "host", HostOnly(url), "err", format.RedactURLError(cerr))
	}
	if res.StatusCode >= 300 {
		log.Warn("backup callback rejected", "host", HostOnly(url), "status", res.StatusCode)
	}
}

// progressTickForTesting reports that the ticker fired. Only the package's own
// tests set it: a test that means to exercise the progress writer has to know
// whether it actually ran, or it silently proves nothing.
var progressTickForTesting func()

// progress updates the row's bytes at most every 2 s so the panel can show
// movement without a write per chunk.
type progress struct {
	stopCh chan struct{}
	done   chan struct{}
}

func newProgress(app core.App, row *core.Record, total func() int64) *progress {
	p := &progress{stopCh: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(p.done)
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-t.C:
				if progressTickForTesting != nil {
					progressTickForTesting()
				}
				row.Set("bytes", total())
				if err := app.Save(row); err != nil {
					log.Warn("could not record backup progress", "id", row.Id, "err", err)
				}
			}
		}
	}()
	return p
}

// stop waits for the ticker goroutine to return, so no progress save can land
// after the run's terminal save.
func (p *progress) stop() {
	close(p.stopCh)
	<-p.done
}
