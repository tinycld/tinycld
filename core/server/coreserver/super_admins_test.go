package coreserver

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newJSONRequestEvent builds a RequestEvent carrying a JSON body, for the
// grant/list handlers (which decode re.Request.Body and write to re.Response).
func newJSONRequestEvent(app core.App, method, body string) *core.RequestEvent {
	req := httptest.NewRequest(method, "/api/admin/super-admins", strings.NewReader(body))
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

// recorderBody returns the response recorder's written body as a string.
func recorderBody(re *core.RequestEvent) string {
	return re.Response.(*httptest.ResponseRecorder).Body.String()
}

func newSuperAdminsTestApp(t *testing.T) (core.App, string) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)
	return app, users.Id
}

func TestHandleGrantSuperAdmin_ByID(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "grantme@test.local")

	re := newJSONRequestEvent(app, "POST", `{"userId":"`+user.Id+`"}`)
	if err := handleGrantSuperAdmin(app, re); err != nil {
		t.Fatalf("grant returned error: %v", err)
	}
	if !isSuperAdmin(app, user.Id) {
		t.Fatal("user should be a super admin after grant")
	}
}

func TestHandleGrantSuperAdmin_ByEmail(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "byemail@test.local")

	re := newJSONRequestEvent(app, "POST", `{"email":"byemail@test.local"}`)
	if err := handleGrantSuperAdmin(app, re); err != nil {
		t.Fatalf("grant by email returned error: %v", err)
	}
	if !isSuperAdmin(app, user.Id) {
		t.Fatal("user should be a super admin after grant by email")
	}
}

func TestHandleGrantSuperAdmin_DuplicateRejected(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "dupe@test.local")
	grantSuperAdmin(t, app, user.Id)

	re := newJSONRequestEvent(app, "POST", `{"userId":"`+user.Id+`"}`)
	err := handleGrantSuperAdmin(app, re)
	if err == nil {
		t.Fatal("expected a duplicate-grant error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already") {
		t.Fatalf("expected an 'already a super admin' error, got: %v", err)
	}
}

func TestHandleGrantSuperAdmin_UnknownUser(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	re := newJSONRequestEvent(app, "POST", `{"userId":"does-not-exist"}`)
	if err := handleGrantSuperAdmin(app, re); err == nil {
		t.Fatal("expected an error for an unknown user id")
	}
}

func TestHandleRevokeSuperAdmin(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "revoke@test.local")
	grantSuperAdmin(t, app, user.Id)
	if !isSuperAdmin(app, user.Id) {
		t.Fatal("precondition: user should be a super admin")
	}

	req := httptest.NewRequest("DELETE", "/api/admin/super-admins/"+user.Id, nil)
	req.SetPathValue("userId", user.Id)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()

	if err := handleRevokeSuperAdmin(app, re); err != nil {
		t.Fatalf("revoke returned error: %v", err)
	}
	if isSuperAdmin(app, user.Id) {
		t.Fatal("user should NOT be a super admin after revoke")
	}
}

func TestHandleRevokeSuperAdmin_NotAMember(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "notmember@test.local")

	req := httptest.NewRequest("DELETE", "/api/admin/super-admins/"+user.Id, nil)
	req.SetPathValue("userId", user.Id)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()

	if err := handleRevokeSuperAdmin(app, re); err == nil {
		t.Fatal("expected a not-a-super-admin error when revoking a non-member")
	}
}

func TestHandleListSuperAdmins_ExpandsNameAndEmail(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "listed@test.local")
	user.Set("name", "Listed User")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	grantSuperAdmin(t, app, user.Id)

	re := newJSONRequestEvent(app, "GET", "")
	if err := handleListSuperAdmins(app, re); err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	var resp struct {
		SuperAdmins []superAdminRow `json:"superAdmins"`
	}
	if err := json.Unmarshal([]byte(recorderBody(re)), &resp); err != nil {
		t.Fatalf("decode list response: %v (body=%s)", err, recorderBody(re))
	}
	if len(resp.SuperAdmins) != 1 {
		t.Fatalf("want 1 super admin, got %d", len(resp.SuperAdmins))
	}
	row := resp.SuperAdmins[0]
	if row.UserID != user.Id || row.Email != "listed@test.local" || row.Name != "Listed User" {
		t.Fatalf("row not expanded correctly: %+v", row)
	}
}

func TestResolveGrantTarget_PrefersIDThenEmail(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "resolve@test.local")

	got, err := resolveGrantTarget(app, grantSuperAdminRequest{UserID: user.Id})
	if err != nil || got.Id != user.Id {
		t.Fatalf("resolve by id failed: got=%v err=%v", got, err)
	}
	got, err = resolveGrantTarget(app, grantSuperAdminRequest{Email: "resolve@test.local"})
	if err != nil || got.Id != user.Id {
		t.Fatalf("resolve by email failed: got=%v err=%v", got, err)
	}
	// When BOTH an id and a (different user's) email are supplied, the id must
	// win — resolveGrantTarget checks UserID before Email. Pin that precedence,
	// since the test name promises it.
	other := newGuardUser(t, app, "other@test.local")
	got, err = resolveGrantTarget(app, grantSuperAdminRequest{UserID: user.Id, Email: "other@test.local"})
	if err != nil {
		t.Fatalf("resolve by id+email errored: %v", err)
	}
	if got.Id != user.Id {
		t.Fatalf("id should win over email: got %s (other=%s), want %s", got.Id, other.Id, user.Id)
	}
	if _, err := resolveGrantTarget(app, grantSuperAdminRequest{}); err == nil {
		t.Fatal("resolve with neither id nor email should error")
	}
}
