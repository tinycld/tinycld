package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// layoutLegs are the group statement's computed rows: one select per
// computed row (and currency), whose cells compute the row's formula from
// the first-level group rows its sections name. A formula reads sections
// and earlier computed rows by name, never by row position.
func (c *compiler) layoutLegs(s *stmt, pivot []pivotValue) (ctes []string, legs []unionLeg, err error) {
	if len(c.p.layout) == 0 {
		return nil, nil, nil
	}
	cur := "(SELECT 1) cur"
	if len(c.currencies) > 0 {
		// With no visible rows the computed rows still come, once, as they do
		// for a report without currencies, in each key's source currency
		// (noRowCurrency) or none.
		cur = "(SELECT DISTINCT " + strings.Join(c.currencies, ", ") + " FROM o1 UNION ALL SELECT " + c.noRowCurrencies(s) +
			" WHERE NOT EXISTS (SELECT 1 FROM o1)) cur"
	}
	lc := &layoutCtx{c: c, s: s, items: map[string]*LayoutItem{}, within: map[string][]*LayoutItem{}, withinAt: map[string]map[string][]*LayoutItem{}, chunkOf: map[string]sectionSlot{}, keys: map[string]string{}, pivotCount: len(pivot)}
	for idx, cp := range c.p.cols {
		if t := c.levelTypes[cp.col.ID]; isNumeric(t) && t != Bool {
			lc.cellNames = append(lc.cellNames, fmt.Sprintf("v%d", idx))
		}
	}
	for j := range pivot {
		lc.cellNames = append(lc.cellNames, fmt.Sprintf("pv%d", j))
	}
	for idx, cp := range c.p.cols {
		if c.sectionMixed[cp.col.ID] {
			lc.cellNames = append(lc.cellNames, fmt.Sprintf("vmx%d", idx))
		}
	}
	for j := range pivot {
		if c.pivotMixed {
			lc.cellNames = append(lc.cellNames, fmt.Sprintf("pvm%d", j))
		}
	}
	// A section whose cell carries its own currency (a record's) is read
	// with that currency.
	for idx, cp := range c.p.cols {
		if c.sectionCur[cp.col.ID].sql != "" {
			lc.cellNames = append(lc.cellNames, fmt.Sprintf("vcur%d", idx))
		}
	}
	lc.perChunk = max(1, chunkColumns/max(1, len(lc.cellNames)))
	// sizes holds, by row and column id, the size of the row's cell in that
	// column; the "" column is its formula's, the cell of every pivot column.
	sizes := map[string]map[string]int{}
	compiled := 0
	// size counts an item's cells toward the layout's caps: a cell that reads
	// an earlier row is compiled with that row's cell in the same column inside
	// it, so its size grows with every reference.
	size := func(item *LayoutItem, at string) error {
		cells := c.cellSizes(item, sizes)
		sizes[item.Row] = cells
		largest := 0
		for _, n := range cells {
			largest = max(largest, n)
		}
		compiled += len(pivot) * cells[""]
		for _, cp := range c.p.cols {
			compiled += cells[cp.col.ID]
		}
		if largest > maxLayoutNodes || compiled > maxLayoutCompiled {
			return invalid(at, "the layout's computed rows are too large; reference fewer earlier rows")
		}
		return nil
	}
	sectionAt := map[string]string{}
	for i, item := range c.p.layout {
		at := fmt.Sprintf("layout.%d", i)
		if item.Section != "" {
			lc.items[item.Section] = item
			sectionAt[item.Section] = at
			continue
		}
		if err := size(item, at); err != nil {
			return nil, nil, err
		}
		// A section read inlines the within rows it holds, so it counts them.
		if item.Within != "" {
			if sizes[item.Within] == nil {
				sizes[item.Within] = map[string]int{}
			}
			for id, n := range sizes[item.Row] {
				sizes[item.Within][id] += n
			}
		}
		leg, err := lc.computedLeg(item, i, at, cur)
		if err != nil {
			return nil, nil, err
		}
		legs = append(legs, leg)
		lc.items[item.Row] = item
		// The lists only grow at their ends, so the clone keeps this row's view.
		lc.withinAt[item.Row] = maps.Clone(lc.within)
		if item.Within != "" {
			lc.within[item.Within] = append(lc.within[item.Within], item)
		}
	}
	// A section with within rows gets one more leg, its total: compiled after
	// every item, so it adds all of the section's within rows. assemble draws
	// the section's group row and subtotal from it.
	for _, sec := range c.p.layout {
		if sec.Section == "" || len(lc.within[sec.Section]) == 0 {
			continue
		}
		item := &LayoutItem{Row: sectionTotalID(sec.Section), Formula: &Node{Subtotal: sec.Section}}
		at := sectionAt[sec.Section]
		if err := size(item, at); err != nil {
			return nil, nil, err
		}
		leg, err := lc.computedLeg(item, len(c.p.layout)+len(legs), at, cur)
		if err != nil {
			return nil, nil, err
		}
		legs = append(legs, leg)
	}
	// A section with no rows has no group row of its own: assemble draws it
	// from this leg, every number cell 0 and each derived cell computed from
	// those 0 cells, as a computed row reads an empty section. Its size is
	// fixed by the columns, not by anything the layout says, so it is counted
	// but never refused: a refusal could point at nothing the author wrote.
	if len(sectionAt) > 0 {
		item := &LayoutItem{Row: emptySectionID, Formula: &Node{Subtotal: emptySectionID}}
		cells := c.cellSizes(item, sizes)
		compiled += len(pivot) * cells[""]
		for _, cp := range c.p.cols {
			compiled += cells[cp.col.ID]
		}
		leg, err := lc.computedLeg(item, len(c.p.layout)+len(legs), "layout", cur)
		if err != nil {
			return nil, nil, err
		}
		legs = append(legs, leg)
	}
	c.layoutCounted += compiled
	return lc.chunkCTEs(), legs, nil
}

