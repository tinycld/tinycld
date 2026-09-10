package oauth

import "testing"

// Every route this package registers must have a DELIBERATE classification in
// ScopeForRoute — exempt, or a specific scope. Falling into default-deny by
// omission is silent: the route 403s only for OAuth callers, so unit tests of
// the handler still pass and only an integration notices.
//
// This has already happened twice. Narrowing exemptPaths away from a blanket
// "/oauth/" prefix orphaned /oauth/userinfo, which the discovery document
// advertises — clients following the well-known metadata got a 403 on the
// standard identity call. This test is the guard against the third time.
//
// Keep it in step with register.go. A new route here is a prompt to decide
// which bucket it belongs in, not to append it mechanically.
func TestEveryRegisteredRouteIsClassified(t *testing.T) {
	// Routes an OAuth access token may legitimately call. These MUST resolve to
	// exempt or a concrete scope — never default-deny, which would 403 a caller
	// the server told to come here.
	reachable := []struct{ method, path, why string }{
		{"GET", "/.well-known/oauth-authorization-server", "discovery, read before any credential exists"},
		{"POST", "/oauth/device", "device has no credential yet"},
		{"POST", "/oauth/token", "the grant itself is the credential"},
		{"POST", "/oauth/revoke", "RFC 7009: presenting the token is the authority"},
		{"GET", "/oauth/userinfo", "advertised in discovery; needs the profile scope"},
		{"GET", "/api/collections/users/records", "identity lookups by any token"},
		// A package's routes: its record collections, its stored files, its
		// bespoke endpoints. Each real package pins its own CLI-reachable
		// routes in its own tests; these are the fixture package's.
		{"GET", "/api/files/notes_items/rec123/body_ab12cd34ef.html", "stored files carry the read scope"},
		{"GET", "/api/collections/notes_folder_counts/records", "a read-only view"},
		{"GET", "/api/collections/labels/records", "a core collection a package claims"},
		{"POST", "/api/collections/labels/records", "a core collection a package claims"},
		{"DELETE", "/api/collections/labels/records/abc123", "a core collection a package claims"},
		{"GET", "/api/tasks/export", "a bespoke read endpoint"},
		{"POST", "/api/notes/items/abc123/move", "a per-record family"},
	}
	for _, r := range reachable {
		if len(ScopeForRoute(r.method, r.path)) == 0 {
			t.Errorf("%s %s falls into DEFAULT-DENY by omission (%s). Add it to "+
				"endpointScopes with the scope it needs, or to exemptPaths if it is "+
				"genuinely credential-less. Silent default-deny 403s only OAuth "+
				"callers, so handler unit tests still pass and only an integration notices.",
				r.method, r.path, r.why)
		}
	}

	// Routes that require an interactive session. Default-deny is the CORRECT
	// answer here — it is the outer half of the fix that stopped a profile-only
	// token from approving its own fully-scoped grant. Asserted positively so a
	// future exemption that re-opens the hole fails loudly.
	sessionOnly := []string{
		"/oauth/authorize", "/oauth/authorize/info",
		"/oauth/authorize/approve", "/oauth/authorize/deny",
		"/oauth/grants/abc123/revoke",
		// Client administration. Default-deny matters MORE here than on the
		// consent surfaces: these routes are the kill switch itself, so a
		// bearer token reaching them could disable whatever would detect it,
		// or re-enable itself after an admin switched it off.
		"/oauth/clients", "/oauth/clients/abc123/disabled",
	}
	for _, p := range sessionOnly {
		if got := ScopeForRoute("POST", p); len(got) != 0 {
			t.Errorf("POST %s resolved to %q, want default-deny: an OAuth bearer must "+
				"not reach a consent or management surface", p, got)
		}
	}

	// GET is classified independently of POST — the list endpoint is a GET,
	// and a read-side exemption would leak the client registry to any bearer.
	if got := ScopeForRoute("GET", "/oauth/clients"); len(got) != 0 {
		t.Errorf("GET /oauth/clients resolved to %q, want default-deny: the client "+
			"registry must not be readable with an OAuth access token", got)
	}
}

// The consent and management surfaces must NOT be exempt: they require an
// interactive session, and an exemption there is what let a profile-only token
// approve its own fully-scoped grant.
func TestConsentSurfacesAreNotExempt(t *testing.T) {
	for _, p := range []string{
		"/oauth/authorize", "/oauth/authorize/info",
		"/oauth/authorize/approve", "/oauth/authorize/deny",
		"/oauth/grants/abc123/revoke",
		"/oauth/clients", "/oauth/clients/abc123/disabled",
	} {
		if ScopeForRoute("POST", p).isExempt() {
			t.Errorf("%s must not be scope-exempt — an OAuth bearer would bypass the "+
				"scope ceiling and could approve a grant for itself", p)
		}
	}
}

// The credential-less endpoints must STAY exempt or the device flow cannot start.
func TestCredentiallessEndpointsStayExempt(t *testing.T) {
	for _, p := range []string{"/oauth/device", "/oauth/token", "/oauth/revoke"} {
		if !ScopeForRoute("POST", p).isExempt() {
			t.Errorf("%s must stay exempt — the caller has no credential yet", p)
		}
	}
}
