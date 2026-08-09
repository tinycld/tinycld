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
		// The CLI's read paths: `mail read` fetches the body_html file the
		// record points at, `drive get` fetches drive content, `mail status`
		// reads the folder-counts view, and send/list resolve the caller's
		// mailbox memberships.
		{"GET", "/api/files/mail_messages/rec123/body_ab12cd34ef.html", "mail bodies are file fields"},
		{"GET", "/api/files/drive_items/rec123/report_ab12cd34ef.pdf", "drive content is a file field"},
		{"GET", "/api/collections/mail_folder_counts/records", "unread counts view"},
		{"GET", "/api/collections/mail_mailbox_members/records", "mailbox membership resolution"},
		// The CLI's extended surface: From-identity resolution, label
		// add/remove, version history, and share links.
		{"GET", "/api/collections/mail_mailbox_aliases/records", "send --from identities"},
		{"GET", "/api/collections/labels/records", "mail label list"},
		{"POST", "/api/collections/label_assignments/records", "mail label add"},
		{"DELETE", "/api/collections/label_assignments/records/abc123", "mail label remove"},
		{"GET", "/api/collections/drive_item_versions/records", "drive versions list"},
		{"POST", "/api/drive/share-link", "drive link create"},
		{"GET", "/api/drive/share-links", "drive link list"},
		{"DELETE", "/api/drive/share-link/abc123", "drive link revoke"},
		{"POST", "/api/drive/versions/restore", "drive versions --restore"},
		{"POST", "/api/drive/versions/snapshot", "drive versions --snapshot"},
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

// The federated search endpoint self-filters: it drops sources the caller's
// grant does not cover and returns the rest. So it must admit ANY read scope —
// classifying it under one package's scope would 403 a token that legitimately
// holds a different one, instead of handing it the subset it may see.
func TestFederatedSearchAdmitsAnyReadScope(t *testing.T) {
	rule := ScopeForRoute("GET", "/api/search")
	if len(rule) == 0 {
		t.Fatal("GET /api/search falls into default-deny — no token could search at all")
	}
	for _, scope := range []string{
		ScopeMailRead, ScopeDriveRead, ScopeContactsRead, ScopeCalendarRead, ScopeCardsRead,
	} {
		if !rule.satisfiedBy([]string{scope}) {
			t.Errorf("a token holding only %q cannot reach /api/search", scope)
		}
	}
	// A write-only or profile-only grant has nothing to read, so it must not
	// reach the endpoint at all.
	for _, scope := range []string{ScopeProfile, ScopeMailSend, ScopeDriveWrite} {
		if rule.satisfiedBy([]string{scope}) {
			t.Errorf("%q alone must not admit a search", scope)
		}
	}
}

// Cards' search route shipped before any cards scope existed, so it was
// default-denied for every token — the route ran but no integration could call
// it. This pins the classification now that cards:read exists.
func TestCardsSearchIsClassified(t *testing.T) {
	rule := ScopeForRoute("GET", "/api/cards/search")
	if len(rule) == 0 {
		t.Fatal("GET /api/cards/search is default-denied")
	}
	if !rule.satisfiedBy([]string{ScopeCardsRead}) {
		t.Error("cards:read must admit the cards search route")
	}
	if rule.satisfiedBy([]string{ScopeMailRead}) {
		t.Error("an unrelated package's scope must not admit cards search")
	}
}

// The board-content collections the cards CLI drives. Every one of these was
// default-denied until the cards commands needed them, which is the failure
// mode TestEveryRegisteredRouteIsClassified exists to catch one layer up: the
// rules admit the caller, the handler runs in tests, and only a real OAuth
// client sees the 403.
func TestCardsContentCollectionsAreReadWrite(t *testing.T) {
	// One representative verb per side. The table is per-collection, so a
	// missing entry fails read and write together.
	for _, collection := range []string{
		"cards_projects", "cards_lists", "cards_cards", "cards_labels",
		"cards_checklist_items", "cards_comments", "cards_attachments",
	} {
		path := "/api/collections/" + collection + "/records"
		read := ScopeForRoute("GET", path)
		if !read.satisfiedBy([]string{ScopeCardsRead}) {
			t.Errorf("GET %s: cards:read must admit a read (got %q)", path, read)
		}
		write := ScopeForRoute("POST", path)
		if !write.satisfiedBy([]string{ScopeCardsWrite}) {
			t.Errorf("POST %s: cards:write must admit a write (got %q)", path, write)
		}
		// Read must not carry write. A token consented to read-only cards
		// access that could still POST would make the consent screen a lie.
		if read.satisfiedBy([]string{ScopeCardsWrite}) && !read.satisfiedBy([]string{ScopeCardsRead}) {
			t.Errorf("GET %s admits cards:write but not cards:read", path)
		}
		if write.satisfiedBy([]string{ScopeCardsRead}) {
			t.Errorf("POST %s: cards:read alone must NOT admit a write", path)
		}
		if read.satisfiedBy([]string{ScopeMailRead}) {
			t.Errorf("GET %s: an unrelated package's scope must not admit it", path)
		}
	}
}

// The sharing surface is deliberately READ-ONLY for OAuth callers. A write to
// cards_project_members adds a person to a board; a write to cards_share_links
// mints a URL that opens the board to anyone holding it. Both are a
// categorically larger grant than editing cards, and "cards:write" on the
// consent screen does not read as "give other people my boards".
//
// Asserted positively so relaxing it is a deliberate edit to this test rather
// than an unnoticed side effect of touching the table.
func TestCardsSharingSurfaceIsReadOnlyForOAuth(t *testing.T) {
	for _, collection := range []string{"cards_project_members", "cards_share_links"} {
		path := "/api/collections/" + collection + "/records"
		if !ScopeForRoute("GET", path).satisfiedBy([]string{ScopeCardsRead}) {
			t.Errorf("GET %s: cards:read must still admit reading the roster/links", path)
		}
		// Every write verb, not just POST: revoking a link is a DELETE and
		// changing a member's role is a PATCH, so a table entry that only
		// blocked creates would leave both open.
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			p := path
			if method != "POST" {
				p += "/abc123"
			}
			for _, scope := range []string{ScopeCardsWrite, ScopeCardsRead} {
				if ScopeForRoute(method, p).satisfiedBy([]string{scope}) {
					t.Errorf("%s %s must not be reachable with %q — an OAuth token "+
						"must not be able to reshare a board or mint a public link",
						method, p, scope)
				}
			}
		}
	}
}
