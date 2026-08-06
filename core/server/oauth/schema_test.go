package oauth

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newSchemaApp builds the oauth collections the way the migration does, so the
// package's own tests do not depend on the migration runner. There is no test
// that automatically keeps this in sync with the migration file — the two are
// mirrored by hand, so update both together when the schema changes. The
// migration itself is validated for real by the generator run against a live
// PocketBase DB.
func newSchemaApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	clients := core.NewBaseCollection(clientsCollection)
	clients.Fields.Add(&core.TextField{Name: "client_id", Required: true})
	clients.Fields.Add(&core.TextField{Name: "name", Required: true})
	clients.Fields.Add(&core.JSONField{Name: "redirect_uris"})
	clients.Fields.Add(&core.TextField{Name: "scopes"})
	clients.Fields.Add(&core.SelectField{
		Name: "type", Required: true, MaxSelect: 1,
		Values: []string{"public", "confidential"},
	})
	clients.Fields.Add(&core.TextField{Name: "client_secret_hash"})
	clients.Fields.Add(&core.BoolField{Name: "is_first_party"})
	clients.Fields.Add(&core.BoolField{Name: "disabled"})
	clients.AddIndex("idx_oauth_clients_client_id", true, "client_id", "")
	if err := app.Save(clients); err != nil {
		t.Fatalf("save oauth_clients: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}

	grants := core.NewBaseCollection(grantsCollection)
	// Deliberately NOT Required — a pending device grant has no user until
	// approval. See the migration's comment on og_user.
	grants.Fields.Add(&core.RelationField{
		Name: "user", Required: false, MaxSelect: 1,
		CollectionId: users.Id, CascadeDelete: true,
	})
	grants.Fields.Add(&core.RelationField{
		Name: "client", Required: true, MaxSelect: 1,
		CollectionId: clients.Id, CascadeDelete: true,
	})
	grants.Fields.Add(&core.TextField{Name: "jti"})
	grants.Fields.Add(&core.TextField{Name: "scopes"})
	// Hidden: true on all four credential-material fields — mirrors the
	// migration's hidden fields so this test schema's API-export behavior
	// (Record.PublicExport) matches production. user_code is a live,
	// guessable (~40-bit) credential while a device grant is pending; nothing
	// client-side reads it off a record (the consent screen only ever sends
	// the value the user typed/URL-supplied), so there is no reason to
	// serialize it.
	grants.Fields.Add(&core.TextField{Name: "refresh_token_hash", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "device_code", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "user_code", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "code_challenge"})
	grants.Fields.Add(&core.TextField{Name: "auth_code_hash", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "redirect_uri"})
	grants.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"pending", "active", "revoked"},
	})
	grants.Fields.Add(&core.DateField{Name: "expires_at"})
	grants.Fields.Add(&core.DateField{Name: "last_used_at"})
	grants.Fields.Add(&core.TextField{Name: "device_label"})
	grants.AddIndex("idx_oauth_grants_jti", true, "jti", "")
	grants.AddIndex("idx_oauth_grants_user", false, "user", "")
	// Partial UNIQUE, mirroring the migration: unique only while user_code is
	// live, since RevokeGrant/issueTokens clear it to '' and many rows
	// legitimately share that cleared value.
	grants.AddIndex("idx_oauth_grants_user_code", true, "user_code", "user_code != ''")
	grants.AddIndex("idx_oauth_grants_device_code", false, "device_code", "")
	grants.AddIndex("idx_oauth_grants_refresh_hash", false, "refresh_token_hash", "")
	if err := app.Save(grants); err != nil {
		t.Fatalf("save oauth_grants: %v", err)
	}

	return app
}

func TestOAuthCollectionsExist(t *testing.T) {
	app := newSchemaApp(t)

	for _, name := range []string{clientsCollection, grantsCollection} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("collection %s missing: %v", name, err)
		}
		// Writes must never be reachable through the record API: PocketBase
		// rules cannot constrain WHICH fields a write touches, which is why
		// users_guard.go exists. Minting and revoking go through Go handlers.
		if col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
			t.Errorf("%s: create/update/delete rules must be nil (superuser-only)", name)
		}
	}
}

