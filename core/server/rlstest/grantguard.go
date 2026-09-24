package rlstest

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// AuthGuard is the clause every rule that tests `user` on a group grant table
// must carry, conjoined with that test.
//
// A grant row stores `user` as "". For a request with no login, PocketBase
// resolves `@request.auth.id` to NULL and rewrites `x = NULL` as
// `(x = "" OR x IS NULL)` — so `…_via_….user ?= @request.auth.id` MATCHES the
// grant row, and an anonymous caller gets whatever the grant's role allows.
// `@request.auth.disabled != true` does not stop it: NULL is not true either.
const AuthGuard = `@request.auth.id != ""`

// RequireAuthGuardOnGrantRules fails the test for every access rule, in every
// collection of app, that tests the `user` field of grantCollection without
// AuthGuard conjoined around that test.
//
// A reference to the grant table's user is any of:
//   - `<grantCollection>_via_<field>.user`, at any depth of a relation path
//   - `@collection.<grantCollection>[:alias].user`
//   - `<field>.user`, where field is a relation that targets grantCollection
//   - a bare `user` (or `user.…`) in grantCollection's own rules
//
// The guard must sit in an `&&` that encloses the reference: a rule shaped
// `(guard && member test) || (share-token test)` passes, because the member
// test is only reached with a login; `guard || member test` does not.
//
// Call it after the package's migrations have been applied. It also fails
// when no rule references the grant table at all, since that almost always
// means the collection name is wrong and the check passed by checking nothing.
func RequireAuthGuardOnGrantRules(t testing.TB, app core.App, grantCollection string) {
	t.Helper()
	refs, violations, err := grantGuardViolations(app, grantCollection)
	if err != nil {
		t.Fatalf("rlstest: %v", err)
	}
	if refs == 0 {
		t.Fatalf("rlstest: no rule references %s.user — wrong collection name?", grantCollection)
	}
	for _, v := range violations {
		t.Errorf("%s\n  a grant row stores user \"\", which an anonymous request's NULL "+
			"@request.auth.id matches; conjoin %s with the test", v, AuthGuard)
	}
}

// grantGuardViolations scans every rule in app. It returns how many rules
// reference the grant table's user and a description of each unguarded one.
func grantGuardViolations(app core.App, grantCollection string) (int, []string, error) {
	grant, err := app.FindCollectionByNameOrId(grantCollection)
	if err != nil {
		return 0, nil, fmt.Errorf("find %s: %w", grantCollection, err)
	}
	cols, err := app.FindAllCollections()
	if err != nil {
		return 0, nil, fmt.Errorf("list collections: %w", err)
	}

	relFields := map[string]bool{}
	for _, c := range cols {
		for _, f := range c.Fields {
			if rel, ok := f.(*core.RelationField); ok && rel.CollectionId == grant.Id {
				relFields[rel.Name] = true
			}
		}
	}

	refs := 0
	var violations []string
	for _, c := range cols {
		m := grantRefMatcher{grant: grant.Name, own: c.Id == grant.Id, relFields: relFields}
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
			found, unguarded, perr := checkRule(*r.rule, m)
			if perr != nil {
				return 0, nil, fmt.Errorf("%s.%sRule: %w\n  rule: %s", c.Name, r.kind, perr, *r.rule)
			}
			if found {
				refs++
			}
			for _, atom := range unguarded {
				violations = append(violations, fmt.Sprintf(
					"%s.%sRule tests %s.user without %s: %q\n  rule: %s",
					c.Name, r.kind, grant.Name, AuthGuard, atom, *r.rule))
			}
		}
	}
	return refs, violations, nil
}

// grantRefMatcher decides whether one clause of a rule reads the grant
// table's user field.
type grantRefMatcher struct {
	grant     string
	own       bool
	relFields map[string]bool
}

var (
	quotedRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
	pathRe   = regexp.MustCompile(`@?[A-Za-z_][\w:]*(?:\.[A-Za-z_][\w:]*)*`)
)

func (m grantRefMatcher) references(atom string) bool {
	stripped := quotedRe.ReplaceAllString(atom, `""`)
	for _, path := range pathRe.FindAllString(stripped, -1) {
		segs := strings.Split(path, ".")
		// @request.* is request data (auth, body, headers, query), never a row.
		if segs[0] == "@request" {
			continue
		}
		if segs[0] == "@collection" && len(segs) >= 3 {
			name, _, _ := strings.Cut(segs[1], ":")
			if name == m.grant && segs[2] == "user" {
				return true
			}
			segs = segs[2:]
		}
		if m.own && segs[0] == "user" {
			return true
		}
		for i := 0; i+1 < len(segs); i++ {
			if segs[i+1] != "user" {
				continue
			}
			if strings.HasPrefix(segs[i], m.grant+"_via_") || m.relFields[segs[i]] {
				return true
			}
		}
	}
	return false
}

