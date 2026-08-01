package rlstest

import (
	"reflect"
	"strings"
	"testing"
)

// HookHandlerCounts enumerates every hook accessor on the app (the On*
// methods) via reflection and returns hook name → bound handler count.
// TaggedHook promotes Length() from the main hook, so tag-scoped bindings
// (e.g. OnRecordUpdateRequest("users")) are counted on their parent hook.
//
// It exists for composition-parity tests: a feature package that splits its
// server entry into Register (host: shared + host-only tail) and
// RegisterTenant (shared only) composes one app per entry point and asserts
// the per-hook difference equals its recorded host-only set — so a
// registration added to one composition without deciding whether the other
// gets it fails with the offending hook name, instead of silently drifting
// the way the tenant once did (multi-org/docs/FINDING-tenant-composition-gap.md).
// coreserver/composition_parity_test.go is the original of this pattern.
//
// app is any value exposing PocketBase's On* accessors — pass the
// *pocketbase.PocketBase (or tests.TestApp) itself.
func HookHandlerCounts(t testing.TB, app any) map[string]int {
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
		t.Fatal("HookHandlerCounts found no On* hook accessors — reflection assumptions broken")
	}
	return counts
}

// AssertCompositionDiff compares host and tenant hook-handler counts and
// fails unless the host binds exactly `hostOnly` more handlers per hook than
// the tenant (hooks absent from hostOnly must match exactly). Use with
// HookHandlerCounts over one app composed via Register and one via
// RegisterTenant.
func AssertCompositionDiff(t testing.TB, hostCounts, tenantCounts, hostOnly map[string]int) {
	t.Helper()

	names := map[string]bool{}
	for name := range hostCounts {
		names[name] = true
	}
	for name := range tenantCounts {
		names[name] = true
	}

	for name := range names {
		diff := hostCounts[name] - tenantCounts[name]
		allowed := hostOnly[name]
		switch {
		case diff > allowed:
			t.Errorf("%s: host binds %d handler(s) the tenant does not (%d allowed). "+
				"A registration was added to Register without deciding whether tenants get it: "+
				"move it into registerShared, or record it in the host-only tail AND in the "+
				"test's host-only map with a reason.", name, diff, allowed)
		case diff < allowed:
			t.Errorf("%s: tenant binds %d MORE handler(s) than recorded (or a recorded "+
				"host-only divergence disappeared — update the host-only map). Tenant-only "+
				"behavior is not a thing a feature should have; shared behavior belongs in "+
				"registerShared.", name, allowed-diff)
		}
	}
}
