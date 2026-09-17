package maildomains

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mrz1836/postmark"
)

type fakeDomains struct {
	created   postmark.DomainDetails
	createErr error
	list      []postmark.Domain
	details   map[int64]postmark.DomainDetails
	listCalls int
	getErr    error
}

func (f *fakeDomains) CreateDomain(context.Context, postmark.DomainCreateRequest) (postmark.DomainDetails, error) {
	if f.createErr != nil {
		return postmark.DomainDetails{}, f.createErr
	}
	return f.created, nil
}
func (f *fakeDomains) GetDomains(context.Context, int, int) (postmark.DomainsList, error) {
	f.listCalls++
	return postmark.DomainsList{Domains: f.list, TotalCount: len(f.list)}, nil
}
func (f *fakeDomains) GetDomain(_ context.Context, id int64) (postmark.DomainDetails, error) {
	if f.getErr != nil {
		return postmark.DomainDetails{}, f.getErr
	}
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
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

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
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	if _, err := r.AddDomain(context.Background(), "acme.com"); !errors.Is(err, ErrDomainAlreadyEnrolled) {
		t.Fatalf("err = %v, want ErrDomainAlreadyEnrolled", err)
	}
}

func TestGetDomainFindsByName(t *testing.T) {
	f := &fakeDomains{
		list:    []postmark.Domain{{ID: 7, Name: "acme.com"}},
		details: map[int64]postmark.DomainDetails{7: {ID: 7, Name: "acme.com", SPFVerified: true, DKIMVerified: true}},
	}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	rec, err := r.GetDomain(context.Background(), "acme.com", 0)
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
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	if _, err := r.GetDomain(context.Background(), "ACME.com", 0); err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
}

// A domain the account has never heard of is its own error: the caller shows
// "not enrolled", which is actionable, rather than "lookup failed".
func TestGetDomainUnknownIsDistinct(t *testing.T) {
	r := NewPostmarkRegistrar(StaticToken("acct"), &fakeDomains{})

	if _, err := r.GetDomain(context.Background(), "nope.com", 0); !errors.Is(err, ErrDomainNotEnrolled) {
		t.Fatalf("err = %v, want ErrDomainNotEnrolled", err)
	}
}

