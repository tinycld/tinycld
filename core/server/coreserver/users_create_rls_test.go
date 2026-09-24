package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

// The users collection kept PocketBase's default createRule of "" (public),
// so an anonymous POST could mint an account with role "owner" and pass every
// role check. Every real sign-up path (invite accept, setup bootstrap, demo
// start, the create-owner CLI, guest OTP) saves from Go, which ignores API
// rules, so the collection is superuser-only for REST creates.

const newUserBody = `{"email":"intruder@test.local","username":"intruder",` +
	`"name":"Intruder","password":"Password123!","passwordConfirm":"Password123!",` +
	`"role":"owner"}`

func runUsersCreateScenario(t *testing.T, app *tests.TestApp, token string, wantStatus int, wantContent []string) {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}
	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/users/records",
		Headers:               headers,
		Body:                  strings.NewReader(newUserBody),
		ExpectedStatus:        wantStatus,
		ExpectedContent:       wantContent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func requireNoIntruder(t *testing.T, app core.App) {
	t.Helper()
	if rec, _ := app.FindAuthRecordByEmail("users", "intruder@test.local"); rec != nil {
		t.Fatalf("users create was refused but the record exists: %s", rec.Id)
	}
}

func TestUsersCreate_AnonymousRefused(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	runUsersCreateScenario(t, env.app, "", http.StatusForbidden, []string{`"data":{}`})
	requireNoIntruder(t, env.app)
}

func TestUsersCreate_AuthenticatedUserRefused(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	runUsersCreateScenario(t, env.app, env.adminToken, http.StatusForbidden, []string{`"data":{}`})
	requireNoIntruder(t, env.app)
}

func TestUsersCreate_SuperuserAllowed(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	su, err := env.app.FindFirstRecordByFilter(core.CollectionNameSuperusers, "id != ''")
	if err != nil {
		t.Fatalf("find superuser: %v", err)
	}
	token, err := su.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	runUsersCreateScenario(t, env.app, token, http.StatusOK, []string{`"role":"owner"`})
}

func TestUsersCreate_ShippedRuleIsSuperuserOnly(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	if rule, ok := rlstest.Rule(t, env.app, "users", "create"); ok {
		t.Fatalf("users.createRule must be nil (superusers only); shipped rule: %q", rule)
	}
}
