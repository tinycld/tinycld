package coreserver

import (
	"errors"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

func TestHookPointDisabledUntilHandlerAdded(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.point")
	if hp.Enabled() {
		t.Fatal("a hook point with no handlers must report disabled")
	}

	hp.Add(func(_ ...any) (any, error) { return nil, nil })
	if !hp.Enabled() {
		t.Fatal("a hook point with a handler must report enabled")
	}
}

// THE load-bearing property: with nothing registered, Call must not reach a
// handler at all. A counter that stays at zero is the proof that an org which
// customizes nothing never pays for a VM borrow.
func TestHookPointFastPathInvokesNothing(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.fastpath")

	var invoked int
	// Registered on a DIFFERENT point, so this one stays empty. If Call ever
	// fell through to a global/shared handler list, this would fire.
	other := RegisterHookPoint("test.fastpath.other")
	other.Add(func(_ ...any) (any, error) {
		invoked++
		return nil, nil
	})

	res, err := hp.Call(map[string]any{"anything": true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Handled {
		t.Fatal("an unregistered hook point must report Handled=false")
	}
	if invoked != 0 {
		t.Fatalf("an unregistered hook point invoked %d handler(s); want 0", invoked)
	}
}

func TestHookPointCallReturnsHandlerValue(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.value")
	hp.Add(func(args ...any) (any, error) {
		payload := args[0].(map[string]any)
		return payload["name"].(string) + "!", nil
	})

	res, err := hp.Call(map[string]any{"name": "drive"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Handled {
		t.Fatal("Handled must be true when a handler ran")
	}
	if res.Value != "drive!" {
		t.Fatalf("got %#v, want %q", res.Value, "drive!")
	}
}

// A handler that returns nil must still mark the call Handled — the caller has
// to distinguish "TS ran and declined to change anything" from "no TS at all".
func TestHookPointHandledWithNilValue(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.nilvalue")
	hp.Add(func(_ ...any) (any, error) { return nil, nil })

	res, err := hp.Call(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Handled {
		t.Fatal("a handler returning nil must still mark the call Handled")
	}
	if res.Value != nil {
		t.Fatalf("got %#v, want nil", res.Value)
	}
}

// The veto path: a throwing handler aborts the chain and surfaces its error, so
// a beforeWrite/beforeDelete point can refuse the operation.
func TestHookPointCallPropagatesError(t *testing.T) {
	resetHookPointsForTesting()

	sentinel := errors.New("blocked by policy")
	hp := RegisterHookPoint("test.err")
	var secondRan bool
	hp.Add(func(_ ...any) (any, error) { return nil, sentinel })
	hp.Add(func(_ ...any) (any, error) { secondRan = true; return "unreachable", nil })

	res, err := hp.Call(map[string]any{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got err %v, want %v", err, sentinel)
	}
	if res.Handled || res.Value != nil {
		t.Fatalf("a failed call must return a zero result, got %#v", res)
	}
	if secondRan {
		t.Fatal("an erroring handler must abort the chain")
	}
}

func TestHookPointLastNonNilWins(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.chain")
	hp.Add(func(_ ...any) (any, error) { return "first", nil })
	hp.Add(func(_ ...any) (any, error) { return nil, nil })
	hp.Add(func(_ ...any) (any, error) { return "third", nil })

	res, err := hp.Call(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "third" {
		t.Fatalf("got %#v, want %q", res.Value, "third")
	}
}

// Registering the same name twice must yield the same point, so a package and
// its host can both reach it without ordering constraints.
func TestRegisterHookPointIsIdempotent(t *testing.T) {
	resetHookPointsForTesting()

	a := RegisterHookPoint("test.same")
	b := RegisterHookPoint("test.same")
	if a != b {
		t.Fatal("RegisterHookPoint must return the same point for the same name")
	}

	a.Add(func(_ ...any) (any, error) { return "x", nil })
	if !b.Enabled() {
		t.Fatal("handlers added via one reference must be visible through the other")
	}
	if LookupHookPoint("test.same") != a {
		t.Fatal("LookupHookPoint must return the registered point")
	}
	if LookupHookPoint("test.missing") != nil {
		t.Fatal("LookupHookPoint must return nil for an unknown name")
	}
}

func TestHookPointConcurrentCallAndAdd(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.concurrent")
	hp.Add(func(args ...any) (any, error) {
		return args[0].(map[string]any)["n"], nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%8 == 0 {
				hp.Add(func(_ ...any) (any, error) { return nil, nil })
				return
			}
			if _, err := hp.Call(map[string]any{"n": i}); err != nil {
				t.Errorf("call %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

// Add(nil) must not flip Enabled — otherwise a mis-wired binder would push the
// hot path onto the slow branch for every request while doing nothing.
func TestHookPointIgnoresNilHandler(t *testing.T) {
	resetHookPointsForTesting()

	hp := RegisterHookPoint("test.nil")
	hp.Add(nil)
	if hp.Enabled() {
		t.Fatal("Add(nil) must not enable the hook point")
	}

	var jsvmCallable jsvm.Callable
	hp.Add(jsvmCallable)
	if hp.Enabled() {
		t.Fatal("Add of a nil jsvm.Callable must not enable the hook point")
	}
}
