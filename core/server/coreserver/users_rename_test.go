package coreserver

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/offboard"
	"tinycld.org/core/rlstest"
)

// Renaming a login is an admin action, and a deleted user (a tombstone) can
// neither be changed nor brought back.

func newRenameEnv(t *testing.T) *recoveryEnv {
	t.Helper()
	t.Setenv("TINYCLD_EMAIL_LOG", filepath.Join(t.TempDir(), "emails.log"))
	app := rlstest.NewAssembledApp(t)
	registerUsersFieldGuardCore(app)
	registerAccountDeleteCore(app)
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	env := &recoveryEnv{t: t, app: app}
	err = app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}, func(e *core.ServeEvent) error {
		mux, err := e.Router.BuildMux()
		env.mux = mux
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func (env *recoveryEnv) rename(target, actor *core.Record, username string) (int, string) {
	return env.do(http.MethodPatch, "/api/collections/users/records/"+target.Id,
		map[string]any{"username": username}, actor)
}

func (env *recoveryEnv) tombstone(target *core.Record) *core.Record {
	env.t.Helper()
	if _, err := offboard.OffboardUser(env.app, target.Id, offboard.Plan{Mode: offboard.ModeKeep}, ""); err != nil {
		env.t.Fatalf("offboard: %v", err)
	}
	fresh, err := env.app.FindRecordById("users", target.Id)
	if err != nil {
		env.t.Fatal(err)
	}
	return fresh
}

func TestRename_AdminRenamesAMember(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	member := env.user("member@x.test", "member")

	code, body := env.rename(member, admin, "newname")
	if code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", code, body)
	}
	got, _ := env.app.FindRecordById("users", member.Id)
	if got.GetString("username") != "newname" {
		t.Fatalf("username = %q, want newname", got.GetString("username"))
	}
}

func TestRename_MemberCannotRenameSelf(t *testing.T) {
	env := newRenameEnv(t)
	member := env.user("member@x.test", "member")
	if code, body := env.rename(member, member, "newname"); code != http.StatusForbidden {
		t.Fatalf("status = %d %s, want 403", code, body)
	}
}

func TestRename_RefusesTheTombstonePrefix(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	member := env.user("member@x.test", "member")
	code, body := env.rename(member, admin, "deleted-abc")
	if code != http.StatusBadRequest || !strings.Contains(body, `"username"`) {
		t.Fatalf("got %d %s, want 400 on username", code, body)
	}
}

func TestRename_RefusesAnInvalidName(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	member := env.user("member@x.test", "member")
	code, body := env.rename(member, admin, "Bad Name")
	if code != http.StatusBadRequest || !strings.Contains(body, `"username"`) {
		t.Fatalf("got %d %s, want 400 on username", code, body)
	}
}

func TestTombstone_CannotBeReenabledByUpdate(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	gone := env.tombstone(env.user("gone@x.test", "member"))
	code, body := env.do(http.MethodPatch, "/api/collections/users/records/"+gone.Id,
		map[string]any{"disabled": false}, admin)
	if code != http.StatusForbidden || !strings.Contains(body, "This user was deleted") {
		t.Fatalf("got %d %s, want 403", code, body)
	}
}

func TestTombstone_CannotBeRenamed(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	gone := env.tombstone(env.user("gone@x.test", "member"))
	if code, body := env.rename(gone, admin, "reuse"); code != http.StatusForbidden {
		t.Fatalf("got %d %s, want 403", code, body)
	}
}

func TestTombstone_CannotBeEnabled(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	gone := env.tombstone(env.user("gone@x.test", "member"))
	code, body := env.do(http.MethodPost, "/api/account/enable", map[string]any{"user_id": gone.Id}, admin)
	if code != http.StatusBadRequest || !strings.Contains(body, "This user was deleted") {
		t.Fatalf("got %d %s, want 400", code, body)
	}
}

// Renaming an owner changes how they sign in, so only an owner may do it,
// as with setting an owner's password.
func TestRename_AdminCannotRenameAnOwner(t *testing.T) {
	env := newRenameEnv(t)
	admin := env.user("admin@x.test", "admin")
	owner := env.user("boss@x.test", "owner")
	if code, body := env.rename(owner, admin, "newboss"); code != http.StatusForbidden {
		t.Fatalf("got %d %s, want 403", code, body)
	}
}

func TestRename_OwnerCanRenameAnOwner(t *testing.T) {
	env := newRenameEnv(t)
	owner := env.user("boss@x.test", "owner")
	other := env.user("boss2@x.test", "owner")
	if code, body := env.rename(other, owner, "newboss"); code != http.StatusOK {
		t.Fatalf("got %d %s, want 200", code, body)
	}
}
