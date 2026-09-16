package maildomains

import (
	"context"
	"errors"
	"testing"

	"github.com/mrz1836/postmark"
)

type fakeDomains struct {
	created   postmark.DomainDetails
	createErr error
	list      []postmark.Domain
	details   map[int64]postmark.DomainDetails
}

func (f *fakeDomains) CreateDomain(context.Context, postmark.DomainCreateRequest) (postmark.DomainDetails, error) {
	if f.createErr != nil {
		return postmark.DomainDetails{}, f.createErr
	}
	return f.created, nil
}
func (f *fakeDomains) GetDomains(context.Context, int, int) (postmark.DomainsList, error) {
	return postmark.DomainsList{Domains: f.list}, nil
}
func (f *fakeDomains) GetDomain(_ context.Context, id int64) (postmark.DomainDetails, error) {
	d, ok := f.details[id]
	if !ok {
		return postmark.DomainDetails{}, errors.New("not found")
	}
	return d, nil
}

// AddDomain returns the DNS records the customer must publish — the whole
// point of the call. These are exactly the values the old code fetched and
// discarded.
func TestAddDomainReturnsRecords(t *testing.T) {
	f := &fakeDomains{created: postmark.DomainDetails{
		ID: 7, Name: "acme.com",
		DKIMHost: "20260916._domainkey.acme.com", DKIMTextValue: "k=rsa;p=MIIB",
		ReturnPathDomain: "pm-bounces.acme.com", ReturnPathDomainCNAMEValue: "pm.mtasv.net",
	}}
	r := NewPostmarkRegistrar("acct", f)

	rec, err := r.AddDomain(context.Background(), "acme.com")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if rec.ID != 7 || rec.Domain != "acme.com" {
		t.Fatalf("rec = %+v, want ID 7 / acme.com", rec)
	}
	if rec.DKIMHost == "" || rec.DKIMTextValue == "" {
		t.Error("DKIM record values must survive into DomainRecords")
	}
	if rec.ReturnPathDomain == "" || rec.ReturnPathCNAMEValue == "" {
		t.Error("return-path record values must survive into DomainRecords")
	}
}

// Postmark domains are unique per ACCOUNT, so on a shared hosting account a
// second org claiming the same domain hits this. It must be a distinct error
// the UI can phrase, not an opaque 422.
func TestAddDomainDuplicateIsDistinct(t *testing.T) {
	f := &fakeDomains{createErr: postmark.APIError{ErrorCode: 504, Message: "A domain with this name already exists."}}
	r := NewPostmarkRegistrar("acct", f)

	if _, err := r.AddDomain(context.Background(), "acme.com"); !errors.Is(err, ErrDomainAlreadyEnrolled) {
		t.Fatalf("err = %v, want ErrDomainAlreadyEnrolled", err)
	}
}

func TestGetDomainFindsByName(t *testing.T) {
	f := &fakeDomains{
		list:    []postmark.Domain{{ID: 7, Name: "acme.com"}},
		details: map[int64]postmark.DomainDetails{7: {ID: 7, Name: "acme.com", SPFVerified: true, DKIMVerified: true}},
	}
	r := NewPostmarkRegistrar("acct", f)

	rec, err := r.GetDomain(context.Background(), "acme.com")
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if !rec.SPFVerified || !rec.DKIMVerified {
		t.Fatalf("rec = %+v, want SPF and DKIM verified", rec)
	}
}

// Name matching is case-insensitive: DNS is, and an admin may type "Acme.com".
func TestGetDomainMatchesCaseInsensitively(t *testing.T) {
	f := &fakeDomains{
		list:    []postmark.Domain{{ID: 7, Name: "acme.com"}},
		details: map[int64]postmark.DomainDetails{7: {ID: 7, Name: "acme.com"}},
	}
	r := NewPostmarkRegistrar("acct", f)

	if _, err := r.GetDomain(context.Background(), "ACME.com"); err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
}

// A domain the account has never heard of is its own error: the caller shows
// "not enrolled", which is actionable, rather than "lookup failed".
func TestGetDomainUnknownIsDistinct(t *testing.T) {
	r := NewPostmarkRegistrar("acct", &fakeDomains{})

	if _, err := r.GetDomain(context.Background(), "nope.com"); !errors.Is(err, ErrDomainNotEnrolled) {
		t.Fatalf("err = %v, want ErrDomainNotEnrolled", err)
	}
}

func TestNoAccountTokenIsNotConfigured(t *testing.T) {
	r := NewPostmarkRegistrar("", &fakeDomains{})

	if _, err := r.AddDomain(context.Background(), "acme.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
