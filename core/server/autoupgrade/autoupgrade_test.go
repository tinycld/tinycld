package autoupgrade

import (
	"context"
	"testing"
)

type fake struct{}

func (fake) PolicyChanged(context.Context, bool) error { return nil }
func (fake) Status(context.Context) (Status, error)    { return Status{Available: true}, nil }

func TestSetDelegate(t *testing.T) {
	t.Cleanup(func() { SetDelegate(nil) })
	if Current() != nil {
		t.Fatal("expected no delegate")
	}
	SetDelegate(fake{})
	if Current() == nil {
		t.Fatal("delegate not installed")
	}
}

func TestUnavailable(t *testing.T) {
	s := Unavailable("no toolchain")
	if s.Available || s.Reason != "no toolchain" {
		t.Fatalf("got %+v", s)
	}
}
