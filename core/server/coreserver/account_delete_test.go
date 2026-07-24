package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// The pb_test_data directory is intentionally not committed (it's a runtime
// fixture for the e2e suite living under tinycld/server/), so use the
// programmatic test app instead. Single-org: account delete just anonymizes
// the caller's users record, so the bundled users collection is enough.
func newAccountDeleteApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	registerAccountDeleteCore(app)
	return app
}

func createTestUser(app core.App, email, password string) (*core.Record, error) {
	coll, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(coll)
	r.SetEmail(email)
	r.Set("name", "Test User")
	r.SetVerified(true)
	r.SetPassword(password)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	return r, nil
}

func TestAccountDeleteSoft(t *testing.T) {
	app := newAccountDeleteApp(t)
	defer app.Cleanup()

	user, err := createTestUser(app, "goodbye@test.local", "Password123!")
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	scenario := &tests.ApiScenario{
		Name:           "delete own account",
		Method:         http.MethodPost,
		URL:            "/api/account/delete",
		Body:           strings.NewReader(`{"email":"goodbye@test.local"}`),
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			updated, err := app.FindRecordById("users", user.Id)
			if err != nil {
				t.Fatalf("FindRecordById: %v", err)
			}
			if !strings.HasSuffix(updated.GetString("email"), "@deleted.tinycld.org") {
				t.Errorf("email not anonymized: %q", updated.GetString("email"))
			}
			if updated.GetString("name") != "Deleted user" {
				t.Errorf("name not anonymized: %q", updated.GetString("name"))
			}
			if updated.GetBool("verified") {
				t.Error("verified not cleared")
			}
		},
	}
	scenario.Test(t)
}

func TestAccountDeleteRequiresEmailMatch(t *testing.T) {
	app := newAccountDeleteApp(t)
	defer app.Cleanup()

	user, err := createTestUser(app, "me@test.local", "Password123!")
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Name:            "wrong email rejected",
		Method:          http.MethodPost,
		URL:             "/api/account/delete",
		Body:            strings.NewReader(`{"email":"wrong@test.local"}`),
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusBadRequest,
		ExpectedContent: []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestAccountDeleteRequiresAuth(t *testing.T) {
	scenario := &tests.ApiScenario{
		Name:            "no auth rejected",
		Method:          http.MethodPost,
		URL:             "/api/account/delete",
		Body:            strings.NewReader(`{"email":"x@test.local"}`),
		ExpectedStatus:  http.StatusUnauthorized,
		ExpectedContent: []string{`"message"`},
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			return newAccountDeleteApp(t)
		},
	}
	scenario.Test(t)
}
