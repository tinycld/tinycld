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
// It exists for composition-parity tests: where a server is composed two ways
// — one adding registrations the other must not have — compose one app per way
// and assert the per-hook difference equals the recorded set. A registration
// added to one composition without deciding whether the other gets it then
// fails with the offending hook name, instead of silently drifting. That
// drift is not hypothetical: it once left a second composition without the
// users field guard, and any member could PATCH their own role to owner.
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

// AssertCompositionDiff compares two compositions' hook-handler counts and
// fails unless the fuller one binds exactly `extra` more handlers per hook
// than the leaner one (hooks absent from extra must match exactly).
func AssertCompositionDiff(t testing.TB, fullCounts, leanCounts, extra map[string]int) {
	t.Helper()

	names := map[string]bool{}
	for name := range fullCounts {
		names[name] = true
	}
	for name := range leanCounts {
		names[name] = true
	}

	for name := range names {
		diff := fullCounts[name] - leanCounts[name]
		allowed := extra[name]
		switch {
		case diff > allowed:
			t.Errorf("%s: the full composition binds %d handler(s) the lean one does not (%d allowed). "+
				"A registration was added to Register without deciding whether tenants get it: "+
				"move it into registerShared, or record it in the host-only tail AND in the "+
				"test's host-only map with a reason.", name, diff, allowed)
		case diff < allowed:
			t.Errorf("%s: the lean composition binds %d MORE handler(s) than recorded (or a recorded "+
				"host-only divergence disappeared — update the host-only map). Tenant-only "+
				"behavior is not a thing a feature should have; shared behavior belongs in "+
				"registerShared.", name, allowed-diff)
		}
	}
}