// TestGrantUserCodeIsUniqueWhileLive is Finding 4's regression test: two
// PENDING grants must never be able to share a live user_code, or
// FindGrantByUserCode's lookup becomes ambiguous — a user could end up
// approving a different device than the one on their screen.
func TestGrantUserCodeIsUniqueWhileLive(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	first, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "pending")
	if err != nil {
		t.Fatalf("NewGrant (first): %v", err)
	}
	first.Set("user_code", "DUPE-CODE")
	if err := app.Save(first); err != nil {
		t.Fatalf("save first grant with user_code: %v", err)
	}

	second, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "pending")
	if err != nil {
		t.Fatalf("NewGrant (second): %v", err)
	}
	second.Set("user_code", "DUPE-CODE")
	if err := app.Save(second); err == nil {
		t.Fatal("a second pending grant must not be able to reuse a live user_code")
	}
}

// TestGrantUserCodeClearedValuesDoNotCollide proves the partial-index scope
// is correct in the other direction: many grants legitimately reach
// user_code = ” (RevokeGrant on deny/revocation, issueTokens on exchange),
// and the UNIQUE constraint must not block that — only a LIVE, non-empty
// user_code needs to be unique.
func TestGrantUserCodeClearedValuesDoNotCollide(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	for i := 0; i < 3; i++ {
		g, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "revoked")
		if err != nil {
			t.Fatalf("NewGrant %d: %v", i, err)
		}
		g.Set("user_code", "") // the cleared state every revoked/exchanged grant reaches
		if err := app.Save(g); err != nil {
			t.Fatalf("save grant %d with cleared user_code: %v", i, err)
		}
	}
}

func TestGrantJTIIsUnique(t *testing.T) {
	app := newSchemaApp(t)
	grants, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		t.Fatalf("find grants: %v", err)
	}

	var found bool
	for _, idx := range grants.Indexes {
		if strings.Contains(idx, "jti") && strings.Contains(idx, "UNIQUE") {
			found = true
		}
	}
	if !found {
		t.Fatal("oauth_grants needs a UNIQUE index on jti — it is the token→grant key")
	}
}

// TestGrantCredentialFieldsAreHiddenFromPublicExport is the actual proof that
// marking refresh_token_hash/device_code/auth_code_hash/user_code
// `hidden: true` closes the exposure: PublicExport is what every JSON API
// response (list, view, and realtime message) is built from, gated on
// !field.GetHidden(). A query-time .select() on the TS client narrows what
// the CLIENT reads out of its local store, but does nothing about what
// crosses the wire in the first place — getList/getFullList/realtime all
// fetch every visible field regardless of what the caller's query later
// projects. This test would fail if Hidden were ever removed from the
// migration (or from this test schema's mirror of it).
//
// user_code is hidden alongside the other three: it is a live, guessable
// (~40-bit) credential while a device grant is pending, and nothing
// client-side reads it off a record — the consent screen (app/p/oauth/
// authorize.tsx) only ever SENDS a value the user typed or that arrived in
// the URL, never reads one back from a query. The server looks pending
// grants up by it via FindGrantByUserCode, a DB filter that does not go
// through PublicExport. The one place a user_code IS returned to a client is
// DeviceResponse.UserCode in device.go — a hand-built JSON struct the CLI
// polling /oauth/device reads to show the user, unrelated to and unaffected
// by this collection field's Hidden flag.
func TestGrantCredentialFieldsAreHiddenFromPublicExport(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	// Populate every credential field with a recognizable, non-empty value so
	// a leak is unambiguous — an empty string field passing this check would
	// prove nothing.
	grant.Set("refresh_token_hash", "leak-would-show-this-rt-hash")
	grant.Set("device_code", "leak-would-show-this-device-code")
	grant.Set("auth_code_hash", "leak-would-show-this-auth-code-hash")
	grant.Set("user_code", "leak-would-show-this-user-code")
	grant.Set("device_label", "Nathan's laptop")
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant with credential material: %v", err)
	}

	export := grant.PublicExport()

	for _, field := range []string{"refresh_token_hash", "device_code", "auth_code_hash", "user_code"} {
		if _, present := export[field]; present {
			t.Errorf("PublicExport must not include %q — this is exactly what an API response and a realtime message send to every subscribed client", field)
		}
	}

	// device_label is a genuinely visible field (the Connected apps screen
	// reads it directly). Confirming it IS exported guards against this test
	// passing by accident — e.g. a broken PublicExport that hides everything
	// would make every assertion above pass for the wrong reason.
	if got, present := export["device_label"]; !present || got != "Nathan's laptop" {
		t.Errorf("device_label must remain visible in PublicExport (got %v, present=%v)", got, present)
	}
}
