package outboundstats

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Core must not know a real package. A test that reaches for "mail" is how
// that rule quietly stops holding.
const fakeSlug = "widgets"

// reg returns an owned registry, so these tests share nothing and could run in
// parallel.
func reg(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry()
}

// The package-level functions must act on the one default registry.
func TestPackageFunctionsUseTheDefaultRegistry(t *testing.T) {
	t.Cleanup(ResetForTesting)
	ResetForTesting()

	Register(fakeSlug, func(core.App, time.Time) (int, error) { return 4, nil })

	if got := Default().Registered(); len(got) != 1 || got[0] != fakeSlug {
		t.Errorf("Default().Registered() = %v, want [%s]", got, fakeSlug)
	}
	if total, _, _ := Total(nil, time.Now()); total != 4 {
		t.Errorf("Total = %d, want 4", total)
	}

	ResetForTesting()
	if got := Registered(); len(got) != 0 {
		t.Errorf("Registered() after reset = %v, want nothing", got)
	}
}

func TestRegistriesAreIndependent(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	a.Register(fakeSlug, func(core.App, time.Time) (int, error) { return 3, nil })

	if total, _, _ := b.Total(nil, time.Now()); total != 0 {
		t.Errorf("second registry total = %d, want 0", total)
	}
	if got := b.Registered(); len(got) != 0 {
		t.Errorf("second registry Registered() = %v, want nothing", got)
	}
}

func TestTotal_NoCountersIsZeroAndComplete(t *testing.T) {
	r := reg(t)

	total, partial, errs := r.Total(nil, time.Now())
	if total != 0 || partial || len(errs) != 0 {
		t.Errorf("Total = (%d, %v, %v), want (0, false, none)", total, partial, errs)
	}
}

func TestTotal_SumsEveryCounter(t *testing.T) {
	r := reg(t)

	r.Register(fakeSlug, func(core.App, time.Time) (int, error) { return 7, nil })
	r.Register("gadgets", func(core.App, time.Time) (int, error) { return 5, nil })

	total, partial, _ := r.Total(nil, time.Now())
	if total != 12 {
		t.Errorf("total = %d, want 12", total)
	}
	if partial {
		t.Error("partial = true with every counter succeeding")
	}
}

// A package registered by two compositions must not have its sends counted
// twice — that would halve every rate computed from the result.
func TestRegister_IsIdempotentPerSlug(t *testing.T) {
	r := reg(t)

	r.Register(fakeSlug, func(core.App, time.Time) (int, error) { return 10, nil })
	r.Register(fakeSlug, func(core.App, time.Time) (int, error) { return 10, nil })

	if total, _, _ := r.Total(nil, time.Now()); total != 10 {
		t.Errorf("total = %d, want 10 — the second registration should be ignored", total)
	}
	if got := r.Registered(); len(got) != 1 {
		t.Errorf("Registered() = %v, want one entry", got)
	}
}

func TestRegister_IgnoresIncomplete(t *testing.T) {
	r := reg(t)

	r.Register("", func(core.App, time.Time) (int, error) { return 1, nil })
	r.Register(fakeSlug, nil)

	if got := r.Registered(); len(got) != 0 {
		t.Errorf("Registered() = %v, want nothing", got)
	}
}

// The distinction this exists for. "Sent nothing" and "could not say" are the
// same number and opposite facts: a caller that cannot tell them apart
// computes a rate of infinity from a denominator that was never really zero.
func TestTotal_ReportsPartialRatherThanUndercounting(t *testing.T) {
	r := reg(t)

	boom := errors.New("cannot count")
	r.Register(fakeSlug, func(core.App, time.Time) (int, error) { return 100, nil })
	r.Register("gadgets", func(core.App, time.Time) (int, error) { return 0, boom })

	total, partial, errs := r.Total(nil, time.Now())
	if !partial {
		t.Error("partial = false with a failing counter — the caller cannot tell the total is incomplete")
	}
	if total != 100 {
		t.Errorf("total = %d, want the 100 that did report", total)
	}
	if len(errs) != 1 || !errors.Is(errs[0], boom) {
		t.Errorf("errs = %v, want the counter's own error", errs)
	}
}

// One failing counter must not suppress the others: a partial answer is more
// useful than none, provided the caller knows it is partial.
func TestTotal_AFailingCounterDoesNotStopTheRest(t *testing.T) {
	r := reg(t)

	r.Register("a", func(core.App, time.Time) (int, error) { return 1, nil })
	r.Register("b", func(core.App, time.Time) (int, error) { return 0, errors.New("boom") })
	r.Register("c", func(core.App, time.Time) (int, error) { return 2, nil })

	total, partial, _ := r.Total(nil, time.Now())
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if !partial {
		t.Error("partial = false, want true")
	}
}

// The window reaches the counter unchanged; a registry that quietly rounded
// or shifted it would make every rate wrong in a way nothing else could see.
func TestTotal_PassesTheWindowThrough(t *testing.T) {
	r := reg(t)

	want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var got time.Time
	r.Register(fakeSlug, func(_ core.App, since time.Time) (int, error) {
		got = since
		return 0, nil
	})

	r.Total(nil, want)
	if !got.Equal(want) {
		t.Errorf("counter saw %v, want %v", got, want)
	}
}
