package coreserver

import (
	"context"
	"testing"

	"tinycld.org/core/maildomains"
	"tinycld.org/core/syscfg"
)

// Standalone: the deployment's own account token wires the direct registrar.
func TestMailDomainsWiredFromSettings(t *testing.T) {
	maildomains.ResetForTesting()
	syscfg.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	t.Cleanup(syscfg.ResetForTesting)

	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_account_token" {
			return "acct-token"
		}
		return ""
	})
	wireMailDomains()

	if _, ok := maildomains.Current().(*maildomains.PostmarkRegistrar); !ok {
		t.Fatalf("Current() = %T, want *maildomains.PostmarkRegistrar", maildomains.Current())
	}
}

// A supervising composition already claimed the seam: core must not reclaim
// it. This is the security property, tested at the wiring level rather than
// trusting the seam alone.
func TestMailDomainsWiringRespectsClaim(t *testing.T) {
	maildomains.ResetForTesting()
	syscfg.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	t.Cleanup(syscfg.ResetForTesting)

	claimed := stubClaimedRegistrar{}
	maildomains.SetRegistrar(claimed)

	syscfg.SetResolver(func(string) string { return "acct-token" })
	wireMailDomains()

	if _, ok := maildomains.Current().(stubClaimedRegistrar); !ok {
		t.Fatalf("Current() = %T, want the supervisor's registrar to survive core wiring", maildomains.Current())
	}
}

type stubClaimedRegistrar struct{}

func (stubClaimedRegistrar) AddDomain(context.Context, string) (*maildomains.DomainRecords, error) {
	return nil, nil
}
func (stubClaimedRegistrar) GetDomain(context.Context, string, int64) (*maildomains.DomainRecords, error) {
	return nil, nil
}
