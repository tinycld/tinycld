package maildomains

import (
	"context"
	"errors"
	"testing"

	"github.com/mrz1836/postmark"
)

type fakeServers struct {
	list  postmark.ServersList
	err   error
	calls int
}

func (f *fakeServers) GetServers(context.Context, int64, int64, string) (postmark.ServersList, error) {
	f.calls++
	return f.list, f.err
}

func srv(name, token string) postmark.Server {
	return postmark.Server{Name: name, APITokens: []string{token}}
}

// An explicitly configured server token wins: a deployment that deliberately
// supplies a scoped token must not have it silently replaced, and an existing
// deployment must keep working without migration.
func TestConfiguredServerTokenWins(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("a", "derived")}}}
	r := NewTokenResolver("acct", "configured", "", f)

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
	r := NewTokenResolver("acct", "", "", f)

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
	r := NewTokenResolver("acct", "", "", f)

	for range 3 {
		if _, err := r.ServerToken(context.Background()); err != nil {
			t.Fatalf("ServerToken: %v", err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("Postmark called %d times, want 1 (cached)", f.calls)
	}
}

// Invalidate forces a re-resolve — the recovery path after a Postmark auth
// failure (a rotated token).
func TestInvalidateForcesReresolve(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{srv("only", "tok-1")}}}
	r := NewTokenResolver("acct", "", "", f)

	if _, err := r.ServerToken(context.Background()); err != nil {
		t.Fatalf("ServerToken: %v", err)
	}
	r.Invalidate()
	f.list = postmark.ServersList{Servers: []postmark.Server{srv("only", "tok-2")}}
	got, err := r.ServerToken(context.Background())
	if err != nil {
		t.Fatalf("ServerToken after Invalidate: %v", err)
	}
	if got != "tok-2" {
		t.Fatalf("token = %q, want the re-resolved %q", got, "tok-2")
	}
}

// Multiple servers with no name configured is a CONFIGURATION ERROR, never a
// silent pick: choosing the wrong server sends mail from the wrong place.
func TestAmbiguousServerIsAnError(t *testing.T) {
	f := &fakeServers{list: postmark.ServersList{Servers: []postmark.Server{
		srv("prod", "tok-prod"), srv("staging", "tok-stg"),
	}}}
	r := NewTokenResolver("acct", "", "", f)

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
	r := NewTokenResolver("acct", "", "PROD", f)

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
	r := NewTokenResolver("", "", "", &fakeServers{})

	if _, err := r.ServerToken(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
