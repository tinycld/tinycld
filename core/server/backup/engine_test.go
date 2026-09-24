package backup

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/pocketbase/pocketbase/core"
	_ "modernc.org/sqlite"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

type closeBuffer struct {
	bytes.Buffer
	closed bool
}

func (c *closeBuffer) Close() error { c.closed = true; return nil }

func TestRunWritesRowFirstThenArchive(t *testing.T) {
	app := newTestApp(t)
	owner := makeUser(t, app, "owner@example.com", "owner")
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	sink := &closeBuffer{}

	rowID, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: sink, Initiator: owner.Id, TargetHost: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := app.FindRecordById("backups", rowID)
	if err != nil {
		t.Fatal(err)
	}
	if row.GetString("status") != "succeeded" || row.GetString("kind") != "manual" || row.GetString("initiated_by") != owner.Id {
		t.Fatalf("row %v", row.PublicExport())
	}
	if row.GetInt("bytes") != sink.Len() || row.GetString("sha256") == "" || !sink.closed {
		t.Fatalf("bytes %d vs %d, sha %q, closed %v", row.GetInt("bytes"), sink.Len(), row.GetString("sha256"), sink.closed)
	}
	if row.GetString("target_host") != "example.com" {
		t.Fatalf("target_host %q", row.GetString("target_host"))
	}
	if row.GetString("error") != "" {
		t.Fatalf("error %q", row.GetString("error"))
	}
	if !row.GetDateTime("finished").Time().After(row.GetDateTime("started").Time().Add(-time.Second)) {
		t.Fatalf("finished %v not after started %v", row.GetDateTime("finished"), row.GetDateTime("started"))
	}

	m, rep, err := format.Inspect(bytes.NewReader(sink.Bytes()), id)
	if err != nil || !rep.OK {
		t.Fatalf("inspect: %v %+v", err, rep)
	}
	if m.Lockfile["tinycld"] != "tinycld@1.2.3" || m.Lockfile["widgets"] != "@example/widgets@1.0.0" || m.Packages["widgets"] != "1.0.0" || m.Core != "1.2.3" {
		t.Fatalf("manifest %+v", m)
	}
	if m.Kind != "manual" || m.Source != "docker" {
		t.Fatalf("manifest kind %q source %q", m.Kind, m.Source)
	}
	if m.Counts.Files != 1 || m.Counts.Collections["users"] != 1 {
		t.Fatalf("counts %+v", m.Counts)
	}
	var names []string
	for _, mem := range rep.Members {
		names = append(names, mem.Name)
	}
	if len(names) != 2 || names[0] != "data.db" || names[1] != "storage/col1/rec1/hello.txt" {
		t.Fatalf("members %v", names)
	}

	// The snapshot must be a real, readable SQLite database — a byte-identical
	// round trip through the container proves nothing about VACUUM INTO.
	dbPath := extractMember(t, sink.Bytes(), id, format.MemberDB)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var users int
	if err := db.QueryRow("SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatalf("query the extracted snapshot: %v", err)
	}
	if users != 1 {
		t.Fatalf("snapshot users %d, want 1", users)
	}

	notifs, _ := app.FindRecordsByFilter("notifications", "type = 'core.backup.succeeded'", "", 0, 0)
	if len(notifs) != 1 {
		t.Fatalf("want 1 notification, got %d", len(notifs))
	}
	audits, _ := app.FindRecordsByFilter("audit_logs", "action = 'backup.created'", "", 0, 0)
	if len(audits) != 1 {
		t.Fatalf("want 1 audit row, got %d", len(audits))
	}
	if entries, _ := os.ReadDir(tmpDir(app)); len(entries) != 0 {
		t.Fatalf("temp dir not clean: %v", entries)
	}
}

