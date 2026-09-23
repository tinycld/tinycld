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

func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(ResetForTesting)
	ResetForTesting()
}

func TestTotal_NoCountersIsZeroAndComplete(t *testing.T) {
	reset(t)

	total, partial, errs := Total(nil, time.Now())
	if total != 0 || partial || len(errs) != 0 {
		t.Errorf("Total = (%d, %v, %v), want (0, false, none)", total, partial, errs)
	}
}

func TestTotal_SumsEveryCounter(t *testing.T) {
	reset(t)

	Register(fakeSlug, func(core.App, time.Time) (int, error) { return 7, nil })
	Register("gadgets", func(core.App, time.Time) (int, error) { return 5, nil })

	total, partial, _ := Total(nil, time.Now())
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
	reset(t)

	Register(fakeSlug, func(core.App, time.Time) (int, error) { return 10, nil })
	Register(fakeSlug, func(core.App, time.Time) (int, error) { return 10, nil })

	if total, _, _ := Total(nil, time.Now()); total != 10 {
		t.Errorf("total = %d, want 10 — the second registration should be ignored", total)
	}
	if got := Registered(); len(got) != 1 {
		t.Errorf("Registered() = %v, want one entry", got)
	}
}

func TestRegister_IgnoresIncomplete(t *testing.T) {
	reset(t)

	Register("", func(core.App, time.Time) (int, error) { return 1, nil })
	Register(fakeSlug, nil)

	if got := Registered(); len(got) != 0 {
		t.Errorf("Registered() = %v, want nothing", got)
	}
}

// The distinction this exists for. "Sent nothing" and "could not say" are the
// same number and opposite facts: a caller that cannot tell them apart
// computes a rate of infinity from a denominator that was never really zero.
func TestTotal_ReportsPartialRatherThanUndercounting(t *testing.T) {
	reset(t)

	boom := errors.New("cannot count")
	Register(fakeSlug, func(core.App, time.Time) (int, error) { return 100, nil })
	Register("gadgets", func(core.App, time.Time) (int, error) { return 0, boom })

	total, partial, errs := Total(nil, time.Now())
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
	reset(t)

	Register("a", func(core.App, time.Time) (int, error) { return 1, nil })
	Register("b", func(core.App, time.Time) (int, error) { return 0, errors.New("boom") })
	Register("c", func(core.App, time.Time) (int, error) { return 2, nil })

	total, partial, _ := Total(nil, time.Now())
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
	reset(t)

	want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var got time.Time
	Register(fakeSlug, func(_ core.App, since time.Time) (int, error) {
		got = since
		return 0, nil
	})

	Total(nil, want)
	if !got.Equal(want) {
		t.Errorf("counter saw %v, want %v", got, want)
	}
}
