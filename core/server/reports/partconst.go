package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

// A union source's part may set a key to a constant (SourcePart.Values): on
// that part's rows the key is known before any row is read. A condition on
// such a key is then known on the part's rows too, and a branch it decides
// reads nothing there. partArms writes w once for each part with those
// values in place, so a part's rows join no link that only a dead branch
// reads. The rows are the same: each value written in place is the value the
// SQL it replaces has on every row of that part.

// partTruth is what a condition is on every row of one part.
type partTruth int8

const (
	partUnknown partTruth = iota
	partFalse
	partTrue
)

func (t partTruth) not() partTruth {
	switch t {
	case partFalse:
		return partTrue
	case partTrue:
		return partFalse
	}
	return partUnknown
}

// partBase is a context whose values are values of one row of w over a union
// of two or more parts: w's own select at its root (joinCtx), a detail row
// (rowsCtx), or a lookup whose report row is one of these (lookupCtx), whose
// leaves on the report's row are its outer paths and its columns. inW is true
// where the SQL the context writes sits in w's own select, which partArms
// writes once for each part; a detail row's SQL is read over w's rows, which
// mix the parts, so a detail row folds by computing its value in w
// (pushPartFold).
type partBase struct {
	c      *compiler
	s      *stmt
	src    *Source
	inW    bool
	lookup bool
}

// partBaseOf is c's partBase, nil where c's values are not of one row of a
// union of two or more parts, or the fold is off. A group, a layout row, a
// heading and an exists where have none: a group's value is computed over the
// rows of every part, and an exists where reads the linked rows, never the
// report's row (ctxBase.outerPath refuses an outer read there).
func partBaseOf(c fctx) *partBase {
	if legacySQL.noPartConstants {
		return nil
	}
	var pb *partBase
	switch x := c.(type) {
	case *joinCtx:
		if x.sc == x.r.root && len(x.prefix) == 0 && x.src == x.r.c.p.base {
			pb = &partBase{c: x.r.c, s: x.r.s, src: x.src, inW: true}
		}
	case *rowsCtx:
		pb = &partBase{c: x.r.c, s: x.r.s, src: x.r.c.p.base}
	case *lookupCtx:
		var row fctx = x
		for {
			inner, ok := row.(*lookupCtx)
			if !ok {
				break
			}
			row = inner.outer
		}
		if outer := partBaseOf(row); outer != nil {
			pb = &partBase{c: outer.c, s: outer.s, src: outer.src, inW: outer.inW, lookup: true}
		}
	}
	if pb == nil || pb.c.dry() || pb.s.app == nil || len(pb.src.Parts) < 2 {
		return nil
	}
	return pb
}

// leaf is the field of the report's row that n reads, when a part may know
// it: a Text, Select, Number or Bool field read by a path of one field (an
// outer path in a lookup), or by a column that is a path of one field. nil for
// any other node.
func (pb *partBase) leaf(n *Node) *Field {
	var f *Field
	switch {
	case n == nil || n.Op != "" || n.Record != "" || n.Param != "" || n.Value != nil:
		return nil
	case n.Col != "":
		cp := pb.c.p.byID[n.Col]
		if cp == nil || cp.path == nil || len(cp.path.hops) > 0 || cp.tm >= 0 || cp.nest > 0 {
			return nil
		}
		f = cp.path.field
	case pb.lookup:
		if len(n.Outer) == 1 && n.Outer[0].Field != "" && n.Outer[0].Link == "" {
			f = pb.src.field(n.Outer[0].Field)
		}
	case len(n.Path) == 1 && n.Path[0].Field != "" && n.Path[0].Link == "":
		f = pb.src.field(n.Path[0].Field)
	}
	if f == nil || f.Key == idField.Key {
		return nil
	}
	switch f.Type {
	case Text, Select, Number, Bool:
		return f
	}
	return nil
}

