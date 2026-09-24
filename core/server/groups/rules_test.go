package groups

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/rlstest"
)

// newMigratedApp applies every shipped core migration so the rules under test
// are the rules the product ships. The username-index dance mirrors
// coreserver/guest_rls_test.go: the bundled fixture already carries the index
// that 1820000000 adds.
func newMigratedApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := rlstest.NewApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}
	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))
	return app
}

func TestGroupsShippedRules(t *testing.T) {
	app := newMigratedApp(t)

	const notGuest = `@request.auth.role != "guest"`
	const admin = `@request.auth.role = "admin" || @request.auth.role = "owner"`

	rlstest.RequireRuleContains(t, app, "groups", "list", notGuest)
	rlstest.RequireRuleContains(t, app, "groups", "view", notGuest)
	rlstest.RequireRuleContains(t, app, "groups", "create", admin)
	rlstest.RequireRuleContains(t, app, "groups", "update", admin)
	rlstest.RequireRuleContains(t, app, "groups", "delete", admin)

	rlstest.RequireRuleContains(t, app, "group_members", "list", notGuest)
	rlstest.RequireRuleContains(t, app, "group_members", "view", notGuest)
	rlstest.RequireRuleContains(t, app, "group_members", "create", admin)
	rlstest.RequireRuleContains(t, app, "group_members", "create", `user.role != "guest"`)
	rlstest.RequireRuleContains(t, app, "group_members", "create", `user.disabled != true`)
	rlstest.RequireRuleContains(t, app, "group_members", "delete", admin)

	if rule, ok := rlstest.Rule(t, app, "group_members", "update"); ok {
		t.Fatalf("group_members update rule must be locked (nil), got %q", rule)
	}

	col, err := app.FindCollectionByNameOrId("group_members")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(col.Indexes, "\n"), "UNIQUE INDEX `idx_group_members_unique` ON `group_members` (`group`, `user`)") {
		t.Fatalf("missing unique (group,user) index: %v", col.Indexes)
	}
	groupsCol, err := app.FindCollectionByNameOrId("groups")
	if err != nil {
		t.Fatal(err)
	}
	if groupsCol.Id != "pbc_groups_01" || col.Id != "pbc_group_members_01" {
		t.Fatalf("collection ids are load-bearing for package migrations: %s %s", groupsCol.Id, col.Id)
	}
}
