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

// TestHandleGrantSuperAdmin_AsSuperuserGrantor is the regression guard for the
// grant 500: when the request is authenticated as a PB superuser, re.Auth is
// non-nil (its id lives in _superusers, NOT users), so blindly stamping it into
// created_by — a relation into users — fails relation validation and 500s. The
// handler must skip created_by for a superuser grantor. The /admin console logs
// in as the PB superuser, so this is the common path the bug report hit.
func TestHandleGrantSuperAdmin_AsSuperuserGrantor(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	user := newGuardUser(t, app, "grantedbysuper@test.local")

	re := newJSONRequestEvent(app, "POST", `{"email":"grantedbysuper@test.local"}`)
	re.Auth = newSuperuserRecord(t, app, "granting-super@test.local")

	if err := handleGrantSuperAdmin(app, re); err != nil {
		t.Fatalf("grant by a superuser grantor returned error (the 500 regression): %v", err)
	}
	if !isSuperAdmin(app, user.Id) {
		t.Fatal("user should be a super admin after a superuser-initiated grant")
	}

	// created_by must be empty — the superuser's id is not a valid users relation.
	rec, err := app.FindFirstRecordByFilter(
		"super_admins", "user = {:user}", map[string]any{"user": user.Id})
	if err != nil {
		t.Fatalf("find granted row: %v", err)
	}
	if got := rec.GetString("created_by"); got != "" {
		t.Fatalf("created_by should be empty for a superuser grantor, got %q", got)
	}
}

// TestHandleGrantSuperAdmin_AsAppUserGrantor pins the complementary case: an
// app-user grantor (an existing super admin minting another) IS recorded in
// created_by, since their id is a valid users relation.
func TestHandleGrantSuperAdmin_AsAppUserGrantor(t *testing.T) {
	app, _ := newSuperAdminsTestApp(t)
	grantor := newGuardUser(t, app, "appgrantor@test.local")
	grantSuperAdmin(t, app, grantor.Id)
	target := newGuardUser(t, app, "appgranted@test.local")

	re := newJSONRequestEvent(app, "POST", `{"email":"appgranted@test.local"}`)
	re.Auth = grantor

	if err := handleGrantSuperAdmin(app, re); err != nil {
		t.Fatalf("grant by an app-user grantor returned error: %v", err)
	}

	rec, err := app.FindFirstRecordByFilter(
		"super_admins", "user = {:user}", map[string]any{"user": target.Id})
	if err != nil {
		t.Fatalf("find granted row: %v", err)
	}
	if got := rec.GetString("created_by"); got != grantor.Id {
		t.Fatalf("created_by = %q, want grantor id %q", got, grantor.Id)
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
