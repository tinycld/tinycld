package rlstest

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestAccessPaths(t *testing.T) {
	const own = `user = @request.auth.id`
	const token = `@collection.zoo_links.token ?= @request.headers.x_token`

	cases := []struct {
		name string
		rule string
		// Counts of the rule's access paths by class.
		guarded, unguardedAuth, public int
	}{
		{"public rule", ``, 0, 0, 1},
		{"whitespace-only rule", "  \n ", 0, 0, 1},
		{"guard alone", `@request.auth.id != ""`, 1, 0, 0},
		{"guarded own row", `@request.auth.id != "" && ` + own, 1, 0, 0},
		{"unguarded own row", own, 0, 1, 0},
		{"guard with single quotes", `@request.auth.id != '' && ` + own, 1, 0, 0},
		{"guard with operands swapped", `"" != @request.auth.id && ` + own, 1, 0, 0},
		{"a different comparison is no guard", `@request.auth.id != "x" && ` + own, 0, 1, 0},
		{"a negative check is no guard", `@request.auth.disabled != true && ` + own, 0, 1, 0},
		{"a positive role check still needs the guard", `@request.auth.role = "admin"`, 0, 1, 0},
		{"guarded member branch, public token branch", `(@request.auth.id != "" && ` + own + `) || (` + token + `)`, 1, 0, 1},
		{"guard in the wrong branch", `(@request.auth.id != "" && ` + token + `) || (` + own + `)`, 1, 1, 0},
		{"guard OR'd with the test", `@request.auth.id != "" || ` + own, 1, 1, 0},
		{"guard inside a nested OR", `(@request.auth.id != "" || x = 1) && ` + own, 1, 1, 0},
		{"top-level guard covers a nested OR", `@request.auth.id != "" && (` + own + ` || (x = 1 && y = 2))`, 2, 0, 0},
		{"&& binds tighter than ||", `x = 1 || y = 2 && @request.auth.id != ""`, 1, 0, 1},
		{"a nested OR is distributed", `x = 1 && (@request.auth.id != "" || ` + own + `)`, 1, 1, 0},
		{"deep nesting", `((x = 1 && (@request.auth.id != "" && (` + own + `))))`, 1, 0, 0},
		{"auth read inside a function argument", `strftime('%Y', @request.auth.created) = "2026"`, 0, 1, 0},
		{"auth text inside a string is not a read", `name = "@request.auth.id"`, 0, 0, 1},
		{"a path with only request data is public", `@request.headers.x_token = token`, 0, 0, 1},
		{"comment dropped", "// user = @request.auth.id\n" + `@request.auth.id != "" && ` + own, 1, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths, err := accessPaths(c.rule)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var guarded, unguardedAuth, public int
			for _, p := range paths {
				switch {
				case p.guarded:
					guarded++
				case p.readsAuth:
					unguardedAuth++
				default:
					public++
				}
			}
			if guarded != c.guarded || unguardedAuth != c.unguardedAuth || public != c.public {
				t.Fatalf("guarded=%d unguardedAuth=%d public=%d; want %d %d %d\n  paths: %+v",
					guarded, unguardedAuth, public, c.guarded, c.unguardedAuth, c.public, paths)
			}
		})
	}
}

func TestAccessPaths_RejectsMalformedRule(t *testing.T) {
	for _, rule := range []string{`(a = 1`, `a = 1 &&`, `name = "open`} {
		if _, err := accessPaths(rule); err == nil {
			t.Errorf("accessPaths(%q) parsed a malformed rule", rule)
		}
	}
}

func TestAccessPaths_RejectsAnExplodingRule(t *testing.T) {
	// 13 two-way ORs ANDed together give 2^13 = 8192 paths.
	rule := strings.Repeat(`(a = 1 || b = 2) && `, 12) + `(a = 1 || b = 2)`
	if _, err := accessPaths(rule); err == nil {
		t.Fatal("accessPaths expanded a rule past the path limit")
	}
}

