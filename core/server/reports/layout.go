package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// layoutLegs are the group statement's computed rows: one select per
// computed row (and currency), whose cells compute the row's formula from
// the first-level group rows its sections name. A formula reads sections
// and earlier computed rows by name, never by row position.
func (c *compiler) layoutLegs(s *stmt, pivot []any) (ctes, legs []string, err error) {
	if len(c.p.layout) == 0 {
		return nil, nil, nil
	}
	cur := "(SELECT 1) cur"
	if len(c.currencies) > 0 {
		// With no visible rows the computed rows still come, once, with no
		// currency, as they do for a report without currencies.
		nulls := strings.TrimSuffix(strings.Repeat("NULL, ", len(c.currencies)), ", ")
		cur = "(SELECT DISTINCT " + strings.Join(c.currencies, ", ") + " FROM o1 UNION ALL SELECT " + nulls +
			" WHERE NOT EXISTS (SELECT 1 FROM o1)) cur"
	}
	lc := &layoutCtx{c: c, s: s, items: map[string]*LayoutItem{}, chunkOf: map[string]sectionSlot{}, keys: map[string]string{}}
	for idx, cp := range c.p.cols {
		if t := c.levelTypes[cp.col.ID]; isNumeric(t) && t != Bool {
			lc.cellNames = append(lc.cellNames, fmt.Sprintf("v%d", idx))
		}
	}
	for j := range pivot {
		lc.cellNames = append(lc.cellNames, fmt.Sprintf("pv%d", j))
	}
	lc.perChunk = max(1, chunkColumns/max(1, len(lc.cellNames)))
	sizes := map[string]int{}
	compiled := 0
	for i, item := range c.p.layout {
		at := fmt.Sprintf("layout.%d", i)
		if item.Section != "" {
			lc.items[item.Section] = item
			continue
		}
		lc.used = nil
		// A formula that reads an earlier row is compiled with that row
		// inside it, so its size grows with every reference, and it is
		// compiled once for every cell.
		sizes[item.Row] = c.p.expandedSize(item.Formula, sizes)
		compiled += sizes[item.Row] * (len(c.p.cols) + len(pivot))
		if sizes[item.Row] > maxLayoutNodes || compiled > maxLayoutCompiled {
			return nil, nil, invalid(at, "the layout's computed rows are too large; reference fewer earlier rows")
		}
		cells := []string{fmt.Sprintf("%d AS __level", len(c.p.groups)+1), s.bind(item.Row) + " AS __row"}
		for g := range c.p.groups {
			cells = append(cells, fmt.Sprintf("NULL AS k%d", g))
		}
		for _, ck := range c.currencies {
			cells = append(cells, "cur."+ck+" AS "+ck)
		}
		// Compiled once on no column, so a layout with no number columns
		// is still checked.
		lc.cell, lc.typ = "", Number
		if _, err := lc.row(item, at+".formula"); err != nil {
			return nil, nil, err
		}
		for idx, cp := range c.p.cols {
			if !isNumeric(c.levelTypes[cp.col.ID]) || c.levelTypes[cp.col.ID] == Bool {
				cells = append(cells, fmt.Sprintf("NULL AS v%d", idx))
				continue
			}
			lc.cell, lc.typ = fmt.Sprintf("v%d", idx), c.levelTypes[cp.col.ID]
			e, err := lc.row(item, at+".formula")
			if err != nil {
				return nil, nil, err
			}
			cells = append(cells, e.sql+" AS "+lc.cell)
		}
		for j := range pivot {
			lc.cell, lc.typ = fmt.Sprintf("pv%d", j), c.levelTypes[c.p.pivot.cell.col.ID]
			e, err := lc.row(item, at+".formula")
			if err != nil {
				return nil, nil, err
			}
			cells = append(cells, e.sql+" AS "+lc.cell)
		}
		cells = append(cells, fmt.Sprintf("%d AS __ord", i))
		if len(lc.used) > maxJoinedChunks {
			return nil, nil, invalid(at, "a computed row reads too many sections")
		}
		legs = append(legs, "SELECT "+strings.Join(cells, ", ")+" FROM "+cur+lc.joins())
		lc.items[item.Row] = item
	}
	return lc.chunkCTEs(), legs, nil
}

// maxJoinedChunks keeps a computed row's select under SQLite's limit of 64
// tables in a join. Under maxLayoutCompiled a row cannot read enough
// sections to join this many chunks (each chunk holds 1000 / cells sections,
// and each section read is compiled once per cell), so the check never fires
// today; it is a guard in case those limits change.
const maxJoinedChunks = 60

