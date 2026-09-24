package rlstest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ganigeorgiev/fexpr"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// PublicPath exempts the access paths of one rule that need no login.
//
// It exempts only the paths that do not read @request.auth: a deliberately
// public branch (a share-token test, a login-screen read). A path that reads
// @request.auth must carry AuthGuard all the same — an allowlist entry never
// excuses it, because that is the path an anonymous NULL matches.
type PublicPath struct {
	Collection string
	// Kind is one of list, view, create, update, delete.
	Kind   string
	Reason string
}

// CorePublicPaths returns core's deliberate no-login access paths. Every scan
// that applies core's migrations (NewAssembledApp does) must include them.
func CorePublicPaths() []PublicPath {
	return []PublicPath{
		{"org_branding", "list", "the login screen shows the org's branding before anyone signs in"},
		{"org_branding", "view", "the login screen shows the org's branding before anyone signs in"},
	}
}

// RequireAuthGuardOnAccessRules fails the test for every access path, in every
// list, view, create, update and delete rule of every non-system collection in
// app, that can be taken without a login.
//
// An access path is one conjunction of the rule's disjunctive normal form:
// `a && (b || c)` has the paths `a && b` and `a && c`. A path requires a login
// only if it contains AuthGuard as one of its conjuncts.
//
// Two kinds of path fail:
//   - A path that reads @request.auth without AuthGuard. For a request with no
//     login PocketBase resolves every @request.auth field to NULL and rewrites
//     `x = NULL` as `(x = ” OR x IS NULL)`, so the path matches every row whose
//     x is empty or absent — an optional user column, a group-grant row, a
//     LEFT JOIN with no child rows. `@request.auth.disabled != true` does not
//     stop it. This kind cannot be allowlisted: add the guard.
//   - A path that does not read @request.auth at all (a public rule "", a
//     share-token branch), unless allow names that rule. Such a path is open to
//     anyone on purpose or by accident; the allowlist makes it on purpose.
//
// An allow entry that exempts nothing (the rule is gone, is superuser-only, or
// has no public path) also fails, so the list cannot outlive what it excuses.
//
// Rules are parsed with fexpr, the parser PocketBase itself evaluates them
// with, so grouping and precedence match what the server does.
func RequireAuthGuardOnAccessRules(t testing.TB, app core.App, allow ...PublicPath) {
	t.Helper()
	violations, err := accessGuardViolations(app, allow)
	if err != nil {
		t.Fatalf("rlstest: %v", err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

// NewAssembledApp returns an app on an EMPTY data dir — only PocketBase's own
// system migrations — with core's JS migrations and each of dirs applied as a
// real install applies them: merged into one directory and run in filename
// order.
//
// A scan must see the rules a real install ships, including the core rules a
// package extends: comment_mentions' create rule gains a branch per package,
// and a per-package app that skipped core's migrations never saw it. The
// default test data dir is not used because its demo collections carry
// PocketBase's sample rules, which are no part of any install.
func NewAssembledApp(t testing.TB, dirs ...string) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatalf("rlstest: NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	merged := t.TempDir()
	for _, dir := range append([]string{CoreMigrationsDir(t)}, dirs...) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("rlstest: read %s: %v", dir, err)
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".js" {
				continue
			}
			dst := filepath.Join(merged, e.Name())
			if _, err := os.Stat(dst); err == nil {
				t.Fatalf("rlstest: two migrations named %s — a real install would load only one", e.Name())
			}
			body, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("rlstest: read %s: %v", e.Name(), err)
			}
			if err := os.WriteFile(dst, body, 0o644); err != nil {
				t.Fatalf("rlstest: stage %s: %v", e.Name(), err)
			}
		}
	}
	Apply(t, app, merged)
	return app
}

// CoreMigrationsDir is core's pb_migrations directory, found from this source
// file so a package test resolves it wherever core is checked out.
func CoreMigrationsDir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("rlstest: cannot locate rlstest source")
	}
	return MigrationsDir(t, filepath.Join(filepath.Dir(file), "..", "pb_migrations"))
}

