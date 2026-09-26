package maildomains

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// refusingRegistrar stands in for a registrar with a naming policy of its own.
type refusingRegistrar struct{}

func (refusingRegistrar) AddDomain(_ context.Context, domain string) (*DomainRecords, error) {
	return nil, fmt.Errorf("add %s: %w", domain,
		Refused("name_blocked", domain+" cannot be used. Choose a different domain."))
}
func (refusingRegistrar) GetDomain(context.Context, string, int64) (*DomainRecords, error) {
	return nil, ErrDomainNotEnrolled
}

// A refusal must survive wrapping and stay distinguishable from other
// failures, since callers answer it with the registrar's own message.
func TestRefusalIsMatchableThroughWrapping(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	SetResolver(refusingRegistrar{})

	_, err := Current().AddDomain(context.Background(), "example.org")
	if !errors.Is(err, ErrDomainRefused) {
		t.Fatalf("errors.Is(err, ErrDomainRefused) = false for %v", err)
	}
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("errors.As did not find *RefusedError in %v", err)
	}
	if refused.Code != "name_blocked" {
		t.Errorf("Code = %q, want name_blocked", refused.Code)
	}
	if refused.Message != "example.org cannot be used. Choose a different domain." {
		t.Errorf("Message = %q", refused.Message)
	}
}

func TestOtherErrorsAreNotRefusals(t *testing.T) {
	for _, err := range []error{ErrNotConfigured, ErrDomainAlreadyEnrolled, ErrDomainNotEnrolled, errors.New("boom")} {
		if errors.Is(err, ErrDomainRefused) {
			t.Errorf("errors.Is(%v, ErrDomainRefused) = true, want false", err)
		}
	}
}
