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

// hasRebuildRoute boots a composition far enough to populate its router, then
// asks whether the package install endpoint was registered. No listener starts.
func hasRebuildRoute(t *testing.T, opts Options) bool {
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

	return serveEvent.Router.HasRoute(http.MethodPost, "/api/admin/packages/install")
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
