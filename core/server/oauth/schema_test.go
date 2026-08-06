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
	// Hidden: true on the three credential-material fields — mirrors the
	// migration's hidden fields so this test schema's API-export behavior
	// (Record.PublicExport) matches production. user_code stays visible: the
	// consent screen looks a pending grant up by it, and it's short-lived,
	// single-use, human-typed.
	grants.Fields.Add(&core.TextField{Name: "refresh_token_hash", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "device_code", Hidden: true})
	grants.Fields.Add(&core.TextField{Name: "user_code"})
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
// marking refresh_token_hash/device_code/auth_code_hash `hidden: true` closes
// the exposure: PublicExport is what every JSON API response (list, view, and
// realtime message) is built from, gated on !field.GetHidden(). A query-time
// .select() on the TS client narrows what the CLIENT reads out of its local
// store, but does nothing about what crosses the wire in the first place —
// getList/getFullList/realtime all fetch every visible field regardless of
// what the caller's query later projects. This test would fail if Hidden were
// ever removed from the migration (or from this test schema's mirror of it).
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
	grant.Set("user_code", "WDJB-MJHT")
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant with credential material: %v", err)
	}

	export := grant.PublicExport()

	for _, field := range []string{"refresh_token_hash", "device_code", "auth_code_hash"} {
		if _, present := export[field]; present {
			t.Errorf("PublicExport must not include %q — this is exactly what an API response and a realtime message send to every subscribed client", field)
		}
	}

	// user_code is deliberately NOT hidden: the browser consent screen looks a
	// pending grant up by it (handleAuthorizeInfo/handleApproveDevice), and
	// it's short-lived, single-use, human-typed — hiding it would break that
	// flow. Confirm it's still exported, so this test can't pass by accident
	// (e.g. from a broken PublicExport that hides everything).
	if got, present := export["user_code"]; !present || got != "WDJB-MJHT" {
		t.Errorf("user_code must remain visible in PublicExport (got %v, present=%v)", got, present)
	}
}
