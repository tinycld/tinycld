package maildomains

import (
	"context"
	"errors"
	"testing"

	"github.com/mrz1836/postmark"
)

type fakeServers struct {
	list    postmark.ServersList
	err     error
	calls   int
	created []postmark.ServerCreateRequest
}

func (f *fakeServers) GetServers(context.Context, int64, int64, string) (postmark.ServersList, error) {
	f.calls++
	return f.list, f.err
}

func (f *fakeServers) CreateServer(_ context.Context, req postmark.ServerCreateRequest) (postmark.Server, error) {
	f.created = append(f.created, req)
	return srv(req.Name, "created-"+req.Name), nil
}

func srv(name, token string) postmark.Server {
	return postmark.Server{Name: name, APITokens: []string{token}}
}

// An explicitly configured server token wins: a deployment that deliberately
// supplies a scoped token must not have it silently replaced, and an existing
// deployment must keep working without migration.
func TestConfiguredServerTokenWins(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("a", "derived")}}}
	r := NewTokenResolver("acct", "configured", "", "", f)

	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatalf("ServerToken: %v", err)
	}
	if got != "configured" {
		t.Fatalf("token = %q, want %q", got, "configured")
	}
	if f.calls != 0 {
		t.Errorf("Postmark called %d times, want 0 when a token is configured", f.calls)
	}
}

// The one-server case: derive it, so an operator supplies only the account key.
func TestDerivesSingleServerToken(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("only", "tok-1")}}}
	r := NewTokenResolver("acct", "", "", "", f)

	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatalf("ServerToken: %v", err)
	}
	if got != "tok-1" {
		t.Fatalf("token = %q, want %q", got, "tok-1")
	}
}

// Derivation is cached: this runs on every send path.
func TestServerTokenCached(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("only", "tok-1")}}}
	r := NewTokenResolver("acct", "", "", "", f)

	for range 3 {
		if _, err := r.ServerToken(context.Background()); err != nil {
			t.Fatalf("ServerToken: %v", err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("Postmark called %d times, want 1 (cached)", f.calls)
	}
}

// Multiple servers with no name configured is a CONFIGURATION ERROR, never a
// silent pick: choosing the wrong server sends mail from the wrong place.
func TestAmbiguousServerIsAnError(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{
		srv("prod", "tok-prod"), srv("staging", "tok-stg"),
	}}}
	r := NewTokenResolver("acct", "", "", "", f)

	if _, err := r.ServerToken(context.Background()); !errors.Is(err, ErrAmbiguousServer) {
		t.Fatalf("err = %v, want ErrAmbiguousServer", err)
	}
}

// With a name configured, the matching server is selected. Match is
// case-insensitive: Postmark server names are display strings.
func TestSelectsNamedServer(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{
		srv("prod", "tok-prod"), srv("staging", "tok-stg"),
	}}}
	r := NewTokenResolver("acct", "", "PROD", "", f)

	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatalf("ServerToken: %v", err)
	}
	if got != "tok-prod" {
		t.Fatalf("token = %q, want %q", got, "tok-prod")
	}
}

// No account token and no configured token: nothing to derive from.
func TestNoCredentialsReturnsNotConfigured(t *testing.T) {
	r := NewTokenResolver("", "", "", "", &fakeServers{})

	if _, err := r.ServerToken(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// A fresh Postmark account has no server yet. The wizard asks for the account
// token only, so the first send has to make one, named after the organization.
func TestCreatesServerWhenAccountHasNone(t *testing.T) {
	f := &fakeServers{}
	r := NewTokenResolver("acct", "", "", "Harbor Dental", f)
	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "created-Harbor Dental" {
		t.Fatalf("token = %q, want the created server's", got)
	}
	if len(f.created) != 1 || f.created[0].Name != "Harbor Dental" || f.created[0].DeliveryType != "Live" {
		t.Fatalf("created = %+v, want one live server named after the org", f.created)
	}
}

// A configured name that is absent is created under that name, so an operator
// who names the server before it exists gets exactly what they named.
func TestCreatesNamedServerWhenMissing(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("other", "tok-o")}}}
	r := NewTokenResolver("acct", "", "PROD", "Harbor Dental", f)
	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "created-PROD" || len(f.created) != 1 || f.created[0].Name != "PROD" {
		t.Fatalf("token = %q created = %+v; want a server named PROD", got, f.created)
	}
}

// Several servers and no name is ambiguous, and one more server would not
// resolve it; nothing is created and the error still names the setting.
func TestAmbiguousAccountDoesNotCreate(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("a", "1"), srv("b", "2")}}}
	r := NewTokenResolver("acct", "", "", "Harbor Dental", f)
	if _, err := r.ServerToken(context.Background()); !errors.Is(err, ErrAmbiguousServer) {
		t.Fatalf("err = %v, want ErrAmbiguousServer", err)
	}
	if len(f.created) != 0 {
		t.Fatalf("created %d server(s); want none", len(f.created))
	}
}

// Without a name to create under, an empty account is still an error.
func TestNoServersAndNothingToCreateIsAnError(t *testing.T) {
	f := &fakeServers{}
	r := NewTokenResolver("acct", "", "", "", f)
	if _, err := r.ServerToken(context.Background()); err == nil || len(f.created) != 0 {
		t.Fatalf("err = %v created = %d; want an error and no server", err, len(f.created))
	}
}

// Every send asks for the token, so a failing derivation is remembered
// rather than repeated against Postmark on each one.
func TestFailedDerivationIsNotRetriedAtOnce(t *testing.T) {
	f := &fakeServers{err: errors.New("401")}
	r := NewTokenResolver("acct", "", "", "", f)
	for range 3 {
		if _, err := r.ServerToken(context.Background()); err == nil {
			t.Fatal("expected an error")
		}
	}
	if f.calls != 1 {
		t.Fatalf("GetServers called %d times; want 1 within the retry window", f.calls)
	}
}

// The process-wide source answers ServerToken; the default knows nothing.
func TestServerTokenSourceSeam(t *testing.T) {
	t.Cleanup(ResetForTesting)
	if _, err := ServerToken(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("default source err = %v, want ErrNotConfigured", err)
	}
	SetServerTokenSource(func(context.Context) (string, error) { return "tok", nil })
	got, err := ServerToken(context.Background())
	if err != nil || got != "tok" {
		t.Fatalf("got %q, %v", got, err)
	}
}
