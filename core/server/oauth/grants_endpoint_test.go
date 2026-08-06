package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// newRevokeRequestEvent builds a RequestEvent for POST /oauth/grants/{id}/revoke
// as handleRevokeGrantByID reads it directly (bypassing the router, same
// convention as the rest of this package's tests): PathValue must be set
// explicitly since no router is running to parse the {id} segment.
func newRevokeRequestEvent(app core.App, grantID string, auth *core.Record) *core.RequestEvent {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/grants/"+grantID+"/revoke", nil)
	req.SetPathValue("id", grantID)

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = auth
	return re
}

func TestRevokeGrantByIDRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	re := newRevokeRequestEvent(app, grant.Id, nil) // anonymous

	err = handleRevokeGrantByID(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller must not be able to revoke a grant (status = %d, err = %v)", status, err)
	}
}

func TestRevokeGrantByIDNotFound(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	re := newRevokeRequestEvent(app, "does-not-exist", user)

	err = handleRevokeGrantByID(app, re)
	if status := apiStatus(err); status != http.StatusNotFound {
		t.Fatalf("a nonexistent grant id must 404 (status = %d, err = %v)", status, err)
	}
}

// TestRevokeGrantByIDRejectsOtherUsersGrant is THE security property of this
// endpoint: without the grant.user != re.Auth.Id check, any signed-in user
// could revoke anyone else's grant by guessing or enumerating ids. Two
// distinct users are seeded so the check is exercised for real, not just
// against a zero-value/empty owner.
func TestRevokeGrantByIDRejectsOtherUsersGrant(t *testing.T) {
	app := newSchemaApp(t)
	ownerID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, ownerID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	attacker := core.NewRecord(users)
	attacker.Set("email", "mallory@example.com")
	attacker.Set("password", "s3cret-password")
	if err := app.Save(attacker); err != nil {
		t.Fatalf("save attacker: %v", err)
	}

	re := newRevokeRequestEvent(app, grant.Id, attacker)

	err = handleRevokeGrantByID(app, re)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("a user must not be able to revoke another user's grant (status = %d, err = %v)", status, err)
	}

	// The grant must be untouched: still active, not revoked by the rejected attempt.
	reloaded, err := app.FindRecordById(grantsCollection, grant.Id)
	if err != nil {
		t.Fatalf("reload grant: %v", err)
	}
	if reloaded.GetString("status") != "active" {
		t.Fatalf("a forbidden revoke attempt must not change grant status; got %q", reloaded.GetString("status"))
	}
}

func TestRevokeGrantByIDRevokesOwnGrant(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	re := newRevokeRequestEvent(app, grant.Id, user)

	if err := handleRevokeGrantByID(app, re); err != nil {
		t.Fatalf("handleRevokeGrantByID: %v", err)
	}
	if re.Response.(*httptest.ResponseRecorder).Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", re.Response.(*httptest.ResponseRecorder).Code)
	}

	reloaded, err := app.FindRecordById(grantsCollection, grant.Id)
	if err != nil {
		t.Fatalf("reload grant: %v", err)
	}
	if reloaded.GetString("status") != "revoked" {
		t.Fatalf("status = %q, want revoked", reloaded.GetString("status"))
	}
	if _, err := VerifyGrant(app, grant.GetString("jti")); err == nil {
		t.Fatal("the grant must not verify after revocation")
	}
}
