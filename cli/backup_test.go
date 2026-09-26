package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"

	"tinycld.org/cli/client"
	"tinycld.org/cli/internal/config"
	"tinycld.org/cli/ui"
	"tinycld.org/core/backup/format"
)

const testPassphrase = "correct horse battery"

// buildArchive produces a real age-encrypted archive, so the inspect and
// restore tests exercise the same format the server writes rather than a
// stand-in the CLI could parse by accident.
func buildArchive(t *testing.T) []byte {
	t.Helper()
	rcpt, err := age.NewScryptRecipient(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	// The lowest work factor the library accepts: these tests build archives
	// on every run and scrypt at the default factor dominates the suite.
	rcpt.SetWorkFactor(2)
	var buf bytes.Buffer
	w, err := format.NewWriter(&buf, rcpt, zstd.SpeedFastest)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WriteManifest(format.Manifest{
		Created:  time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
		Instance: "acme",
		Source:   "standalone",
		Kind:     "manual",
		Core:     "2.4.0",
		Packages: map[string]string{"mail": "1.2.0", "drive": "0.9.1"},
		Lockfile: format.Lockfile{"mail": "sha256-mail", "drive": "sha256-drive"},
		Counts:   format.Counts{Collections: map[string]int{"messages": 3}, Files: 2, Bytes: 11},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile(format.MemberDB, 7, strings.NewReader("sqlite3")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile(format.StoragePrefix+"a.txt", 4, strings.NewReader("abcd")); err != nil {
		t.Fatal(err)
	}
	// Big and incompressible, so the archive spans several age payload chunks.
	// The truncation test depends on that: with one chunk, cutting the tail
	// breaks the manifest too, and the "prints what it could read" behaviour
	// has nothing to prove.
	bulk := make([]byte, 512*1024)
	if _, err := rand.Read(bulk); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile(format.StoragePrefix+"bulk.bin", int64(len(bulk)), bytes.NewReader(bulk)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeArchiveFile(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.age")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordedRestore is what the fake server saw on a restore request, so the
// tests can assert the part order the streaming server-side reader depends on.
type recordedRestore struct {
	partOrder  []string
	passphrase string
	force      string
	archive    []byte
	json       map[string]any
}

// backupServer is the fake org-backup API: the create/restore endpoints, the
// ledger row polls, and the records list `backup list` reads.
type backupServer struct {
	srv     *httptest.Server
	archive []byte

	mu sync.Mutex
	// createBody is the JSON body the last POST /api/org-backups carried.
	createBody map[string]any
	// createStatus, when non-zero, is returned instead of running a backup.
	createStatus  int
	createMessage string
	requests      []string
	// rows is the scripted sequence GET /api/org-backups/{id} walks through,
	// per id. The last entry repeats once exhausted.
	rows map[string][]ledgerRow
	// polls counts GETs per id.
	polls map[string]int
	// restore records what the restore endpoint received.
	restore recordedRestore
	// mismatch makes the restore endpoint answer 409 with a package diff.
	mismatch bool
	// mismatchWithoutDiff answers 409 with the message only — an older server
	// that has no structured diff to send.
	mismatchWithoutDiff bool
	// conflictMessage overrides that bare 409's message.
	conflictMessage string
	// swapped is the source PATCHed into a waiting restore.
	swapped string
	// list is what the records API returns for an unfiltered read.
	list []ledgerRow
	// filtered is what a filtered read returns, and listFilter is the filter
	// the last one carried.
	filtered   []ledgerRow
	listFilter string
	// deadPolls makes the next N ledger GETs hang up without a response, so
	// the client sees a connection error the way it does across a restart.
	deadPolls int
	// notFoundAfter makes the ledger GET answer 404 once it has served this
	// many rows — the restore replaced the database the row lived in.
	notFoundAfter int
	// restoringPolls makes the next N ledger GETs answer the maintenance 503
	// with {"status":"restoring"} — the server is up and staging a restore.
	restoringPolls int
	// restoringSeen counts the 503s actually served.
	restoringSeen int
}

func newBackupServer(t *testing.T) *backupServer {
	t.Helper()
	s := &backupServer{
		archive: buildArchive(t),
		rows:    map[string][]ledgerRow{},
		polls:   map[string]int{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/org-backups", s.handleCreate)
	mux.HandleFunc("POST /api/org-backups/restore", s.handleRestore)
	mux.HandleFunc("PATCH /api/org-backups/restore/{id}", s.handleSwap)
	mux.HandleFunc("GET /api/org-backups/{id}", s.handleGet)
	mux.HandleFunc("GET /api/collections/backups/records", s.handleList)
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *backupServer) record(what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, what)
}

func (s *backupServer) restoringCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restoringSeen
}

func (s *backupServer) pollCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.polls[id]
}

func (s *backupServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *backupServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	s.record("POST /api/org-backups")
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.createBody = body
	status, message := s.createStatus, s.createMessage
	s.mu.Unlock()

	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}
	if stream, _ := body["stream"].(bool); stream {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.archive)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": "b1"})
}

func (s *backupServer) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.record("GET /api/org-backups/" + id)
	s.mu.Lock()
	if s.deadPolls > 0 {
		s.deadPolls--
		// Counted like any other poll: a test asserting how many times the CLI
		// asked must see the attempts that never got an answer.
		s.polls[id]++
		s.mu.Unlock()
		// Hijack and close: the client sees a connection error, which is what
		// a restarting server looks like from the CLI's side.
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			conn.Close()
		}
		return
	}
	if s.restoringPolls > 0 {
		s.restoringPolls--
		// Counted in restoringSeen, not in polls: polls indexes the scripted row
		// sequence, and a 503 served no row — advancing it would skip one.
		s.restoringSeen++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "restoring",
			"message": "Restoring from a backup. Try again in a minute.",
		})
		return
	}
	seq := s.rows[id]
	n := s.polls[id]
	s.polls[id] = n + 1
	gone := s.notFoundAfter > 0 && n >= s.notFoundAfter
	s.mu.Unlock()

	if gone || len(seq) == 0 {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"backup not found"}`))
		return
	}
	if n >= len(seq) {
		n = len(seq) - 1
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(seq[n])
}

func (s *backupServer) handleRestore(w http.ResponseWriter, r *http.Request) {
	s.record("POST /api/org-backups/restore")
	rec := recordedRestore{}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		_, params, err := mime.ParseMediaType(ct)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			rec.partOrder = append(rec.partOrder, part.FormName())
			body, _ := io.ReadAll(part)
			switch part.FormName() {
			case "passphrase":
				rec.passphrase = string(body)
			case "force":
				rec.force = string(body)
			case "archive":
				rec.archive = body
			}
		}
	} else {
		_ = json.NewDecoder(r.Body).Decode(&rec.json)
	}
	s.mu.Lock()
	s.restore = rec
	mismatch, bare, bareMessage := s.mismatch, s.mismatchWithoutDiff, s.conflictMessage
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if bare {
		message := bareMessage
		if message == "" {
			message = "backup: archive does not match this binary's package set (missing: widgets)"
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}
	if mismatch {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "backup: archive does not match this binary's package set (missing: widgets)",
			"diff": map[string]any{
				"missing":      []string{"widgets"},
				"extra":        []string{},
				"versionDelta": map[string][2]string{},
			},
			"jobId": "r1",
		})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": "r1"})
}

func (s *backupServer) handleSwap(w http.ResponseWriter, r *http.Request) {
	s.record("PATCH /api/org-backups/restore/" + r.PathValue("id"))
	var body struct {
		Source string `json:"source"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.swapped = body.Source
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *backupServer) handleList(w http.ResponseWriter, r *http.Request) {
	s.record("GET /api/collections/backups/records?" + r.URL.RawQuery)
	s.mu.Lock()
	rows := s.list
	if f := r.URL.Query().Get("filter"); f != "" {
		s.listFilter = f
		rows = s.filtered
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(client.ListResult[ledgerRow]{
		Page: 1, PerPage: 200, TotalItems: len(rows), TotalPages: 1, Items: rows,
	})
}

// backupDeps wires deps to the fake server with a stored credential and the
// passphrase in the environment, so a command reaches the endpoint without a
// terminal.
func backupDeps(t *testing.T, s *backupServer) *deps {
	t.Helper()
	d, store := testDeps(t)
	d.httpClient = s.srv.Client()
	d.getenv = func(name string) string {
		if name == "TINYCLD_BACKUP_PASSPHRASE" {
			return testPassphrase
		}
		return ""
	}

	host := strings.TrimPrefix(s.srv.URL, "http://")
	cfg := &config.Config{
		Current:  host,
		Contexts: map[string]config.Context{host: {Origin: s.srv.URL}},
	}
	if err := cfg.Save(d.configDir); err != nil {
		t.Fatal(err)
	}
	tok, err := json.Marshal(client.TokenSet{AccessToken: "access-1", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(host, string(tok)); err != nil {
		t.Fatal(err)
	}
	return d
}

func wantExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with exit code %d, got nil", want)
	}
	if got := exitCode(err); got != want {
		t.Fatalf("exit code = %d, want %d (err: %v)", got, want, err)
	}
}

func TestBackupCreateOutStreamsAValidArchive(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	out := filepath.Join(t.TempDir(), "out.age")

	_, stderr, err := runCLI(t, d, "backup", "create", "--out", out)
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	id, err := age.NewScryptIdentity(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	m, rep, verr := format.Inspect(bytes.NewReader(body), id)
	if verr != nil {
		t.Fatalf("the written archive does not verify: %v", verr)
	}
	if m.Core != "2.4.0" || !rep.OK {
		t.Fatalf("manifest = %+v report = %+v", m, rep)
	}
	if !strings.Contains(stderr, out) {
		t.Errorf("stderr should name the file it wrote, got %q", stderr)
	}
	if s.createBody["stream"] != true {
		t.Errorf("create body = %+v, want stream:true", s.createBody)
	}
}

func TestBackupCreateOutDashWritesStdout(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)

	stdout, _, err := runCLI(t, d, "backup", "create", "--out", "-")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != string(s.archive) {
		t.Fatalf("stdout carried %d bytes, want the %d-byte archive verbatim", len(stdout), len(s.archive))
	}
}

func TestBackupCreateToPollsTheLedger(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["b1"] = []ledgerRow{
		{ID: "b1", Kind: "manual", Status: "running", Bytes: 10},
		{ID: "b1", Kind: "manual", Status: "running", Bytes: 20},
		{ID: "b1", Kind: "manual", Status: "succeeded", Bytes: 20},
	}

	_, stderr, err := runCLI(t, d, "backup", "create", "--to", "https://store.example/put?sig=abc")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.createBody["target"]; got != "https://store.example/put?sig=abc" {
		t.Errorf("target = %v", got)
	}
	if !strings.Contains(stderr, "20 B") {
		t.Errorf("stderr should report the transferred bytes, got %q", stderr)
	}
	if !strings.Contains(stderr, "succeeded") {
		t.Errorf("stderr should report the terminal status, got %q", stderr)
	}
}

func TestBackupCreateRefusesShortPassphraseWithoutARequest(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	d.getenv = func(string) string { return "short" }

	_, _, err := runCLI(t, d, "backup", "create", "--out", filepath.Join(t.TempDir(), "x.age"))
	wantExitCode(t, err, 2)
	if !strings.Contains(err.Error(), "at least 12") {
		t.Errorf("error = %v, want the length rule", err)
	}
	// The whole point of checking client-side: a too-short phrase must never
	// leave the machine.
	if seen := s.seen(); len(seen) != 0 {
		t.Errorf("server saw %v, want no request at all", seen)
	}
}

func TestBackupCreateRateLimitedExitsOne(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.createStatus = http.StatusTooManyRequests
	s.createMessage = "The daily limit for manual backups has been reached."

	_, _, err := runCLI(t, d, "backup", "create", "--to", "https://store.example/put")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "daily limit") {
		t.Errorf("error = %v, want the server's message", err)
	}
}

func TestBackupCreateRequiresExactlyOneDestination(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)

	_, _, err := runCLI(t, d, "backup", "create")
	wantExitCode(t, err, 2)

	_, _, err = runCLI(t, d, "backup", "create", "--out", "-", "--to", "https://store.example/put")
	wantExitCode(t, err, 2)
	if seen := s.seen(); len(seen) != 0 {
		t.Errorf("server saw %v, want no request", seen)
	}
}