// compiled counts a formula node compiled for a computed row (leafTracker).
func (lc *layoutCtx) compiled(*Node) { lc.c.layoutNodes++ }

// sectionTotalID names the leg that holds a section's total with its within
// rows; "~" is never in a layout id (idRe), so it cannot clash with a row.
func sectionTotalID(section string) string { return "~" + section }

// emptySectionID names the leg of an empty section's cells and its {subtotal}
// leaf, which is 0. A section id is never empty, so no section total is "~".
const emptySectionID = "~"

// computedLeg is the union leg of one computed row (or a section total): its
// formula compiled for every cell column, ordered by ord.
func (lc *layoutCtx) computedLeg(item *LayoutItem, ord int, at, cur string) (unionLeg, error) {
	c, s := lc.c, lc.s
	// A computed row has a figure only in a number column, so cells naming
	// any other is refused here, where the columns' types are known.
	for _, id := range item.cellIDs() {
		if t := c.levelTypes[id]; !isNumeric(t) || t == Bool {
			return unionLeg{}, invalid(at+".cells."+id, "cells name number and amount columns")
		}
	}
	lc.used, lc.wheres, lc.current = nil, nil, item
	cells := []string{fmt.Sprintf("%d AS __level", len(c.p.groups)+1), s.bind(item.Row) + " AS __row"}
	for g := range c.p.groups {
		cells = append(cells, fmt.Sprintf("NULL AS k%d", g))
	}
	for _, ck := range c.currencies {
		cells = append(cells, "cur."+ck+" AS "+ck)
	}
	// The formula is compiled once on no column, so a layout with no number
	// columns still checks it. A cells formula is not: cells name only number
	// columns (above), so the loop below compiles each one once, in its
	// column, at layout.N.formula.cells.<id>, which is what cellSizes counts.
	lc.cell, lc.typ = "", Number
	if _, err := lc.cellOf(item, at+".formula"); err != nil {
		return unionLeg{}, err
	}
	carried := make([]string, 0, len(c.p.cols))
	for idx, cp := range c.p.cols {
		if !isNumeric(c.levelTypes[cp.col.ID]) || c.levelTypes[cp.col.ID] == Bool {
			cells = append(cells, fmt.Sprintf("NULL AS v%d", idx))
			carried = append(carried, noCarriedCell(idx), noMixedCell(idx))
			continue
		}
		lc.cell, lc.typ, lc.column, lc.idx = fmt.Sprintf("v%d", idx), c.levelTypes[cp.col.ID], cp, idx
		e, err := lc.cellOf(item, at+".formula")
		if err != nil {
			return unionLeg{}, err
		}
		cells = append(cells, e.sql+" AS "+lc.cell)
		carried = append(carried, lc.carriedCell(cp, e, idx), mixedCell(e, idx))
	}
	var pivotMixed []string
	for j := range lc.pivotCount {
		lc.cell, lc.typ, lc.column, lc.idx, lc.pivotIdx = fmt.Sprintf("pv%d", j), c.levelTypes[c.p.pivot.cell.col.ID], c.p.pivot.cell, -1, j
		e, err := lc.cellOf(item, at+".formula")
		if err != nil {
			return unionLeg{}, err
		}
		cells = append(cells, e.sql+" AS "+lc.cell)
		if c.pivotMixed {
			pivotMixed = append(pivotMixed, pivotMixedCell(e, j))
		}
	}
	// A computed row points at no record: no column has a label.
	for idx := range c.p.cols {
		cells = append(cells, labelRef{}.cells(idx)...)
	}
	cells = append(append(cells, carried...), pivotMixed...)
	// A computed row has no opening row: its opening cells are blank.
	for idx := range c.p.cols {
		cells = append(cells, blankOpeningCells(idx)...)
	}
	// The level rows' last columns, so every leg of the union has the same.
	cells = append(cells, fmt.Sprintf("%d AS __ord", ord), "1 AS __own", "NULL AS __label", "NULL AS __labelcur", "NULL AS __n", "NULL AS __kids")
	if len(lc.used) > maxJoinedChunks {
		return unionLeg{}, invalid(at, "a computed row reads too many sections")
	}
	from := cur + lc.joins()
	for _, w := range lc.wheres {
		c.probe(w.at, "SELECT 1 FROM "+from+" WHERE "+w.sql)
	}
	return unionLeg{cells: cells, from: from}, nil
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
	// current is the item being compiled; within holds, by section id, the
	// within rows compiled before it, in layout order, so a section read adds
	// only the within rows above the reader.
	current *LayoutItem
	within  map[string][]*LayoutItem
	// withinAt is, by computed row id, within as it was when that row was
	// compiled, so the row inlined into a later reader has the value it shows.
	withinAt map[string]map[string][]*LayoutItem
	// pivotCount is the number of pivot cells (pv<j>) a leg compiles.
	pivotCount int
	// cell is the result column being compiled; empty for the check pass.
	cell string
	// typ is the type of that column's cells, which a subtotal keeps so the
	// money rules apply to a computed row.
	typ FieldType
	// col is that column (the pivot's cell column for a pivot cell), and idx
	// its index, -1 for a pivot cell; pivotIdx is a pivot cell's index j
	// (its cell pv<j>).
	column   *colPlan
	idx      int
	pivotIdx int
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
	// wheres are the lookups' wheres the computed row being compiled reads
	// that may be unknown (probeWhere), probed over its select's rows.
	wheres []condProbe
}