// joins joins the section CTEs the computed row being compiled reads, each
// to the row's current currency.
func (lc *layoutCtx) joins() string {
	var out strings.Builder
	for _, k := range lc.used {
		on := []string{"1"}
		for _, ck := range lc.c.currencies {
			on = append(on, fmt.Sprintf("ls%d.%s IS cur.%s", k, ck, ck))
		}
		fmt.Fprintf(&out, " LEFT JOIN ls%d ON %s", k, strings.Join(on, " AND "))
	}
	return out.String()
}

// layoutCtx compiles a computed row's formula for one cell column: a
// {subtotal} leaf is that column's value on a section's group row, or on an
// earlier computed row. items holds only the items before the one being
// compiled, so a formula cannot read itself or a later row.
type layoutCtx struct {
	ctxBase
	c     *compiler
	s     *stmt
	items map[string]*LayoutItem
	// cell is the result column being compiled; empty for the check pass.
	cell string
	// typ is the type of that column's cells, which a subtotal keeps so the
	// money rules apply to a computed row.
	typ FieldType
	// The sections a computed row reads are grouped in chunks, one CTE each
	// (see chunkCTEs), so a SELECT stays under SQLite's column limit. chunkOf
	// and keys are by section id; used lists the chunks the computed row
	// being compiled joins; cellNames are the cell columns of o1 to read.
	chunks    [][]string
	chunkOf   map[string]sectionSlot
	keys      map[string]string
	used      []int
	perChunk  int
	cellNames []string
}

// sectionSlot is where a section's cells are read: its chunk's CTE, and n,
// the order in which the layout first read it.
type sectionSlot struct {
	chunk, n int
}

// column names a section's cell in its chunk's CTE by the section's number,
// never by its id: SQLite column names ignore case, and ids "L" and "l" are
// two sections.
func (s sectionSlot) column(cell string) string {
	return fmt.Sprintf("s%d_%s", s.n, cell)
}

// chunkColumns is the most columns a section chunk's CTE holds.
const chunkColumns = 1000

// maxLayoutNodes caps the size of a computed row's formula after the rows it
// reads are inlined.
const maxLayoutNodes = 2000

// maxLayoutCompiled caps the nodes of all computed rows, each counted once
// for every cell column it is compiled for. A statement of 30 rows of about
// 10 nodes with 50 pivot cells (about 16,000) fits.
const maxLayoutCompiled = 20000

const errLayoutKey = "a section key is text, a number, true, false or null"

// expandedSize counts a formula's nodes with each earlier row it reads
// counted in full.
func (p *plan) expandedSize(n *Node, rows map[string]int) int {
	if n == nil {
		return 0
	}
	size := 1 + rows[n.Subtotal]
	for _, a := range n.Args {
		size += p.expandedSize(a, rows)
	}
	// A lookup's where and value are compiled with it; bind has refused a
	// value that does not decode.
	size += p.expandedSize(n.Where, rows)
	if n.Op == "lookup" {
		if v, err := p.lookupOf(n, ""); err == nil {
			size += p.expandedSize(v, rows)
		}
	}
	return size
}

func (lc *layoutCtx) row(item *LayoutItem, at string) (expr, error) {
	e, err := compileFormula(lc, item.Formula, at)
	if err != nil {
		return expr{}, err
	}
	if !isNumeric(e.typ) || e.typ == Bool {
		return expr{}, invalid(at, "a computed row's formula is a number")
	}
	return e, nil
}

func (lc *layoutCtx) subtotal(id, at string) (expr, error) {
	item, ok := lc.items[id]
	if !ok {
		return expr{}, unknownRef(at, "unknown_subtotal", fmt.Sprintf("the layout has no section or earlier row %q", id))
	}
	if item.Row != "" {
		return lc.row(item, at)
	}
	if lc.cell == "" {
		return expr{sql: "NULL", typ: Number}, nil
	}
	slot, ok := lc.chunkOf[id]
	if !ok {
		// A section's cells are read from o1 once, in the CTE of its chunk,
		// however many cells and computed rows use them.
		if n := len(lc.chunks); n == 0 || len(lc.chunks[n-1]) >= lc.perChunk {
			lc.chunks = append(lc.chunks, nil)
		}
		slot = sectionSlot{chunk: len(lc.chunks) - 1, n: len(lc.chunkOf)}
		lc.chunks[slot.chunk] = append(lc.chunks[slot.chunk], id)
		lc.chunkOf[id] = slot
		lc.keys[id] = lc.s.bind(sectionKey(item))
	}
	if !slices.Contains(lc.used, slot.chunk) {
		lc.used = append(lc.used, slot.chunk)
	}
	// An empty section counts as zero.
	return expr{sql: fmt.Sprintf("COALESCE(ls%d.%s, 0)", slot.chunk, slot.column(lc.cell)), typ: lc.typ}, nil
}