func TestNoAccountTokenIsNotConfigured(t *testing.T) {
	r := NewPostmarkRegistrar(StaticToken(""), &fakeDomains{})

	if _, err := r.AddDomain(context.Background(), "acme.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// With the id known, the lookup is ONE direct call — no listing. This is the
// whole point: a list-and-scan reports a domain past the first page as
// unenrolled, and on a hosting account one Postmark account serves every org.
func TestGetDomainByIDSkipsListing(t *testing.T) {
	f := &fakeDomains{details: map[int64]postmark.DomainDetails{
		7: {ID: 7, Name: "acme.com", DKIMVerified: true},
	}}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	rec, err := r.GetDomain(context.Background(), "acme.com", 7)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if !rec.DKIMVerified || rec.ID != 7 {
		t.Fatalf("rec = %+v, want the domain fetched by id", rec)
	}
	if f.listCalls != 0 {
		t.Errorf("GetDomains called %d times, want 0 when the id is known", f.listCalls)
	}
}

// A zero id is a row enrolled before ids were stored: fall back to the scan so
// existing installs keep working and can self-heal.
func TestGetDomainZeroIDFallsBackToScan(t *testing.T) {
	f := &fakeDomains{
		list:    []postmark.Domain{{ID: 7, Name: "acme.com"}},
		details: map[int64]postmark.DomainDetails{7: {ID: 7, Name: "acme.com"}},
	}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	rec, err := r.GetDomain(context.Background(), "acme.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.ID != 7 {
		t.Fatalf("rec.ID = %d, want 7 so the caller can persist it", rec.ID)
	}
	if f.listCalls != 1 {
		t.Errorf("GetDomains called %d times, want 1 for the fallback", f.listCalls)
	}
}

// A stored id that Postmark no longer knows (the domain was deleted in their
// dashboard) must read as not-enrolled, not as an opaque API error.
func TestGetDomainStaleIDIsNotEnrolled(t *testing.T) {
	r := NewPostmarkRegistrar(StaticToken("acct"), &fakeDomains{getErr: postmark.APIError{
		ErrorCode: 701, Message: "The domain does not exist.",
	}})

	if _, err := r.GetDomain(context.Background(), "acme.com", 999); !errors.Is(err, ErrDomainNotEnrolled) {
		t.Fatalf("err = %v, want ErrDomainNotEnrolled for a stale id", err)
	}
}

// A provider id is an account-global handle, and on a shared hosting account
// it may name a DIFFERENT org's domain. GetDomain must refuse to hand back a
// domain whose name is not the one the caller asked for, because the caller
// supplying the id is exactly the party that would be attacking.
//
// This is the cross-tenant disclosure this check exists to stop: without the
// name comparison the victim's DKIMHost / DKIMTextValue / return-path host
// come back to the attacker verbatim.
func TestGetDomainByIDRejectsMismatchedName(t *testing.T) {
	f := &fakeDomains{details: map[int64]postmark.DomainDetails{
		// The VICTIM's domain, enrolled by another org on the shared account.
		42: {
			ID: 42, Name: "victim.example",
			DKIMHost: "20260916._domainkey.victim.example", DKIMTextValue: "k=rsa;p=VICTIMKEY",
			ReturnPathDomain: "pm-bounces.victim.example", ReturnPathDomainCNAMEValue: "pm.mtasv.net",
			DKIMVerified: true, SPFVerified: true, ReturnPathDomainVerified: true,
		},
	}}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	// The attacker owns attacker.example and points its stored id at 42.
	rec, err := r.GetDomain(context.Background(), "attacker.example", 42)

	if !errors.Is(err, ErrDomainNotEnrolled) {
		t.Fatalf("err = %v, want ErrDomainNotEnrolled for an id naming another domain", err)
	}
	if rec != nil {
		t.Fatalf("rec = %+v, want nil — no records may cross the tenant boundary", rec)
	}
	// The error text must name the REQUESTED domain, never the victim's.
	if strings.Contains(err.Error(), "victim.example") {
		t.Errorf("error %q leaks the victim's domain name", err)
	}
}

// Belt-and-braces on the same defect, asserting on the DATA rather than the
// error: a mismatch must not surface the victim's DNS records under any
// field, since those are what an attacker iterating ids is harvesting.
func TestGetDomainByIDMismatchLeaksNoRecords(t *testing.T) {
	f := &fakeDomains{details: map[int64]postmark.DomainDetails{
		42: {
			ID: 42, Name: "victim.example",
			DKIMHost: "20260916._domainkey.victim.example", DKIMTextValue: "k=rsa;p=VICTIMKEY",
			ReturnPathDomain: "pm-bounces.victim.example",
		},
	}}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	rec, err := r.GetDomain(context.Background(), "attacker.example", 42)
	if err == nil {
		t.Fatal("GetDomain returned no error for a mismatched id")
	}
	if rec == nil {
		return
	}
	for name, got := range map[string]string{
		"Domain":           rec.Domain,
		"DKIMHost":         rec.DKIMHost,
		"DKIMTextValue":    rec.DKIMTextValue,
		"ReturnPathDomain": rec.ReturnPathDomain,
	} {
		if got != "" {
			t.Errorf("rec.%s = %q, want empty — the victim's records must not cross orgs", name, got)
		}
	}
}

// Case is not ownership: Postmark normalises names, so a stored id whose name
// differs only in case is still the caller's own domain and must resolve.
func TestGetDomainByIDMatchesCaseInsensitively(t *testing.T) {
	f := &fakeDomains{details: map[int64]postmark.DomainDetails{
		7: {ID: 7, Name: "Acme.COM", DKIMVerified: true},
	}}
	r := NewPostmarkRegistrar(StaticToken("acct"), f)

	rec, err := r.GetDomain(context.Background(), "acme.com", 7)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.ID != 7 {
		t.Fatalf("rec.ID = %d, want 7", rec.ID)
	}
}
