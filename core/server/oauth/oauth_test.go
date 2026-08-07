package oauth

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSigningKeyIsDomainSeparated(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	key, err := signingKey(app)
	if err != nil {
		t.Fatalf("signingKey: %v", err)
	}
	if key == "" {
		t.Fatal("signingKey returned empty string")
	}
	// A hex-encoded HMAC-SHA256 is always 64 chars.
	if len(key) != 64 {
		t.Fatalf("signingKey length = %d, want 64", len(key))
	}
	// The whole point of domain separation: never the raw auth secret.
	su, err := app.FindCachedCollectionByNameOrId("_superusers")
	if err != nil {
		t.Fatalf("find _superusers: %v", err)
	}
	if strings.Contains(key, su.AuthToken.Secret) {
		t.Fatal("signing key leaks the raw _superusers auth secret")
	}
}

func TestSigningKeyIsStable(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	first, err := signingKey(app)
	if err != nil {
		t.Fatalf("signingKey: %v", err)
	}
	second, err := signingKey(app)
	if err != nil {
		t.Fatalf("signingKey (2nd): %v", err)
	}
	if first != second {
		t.Fatal("signingKey is not deterministic across calls")
	}
}

func TestParseScopes(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"mail:read", 1},
		{"mail:read drive:write", 2},
		{"  mail:read   drive:write  ", 2},
	}
	for _, c := range cases {
		if got := ParseScopes(c.in); len(got) != c.want {
			t.Errorf("ParseScopes(%q) = %v, want %d scopes", c.in, got, c.want)
		}
	}
}

func TestHasScope(t *testing.T) {
	granted := []string{"mail:read", "drive:write"}
	if !HasScope(granted, "mail:read") {
		t.Error("HasScope should find a granted scope")
	}
	if HasScope(granted, "mail:send") {
		t.Error("HasScope must not find an ungranted scope")
	}
	if HasScope(nil, "mail:read") {
		t.Error("HasScope on nil must be false")
	}
}
