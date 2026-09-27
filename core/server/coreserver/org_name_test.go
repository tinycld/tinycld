package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSetOrgName(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	if err := setOrgName(app, "  Harbor Dental  "); err != nil {
		t.Fatal(err)
	}
	if got := app.Settings().Meta.AppName; got != "Harbor Dental" {
		t.Fatalf("AppName = %q", got)
	}
	if err := setOrgName(app, "   "); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := setOrgName(app, strings.Repeat("x", 256)); err == nil {
		t.Fatal("256-char name accepted")
	}
}

// The rename endpoint's gate and validation, over HTTP: nobody but a signed-in
// owner or admin may rename the workspace, and a blank name is refused.
func TestOrgNameEndpoint(t *testing.T) {
	cases := []struct {
		name       string
		role       string // "" signs in nobody
		body       string
		wantStatus int
		wantBody   string
		wantSaved  string
	}{
		{"anonymous", "", `{"name":"Harbor Dental"}`, http.StatusUnauthorized, "Sign in to rename", "Acme"},
		{"member", "member", `{"name":"Harbor Dental"}`, http.StatusForbidden, "Only an owner or admin", "Acme"},
		{"empty name", "owner", `{"name":"   "}`, http.StatusBadRequest, "Name is required.", "Acme"},
		{"owner", "owner", `{"name":"Harbor Dental"}`, http.StatusOK, `"name":"Harbor Dental"`, "Harbor Dental"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := setupInviteTestApp(t)
			app.Settings().Meta.AppName = "Acme"
			RegisterOrgNameEndpoint(app)
			headers := map[string]string{}
			if tc.role != "" {
				user := setUserRole(t, app, mustCreateUser(t, app, tc.role+"@test.local", false), tc.role)
				tok, err := tokenForUser(app, user)
				if err != nil {
					t.Fatal(err)
				}
				headers["Authorization"] = tok
			}
			scenario := &tests.ApiScenario{
				Name:                  tc.name,
				Method:                http.MethodPost,
				URL:                   "/api/org-info/name",
				Body:                  strings.NewReader(tc.body),
				Headers:               headers,
				ExpectedStatus:        tc.wantStatus,
				ExpectedContent:       []string{tc.wantBody},
				TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
				DisableTestAppCleanup: true,
			}
			scenario.Test(t)
			if got := app.Settings().Meta.AppName; got != tc.wantSaved {
				t.Fatalf("AppName = %q, want %q", got, tc.wantSaved)
			}
		})
	}
}
