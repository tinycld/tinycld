package reports

import (
	"fmt"
	"slices"
	"strings"
)

// A record group with a fill shows the target's records that have no base
// rows too (R6): under each parent row, every record that matches the fill's
// where, under every currency the report shows. Level L's keyset f<L> is its
// base keys united with its parent rows crossed with those records; a<L> adds
// the keys with no base row as rows whose totals are those of no rows.

// fillRows are a<L>'s rows for the keys of the level's keyset that have no
// base row (fill records). Their totals are those of no rows (a sum and a
// count 0, a min blank): the level's aggregates over an empty select of w,
// which returns one row, whatever an aggregate reads.
//
// The keys with no base row are the keyset's less w's, by EXCEPT (which
// matches blank keys as equal), so w's rows are read once: a NOT EXISTS
// over w per key would read the base source again for every key.
func (lv *level) fillRows() string {
	keys := strings.Join(lv.keys, ", ")
	empty := fmt.Sprintf("SELECT %s FROM w WHERE 0", strings.Join(append(slices.Clone(lv.aggs), "count(*) AS __n"), ", "))
	return fmt.Sprintf("SELECT %s, e.* FROM (SELECT %s FROM %s EXCEPT SELECT %s FROM w) kx CROSS JOIN (%s) e",
		strings.Join(prefixed("kx.", lv.keys), ", "), keys, lv.c.fillSets[lv.n], keys, empty)
}

// parentKeys selects the parent rows of level n: the currencies for level 1
// (or one no-currency row when there are no rows); for a deeper level the
// rows of level n-1 that show and are open, its fill rows included. A folded
// or collapsed row, or one under a folded ancestor, shows no children, so its
// fill records are never computed.
func (c *compiler) parentKeys(s *stmt, n int, expand *expansion) string {
	if n == 1 {
		if len(c.currencies) == 0 {
			return "SELECT 1 AS __one"
		}
		cks := strings.Join(c.currencies, ", ")
		nulls := strings.TrimSuffix(strings.Repeat("NULL, ", len(c.currencies)), ", ")
		return fmt.Sprintf("SELECT DISTINCT %s FROM w UNION ALL SELECT %s WHERE NOT EXISTS (SELECT 1 FROM w)", cks, nulls)
	}
	expanded := expand.shownUnder(c, s, n-1, "")
	if set, ok := c.fillSets[n-1]; ok {
		return fmt.Sprintf("SELECT * FROM %s WHERE %s", set, expanded)
	}
	keys := make([]string, 0, n-1+len(c.currencies))
	for i := range n - 1 {
		keys = append(keys, fmt.Sprintf("k%d", i))
	}
	return fmt.Sprintf("SELECT DISTINCT %s FROM w WHERE %s", strings.Join(append(keys, c.currencies...), ", "), expanded)
}

