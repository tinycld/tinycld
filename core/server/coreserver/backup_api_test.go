package coreserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

const testPassphrase = "correct horse battery"

// setupBackupCollections boots a PocketBase app on an EMPTY data dir and adds
// the tinycld collections the backup engine touches. It is a copy of
// backup/testapp_test.go's newTestApp: a _test file is not importable from
// another package, and the engine's manifest counts every non-system
// collection and every stored file, so the PB demo fixture would make an exact
// count impossible to assert.
//
// The registry's feature row uses a fictional slug: core must not name a
// feature package, in its tests no less than its code.
func setupBackupCollections(t *testing.T) *tests.TestApp {
	t.Helper()

	// LedgerPath(app) is filepath.Dir(app.DataDir()), and the backup engine keeps
	// its scratch directory there — which RegisterBackupBoot clears at boot. So
	// the app's data dir MUST have a parent this test owns.
	//
	// Passing DataDir is not enough: NewTestAppWithConfig CLONES it with
	// TempDirClone, which is os.MkdirTemp("", "pb_test_*") — the system temp root.
	// LedgerPath would then be $TMPDIR itself, shared by every test process on the
	// machine, and this package's boot hook would delete $TMPDIR/backup-tmp out
	// from under a backup running in a parallel `go test` process. That really
	// happened: a concurrent run failed with
	// "open .../T/backup-tmp/<id>.db: no such file or directory".
	//
	// MkdirTemp honours TMPDIR, so pointing TMPDIR at this test's own directory
	// puts the clone — and therefore LedgerPath — inside it.
	sharedTemp := os.TempDir() // read BEFORE the override, or it reports the override
	t.Setenv("TMPDIR", t.TempDir())

	dir := filepath.Join(t.TempDir(), "pb_data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: dir, EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	// The guard for the above: if a future change makes the data dir land in a
	// shared root again, this fails here rather than by corrupting another
	// process's backup.
	if parent := filepath.Clean(filepath.Dir(app.DataDir())); parent == filepath.Clean(sharedTemp) {
		t.Fatalf("the test app's data dir is directly in the shared temp root (%s), so "+
			"LedgerPath is shared by every test process and the boot hook would wipe "+
			"another run's backup-tmp", parent)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	relaxUsernameMinLength(users)
	if users.Fields.GetByName("role") == nil {
		users.Fields.Add(&core.SelectField{Name: "role", Values: []string{"owner", "admin", "member", "guest"}, MaxSelect: 1})
		users.Fields.Add(&core.BoolField{Name: "disabled"})
	}
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	mustCreate := func(c *core.Collection) {
		if err := app.Save(c); err != nil {
			t.Fatalf("create %s: %v", c.Name, err)
		}
	}
	backups := core.NewBaseCollection("backups")
	backups.Fields.Add(
		&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"manual", "scheduled", "pre_restore", "restore"}},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"running", "waiting_for_source", "succeeded", "failed", "interrupted"}},
		&core.RelationField{Name: "initiated_by", CollectionId: users.Id, MaxSelect: 1},
		&core.DateField{Name: "started", Required: true},
		&core.DateField{Name: "finished"},
		&core.NumberField{Name: "bytes"},
		&core.TextField{Name: "sha256", Max: 64},
		&core.JSONField{Name: "manifest", MaxSize: 200000},
		&core.TextField{Name: "target_host", Max: 253},
		&core.TextField{Name: "error", Max: 2000},
		&core.JSONField{Name: "metadata", MaxSize: 20000},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	mustCreate(backups)

	reg := core.NewBaseCollection("pkg_registry")
	reg.Fields.Add(
		&core.TextField{Name: "name"}, &core.TextField{Name: "slug", Required: true},
		&core.TextField{Name: "npm_package"}, &core.TextField{Name: "version"},
		&core.SelectField{Name: "status", MaxSelect: 1, Values: []string{"bundled", "available", "installed", "disabled"}},
	)
	mustCreate(reg)
	addRegistry := func(slug, version, spec, status string) {
		r := core.NewRecord(reg)
		r.Set("name", slug)
		r.Set("slug", slug)
		r.Set("version", version)
		r.Set("npm_package", spec)
		r.Set("status", status)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	addRegistry("core", "1.2.3", "tinycld@1.2.3", "bundled")
	addRegistry("widgets", "1.0.0", "@example/widgets@1.0.0", "installed")

	notifs := core.NewBaseCollection("notifications")
	notifs.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "type"}, &core.TextField{Name: "package"}, &core.TextField{Name: "title"},
		&core.TextField{Name: "body"}, &core.TextField{Name: "url"}, &core.JSONField{Name: "metadata", MaxSize: 20000},
		&core.BoolField{Name: "read"}, &core.BoolField{Name: "dismissed"},
	)
	mustCreate(notifs)

	audit := core.NewBaseCollection("audit_logs")
	audit.Fields.Add(
		&core.TextField{Name: "action"}, &core.TextField{Name: "resource_type"}, &core.TextField{Name: "resource_id"},
		&core.TextField{Name: "resource_label"}, &core.RelationField{Name: "actor", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "ip_address"}, &core.TextField{Name: "user_agent"}, &core.JSONField{Name: "metadata", MaxSize: 20000},
	)
	mustCreate(audit)

	// The install log the restore rebuild writes its row into. Only the subset
	// createInstallLog touches; pkg_slug is REQUIRED, which is the point.
	installLog := core.NewBaseCollection("pkg_install_log")
	installLog.Fields.Add(
		&core.SelectField{Name: "action", Required: true, MaxSelect: 1,
			Values: []string{"install", "uninstall", "enable", "disable"}},
		&core.TextField{Name: "pkg_slug", Required: true},
		&core.TextField{Name: "npm_package"},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1,
			Values: []string{"pending", "running", "success", "failed", "rolled_back"}},
		&core.TextField{Name: "error", Max: 5000},
		&core.TextField{Name: "job_id"},
		&core.DateField{Name: "started_at"},
		&core.DateField{Name: "completed_at"},
	)
	mustCreate(installLog)

	// A file in local storage so the backup walk has something to copy.
	storageDir := filepath.Join(app.DataDir(), "storage", "col1", "rec1")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello file"), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

