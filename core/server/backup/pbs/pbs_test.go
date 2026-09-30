package pbs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gopbs "github.com/osshield/gopbs/pbs"
	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/repo/repotest"
	"tinycld.org/core/backup/snapshot"
)

func configFor(t *testing.T, srv *pbstest.Server, key string) json.RawMessage {
	t.Helper()
	c := srv.Config()
	raw, _ := json.Marshal(Config{
		Server: c.BaseURL, Fingerprint: c.Fingerprint, Datastore: c.Datastore,
		AuthID: "test@pbs!test", Secret: "secret", Key: key, BackupID: "acme.example",
	})
	return raw
}

func TestPBSMeetsTheContract(t *testing.T) {
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(configFor(t, pbstest.NewServer(t), ""))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}

func TestPBSMeetsTheContractEncrypted(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(configFor(t, pbstest.NewServer(t), key))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}

func TestInterruptedPutLeavesNoSnapshot(t *testing.T) {
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	srv.DropAfterBytes(1024)
	if _, err := r.Put(context.Background(), repotest.Fixture(t), nil); err == nil {
		t.Fatal("put succeeded through a dropped connection")
	}
	if n := len(srv.Snapshots()); n != 0 {
		t.Fatalf("%d snapshots kept", n)
	}
}

func TestBadTokenErrorHidesTheSecret(t *testing.T) {
	srv := pbstest.NewServer(t)
	srv.RejectAuth(true)
	r, _ := Open(configFor(t, srv, ""))
	_, err := r.List(context.Background())
	if !errors.Is(err, gopbs.ErrAuth) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRequiresFields(t *testing.T) {
	for _, raw := range []string{`{}`, `{"server":"pbs.example"}`, `{"server":"pbs.example","datastore":"s","auth_id":"a@pbs!t"}`} {
		if _, err := ParseConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	c, err := ParseConfig(json.RawMessage(`{"server":"pbs.example","datastore":"s","auth_id":"a@pbs!t","secret":"x","backup_id":"acme"}`))
	if err != nil || c.baseURL() != "https://pbs.example:8007" || c.Host() != "pbs.example" {
		t.Fatalf("%+v %v", c, err)
	}
}

// TestAgainstARealServer runs the contract against a real PBS when
// TINYCLD_TEST_PBS holds a Config JSON (without backup_id). It is for a
// manual check before a release; CI runs the pbstest suite above.
func TestAgainstARealServer(t *testing.T) {
	raw := os.Getenv("TINYCLD_TEST_PBS")
	if raw == "" {
		t.Skip("TINYCLD_TEST_PBS is not set")
	}
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	c.BackupID = "tinycld-test-" + strconv.FormatInt(time.Now().Unix(), 10)
	cfg, _ := json.Marshal(c)
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}

// Deployments may share a datastore and a token, so a ref naming another
// backup ID (or type) must be refused before anything is read: restoring it
// would hand one organization another's data.
func TestReadRefusesAnotherBackupsSnapshot(t *testing.T) {
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	// Anything that reaches the server now fails with ErrAuth, so a refusal
	// that is not ErrAuth happened before the first request.
	srv.RejectAuth(true)
	for _, ref := range []repo.Ref{
		"host/other.example/2026-09-29T03:00:00Z",
		"vm/acme.example/2026-09-29T03:00:00Z",
	} {
		if _, err := r.Manifest(context.Background(), ref); !errors.Is(err, errNotThisBackup) {
			t.Errorf("Manifest(%s) = %v", ref, err)
		}
		if err := r.Fetch(context.Background(), ref, t.TempDir()); !errors.Is(err, errNotThisBackup) {
			t.Errorf("Fetch(%s) = %v", ref, err)
		}
	}
}

// stuckFile is a stored file whose read never returns until it is closed, as a
// read from object storage that has gone silent.
type stuckFile struct {
	reading chan struct{}
	closed  chan struct{}
	once    sync.Once
	open    *atomic.Int32
}

func (f *stuckFile) Read([]byte) (int, error) {
	select {
	case f.reading <- struct{}{}:
	default:
	}
	<-f.closed
	return 0, io.ErrClosedPipe
}

func (f *stuckFile) Close() error {
	f.once.Do(func() {
		close(f.closed)
		f.open.Add(-1)
	})
	return nil
}

// A stored file whose read never returns used to wedge Put past its own
// context: the upload waits for its chunker, the chunker for the tar pipe, the
// tar goroutine for the file. Put must return once its context ends, and only
// after the tar goroutine is done with the file, because the caller releases
// the snapshot — and the file lister under it — next.
func TestPutEndsWhileAStoredFileIsStuckAndJoinsItsTarWriter(t *testing.T) {
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	snap := repotest.Fixture(t)
	var open atomic.Int32
	stuck := &stuckFile{reading: make(chan struct{}, 1), closed: make(chan struct{}), open: &open}
	snap.Files[0].Open = func() (io.ReadCloser, error) {
		open.Add(1)
		return stuck, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-stuck.reading
		cancel()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := r.Put(ctx, snap, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled Put succeeded")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Put never returned after its context ended")
	}
	if n := open.Load(); n != 0 {
		t.Fatalf("Put returned with %d stored files still open", n)
	}
	if n := len(srv.Snapshots()); n != 0 {
		t.Fatalf("%d snapshots kept", n)
	}
}

// A Put whose upload fails for a reason that never touches ctx — a dropped
// connection, not a cancel — used to still join <-tarDone against the outer
// ctx: with a stored file stuck mid-read, that join would wait on the
// caller's ctx, which in production has no deadline of its own. Put must
// return promptly on this path too.
//
// Files[0] is large enough (40 MiB, over the chunker's 16 MiB max chunk
// size) that several chunks queue up for upload before the tar writer could
// reach Files[1]; with every upload stalled from the start, the chunker's
// own backpressure blocks the tar writer behind Files[0] alone, so Files[1]
// — the one that would otherwise block forever on Read — is never opened.
// Severing the connection here (a network failure, not a cancel) must still
// let Put return promptly, and must never open the still-queued file.
func TestPutEndsWhenUploadFailsWithoutCtxWhileAFileIsStillQueued(t *testing.T) {
	srv := pbstest.NewServer(t)
	c := srv.Config()
	proxy := newStallProxy(t, c.BaseURL[len("https://"):])
	// Past several chunks (the chunker's minimum is 1 MiB), so the jobs
	// buffer backs up before the connection stalls — but stallUpload cuts
	// the proxy's own read of the connection off at exactly this point too,
	// so waitForUploadAtLeast below must ask for less than this.
	proxy.stallUpload.Store(8 << 20)
	raw, _ := json.Marshal(Config{
		Server: proxy.addr(), Fingerprint: c.Fingerprint, Datastore: c.Datastore,
		AuthID: "test@pbs!test", Secret: "secret", BackupID: "acme.example",
	})
	repository, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 40<<20)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	var open atomic.Int32
	stuck := &stuckFile{reading: make(chan struct{}, 1), closed: make(chan struct{}), open: &open}
	snap := &snapshot.Snapshot{
		Manifest: format.Manifest{Format: format.FormatV1, Created: time.Now().UTC().Truncate(time.Second)},
		DBPath:   dbPath,
		Files: []snapshot.StoredFile{
			{Key: "c1/r1/big.bin", Size: int64(len(big)), Open: func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(big)), nil
			}},
			{Key: "c1/r2/queued.bin", Size: 1, Open: func() (io.ReadCloser, error) {
				open.Add(1)
				return stuck, nil
			}},
		},
		Release: func() error { return nil },
	}

	done := make(chan error, 1)
	go func() {
		_, err := repository.Put(context.Background(), snap, nil)
		done <- err
	}()
	// Wait for real bytes to reach the proxy — proof the chunker has started
	// producing and dispatching chunks against the stalled connection —
	// before severing it. A fixed sleep here raced the chunker's own speed
	// and could fire before it started or after it reached Files[1].
	proxy.waitForUploadAtLeast(t, 4<<20)
	proxy.severNow()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a put through a dropped connection succeeded")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want a non-ctx failure, got %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Put never returned after its upload failed")
	}
	if n := open.Load(); n != 0 {
		t.Fatalf("Put returned with %d stored files still open (the queued file must never be opened)", n)
	}
	if n := len(srv.Snapshots()); n != 0 {
		t.Fatalf("%d snapshots kept", n)
	}
}

