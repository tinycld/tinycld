package davprefix

import (
	"strings"
	"testing"
)

// A reserved namespace shadows infrastructure the app needs: /api puts a
// Basic-Auth DAV handler in front of PocketBase's entire REST API, /_ in front
// of its dashboard, /.well-known in front of the protocol discovery DAV clients
// themselves use. A hostile or sloppy package must not intercept those.
func TestClaim_RejectsReservedNamespaces(t *testing.T) {
	for _, bad := range []string{"/api", "/api/v2", "/_", "/_/x", "/.well-known", "/.well-known/caldav"} {
		err := New().Claim("doodads", bad)
		if err == nil {
			t.Errorf("prefix %q accepted — it shadows a reserved namespace", bad)
			continue
		}
		if !strings.Contains(err.Error(), "doodads") {
			t.Errorf("prefix %q: error %q does not name the package", bad, err)
		}
	}
}

// A malformed prefix mounts somewhere other than intended — "" and "/" are a
// site-wide catch-all in front of the SPA.
func TestClaim_RejectsMalformedPrefix(t *testing.T) {
	for _, bad := range []string{"doodads", "", "/", "/doodads/"} {
		err := New().Claim("doodads", bad)
		if err == nil {
			t.Errorf("prefix %q: expected an error, got none", bad)
			continue
		}
		if !strings.Contains(err.Error(), "doodads") {
			t.Errorf("prefix %q: error %q does not name the package", bad, err)
		}
	}
}

// Two packages claiming one prefix is a ServeMux panic at boot. Fail the claim
// instead, naming both.
func TestClaim_RejectsDuplicate(t *testing.T) {
	r := New()
	if err := r.Claim("doodads", "/dav/things"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	err := r.Claim("widgets", "/dav/things")
	if err == nil {
		t.Fatal("duplicate prefix accepted")
	}
	for _, slug := range []string{"doodads", "widgets"} {
		if !strings.Contains(err.Error(), slug) {
			t.Errorf("error %q does not name %s", err, slug)
		}
	}
}

// The per-protocol check this replaces could not see across protocols, so a
// webdav source could claim /caldav. One registry per process catches it.
func TestClaim_RejectsAcrossProtocols(t *testing.T) {
	r := New()
	if err := r.Claim("doodads", "/caldav"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := r.Claim("widgets", "/caldav"); err == nil {
		t.Fatal("a second protocol claimed a prefix another already owns")
	}
}

func TestClaim_AcceptsDistinctPrefixes(t *testing.T) {
	r := New()
	for _, p := range []string{"/caldav", "/carddav", "/dav/files"} {
		if err := r.Claim("doodads", p); err != nil {
			t.Errorf("Claim(%q) = %v, want nil", p, err)
		}
	}
}
