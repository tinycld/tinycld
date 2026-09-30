package coreserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/pbs"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
	"tinycld.org/core/syscfg"
)

// fakeRepo is a repository kind the test registers itself, so these tests
// never touch the network. It is registered under "fake" — never "pbs", which
// pbs.Register() owns.
type fakeRepo struct{ snaps []repo.SnapshotInfo }

func (fakeRepo) Kind() string { return "fake" }

func (fakeRepo) Put(context.Context, *snapshot.Snapshot, func(int64)) (repo.PutResult, error) {
	return repo.PutResult{Ref: "fake/1", Bytes: 10, UploadedBytes: 1}, nil
}

func (fakeRepo) Manifest(context.Context, repo.Ref) (format.Manifest, error) {
	return format.Manifest{}, errors.New("not used")
}

func (fakeRepo) Fetch(context.Context, repo.Ref, string) error { return errors.New("not used") }

func (f fakeRepo) List(context.Context) ([]repo.SnapshotInfo, error) { return f.snaps, nil }

// registerFakeRepository resets the repo registry and registers "fake" on it.
// The registry is package-global, so cleanup resets it again and re-registers
// "pbs" — pbs.Register is idempotent (it checks the registry first), so this
// leaves the registry exactly as any other coreserver test expects to find it.
func registerFakeRepository(t *testing.T, snaps []repo.SnapshotInfo) {
	t.Helper()
	repo.ResetForTesting()
	repo.Register("fake", func(json.RawMessage) (repo.Repository, error) {
		return fakeRepo{snaps: snaps}, nil
	})
	t.Cleanup(func() {
		repo.ResetForTesting()
		pbs.Register()
	})
}

func TestRepositoryBackupSucceedsAsAdmin(t *testing.T) {
	app := backupTestApp(t)
	registerFakeRepository(t, nil)
	syscfg.SetResolver(mapProvider{values: map[string]string{repoKeyKind: "fake"}}.Get)
	t.Cleanup(syscfg.ResetForTesting)

	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "an admin backs up to the configured repository",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"repository":true}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusAccepted,
		ExpectedContent:       []string{`"id":`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, _ *http.Response) {
			waitFor(t, func() bool {
				rows, err := app.FindRecordsByFilter("backups", "repository = 'fake'", "", 1, 0)
				return err == nil && len(rows) == 1 && rows[0].GetString("status") == "succeeded"
			})
			rows, err := app.FindRecordsByFilter("backups", "repository = 'fake'", "", 1, 0)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows = %v, err = %v", rows, err)
			}
			if rows[0].GetString("ref") != "fake/1" {
				t.Fatalf("ref = %q, want fake/1", rows[0].GetString("ref"))
			}
		},
	}).Test(t)
}

func TestRepositoryBackupWithNoKindConfiguredIs400(t *testing.T) {
	app := backupTestApp(t)
	syscfg.SetResolver(mapProvider{values: map[string]string{}}.Get)
	t.Cleanup(syscfg.ResetForTesting)

	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "no repository configured",
		Method: http.MethodPost,
		URL:    "/api/org-backups",
		Body:   strings.NewReader(`{"repository":true}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"No backup repository is configured."},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestSnapshotsListAsAdminForbiddenForMember(t *testing.T) {
	app := backupTestApp(t)
	registerFakeRepository(t, []repo.SnapshotInfo{{Ref: "fake/1", Bytes: 10}})
	syscfg.SetResolver(mapProvider{values: map[string]string{repoKeyKind: "fake"}}.Get)
	t.Cleanup(syscfg.ResetForTesting)

	adminTok := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:                  "an admin lists snapshots",
		Method:                http.MethodGet,
		URL:                   "/api/org-backups/snapshots",
		Headers:               map[string]string{"Authorization": adminTok},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"ref":"fake/1"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)

	memberTok := authFor(t, app, "member@example.com", "member")
	(&tests.ApiScenario{
		Name:                  "a member cannot list snapshots",
		Method:                http.MethodGet,
		URL:                   "/api/org-backups/snapshots",
		Headers:               map[string]string{"Authorization": memberTok},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"status":403`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRepositoryTestRefusesLoopbackWithoutOverride(t *testing.T) {
	app := backupTestApp(t)
	allowLoopbackBackupTargets(t, false)

	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "testing a loopback PBS server is refused",
		Method: http.MethodPost,
		URL:    "/api/org-backups/repository/test",
		Body: strings.NewReader(`{"kind":"pbs","config":{"server":"127.0.0.1","datastore":"main",` +
			`"auth_id":"root@pam!tok","secret":"top-secret-value"}}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusUnprocessableEntity,
		ExpectedContent:       []string{"loopback"},
		NotExpectedContent:    []string{"top-secret-value"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestGenerateKeyReturnsParsablePBSKey(t *testing.T) {
	app := backupTestApp(t)
	token := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "generate a PBS key",
		Method: http.MethodPost,
		URL:    "/api/org-backups/repository/generate-key",
		Body:   strings.NewReader(`{"kind":"pbs"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"key":`, `\"data\"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRestoreFromSnapshotRefusesAnAdminAllowsOwner(t *testing.T) {
	app := backupTestApp(t)
	registerFakeRepository(t, []repo.SnapshotInfo{{Ref: "fake/1", Bytes: 10}})
	syscfg.SetResolver(mapProvider{values: map[string]string{repoKeyKind: "fake"}}.Get)
	t.Cleanup(syscfg.ResetForTesting)

	adminTok := authFor(t, app, "admin@example.com", "admin")
	(&tests.ApiScenario{
		Name:   "an admin cannot restore from a snapshot",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body:   strings.NewReader(`{"snapshot":"fake/1"}`),
		Headers: map[string]string{
			"Authorization": adminTok,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"status":403`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func TestRestoreRefusesBothSourceAndSnapshot(t *testing.T) {
	app := backupTestApp(t)
	registerFakeRepository(t, []repo.SnapshotInfo{{Ref: "fake/1", Bytes: 10}})
	syscfg.SetResolver(mapProvider{values: map[string]string{repoKeyKind: "fake"}}.Get)
	t.Cleanup(syscfg.ResetForTesting)

	token := authFor(t, app, "owner@example.com", "owner")
	(&tests.ApiScenario{
		Name:   "naming both a source and a snapshot is refused",
		Method: http.MethodPost,
		URL:    "/api/org-backups/restore",
		Body: strings.NewReader(`{"source":"https://example.com/x.age","snapshot":"fake/1",` +
			`"passphrase":"` + testPassphrase + `"}`),
		Headers: map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json",
		},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"Only one of source or snapshot"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}
