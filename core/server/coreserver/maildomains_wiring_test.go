package coreserver

import (
	"context"
	"errors"
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

// TestMailDomainsResolveTokenAfterLateLoad reproduces the REAL production
// boot order, not the inverted one the other tests here use for convenience.
//
// In production (system_config.go RegisterSystemConfig), wireMailDomains()
// runs synchronously and FIRST — before systemConfig.load(app), which only
// happens later inside an OnServe hook. So at wiring time syscfg.Get returns
// "" for every key, including the account token. A registrar that captures
// the token at construction (the old behaviour) is permanently starved: it
// is stuck holding "" for the life of the process, so add-domain 503s
// forever on a plain standalone Postmark deployment, even after the
// system_settings row loads and even across restarts. This is C1 from the
// final whole-branch review.
//
// This test wires FIRST while the resolver has nothing, THEN populates it
// (simulating the later OnServe load), THEN asserts a call succeeds — the
// opposite order from TestMailDomainsWiredFromSettings above, which sets the
// resolver before wiring and so cannot catch a captured-at-construction bug.
func TestMailDomainsResolveTokenAfterLateLoad(t *testing.T) {
	maildomains.ResetForTesting()
	syscfg.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	t.Cleanup(syscfg.ResetForTesting)

	token := ""
	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_account_token" {
			return token
		}
		return ""
	})

	// Wire while the token does not exist yet — the real boot order.
	wireMailDomains()

	reg, ok := maildomains.Current().(*maildomains.PostmarkRegistrar)
	if !ok {
		t.Fatalf("Current() = %T, want *maildomains.PostmarkRegistrar", maildomains.Current())
	}

	// AddDomain must fail before the settings row has loaded.
	if _, err := reg.AddDomain(context.Background(), "acme.com"); !errors.Is(err, maildomains.ErrNotConfigured) {
		t.Fatalf("AddDomain before load: err = %v, want ErrNotConfigured", err)
	}

	// Simulate the later OnServe load populating the value.
	token = "acct-token"

	// The SAME registrar instance (no re-wiring) must now resolve the token
	// and get PAST the ErrNotConfigured gate. wireMailDomains builds a real
	// postmark.Client, so we cannot let this reach the network in a unit
	// test; an already-expired context makes the underlying HTTP call fail
	// immediately with a deadline error instead. Any error OTHER than
	// ErrNotConfigured proves the token resolved — that is the only thing
	// this regression is about.
	expiredCtx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-expiredCtx.Done()
	if _, err := reg.AddDomain(expiredCtx, "acme.com"); errors.Is(err, maildomains.ErrNotConfigured) {
		t.Fatalf("AddDomain after load: err = %v, want the token to resolve (not ErrNotConfigured)", err)
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
