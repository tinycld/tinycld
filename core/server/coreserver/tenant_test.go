package coreserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// tenant_test.go asserts core's authorization THE WAY A HOSTED TENANT SEES IT:
// an app composed by RegisterTenant, requests served through the real router
// mux, no host-only registration bound. This is the permanent coverage for
// multi-org/docs/FINDING-tenant-composition-gap.md — the two proven holes
// (member self-promotion to owner, disabled users keeping REST access) existed
// precisely because no test ran the tenant configuration.
//
// Nothing here may call Register() or bind a guard directly. If one of these
// tests starts passing only because a hook was bound by hand, it has stopped
// testing what it is named for.

// tenantAuthzEnv is a bootstrapped tenant-composed app plus its serve mux.
type tenantAuthzEnv struct {
	app *pocketbase.PocketBase
	mux http.Handler
}

// setupTenantApp composes a real *pocketbase.PocketBase via RegisterTenant
// (empty materialized dirs — no packages installed, the lean-tenant shape),
// bootstraps it, and installs the users schema the guards govern: the `role` /
// `disabled` / `is_demo` fields and the deliberately loose updateRule that
// core's app migrations ship. The rule is restated here because core's JS
// migrations live in the app shell, outside this module — but the thing under
// test is the Go guard the tenant composition binds, not the rule string.
func setupTenantApp(t *testing.T) *tenantAuthzEnv {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})
	if err := RegisterTenant(app, TenantOptions{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 1,
	}); err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	users.Fields.Add(&core.BoolField{Name: "is_demo"})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	// The shipped users.updateRule is loose ON PURPOSE (any authed user may
	// attempt an update, so pbtsdb mutations work client-side); the users
	// field guard narrows it to self / admin-allowlisted-field. In a tenant
	// with no guard bound, this rule alone WAS the whole policy — that is the
	// finding.
	users.UpdateRule = stringPtr(
		`@request.auth.id != "" && (id = @request.auth.id || ` +
			`@request.auth.role = "owner" || @request.auth.role = "admin")`,
	)
	if err := app.Save(users); err != nil {
		t.Fatalf("save users: %v", err)
	}

	// The same mux-building path serve-org's apis.Serve uses: triggers the
	// full OnServe chain, launches no installer, binds no listener.
	mux, err := apis.BuildServeMux(app, apis.ServeConfig{})
	if err != nil {
		t.Fatalf("BuildServeMux: %v", err)
	}
	return &tenantAuthzEnv{app: app, mux: mux}
}

func (env *tenantAuthzEnv) makeUser(t *testing.T, email, role string, disabled bool) (*core.Record, string) {
	t.Helper()
	col, err := env.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("name", "Original Name")
	r.SetVerified(true)
	r.SetPassword("Password123!")
	r.Set("role", role)
	r.Set("disabled", disabled)
	if err := env.app.Save(r); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	token, err := r.NewAuthToken()
	if err != nil {
		t.Fatalf("token for %s: %v", email, err)
	}
	return r, token
}

func (env *tenantAuthzEnv) request(t *testing.T, method, path, token string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(raw))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	return rec
}

// TestTenantMemberCannotSelfPromote reproduces the finding's proven attack in
// the fixed configuration: a plain member PATCHes their own users record with
// role=owner. The loose updateRule permits it; the field guard — which the
// tenant composition now binds — must refuse it.
func TestTenantMemberCannotSelfPromote(t *testing.T) {
	env := setupTenantApp(t)
	member, token := env.makeUser(t, "member@tenant.test", "member", false)

	rec := env.request(t, http.MethodPatch,
		"/api/collections/users/records/"+member.Id, token,
		map[string]any{"role": "owner"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("self role escalation: want 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	fresh, err := env.app.FindRecordById("users", member.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.GetString("role"); got != "member" {
		t.Fatalf("role changed to %q despite rejected request", got)
	}

	// Fixture sanity: the same member editing an allowlisted field on the
	// same record succeeds, so the 403 above is the guard rejecting the
	// FIELD, not a broken auth/fixture producing blanket denials.
	rec = env.request(t, http.MethodPatch,
		"/api/collections/users/records/"+member.Id, token,
		map[string]any{"name": "Renamed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("allowlisted self-edit: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestTenantDisabledUserRefusedAuth covers the second missing guard: a
// suspended user must not be able to sign in or keep renewing a live session
// in a tenant. The disabled flag is enforced by a Go hook (PB has no
// authRule), so before the fix a tenant simply did not have it.
func TestTenantDisabledUserRefusedAuth(t *testing.T) {
	env := setupTenantApp(t)
	_, token := env.makeUser(t, "suspended@tenant.test", "member", true)

	rec := env.request(t, http.MethodPost,
		"/api/collections/users/auth-with-password", "",
		map[string]any{"identity": "suspended@tenant.test", "password": "Password123!"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled password auth: want 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A still-valid token must not be renewable either — refresh passes
	// through OnRecordAuthRequest, the common tail the guard binds.
	rec = env.request(t, http.MethodPost,
		"/api/collections/users/auth-refresh", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled auth refresh: want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
}