// extractMember writes one archive member to a file and returns its path.
func extractMember(t *testing.T, archive []byte, identity age.Identity, member string) string {
	t.Helper()
	rd, err := format.NewReader(bytes.NewReader(archive), identity)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	if _, err := rd.ReadManifest(); err != nil {
		t.Fatal(err)
	}
	for {
		hdr, body, err := rd.Next()
		if err == io.EOF {
			t.Fatalf("member %q not found", member)
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name != member {
			continue
		}
		path := filepath.Join(t.TempDir(), filepath.Base(member))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(f, body); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return path
	}
}

type failingSink struct {
	n      int
	closed bool
}

func (f *failingSink) Write(p []byte) (int, error) {
	f.n += len(p)
	if f.n > 1024 {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}
func (f *failingSink) Close() error { f.closed = true; return nil }

func TestRunRecordsFailure(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	id, _ := age.GenerateX25519Identity()
	sink := &failingSink{}
	rowID, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: sink})
	if err == nil {
		t.Fatal("expected error")
	}
	// A failed run still owns the sink: an HTTP PUT sink holds a pipe and a
	// goroutine, and not closing it leaks both.
	if !sink.closed {
		t.Fatal("a failed run must still close its sink")
	}
	row, ferr := app.FindRecordById("backups", rowID)
	if ferr != nil {
		t.Fatal(ferr)
	}
	if row.GetString("status") != "failed" || row.GetString("error") == "" {
		t.Fatalf("row %v", row.PublicExport())
	}
	notifs, _ := app.FindRecordsByFilter("notifications", "type = 'core.backup.failed'", "", 0, 0)
	if len(notifs) != 1 {
		t.Fatalf("want failure notification, got %d", len(notifs))
	}
	audits, _ := app.FindRecordsByFilter("audit_logs", "action = 'backup.failed'", "", 0, 0)
	if len(audits) != 1 {
		t.Fatalf("want 1 failure audit row, got %d", len(audits))
	}
	if entries, _ := os.ReadDir(tmpDir(app)); len(entries) != 0 {
		t.Fatalf("temp dir not clean: %v", entries)
	}
	// The interlock must be free again after a failed run.
	if installjob.Running() {
		t.Fatal("interlock still held after a failed run")
	}
}

func TestRunRefusesWhileJobRunning(t *testing.T) {
	app := newTestApp(t)
	other := installjob.New("install", "x", "x")
	if _, ok := installjob.Claim(other); !ok {
		t.Fatal("claim")
	}
	t.Cleanup(func() { installjob.Release(other) })
	id, _ := age.GenerateX25519Identity()
	_, err := Run(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: &closeBuffer{}})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("want ErrBusy, got %v", err)
	}
	rows, _ := app.FindRecordsByFilter("backups", "id != ''", "", 0, 0)
	if len(rows) != 0 {
		t.Fatalf("a refused run must not leave a ledger row: %d", len(rows))
	}
}

func TestDailyLimitAppliesToManualOnly(t *testing.T) {
	app := newTestApp(t)
	owner := makeUser(t, app, "owner@example.com", "owner")
	t.Cleanup(resetDailyLimitForTesting)
	resetDailyLimitForTesting()
	SetDailyLimit(func(core.App) int { return 1 })
	id, _ := age.GenerateX25519Identity()
	if _, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: &closeBuffer{}, Initiator: owner.Id}); err != nil {
		t.Fatal(err)
	}
	_, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: &closeBuffer{}, Initiator: owner.Id})
	if !errors.Is(err, ErrRateLimit) {
		t.Fatalf("want ErrRateLimit, got %v", err)
	}
	if _, err := Run(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: &closeBuffer{}}); err != nil {
		t.Fatalf("scheduled must be exempt: %v", err)
	}
}

func TestSetDailyLimitClaimsOnce(t *testing.T) {
	t.Cleanup(resetDailyLimitForTesting)
	resetDailyLimitForTesting()
	SetDailyLimit(func(core.App) int { return 3 })
	SetDailyLimit(func(core.App) int { return 99 })
	if got := dailyLimit(nil); got != 3 {
		t.Fatalf("limit %d, want the first claim (3)", got)
	}
}