// aviaryApp builds a fictional package on an app with no demo collections:
// birds, readable by their keeper, with the given rules.
func aviaryApp(t *testing.T, rules map[string]string) core.App {
	t.Helper()
	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	// PocketBase's own init rules on users are not under test here.
	users.ListRule, users.ViewRule, users.CreateRule, users.UpdateRule, users.DeleteRule = nil, nil, nil, nil, nil
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	birds := core.NewBaseCollection("birds")
	birds.Fields.Add(
		&core.RelationField{Name: "keeper", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "token"},
	)
	for kind, rule := range rules {
		r := rule
		switch kind {
		case "list":
			birds.ListRule = &r
		case "view":
			birds.ViewRule = &r
		case "create":
			birds.CreateRule = &r
		case "update":
			birds.UpdateRule = &r
		case "delete":
			birds.DeleteRule = &r
		}
	}
	if err := app.Save(birds); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestAccessGuardViolations(t *testing.T) {
	const guarded = `@request.auth.id != "" && keeper = @request.auth.id`
	const unguarded = `@request.auth.disabled != true && keeper = @request.auth.id`
	const tokenRead = `(` + guarded + `) || token = @request.headers.x_token`
	allowTokenList := PublicPath{"birds", "list", "a share link reads a bird by token"}

	cases := []struct {
		name  string
		rules map[string]string
		allow []PublicPath
		// Substrings, one per expected violation.
		want []string
	}{
		{"every path guarded", map[string]string{"list": guarded, "delete": guarded}, nil, nil},
		{"superuser-only rules are skipped", map[string]string{}, nil, nil},
		{"unguarded auth read", map[string]string{"list": unguarded}, nil,
			[]string{"birds.listRule reads @request.auth"}},
		{"public rule", map[string]string{"view": ``}, nil,
			[]string{"birds.viewRule has a path that needs no login"}},
		{"public token branch, not allowlisted", map[string]string{"list": tokenRead}, nil,
			[]string{"birds.listRule has a path that needs no login"}},
		{"public token branch, allowlisted", map[string]string{"list": tokenRead},
			[]PublicPath{allowTokenList}, nil},
		{"the allowlist never excuses an auth read",
			map[string]string{"list": `(` + unguarded + `) || token = @request.headers.x_token`},
			[]PublicPath{allowTokenList},
			[]string{"birds.listRule reads @request.auth"}},
		{"an allow entry for a rule with no public path is stale",
			map[string]string{"list": guarded}, []PublicPath{allowTokenList},
			[]string{"allow entry birds.list exempts nothing"}},
		{"an allow entry for a missing collection is stale",
			map[string]string{"list": guarded}, []PublicPath{{"aviaries", "list", "gone"}},
			[]string{"allow entry aviaries.list exempts nothing"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := aviaryApp(t, c.rules)
			violations, err := accessGuardViolations(app, c.allow)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != len(c.want) {
				t.Fatalf("got %d violations, want %d:\n%s", len(violations), len(c.want), strings.Join(violations, "\n"))
			}
			for i, w := range c.want {
				if !strings.Contains(violations[i], w) {
					t.Errorf("violation %d does not contain %q:\n%s", i, w, violations[i])
				}
			}
		})
	}
}

func TestAccessGuardViolations_AllowEntryNeedsAReason(t *testing.T) {
	app := aviaryApp(t, map[string]string{"list": ``})
	if _, err := accessGuardViolations(app, []PublicPath{{"birds", "list", " "}}); err == nil {
		t.Fatal("an allow entry with no reason was accepted")
	}
}

func TestRequireAuthGuardOnAccessRules_PassesAGuardedApp(t *testing.T) {
	app := aviaryApp(t, map[string]string{
		"list": `(@request.auth.id != "" && keeper = @request.auth.id) || token = @request.headers.x_token`,
		"view": `@request.auth.id != "" && keeper = @request.auth.id`,
	})
	RequireAuthGuardOnAccessRules(t, app, PublicPath{"birds", "list", "a share link reads a bird by token"})
}
