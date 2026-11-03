package reports

import "encoding/json"

// resolveAliases replaces an aliased link or field key on every path with
// its current key. It reads every registered source, refused or not: Load
// has no app, and bind refuses a source the app refused. A key it cannot
// place (an unknown source, a link that does not exist) is left for Validate
// to report.
func resolveAliases(spec *Spec) []Alias {
	base := lookupRegistered(spec.Source)
	if base == nil {
		return nil
	}
	a := &aliaser{base: base, targets: map[string]*Source{}}
	for i := range spec.Columns {
		c := &spec.Columns[i]
		// A label field is a field of the record the path points at.
		if t := a.path(base, c.Path); t != nil && c.LabelField != "" {
			c.LabelField = a.key(t, c.LabelField)
		}
	}
	// Group targets come before formulas, since a formula's record leaf reads
	// one; a group column's formula is resolved later, so pathTarget reads
	// links by old keys too.
	a.findTargets(spec)
	for i := range spec.Columns {
		a.node(base, spec.Columns[i].Formula)
	}
	a.node(base, spec.Filter)
	for i := range spec.Groups {
		g := &spec.Groups[i]
		a.node(base, g.Label)
		a.node(base, g.Having)
		if g.Fill != nil {
			// A fill's where reads the records the group links to.
			a.node(a.targets[g.ID], g.Fill.Where)
		}
	}
	for i := range spec.Sort {
		a.node(base, spec.Sort[i].Formula)
	}
	if spec.Detail != nil {
		a.node(base, spec.Detail.Label)
	}
	for i := range spec.Layout {
		a.node(base, spec.Layout[i].Formula)
	}
	if pv := spec.Pivot; pv != nil {
		if t := a.path(base, pv.Path); t != nil && pv.LabelField != "" {
			pv.LabelField = a.key(t, pv.LabelField)
		}
	}
	for i := range spec.Params {
		// An options source's paths and where read that source, not base.
		of := spec.Params[i].OptionsFrom
		if of == nil {
			continue
		}
		if src := lookupRegistered(of.Source); src != nil {
			a.path(src, of.Value)
			a.path(src, of.Label)
			a.path(src, of.Group)
			a.node(src, of.Where)
		}
	}
	return a.out
}

// aliaser resolves aliased keys in place and records each one. base is the
// report's source; targets maps a group id to the source its key links to,
// for record leaves and fill.
type aliaser struct {
	base    *Source
	targets map[string]*Source
	out     []Alias
}

// findTargets fills targets. A record leaf may name an earlier group, so
// groups are read in order; with two groups of one id the first counts, as
// in the compiler (groupIndex).
func (a *aliaser) findTargets(spec *Spec) {
	cols := map[string]*Column{}
	for i := range spec.Columns {
		cols[spec.Columns[i].ID] = &spec.Columns[i]
	}
	for _, g := range spec.Groups {
		if _, done := a.targets[g.ID]; done {
			continue
		}
		var target *Source
		// A bucketed date has no record (rows.go reads e.target only unbucketed).
		if c := cols[g.Col]; c != nil && g.Bucket == "" {
			target = a.targetOf(c, cols, map[string]bool{})
		}
		a.targets[g.ID] = target
	}
}

// path resolves a path's keys in place and returns the source the path ends
// on, or nil when a hop cannot be followed.
func (a *aliaser) path(src *Source, path Path) *Source {
	for i := range path {
		if src == nil {
			return nil
		}
		hop := &path[i]
		if hop.Link != "" {
			hop.Link = a.key(src, hop.Link)
			l := src.link(hop.Link)
			if l == nil {
				return nil
			}
			src = lookupRegistered(l.Target)
			if src == nil {
				return nil
			}
		}
		if hop.Field != "" {
			hop.Field = a.key(src, hop.Field)
		}
	}
	return src
}

func (a *aliaser) key(src *Source, key string) string {
	if src.field(key) != nil || src.link(key) != nil {
		return key
	}
	to, ok := src.Aliases[key]
	if !ok {
		return key
	}
	a.out = append(a.out, Alias{Source: src.Key, From: key, To: to})
	return to
}