// PBS speaks only https. A pasted http:// URL used to become
// "https://http://host", which fails later with an error naming neither cause.
func TestParseConfigRefusesASchemeOtherThanHTTPS(t *testing.T) {
	base := `{"datastore":"s","auth_id":"a@pbs!t","secret":"x","backup_id":"acme","server":%q}`
	for _, server := range []string{"http://pbs.example", "ftp://pbs.example:8007", "HTTP://pbs.example"} {
		_, err := ParseConfig(json.RawMessage(fmt.Sprintf(base, server)))
		if err == nil {
			t.Errorf("accepted %s", server)
			continue
		}
		if strings.Contains(err.Error(), "pbs.example") {
			t.Errorf("the error quotes the server: %v", err)
		}
	}
	for server, want := range map[string]string{
		"pbs.example":             "https://pbs.example:8007",
		"pbs.example:9000":        "https://pbs.example:9000",
		"https://pbs.example":     "https://pbs.example:8007",
		"HTTPS://pbs.example:443": "https://pbs.example:443",
	} {
		c, err := ParseConfig(json.RawMessage(fmt.Sprintf(base, server)))
		if err != nil || c.baseURL() != want {
			t.Errorf("%s: baseURL %q, err %v", server, c.baseURL(), err)
		}
	}
}
