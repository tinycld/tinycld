package pbs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/repo/repotest"
)

// logCapture is the default logger's output while a test runs. The package
// logs through slog.Default (logging.ForPackage), so the capture sees it.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

// records returns each captured record whose msg is msg, decoded.
func (c *logCapture) records(t *testing.T, msg string) []map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(c.buf.Bytes()))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("log line is not JSON: %s", sc.Text())
		}
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(c, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return c
}

// fastProbe makes the probe after a failure run its attempts with no wait.
func fastProbe(t *testing.T) {
	t.Helper()
	prev := probeDelays
	probeDelays = []time.Duration{0, 0, 0}
	t.Cleanup(func() { probeDelays = prev })
}

// probeRecord waits for the background probes, then returns the one probe
// record of op.
func probeRecord(t *testing.T, logs *logCapture, op string) map[string]any {
	t.Helper()
	if !WaitForProbes(30 * time.Second) {
		t.Fatal("the probe did not finish")
	}
	return onlyRecord(t, logs.records(t, "pbs: probe after a failure"), op)
}

// onlyRecord returns the one record of op among recs.
func onlyRecord(t *testing.T, recs []map[string]any, op string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, r := range recs {
		if r["op"] == op {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d records for op %q, want 1: %v", len(found), op, recs)
	}
	return found[0]
}

func repoAt(t *testing.T, server string) *Repository {
	t.Helper()
	raw, _ := json.Marshal(Config{
		Server: server, Datastore: "store1",
		AuthID: "test@pbs!test", Secret: "secret", BackupID: "acme.example",
	})
	r, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r.(*Repository)
}

func listOf(t *testing.T, rec map[string]any, key string) []map[string]any {
	t.Helper()
	raw, _ := rec[key].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("%s holds %T, want objects: %v", key, v, rec[key])
		}
		out = append(out, m)
	}
	return out
}

func TestDiagnosticsLogARefusedConnect(t *testing.T) {
	logs := captureLogs(t)
	fastProbe(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens there now: every connect is refused

	if _, err := repoAt(t, "https://"+addr).List(context.Background()); err == nil {
		t.Fatal("List succeeded with no server")
	}

	rec := onlyRecord(t, logs.records(t, "pbs: operation failed"), "list")
	if rec["level"] != "WARN" || rec["server"] != addr || rec["error_class"] != "dial-refused" {
		t.Fatalf("failure record %v", rec)
	}
	dials := listOf(t, rec, "failed_dials")
	if len(dials) == 0 || dials[0]["error"] == "" || dials[0]["dial_ms"] == nil {
		t.Fatalf("failed_dials %v", rec["failed_dials"])
	}

	probe := probeRecord(t, logs, "list")
	attempts := listOf(t, probe, "attempts")
	if len(attempts) != 3 {
		t.Fatalf("probe attempts %v", probe["attempts"])
	}
	for _, a := range attempts {
		if a["error"] == nil || a["error"] == "" {
			t.Fatalf("a probe of a closed port succeeded: %v", a)
		}
	}
}

// resetListener accepts each connection and resets it at once, as a server
// (or a device on the path) that refuses the connection after it is set up.
func resetListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 512)
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = c.Read(buf) // the ClientHello, so the reset answers a write
			_ = c.(*net.TCPConn).SetLinger(0)
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}

func TestDiagnosticsLogAConnectionTheServerResets(t *testing.T) {
	logs := captureLogs(t)
	fastProbe(t)
	addr := resetListener(t)

	if _, err := repoAt(t, "https://"+addr).List(context.Background()); err == nil {
		t.Fatal("List succeeded through a reset")
	}

	rec := onlyRecord(t, logs.records(t, "pbs: operation failed"), "list")
	if rec["error_class"] != "reset" && rec["error_class"] != "eof" {
		t.Fatalf("error_class %v, want reset (or eof, when the FIN wins): %v", rec["error_class"], rec)
	}
	conns := listOf(t, rec, "conns")
	if len(conns) == 0 {
		t.Fatalf("no connection recorded: %v", rec)
	}
	c := conns[0]
	if c["remote"] != addr || c["local"] == "" || c["dial_ms"] == nil || c["io_error"] == nil {
		t.Fatalf("connection record %v", c)
	}
	if c["written_bytes"].(float64) <= 0 {
		t.Fatalf("the ClientHello was not counted: %v", c)
	}

	probe := probeRecord(t, logs, "list")
	for _, a := range listOf(t, probe, "attempts") {
		if a["connect_ms"] == nil || a["error"] == nil {
			t.Fatalf("a probe attempt did not record the connect and the TLS failure: %v", a)
		}
	}
}

func TestDiagnosticsLogAConnectionDroppedDuringAnUpload(t *testing.T) {
	logs := captureLogs(t)
	fastProbe(t)
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	srv.DropAfterBytes(1024)

	if _, err := r.Put(context.Background(), repotest.Fixture(t), nil); err == nil {
		t.Fatal("put succeeded through a dropped connection")
	}

	rec := onlyRecord(t, logs.records(t, "pbs: operation failed"), "backup")
	conns := listOf(t, rec, "conns")
	if len(conns) == 0 || conns[0]["first_byte_ms"] == nil || conns[0]["read_bytes"].(float64) <= 0 {
		t.Fatalf("the session's connection was not recorded: %v", rec)
	}

	// The server still answers, so every probe attempt completes TLS.
	probe := probeRecord(t, logs, "backup")
	for _, a := range listOf(t, probe, "attempts") {
		if a["error"] != nil || a["tls_ms"] == nil {
			t.Fatalf("probe of a live server: %v", a)
		}
	}
}

func TestDiagnosticsLogAFinishedOperation(t *testing.T) {
	logs := captureLogs(t)
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))

	if _, err := r.Put(context.Background(), repotest.Fixture(t), nil); err != nil {
		t.Fatal(err)
	}

	rec := onlyRecord(t, logs.records(t, "pbs: operation finished"), "backup")
	if rec["level"] != "INFO" || rec["elapsed_ms"] == nil {
		t.Fatalf("finish record %v", rec)
	}
	conns := listOf(t, rec, "conns")
	if len(conns) == 0 || conns[0]["dial_ms"] == nil || conns[0]["written_bytes"].(float64) <= 0 {
		t.Fatalf("finish record conns %v", rec["conns"])
	}
	WaitForProbes(30 * time.Second)
	if len(logs.records(t, "pbs: operation failed")) != 0 || len(logs.records(t, "pbs: probe after a failure")) != 0 {
		t.Fatal("a finished operation logged a failure or ran a probe")
	}
}

// The PBS datastore's disks spin down, and the first request after a quiet
// period waits about 50 s for them. The client must wait longer than that
// for every answer, or the first operation after idle always fails.
func TestClientWaitsForASleepingDatastore(t *testing.T) {
	cfg, err := ParseConfig(configFor(t, pbstest.NewServer(t), ""))
	if err != nil {
		t.Fatal(err)
	}
	c := clientConfig(cfg, nil, nil)
	if c.ResponseTimeout < 90*time.Second {
		t.Fatalf("ResponseTimeout %v: shorter than a disk spin-up with margin", c.ResponseTimeout)
	}
	if c.Dial == nil {
		t.Fatal("the client does not dial through the diagnostics")
	}
}