func TestBackupInspectFile(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	path := writeArchiveFile(t, s.archive)

	stdout, _, err := runCLI(t, d, "backup", "inspect", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"format", "created", "2026-09-24T12:00:00Z", "standalone", "2.4.0", "package mail", "1.2.0", "member data.db", "OK", "verified"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("inspect output missing %q:\n%s", want, stdout)
		}
	}

	stdout, _, err = runCLI(t, d, "backup", "inspect", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Manifest format.Manifest `json:"manifest"`
		Report   format.Report   `json:"report"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("inspect --json parse: %v\n%s", err, stdout)
	}
	if payload.Manifest.Core != "2.4.0" || !payload.Report.OK || len(payload.Report.Members) != 3 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBackupInspectTruncatedFileFails(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// Cut the tail off. The manifest is the first member, so it still reads —
	// which is what makes this the interesting failure: the command has
	// something to print and must still exit non-zero.
	path := writeArchiveFile(t, s.archive[:len(s.archive)-64])

	stdout, _, err := runCLI(t, d, "backup", "inspect", path)
	wantExitCode(t, err, 1)
	if !strings.Contains(stdout, "2.4.0") {
		t.Errorf("a failed verification should still print what it could read:\n%s", stdout)
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("error = %v, want it to name the verification", err)
	}
}

func TestBackupInspectWrongPassphraseFails(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	d.getenv = func(string) string { return "not the passphrase" }
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "inspect", path)
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "passphrase") {
		t.Errorf("error = %v, want it to name the passphrase as a cause", err)
	}
}

func TestBackupRestoreFromFileUploadsInOrder(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{
		{ID: "r1", Kind: "restore", Status: "running"},
		{ID: "r1", Kind: "restore", Status: "succeeded", Bytes: 512},
	}
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	// The server reads the passphrase before it will accept the archive, so a
	// reordered body is rejected: the order is part of the contract.
	if got := strings.Join(s.restore.partOrder, ","); got != "passphrase,archive" {
		t.Errorf("part order = %q, want passphrase,archive", got)
	}
	if s.restore.passphrase != testPassphrase {
		t.Errorf("passphrase part = %q", s.restore.passphrase)
	}
	if s.restore.force != "" {
		t.Errorf("force part = %q, want it absent without --force", s.restore.force)
	}
	if !bytes.Equal(s.restore.archive, s.archive) {
		t.Errorf("archive part was %d bytes, want %d", len(s.restore.archive), len(s.archive))
	}
	if !strings.Contains(stderr, "succeeded") {
		t.Errorf("stderr = %q, want the terminal status", stderr)
	}
}

func TestBackupRestoreForceSendsField(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "succeeded"}}
	path := writeArchiveFile(t, s.archive)

	if _, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--force", "--yes"); err != nil {
		t.Fatal(err)
	}
	if s.restore.force != "true" {
		t.Errorf("force part = %q, want \"true\"", s.restore.force)
	}
	if got := strings.Join(s.restore.partOrder, ","); got != "passphrase,force,archive" {
		t.Errorf("part order = %q", got)
	}
}

func TestBackupRestoreMismatchPrintsDiffExitsOne(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.mismatch = true
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(stderr, "missing: widgets") {
		t.Errorf("stderr should name the missing package:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("stderr should say how to proceed anyway:\n%s", stderr)
	}
}

func TestBackupRestoreRequiresYesWhenNotInteractive(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path)
	if err == nil {
		t.Fatal("a restore without --yes on a non-TTY must refuse")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error = %v, want it to name --yes", err)
	}
	for _, r := range s.seen() {
		if strings.HasPrefix(r, "POST") {
			t.Errorf("server saw %q before the confirmation", r)
		}
	}
}

func TestBackupRestoreFromURLRefusesWaitingUnderYes(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "waiting_for_source"}}

	_, _, err := runCLI(t, d, "backup", "restore", "--from", "https://store.example/get?sig=abc", "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "fresh source URL") {
		t.Errorf("error = %v, want it to explain what is needed", err)
	}
	if s.restore.json["source"] != "https://store.example/get?sig=abc" {
		t.Errorf("restore body = %+v", s.restore.json)
	}
}

func TestBackupRestoreFromURLSwapsAFreshSource(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	d.isInteractive = true
	s.rows["r1"] = []ledgerRow{
		{ID: "r1", Kind: "restore", Status: "waiting_for_source"},
		{ID: "r1", Kind: "restore", Status: "running"},
		{ID: "r1", Kind: "restore", Status: "succeeded", Bytes: 64},
	}

	root := newRootCmd(d)
	var stdout, stderr bytes.Buffer
	d.stdout, d.stderr = &stdout, &stderr
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	// The confirmation and the fresh URL both come off stdin, in that order.
	root.SetIn(strings.NewReader("y\nhttps://store.example/fresh?sig=def\n"))
	root.SetArgs([]string{"backup", "restore", "--from", "https://store.example/get?sig=abc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr.String())
	}
	if s.swapped != "https://store.example/fresh?sig=def" {
		t.Errorf("swapped source = %q", s.swapped)
	}
	if !strings.Contains(stderr.String(), "succeeded") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestBackupRestoreSurvivesServerRestart(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// The rebuilder replaces the process, so the first polls after the restore
	// is staged find nothing listening.
	s.deadPolls = 2
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "succeeded", Bytes: 99}}
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "unreachable") {
		t.Errorf("stderr should say the server went away:\n%s", stderr)
	}
	if !strings.Contains(stderr, "succeeded") {
		t.Errorf("stderr should report the outcome read after the restart:\n%s", stderr)
	}
}

func TestBackupRestoreFailedRowExitsOne(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "failed", Error: "checksum mismatch"}}
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want the row's reason", err)
	}
}

func TestBackupRestoreRefusesAnUnverifiableArchive(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	path := writeArchiveFile(t, s.archive[:len(s.archive)-64])

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	// Uploading a broken archive costs the whole transfer and then replaces
	// the live data with it, so the refusal belongs before the request.
	for _, r := range s.seen() {
		if strings.HasPrefix(r, "POST") {
			t.Errorf("server saw %q for an archive that does not verify", r)
		}
	}
}

func TestBackupListRendersLedger(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.list = []ledgerRow{
		{ID: "b2", Kind: "manual", Status: "succeeded", Started: "2026-09-24 12:00:00Z", Bytes: 2048, TargetHost: "store.example"},
		{ID: "b1", Kind: "scheduled", Status: "failed", Started: "2026-09-23 12:00:00Z", Error: "target refused"},
	}

	stdout, _, err := runCLI(t, d, "backup", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"KIND", "STATUS", "STARTED", "SIZE", "TARGET", "manual", "2.0 KB", "store.example", "target refused"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output missing %q:\n%s", want, stdout)
		}
	}
	if seen := s.seen(); len(seen) == 0 || !strings.Contains(seen[0], "sort=-started") {
		t.Errorf("list request = %v, want it sorted newest first", seen)
	}

	stdout, _, err = runCLI(t, d, "backup", "list", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []ledgerRow
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("list --output json parse: %v\n%s", err, stdout)
	}
	if len(rows) != 2 || rows[0].ID != "b2" {
		t.Fatalf("rows = %+v", rows)
	}
}

// The passphrase is the one secret this command group handles, and both
// streams are things an operator pastes into a bug report.
func TestBackupOutputNeverCarriesThePassphrase(t *testing.T) {
	s := newBackupServer(t)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "succeeded"}}
	path := writeArchiveFile(t, s.archive)

	for _, args := range [][]string{
		{"backup", "create", "--out", filepath.Join(t.TempDir(), "o.age")},
		{"backup", "inspect", path},
		{"backup", "restore", "--from", path, "--yes"},
	} {
		d := backupDeps(t, s)
		stdout, stderr, err := runCLI(t, d, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		// Not stdout for `create --out <file>`: that writes the archive, whose
		// ciphertext is not the phrase. Every other stream is prose.
		if strings.Contains(stderr, testPassphrase) {
			t.Errorf("%v leaked the passphrase to stderr", args)
		}
		if args[1] != "create" && strings.Contains(stdout, testPassphrase) {
			t.Errorf("%v leaked the passphrase to stdout", args)
		}
	}
}

// A presigned URL carries its credentials in the query string, and Go's
// *url.Error prints the whole URL. An operator pastes a CLI error into a bug
// report, so the signature must be stripped before it gets there.
func TestBackupInspectURLErrorHidesTheQueryString(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// A closed listener: the transport fails with a *url.Error carrying the
	// full URL, which is the case the redaction exists for.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	_, _, err = runCLI(t, d, "backup", "inspect", "http://"+addr+"/archive.age?X-Amz-Signature=SECRETSIG")
	if err == nil {
		t.Fatal("inspecting an unreachable URL must fail")
	}
	if strings.Contains(err.Error(), "SECRETSIG") {
		t.Errorf("error leaked the presigned query string: %v", err)
	}
	// The host is what an operator needs, and keeping it asserted stops the
	// redaction being "widened" into dropping everything useful.
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("error should still name the host: %v", err)
	}
}

// The restore replaces the whole database, the safety-copy row included, so a
// poll that already read a state and then gets a 404 has seen everything there
// is to see. Reporting the 404 instead would turn a restore that worked into a
// failure.
func TestBackupRestoreReadsTheOutcomeWrittenAfterTheSwap(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// Non-terminal, so the poll goes round again and meets the 404 — a
	// terminal first row would return before the 404 and prove nothing.
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running", Bytes: 128}}
	s.notFoundAfter = 1
	// The row the restored database's own boot wrote, pointing back at r1.
	s.filtered = []ledgerRow{{
		ID: "r9", Kind: "restore", Status: "succeeded", Bytes: 4096,
		Metadata: map[string]any{"restored_from_job": "r1"},
	}}
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "succeeded") || !strings.Contains(stderr, "4.0 KB") {
		t.Errorf("stderr = %q, want the outcome from the row written after the swap", stderr)
	}
	if !strings.Contains(s.listFilter, "restored_from_job") || !strings.Contains(s.listFilter, `"r1"`) {
		t.Errorf("filter = %q, want it keyed on the original job id", s.listFilter)
	}
}

// A rolled-back restore writes a FAILED row after the swap. Reading the 404 as
// success would report the rollback as a good restore, which is the worst
// possible answer: the operator believes their data was replaced when it was
// put back.
func TestBackupRestoreReportsARollbackRecordedAfterTheSwap(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running"}}
	s.notFoundAfter = 1
	s.filtered = []ledgerRow{{
		ID: "r9", Kind: "restore", Status: "failed", Error: "the restored data did not boot",
		Metadata: map[string]any{"restored_from_job": "r1"},
	}}
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "did not boot") {
		t.Errorf("error = %v, want the rollback's reason", err)
	}
}

// The row is gone and nothing replaced it: the outcome is genuinely unknown, so
// the command says so rather than guessing either way.
func TestBackupRestoreRefusesToGuessWhenNoOutcomeWasRecorded(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running"}}
	s.notFoundAfter = 1
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "backup list") {
		t.Errorf("error = %v, want it to say where to look", err)
	}
}

// A 404 on the FIRST poll never saw a row at all, so there is nothing to
// follow: the command must not claim success.
func TestBackupRestoreFailsWhenTheRowNeverAppears(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// No rows scripted for r1 at all: every poll 404s.
	path := writeArchiveFile(t, s.archive)

	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %v, want the server's refusal, not a guessed outcome", err)
	}
}

// A server that cannot restart itself (dev mode) stages the restore, arms it on
// disk, and goes back to serving its CURRENT data: the row keeps status
// "running" and sets metadata.awaiting_restart. It never becomes terminal, so a
// poll that waits for a terminal status waits forever — this asserts the poll
// stops and the operator is told what is left to do.
func TestBackupRestoreStopsWhenStagedAwaitingARestart(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["r1"] = []ledgerRow{
		{ID: "r1", Kind: "restore", Status: "running"},
		{ID: "r1", Kind: "restore", Status: "running", Metadata: map[string]any{"awaiting_restart": true}},
	}
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	// Nothing failed: the data is staged and one restart away.
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "restore staged; restart the server to apply it") {
		t.Errorf("stderr = %q, want the staged-awaiting-restart message", stderr)
	}
	// The poll must STOP. Without the check it would keep asking forever, which
	// a passing exit code alone would not catch.
	if got := s.pollCount("r1"); got != 2 {
		t.Errorf("polled the row %d times, want 2 — the poll did not stop at the staged row", got)
	}
}

// pollRow is shared with `create --to`, and only a restore replaces the database
// its own row lives in. A backup whose row 404s has simply lost the row, so
// chasing a follow-up row that cannot exist would report the wrong thing.
func TestBackupCreateReportsAVanishedRowWithoutChasingASwap(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.rows["b1"] = []ledgerRow{{ID: "b1", Kind: "manual", Status: "running", Bytes: 10}}
	s.notFoundAfter = 1

	_, _, err := runCLI(t, d, "backup", "create", "--to", "https://store.example/put")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "disappeared") {
		t.Errorf("error = %v, want it to say the row went away", err)
	}
	// The records API is the restore-only swap lookup. A backup must not touch it.
	for _, r := range s.seen() {
		if strings.Contains(r, "/api/collections/backups/records") {
			t.Errorf("create chased the restore-only swap lookup: %q", r)
		}
	}
}

// The reconnect window exists so a restore survives the restart it causes. It
// must also END: a server that never comes back has to be reported, not waited
// on forever. Driven by an injected clock — the real window is five minutes.
func TestBackupRestoreGivesUpAfterTheReconnectWindow(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	// Every poll after the first finds nothing listening.
	s.deadPolls = 1000
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running"}}

	clock := time.Now()
	d.now = func() time.Time { return clock }
	// Advance past the window on the same schedule the poll sleeps on, so the
	// loop reaches the deadline instead of spinning on a frozen clock.
	d.sleep = func(time.Duration) { clock = clock.Add(pollInterval) }

	path := writeArchiveFile(t, s.archive)
	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "did not come back within") {
		t.Errorf("error = %v, want the reconnect window to be named", err)
	}
	// It gave up at the deadline rather than after some other number of tries.
	if got := s.pollCount("r1"); got < 2 {
		t.Errorf("polled %d times, want the window's worth of retries", got)
	}
}

// A server that sends the refusal as prose with no structured diff still gets
// the operator the way out. Dropping the hint on that path would leave the
// mismatch looking like a dead end on exactly the older servers most likely to
// hit it.
func TestBackupRestoreMismatchWithoutADiffStillPrintsTheForceHint(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.mismatchWithoutDiff = true
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if !strings.Contains(stderr, "--force") {
		t.Errorf("stderr should still say how to proceed anyway:\n%s", stderr)
	}
	if !strings.Contains(stderr, "missing: widgets") {
		t.Errorf("stderr should carry the server's own message:\n%s", stderr)
	}
}

// …and a 409 that is NOT a package mismatch (another job holds the lock) must
// not get the --force hint: forcing would not help and the operator would try it.
func TestBackupRestoreBusyConflictDoesNotSuggestForce(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.mismatchWithoutDiff = true
	s.conflictMessage = "Another backup, restore or package job is running."
	path := writeArchiveFile(t, s.archive)

	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	wantExitCode(t, err, 1)
	if strings.Contains(stderr, "--force") {
		t.Errorf("a busy conflict is not a mismatch; --force would not help:\n%s", stderr)
	}
	if !strings.Contains(err.Error(), "Another backup") {
		t.Errorf("error = %v, want the server's reason", err)
	}
}

// While a server stages a restore, every request outside the maintenance
// exemptions meets 503 {"status":"restoring"}. That is the restore working — and
// this poll may be following THAT restore — so it must keep polling within the
// reconnect window rather than report the 503 as the result.
func TestBackupRestoreKeepsPollingThroughARestoring503(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.restoringPolls = 3
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "succeeded", Bytes: 11}}

	path := writeArchiveFile(t, s.archive)
	_, stderr, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes")
	if err != nil {
		t.Fatalf("a restoring 503 must not fail the poll: %v\n%s", err, stderr)
	}
	if got := s.restoringCount(); got != 3 {
		t.Errorf("served %d restoring 503s, want 3", got)
	}
	if !strings.Contains(stderr, "server is restoring") {
		t.Errorf("the wait should be announced once:\n%s", stderr)
	}
	// Announced ONCE, not per poll: three lines of the same news is noise.
	if n := strings.Count(stderr, "server is restoring"); n != 1 {
		t.Errorf("announced the restoring wait %d times, want 1:\n%s", n, stderr)
	}
	if !strings.Contains(stderr, "succeeded") {
		t.Errorf("the poll should have gone on to read the outcome:\n%s", stderr)
	}
}

// …and it must still END. A server stuck restoring past the window is reported,
// not waited on forever, and the message names the restoring state rather than
// claiming the server never came back.
func TestBackupRestoreGivesUpOnAnEndlessRestoring503(t *testing.T) {
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.restoringPolls = 100000
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running"}}

	clock := time.Now()
	d.now = func() time.Time { return clock }
	d.sleep = func(time.Duration) { clock = clock.Add(pollInterval) }

	path := writeArchiveFile(t, s.archive)
	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes",
		"--restart-timeout", "20s")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "still restoring after 20s") {
		t.Errorf("error = %v, want the restoring state and the flag's window named", err)
	}
}

// A 503 that is NOT the maintenance answer is a plain failure: a poll that waited
// out every 503 would sit through a real overload for the whole window instead of
// reporting it.
func TestPollTreatsAnOrdinary503AsAFailure(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"the maintenance answer", `{"status":"restoring","message":"Restoring from a backup."}`, true},
		{"an overload", `{"message":"Service Unavailable"}`, false},
		{"another status", `{"status":"draining"}`, false},
		{"not JSON", `<html>503</html>`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := &client.APIError{Status: http.StatusServiceUnavailable, Body: []byte(c.body)}
			if got := isRestoringError(err); got != c.want {
				t.Errorf("isRestoringError = %v, want %v", got, c.want)
			}
		})
	}
	// Only a 503 counts: a 409 carrying the same body is a different refusal.
	other := &client.APIError{Status: http.StatusConflict, Body: []byte(`{"status":"restoring"}`)}
	if isRestoringError(other) {
		t.Error("only a 503 is the maintenance answer")
	}
}

// --restart-timeout overrides the reconnect window. The default is 30 minutes,
// which is what a rebuild-and-restart needs, so every test that drives the window
// to its end uses the flag rather than a fake clock's worth of 30 minutes.
func TestRestartTimeoutFlagOverridesTheReconnectWindow(t *testing.T) {
	if reconnectWindow != 30*time.Minute {
		t.Fatalf("reconnectWindow = %s, want 30m — a rebuild takes longer than five minutes", reconnectWindow)
	}
	s := newBackupServer(t)
	d := backupDeps(t, s)
	s.deadPolls = 1000
	s.rows["r1"] = []ledgerRow{{ID: "r1", Kind: "restore", Status: "running"}}

	clock := time.Now()
	d.now = func() time.Time { return clock }
	d.sleep = func(time.Duration) { clock = clock.Add(pollInterval) }

	path := writeArchiveFile(t, s.archive)
	_, _, err := runCLI(t, d, "backup", "restore", "--from", path, "--yes",
		"--restart-timeout", "10s")
	wantExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "did not come back within 10s") {
		t.Errorf("error = %v, want the flag's window rather than the default", err)
	}
	// The flag really shortened the wait: the default would take 900 polls.
	if got := s.pollCount("r1"); got > 20 {
		t.Errorf("polled %d times for a 10s window — the flag was ignored", got)
	}
}

// The help text has to name the default, or an operator watching a long restore
// cannot tell whether the poll is about to give up.
func TestRestartTimeoutHelpNamesTheDefault(t *testing.T) {
	for _, args := range [][]string{
		{"backup", "restore", "--help"},
		{"backup", "create", "--help"},
	} {
		d, _ := testDeps(t)
		stdout, _, err := runCLI(t, d, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(stdout, "--restart-timeout") {
			t.Errorf("%v: help does not mention --restart-timeout:\n%s", args, stdout)
		}
		if !strings.Contains(stdout, reconnectWindow.String()) {
			t.Errorf("%v: help does not name the %s default:\n%s", args, reconnectWindow, stdout)
		}
	}
}

// A misused command exits 2, a command that ran and failed exits 1. Without the
// distinction a script cannot tell "I called it wrong" from "it did not work",
// and cobra's own flag and argument errors are plain errors that would exit 1.
func TestUsageErrorsExitTwo(t *testing.T) {
	cases := [][]string{
		{"--no-such-flag"},
		{"backup", "--no-such-flag"},
		{"backup", "restore", "--no-such-flag"},
		{"backup", "inspect"},
		{"backup", "inspect", "one", "two"},
		{"backup", "list", "unexpected-arg"},
		{"context", "use"},
		{"backup", "restore", "--restart-timeout", "not-a-duration"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, _ := testDeps(t)
			_, _, err := runCLI(t, d, args...)
			wantExitCode(t, err, 2)
		})
	}
}

// Every place that names the passphrase sources has to name them in the order
// PassphraseSource.Read actually checks them: --passphrase-file, then the
// environment variable, then the prompt. `backup create`'s help listed the
// environment variable first, so a reader who had set both was told the wrong one
// would win.
//
// Asserted by position, not by presence: a help string can mention all three and
// still mislead about which one takes effect.
func TestPassphraseSourceOrderIsConsistentEverywhere(t *testing.T) {
	for _, args := range [][]string{
		{"backup", "create", "--help"},
		{"backup", "restore", "--help"},
		{"backup", "inspect", "--help"},
	} {
		d, _ := testDeps(t)
		stdout, _, err := runCLI(t, d, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		file := strings.Index(stdout, "--passphrase-file")
		env := strings.Index(stdout, ui.PassphraseEnv)
		if file < 0 {
			t.Errorf("%v: help never mentions --passphrase-file:\n%s", args, stdout)
			continue
		}
		// Not every command's prose lists all three; the rule is only that where
		// both appear, the file comes first.
		if env >= 0 && env < file {
			t.Errorf("%v: names %s before --passphrase-file, but the file wins:\n%s",
				args, ui.PassphraseEnv, stdout)
		}
	}
}

// The error raised when NO source is available has to read in the same order, or
// it sends an operator to the wrong one first.
func TestNoPassphraseErrorNamesTheSourcesInOrder(t *testing.T) {
	msg := ui.ErrNoPassphrase.Error()
	file := strings.Index(msg, "--passphrase-file")
	env := strings.Index(msg, ui.PassphraseEnv)
	prompt := strings.Index(msg, "interactively")
	if file < 0 || env < 0 || prompt < 0 {
		t.Fatalf("the error should name all three sources: %q", msg)
	}
	if !(file < env && env < prompt) {
		t.Fatalf("the error names the sources out of order: %q", msg)
	}
}