func accessGuardViolations(app core.App, allow []PublicPath) ([]string, error) {
	type ruleKey struct{ collection, kind string }
	allowed := map[ruleKey]PublicPath{}
	for _, a := range allow {
		if strings.TrimSpace(a.Reason) == "" {
			return nil, fmt.Errorf("allow entry %s.%s has no reason", a.Collection, a.Kind)
		}
		allowed[ruleKey{a.Collection, a.Kind}] = a
	}
	used := map[ruleKey]bool{}

	cols, err := app.FindAllCollections()
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	var violations []string
	for _, c := range cols {
		if c.System {
			continue
		}
		for _, r := range []struct {
			kind string
			rule *string
		}{
			{"list", c.ListRule},
			{"view", c.ViewRule},
			{"create", c.CreateRule},
			{"update", c.UpdateRule},
			{"delete", c.DeleteRule},
		} {
			if r.rule == nil {
				continue
			}
			paths, perr := accessPaths(*r.rule)
			if perr != nil {
				return nil, fmt.Errorf("%s.%sRule: %w\n  rule: %s", c.Name, r.kind, perr, *r.rule)
			}
			key := ruleKey{c.Name, r.kind}
			var unguardedAuth, public []string
			for _, p := range paths {
				switch {
				case p.guarded:
					continue
				case p.readsAuth:
					unguardedAuth = append(unguardedAuth, p.text)
				default:
					public = append(public, p.text)
				}
			}
			if len(public) > 0 {
				if _, ok := allowed[key]; ok {
					used[key] = true
					public = nil
				}
			}
			if len(unguardedAuth) > 0 {
				violations = append(violations, fmt.Sprintf(
					"%s.%sRule reads @request.auth on a path without %s — an anonymous "+
						"request's NULL auth matches empty or absent values there:\n    %s\n  rule: %s",
					c.Name, r.kind, AuthGuard, strings.Join(unguardedAuth, "\n    "), *r.rule))
			}
			if len(public) > 0 {
				violations = append(violations, fmt.Sprintf(
					"%s.%sRule has a path that needs no login; add %s, or allowlist it "+
						"with a reason if it is public on purpose:\n    %s\n  rule: %q",
					c.Name, r.kind, AuthGuard, strings.Join(public, "\n    "), *r.rule))
			}
		}
	}
	for _, a := range allow {
		if !used[ruleKey{a.Collection, a.Kind}] {
			violations = append(violations, fmt.Sprintf(
				"allow entry %s.%s exempts nothing: the rule has no public path (or no longer "+
					"exists) — delete the entry (reason was: %s)", a.Collection, a.Kind, a.Reason))
		}
	}
	return violations, nil
}

// accessPath is one conjunction of a rule's disjunctive normal form.
type accessPath struct {
	text      string
	guarded   bool
	readsAuth bool
}

// maxAccessPaths bounds the DNF expansion. Real rules have a handful of paths;
// hitting this means a rule the check cannot reason about, which is a failure
// rather than a silent pass.
const maxAccessPaths = 4096

func accessPaths(rule string) ([]accessPath, error) {
	groups, err := fexpr.Parse(rule)
	if err != nil {
		if errors.Is(err, fexpr.ErrEmpty) {
			// A rule of "" is public: one path with no conditions at all.
			return []accessPath{{text: `"" (public)`}}, nil
		}
		return nil, err
	}
	dnf, err := groupsDNF(groups)
	if err != nil {
		return nil, err
	}
	paths := make([]accessPath, 0, len(dnf))
	for _, conj := range dnf {
		p := accessPath{}
		texts := make([]string, 0, len(conj))
		for _, e := range conj {
			if isGuardExpr(e) {
				p.guarded = true
			}
			if readsAuth(e.Left) || readsAuth(e.Right) {
				p.readsAuth = true
			}
			texts = append(texts, exprText(e))
		}
		p.text = strings.Join(texts, " && ")
		paths = append(paths, p)
	}
	return paths, nil
}