// constant is the value part i sets field f to, false where the part maps f
// to a column or sets no value (a blank).
func (pb *partBase) constant(i int, f *Field) (any, bool) {
	part := pb.src.Parts[i]
	if _, mapped := part.Columns[f.Key]; mapped {
		return nil, false
	}
	v, ok := part.Values[f.Key]
	return v, ok && v != nil
}

// truth is what condition n is on every row of part i: eq, ne and in of a
// leaf with a literal, and not, and, or over those. An and or an or that
// drops an unset param's condition keeps the truth found here: a term it
// drops is unknown, so it decides nothing.
func (pb *partBase) truth(n *Node, i int) partTruth {
	if n == nil {
		return partUnknown
	}
	switch n.Op {
	case "eq", "ne":
		if len(n.Args) != 2 {
			return partUnknown
		}
		for k := range 2 {
			if t := pb.compare(n.Args[k], n.Args[1-k], i, false); t != partUnknown {
				if n.Op == "ne" {
					return t.not()
				}
				return t
			}
		}
	case "in":
		if len(n.Args) == 2 {
			return pb.compare(n.Args[0], n.Args[1], i, true)
		}
	case "not":
		if len(n.Args) == 1 {
			return pb.truth(n.Args[0], i).not()
		}
	case "and", "or":
		if len(n.Args) < 2 {
			return partUnknown
		}
		decides, rest := partFalse, partTrue
		if n.Op == "or" {
			decides, rest = partTrue, partFalse
		}
		all := true
		for _, a := range n.Args {
			switch pb.truth(a, i) {
			case decides:
				return decides
			case rest:
			default:
				all = false
			}
		}
		if all {
			return rest
		}
	}
	return partUnknown
}

// compare is what "leaf IS lit" (with list, "leaf IN lit") is on part i's
// rows: known only when the part sets the leaf to a constant that every value
// of the literal compares with as SQL does.
func (pb *partBase) compare(leaf, lit *Node, i int, list bool) partTruth {
	f := pb.leaf(leaf)
	if f == nil || lit == nil || lit.Op != "" || lit.Value == nil {
		return partUnknown
	}
	v, ok := pb.constant(i, f)
	if !ok {
		return partUnknown
	}
	dec := json.NewDecoder(bytes.NewReader(lit.Value))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return partUnknown
	}
	items, isList := raw.([]any)
	if list != isList {
		return partUnknown
	}
	if !list {
		items = []any{raw}
	}
	found := false
	for _, item := range items {
		eq, known := partEquals(f, v, item)
		if !known {
			return partUnknown
		}
		found = found || eq
	}
	if found {
		return partTrue
	}
	return partFalse
}

// partEquals is whether a part's constant v of field f and a literal's value
// are one value as SQL's IS and IN compare them; known is false where Go
// cannot say: a constant and a literal of different kinds, a blank select
// (read as NULL), a day (a day of the reader's zone in a condition), or an
// integer past 2^53 against a fraction.
func partEquals(f *Field, v, lit any) (eq, known bool) {
	switch f.Type {
	case Text, Select:
		a, okA := v.(string)
		b, okB := lit.(string)
		if !okA || !okB || isDay(b) || (f.Type == Select && a == "") {
			return false, false
		}
		return a == b, true
	case Number:
		n, okB := lit.(json.Number)
		if !okB {
			return false, false
		}
		return numbersEqual(v, numberValue(n))
	case Bool:
		a, okA := v.(bool)
		b, okB := lit.(bool)
		if !okA || !okB {
			return false, false
		}
		return a == b, true
	}
	return false, false
}

// numbersEqual compares a constant and a literal number: exactly when both
// are integers, else as doubles, where every integer of either side is exact.
func numbersEqual(v, lit any) (eq, known bool) {
	var a int64
	var fa float64
	isInt := true
	switch x := v.(type) {
	case int:
		a = int64(x)
	case int32:
		a = int64(x)
	case int64:
		a = x
	case float64:
		fa, isInt = x, false
	case float32:
		fa, isInt = float64(x), false
	default:
		return false, false
	}
	const exact = 1 << 53
	switch b := lit.(type) {
	case int64:
		if isInt {
			return a == b, true
		}
		if b > exact || b < -exact {
			return false, false
		}
		return fa == float64(b), true
	case float64:
		if isInt {
			if a > exact || a < -exact {
				return false, false
			}
			fa = float64(a)
		}
		if math.IsNaN(fa) || math.IsNaN(b) {
			return false, false
		}
		return fa == b, true
	}
	return false, false
}