// probeWhere takes a lookup's where on the computed row being compiled,
// probed once the sections its select joins are known (layoutLegs).
func (lc *layoutCtx) probeWhere(at, sql string) bool {
	lc.wheres = append(lc.wheres, condProbe{at: at, sql: sql})
	return true
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

// cellSizes is a computed row's cell size in each column (as cellOf compiles
// it), with the earlier rows it reads inlined at their size in that column;
// the "" column is its formula's (pivot cells, and the check pass, which
// compiles the formula only and inlines an earlier row by its formula). A
// cells formula is compiled only in its own column. A derived
// cell inlines the row's own cell for every {col} it reads, through the
// derived columns it reads. A column with no figure is a NULL.
func (c *compiler) cellSizes(item *LayoutItem, rows map[string]map[string]int) map[string]int {
	in := func(id string) map[string]int {
		col := make(map[string]int, len(rows))
		for row, cells := range rows {
			col[row] = cells[id]
		}
		return col
	}
	sizes := map[string]int{"": c.p.expandedSize(item.Formula, in(""))}
	var cell func(id string) int
	cell = func(id string) int {
		if n, done := sizes[id]; done {
			return n
		}
		n := 1
		switch t := c.levelTypes[id]; {
		case !isNumeric(t) || t == Bool:
		case item.Cells[id] != nil:
			n = c.p.expandedSize(item.Cells[id], in(id))
		case c.p.derived[id]:
			// Bind refuses a cycle of columns, so the reads end.
			n = derivedSize(c.p.byID[id].col.Formula, cell)
		case item.Formula != nil:
			n = c.p.expandedSize(item.Formula, in(id))
		}
		sizes[id] = n
		return n
	}
	for _, cp := range c.p.cols {
		cell(cp.col.ID)
	}
	return sizes
}

// derivedSize counts a derived column's formula with each {col} it reads
// replaced by the row's cell in that column; scalarOnly keeps it to args.
func derivedSize(n *Node, cell func(id string) int) int {
	if n == nil {
		return 0
	}
	if n.Col != "" {
		return cell(n.Col)
	}
	size := 1
	for _, a := range n.Args {
		size += derivedSize(a, cell)
	}
	return size
}

// cellOf is item's cell in the column being compiled (lc.cell): its cells
// formula for that column, a derived column's formula over the row's own
// cells, else its formula, else blank.
func (lc *layoutCtx) cellOf(item *LayoutItem, at string) (expr, error) {
	// The check pass (no column) compiles the formula only: a cells formula
	// is compiled in its own column, so the caps count every compile of it.
	if lc.cell == "" || lc.column == nil || lc.idx < 0 {
		return lc.formulaCell(item, at)
	}
	id := lc.column.col.ID
	switch {
	case lc.c.p.percents[id] != nil:
		// A computed row's or section total's percent divides its own cell;
		// assemble fills it (sectionPercentCell).
		return expr{sql: "NULL", typ: Percent}, nil
	case item.Cells[id] != nil:
		return lc.numberCell(item.Cells[id], at+".cells."+id)
	case lc.c.p.derived[id]:
		return compileFormula(&derivedCtx{layoutCtx: lc, item: item, visiting: map[string]bool{}}, lc.column.col.Formula, at)
	}
	return lc.formulaCell(item, at)
}

// formulaCell is item's formula in the column being compiled, blank for a
// row with cells and no formula.
func (lc *layoutCtx) formulaCell(item *LayoutItem, at string) (expr, error) {
	if item.Formula == nil {
		return expr{sql: "NULL", typ: lc.typ}, nil
	}
	e, err := compileFormula(lc, item.Formula, at)
	if err != nil {
		return expr{}, err
	}
	if !isNumeric(e.typ) || e.typ == Bool {
		return expr{}, invalid(at, "a computed row's formula is a number")
	}
	return e, nil
}

// numberCell is one cells formula in the column being compiled.
func (lc *layoutCtx) numberCell(n *Node, at string) (expr, error) {
	e, err := compileFormula(lc, n, at)
	if err != nil {
		return expr{}, err
	}
	if !isNumeric(e.typ) || e.typ == Bool {
		return expr{}, invalid(at, "cells name number and amount columns")
	}
	return e, nil
}

// derivedCtx compiles a derived column on a computed row: {col Y} is the
// row's own cell in Y, compiled with lc's column state switched to Y.
type derivedCtx struct {
	*layoutCtx
	item     *LayoutItem
	visiting map[string]bool
}

func (d *derivedCtx) col(id, at string) (expr, error) {
	cp := d.c.p.byID[id]
	if cp == nil {
		return expr{}, unknownColumn(at, id)
	}
	done, err := enter(d.visiting, id, at)
	if err != nil {
		return expr{}, err
	}
	defer done()
	t := d.c.levelTypes[id]
	if !isNumeric(t) || t == Bool {
		return expr{sql: "NULL", typ: t}, nil
	}
	lc := d.layoutCtx
	cell, typ, column, idx, pivotIdx := lc.cell, lc.typ, lc.column, lc.idx, lc.pivotIdx
	// Only the column state is switched back: the sections and lookup wheres the
	// read joins (used, wheres, chunks) belong to the row's select and stay.
	defer func() { lc.cell, lc.typ, lc.column, lc.idx, lc.pivotIdx = cell, typ, column, idx, pivotIdx }()
	i := d.c.p.index(cp)
	lc.cell, lc.typ, lc.column, lc.idx = fmt.Sprintf("v%d", i), t, cp, i
	return lc.cellOf(d.item, at)
}

// inline is an earlier row's cell compiled as it was for its own leg, from
// its place in the layout: its section reads add only the within rows above
// it, never those between it and the reader (ruling 9).
func (lc *layoutCtx) inline(item *LayoutItem, at string) (expr, error) {
	current, within := lc.current, lc.within
	lc.current, lc.within = item, lc.withinAt[item.Row]
	defer func() { lc.current, lc.within = current, within }()
	return lc.cellOf(item, at)
}

// bareSection marks a {subtotal} leaf that reads a section's own rows only,
// without its within rows: the add that subtotal builds reads it. idRe allows
// letters, digits and _, so no layout id holds it.
const bareSection = "\x00bare"

func (lc *layoutCtx) subtotal(id, at string) (expr, error) {
	if id == emptySectionID {
		return lc.emptySection(), nil
	}
	bare := strings.HasSuffix(id, bareSection)
	id = strings.TrimSuffix(id, bareSection)
	item, ok := lc.items[id]
	if !ok {
		return expr{}, unknownRef(at, "unknown_subtotal", fmt.Sprintf("the layout has no section or earlier row %q", id))
	}
	if item.Row != "" {
		return lc.inline(item, at)
	}
	if lc.current != nil && lc.current.Within == id {
		return expr{}, invalid(at, fmt.Sprintf("a row inside %q cannot read that section: it would read itself", id))
	}
	// A section read adds its earlier within rows through add, so add's
	// money, currency and blank rules apply exactly as to any formula.
	if rows := lc.within[id]; !bare && len(rows) > 0 {
		args := []*Node{{Subtotal: id + bareSection}}
		for _, w := range rows {
			args = append(args, &Node{Subtotal: w.Row})
		}
		return compileFormula(lc, &Node{Op: "add", Args: args}, at)
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
	// An empty section counts as zero; a section blank because a sum lookup
	// met two currencies has no value, and neither has a row that reads it
	// (R3).
	e := expr{sql: fmt.Sprintf("COALESCE(ls%d.%s, 0)", slot.chunk, slot.column(lc.cell)), typ: lc.typ}
	if helper := lc.mixedHelper(); helper != "" {
		e.mixed = fmt.Sprintf("(ls%d.%s IS NOT NULL)", slot.chunk, slot.column(helper))
		e.sql = fmt.Sprintf("(CASE WHEN %s THEN NULL ELSE %s END)", e.mixed, e.sql)
	}
	return withCurrency(e, lc.sectionCurrency(id, slot)), nil
}

// emptySection is an empty section's cell in the column being compiled: 0,
// in the computed row's currency. A currency the section's cells carry (a
// record's) comes from a row, and an empty section has none.
func (lc *layoutCtx) emptySection() expr {
	if lc.cell == "" {
		return expr{sql: "0", typ: Number}
	}
	// A column that totals no rows is blank on every section, so on this one too.
	if lc.idx >= 0 && !lc.c.p.totalsRows(lc.column) {
		return expr{sql: "NULL", typ: lc.typ}
	}
	cur := lc.c.sectionCur[lc.column.col.ID]
	if cur.sql != "" && lc.idx >= 0 {
		cur = currency{}
	}
	return withCurrency(expr{sql: "0", typ: lc.typ}, cur)
}

// mixedHelper is the helper cell beside the cell being compiled that marks
// a section's cell blank for mixed currencies, "" when it cannot be.
func (lc *layoutCtx) mixedHelper() string {
	if lc.idx < 0 {
		if !lc.c.pivotMixed {
			return ""
		}
		return fmt.Sprintf("pvm%d", lc.pivotIdx)
	}
	if !lc.c.sectionMixed[lc.column.col.ID] {
		return ""
	}
	return fmt.Sprintf("vmx%d", lc.idx)
}

// pivotMixedCell is a computed row's pvm<j>: 1 where its pivot cell j is
// blank for mixed currencies.
func pivotMixedCell(e expr, j int) string {
	blank := e.blankFor()
	if blank == "" {
		return fmt.Sprintf("NULL AS pvm%d", j)
	}
	return fmt.Sprintf("(CASE WHEN %s THEN 1 END) AS pvm%d", blank, j)
}

// sectionCurrency is the currency of a section's cell in the column being
// compiled: the column's on a first-level group row. A currency key is the
// row's currency, which the computed row is in (one row per currency); a
// currency the section's cell carries is read from the section beside the
// cell and is that section's alone, so it combines with no other section.
func (lc *layoutCtx) sectionCurrency(id string, slot sectionSlot) currency {
	cur := lc.c.sectionCur[lc.column.col.ID]
	if cur.sql == "" || lc.idx < 0 {
		return cur
	}
	return currency{sql: fmt.Sprintf("ls%d.%s", slot.chunk, slot.column(fmt.Sprintf("vcur%d", lc.idx))), id: "sub:" + id + ":" + cur.id}
}

// carriedCell is a computed row's currency in a column, when it is not the
// row's own (the currency key the computed row is in): one its amount carries
// (a looked-up row's, a section's record), or a source's fixed currency that
// is not the column's.
func (lc *layoutCtx) carriedCell(cp *colPlan, e expr, idx int) string {
	fixed := e.cur.fixed
	if fixed == nil || lc.c.sectionCur[cp.col.ID].same(e.cur) || lc.c.dry() {
		return lc.c.carriedCell(lc.s, cp, e, idx)
	}
	lc.c.carried[cp.col.ID] = true
	return fmt.Sprintf("%s AS vcur%d", lc.s.bind(lc.c.fixedCurrency(fixed)), idx)
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
	return expr{}, invalid(at, fmt.Sprintf("a computed row reads sections' subtotals, earlier rows, params, values and lookups: its cell in %q is this formula, so reading the column would read itself", id))
}
func (lc *layoutCtx) path(_ Path, at string) (expr, error) { return expr{}, lc.only(at) }
func (lc *layoutCtx) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, lc.only(at)
}
func (lc *layoutCtx) param(id, at string) (expr, error) {
	v, err := paramOf(lc.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramLeaf(lc.c, lc.s, lc, id, v)
}
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
	return invalid(at, "a computed row reads sections' subtotals, earlier rows, params, values and lookups")
}
func (lc *layoutCtx) bind(v any) string   { return lc.s.bind(v) }
func (lc *layoutCtx) fiscalStart() string { return lc.bind(int64(lc.c.fiscal)) }
func (lc *layoutCtx) filter() bool        { return false }
func (lc *layoutCtx) label(e expr) string { return lc.c.textOf(lc.s, e) }

// currencySQL reads a currency key from the computed row's currency (cur).
func (lc *layoutCtx) currencySQL(cur currency) string {
	return currencyCode(lc, lc.c, cur, func(key string) string { return "cur." + key })
}

// layoutRows emits the rows for one currency: each section's group row
// (titled, always with its subtotal) and each computed row, in the layout's
// order. First-level groups no section names are left out.
func (rn *runner) layoutRows(t *tree, total map[string]any) {
	ck := rn.currencyKey(total)
	byKey := rn.sectionsByKey(t, ck)
	for _, item := range rn.c.p.layout {
		// A within row is emitted inside its section (groupWithLabel).
		if item.Within != "" {
			continue
		}
		if item.Row != "" {
			row := t.computed[item.Row+"|"+ck]
			if row == nil {
				row = map[string]any{}
			}
			id := "c:" + item.Row + rn.currencySuffix(total)
			rn.push(Row{ID: id, Kind: KindComputed, Label: item.Label, Cells: rn.cells(row)}, row)
			continue
		}
		row := rn.sectionRow(t, item, total, byKey)
		groupKey := rn.rowKey(row, 1)
		rn.groupWithLabel(t, row, 1, 0, "", "g:"+keyString(groupKey), rowLabel{text: item.Title}, groupKey, true, item)
	}
}

// sectionsByKey is one currency's first-level group rows by key.
func (rn *runner) sectionsByKey(t *tree, ck string) map[string]map[string]any {
	byKey := map[string]map[string]any{}
	for _, g := range t.children[ck] {
		byKey[keyString([]any{g["k0"]})] = g
	}
	return byKey
}

// sectionRow is a section's group row in one currency: its group's row, or
// a zero row for a section with no rows, with its within rows added. It may
// be the group's own row, so a caller must not change it.
func (rn *runner) sectionRow(t *tree, item *LayoutItem, total map[string]any, byKey map[string]map[string]any) map[string]any {
	ck := rn.currencyKey(total)
	key := sectionKey(item)
	row := byKey[keyString([]any{key})]
	if row == nil {
		row = map[string]any{"k0": key, "__level": int64(1)}
		for _, name := range rn.c.currencies {
			row[name] = total[name]
		}
		// A section the row cap cut stays blank: it may hold rows.
		if zero := t.computed[emptySectionID+"|"+ck]; zero != nil && !rn.sectionsCapped {
			row = rn.withinTotal(row, zero)
		}
	}
	if sum := t.computed[sectionTotalID(item.Section)+"|"+ck]; sum != nil {
		row = rn.withinTotal(row, sum)
	}
	return row
}

// withinTotal is a section's row with its number cells (and their currency
// and blank helpers) taken from the section-total leg, so the group row and
// the subtotal add the section's within rows.
func (rn *runner) withinTotal(row, total map[string]any) map[string]any {
	out := maps.Clone(row)
	for idx, cp := range rn.c.p.cols {
		// A computed leg's text and yes/no cells are NULL; the section keeps its own.
		if t := rn.c.levelTypes[cp.col.ID]; !isNumeric(t) || t == Bool {
			continue
		}
		for _, name := range []string{"v%d", "vcur%d", "vmx%d"} {
			key := fmt.Sprintf(name, idx)
			if v, ok := total[key]; ok {
				out[key] = v
			}
		}
	}
	for j := range rn.pivotValues {
		for _, name := range []string{"pv%d", "pvm%d"} {
			key := fmt.Sprintf(name, j)
			if v, ok := total[key]; ok {
				out[key] = v
			}
		}
	}
	return out
}