// groupsDNF expands one level of fexpr groups. PocketBase joins a level's
// items into one SQL fragment (`a AND b OR c`), so SQL's precedence applies:
// && binds tighter than ||. The level is therefore split at each || into
// conjunctions, and each conjunction is the cross product of its items.
func groupsDNF(groups []fexpr.ExprGroup) ([][]fexpr.Expr, error) {
	var result [][]fexpr.Expr
	var conj [][]fexpr.Expr
	flush := func() {
		if conj != nil {
			result = append(result, conj...)
		}
	}
	for i, g := range groups {
		item, err := itemDNF(g.Item)
		if err != nil {
			return nil, err
		}
		if i == 0 || g.Join == fexpr.JoinOr {
			flush()
			conj = item
			continue
		}
		conj, err = crossDNF(conj, item)
		if err != nil {
			return nil, err
		}
	}
	flush()
	if len(result) > maxAccessPaths {
		return nil, fmt.Errorf("more than %d access paths", maxAccessPaths)
	}
	return result, nil
}

func itemDNF(item any) ([][]fexpr.Expr, error) {
	switch v := item.(type) {
	case fexpr.Expr:
		return [][]fexpr.Expr{{v}}, nil
	case fexpr.ExprGroup:
		return groupsDNF([]fexpr.ExprGroup{v})
	case []fexpr.ExprGroup:
		return groupsDNF(v)
	default:
		return nil, fmt.Errorf("unsupported expression item %T", item)
	}
}

func crossDNF(a, b [][]fexpr.Expr) ([][]fexpr.Expr, error) {
	if len(a)*len(b) > maxAccessPaths {
		return nil, fmt.Errorf("more than %d access paths", maxAccessPaths)
	}
	out := make([][]fexpr.Expr, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			c := make([]fexpr.Expr, 0, len(x)+len(y))
			out = append(out, append(append(c, x...), y...))
		}
	}
	return out, nil
}

// isGuardExpr recognises AuthGuard structurally, in either operand order and
// either quote style, since fexpr has already stripped the quotes.
func isGuardExpr(e fexpr.Expr) bool {
	if e.Op != fexpr.SignNeq {
		return false
	}
	isAuthID := func(tok fexpr.Token) bool {
		return tok.Type == fexpr.TokenIdentifier && tok.Literal == "@request.auth.id"
	}
	isEmpty := func(tok fexpr.Token) bool {
		return tok.Type == fexpr.TokenText && tok.Literal == ""
	}
	return (isAuthID(e.Left) && isEmpty(e.Right)) || (isEmpty(e.Left) && isAuthID(e.Right))
}

func readsAuth(tok fexpr.Token) bool {
	switch tok.Type {
	case fexpr.TokenIdentifier:
		return tok.Literal == "@request.auth" || strings.HasPrefix(tok.Literal, "@request.auth.") ||
			strings.HasPrefix(tok.Literal, "@request.auth:")
	case fexpr.TokenFunction:
		args, _ := tok.Meta.([]fexpr.Token)
		for _, a := range args {
			if readsAuth(a) {
				return true
			}
		}
	}
	return false
}

func exprText(e fexpr.Expr) string {
	return tokenText(e.Left) + " " + string(e.Op) + " " + tokenText(e.Right)
}

func tokenText(tok fexpr.Token) string {
	switch tok.Type {
	case fexpr.TokenText:
		return fmt.Sprintf("%q", tok.Literal)
	case fexpr.TokenFunction:
		args, _ := tok.Meta.([]fexpr.Token)
		parts := make([]string, 0, len(args))
		for _, a := range args {
			parts = append(parts, tokenText(a))
		}
		return tok.Literal + "(" + strings.Join(parts, ", ") + ")"
	default:
		return tok.Literal
	}
}