func TestMarkInterrupted(t *testing.T) {
	app := newTestApp(t)
	row := newRow(app, KindManual, "", "")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	if err := MarkInterrupted(app, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := app.FindRecordById("backups", row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetString("status") != "interrupted" {
		t.Fatalf("status %q", got.GetString("status"))
	}
	if got.GetString("error") == "" || got.GetDateTime("finished").IsZero() {
		t.Fatalf("an interrupted row needs a reason and a finish time: %v", got.PublicExport())
	}
}

func TestMarkInterruptedLeavesRunsFromThisBoot(t *testing.T) {
	app := newTestApp(t)
	row := newRow(app, KindScheduled, "", "")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	if err := MarkInterrupted(app, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := app.FindRecordById("backups", row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetString("status") != "running" {
		t.Fatalf("status %q, want running", got.GetString("status"))
	}
}

func TestCallbackReceivesRow(t *testing.T) {
	app := newTestApp(t)
	got := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- body
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	id, _ := age.GenerateX25519Identity()
	rowID, err := Run(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: &closeBuffer{}, Callback: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	var body []byte
	select {
	case body = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the callback was never called")
	}
	if !bytes.Contains(body, []byte(rowID)) || !bytes.Contains(body, []byte(`"status":"succeeded"`)) {
		t.Fatalf("callback body %s", body)
	}
}

func TestStartRunsAsynchronously(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	id, _ := age.GenerateX25519Identity()
	// The callback is the LAST thing a run does, so waiting on it is what keeps
	// the test from tearing the app down under the still-running goroutine.
	finished := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		finished <- body
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)

	rowID, err := Start(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: &closeBuffer{}, Callback: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	// The row exists the moment Start returns: the ledger is written first.
	if _, err := app.FindRecordById("backups", rowID); err != nil {
		t.Fatalf("Start must insert the row before returning: %v", err)
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("the asynchronous run never finished")
	}
	row, err := app.FindRecordById("backups", rowID)
	if err != nil {
		t.Fatal(err)
	}
	if row.GetString("status") != "succeeded" {
		t.Fatalf("status %q", row.GetString("status"))
	}
}

func TestPreRestoreDoesNotAnnounce(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	id, _ := age.GenerateX25519Identity()
	if _, err := Run(app, Request{Kind: KindPreRestore, Recipient: id.Recipient(), Sink: &closeBuffer{}}); err != nil {
		t.Fatal(err)
	}
	notifs, _ := app.FindRecordsByFilter("notifications", "id != ''", "", 0, 0)
	if len(notifs) != 0 {
		t.Fatalf("a pre-restore backup must not announce itself: %d", len(notifs))
	}
}

func TestHostOnlyKeepsOnlyTheHostname(t *testing.T) {
	cases := map[string]string{
		"https://example.com/path?sig=secret":      "example.com",
		"https://user:pw@files.example.com:8443/x": "files.example.com",
		"":          "",
		"not a url": "",
	}
	for raw, want := range cases {
		if got := HostOnly(raw); got != want {
			t.Fatalf("HostOnly(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSetSourceIsRecordedInTheManifest(t *testing.T) {
	app := newTestApp(t)
	t.Cleanup(func() { SetSource("docker") })
	SetSource("standalone")
	id, _ := age.GenerateX25519Identity()
	sink := &closeBuffer{}
	if _, err := Run(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: sink}); err != nil {
		t.Fatal(err)
	}
	m, _, err := format.Inspect(bytes.NewReader(sink.Bytes()), id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "standalone" {
		t.Fatalf("source %q", m.Source)
	}
}

func TestLedgerPathIsTheParentOfTheDataDir(t *testing.T) {
	app, err := newBareApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	parent := filepath.Dir(app.DataDir())
	if got := LedgerPath(app); got != parent {
		t.Fatalf("LedgerPath = %q, want %q", got, parent)
	}
	if got, want := tmpDir(app), filepath.Join(parent, "backup-tmp"); got != want {
		t.Fatalf("tmpDir = %q, want %q", got, want)
	}
}

// slowSink stalls a write in the middle of the archive body, so the run
// outlives the 2 s progress ticker. That is the only window in which the
// ticker goroutine and the run touch the same record; without it, -race never
// exercises the progress writer at all. It stalls on a LARGE write, because
// every small one belongs to the age and zstd headers, which NewWriter emits
// before the ticker exists.
type slowSink struct {
	bytes.Buffer
	delay   time.Duration
	stalled bool
	ticked  func() bool
}

func (s *slowSink) Write(p []byte) (int, error) {
	if !s.stalled && len(p) > 1024 {
		s.stalled = true
		deadline := time.Now().Add(s.delay)
		for time.Now().Before(deadline) && !s.ticked() {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return s.Buffer.Write(p)
}

func (s *slowSink) Close() error { return nil }

func TestProgressTickerDoesNotRaceTheFinalSave(t *testing.T) {
	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	id, _ := age.GenerateX25519Identity()
	var ticks atomic.Int64
	progressTickForTesting = func() { ticks.Add(1) }
	t.Cleanup(func() { progressTickForTesting = nil })

	sink := &slowSink{delay: 10 * time.Second, ticked: func() bool { return ticks.Load() > 0 }}
	rowID, err := Run(app, Request{Kind: KindScheduled, Recipient: id.Recipient(), Sink: sink})
	if err != nil {
		t.Fatal(err)
	}
	if ticks.Load() == 0 {
		t.Fatal("the progress ticker never fired, so this test proves nothing")
	}
	row, err := app.FindRecordById("backups", rowID)
	if err != nil {
		t.Fatal(err)
	}
	// The terminal save must win: the ticker cannot leave a stale byte count.
	if row.GetString("status") != "succeeded" {
		t.Fatalf("status %q", row.GetString("status"))
	}
	if row.GetInt("bytes") != sink.Len() {
		t.Fatalf("bytes %d, want the final %d", row.GetInt("bytes"), sink.Len())
	}
}
