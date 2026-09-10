package coreserver

import (
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/quota"
)

// TestFlagValue_ReadsBothSpellings covers the two forms cobra accepts, since
// standalone mode must read --dir before PocketBase parses its own flags.
func TestFlagValue_ReadsBothSpellings(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"separate value", []string{"serve", "--dir", "/srv/tc"}, "/srv/tc"},
		{"equals form", []string{"serve", "--dir=/srv/tc"}, "/srv/tc"},
		{"absent", []string{"serve"}, ""},
		{"flag present but valueless", []string{"serve", "--dir"}, ""},
		{"not confused by a similar prefix", []string{"serve", "--dirty", "x"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FlagValue(tc.args, "--dir"); got != tc.want {
				t.Errorf("FlagValue(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestStandaloneStateDir pins the mapping from PocketBase's data dir to the
// state root core's own helpers use. They must agree, or releases and builds
// would land beside the binary while the database lived elsewhere.
func TestStandaloneStateDir(t *testing.T) {
	cases := []struct{ dataDir, want string }{
		{"/srv/tc/pb_data", "/srv/tc"},
		{"./tinycld-data/pb_data", "tinycld-data"},
		{"/srv/tc/custom", "/srv/tc"},
	}

	for _, tc := range cases {
		if got := StandaloneStateDir(tc.dataDir); got != tc.want {
			t.Errorf("StandaloneStateDir(%q) = %q, want %q", tc.dataDir, got, tc.want)
		}
	}
}

// TestStandaloneSkipsSelfRebuild pins the distribution's core limitation: a
// single binary cannot rebuild itself (those pipelines shell out to the Go
// toolchain and pnpm, neither of which ships beside a static binary), so the
// install/upgrade API must not be registered. Upgrading means downloading a
// new binary.
func TestStandaloneSkipsSelfRebuild(t *testing.T) {
	standalone := Options{MigrationsFS: fstest.MapFS{}}
	if standalone.supportsSelfRebuild() {
		t.Error("a single-binary build must not expose the self-rebuild API")
	}

	container := Options{MigrationsDir: "./pb_migrations"}
	if !container.supportsSelfRebuild() {
		t.Error("a path-based build must keep the self-rebuild API")
	}
}

// routerFor boots a composition far enough to populate its router, so a test
// can ask which routes it registered. No listener starts.
func routerFor(t *testing.T, opts Options) *router.Router[*core.RequestEvent] {
	t.Helper()
	quota.ResetSourcesForTesting()

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(app, opts)

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	serveEvent := new(core.ServeEvent)
	serveEvent.App = app
	serveEvent.Router = router.NewRouter[*core.RequestEvent](nil)
	if err := app.OnServe().Trigger(serveEvent); err != nil {
		t.Fatalf("OnServe: %v", err)
	}

	return serveEvent.Router
}

// hasRebuildRoute reports whether the package install endpoint was registered.
func hasRebuildRoute(t *testing.T, opts Options) bool {
	t.Helper()
	return routerFor(t, opts).HasRoute(http.MethodPost, "/api/admin/packages/install")
}

// TestStandalone_OmitsRebuildRoutes is the route-level counterpart to
// TestStandaloneSkipsSelfRebuild: it proves the endpoint is genuinely absent
// from a single-binary composition and present in a path-based one.
func TestStandalone_OmitsRebuildRoutes(t *testing.T) {
	base := func() Options {
		return Options{
			HooksDir:      t.TempDir(),
			MigrationsDir: t.TempDir(),
			TypesDir:      t.TempDir(),
			PublicDir:     t.TempDir(),
			HooksPoolSize: 1,
		}
	}

	if !hasRebuildRoute(t, base()) {
		t.Error("a path-based build must keep the package rebuild API")
	}

	standalone := base()
	standalone.MigrationsFS = fstest.MapFS{}
	standalone.HooksFS = fstest.MapFS{}
	standalone.PublicFS = fstest.MapFS{}
	if hasRebuildRoute(t, standalone) {
		t.Error("a single-binary build must not expose the package rebuild API")
	}
}

// TestStandalone_DoesNotRegisterPoolAssetRoutes is the regression guard for a
// bug that let the binary boot and serve its SPA shell while every script 404'd.
//
// The /_expo/static/ and /assets/ routes read the cross-release asset pool the
// container entrypoint maintains under <releasesDir>/_static/. They are
// registered BEFORE the catch-all so the prefixes win — which means in a
// single-binary build they shadow the embedded bundle and answer 404 for assets
// that are present inside the binary. A standalone build has no pool, so the
// catch-all must own those paths.
func TestStandalone_DoesNotRegisterPoolAssetRoutes(t *testing.T) {
	opts := Options{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		TypesDir:      t.TempDir(),
		PublicDir:     t.TempDir(),
		ReleasesDir:   t.TempDir(),
		HooksPoolSize: 1,
		MigrationsFS:  fstest.MapFS{},
		HooksFS:       fstest.MapFS{},
		PublicFS:      fstest.MapFS{},
	}

	r := routerFor(t, opts)
	for _, path := range []string{"/_expo/static/{path...}", "/assets/{path...}"} {
		if r.HasRoute(http.MethodGet, path) {
			t.Errorf("standalone must not register %q — it shadows the embedded bundle", path)
		}
	}

	// The catch-all must still be there to serve them. It is registered with
	// Any, whose method is "" — HasRoute(MethodGet, …) would not match it.
	if !r.HasRoute("", "/{path...}") {
		t.Error("expected the catch-all to serve embedded assets")
	}
}

// TestShouldInjectDataDir pins when standalone mode may append --dir to os.Args.
//
// --dir is a persistent flag, but appending it to a bare `--help` or `--version`
// invocation makes cobra read the path as a stray positional argument and fail
// with `unknown command "./tinycld-data/pb_data"`. Only inject when a command
// will actually use it and the user has not set it.
func TestShouldInjectDataDir(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"bare serve", []string{"serve"}, true},
		{"serve with an address", []string{"serve", "--http", "127.0.0.1:9000"}, true},
		{"serve with domains", []string{"serve", "example.com"}, true},
		{"user set --dir", []string{"serve", "--dir", "/srv/x"}, false},
		{"user set --dir= form", []string{"serve", "--dir=/srv/x"}, false},
		{"help", []string{"--help"}, false},
		{"version", []string{"--version"}, false},
		{"short help", []string{"-h"}, false},
		{"no args", nil, false},
		// Every real command operates on the database, so each needs the
		// default data dir — not just serve.
		{"superuser", []string{"superuser", "create", "a@b.c", "pw"}, true},
		{"create-owner", []string{"create-owner"}, true},
		{"export-types", []string{"export-types"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldInjectDataDir(tc.args); got != tc.want {
				t.Errorf("ShouldInjectDataDir(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