// partMark is the mark of fold n: an SQL comment, so a statement that is not
// written per part reads the span between two marks as it is.
func partMark(n int) string { return fmt.Sprintf("/*pc%d*/", n) }

// mark puts sql between two marks of a new fold whose values per part are
// per, sql itself where no part has one.
func (pb *partBase) mark(sql string, per []string) string {
	if !slices.ContainsFunc(per, func(v string) bool { return v != "" }) {
		return sql
	}
	n := len(pb.s.folds)
	pb.s.folds = append(pb.s.folds, per)
	return partMark(n) + sql + partMark(n)
}

// foldsCondition are the operators whose truth partBase.truth finds.
var foldsCondition = map[string]bool{"eq": true, "ne": true, "in": true, "not": true, "and": true, "or": true}

// partConstantFold marks condition e (n compiled in w's own select) with what
// it is on each part's rows, "1" or "0", where a part's constant decides it
// (partBase.truth). A mark wraps only SQL compiled before it, so a mark
// inside another has a lower number: partArms writes the outer one first.
func partConstantFold(c fctx, n *Node, e expr) expr {
	pb := partBaseOf(c)
	if pb == nil || !pb.inW || !foldsCondition[n.Op] || e.typ != Bool || e.unset {
		return e
	}
	per := make([]string, len(pb.src.Parts))
	for i := range per {
		switch pb.truth(n, i) {
		case partTrue:
			per[i] = "1"
		case partFalse:
			per[i] = "0"
		}
	}
	e.sql = pb.mark(e.sql, per)
	return e
}

