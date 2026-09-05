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
		// text and calc contribute only comment commands — the documents
		// themselves are drive_items, reached through the drive scopes above.
		{"GET", "/api/collections/text_comments/records", "text comments list"},
		{"POST", "/api/collections/text_comments/records", "text comments --add"},
		{"PATCH", "/api/collections/text_comments/records/abc123", "text comments --resolve"},
		{"GET", "/api/collections/calc_comments/records", "calc comments list"},
		{"POST", "/api/collections/calc_comments/records", "calc comments --add"},
		{"PATCH", "/api/collections/calc_comments/records/abc123", "calc comments --resolve"},
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
		ScopeMailRead, ScopeDriveRead, ScopeContactsRead, ScopeCalendarRead, ScopeBoardsRead,
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

// Boards' search route shipped before any cards scope existed, so it was
// default-denied for every token — the route ran but no integration could call
// it. This pins the classification now that boards:read exists.
func TestCardsSearchIsClassified(t *testing.T) {
	rule := ScopeForRoute("GET", "/api/boards/search")
	if len(rule) == 0 {
		t.Fatal("GET /api/boards/search is default-denied")
	}
	if !rule.satisfiedBy([]string{ScopeBoardsRead}) {
		t.Error("boards:read must admit the cards search route")
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
		"boards_projects", "boards_lists", "boards_cards", "boards_labels",
		"boards_checklist_items", "boards_comments", "boards_attachments",
		// The junctions. boards_card_links spans two boards, which is why it
		// is worth naming here rather than assuming it rides along: its own
		// create rule demands write on the source and membership on the
		// target, so the scope grant adds nothing a caller could not already
		// do in the app.
		"boards_card_links", "boards_comment_reactions", "boards_card_watchers",
		// The planning collections. boards_epics shipped with no entry and
		// was default-denied for the life of the feature; naming it here is
		// what keeps the next collection from repeating that.
		"boards_epics", "boards_sprints",
	} {
		path := "/api/collections/" + collection + "/records"
		read := ScopeForRoute("GET", path)
		if !read.satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("GET %s: boards:read must admit a read (got %q)", path, read)
		}
		write := ScopeForRoute("POST", path)
		if !write.satisfiedBy([]string{ScopeBoardsWrite}) {
			t.Errorf("POST %s: boards:write must admit a write (got %q)", path, write)
		}
		// Read must not carry write. A token consented to read-only cards
		// access that could still POST would make the consent screen a lie.
		if read.satisfiedBy([]string{ScopeBoardsWrite}) && !read.satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("GET %s admits boards:write but not boards:read", path)
		}
		if write.satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("POST %s: boards:read alone must NOT admit a write", path)
		}
		if read.satisfiedBy([]string{ScopeMailRead}) {
			t.Errorf("GET %s: an unrelated package's scope must not admit it", path)
		}
	}
}

// The sharing surface is deliberately READ-ONLY for OAuth callers. A write to
// boards_project_members adds a person to a board; a write to boards_share_links
// mints a URL that opens the board to anyone holding it. Both are a
// categorically larger grant than editing cards, and "boards:write" on the
// consent screen does not read as "give other people my boards".
//
// Asserted positively so relaxing it is a deliberate edit to this test rather
// than an unnoticed side effect of touching the table.
func TestCardsSharingSurfaceIsReadOnlyForOAuth(t *testing.T) {
	for _, collection := range []string{"boards_project_members", "boards_share_links"} {
		path := "/api/collections/" + collection + "/records"
		if !ScopeForRoute("GET", path).satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("GET %s: boards:read must still admit reading the roster/links", path)
		}
		// Every write verb, not just POST: revoking a link is a DELETE and
		// changing a member's role is a PATCH, so a table entry that only
		// blocked creates would leave both open.
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			p := path
			if method != "POST" {
				p += "/abc123"
			}
			for _, scope := range []string{ScopeBoardsWrite, ScopeBoardsRead} {
				if ScopeForRoute(method, p).satisfiedBy([]string{scope}) {
					t.Errorf("%s %s must not be reachable with %q — an OAuth token "+
						"must not be able to reshare a board or mint a public link",
						method, p, scope)
				}
			}
		}
	}
}

