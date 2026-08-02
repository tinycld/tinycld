package coreserver

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/quota"
)

// This test is the tripwire for the tenant composition gap
// (multi-org/docs/FINDING-tenant-composition-gap.md): serve-org once
// hand-rolled a subset of Register (jsvm + quota only), so every guard core
// added after the copy silently never reached a tenant — including the users
// field guard, whose absence let any member PATCH their own role to owner.
//
// It composes one app per entry point and compares, hook by hook, how many
// handlers each bound. The host is allowed to differ from the tenant ONLY
// where hostOnlyHookDiff records the difference with a reason. Adding a
// registration to Register without deciding whether tenants get it makes
// this test fail with the unexplained hook names.

// hostOnlyHookDiff is the recorded set of host-only divergences: hook name →
// how many MORE handlers the host composition binds there than the tenant.
// Every count must be attributable to a registration in Register's host-only
// tail (each of which carries its reason in server.go).
var hostOnlyHookDiff = map[string]int{
	// OnServe: one bind each from RegisterPackageInstallEndpoints,
	// RegisterVapidAdminEndpoints, RegisterAppUpdateEndpoints,
	// RegisterSetupBootstrap, RegisterDemoStart, RegisterDemoLead, and
	// registerStaticServe. (RegisterDemoReset registers nothing unless
	// DEMO_RESET_ENABLED is set.)
	"OnServe": 7,

	// registerSchemaHooks regenerates workspace TypeScript on collection
	// edits; a tenant has no workspace.
	"OnCollectionCreateRequest": 1,
	"OnCollectionUpdateRequest": 1,
	"OnCollectionDeleteRequest": 1,

	// RegisterPackageInstallEndpoints' restart guard (pkg_install.go): vetoes
	// an app restart while an install job is running. Tenants have no install
	// jobs and never self-restart — the router owns their lifecycle.
	"OnTerminate": 1,
}

// hookHandlerCounts enumerates every hook accessor on the app (the On*
// methods) via reflection and returns hook name → bound handler count.
// TaggedHook promotes Length() from the main hook, so tag-scoped bindings
// (e.g. OnRecordUpdateRequest("users")) are counted on their parent hook.
func hookHandlerCounts(t *testing.T, app *pocketbase.PocketBase) map[string]int {
	t.Helper()

	counts := map[string]int{}
	v := reflect.ValueOf(app)
	tp := v.Type()
	for i := 0; i < tp.NumMethod(); i++ {
		m := tp.Method(i)
		if !strings.HasPrefix(m.Name, "On") {
			continue
		}
		mt := m.Func.Type()
		// A hook accessor takes only the receiver (plus optional variadic
		// tags) and returns exactly one value.
		if mt.NumOut() != 1 || mt.NumIn() > 2 || (mt.NumIn() == 2 && !mt.IsVariadic()) {
			continue
		}
		res := v.Method(i).Call(nil)
		h, ok := res[0].Interface().(interface{ Length() int })
		if !ok {
			continue
		}
		counts[m.Name] = h.Length()
	}
	if len(counts) == 0 {
		t.Fatal("hookHandlerCounts found no On* hook accessors — reflection assumptions broken")
	}
	return counts
}

func TestTenantCompositionMatchesHostMinusRecordedExceptions(t *testing.T) {
	// The host path falls back to quota.RegisteredSources() (a process
	// global other tests may have populated) when Options.QuotaSources is
	// empty; clear it so both compositions see the same (empty) quota input.
	quota.ResetSourcesForTesting()

	host := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(host, Options{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		TypesDir:      t.TempDir(),
		PublicDir:     t.TempDir(),
		HooksPoolSize: 1,
	})

	tenant := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := RegisterTenant(tenant, TenantOptions{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 1,
	}); err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	hostCounts := hookHandlerCounts(t, host)
	tenantCounts := hookHandlerCounts(t, tenant)

	var problems []string
	names := make([]string, 0, len(hostCounts))
	for name := range hostCounts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		diff := hostCounts[name] - tenantCounts[name]
		if diff == hostOnlyHookDiff[name] {
			continue
		}
		switch {
		case diff > hostOnlyHookDiff[name]:
			problems = append(problems, fmt.Sprintf(
				"%s: host binds %d handler(s) the tenant does not (%d allowed). "+
					"A registration was added to Register without deciding whether tenants get it: "+
					"move it into registerSharedEarly/registerSharedCore, or record it host-only in "+
					"server.go's tail AND in hostOnlyHookDiff with a reason.",
				name, diff, hostOnlyHookDiff[name]))
		default:
			problems = append(problems, fmt.Sprintf(
				"%s: tenant binds %d MORE handler(s) than the host (or a recorded host-only "+
					"divergence disappeared — update hostOnlyHookDiff). Tenant-only behavior belongs "+
					"in RegisterTenant with a comment; shared behavior belongs in the shared set.",
				name, -(diff-hostOnlyHookDiff[name])))
		}
	}

	if len(problems) > 0 {
		t.Fatalf("host and tenant compositions diverged without a recorded reason:\n  %s",
			strings.Join(problems, "\n  "))
	}
}
