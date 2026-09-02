package coreserver

import (
	"sync/atomic"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/quota"
)

// setupCeilingApp builds a storage-bearing collection and a tenant-shaped
// PocketBase app over the same data dir, so RegisterTenant's quota hooks bind
// against records the test can actually save.
func setupCeilingApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(testApp.Cleanup)

	items := core.NewBaseCollection("ceiling_items")
	items.Fields.Add(&core.TextField{Name: "name"})
	items.Fields.Add(&core.NumberField{Name: "size"})
	if err := testApp.Save(items); err != nil {
		t.Fatalf("save collection: %v", err)
	}

	return pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: testApp.DataDir()})
}

func saveSizedItem(t *testing.T, app core.App, name string, size int) error {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("ceiling_items")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("size", size)
	return app.Save(r)
}

func ceilingSources() []quota.Source {
	return []quota.Source{{Slug: "test", Collection: "ceiling_items", SizeField: "size"}}
}

// The headline property of a live resolver: a ceiling raised while the tenant
// is running takes effect on the very next write, with no respawn.
//
// It fails if RegisterTenant ignores the registered resolver — the boot-time
// TenantOptions.QuotaLimits below caps the org at 100 bytes forever, so the
// 400-byte write after the raise is refused and this test reports the stale
// ceiling.
func TestRegisterTenant_EnforcesTheRegisteredResolversCurrentCeiling(t *testing.T) {
	pbApp := setupCeilingApp(t)

	var ceiling atomic.Int64
	ceiling.Store(200)

	if err := RegisterTenant(pbApp, TenantOptions{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 1,
		QuotaSources:  ceilingSources(),
		// The boot-time value the seam must override. Deliberately lower than
		// every ceiling the resolver reports, so a write that succeeds proves
		// the resolver was consulted rather than this.
		QuotaLimits: quota.FixedLimits(100),
		RegisterExtras: func(app *pocketbase.PocketBase) {
			SetStorageLimitsResolver(app, func(core.App) quota.Limits {
				return quota.Limits{PerOrg: ceiling.Load()}
			})
		},
	}); err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if err := saveSizedItem(t, pbApp, "first", 150); err != nil {
		t.Fatalf("a 150-byte write under a 200-byte ceiling must be allowed: %v", err)
	}
	if err := saveSizedItem(t, pbApp, "second", 400); err == nil {
		t.Fatal("a 400-byte write over the 200-byte ceiling must be refused")
	}

	// The push an operator's raise would deliver.
	ceiling.Store(1000)

	if err := saveSizedItem(t, pbApp, "third", 400); err != nil {
		t.Fatalf("the raised ceiling must apply without a respawn: %v", err)
	}
}

// A tenant whose composition registers no resolver must still enforce the
// boot-time ceiling — the standalone and pre-seam case.
func TestRegisterTenant_FallsBackToTheBootTimeCeiling(t *testing.T) {
	pbApp := setupCeilingApp(t)

	if err := RegisterTenant(pbApp, TenantOptions{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 1,
		QuotaSources:  ceilingSources(),
		QuotaLimits:   quota.FixedLimits(100),
	}); err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if err := saveSizedItem(t, pbApp, "over", 400); err == nil {
		t.Fatal("with no registered resolver the boot-time ceiling must still refuse an over-limit write")
	}
}

// A package that computed no resolver must not silently disable the ceiling
// TenantOptions.QuotaLimits would otherwise have enforced.
func TestStorageLimitsResolver_ANilResolverReadsAsAbsent(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if _, ok := StorageLimitsResolver(app); ok {
		t.Fatal("a bare app must report no registered resolver")
	}

	SetStorageLimitsResolver(app, nil)
	if _, ok := StorageLimitsResolver(app); ok {
		t.Fatal("a nil resolver must read as absent, not as an enforcement-disabling registration")
	}

	SetStorageLimitsResolver(app, func(core.App) quota.Limits { return quota.Limits{PerOrg: 7} })
	got, ok := StorageLimitsResolver(app)
	if !ok {
		t.Fatal("a registered resolver must read back")
	}
	if lim := got(app); lim.PerOrg != 7 {
		t.Fatalf("PerOrg = %d, want the registered resolver's 7", lim.PerOrg)
	}
}
