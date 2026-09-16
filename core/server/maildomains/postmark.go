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

// PostmarkRegistrar is the DIRECT implementation: it holds the account token
// and calls Postmark in-process. This is the standalone path, where the
// deployment legitimately owns its own account.
type PostmarkRegistrar struct {
	accountToken string
	client       PostmarkDomains
}

// NewPostmarkRegistrar builds the direct registrar. An empty accountToken
// yields one that reports ErrNotConfigured rather than failing at the API.
func NewPostmarkRegistrar(accountToken string, client PostmarkDomains) *PostmarkRegistrar {
	return &PostmarkRegistrar{accountToken: strings.TrimSpace(accountToken), client: client}
}

func (p *PostmarkRegistrar) AddDomain(ctx context.Context, domain string) (*DomainRecords, error) {
	if p.accountToken == "" {
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

func (p *PostmarkRegistrar) GetDomain(ctx context.Context, domain string) (*DomainRecords, error) {
	if p.accountToken == "" {
		return nil, ErrNotConfigured
	}
	list, err := p.client.GetDomains(ctx, domainListLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("postmark list domains: %w", err)
	}
	for _, d := range list.Domains {
		if !strings.EqualFold(d.Name, domain) {
			continue
		}
		details, err := p.client.GetDomain(ctx, d.ID)
		if err != nil {
			return nil, fmt.Errorf("postmark get domain: %w", err)
		}
		return toDomainRecords(details), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrDomainNotEnrolled, domain)
}

// isAlreadyExists recognises Postmark's duplicate-name refusal. Matched on the
// message as well as the code because the code is not documented as stable,
// and misclassifying a duplicate as a generic failure would show an operator a
// raw provider string.
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
