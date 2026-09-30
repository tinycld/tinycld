package pbs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo/repotest"
)

// stallProxy sits between the client and a pbstest server and, once a
// connection has sent the client stallAfter bytes, stops forwarding without
// closing anything. That is a PBS that goes silent: the socket stays open and
// the client, which has no read-idle timeout, waits on it forever.
type stallProxy struct {
	ln         net.Listener
	backend    string
	stallAfter atomic.Int64 // < 0: never stall
	release    chan struct{}

	mu    sync.Mutex
	conns []net.Conn
}

func newStallProxy(t *testing.T, backend string) *stallProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &stallProxy{ln: ln, backend: backend, release: make(chan struct{})}
	p.stallAfter.Store(-1)
	go p.serve()
	t.Cleanup(func() {
		close(p.release)
		_ = ln.Close()
		p.mu.Lock()
		for _, c := range p.conns {
			_ = c.Close()
		}
		p.mu.Unlock()
	})
	return p
}

func (p *stallProxy) addr() string { return "https://" + p.ln.Addr().String() }

func (p *stallProxy) serve() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", p.backend)
		if err != nil {
			_ = client.Close()
			continue
		}
		p.mu.Lock()
		p.conns = append(p.conns, client, server)
		p.mu.Unlock()
		go func() { _, _ = io.Copy(server, client) }()
		go p.toClient(client, server)
	}
}

func (p *stallProxy) toClient(client, server net.Conn) {
	var sent int64
	buf := make([]byte, 32<<10)
	for {
		n, err := server.Read(buf)
		chunk := buf[:n]
		if limit := p.stallAfter.Load(); limit >= 0 && sent+int64(n) > limit {
			if keep := limit - sent; keep > 0 {
				_, _ = client.Write(chunk[:keep])
			}
			<-p.release
			return
		}
		if _, werr := client.Write(chunk); werr != nil {
			return
		}
		sent += int64(n)
		if err != nil {
			return
		}
	}
}

// stalledRepo returns a repository that backs up straight to srv and one
// that reads through a proxy that goes silent after stallAfter bytes.
func stalledRepo(t *testing.T, stallAfter int64) (direct, stalled *Repository) {
	t.Helper()
	srv := pbstest.NewServer(t)
	d, err := Open(configFor(t, srv, ""))
	if err != nil {
		t.Fatal(err)
	}
	c := srv.Config()
	proxy := newStallProxy(t, c.BaseURL[len("https://"):])
	proxy.stallAfter.Store(stallAfter)
	raw, _ := json.Marshal(Config{
		Server: proxy.addr(), Fingerprint: c.Fingerprint, Datastore: c.Datastore,
		AuthID: "test@pbs!test", Secret: "secret", BackupID: "acme.example",
	})
	s, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d.(*Repository), s.(*Repository)
}

func shortStallDeadline(t *testing.T) {
	prev := format.StallDeadline
	format.StallDeadline = 300 * time.Millisecond
	t.Cleanup(func() { format.StallDeadline = prev })
}

// A server that goes silent in the middle of the chunk stream: the reader has
// no read-idle timeout, so only the watchdog on the stream's progress ends it.
func TestFetchGivesUpOnAStalledServer(t *testing.T) {
	shortStallDeadline(t)
	direct, stalled := stalledRepo(t, 256<<10)
	res, err := direct.Put(context.Background(), repotest.Fixture(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- stalled.Fetch(context.Background(), res.Ref, t.TempDir()) }()
	select {
	case err := <-done:
		if !errors.Is(err, format.ErrStalled) {
			t.Fatalf("want ErrStalled, got %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a stalled fetch was never given up on")
	}
}

// A server that never answers at all, so the manifest read stalls before its
// first byte.
func TestManifestGivesUpOnAStalledServer(t *testing.T) {
	shortStallDeadline(t)
	direct, stalled := stalledRepo(t, 0)
	res, err := direct.Put(context.Background(), repotest.Fixture(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := stalled.Manifest(context.Background(), res.Ref)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, format.ErrStalled) {
			t.Fatalf("want ErrStalled, got %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a stalled manifest read was never given up on")
	}
}

// The engine's watchdog ends a Put by cancelling its context; against a server
// that has gone silent during the session, that must be enough for Put to return.
func TestPutReturnsWhenItsContextEndsOnAStalledServer(t *testing.T) {
	_, stalled := stalledRepo(t, 4<<10)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := stalled.Put(ctx, repotest.Fixture(t), nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a Put to a silent server succeeded")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Put never returned after its context ended")
	}
}
