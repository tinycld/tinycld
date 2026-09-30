package pbs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gopbs "github.com/osshield/gopbs/pbs"
	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/repo/repotest"
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