// backupTestApp is the app plus the routes under test, with the restore's
// process-ending seams stubbed: a real rebuilder would try to build a binary
// and a real restart would kill the test process.
func backupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupBackupCollections(t)
	restoreSeams(t)
	RegisterBackupEndpoints(app)
	return app
}

// restoreSeams stubs the two process-ending seams a restore reaches at phase 6.
// A real rebuilder would try to build a binary and a real restart would kill the
// test process, so every restore test replaces both.
//
// The no-op rebuilder releases the job it is handed: the restore hands its claim
// over rather than releasing it, so a rebuilder that dropped it would leave the
// interlock held and every later test would see ErrBusy.
//
// Cleanup goes through backup.ResetForTesting rather than unsetting one seam,
// because the package holds more process-wide state than the rebuilder — a left
// restoring flag would put every later request behind the maintenance 503.
func restoreSeams(t *testing.T) {
	t.Helper()
	backup.RegisterRebuilder(func(_ context.Context, job *installjob.Job, _ format.Lockfile) error {
		installjob.Release(job)
		return nil
	})
	backup.SetRestart(func() {})
	t.Cleanup(backup.ResetForTesting)
}

func makeBackupUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("password12345")
	u.Set("role", role)
	u.Set("username", strings.Split(email, "@")[0])
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func authFor(t *testing.T, app core.App, email, role string) string {
	t.Helper()
	token, err := tokenForUser(app, makeBackupUser(t, app, email, role))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// waitFor polls cond for up to 5 s. A backup started with POST /api/org-backups
// runs on its own goroutine, so the assertion has to wait for it rather than
// read the ledger the instant the 202 lands.
func waitFor(t testing.TB, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was never met within 5s")
}

// closeBuffer lets a test hand backup.Run a sink it can read afterwards.
type closeBuffer struct{ buf *bytes.Buffer }

func (c closeBuffer) Write(p []byte) (int, error) { return c.buf.Write(p) }
func (c closeBuffer) Close() error                { return nil }

func streamBackupForTest(t *testing.T, app core.App, out *bytes.Buffer) {
	t.Helper()
	rcpt, err := age.NewScryptRecipient(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Run(app, backup.Request{
		Kind: backup.KindManual, Recipient: rcpt, Sink: closeBuffer{buf: out},
	}); err != nil {
		t.Fatal(err)
	}
}

func multipartArchive(t *testing.T, data []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	// Fields go before the file part: the handler reads them in stream order so
	// the archive never has to be buffered.
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.CreateFormFile("archive", "backup.age")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), w.FormDataContentType()
}

func TestBackupStreamAsAdmin(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")
	scenario := &tests.ApiScenario{
		Name:   "stream a backup to the client",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"stream":true,"passphrase":"` + testPassphrase + `"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus: http.StatusOK,
		// The age header is the first thing an archive writes. Asserting it also
		// satisfies ApiScenario, which demands an empty body when nothing is
		// expected — and a streamed archive is the opposite of an empty body.
		ExpectedContent:       []string{"age-encryption.org/v1"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := age.NewScryptIdentity(testPassphrase)
			if err != nil {
				t.Fatal(err)
			}
			_, rep, err := format.Inspect(bytes.NewReader(body), identity)
			if err != nil || !rep.OK {
				t.Fatalf("streamed archive unreadable: %v (report ok=%v)", err, rep.OK)
			}
			if ct := res.Header.Get("Content-Type"); ct != "application/octet-stream" {
				t.Fatalf("content-type %q, want application/octet-stream", ct)
			}
		},
	}
	scenario.Test(t)
}

func TestBackupToTargetReturns202AndRecordsHostOnly(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")

	received := make(chan int, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case received <- len(b):
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	scenario := &tests.ApiScenario{
		Name:   "PUT a backup at a target URL",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body: strings.NewReader(`{"target":"` + srv.URL + `/x.age?sig=secret","passphrase":"` +
			testPassphrase + `"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusAccepted,
		ExpectedContent:       []string{`"id":`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, _ *http.Response) {
			waitFor(t, func() bool { return len(received) > 0 })
			// The engine's terminal defer writes the row, THEN announces (a
			// notification and an audit row), THEN posts any callback. So
			// "status = succeeded" is NOT the end of the goroutine: waiting only
			// for it let the test return while announce was still running, the
			// fixture's app was torn down under it, and audit.Log dereferenced a
			// closed database — a data race against app.Cleanup and a SIGSEGV.
			// The audit row is written last of the DB work, so it is the signal
			// that the goroutine is done with the app.
			waitFor(t, func() bool {
				logs, _ := app.FindRecordsByFilter("audit_logs", "action = 'backup.created'", "", 0, 0)
				return len(logs) == 1
			})
			rows, err := app.FindRecordsByFilter("backups", "status = 'succeeded'", "", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("succeeded backup rows = %d, want 1", len(rows))
			}
			// Only the hostname is kept: the target URL carries its own
			// credentials in the query string.
			if host := rows[0].GetString("target_host"); host != "127.0.0.1" {
				t.Fatalf("target_host = %q, want the hostname only", host)
			}
		},
	}
	scenario.Test(t)
}

func TestBackupRejectsShortPassphrase(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "a short passphrase is refused before any work",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"stream":true,"passphrase":"short"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"12 characters"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestBackupRejectsNonHTTPTarget(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "a file:// target is refused",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"target":"file:///tmp/x.age","passphrase":"` + testPassphrase + `"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"http"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestBackupForbiddenForMember(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "member@example.com", "member")
	(&tests.ApiScenario{
		Name:   "a member cannot start a backup",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"stream":true,"passphrase":"` + testPassphrase + `"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"status":403`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestBackupGetReturnsTheLedgerRow(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")

	col, err := app.FindCollectionByNameOrId("backups")
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("kind", "manual")
	row.Set("status", "succeeded")
	row.Set("started", types.NowDateTime())
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	(&tests.ApiScenario{
		Name:                  "an admin reads one ledger row",
		Method:                http.MethodGet,
		URL:                   "/api/org-backups/" + row.Id,
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"status":"succeeded"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRestoreRefusesAnAdminAndAcceptsAnOwner(t *testing.T) {
	app := backupTestApp(t)
	adminTok := authFor(t, app, "admin@example.com", "admin")
	ownerTok := authFor(t, app, "owner@example.com", "owner")

	var archive bytes.Buffer
	streamBackupForTest(t, app, &archive)

	body, contentType := multipartArchive(t, archive.Bytes(), map[string]string{"passphrase": testPassphrase})
	(&tests.ApiScenario{
		Name:   "an admin cannot restore",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": adminTok,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"status":403`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)

	body, contentType = multipartArchive(t, archive.Bytes(), map[string]string{"passphrase": testPassphrase})
	(&tests.ApiScenario{
		Name:   "an owner restores from an upload",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": ownerTok,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusAccepted,
		ExpectedContent:       []string{`"jobId":`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRestoreRejectsShortPassphrase(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")

	body, contentType := multipartArchive(t, []byte("not an archive"), map[string]string{"passphrase": "short"})
	(&tests.ApiScenario{
		Name:   "an upload with a short passphrase is refused",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"12 characters"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

// A multipart restore runs synchronously, so a package-set mismatch surfaces as
// the HTTP status rather than only on the ledger row.
func TestRestoreMismatchReturns409WithDiff(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")
	// No rebuilder: a deployment that cannot rebuild itself refuses a restore of
	// a package set it does not carry.
	backup.RegisterRebuilder(nil)

	var archive bytes.Buffer
	streamBackupForTest(t, app, &archive)

	regs, err := app.FindRecordsByFilter("pkg_registry", "slug = 'widgets'", "", 0, 0)
	if err != nil || len(regs) != 1 {
		t.Fatalf("registry rows for the fictional package: %v, %v", regs, err)
	}
	if err := app.Delete(regs[0]); err != nil {
		t.Fatal(err)
	}

	body, contentType := multipartArchive(t, archive.Bytes(), map[string]string{"passphrase": testPassphrase})
	(&tests.ApiScenario{
		Name:   "a package set this binary cannot run is refused with the diff",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusConflict,
		ExpectedContent:       []string{`"missing":["widgets"]`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

// force skips the package-set check and the rebuild, so its VALUE has to decide.
// A handler that read the field's mere presence would let force=false through as
// force=true — and this test's archive names a package the registry no longer
// has, so a wrongly-forced restore would be accepted instead of refused.
func TestRestoreForceFalseIsNotForce(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")
	backup.RegisterRebuilder(nil)

	var archive bytes.Buffer
	streamBackupForTest(t, app, &archive)

	regs, err := app.FindRecordsByFilter("pkg_registry", "slug = 'widgets'", "", 0, 0)
	if err != nil || len(regs) != 1 {
		t.Fatalf("registry rows for the fictional package: %v, %v", regs, err)
	}
	if err := app.Delete(regs[0]); err != nil {
		t.Fatal(err)
	}

	body, contentType := multipartArchive(t, archive.Bytes(), map[string]string{
		"passphrase": testPassphrase,
		"force":      "false",
	})
	(&tests.ApiScenario{
		Name:   "force=false still refuses a package set this binary cannot run",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusConflict,
		ExpectedContent:       []string{`"missing":["widgets"]`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

// The archive must be the LAST part so it can be streamed into the restore
// instead of buffered. A client that sends it first gets told exactly that,
// rather than the "too short" message the passphrase check would otherwise
// produce for an empty passphrase it never saw.
func TestRestoreRejectsAnArchiveBeforeItsPassphrase(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("archive", "backup.age")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("archive bytes")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("passphrase", testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	(&tests.ApiScenario{
		Name:   "the archive part before the passphrase field is refused by order",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body.Bytes()),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  w.FormDataContentType(),
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"must come before the archive"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

// A passphrase longer than the field cap is refused, not truncated. Truncating
// would silently change the passphrase and then fail to decrypt, reporting a
// problem with the archive instead of with the field.
func TestRestoreRejectsAnOverlongPassphrase(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")

	body, contentType := multipartArchive(t, []byte("archive bytes"), map[string]string{
		"passphrase": strings.Repeat("x", maxPassphraseField+1),
	})
	(&tests.ApiScenario{
		Name:   "a passphrase over the field cap is refused",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   bytes.NewReader(body),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  contentType,
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"too long"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRestoreSwapSourceRequiresAWaitingRestore(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "owner@example.com", "owner")
	(&tests.ApiScenario{
		Name:   "swapping a source nothing is waiting for is a 404",
		Method: http.MethodPatch,
		URL:    "/api/org-backups/restore/nosuchjob",
		Body:   strings.NewReader(`{"source":"https://example.test/a.age"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusNotFound,
		ExpectedContent:       []string{`"status":404`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestVerifyEndpoint(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:                  "verify reports the live integrity check",
		Method:                http.MethodGet,
		URL:                   "/api/org-backups/verify",
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"integrityOk":true`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

// Both post-bootstrap steps run, on the layout a boot after a staged restore
// really finds: an armed marker plus a fully staged pending directory. The
// hook's own ApplyPendingRestore swaps that in and writes the swapped marker,
// then FinalizeRestore records it and MarkInterrupted closes what a dead process
// left behind.
//
// This does NOT prove the ordering. Tasks 8 and 9 require FinalizeRestore to run
// before MarkInterrupted, and the code does that, but the two implementations
// cannot currently collide: FinalizeRestore inserts its row already set to
// "succeeded" and with started = now, while MarkInterrupted only rewrites rows
// that are "running" AND started before bootedAt. Reversing the two calls keeps
// this test green — verified by doing it.
//
// The order is therefore DEFENSIVE, and the reason it is worth keeping is that
// either half could change: a finalize that inserted "running" first, or a
// MarkInterrupted that stopped filtering on bootedAt, would immediately close
// the row the finalize had just written and report a restore that worked as
// interrupted. What this test pins is that both steps ran and neither undid the
// other — not the sequence.
func TestBootHookFinalizesAndMarksInterrupted(t *testing.T) {
	app := setupBackupCollections(t)
	restoreSeams(t)
	makeBackupUser(t, app, "owner@example.com", "owner")

	// A row left running by the process that armed the restore, started before
	// this boot. It is the row MarkInterrupted exists for.
	col, err := app.FindCollectionByNameOrId("backups")
	if err != nil {
		t.Fatal(err)
	}
	abandoned := core.NewRecord(col)
	abandoned.Set("kind", "manual")
	abandoned.Set("status", "running")
	abandoned.Set("started", types.NowDateTime().Add(-time.Hour))
	if err := app.Save(abandoned); err != nil {
		t.Fatal(err)
	}

	dataDir := app.DataDir()
	restoreDir := filepath.Join(filepath.Dir(dataDir), "restore")
	pending := filepath.Join(restoreDir, "pending", "r1")
	if err := os.MkdirAll(pending, 0o700); err != nil {
		t.Fatal(err)
	}
	// The staged database is a copy of the live one, so the app can still read
	// it after the swap renames pb_data. What is under test is the two
	// post-bootstrap steps, not what the archive contained.
	live, err := os.ReadFile(filepath.Join(dataDir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "data.db"), live, 0o644); err != nil {
		t.Fatal(err)
	}
	// The sentinel is the boot swap's only sound evidence that staging finished.
	if err := os.WriteFile(filepath.Join(pending, ".staged"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	pre := filepath.Join(restoreDir, "pre", "r1.age")
	if err := os.MkdirAll(filepath.Dir(pre), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre, []byte("pre"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(map[string]any{
		"id":       "r1",
		"pending":  pending,
		"pre":      pre,
		"manifest": format.Manifest{Core: "1.2.3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restoreDir, "armed"), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	RegisterBackupBoot(app)
	if err := app.OnBootstrap().Trigger(&core.BootstrapEvent{App: app}, func(*core.BootstrapEvent) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// FinalizeRestore ran: the restore is on record as succeeded, with the
	// manifest Verify compares against.
	rows, err := app.FindRecordsByFilter("backups", "kind = 'restore'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("restore rows = %d, want the one the finalize inserted", len(rows))
	}
	if got := rows[0].GetString("status"); got != "succeeded" {
		t.Fatalf("the finalized restore row is %q, want succeeded", got)
	}
	var manifest format.Manifest
	if err := rows[0].UnmarshalJSONField("manifest", &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Core != "1.2.3" {
		t.Fatalf("the finalized row's manifest is %+v; Verify has nothing to "+
			"compare without it", manifest)
	}

	// MarkInterrupted ran: the dead process's row is closed, and the finalize did
	// not leave it running.
	reread, err := app.FindRecordById("backups", abandoned.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := reread.GetString("status"); got != "interrupted" {
		t.Fatalf("the abandoned run is %q, want interrupted", got)
	}
}

// A row a previous process left running is still closed by the same hook.
func TestBootHookMarksAnAbandonedRunInterrupted(t *testing.T) {
	app := setupBackupCollections(t)
	restoreSeams(t)

	col, err := app.FindCollectionByNameOrId("backups")
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("kind", "manual")
	row.Set("status", "running")
	row.Set("started", types.NowDateTime().Add(-time.Hour))
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	RegisterBackupBoot(app)
	if err := app.OnBootstrap().Trigger(&core.BootstrapEvent{App: app}, func(*core.BootstrapEvent) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	reread, err := app.FindRecordById("backups", row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := reread.GetString("status"); got != "interrupted" {
		t.Fatalf("an abandoned run is %q, want interrupted", got)
	}
}

// The maintenance middleware must sit ahead of record CRUD, so a write cannot
// land in a pb_data the next process is about to replace.
func TestMaintenanceMiddlewareBindsBeforeAuthLoading(t *testing.T) {
	app := setupBackupCollections(t)
	restoreSeams(t)
	RegisterBackupBoot(app)

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	e := &core.ServeEvent{App: app, Router: pbRouter}
	if err := app.OnServe().Trigger(e, func(*core.ServeEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}

	var maintenance, loadAuth *hook.Handler[*core.RequestEvent]
	for _, mw := range e.Router.Middlewares {
		switch mw.Id {
		case maintenanceMiddlewareID:
			maintenance = mw
		case apis.DefaultLoadAuthTokenMiddlewareId:
			loadAuth = mw
		}
	}
	if maintenance == nil {
		t.Fatalf("no %q middleware bound", maintenanceMiddlewareID)
	}
	if loadAuth == nil {
		t.Fatal("PocketBase's load-auth-token middleware is not on the router")
	}
	if maintenance.Priority >= loadAuth.Priority {
		t.Fatalf("the maintenance middleware's priority is %d; it must run before "+
			"the load-auth-token middleware at %d, or a record write reaches a "+
			"pb_data the next process replaces", maintenance.Priority, loadAuth.Priority)
	}
}

// A restore's job carries no slug and no npm spec, and pkg_install_log.pkg_slug
// is required — so createInstallLog's save failed on every restore rebuild and
// the operator's install history had no trace of the rebuild that replaced their
// whole deployment. The row must persist, and it must name something.
func TestRestoreRebuildWritesAnInstallLogRow(t *testing.T) {
	app := setupBackupCollections(t)

	// The job exactly as beginRestore builds it: action only.
	job := installjob.New("restore", "", "")
	job.ID = "restore-job-1"
	nameRestoreJob(job)

	row := createInstallLog(app, job, "install")
	if row == nil {
		t.Fatal("createInstallLog returned nil — the row did not save, so a restore " +
			"rebuild leaves no trace in the install history")
	}
	if got := row.GetString("pkg_slug"); got != baseRegistrySlug {
		t.Fatalf("pkg_slug = %q, want %q — a restore rebuild replaces the whole "+
			"package set, not one member", got, baseRegistrySlug)
	}
	if got := row.GetString("job_id"); got != job.ID {
		t.Fatalf("job_id = %q, want the restore's job id %q", got, job.ID)
	}
	rows, err := app.FindRecordsByFilter("pkg_install_log", "job_id = 'restore-job-1'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("persisted install-log rows = %d, want 1", len(rows))
	}
}

// A target-URL backup runs on a goroutine that OUTLIVES its HTTP request, so it
// must not hold the request's context: net/http cancels that context the instant
// the handler returns, which aborted the PUT after its first bytes and finalized
// the row as "failed: io: read/write on closed pipe".
//
// The ApiScenario tests above cannot catch this. They invoke the handler
// directly and never cancel the request context, so the PUT survives there no
// matter what context it was given — which is exactly why the bug shipped with
// TestBackupToTargetReturns202AndRecordsHostOnly already asserting "succeeded".
// This test serves the route over a real net/http listener, so the cancellation
// is the real one rather than a simulated one.
func TestBackupToTargetSurvivesTheRequestEnding(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")

	var mu sync.Mutex
	var got []byte
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = b
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sink.Close)

	// The app's own router, served by net/http — so the request context is
	// cancelled by the transport when the handler returns, as in production.
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)

	body := `{"target":"` + sink.URL + `/x.age","passphrase":"` + testPassphrase + `"}`
	req, err := http.NewRequest(http.MethodPost, api.URL+"/api/org-backups", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", res.StatusCode)
	}

	// The audit row is the last DB write the run's terminal defer makes, so it
	// is the signal that the goroutine is finished with the app.
	waitFor(t, func() bool {
		logs, _ := app.FindRecordsByFilter("audit_logs", "action = 'backup.created'", "", 0, 0)
		return len(logs) == 1
	})

	rows, err := app.FindRecordsByFilter("backups", "", "-started", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("backup rows = %d, want 1", len(rows))
	}
	if status := rows[0].GetString("status"); status != "succeeded" {
		t.Fatalf("status = %q (error %q), want succeeded",
			status, rows[0].GetString("error"))
	}

	mu.Lock()
	archive := got
	mu.Unlock()
	identity, err := age.NewScryptIdentity(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	_, rep, err := format.Inspect(bytes.NewReader(archive), identity)
	if err != nil || !rep.OK {
		t.Fatalf("archive at the target unreadable: %v (report ok=%v)", err, rep.OK)
	}
}

// A probe boots a full server on the real data directory and is then killed. It
// must leave every piece of restore state alone: a kill between the swap and the
// finalize makes the REAL boot see a swapped marker with no finished restore
// behind it and roll the operator's restore back.
func TestBootProbeLeavesRestoreStateForTheRealBoot(t *testing.T) {
	t.Setenv("TINYCLD_BOOT_PROBE", "1")
	app := setupBackupCollections(t)
	restoreSeams(t)
	makeBackupUser(t, app, "owner@example.com", "owner")

	col, err := app.FindCollectionByNameOrId("backups")
	if err != nil {
		t.Fatal(err)
	}
	abandoned := core.NewRecord(col)
	abandoned.Set("kind", "manual")
	abandoned.Set("status", "running")
	abandoned.Set("started", types.NowDateTime().Add(-time.Hour))
	if err := app.Save(abandoned); err != nil {
		t.Fatal(err)
	}

	dataDir := app.DataDir()
	restoreDir := filepath.Join(filepath.Dir(dataDir), "restore")
	pending := filepath.Join(restoreDir, "pending", "r1")
	if err := os.MkdirAll(pending, 0o700); err != nil {
		t.Fatal(err)
	}
	live, err := os.ReadFile(filepath.Join(dataDir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "data.db"), live, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, ".staged"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(map[string]any{
		"id": "r1", "pending": pending, "manifest": format.Manifest{Core: "1.2.3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	armed := filepath.Join(restoreDir, "armed")
	if err := os.WriteFile(armed, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	// A scratch file the real boot would wipe. The probe must leave it: the run
	// that owns it may still be the live process's.
	scratch := filepath.Join(filepath.Dir(dataDir), "backup-tmp")
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		t.Fatal(err)
	}

	RegisterBackupBoot(app)
	if err := app.OnBootstrap().Trigger(&core.BootstrapEvent{App: app}, func(*core.BootstrapEvent) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(armed); err != nil {
		t.Fatalf("the probe consumed the armed marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "swapped")); !os.IsNotExist(err) {
		t.Fatal("the probe performed the swap")
	}
	if _, err := os.Stat(filepath.Join(pending, "data.db")); err != nil {
		t.Fatalf("the probe moved the staged data in: %v", err)
	}
	if _, err := os.Stat(scratch); err != nil {
		t.Fatalf("the probe wiped the backup scratch directory: %v", err)
	}
	reread, err := app.FindRecordById("backups", abandoned.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := reread.GetString("status"); got != "running" {
		t.Fatalf("the probe closed a running row as %q", got)
	}
	rows, err := app.FindRecordsByFilter("backups", "kind = 'restore'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("the probe finalized a restore: %d rows", len(rows))
	}
}
