package coreserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// Reuses setupGuardTestApp + makeUser/makeOrg/makeMembership from
// users_guard_test.go, which build the orgs/user_org schema this needs.

func newOrgPostEvent(app core.App, orgID string) *core.RequestEvent {
	req := httptest.NewRequest("POST", "/api/admin/orgs/"+orgID+"/impersonate", nil)
	req.SetPathValue("orgId", orgID)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

func TestOrgOwner(t *testing.T) {
	app := setupGuardTestApp(t)
	owner := makeUser(t, app, "owner@test.local")
	member := makeUser(t, app, "member@test.local")
	org := makeOrg(t, app, "Acme", "acme")
	makeMembership(t, app, member, org, "member")
	makeMembership(t, app, owner, org, "owner")

	got := orgOwner(app, org.Id)
	if got == nil || got.Id != owner.Id {
		t.Fatalf("orgOwner = %v, want owner %s", got, owner.Id)
	}

	// An org with no owner row resolves to nil.
	ownerless := makeOrg(t, app, "Empty", "empty")
	if orgOwner(app, ownerless.Id) != nil {
		t.Error("expected nil owner for an org with no owner membership")
	}
}

func TestHandleImpersonateOwner(t *testing.T) {
	app := setupGuardTestApp(t)
	owner := makeUser(t, app, "owner@test.local")
	org := makeOrg(t, app, "Acme", "acme")
	makeMembership(t, app, owner, org, "owner")

	re := newOrgPostEvent(app, org.Id)
	if err := handleImpersonateOwner(app, re); err != nil {
		t.Fatalf("impersonate: %v", err)
	}

	rec := re.Response.(*httptest.ResponseRecorder)
	var body struct {
		Token string        `json:"token"`
		Owner orgAdminOwner `json:"owner"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Token == "" {
		t.Error("expected a non-empty impersonation token")
	}
	if body.Owner.ID != owner.Id || body.Owner.Email != "owner@test.local" {
		t.Errorf("owner = %+v, want id=%s email=owner@test.local", body.Owner, owner.Id)
	}
}

func TestHandleImpersonateOwner_NoOwner(t *testing.T) {
	app := setupGuardTestApp(t)
	org := makeOrg(t, app, "Empty", "empty")

	if err := handleImpersonateOwner(app, newOrgPostEvent(app, org.Id)); err == nil {
		t.Fatal("expected an error impersonating an org with no owner")
	}
}