// fillKeys selects level n's keys: those of its base rows, and each parent row
// joined with the target's records that match the fill's where. The target's
// records are read through its rule CTE, so a record the reader cannot list
// never fills in.
//
// The join is a CROSS JOIN, so p is always the outer loop. p is the
// currencies, or the expanded rows of the level above, and the target's
// records are a table SQLite can search by the fill's where, or index for it.
// With a plain join SQLite may put the records outer (it reads them in id
// order for the UNION) and run p, a read of w or of the keyset above, again
// for each record: records times base rows.
func (c *compiler) fillKeys(r *rowsCTE, n int, expand *expansion, at string) (string, error) {
	target := c.groupTargets[n-1]
	if target == nil {
		return "", invalid(at, "only a group by a link to one record shows records with no rows")
	}
	sc := newScope(r.s, target)
	where, mixed := "", ""
	if w := c.p.groups[n-1].group.Fill.Where; w != nil {
		j := &joinCtx{r: r, sc: sc, src: target, isFilter: true, visiting: map[string]bool{}}
		e, err := compileWhere(&fillCtx{joinCtx: j, n: n}, w, at)
		if err != nil {
			return "", err
		}
		// A blank condition is NULL, which WHERE drops as it drops false.
		if !e.unset {
			where = "\nWHERE " + e.sql
		}
		if e.condMixed != "" {
			mixed = e.condMixed
		}
	}
	keys := make([]string, 0, n+len(c.currencies))
	picked := make([]string, 0, n+len(c.currencies))
	for i := range n - 1 {
		keys = append(keys, fmt.Sprintf("k%d", i))
		picked = append(picked, fmt.Sprintf("p.k%d AS k%d", i, i))
	}
	keys = append(keys, fmt.Sprintf("k%d", n-1))
	picked = append(picked, q(sc.alias, "id")+fmt.Sprintf(" AS k%d", n-1))
	for _, ck := range c.currencies {
		keys = append(keys, ck)
		picked = append(picked, "p."+ck+" AS "+ck)
	}
	from := strings.TrimPrefix(sc.render(), "FROM ")
	parents := c.parentKeys(r.s, n, expand)
	if mixed != "" {
		c.probe(c.p.groups[n-1].at+".fill.where", fmt.Sprintf("SELECT 1 FROM (%s) p\nCROSS JOIN %s\nWHERE %s", parents, from, mixed))
	}
	return fmt.Sprintf("SELECT %s FROM w UNION SELECT %s\nFROM (%s) p\nCROSS JOIN %s%s",
		strings.Join(keys, ", "), strings.Join(picked, ", "), parents, from, where), nil
}

// nextFill is true when the group below level n fills in its records, so a
// level-n row with no base rows still has rows to open onto.
func (c *compiler) nextFill(n int) bool {
	return n < len(c.p.groups) && c.p.groups[n].group.Fill != nil
}

// fillCtx compiles a fill's where: the target's paths (joinCtx on its scope),
// and the parent row p: {col} naming an enclosing group's column reads that
// parent's key, {record} naming an enclosing record group reads its record.
type fillCtx struct {
	*joinCtx
	n int
}

func (f *fillCtx) col(id, at string) (expr, error) {
	c := f.r.c
	for i := range f.n - 1 {
		g := c.p.groups[i]
		if g.col.col.ID != id {
			continue
		}
		key := fmt.Sprintf("p.k%d", i)
		// A bucketed group's key is its bucket's text (2026-01), not a day.
		if g.group.Bucket != "" {
			return expr{sql: key, typ: Text}, nil
		}
		e, err := f.r.rowValue(g.col, at, map[string]bool{})
		if err != nil {
			return expr{}, err
		}
		e.sql = key
		// A carried currency was read inside w; the parent row holds it as
		// the currency key the group splits by.
		if e.cur.sql != "" {
			ck, ok := c.valueCur[e.cur.id]
			if !ok {
				return expr{}, invalid(at, "this amount's currency is read per row, and the group has no currency key to group it by")
			}
			e.cur = currency{key: ck, id: e.cur.id}
		}
		return e, nil
	}
	return expr{}, invalid(at, "a fill's where reads its records' fields, params, and the keys of the groups above it")
}

func (f *fillCtx) record(group string, p Path, at string) (expr, error) {
	c := f.r.c
	gi, target, err := c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	if gi >= f.n-1 {
		return expr{}, invalid(at, "a fill's where reads the records of the groups above it")
	}
	bound, err := bindRecordPath(c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	key := fmt.Sprintf("p.k%d", gi)
	e := recordExpr(bound)
	e.sql = recordSQL(f.r.s, target, bound, key)
	e.cur = recordCurrency(f.r.s, target, bound, key, recordCurrencyID(group, p))
	return e, nil
}

func (f *fillCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(f, f.r, n, at) }

// currencySQL reads a currency key on the parent row p, as col reads its keys.
func (f *fillCtx) currencySQL(cur currency) string {
	return currencyCode(f, f.r.c, cur, func(key string) string { return "p." + key })
}
