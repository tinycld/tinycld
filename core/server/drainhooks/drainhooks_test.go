package drainhooks

import (
	"reflect"
	"testing"
)

func TestRunBeginCallsEveryHandlerInOrder(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	var got []string
	OnBegin("acme-listeners", func() { got = append(got, "acme-listeners") })
	OnBegin("acme-queue", func() { got = append(got, "acme-queue") })

	RunBegin()
	if want := []string{"acme-listeners", "acme-queue"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handlers ran %v, want %v", got, want)
	}
}

// A package registers from its Register(), which can run again (a dev
// reload); the second registration replaces the first instead of running
// the handler twice.
func TestOnBeginReplacesASameNamedHandler(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	calls := map[string]int{}
	OnBegin("acme-listeners", func() { calls["first"]++ })
	OnBegin("acme-listeners", func() { calls["second"]++ })

	RunBegin()
	if calls["first"] != 0 || calls["second"] != 1 {
		t.Fatalf("calls = %v, want only the second handler once", calls)
	}
}

// One package's panic must not keep the next package's listeners open.
func TestRunBeginSurvivesAPanickingHandler(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	ran := false
	OnBegin("acme-broken", func() { panic("boom") })
	OnBegin("acme-listeners", func() { ran = true })

	RunBegin()
	if !ran {
		t.Fatal("a panicking handler stopped the handlers after it")
	}
}

func TestRunBeginWithNothingRegistered(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()
	RunBegin()
}
