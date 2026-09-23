package sendquota

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(ResetForTesting)
	ResetForTesting()
}

func TestLimits_UnclaimedIsUnlimited(t *testing.T) {
	reset(t)

	got := Limits(nil)
	if got.PerDay != 0 || got.PerHour != 0 {
		t.Errorf("Limits = %+v, want the zero value (unlimited)", got)
	}
	if IsClaimed() {
		t.Error("IsClaimed = true with nobody having claimed the seam")
	}
}

func TestSetLimits_Resolves(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) SendLimits { return SendLimits{PerDay: 500, PerHour: 5} })

	got := Limits(nil)
	if got.PerDay != 500 || got.PerHour != 5 {
		t.Errorf("Limits = %+v, want {500 5}", got)
	}
	if !IsClaimed() {
		t.Error("IsClaimed = false after SetLimits")
	}
}

// The claim is what makes a throttle a throttle. The deployment this limit
// is applied to must not be able to point it somewhere it controls.
func TestSetLimits_ClaimIsNotOverwritten(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) SendLimits { return SendLimits{PerHour: 5} })
	SetLimits(func(core.App) SendLimits { return SendLimits{PerHour: 99999} })

	if got := Limits(nil); got.PerHour != 5 {
		t.Errorf("PerHour = %d, want the first resolver's 5", got.PerHour)
	}
}

func TestSetLimits_NilIsIgnored(t *testing.T) {
	reset(t)

	SetLimits(nil)
	if IsClaimed() {
		t.Error("a nil resolver must not claim the seam")
	}
}

// A negative ceiling reaching an enforcement comparison would refuse every
// send. Normalizing here means that cannot happen even if whoever supplies
// the numbers never validated them.
func TestLimits_NegativeMeansUnlimited(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) SendLimits { return SendLimits{PerDay: -1, PerHour: -5} })

	got := Limits(nil)
	if got.PerDay != 0 || got.PerHour != 0 {
		t.Errorf("Limits = %+v, want negatives normalized to unlimited", got)
	}
}

// The two ceilings are independent: a throttle sets PerHour without
// disturbing the ordinary daily volume, and vice versa.
func TestLimits_CeilingsAreIndependent(t *testing.T) {
	reset(t)

	SetLimits(func(core.App) SendLimits { return SendLimits{PerHour: 5} })

	got := Limits(nil)
	if got.PerHour != 5 {
		t.Errorf("PerHour = %d, want 5", got.PerHour)
	}
	if got.PerDay != 0 {
		t.Errorf("PerDay = %d, want 0 — a throttle must not imply a daily ceiling", got.PerDay)
	}
}
