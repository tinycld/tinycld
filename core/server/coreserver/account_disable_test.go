package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// Disable is the reversible counterpart to delete: the row, its authored
// content and every FK survive, and an admin can restore access. These tests
// pin the three properties that make it a suspension rather than a soft
// delete-in-disguise — the flag is set, sessions die immediately, and only an
// admin can clear it.

// newDisableApp wires the account routes plus the auth guard onto a test app
// with a `disabled` field on users (NewTestApp ships the stock collection).
func newDisableApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	if err := app.Save(users); err != nil {
		t.Fatalf("add users fields: %v", err)
	}
	registerAccountDeleteCore(app)
	registerDisabledUserGuardCore(app)
	return app
}

func disableTestUser(t testing.TB, app core.App, email, role string) *core.Record {
	t.Helper()
	r, err := createTestUser(app, email, "Password123!")
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	if role != "" {
		r.Set("role", role)
		if err := app.Save(r); err != nil {
			t.Fatalf("set role: %v", err)
		}
	}
	return r
}

func TestAccountDisable_SetsFlagAndKillsSessions(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	user := disableTestUser(t, app, "quitter@test.local", "member")
	tokenKeyBefore := user.GetString("tokenKey")
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "disable own account",
		Method:                http.MethodPost,
		URL:                   "/api/account/disable",
		Body:                  strings.NewReader(`{"email":"quitter@test.local"}`),
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusNoContent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			updated, err := app.FindRecordById("users", user.Id)
			if err != nil {
				t.Fatalf("FindRecordById: %v", err)
			}
			if !updated.GetBool("disabled") {
				t.Error("disabled flag not set")
			}
			// The row and its identity survive — that is what makes this
			// reversible, and distinguishes it from delete.
			if updated.GetString("email") != "quitter@test.local" {
				t.Errorf("email should be untouched, got %q", updated.GetString("email"))
			}
			// RefreshTokenKey must have rotated, or the caller's existing JWT
			// would keep working until it expired and the suspension would be
			// advisory for hours.
			if updated.GetString("tokenKey") == tokenKeyBefore {
				t.Error("tokenKey not rotated — existing sessions would survive")
			}
		},
	}
	scenario.Test(t)
}

func TestAccountDisable_RequiresEmailMatch(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	user := disableTestUser(t, app, "me@test.local", "member")
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "wrong email rejected",
		Method:                http.MethodPost,
		URL:                   "/api/account/disable",
		Body:                  strings.NewReader(`{"email":"wrong@test.local"}`),
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestAccountEnable_AdminOnly is the guard that keeps disable meaningful: a
// peer (or the subject, if they still held a token) must not be able to lift
// a suspension.
func TestAccountEnable_AdminOnly(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	suspended := disableTestUser(t, app, "suspended@test.local", "member")
	suspended.Set("disabled", true)
	if err := app.Save(suspended); err != nil {
		t.Fatal(err)
	}

	peer := disableTestUser(t, app, "peer@test.local", "member")
	peerToken, err := peer.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "member cannot enable",
		Method:                http.MethodPost,
		URL:                   "/api/account/enable",
		Body:                  strings.NewReader(`{"user_id":"` + suspended.Id + `"}`),
		Headers:               map[string]string{"Authorization": peerToken},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			still, err := app.FindRecordById("users", suspended.Id)
			if err != nil {
				t.Fatal(err)
			}
			if !still.GetBool("disabled") {
				t.Error("a non-admin cleared the disabled flag")
			}
		},
	}
	scenario.Test(t)
}

func TestAccountEnable_AdminRestores(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	suspended := disableTestUser(t, app, "suspended2@test.local", "member")
	suspended.Set("disabled", true)
	if err := app.Save(suspended); err != nil {
		t.Fatal(err)
	}

	admin := disableTestUser(t, app, "admin@test.local", "admin")
	adminToken, err := admin.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "admin enables",
		Method:                http.MethodPost,
		URL:                   "/api/account/enable",
		Body:                  strings.NewReader(`{"user_id":"` + suspended.Id + `"}`),
		Headers:               map[string]string{"Authorization": adminToken},
		ExpectedStatus:        http.StatusNoContent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			restored, err := app.FindRecordById("users", suspended.Id)
			if err != nil {
				t.Fatal(err)
			}
			if restored.GetBool("disabled") {
				t.Error("admin enable did not clear the flag")
			}
		},
	}
	scenario.Test(t)
}

// TestDisabledUser_CannotAuthenticate covers the password sign-in path. The
// account is otherwise valid — correct password, verified — so a pass here
// means the guard, not a credential failure, is doing the rejecting.
func TestDisabledUser_CannotAuthenticate(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	user := disableTestUser(t, app, "locked@test.local", "member")
	user.Set("disabled", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "disabled password login refused",
		Method:                http.MethodPost,
		URL:                   "/api/collections/users/auth-with-password",
		Body:                  strings.NewReader(`{"identity":"locked@test.local","password":"Password123!"}`),
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{"disabled"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestDisabledUser_CannotRefreshSession is the reason the guard binds two
// hooks. A user disabled mid-session still holds a valid JWT; without the
// OnRecordAuthRequest binding they could renew it indefinitely.
func TestDisabledUser_CannotRefreshSession(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	user := disableTestUser(t, app, "midsession@test.local", "member")
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	// Disable WITHOUT rotating tokenKey, so the token stays cryptographically
	// valid — isolating the guard from the token-rotation belt-and-braces.
	user.Set("disabled", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:                  "disabled auth-refresh refused",
		Method:                http.MethodPost,
		URL:                   "/api/collections/users/auth-refresh",
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{"disabled"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEnabledUser_CanStillAuthenticate is the positive control. Without it a
// guard that rejected EVERY login would pass every test above.
//
// The stock test users collection has MFA enabled, so a *successful* first
// factor answers 401 + {"mfaId":...} rather than a token. That response is
// itself the proof we need: the request reached PocketBase's auth flow instead
// of being turned away by the guard, which answers 403 + "disabled".
func TestEnabledUser_CanStillAuthenticate(t *testing.T) {
	app := newDisableApp(t)
	defer app.Cleanup()

	disableTestUser(t, app, "active@test.local", "member")

	scenario := &tests.ApiScenario{
		Name:                  "active user passes the guard",
		Method:                http.MethodPost,
		URL:                   "/api/collections/users/auth-with-password",
		Body:                  strings.NewReader(`{"identity":"active@test.local","password":"Password123!"}`),
		ExpectedStatus:        http.StatusUnauthorized,
		ExpectedContent:       []string{`"mfaId"`},
		NotExpectedContent:    []string{"disabled"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}