// node resolves a formula's paths on src. A record leaf's path is on its
// group's target, a lookup's on the source it looks up, and an exists where's
// on the source its link path ends on.
func (a *aliaser) node(src *Source, n *Node) {
	if n == nil {
		return
	}
	switch {
	case n.Op == "lookup":
		a.lookup(n)
		return
	case n.Record != "":
		a.path(a.targets[n.Record], n.Path)
	case src != nil:
		a.path(src, n.Path)
	}
	for _, arg := range n.Args {
		a.node(src, arg)
	}
	if n.Link != nil {
		a.node(a.path(src, n.Link), n.Where)
	}
}

// lookup resolves a lookup's value and where against the source it looks
// up. The value is kept as raw JSON, so it is encoded again only when a key
// in it changed.
func (a *aliaser) lookup(n *Node) {
	src := lookupRegistered(n.Source)
	if v, err := lookupValue(n, ""); err == nil {
		before := len(a.out)
		a.node(src, v)
		if len(a.out) > before {
			if raw, err := json.Marshal(v); err == nil {
				n.Value = raw
			}
		}
	}
	a.node(src, n.Where)
}

// targetOf is the source a group column's value links to, as the compiler
// finds it (expr.target): a path that ends at a to-one link or at id, a
// column it names, a record leaf, a lookup's value, or the results of if,
// case and coalesce. nil when it links nowhere. A formula's keys may still be
// old here (pathTarget).
func (a *aliaser) targetOf(c *Column, cols map[string]*Column, seen map[string]bool) *Source {
	if seen[c.ID] {
		return nil
	}
	seen[c.ID] = true
	if c.Path != nil {
		return pathTarget(a.base, c.Path)
	}
	return a.nodeTarget(a.base, c.Formula, cols, seen)
}

func (a *aliaser) nodeTarget(src *Source, n *Node, cols map[string]*Column, seen map[string]bool) *Source {
	switch {
	case n == nil:
		return nil
	case n.Record != "":
		return pathTarget(a.targets[n.Record], n.Path)
	case n.Path != nil:
		return pathTarget(src, n.Path)
	case n.Col != "":
		// A column reads the report's row, inside a lookup's value too.
		if c := cols[n.Col]; c != nil {
			return a.targetOf(c, cols, seen)
		}
	case n.Op == "lookup":
		if v, err := lookupValue(n, ""); err == nil {
			return a.nodeTarget(lookupRegistered(n.Source), v, cols, seen)
		}
	case n.Op == "if":
		// The first argument is the condition, which links nowhere.
		if len(n.Args) > 1 {
			return a.firstTarget(src, n.Args[1:], cols, seen)
		}
	case n.Op == "case":
		return a.firstTarget(src, caseResults(n.Args), cols, seen)
	case n.Op == "coalesce":
		return a.firstTarget(src, n.Args, cols, seen)
	}
	return nil
}

// caseResults are case's results, as the compiler reads them: the second of
// each condition and result pair, then an else (the last of an odd count).
func caseResults(args []*Node) []*Node {
	var out []*Node
	for i := 0; i+1 < len(args); i += 2 {
		out = append(out, args[i+1])
	}
	if len(args)%2 == 1 {
		out = append(out, args[len(args)-1])
	}
	return out
}

// firstTarget is the first target among results; the compiler refuses
// results that link to different sources.
func (a *aliaser) firstTarget(src *Source, args []*Node, cols map[string]*Column, seen map[string]bool) *Source {
	for _, arg := range args {
		if t := a.nodeTarget(src, arg, cols, seen); t != nil {
			return t
		}
	}
	return nil
}

// pathTarget is the source a value path points at, as bind finds it
// (pathPlan.target): the end of to-one links, or the source whose id it
// reads. A to-many hop makes a total, which points nowhere. A link may still
// have its old key.
func pathTarget(src *Source, path Path) *Source {
	for _, hop := range path {
		if src == nil {
			return nil
		}
		if hop.Link != "" {
			l := src.link(hop.Link)
			if l == nil {
				l = src.link(src.Aliases[hop.Link])
			}
			if l == nil || l.Cardinality != ToOne {
				return nil
			}
			src = lookupRegistered(l.Target)
			continue
		}
		if hop.Field == idField.Key {
			return src
		}
		return nil
	}
	return src
}
