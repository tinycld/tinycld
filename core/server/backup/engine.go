// Package backup runs one organization's backup: a consistent snapshot of the
// database plus every stored file, streamed into an encrypted archive.
//
// Every run inserts its ledger row BEFORE doing any work and always ends in a
// terminal status. "When was this last backed up" is then a read of the ledger
// rather than a guess, and a run killed by a restart shows as interrupted
// instead of running forever.
package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/audit"
	"tinycld.org/core/backup/format"
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
type Request struct {
	Kind       Kind
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
func Start(app core.App, req Request) (string, error) {
	row, job, err := begin(app, req)
	if err != nil {
		return "", err
	}
	go func() { _ = run(app, req, row, job) }()
	return row.Id, nil
}

// Run is Start without the goroutine: it returns when the sink is closed.
func Run(app core.App, req Request) (string, error) {
	row, job, err := begin(app, req)
	if err != nil {
		return "", err
	}
	return row.Id, run(app, req, row, job)
}

// begin refuses before it writes: a run turned away by the ceiling or the
// interlock leaves no ledger row, because nothing was attempted.
func begin(app core.App, req Request) (*core.Record, *installjob.Job, error) {
	if req.Kind == KindManual && !allowManual(app) {
		return nil, nil, ErrRateLimit
	}
	job := installjob.New("backup", "", "")
	if _, ok := installjob.Claim(job); !ok {
		return nil, nil, ErrBusy
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
	var written int64
	var sha string
	var sinkClosed bool
	var manifest format.Manifest
	// Named-return err: this closure is the single place a run becomes
	// terminal, whichever return below got here.
	defer func() {
		// A failed run still owns the sink. An HTTP PUT sink holds a pipe and
		// the goroutine reading it, so leaving it open leaks both for the life
		// of the process.
		if !sinkClosed {
			if cerr := req.Sink.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
		status, errMsg := "succeeded", ""
		if err != nil {
			status, errMsg = "failed", err.Error()
			log.Error("backup failed", "id", row.Id, "kind", req.Kind, "err", err)
		}
		if ferr := finishRow(app, row, status, written, sha, errMsg, manifest); ferr != nil {
			log.Error("could not finalize backup row", "id", row.Id, "err", ferr)
		}
		announce(app, req, row, status, errMsg)
		postCallback(req.Callback, row)
	}()

	manifest, err = buildManifest(app, req.Kind)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(tmpDir(app), 0o700); err != nil {
		return err
	}
	snap := filepath.Join(tmpDir(app), row.Id+".db")
	if err = vacuumInto(app, snap); err != nil {
		return err
	}
	defer func() {
		if rerr := os.Remove(snap); rerr != nil && !os.IsNotExist(rerr) {
			log.Warn("could not remove a backup snapshot", "path", snap, "err", rerr)
		}
	}()

	fs, err := app.NewFilesystem()
	if err != nil {
		return err
	}
	defer fs.Close()
	files, err := fs.List("")
	if err != nil {
		return err
	}
	// Counts go in the manifest, so they must be known before it is written.
	manifest.Counts.Files = len(files)
	for _, f := range files {
		manifest.Counts.Bytes += f.Size
	}

	counter := &countingWriter{w: req.Sink}
	w, err := format.NewWriter(counter, req.Recipient, zstd.SpeedDefault)
	if err != nil {
		return err
	}
	progress := newProgress(app, row, counter)
	// stop() runs before the outer defer finalizes the row, so the ticker
	// cannot save a stale byte count over the terminal one.
	defer progress.stop()

	if err = w.WriteManifest(manifest); err != nil {
		return err
	}
	if err = writeSnapshot(w, snap); err != nil {
		return err
	}
	for _, f := range files {
		if err = writeStored(w, fs, f.Key, f.Size); err != nil {
			return err
		}
	}
	if err = w.Close(); err != nil {
		return err
	}
	err = req.Sink.Close()
	sinkClosed = true
	if err != nil {
		return err
	}
	written, sha = counter.total(), w.Sha256()
	return nil
}

func writeSnapshot(w *format.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	return w.WriteFile(format.MemberDB, fi.Size(), f)
}

func writeStored(w *format.Writer, fs *filesystem.System, key string, size int64) error {
	r, err := fs.GetReader(key)
	if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	defer r.Close()
	return w.WriteFile(format.StoragePrefix+key, size, r)
}

func buildManifest(app core.App, kind Kind) (format.Manifest, error) {
	m := format.Manifest{
		Created:  time.Now().UTC(),
		Instance: app.Settings().Meta.AppURL,
		Source:   currentSource(),
		Kind:     string(kind),
		Lockfile: format.Lockfile{},
		Packages: map[string]string{},
		Counts:   format.Counts{Collections: map[string]int{}},
	}
	regs, err := app.FindRecordsByFilter("pkg_registry", "status = 'installed' || status = 'bundled'", "slug", 0, 0)
	if err != nil {
		return m, err
	}
	for _, r := range regs {
		slug := r.GetString("slug")
		if slug == "core" {
			m.Core = r.GetString("version")
			m.Lockfile["tinycld"] = r.GetString("npm_package")
			continue
		}
		m.Lockfile[slug] = r.GetString("npm_package")
		m.Packages[slug] = r.GetString("version")
	}
	cols, err := app.FindAllCollections()
	if err != nil {
		return m, err
	}
	for _, c := range cols {
		if c.System {
			continue
		}
		var n int
		// A collection name is an identifier, not a parameter, so it is quoted
		// rather than bound.
		if err := app.DB().Select("count(*)").From("`" + c.Name + "`").Row(&n); err != nil {
			log.Warn("could not count a collection for the backup manifest", "collection", c.Name, "err", err)
			continue
		}
		m.Counts.Collections[c.Name] = n
	}
	return m, nil
}

// announce tells the people who can act, and records the run in the audit log.
// Neither failure fails the backup: the archive is already written.
func announce(app core.App, req Request, row *core.Record, status, errMsg string) {
	if req.Kind == KindPreRestore {
		return // part of a restore; the restore announces itself
	}
	title, body, typ := "Backup completed", "A backup of this organization finished successfully.", "core.backup.succeeded"
	action := "backup.created"
	if status != "succeeded" {
		title, body, typ = "Backup failed", "A backup of this organization failed: "+errMsg, "core.backup.failed"
		action = "backup.failed"
	}
	if _, err := notify.Administrators(app, notify.NotifyParams{Type: typ, Package: "core", Title: title, Body: body, URL: "/settings/backups"}); err != nil {
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

// postCallback hands the finished row to whoever asked to be told. The row is
// exported publicly, so a callback never carries a passphrase or a recipient.
func postCallback(url string, row *core.Record) {
	if url == "" {
		return
	}
	body, err := json.Marshal(row.PublicExport())
	if err != nil {
		log.Warn("could not encode a backup row for its callback", "err", err)
		return
	}
	res, err := format.NoRedirectClient().Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Warn("backup callback failed", "host", HostOnly(url), "err", err)
		return
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode >= 300 {
		log.Warn("backup callback rejected", "host", HostOnly(url), "status", res.StatusCode)
	}
}

// countingWriter reports how many bytes reached the sink. The progress ticker
// reads it from another goroutine, so the count is behind a mutex.
type countingWriter struct {
	w  io.Writer
	mu sync.Mutex
	n  int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.mu.Lock()
	c.n += int64(n)
	c.mu.Unlock()
	return n, err
}

func (c *countingWriter) total() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
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

func newProgress(app core.App, row *core.Record, c *countingWriter) *progress {
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
				row.Set("bytes", c.total())
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
