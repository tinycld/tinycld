package maildomains

import (
	"context"
	"errors"
	"testing"
)

type stubRegistrar struct{ name string }

func (s stubRegistrar) AddDomain(context.Context, string) (*DomainRecords, error) {
	return &DomainRecords{Domain: s.name}, nil
}
func (s stubRegistrar) GetDomain(context.Context, string, int64) (*DomainRecords, error) {
	return &DomainRecords{Domain: s.name}, nil
}

// The zero state resolves nothing: a deployment that wired nothing up must
// get a clear "not configured", never a nil panic.
func TestUnconfiguredReturnsErrNotConfigured(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)

	if _, err := Current().AddDomain(context.Background(), "acme.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("AddDomain err = %v, want ErrNotConfigured", err)
	}
	if _, err := Current().GetDomain(context.Background(), "acme.com", 0); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetDomain err = %v, want ErrNotConfigured", err)
	}
}

// SetResolver is the standalone path: the deployment points the seam at its
// own implementation.
func TestSetResolverInstalls(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)

	SetResolver(stubRegistrar{name: "own"})
	rec, err := Current().GetDomain(context.Background(), "acme.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.Domain != "own" {
		t.Fatalf("Domain = %q, want %q", rec.Domain, "own")
	}
	if IsClaimed() {
		t.Error("SetResolver must not claim the seam")
	}
}

// The security-critical case, copied from syscfg: once a supervising
// composition claims the seam, core's own wiring must not be able to point it
// back at the deployment's own credentials.
func TestSetRegistrarClaimsAndBlocksResolver(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)

	SetRegistrar(stubRegistrar{name: "supervisor"})
	if !IsClaimed() {
		t.Fatal("SetRegistrar must claim the seam")
	}

	SetResolver(stubRegistrar{name: "own"})
	rec, err := Current().GetDomain(context.Background(), "acme.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.Domain != "supervisor" {
		t.Fatalf("Domain = %q, want the supervisor's registrar to survive SetResolver", rec.Domain)
	}
}

// A nil registrar is a caller bug, not an intent to disable the seam.
// Installing it would turn every call into a nil dereference.
func TestNilRegistrarIgnored(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)

	SetRegistrar(stubRegistrar{name: "good"})
	SetRegistrar(nil)
	rec, err := Current().GetDomain(context.Background(), "acme.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.Domain != "good" {
		t.Fatalf("Domain = %q, want nil SetRegistrar to be ignored", rec.Domain)
	}
}
