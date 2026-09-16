// Package maildomains is the indirection through which a mail feature performs
// the domain operations that require the operator's ACCOUNT-level mail-provider
// credentials — enrolling a sending domain and reading back its verification
// state and DNS records.
//
// It exists for the same reason syscfg does, one level up. syscfg delegates a
// VALUE a deployment must not hold; this delegates an OPERATION a deployment
// must not perform. Every call in Postmark's Domains API is account-token
// authenticated, including the reads, and an account token can enumerate,
// modify and delete every domain on the account — so in a hosted composition
// the token cannot live in the tenant at any privilege, and the operation must
// travel to whoever holds it instead.
//
// By default it is unconfigured and every call returns ErrNotConfigured. A
// standalone deployment points it at its own implementation with SetResolver; a
// SUPERVISING composition installs one with SetRegistrar, which CLAIMS the seam
// so the deployment's own wiring cannot later reclaim it.
//
// Nothing here names a feature package: this is a capability, like mailer and
// syscfg beside it.
package maildomains

import (
	"context"
	"errors"
	"sync"
)

// ErrNotConfigured is returned by the zero registrar. Callers surface it as
// "domain provisioning is not configured for this deployment" rather than as
// an opaque provider failure.
var ErrNotConfigured = errors.New("maildomains: no registrar configured")

// DomainRecords is the verification state of a sending domain plus the DNS
// records its owner must publish.
//
// Deliberately pure data with no credential of any kind: it is the wire type
// of the delegating implementation, so everything on it crosses a process
// boundary into a tenant that must not be trusted with the account token.
type DomainRecords struct {
	Domain               string `json:"domain"`
	ID                   int64  `json:"id"`
	SPFVerified          bool   `json:"spf_verified"`
	DKIMVerified         bool   `json:"dkim_verified"`
	ReturnPathVerified   bool   `json:"return_path_verified"`
	DKIMHost             string `json:"dkim_host,omitempty"`
	DKIMTextValue        string `json:"dkim_text_value,omitempty"`
	ReturnPathDomain     string `json:"return_path_domain,omitempty"`
	ReturnPathCNAMEValue string `json:"return_path_cname_value,omitempty"`
}

// Registrar performs the account-token operations.
//
// AddDomain enrolls the domain with the provider and returns its initial
// records. GetDomain reads back the current state of an already-enrolled
// domain. Both take plain data and return plain data, which is what lets the
// hosted implementation be a transport.
type Registrar interface {
	AddDomain(ctx context.Context, domain string) (*DomainRecords, error)

	// GetDomain reads the current state of an enrolled domain.
	//
	// providerDomainID is the provider's own id, or 0 when it is not known — a
	// row enrolled before the id was stored. A zero id forces a by-name lookup,
	// which is why it is worth persisting the id from the returned records:
	// Postmark has no lookup-by-name, so the fallback must page the account's
	// whole domain list and would report a domain past the first page as
	// unenrolled.
	GetDomain(ctx context.Context, domain string, providerDomainID int64) (*DomainRecords, error)
}

var (
	mu      sync.RWMutex
	current Registrar = unconfigured{}
	// claimed records that a SUPERVISING composition installed the registrar.
	//
	// Tracked explicitly rather than inferred from current != unconfigured{},
	// for the reason syscfg documents: a supervisor whose config failed to
	// load still owns this operation, and the correct degraded state is "no
	// domain provisioning", never "the deployment uses its own credentials".
	claimed bool
)

// unconfigured is the zero registrar: it performs nothing. It stands in before
// any Set* call so a read during boot returns an error rather than panicking.
type unconfigured struct{}

func (unconfigured) AddDomain(context.Context, string) (*DomainRecords, error) {
	return nil, ErrNotConfigured
}
func (unconfigured) GetDomain(context.Context, string, int64) (*DomainRecords, error) {
	return nil, ErrNotConfigured
}

// SetResolver points the seam at this deployment's own registrar — the
// standalone shape, where the deployment legitimately holds its own account
// token.
//
// Ignored once a supervising composition has claimed the seam. Core's wiring
// runs after a supervisor's, so without this the deployment would silently
// reclaim an operation its operator owns.
func SetResolver(r Registrar) {
	if r == nil {
		return
	}
	mu.RLock()
	taken := claimed
	mu.RUnlock()
	if taken {
		return
	}
	mu.Lock()
	current = r
	mu.Unlock()
}

// SetRegistrar installs the process-wide registrar and CLAIMS the seam. A
// supervising composition calls this BEFORE any package registers, so every
// consumer's first call already routes through it.
//
// A nil registrar is ignored rather than installed: it would turn every call
// into a nil dereference, and a caller passing nil has a bug, not an intent.
func SetRegistrar(r Registrar) {
	if r == nil {
		return
	}
	mu.Lock()
	current, claimed = r, true
	mu.Unlock()
}

// Current returns the installed registrar. Never nil.
func Current() Registrar {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// IsClaimed reports whether a supervising composition installed the registrar.
func IsClaimed() bool {
	mu.RLock()
	defer mu.RUnlock()
	return claimed
}

// ResetForTesting restores the zero state: no registrar, no claim. Tests that
// install one must call it (t.Cleanup) so the claim does not leak into the
// next test and make SetResolver inert there.
func ResetForTesting() {
	mu.Lock()
	current, claimed = unconfigured{}, false
	mu.Unlock()
}
