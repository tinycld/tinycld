package deliveryevents

import (
	"errors"
	"strings"
	"testing"

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

func TestApply_NoSinksIsUnhandled(t *testing.T) {
	reset(t)
	handled, err := Apply(nil, Event{Kind: Delivered, ProviderMessageID: "m1"})
	if handled || err != nil {
		t.Fatalf("Apply = (%v, %v), want (false, nil)", handled, err)
	}
}

func TestApply_FirstHandlingSinkWins(t *testing.T) {
	reset(t)
	var calls []string
	Register(fakeSlug, func(core.App, Event) (bool, error) { calls = append(calls, fakeSlug); return false, nil })
	Register("gadgets", func(core.App, Event) (bool, error) { calls = append(calls, "gadgets"); return true, nil })
	Register("gizmos", func(core.App, Event) (bool, error) { calls = append(calls, "gizmos"); return true, nil })
	handled, _ := Apply(nil, Event{Kind: Bounced, ProviderMessageID: "m1"})
	if !handled || strings.Join(calls, ",") != "widgets,gadgets" {
		t.Fatalf("handled=%v calls=%v", handled, calls)
	}
}

func TestApply_ErrorStopsAndIsReturned(t *testing.T) {
	reset(t)
	boom := errors.New("boom")
	Register(fakeSlug, func(core.App, Event) (bool, error) { return false, boom })
	if _, err := Apply(nil, Event{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestRegister_IdempotentPerSlug(t *testing.T) {
	reset(t)
	n := 0
	s := func(core.App, Event) (bool, error) { n++; return false, nil }
	Register(fakeSlug, s)
	Register(fakeSlug, s)
	_, _ = Apply(nil, Event{})
	if n != 1 {
		t.Fatalf("sink ran %d times, want 1", n)
	}
}