// boards_activity is READ-ONLY, and unlike the sharing surface above that is
// not a policy judgement — it is the schema. Its create, update and delete
// rules are all nil (pb-migrations/1980000008), so history is written by the
// server alone and no client of any kind appends to it.
//
// Asserted separately from the sharing surface because the reasons differ and
// should not be conflated: relaxing the sharing entries would be a deliberate
// security decision, whereas granting write here would name a capability that
// does not exist and could never succeed.
//
// boards_sprint_snapshots is the same shape — a sprint's daily scope/done row,
// written by the server's sweep and lifecycle endpoints — and is asserted in
// the same loop for the same reason.
func TestCardsServerWrittenCollectionsAreReadOnlyForOAuth(t *testing.T) {
	for _, collection := range []string{"boards_activity", "boards_sprint_snapshots"} {
		path := "/api/collections/" + collection + "/records"

		if !ScopeForRoute("GET", path).satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("GET %s: boards:read must admit reading it", path)
		}
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			p := path
			if method != "POST" {
				p += "/abc123"
			}
			for _, scope := range []string{ScopeBoardsWrite, ScopeBoardsRead} {
				if ScopeForRoute(method, p).satisfiedBy([]string{scope}) {
					t.Errorf("%s %s must not be reachable with %q — the collection is "+
						"server-written and its write rules are nil", method, p, scope)
				}
			}
		}
	}
}

// Boards' per-record POST endpoints are classified by prefix, since an id sits
// in the middle of the path. The move endpoint shipped unclassified, so
// `card move --board` over an OAuth token was denied while a session succeeded
// — exactly the split the CLI's testserver cannot see. Both families are
// content writes the handler re-authorizes in Go, so boards:write is the grant
// and boards:read alone must not open them.
func TestCardsRecordEndpointsAreClassified(t *testing.T) {
	for _, path := range []string{
		"/api/boards/cards/abc123/move",
		"/api/boards/sprints/abc123/start",
		"/api/boards/sprints/abc123/complete",
	} {
		rule := ScopeForRoute("POST", path)
		if len(rule) == 0 {
			t.Errorf("POST %s is default-denied", path)
			continue
		}
		if !rule.satisfiedBy([]string{ScopeBoardsWrite}) {
			t.Errorf("POST %s: boards:write must admit it (got %q)", path, rule)
		}
		if rule.satisfiedBy([]string{ScopeBoardsRead}) {
			t.Errorf("POST %s: boards:read alone must NOT admit a write", path)
		}
	}
	// The bare family prefix is a different route and must not ride along.
	for _, path := range []string{"/api/boards/cards/", "/api/boards/sprints/"} {
		if len(ScopeForRoute("POST", path)) != 0 {
			t.Errorf("POST %s: the bare prefix must stay default-denied", path)
		}
	}
}

// Boards' file transfer routes, classified exactly as contacts' and calendar's
// are: the export reads a whole board, the import writes one.
//
// The read/write split is the assertion that matters. An export handed
// boards:write would be reachable by a token a user granted only to change
// cards, and an import admitted by boards:read alone would let a read-only
// integration create a board full of content.
func TestBoardsTransferEndpointsAreClassified(t *testing.T) {
	export := ScopeForRoute("GET", "/api/boards/export")
	if len(export) == 0 {
		t.Fatal("GET /api/boards/export is default-denied")
	}
	if !export.satisfiedBy([]string{ScopeBoardsRead}) {
		t.Errorf("GET /api/boards/export: boards:read must admit it (got %q)", export)
	}

	imp := ScopeForRoute("POST", "/api/boards/import")
	if len(imp) == 0 {
		t.Fatal("POST /api/boards/import is default-denied")
	}
	if !imp.satisfiedBy([]string{ScopeBoardsWrite}) {
		t.Errorf("POST /api/boards/import: boards:write must admit it (got %q)", imp)
	}
	if imp.satisfiedBy([]string{ScopeBoardsRead}) {
		t.Error("POST /api/boards/import: boards:read alone must NOT admit a write")
	}
}
