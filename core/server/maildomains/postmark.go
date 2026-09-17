package maildomains

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mrz1836/postmark"
)

// ErrDomainAlreadyEnrolled means the provider account already has this domain.
// On a shared hosting account that is the normal collision between two orgs
// claiming the same name, so it is a distinct error the caller can phrase as
// "already configured on this host" rather than surfacing a raw 422.
var ErrDomainAlreadyEnrolled = errors.New("maildomains: domain already enrolled on this provider account")

// ErrDomainNotEnrolled means the account has never heard of the domain — the
// state every domain is in before AddDomain runs.
var ErrDomainNotEnrolled = errors.New("maildomains: domain not enrolled with the provider")

// domainListLimit bounds the domain listing Postmark pages over.
const domainListLimit = 100

// PostmarkDomains is the slice of the Postmark client this registrar needs.
// Every method on it is an ACCOUNT-token call (postmark/domains.go), which is
// the entire reason this seam exists.
type PostmarkDomains interface {
	CreateDomain(ctx context.Context, req postmark.DomainCreateRequest) (postmark.DomainDetails, error)
	GetDomains(ctx context.Context, count, offset int) (postmark.DomainsList, error)
	GetDomain(ctx context.Context, domainID int64) (postmark.DomainDetails, error)
}

// PostmarkRegistrar is the DIRECT implementation: it calls Postmark
// in-process, authenticated with the account token its accessor returns.
// This is the standalone path, where the deployment legitimately owns its
// own account.
type PostmarkRegistrar struct {
	// accountToken is read PER CALL, not captured at construction. At the
	// production call site (coreserver.wireMailDomains) construction happens
	// synchronously inside RegisterSystemConfig, before systemConfig.load(app)
	// has run in the later OnServe hook — so a value captured once here would
	// be permanently "" and every call would return ErrNotConfigured forever,
	// even after the settings row loads. Reading it lazily on each call is the
	// same pattern syscfg itself documents (system_config.go: "resolver reads
	// lazily... so it's safe to set here before the OnServe load") and matches
	// how mailer already reads syscfg per-send rather than once at wiring
	// time. It also means a later operator edit to the token takes effect
	// immediately, with no re-wiring required.
	accountToken func() string
	client       PostmarkDomains
}

// NewPostmarkRegistrar builds the direct registrar from a token accessor. An
// accessor that returns "" (checked per call, after TrimSpace) yields
// ErrNotConfigured rather than a failing API call.
//
// Takes a func() string rather than a plain string so the token can resolve
// from a seam that populates after construction (syscfg, in core's wiring)
// without becoming stale. A caller that already holds a concrete token
// up front (hosting's mailDomainRegistrar) can still use one: wrap it with
// StaticToken.
func NewPostmarkRegistrar(accountToken func() string, client PostmarkDomains) *PostmarkRegistrar {
	return &PostmarkRegistrar{accountToken: accountToken, client: client}
}

// StaticToken wraps an already-known token as a func() string, for a caller
// that holds a concrete value up front rather than a seam to read lazily
// (e.g. hosting's mailDomainRegistrar, which reads the token once from its
// own control-plane config on each request).
func StaticToken(token string) func() string {
	return func() string { return token }
}

func (p *PostmarkRegistrar) token() string {
	return strings.TrimSpace(p.accountToken())
}

func (p *PostmarkRegistrar) AddDomain(ctx context.Context, domain string) (*DomainRecords, error) {
	if p.token() == "" {
		return nil, ErrNotConfigured
	}
	details, err := p.client.CreateDomain(ctx, postmark.DomainCreateRequest{Name: domain})
	if err != nil {
		if isAlreadyExists(err) {
			return nil, fmt.Errorf("%w: %s", ErrDomainAlreadyEnrolled, domain)
		}
		return nil, fmt.Errorf("postmark create domain: %w", err)
	}
	return toDomainRecords(details), nil
}

func (p *PostmarkRegistrar) GetDomain(ctx context.Context, domain string, providerDomainID int64) (*DomainRecords, error) {
	if p.token() == "" {
		return nil, ErrNotConfigured
	}
	if providerDomainID != 0 {
		details, err := p.client.GetDomain(ctx, providerDomainID)
		if err != nil {
			// A stored id Postmark no longer knows means the domain was removed
			// on their side. That is "not enrolled" — the actionable answer —
			// not an opaque API failure the admin cannot interpret.
			if isNotFound(err) {
				return nil, fmt.Errorf("%w: %s", ErrDomainNotEnrolled, domain)
			}
			return nil, fmt.Errorf("postmark get domain: %w", err)
		}
		return toDomainRecords(details), nil
	}
	return p.findByName(ctx, domain)
}

// findByName is the fallback for a row enrolled before the id was stored. It
// pages the account's domain list; the caller is expected to persist the id
// from the result so this runs at most once per domain.
func (p *PostmarkRegistrar) findByName(ctx context.Context, domain string) (*DomainRecords, error) {
	for offset := 0; ; offset += domainListLimit {
		list, err := p.client.GetDomains(ctx, domainListLimit, offset)
		if err != nil {
			return nil, fmt.Errorf("postmark list domains: %w", err)
		}
		for _, d := range list.Domains {
			if strings.EqualFold(d.Name, domain) {
				details, err := p.client.GetDomain(ctx, d.ID)
				if err != nil {
					return nil, fmt.Errorf("postmark get domain: %w", err)
				}
				return toDomainRecords(details), nil
			}
		}
		// Stop at the last page. TotalCount is the account's full size, so
		// this terminates even when a page comes back short.
		if len(list.Domains) == 0 || offset+len(list.Domains) >= list.TotalCount {
			return nil, fmt.Errorf("%w: %s", ErrDomainNotEnrolled, domain)
		}
	}
}

// isNotFound recognises Postmark's "no such domain" refusal. Matched on the
// message for the same reason as isAlreadyExists: the numeric codes are not
// documented as stable.
func isNotFound(err error) bool {
	var apiErr postmark.APIError
	if errors.As(err, &apiErr) {
		msg := strings.ToLower(apiErr.Message)
		return strings.Contains(msg, "does not exist") || strings.Contains(msg, "not found")
	}
	return false
}

// isAlreadyExists recognises Postmark's duplicate-name refusal.
//
// Matched on the message rather than the numeric code, which Postmark does not
// document as stable. If they reword it this returns false and the caller
// surfaces a generic wrapped error — worse copy, but never a wrong outcome.
func isAlreadyExists(err error) bool {
	var apiErr postmark.APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(strings.ToLower(apiErr.Message), "already exists")
	}
	return false
}

func toDomainRecords(d postmark.DomainDetails) *DomainRecords {
	return &DomainRecords{
		Domain:               d.Name,
		ID:                   d.ID,
		SPFVerified:          d.SPFVerified,
		DKIMVerified:         d.DKIMVerified,
		ReturnPathVerified:   d.ReturnPathDomainVerified,
		DKIMHost:             d.DKIMHost,
		DKIMTextValue:        d.DKIMTextValue,
		ReturnPathDomain:     d.ReturnPathDomain,
		ReturnPathCNAMEValue: d.ReturnPathDomainCNAMEValue,
	}
}