// picks is, for each result of an if or a case (its args at the result
// positions), the parts whose constant never picks it: an if's then where its
// condition is false, its else where it is true; a case's result where its
// own condition is false or an earlier one is true, its else where an
// earlier one is true. nil when no part decides a condition.
func (pb *partBase) picks(n *Node) map[int][]bool {
	parts := len(pb.src.Parts)
	out := map[int][]bool{}
	dead := func(arg, part int) {
		if out[arg] == nil {
			out[arg] = make([]bool, parts)
		}
		out[arg][part] = true
	}
	for i := range parts {
		switch n.Op {
		case "if":
			if len(n.Args) != 3 {
				return nil
			}
			switch pb.truth(n.Args[0], i) {
			case partTrue:
				dead(2, i)
			case partFalse:
				dead(1, i)
			}
		case "case":
			picked := false
			for k := 0; k+1 < len(n.Args); k += 2 {
				t := pb.truth(n.Args[k], i)
				if picked || t == partFalse {
					dead(k+1, i)
				}
				picked = picked || t == partTrue
			}
			if len(n.Args)%2 == 1 && picked {
				dead(len(n.Args)-1, i)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// partBranches marks each result of an if or a case compiled in w's own
// select that a part's constant never picks (picks) as NULL on that part's
// rows: the CASE never reads it there, so the joins only it reads are not
// written in that part's select. Its shown form is marked too.
func partBranches(c fctx, n *Node, args []expr) []expr {
	if n.Op != "if" && n.Op != "case" {
		return args
	}
	pb := partBaseOf(c)
	if pb == nil || !pb.inW {
		return args
	}
	picks := pb.picks(n)
	if picks == nil {
		return args
	}
	out := slices.Clone(args)
	for k, dead := range picks {
		if k >= len(out) {
			continue
		}
		per := make([]string, len(dead))
		for i, d := range dead {
			if d {
				per[i] = "NULL"
			}
		}
		out[k].sql = pb.mark(out[k].sql, per)
		if out[k].shown != "" {
			out[k].shown = pb.mark(out[k].shown, per)
		}
	}
	return out
}

// pushOps are the operators an if or a case that a detail row computes in w
// may hold (pushPartFold): each compiles to the same SQL over w's rows as over
// a detail row.
var pushOps = map[string]bool{
	"eq": true, "ne": true, "in": true, "not": true, "and": true, "or": true,
	"lt": true, "lte": true, "gt": true, "gte": true,
	"if": true, "case": true, "coalesce": true, "isEmpty": true,
}

// pushPartFold computes, for a detail row, an if or a case whose branch a
// part's constant decides inside w, as w's own select compiles it (joinCtx),
// and reads it from w: a detail row reads w's rows of every part, where
// partArms writes w once for each part with the branch decided. It applies
// only to a node that reads the row's own values (pushable); any other node,
// or a compile that fails there, is compiled on the detail row as before.
func pushPartFold(c fctx, n *Node, at string, depth int, count *int) (expr, bool) {
	rc, ok := c.(*rowsCtx)
	if !ok || (n.Op != "if" && n.Op != "case") {
		return expr{}, false
	}
	pb := partBaseOf(rc)
	if pb == nil || pb.picks(n) == nil || !pb.pushable(n) {
		return expr{}, false
	}
	j := rc.r.join(false)
	j.visiting = rc.visiting
	// compileNode counted n already; the compile in w counts it again.
	spent := *count
	*count--
	e, err := compileNode(j, n, at, depth, count)
	if err != nil {
		*count = spent
		return expr{}, false
	}
	rc.reads.value()
	out := expr{sql: rc.r.pushed(e.sql), typ: e.typ, options: e.options, target: e.target}
	if e.shown != "" {
		out.shown = rc.r.pushed(e.shown)
	}
	return out, true
}

// pushed puts a value a detail row reads in w, once, and returns its
// reference.
func (r *rowsCTE) pushed(sql string) string {
	key := "fold:" + sql
	if name, ok := r.byExpr[key]; ok {
		return "w." + name
	}
	name := fmt.Sprintf("pf%d", len(r.order))
	r.byExpr[key] = name
	return r.add(name, sql)
}

// pushable is true when n reads only literals and the row's own values of a
// type both contexts write alike (pushType), through pushOps: no param, no
// lookup, no total, no running value, no record and no text operator, whose
// SQL differs between w and a detail row.
func (pb *partBase) pushable(n *Node) bool {
	switch {
	case n == nil || n.Record != "" || n.Param != "" || n.Outer != nil || n.Subtotal != "" ||
		n.GroupKey != "" || n.PivotBound != "" || n.Link != nil || n.Where != nil || n.Source != "" || n.Bound != "":
		return false
	case n.Value != nil:
		return n.Op == ""
	case n.Col != "":
		cp := pb.c.p.byID[n.Col]
		return n.Op == "" && cp != nil && cp.path != nil && cp.tm < 0 && cp.nest == 0 && pushType(cp.path.field)
	case n.Path != nil:
		if n.Op != "" {
			return false
		}
		bound, err := bindValuePath(pb.c.p.app, pb.src, n.Path, "", pathValue)
		return err == nil && pushType(bound.field)
	}
	if !pushOps[n.Op] {
		return false
	}
	for _, a := range n.Args {
		if !pb.pushable(a) {
			return false
		}
	}
	return true
}

// pushType is a field type whose value reads alike in w and on a detail row:
// no amount or percent (a currency key), no date-time (a zoned day) and no
// relation (a link's label and its eq).
func pushType(f *Field) bool {
	if f == nil {
		return false
	}
	switch f.Type {
	case Text, Select, Number, Bool, Date:
		return true
	}
	return false
}

// partArms writes w's select over a union scope's rows one part at a time,
// "" where it does not apply or where no part's select drops a join. sel is the select list and where the condition
// (blank for none), each as w compiled it. In each part's select, each span a
// part's constant decides (partConstantFold, partBranches) is the value it
// has there, and a LEFT JOIN of a link that nothing left in the select reads
// is not written. Each select reads its part from the part's own rule CTE,
// inside the part's bounds, as boundedFrom does. The rows are the same: a
// part's rows are its rows of the union, and a left join on its target's id
// never adds or drops a row.
func (sc *scope) partArms(sel, where string) string {
	if legacySQL.noPartConstants || len(sc.src.Parts) < 2 || sc.s.app == nil {
		return ""
	}
	var marks []int
	for n := range sc.s.folds {
		mark := partMark(n)
		if strings.Contains(sel, mark) || strings.Contains(where, mark) {
			marks = append(marks, n)
		}
	}
	if len(marks) == 0 {
		return ""
	}
	// The outer mark first: a mark wraps only marks with lower numbers.
	slices.Reverse(marks)
	keys := sc.src.unionKeys()
	tms := make([]string, len(sc.tms))
	for i, tm := range sc.tms {
		tms[i] = tm.render()
	}
	parts := len(sc.src.Parts)
	sels, wheres, joins := make([]string, parts), make([]string, parts), make([][]string, parts)
	drops := false
	for i := range sc.src.Parts {
		fold := func(text string) string {
			for _, n := range marks {
				mark := partMark(n)
				if v := sc.s.folds[n][i]; v != "" {
					text = foldSpans(text, mark, v)
				}
				text = strings.ReplaceAll(text, mark, "")
			}
			return text
		}
		sels[i], wheres[i] = fold(sel), fold(where)
		joins[i] = sc.armJoins(sels[i]+"\n"+wheres[i], tms)
		drops = drops || len(joins[i]) < len(sc.sql)
	}
	// A fold that leaves every part every join saves no read: w stays one
	// select, whose marks are comments.
	if !drops {
		return ""
	}
	arms := make([]string, parts)
	for i, part := range sc.src.Parts {
		from := "(SELECT " + unionPartCols(sc.s.app, sc.src, i, keys, sc.s.params) + " FROM " + sc.s.rule(part.Collection) + " p"
		if !legacySQL.noPartBounds {
			if w := partWhere(sc.s.app, part, sc.partBounds); w != "" {
				from += " WHERE " + w
			}
		}
		lines := append([]string{"SELECT " + sels[i], "FROM " + from + ") " + sc.alias}, joins[i]...)
		lines = append(lines, tms...)
		if wheres[i] != "" {
			lines = append(lines, "WHERE "+wheres[i])
		}
		arms[i] = strings.Join(lines, "\n")
	}
	return strings.Join(arms, "\nUNION ALL\n")
}

// foldSpans writes value in place of each span of text between two marks.
func foldSpans(text, mark, value string) string {
	for {
		start := strings.Index(text, mark)
		if start < 0 {
			return text
		}
		end := strings.Index(text[start+len(mark):], mark)
		if end < 0 {
			return text
		}
		text = text[:start] + value + text[start+len(mark)+end+len(mark):]
	}
}

// armJoins are the scope's joins a part's select writes: each but a LEFT
// JOIN of a link whose alias nothing else of the select reads (text, the
// to-many subqueries tms and the joins kept).
func (sc *scope) armJoins(text string, tms []string) []string {
	kept := make([]bool, len(sc.sql))
	refs := make([]*regexp.Regexp, len(sc.sql))
	for i, j := range sc.sql {
		kept[i] = true
		if j.alias != "" {
			refs[i] = regexp.MustCompile(`\b` + regexp.QuoteMeta(j.alias) + `\.`)
		}
	}
	reads := func(ref *regexp.Regexp, skip int) bool {
		if ref.MatchString(text) || slices.ContainsFunc(tms, ref.MatchString) {
			return true
		}
		for i, j := range sc.sql {
			if i != skip && kept[i] && ref.MatchString(j.sql) {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for i := range sc.sql {
			if kept[i] && refs[i] != nil && !reads(refs[i], i) {
				kept[i], changed = false, true
			}
		}
	}
	var out []string
	for i, j := range sc.sql {
		if kept[i] {
			out = append(out, j.sql)
		}
	}
	return out
}
