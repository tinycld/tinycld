package coreserver

import (
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// requireSuperuserOrToken authorizes the SSE progress stream via a ?token= query
// param (EventSource can't send headers). The token's auth-record lookup must use
// the token TYPE, not a collection id — an earlier version passed the superusers
// collection id, which matched no valid type and 403'd every install's progress
// stream. These guard that regression and the security inverse (a regular user's
// token must NOT authorize the superuser-only endpoint).

func newAuthGuardEvent(app core.App, token string) *core.RequestEvent {
	req := httptest.NewRequest("GET", "/api/admin/packages/events/job_1?token="+token, nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

func newGuardSuperuserToken(t *testing.T, app core.App) string {
	t.Helper()
	su, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(su)
	rec.SetEmail("ssetoken@test.local")
	rec.SetPassword("Superuser1234!")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save superuser: %v", err)
	}
	tok, err := rec.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}
	return tok
}

func TestRequireSuperuserOrToken_ValidSuperuserToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	token := newGuardSuperuserToken(t, app)
	if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, token)); err != nil {
		t.Fatalf("valid superuser token should be authorized, got: %v", err)
	}
}

func TestRequireSuperuserOrToken_RejectsEmptyAndGarbage(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	for _, tok := range []string{"", "not-a-jwt", "a.b.c"} {
		if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, tok)); err == nil {
			t.Fatalf("token %q should be rejected", tok)
		}
	}
}

func TestRequireSuperuserOrToken_RejectsRegularUserToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("regular@test.local")
	user.SetPassword("Regular1234!")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	tok, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}

	if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, tok)); err == nil {
		t.Fatal("a regular user's token must NOT authorize the superuser endpoint")
	}
}
