package sharequota

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// Every test here names a fictional package. Core must not know a real one,
// and a test that reaches for "drive" is how that rule quietly stops holding.
const fakeSlug = "widgets"

func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(ResetForTesting)
	ResetForTesting()
}

func TestLimits_UnclaimedIsUnlimited(t *testing.T) {
	reset(t)

	got := Limits(nil)
	if got.Lifetime != 0 || got.PerDay != 0 {
		t.Errorf("Limits = %+v, want the zero value (unlimited)", got)
	}
	if IsClaimed() {
		t.Error("IsClaimed = true with nobody having claimed the seam")
	}
}

func TestSetLimits_Resolves(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) ShareLimits {
		return ShareLimits{Lifetime: 100, PerDay: 20}
	})

	got := Limits(nil)
	if got.Lifetime != 100 || got.PerDay != 20 {
		t.Errorf("Limits = %+v, want {100 20}", got)
	}
	if !IsClaimed() {
		t.Error("IsClaimed = false after SetLimits")
	}
}

// The claim is the point: a supervising composition owns these ceilings, and
// core's own wiring must not be able to point them back at something the
// deployment controls.
func TestSetLimits_ClaimIsNotOverwritten(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) ShareLimits { return ShareLimits{PerDay: 20} })
	SetLimits(func(core.App) ShareLimits { return ShareLimits{PerDay: 99999} })

	if got := Limits(nil); got.PerDay != 20 {
		t.Errorf("PerDay = %d, want the first resolver's 20", got.PerDay)
	}
}

// A nil resolver would silently mean unlimited, which is the opposite of what
// a caller installing one intends.
func TestSetLimits_NilIsIgnored(t *testing.T) {
	reset(t)

	SetLimits(nil)
	if IsClaimed() {
		t.Error("a nil resolver must not claim the seam")
	}
}

// A negative ceiling reaching an enforcement expression would refuse every
// download. Normalizing here means that cannot happen even if no plan
// validator ran.
func TestLimits_NegativeMeansUnlimited(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) ShareLimits {
		return ShareLimits{Lifetime: -1, PerDay: -50}
	})

	got := Limits(nil)
	if got.Lifetime != 0 || got.PerDay != 0 {
		t.Errorf("Limits = %+v, want negatives normalized to unlimited", got)
	}
}

func TestRegisterMetered_Records(t *testing.T) {
	reset(t)

	err := RegisterMetered(MeteredCollection{
		Slug:       fakeSlug,
		Collection: "widget_attachments",
		Meter:      func(core.App, *core.FileDownloadRequestEvent) error { return nil },
	})
	if err != nil {
		t.Fatalf("RegisterMetered: %v", err)
	}

	got := RegisteredMetered()
	if len(got) != 1 || got[0].Collection != "widget_attachments" {
		t.Fatalf("RegisteredMetered = %+v, want one widget_attachments entry", got)
	}
}

// Two compositions registering the same package must not bind two hooks and
// charge every download twice.
func TestRegisterMetered_IsIdempotent(t *testing.T) {
	reset(t)

	meter := func(core.App, *core.FileDownloadRequestEvent) error { return nil }
	for i := 0; i < 3; i++ {
		if err := RegisterMetered(MeteredCollection{
			Slug: fakeSlug, Collection: "widget_attachments", Meter: meter,
		}); err != nil {
			t.Fatalf("RegisterMetered %d: %v", i, err)
		}
	}

	if got := RegisteredMetered(); len(got) != 1 {
		t.Errorf("registered %d entries, want 1", len(got))
	}
}

func TestRegisterMetered_RejectsIncomplete(t *testing.T) {
	reset(t)

	if err := RegisterMetered(MeteredCollection{Slug: fakeSlug, Meter: func(core.App, *core.FileDownloadRequestEvent) error { return nil }}); err == nil {
		t.Error("a collection with no name must be rejected")
	}
	if err := RegisterMetered(MeteredCollection{Slug: fakeSlug, Collection: "widget_attachments"}); err == nil {
		t.Error("a collection with no meter must be rejected")
	}
	if got := RegisteredMetered(); len(got) != 0 {
		t.Errorf("a rejected registration must record nothing, got %+v", got)
	}
}

// The returned slice must not alias the registry.
func TestRegisteredMetered_ReturnsACopy(t *testing.T) {
	reset(t)

	_ = RegisterMetered(MeteredCollection{
		Slug: fakeSlug, Collection: "widget_attachments",
		Meter: func(core.App, *core.FileDownloadRequestEvent) error { return nil },
	})

	got := RegisteredMetered()
	got[0].Collection = "mutated"

	if again := RegisteredMetered(); again[0].Collection != "widget_attachments" {
		t.Errorf("the registry was mutated through a returned slice: %q", again[0].Collection)
	}
}

// A meter refuses by returning an error rather than calling e.Next(). Pinned
// because the calling convention is the whole contract between core and a
// package, and it is not obvious from the type alone.
func TestMeter_ErrorIsTheRefusal(t *testing.T) {
	reset(t)

	refusal := errors.New("over the daily limit")
	meter := Meter(func(core.App, *core.FileDownloadRequestEvent) error { return refusal })

	if err := meter(nil, nil); !errors.Is(err, refusal) {
		t.Errorf("meter returned %v, want the refusal to propagate", err)
	}
}
