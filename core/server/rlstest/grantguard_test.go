package rlstest

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional package: zoos are shared through zoo_keepers, whose rows carry
// an optional user (a grant row leaves it "") and a group.
func zooMatcher() grantRefMatcher {
	return grantRefMatcher{grant: "zoo_keepers", relFields: map[string]bool{"keeper": true}}
}

func TestCheckRule(t *testing.T) {
	const via = `zoo.zoo_keepers_via_zoo.user ?= @request.auth.id`
	const token = `@collection.zoo_links.token ?= @request.headers.x_token`

	cases := []struct {
		name      string
		rule      string
		own       bool
		found     bool
		unguarded int
	}{
		{"no reference", `@request.auth.id != "" && owner = @request.auth.id`, false, false, 0},
		{"unguarded back-relation", `@request.auth.disabled != true && ` + via, false, true, 1},
		{"guarded back-relation", `@request.auth.id != "" && @request.auth.disabled != true && ` + via, false, true, 0},
		{"guard with single quotes", `@request.auth.id != '' && ` + via, false, true, 0},
		{"guard in the member branch only", `(@request.auth.id != "" && ` + via + `) || (` + token + `)`, false, true, 0},
		{"guard in another OR branch", `(@request.auth.id != "" && ` + token + `) || (` + via + `)`, false, true, 1},
		{"guard OR'd with the test", `@request.auth.id != "" || ` + via, false, true, 1},
		{"guard inside a nested OR", `(@request.auth.id != "" || x = 1) && ` + via, false, true, 1},
		{"top-level guard covers nested OR", `@request.auth.id != "" && (owner = @request.auth.id || (` + via + ` && role ?= "owner"))`, false, true, 0},
		{"@collection form", `@collection.zoo_keepers.zoo ?= zoo && @collection.zoo_keepers.user ?= @request.auth.id`, false, true, 1},
		{"@collection alias form", `@collection.zoo_keepers:k.user ?= @request.auth.id`, false, true, 1},
		{"@collection of another table", `@collection.zoo_links.user ?= @request.auth.id`, false, false, 0},
		{"forward relation to the grant table", `keeper.user = @request.auth.id`, false, true, 1},
		{"own bare user", `@request.auth.disabled != true && (user = @request.auth.id || x = 1)`, true, true, 1},
		{"own bare user guarded", `@request.auth.id != "" && (user = @request.auth.id || x = 1)`, true, true, 0},
		{"own body user is request data", `@request.body.user:isset = false || @request.body.user = user`, true, true, 1},
		{"bare user outside the grant table", `user = @request.auth.id`, false, false, 0},
		{"reference text inside a string", `name = "zoo_keepers_via_zoo.user && x"`, false, false, 0},
		{"function call parens stay in the clause", `strftime('%Y', created) = "2026" && ` + via, false, true, 1},
		{"comment dropped", "// zoo_keepers_via_zoo.user\n" + `@request.auth.id != "" && ` + via, false, true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := zooMatcher()
			m.own = c.own
			found, unguarded, err := checkRule(c.rule, m)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if found != c.found || len(unguarded) != c.unguarded {
				t.Fatalf("found=%v unguarded=%q; want found=%v and %d unguarded",
					found, unguarded, c.found, c.unguarded)
			}
		})
	}
}

func TestCheckRule_RejectsMalformedRule(t *testing.T) {
	for _, rule := range []string{`(a = 1`, `a = 1 &&`, `name = "open`} {
		if _, _, err := checkRule(rule, zooMatcher()); err == nil {
			t.Errorf("checkRule(%q) parsed a malformed rule", rule)
		}
	}
}

// zooApp builds the fictional package on a real app, with the rules given.
func zooApp(t *testing.T, keeperList, animalList string) core.App {
	t.Helper()
	app := NewApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	zoos := core.NewBaseCollection("zoos")
	zoos.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(zoos); err != nil {
		t.Fatal(err)
	}
	keepers := core.NewBaseCollection("zoo_keepers")
	keepers.Fields.Add(
		&core.RelationField{Name: "zoo", CollectionId: zoos.Id, MaxSelect: 1},
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "group"},
	)
	if err := app.Save(keepers); err != nil {
		t.Fatal(err)
	}
	animals := core.NewBaseCollection("zoo_animals")
	animals.Fields.Add(&core.RelationField{Name: "zoo", CollectionId: zoos.Id, MaxSelect: 1})
	if err := app.Save(animals); err != nil {
		t.Fatal(err)
	}

	keepers.ListRule = &keeperList
	if err := app.Save(keepers); err != nil {
		t.Fatal(err)
	}
	animals.ListRule = &animalList
	if err := app.Save(animals); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestGrantGuardViolations_FindsEveryUnguardedRule(t *testing.T) {
	app := zooApp(t,
		`@request.auth.disabled != true && user = @request.auth.id`,
		`@request.auth.disabled != true && zoo.zoo_keepers_via_zoo.user ?= @request.auth.id`,
	)
	refs, violations, err := grantGuardViolations(app, "zoo_keepers")
	if err != nil {
		t.Fatal(err)
	}
	if refs != 2 || len(violations) != 2 {
		t.Fatalf("refs=%d violations=%q; want 2 and 2", refs, violations)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{"zoo_keepers.listRule", "zoo_animals.listRule"} {
		if !strings.Contains(joined, want) {
			t.Errorf("violations do not name %s:\n%s", want, joined)
		}
	}
}

func TestRequireAuthGuardOnGrantRules_PassesGuardedRules(t *testing.T) {
	app := zooApp(t,
		`@request.auth.id != "" && user = @request.auth.id`,
		`@request.auth.id != "" && zoo.zoo_keepers_via_zoo.user ?= @request.auth.id`,
	)
	RequireAuthGuardOnGrantRules(t, app, "zoo_keepers")
}

func TestGrantGuardViolations_CountsNoReferences(t *testing.T) {
	app := zooApp(t, `@request.auth.id != ""`, `@request.auth.id != ""`)
	refs, violations, err := grantGuardViolations(app, "zoo_keepers")
	if err != nil {
		t.Fatal(err)
	}
	if refs != 0 || len(violations) != 0 {
		t.Fatalf("refs=%d violations=%q; want 0 and none", refs, violations)
	}
}
