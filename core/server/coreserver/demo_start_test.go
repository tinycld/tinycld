package coreserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupDemoStartTestApp builds a TestApp with the minimum schema
// RegisterDemoStart touches: users.is_demo + users.role. Single-org: there is
// no orgs/user_org collection to build.
func setupDemoStartTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	changed := false
	if users.Fields.GetByName("is_demo") == nil {
		users.Fields.Add(&core.BoolField{Name: "is_demo"})
		changed = true
	}
	if users.Fields.GetByName("role") == nil {
		users.Fields.Add(&core.SelectField{
			Name:      "role",
			Values:    []string{"owner", "admin", "member", "guest"},
			MaxSelect: 1,
		})
		changed = true
	}
	if changed {
		if err := app.Save(users); err != nil {
			t.Fatalf("save users schema: %v", err)
		}
	}

	registerDemoStartCore(app)
	return app
}

// TestDemoStartCreatesUser covers the cold-start path: no demo user exists.
// After one POST the endpoint must return a PocketBase auth response and the
// user must exist with is_demo=true and role=owner.
func TestDemoStartCreatesUser(t *testing.T) {
	app := setupDemoStartTestApp(t)

	scenario := &tests.ApiScenario{
		Name:                  "cold start creates demo identity",
		Method:                http.MethodPost,
		URL:                   "/api/demo/start",
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"token":`, `"record":`, `"is_demo":true`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			user, err := app.FindAuthRecordByEmail("users", demoUserEmail)
			if err != nil {
				t.Fatalf("demo user not created: %v", err)
			}
			if !user.GetBool("is_demo") {
				t.Error("is_demo flag not set on created demo user")
			}
			if user.GetString("role") != "owner" {
				t.Errorf("expected role=owner, got %q", user.GetString("role"))
			}
		},
	}
	scenario.Test(t)
}

// TestDemoStartIsIdempotent covers the warm path: a demo user already exists.
// The endpoint must not create duplicates and must still return a valid auth
// token. Idempotency matters because the marketing CTA can fire repeatedly
// (browser back, double-click, retry on flaky network).
func TestDemoStartIsIdempotent(t *testing.T) {
	app := setupDemoStartTestApp(t)

	// One pass through the HTTP endpoint proves the route returns a valid
	// auth response (token + record).
	scenario := &tests.ApiScenario{
		Name:                  "first call returns auth",
		Method:                http.MethodPost,
		URL:                   "/api/demo/start",
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"token":`, `"record":`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)

	// Idempotency is a property of ensureDemoUser against the DB, not of the
	// HTTP routing. Exercise it by invoking that logic directly twice more on
	// the same app/DB.
	for i := 0; i < 2; i++ {
		if err := app.RunInTransaction(func(txApp core.App) error {
			_, err := ensureDemoUser(txApp)
			return err
		}); err != nil {
			t.Fatalf("repeat ensureDemoUser (iteration %d): %v", i, err)
		}
	}

	users, err := app.FindRecordsByFilter(
		"users",
		"email = {:email}",
		"-id", 0, 0,
		dbx.Params{"email": demoUserEmail},
	)
	if err != nil {
		t.Fatalf("FindRecordsByFilter users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected exactly 1 demo user, got %d", len(users))
	}
}

// TestDemoStartReturnsValidAuthToken confirms the response is shaped like a
// PocketBase auth response — the front-end depends on this exact shape so it
// can drop the result straight into pb.authStore via importAuth.
func TestDemoStartReturnsValidAuthToken(t *testing.T) {
	app := setupDemoStartTestApp(t)

	scenario := &tests.ApiScenario{
		Name:                  "auth response shape",
		Method:                http.MethodPost,
		URL:                   "/api/demo/start",
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"token":`, `"record":`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			var payload struct {
				Token  string `json:"token"`
				Record struct {
					ID     string `json:"id"`
					Email  string `json:"email"`
					IsDemo bool   `json:"is_demo"`
				} `json:"record"`
			}
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload.Token == "" {
				t.Error("empty token")
			}
			if !strings.Contains(payload.Token, ".") {
				t.Errorf("token doesn't look like a JWT: %q", payload.Token)
			}
			if payload.Record.Email != demoUserEmail {
				t.Errorf("expected email %q, got %q", demoUserEmail, payload.Record.Email)
			}
			if !payload.Record.IsDemo {
				t.Error("record.is_demo should be true so client suppresses outbound effects")
			}
			if payload.Record.ID == "" {
				t.Error("empty record.id")
			}
		},
	}
	scenario.Test(t)
}

// TestDemoStart_SetsUsername verifies that the demo user gets the stable
// "demo" username so the front-end can address the demo session by username.
func TestDemoStart_SetsUsername(t *testing.T) {
	app := setupDemoStartTestApp(t)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/demo/start",
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"username":"demo"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, _ *http.Response) {
			tt := t.(*testing.T)
			rec, err := app.FindFirstRecordByFilter(
				"users", "username = {:u}", dbx.Params{"u": demoUserUsername})
			if err != nil {
				tt.Fatalf("demo user not found by username: %v", err)
			}
			if got := rec.GetString("username"); got != demoUserUsername {
				tt.Errorf("username = %q, want %q", got, demoUserUsername)
			}
		},
	}
	scenario.Test(t)
}

// TestDemoStartUserPasswordIsUnknowable verifies that even though the
// endpoint creates a real auth user, the password is set to fresh random
// bytes and never returned. Anyone who learns the email can't sign in via
// /authWithPassword — the demo flow is the only door.
func TestDemoStartUserPasswordIsUnknowable(t *testing.T) {
	app := setupDemoStartTestApp(t)

	scenario := &tests.ApiScenario{
		Name:                  "password not exposed",
		Method:                http.MethodPost,
		URL:                   "/api/demo/start",
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"token":`},
		NotExpectedContent:    []string{`"password":`, `"tokenKey":`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			var raw map[string]any
			if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
				t.Fatalf("decode: %v", err)
			}
			rec, _ := raw["record"].(map[string]any)
			if _, hasPwd := rec["password"]; hasPwd {
				t.Error("response leaks password field")
			}
			if _, hasTokenKey := rec["tokenKey"]; hasTokenKey {
				t.Error("response leaks tokenKey field")
			}
		},
	}
	scenario.Test(t)
}