// checkRule parses a rule and reports whether any clause references the grant
// user, and which referencing clauses are not under an enclosing AuthGuard.
func checkRule(rule string, m grantRefMatcher) (bool, []string, error) {
	toks, err := tokenizeRule(rule)
	if err != nil {
		return false, nil, err
	}
	if len(toks) == 0 {
		return false, nil, nil
	}
	p := &ruleParser{toks: toks}
	root, err := p.parseOr()
	if err != nil {
		return false, nil, err
	}
	if p.pos != len(p.toks) {
		return false, nil, fmt.Errorf("unexpected %q", p.toks[p.pos].text)
	}
	found := false
	var unguarded []string
	var walk func(n *ruleNode, guarded bool)
	walk = func(n *ruleNode, guarded bool) {
		switch n.kind {
		case nodeAtom:
			if m.references(n.text) {
				found = true
				if !guarded {
					unguarded = append(unguarded, n.text)
				}
			}
		case nodeAnd:
			g := guarded
			for _, c := range n.children {
				if c.kind == nodeAtom && isAuthGuard(c.text) {
					g = true
				}
			}
			for _, c := range n.children {
				walk(c, g)
			}
		case nodeOr:
			for _, c := range n.children {
				walk(c, guarded)
			}
		}
	}
	walk(root, false)
	return found, unguarded, nil
}

func isAuthGuard(atom string) bool {
	return atom == AuthGuard || atom == `@request.auth.id != ''`
}

type tokKind int

const (
	tokAtom tokKind = iota
	tokAnd
	tokOr
	tokOpen
	tokClose
)

type ruleTok struct {
	kind tokKind
	text string
}

// tokenizeRule splits a rule into clauses, `&&`, `||` and grouping parens.
// Quoted strings and function-call parens stay inside their clause, and `//`
// comments are dropped.
func tokenizeRule(rule string) ([]ruleTok, error) {
	var toks []ruleTok
	var buf strings.Builder
	flush := func() {
		text := strings.Join(strings.Fields(buf.String()), " ")
		if text != "" {
			toks = append(toks, ruleTok{kind: tokAtom, text: text})
		}
		buf.Reset()
	}
	isIdent := func(b byte) bool {
		return b == '_' || b == ':' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}
	for i := 0; i < len(rule); i++ {
		ch := rule[i]
		switch {
		case ch == '"' || ch == '\'':
			j := i + 1
			for j < len(rule) && rule[j] != ch {
				if rule[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(rule) {
				return nil, fmt.Errorf("unterminated string")
			}
			buf.WriteString(rule[i : j+1])
			i = j
		case ch == '/' && i+1 < len(rule) && rule[i+1] == '/':
			for i < len(rule) && rule[i] != '\n' {
				i++
			}
			buf.WriteByte(' ')
		case ch == '&' && i+1 < len(rule) && rule[i+1] == '&':
			flush()
			toks = append(toks, ruleTok{kind: tokAnd})
			i++
		case ch == '|' && i+1 < len(rule) && rule[i+1] == '|':
			flush()
			toks = append(toks, ruleTok{kind: tokOr})
			i++
		case ch == '(':
			cur := strings.TrimRight(buf.String(), " \t\n")
			if cur != "" && cur == buf.String() && isIdent(cur[len(cur)-1]) {
				// A function call such as strftime(…): keep it in the clause.
				depth := 0
				j := i
				for ; j < len(rule); j++ {
					if rule[j] == '(' {
						depth++
					} else if rule[j] == ')' {
						depth--
						if depth == 0 {
							break
						}
					}
				}
				if j >= len(rule) {
					return nil, fmt.Errorf("unbalanced parens")
				}
				buf.WriteString(rule[i : j+1])
				i = j
				continue
			}
			flush()
			toks = append(toks, ruleTok{kind: tokOpen})
		case ch == ')':
			flush()
			toks = append(toks, ruleTok{kind: tokClose})
		default:
			buf.WriteByte(ch)
		}
	}
	flush()
	return toks, nil
}

type nodeKind int

const (
	nodeAtom nodeKind = iota
	nodeAnd
	nodeOr
)

type ruleNode struct {
	kind     nodeKind
	text     string
	children []*ruleNode
}

// ruleParser gives `&&` precedence over `||`, as PocketBase's filter
// grammar does.
type ruleParser struct {
	toks []ruleTok
	pos  int
}

func (p *ruleParser) parseOr() (*ruleNode, error) {
	return p.parseList(tokOr, nodeOr, p.parseAnd)
}

func (p *ruleParser) parseAnd() (*ruleNode, error) {
	return p.parseList(tokAnd, nodeAnd, p.parsePrimary)
}

func (p *ruleParser) parseList(op tokKind, kind nodeKind, next func() (*ruleNode, error)) (*ruleNode, error) {
	first, err := next()
	if err != nil {
		return nil, err
	}
	children := []*ruleNode{first}
	for p.pos < len(p.toks) && p.toks[p.pos].kind == op {
		p.pos++
		n, err := next()
		if err != nil {
			return nil, err
		}
		children = append(children, n)
	}
	if len(children) == 1 {
		return first, nil
	}
	return &ruleNode{kind: kind, children: children}, nil
}

func (p *ruleParser) parsePrimary() (*ruleNode, error) {
	if p.pos >= len(p.toks) {
		return nil, fmt.Errorf("rule ends where a clause was expected")
	}
	tok := p.toks[p.pos]
	switch tok.kind {
	case tokAtom:
		p.pos++
		return &ruleNode{kind: nodeAtom, text: tok.text}, nil
	case tokOpen:
		p.pos++
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.toks) || p.toks[p.pos].kind != tokClose {
			return nil, fmt.Errorf("missing )")
		}
		p.pos++
		return n, nil
	default:
		return nil, fmt.Errorf("unexpected operator at clause %d", p.pos)
	}
}
