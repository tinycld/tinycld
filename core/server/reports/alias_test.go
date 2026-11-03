package reports

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// targetCases group by each kind of value the compiler finds a record
// target for (or none); each must compile.
var targetCases = []string{
	// A path that ends at a link, at id, or at a field; a bucketed date.
	`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},{"id":"i","path":[{"link":"animal"},{"field":"id"}]},
		{"id":"n","path":[{"field":"note"}]},{"id":"d","path":[{"field":"fed"}]}],
		"groups":[{"id":"ga","col":"a"},{"id":"gi","col":"i"},{"id":"gn","col":"n"},{"id":"gd","col":"d","bucket":"month"}]}`,
	// A column it names; if, case and coalesce results.
	`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"c","formula":{"col":"a"}},
		{"id":"f","formula":{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"x"}]},{"col":"a"},{"col":"a"}]}},
		{"id":"k","formula":{"op":"case","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"x"}]},{"path":[{"link":"animal"},{"link":"site"}]},{"path":[{"link":"animal"},{"link":"site"}]}]}},
		{"id":"o","formula":{"op":"coalesce","args":[{"path":[{"link":"animal"},{"link":"owner"}]}]}}],
		"groups":[{"id":"gc","col":"c"},{"id":"gf","col":"f"},{"id":"gk","col":"k"},{"id":"go","col":"o"}]}`,
	// A lookup's value: a link of the looked-up source, or the report's column.
	`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"s","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"link":"site"}]}}},
		{"id":"b","formula":{"op":"lookup","source":"zoo.sites","value":{"col":"a"}}}],
		"groups":[{"id":"gs","col":"s"},{"id":"gb","col":"b"}]}`,
	// A to-many total links nowhere.
	`{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"sum"},{"id":"a","path":[{"field":"name"}]}],
		"groups":[{"id":"ga","col":"a"}]}`,
}

// The aliaser finds group targets at load time, without compiling. It must
// find the target the compiler finds for every group of every spec the
// package's tests hold, or a new target rule would leave a renamed key of a
// saved report unresolved.
func TestAliaserTargetsMatchTheCompiler(t *testing.T) {
	app := withZooRegistry(t)
	compared := 0
	// compile is the spec bound and its rows compiled, which finds the
	// compiler's group targets.
	compile := func(raw string) (*Spec, *plan, *compiler, error) {
		spec, _, err := Load([]byte(raw))
		if err != nil {
			return nil, nil, nil, err
		}
		p, se := bind(app, spec)
		if se != nil {
			return nil, nil, nil, se
		}
		c := newCompiler(p, nil, nil, nil)
		_, err = c.rows(newStmt(nil, nil))
		return spec, p, c, err
	}
	check := func(raw string, must bool) {
		spec, p, c, err := compile(raw)
		if err != nil {
			if must {
				t.Fatalf("%s: %v", raw, err)
			}
			return
		}
		a := &aliaser{base: p.base, targets: map[string]*Source{}}
		a.findTargets(spec)
		for i, g := range p.groups {
			if want, got := sourceKey(c.groupTargets[i]), sourceKey(a.targets[g.group.ID]); want != got {
				t.Fatalf("group %s: aliaser target %q, compiler %q\n%s", g.group.ID, got, want, raw)
			}
			compared++
		}
	}
	for _, raw := range targetCases {
		check(raw, true)
	}
	for _, raw := range testSpecLiterals(t) {
		check(raw, false)
	}
	if compared < 50 {
		t.Fatalf("compared only %d groups", compared)
	}
	t.Logf("compared %d groups", compared)
}

func sourceKey(s *Source) string {
	if s == nil {
		return ""
	}
	return s.Key
}

// testSpecLiterals is every string literal in this package's tests that
// holds a report spec with groups.
func testSpecLiterals(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && strings.Contains(s, `"groups"`) {
				out = append(out, s)
			}
			return true
		})
	}
	return out
}

// An exists where under a source the aliaser cannot place is still walked,
// as args are: a lookup in it names its own source.
func TestAnExistsWhereIsWalkedUnderAnUnknownSource(t *testing.T) {
	withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"l","formula":{"op":"lookup","source":"zoo.nowhere",
		"where":{"op":"exists","link":[{"link":"gone"}],"where":{"op":"gt","args":[
			{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"mass"}]}},{"value":1}]}},
		"value":{"value":1}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inner, err := lookupValue(spec.Columns[0].Formula.Where.Where.Args[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if got := inner.Path[0].Field; got != "weight" {
		t.Fatalf("lookup value under an unknown source = %q, want weight", got)
	}
}