// chunkCTEs renders one CTE per chunk of sections. Each has one row per
// currency, with a column for every section and cell it holds: the cell of
// the o1 row whose key is the section's.
func (lc *layoutCtx) chunkCTEs() []string {
	ctes := make([]string, 0, len(lc.chunks))
	for k, ids := range lc.chunks {
		cols := slices.Clone(lc.c.currencies)
		for _, id := range ids {
			for _, cell := range lc.cellNames {
				cols = append(cols, fmt.Sprintf("MAX(CASE WHEN k0 IS %s THEN %s END) AS %s", lc.keys[id], cell, lc.chunkOf[id].column(cell)))
			}
		}
		group := ""
		if len(lc.c.currencies) > 0 {
			group = " GROUP BY " + strings.Join(lc.c.currencies, ", ")
		}
		ctes = append(ctes, fmt.Sprintf("ls%d AS (SELECT %s FROM o1%s)", k, strings.Join(cols, ", "), group))
	}
	return ctes
}

// sectionKey is a section's key as the binder accepted it.
func sectionKey(item *LayoutItem) any {
	key, _ := layoutKey(item.Key)
	return key
}

func layoutKey(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch x := v.(type) {
	case json.Number:
		return numberValue(x), nil
	case bool:
		// A bool group key is read from SQLite as 0 or 1.
		if x {
			return int64(1), nil
		}
		return int64(0), nil
	case nil, string:
		return x, nil
	}
	return nil, fmt.Errorf("not a scalar")
}

// col is refused: a computed row's cell in a column is its own formula, so
// a column read from it (even inside a lookup) would read itself.
func (lc *layoutCtx) col(id, at string) (expr, error) {
	return expr{}, invalid(at, fmt.Sprintf("a computed row reads sections' subtotals, earlier rows, values and lookups: its cell in %q is this formula, so reading the column would read itself", id))
}
func (lc *layoutCtx) path(_ Path, at string) (expr, error) { return expr{}, lc.only(at) }
func (lc *layoutCtx) param(_, at string) (expr, error)     { return expr{}, lc.only(at) }
func (lc *layoutCtx) aggregate(_ Aggregate, _ *Node, _ int, at string) (expr, error) {
	return expr{}, lc.only(at)
}
func (lc *layoutCtx) window(_ string, _ *Node, _ int, at string) (expr, error) {
	return expr{}, lc.only(at)
}
func (lc *layoutCtx) exists(_ *Node, at string) (expr, error) { return expr{}, lc.only(at) }

// lookup reads its source's rule CTE in the layout statement. There is no
// report row to read: a column inside it is refused as above, and a param
// resolves in the lookup itself.
func (lc *layoutCtx) lookup(n *Node, at string) (expr, error) {
	return lookupIn(lc, &rowsCTE{c: lc.c, s: lc.s}, n, at)
}
func (lc *layoutCtx) only(at string) error {
	return invalid(at, "a computed row reads sections' subtotals, earlier rows, values and lookups")
}
func (lc *layoutCtx) bind(v any) string   { return lc.s.bind(v) }
func (lc *layoutCtx) fiscalStart() string { return lc.bind(int64(lc.c.fiscal)) }
func (lc *layoutCtx) filter() bool        { return false }

// layoutRows emits the rows for one currency: each section's group row
// (titled, always with its subtotal) and each computed row, in the layout's
// order. First-level groups no section names are left out.
func (rn *runner) layoutRows(t *tree, total map[string]any) {
	ck := rn.currencyKey(total)
	byKey := map[string]map[string]any{}
	for _, g := range t.children[ck] {
		byKey[keyString([]any{g["k0"]})] = g
	}
	for _, item := range rn.c.p.layout {
		if item.Row != "" {
			row := t.computed[item.Row+"|"+ck]
			if row == nil {
				row = map[string]any{}
			}
			id := "c:" + item.Row + rn.currencySuffix(total)
			rn.push(Row{ID: id, Kind: KindComputed, Label: item.Label, Cells: rn.cells(row)})
			continue
		}
		key := sectionKey(item)
		row := byKey[keyString([]any{key})]
		if row == nil {
			row = map[string]any{"k0": key, "__level": int64(1)}
			for _, name := range rn.c.currencies {
				row[name] = total[name]
			}
		}
		groupKey := rn.rowKey(row, 1)
		rn.groupWithLabel(t, row, 1, "", "g:"+keyString(groupKey), item.Title, groupKey, true)
	}
}
