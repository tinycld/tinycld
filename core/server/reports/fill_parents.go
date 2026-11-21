package reports

import (
	"fmt"
	"slices"
	"strings"
)

// A filled record group whose every group above reads only that record (the
// account's type above the account) lets each record name its own parents: a
// record with no base row then shows under its own parent rows, also when no
// base row gave the report that parent (a section with no line in the period).
// The parents no longer come from w: fr<L> holds each fill record with the
// keys of the groups above computed from the record, and every level from the
// first to L unites its keys in w with fr<L>'s.

// ownParents is the plan of such a report: level is the filled record group's
// level (2 or more), and keys holds, for each group above it, the group's key
// as a formula on the record.
type ownParents struct {
	level int
	keys  []*Node
}

// ownParentsOf returns the plan when the first filled group is a record group
// below at least one group, and every group above it has no bucket, no shown
// form, is not an amount, and reads only the record. types are the group
// keys' types. nil leaves every fill as it is: under each parent row.
func (c *compiler) ownParentsOf(types []FieldType) *ownParents {
	f := slices.IndexFunc(c.p.groups, func(g *groupPlan) bool { return g.group.Fill != nil })
	if f < 1 || c.groupTargets[f] == nil {
		return nil
	}
	link := c.p.groups[f].col.col.Path
	if len(link) == 0 || slices.ContainsFunc(link, func(h Hop) bool { return h.Link == "" || h.Mode != "" }) {
		return nil
	}
	own := &ownParents{level: f + 1}
	for i, g := range c.p.groups[:f] {
		if g.group.Bucket != "" || c.groupShown[i] || types[i] == Money {
			return nil
		}
		key, ok := c.onRecord(&Node{Col: g.col.col.ID}, link, map[string]bool{})
		if !ok {
			return nil
		}
		own.keys = append(own.keys, key)
	}
	return own
}

// onRecord rewrites a row formula as one on the record link points at: a path
// through link loses that prefix, and a {col} becomes its column's own
// rewrite. It is false for a formula that reads anything but the record,
// params and values: another path, a total, a running value, linked rows, a
// lookup, a group's record, a subtotal.
func (c *compiler) onRecord(n *Node, link Path, seen map[string]bool) (*Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Record != "" || n.Subtotal != "" || n.Source != "" || n.GroupKey != "" || n.PivotBound != "" ||
		len(n.Link) > 0 || len(n.Outer) > 0 || n.Where != nil {
		return nil, false
	}
	switch n.Op {
	case "sum", "count", "countDistinct", "avg", "min", "max", "exists", "notExists",
		"runningSum", "percentOfParent", "percentOfTotal", "percentOfSection":
		return nil, false
	}
	if n.Col != "" {
		cp := c.p.byID[n.Col]
		if cp == nil || seen[n.Col] || cp.col.Aggregate != "" || cp.nest != 0 {
			return nil, false
		}
		if len(cp.col.Path) > 0 {
			return c.onRecord(&Node{Path: cp.col.Path}, link, seen)
		}
		seen[n.Col] = true
		defer delete(seen, n.Col)
		return c.onRecord(cp.col.Formula, link, seen)
	}
	out := *n
	if len(n.Path) > 0 {
		if len(n.Path) <= len(link) || slices.ContainsFunc(n.Path, func(h Hop) bool { return h.Mode != "" }) {
			return nil, false
		}
		for i, h := range link {
			if n.Path[i].Link != h.Link {
				return nil, false
			}
		}
		out.Path = n.Path[len(link):]
		return &out, true
	}
	out.Args = make([]*Node, 0, len(n.Args))
	for _, a := range n.Args {
		arg, ok := c.onRecord(a, link, seen)
		if !ok {
			return nil, false
		}
		out.Args = append(out.Args, arg)
	}
	return &out, true
}

// ownKeys compiles the keys of the groups above level n on the fill record
// (scope sc), as w holds them: a skip level's blank key is NULL. nil when the
// records of level n do not name their own parents.
func (c *compiler) ownKeys(r *rowsCTE, sc *scope, target *Source, n int) ([]expr, error) {
	if c.fillOwn == nil || c.fillOwn.level != n {
		return nil, nil
	}
	keys := make([]expr, 0, n-1)
	for i, node := range c.fillOwn.keys {
		g := c.p.groups[i]
		j := &joinCtx{r: r, sc: sc, src: target, visiting: map[string]bool{}}
		e, err := compileFormula(j, node, g.at+".col")
		if err != nil {
			return nil, err
		}
		if g.group.Blank == GroupBlankSkip {
			e.sql = "NULLIF(" + e.sql + ", '')"
		}
		keys = append(keys, e)
	}
	return keys, nil
}

// fillRecords names the CTE that holds the fill records of level n with their
// own parents' keys.
func fillRecords(n int) string {
	return fmt.Sprintf("fr%d", n)
}

// ownFillKeys is the keyset of level m (1 to n) when the records of level n
// name their own parents: its keys in w, and those of the fill records whose
// enclosing groups are open. A record under a closed group makes that group's
// row and no row of its own.
func (c *compiler) ownFillKeys(s *stmt, m, n int, expand *expansion) string {
	keys := make([]string, 0, m+len(c.currencies))
	for i := range m {
		keys = append(keys, fmt.Sprintf("k%d", i))
	}
	cols := strings.Join(append(keys, c.currencies...), ", ")
	return fmt.Sprintf("SELECT %s FROM w UNION SELECT %s FROM %s WHERE %s",
		cols, cols, fillRecords(n), c.fillsUnder(s, m-1, expand))
}

// fillsUnder is the condition on a parent row (depth groups deep) under which
// its fill records are computed: the row shows and is open. A report with a
// "rows" column computes them under every parent: a closed group's cell adds
// its rows, as it adds its base rows, so the group reads the same closed and
// open.
func (c *compiler) fillsUnder(s *stmt, depth int, expand *expansion) string {
	if slices.ContainsFunc(c.p.cols, func(cp *colPlan) bool { return cp.col.Total == TotalRows }) {
		return "1"
	}
	return expand.mayShowUnder(c, s, depth, "")
}

// ownParentCurrencies are the currencies the fill records that name their own
// parents sit under: those of the base rows, or the sources' when there is no
// base row (parentKeys). When a currency key has no source currency, a run
// with no base row gives the records no parent: the parent rows would name no
// currency, and their "rows" cells would add the records' months of several
// currencies into one figure with no code and no notice (totalOverFillRows).
func (c *compiler) ownParentCurrencies(s *stmt, expand *expansion) string {
	if slices.ContainsFunc(c.currencies, func(ck string) bool { return c.noRowCurrency(ck) == nil }) {
		return fmt.Sprintf("SELECT DISTINCT %s FROM w", strings.Join(c.currencies, ", "))
	}
	return c.parentKeys(s, 1, expand)
}

// filled is true when level n's rows include keys with no base row: its group
// fills in its records, or a filled group below names it as a parent.
func (c *compiler) filled(n int) bool {
	if n < 1 || n > len(c.p.groups) {
		return false
	}
	return c.p.groups[n-1].group.Fill != nil || (c.fillOwn != nil && n < c.fillOwn.level)
}
