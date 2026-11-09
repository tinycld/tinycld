package reports

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// columns describes the result's columns, pivot cell columns last.
func (rn *runner) columns() []ResultColumn {
	c := rn.c
	out := make([]ResultColumn, 0, len(c.p.cols))
	for _, cp := range c.p.cols {
		if cp.col.Hidden {
			continue
		}
		rc := ResultColumn{ID: cp.col.ID, Label: labelOf(cp), Aggregate: cp.col.Aggregate, Format: cp.col.Format}
		rc.Type, rc.TotalType = c.resultType(cp)
		if cp.path != nil && cp.path.field.Type == Select {
			rc.Options = cp.path.field.Options
		} else if cp.path == nil && rc.Type == Select {
			rc.Options = c.formulaOptions[cp.col.ID]
		}
		if typ, ok := c.labelTypes[cp.col.ID]; ok {
			rc.LabelCell, rc.LabelType = cp.col.ID+"@label", typ
			if typ == Money {
				rc.LabelCurrencyCell = cp.col.ID + "@labelCurrency"
			}
		}
		rc.Sortable = c.p.pivot == nil && c.sortable[cp.col.ID]
		rc.Heading = rn.headings[cp.col.ID]
		rn.currencyOfColumn(cp, &rc)
		out = append(out, rc)
	}
	if pv := c.p.pivot; pv != nil {
		cell := pv.cell
		for j, v := range rn.pivotValues {
			label := rn.pivotLabel(j, v.key)
			rc := ResultColumn{
				ID: fmt.Sprintf("%s.p%d", cell.col.ID, j), Label: label.text,
				Type: c.levelTypes[cell.col.ID], Aggregate: cell.col.Aggregate, Format: cell.col.Format,
				Pivot: &PivotColumn{Cell: cell.col.ID, Key: v.key, LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency},
			}
			rn.currencyOfColumn(cell, &rc)
			out = append(out, rc)
		}
	}
	return out
}

// currencyOfColumn names a money column's currency: the cell that holds each
// row's currency (a currency key's, or the one its value carries), or the
// fixed code. A child row column reads its own currency field, which
// nestLevel selects.
func (rn *runner) currencyOfColumn(cp *colPlan, rc *ResultColumn) {
	cur := rn.c.currencyOf[cp.col.ID]
	if cp.nest > 0 {
		cur = rn.c.p.nestCurrency(cp)
	}
	switch {
	case cur.key != "" || rn.c.carried[cp.col.ID]:
		rc.CurrencyCell = cp.col.ID + "@currency"
	case cur.fixed != nil:
		rc.Currency = rn.c.fixedCurrency(cur.fixed)
	}
}

func labelOf(cp *colPlan) string {
	switch {
	case cp.col.Label != "":
		return cp.col.Label
	case cp.path != nil && cp.path.endsAtLink:
		return cp.path.hops[len(cp.path.hops)-1].link.Label
	case cp.path != nil:
		return cp.path.field.Label
	}
	return cp.col.ID
}

// markCurrencies marks every money column whose total rows hold more than
// one currency, and says so in a notice.
func (rn *runner) markCurrencies(groupRows []map[string]any) {
	mixed := map[string]bool{}
	for _, ck := range rn.c.currencies {
		seen := map[string]bool{}
		for _, row := range groupRows {
			if toInt(row["__level"]) == 0 && row[ck] != nil {
				seen[fmt.Sprint(row[ck])] = true
			}
		}
		mixed[ck] = len(seen) > 1
	}
	var cols []string
	for i := range rn.res.Columns {
		rc := &rn.res.Columns[i]
		id := rc.ID
		if rc.Pivot != nil {
			id = rc.Pivot.Cell
		}
		if ck := rn.c.currencyOf[id].key; ck != "" && mixed[ck] {
			rc.MixedCurrency = true
			cols = append(cols, rc.ID)
		}
	}
	if len(cols) > 0 {
		rn.notice(Notice{Code: "mixedCurrency", Message: "Amounts in different currencies are totalled apart, one row per currency.", Columns: cols})
	}
}

// markMixedLookups names in a notice every column a row of this page shows
// no value in because a sum lookup met amounts in two currencies, and every
// condition that read such a sum (R3). With the totals' notice due too, the
// two are one notice: its message, both columns.
func (rn *runner) markMixedLookups() {
	shown := map[string]bool{}
	for _, r := range rn.res.Rows {
		for _, id := range rn.mixedRows[r.ID] {
			shown[id] = true
		}
	}
	var cols []string
	for _, cp := range rn.c.p.cols {
		if shown[cp.col.ID] {
			cols = append(cols, cp.col.ID)
		}
	}
	conds := rn.mixedConditions
	if len(cols) == 0 && len(conds) == 0 {
		return
	}
	words := make([]string, len(conds))
	for i, at := range conds {
		words[i] = rn.c.conditionSentence(at)
	}
	for i := range rn.res.Notices {
		if n := &rn.res.Notices[i]; n.Code == "mixedCurrency" {
			for _, id := range cols {
				if !slices.Contains(n.Columns, id) {
					n.Columns = append(n.Columns, id)
				}
			}
			n.Conditions = conds
			n.Message, n.Parts = noticeText(append([]string{n.Message}, words...))
			return
		}
	}
	sentences := words
	if len(cols) > 0 {
		sentences = append([]string{"A looked-up total mixes currencies, so it shows no value."}, words...)
	}
	message, parts := noticeText(sentences)
	rn.notice(Notice{Code: "mixedCurrency", Message: message, Parts: parts, Columns: cols, Conditions: conds})
}

// noticeText is a notice's message made of sentences, and its parts: the
// sentences when there is more than one (Notice.Parts).
func noticeText(sentences []string) (string, []string) {
	if len(sentences) < 2 {
		return strings.Join(sentences, " "), nil
	}
	return strings.Join(sentences, " "), sentences
}

// conditionSentence tells, for a notice, what a condition or sort key (by
// its spec path) did with a looked-up total that mixes currencies.
func (c *compiler) conditionSentence(at string) string {
	for i, s := range c.p.sort {
		if s.at != at {
			continue
		}
		subject := fmt.Sprintf("The formula of sort key %d", i+1)
		if s.col != nil {
			subject = fmt.Sprintf("The sort by %q", columnName(s.col))
		}
		return subject + " reads a looked-up total that mixes currencies, so it sorts as blank there."
	}
	if owner := c.lookupWhereOwner(at); owner != "" {
		return "The where of a lookup in " + owner + " reads a looked-up total that mixes currencies, so that total counts as unknown there."
	}
	return c.conditionWords(at) + " reads a looked-up total that mixes currencies, so that total counts as unknown there."
}

// lookupWhereOwner names the part of the spec a lookup's where (by its spec
// path, which ends in .where) is in, "" for any other path.
func (c *compiler) lookupWhereOwner(at string) string {
	parts := strings.Split(at, ".")
	if len(parts) < 2 || parts[len(parts)-1] != "where" {
		return ""
	}
	i := -1
	if n, err := strconv.Atoi(parts[1]); err == nil {
		i = n
	}
	switch {
	case parts[0] == "filter":
		return "the filter"
	case parts[0] == "detail":
		return "the detail label"
	case parts[0] == "columns" && i >= 0 && i < len(c.p.spec.Columns):
		if cp := c.p.byID[c.p.spec.Columns[i].ID]; cp != nil {
			return fmt.Sprintf("the %q column", columnName(cp))
		}
	case parts[0] == "sort" && i >= 0:
		return fmt.Sprintf("sort key %d", i+1)
	case parts[0] == "layout" && i >= 0 && i < len(c.p.spec.Layout):
		return fmt.Sprintf("the computed row %q", c.p.spec.Layout[i].Row)
	case parts[0] == "groups" && i >= 0 && i < len(c.p.groups) && len(parts) > 2:
		// groups.<i>.fill.where is the fill's own where (conditionWords).
		if parts[2] == "fill" && len(parts) == 4 {
			return ""
		}
		whats := map[string]string{"having": "the having", "label": "the label", "fill": "the fill", "opening": "the opening row"}
		if what, ok := whats[parts[2]]; ok {
			return fmt.Sprintf("%s of the %q group", what, columnName(c.p.groups[i].col))
		}
	}
	return ""
}

func columnName(cp *colPlan) string {
	if cp.col.Label != "" {
		return cp.col.Label
	}
	return cp.col.ID
}

// conditionWords names a condition by its spec path for a notice: the
// filter, or a group's having or fill by the group's column.
func (c *compiler) conditionWords(at string) string {
	for i, g := range c.p.groups {
		name := columnName(g.col)
		switch at {
		case fmt.Sprintf("groups.%d.having", i):
			return fmt.Sprintf("The having of the %q group", name)
		case fmt.Sprintf("groups.%d.fill.where", i):
			return fmt.Sprintf("The fill of the %q group", name)
		}
	}
	return "The filter"
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}

// tree indexes the group statement's rows for assembly.
type tree struct {
	children map[string][]map[string]any // by parent key, in order
	totals   []map[string]any
	computed map[string]map[string]any // by row id and currency key
	details  map[string][]map[string]any
	nested   []map[any][]map[string]any
}

func keyString(key []any) string {
	b, _ := json.Marshal(key)
	return string(b)
}

// rowKey is a level-n row's group key: its n group keys, then its
// currency keys.
func (rn *runner) rowKey(row map[string]any, n int) []any {
	key := make([]any, 0, n+len(rn.c.currencies))
	for i := range n {
		key = append(key, row[fmt.Sprintf("k%d", i)])
	}
	for _, ck := range rn.c.currencies {
		key = append(key, row[ck])
	}
	return key
}

func (rn *runner) currencyKey(row map[string]any) string { return keyString(rn.rowKey(row, 0)) }

func (rn *runner) assemble(groupRows, details []map[string]any, nested []map[any][]map[string]any) {
	t := &tree{children: map[string][]map[string]any{}, computed: map[string]map[string]any{}, details: map[string][]map[string]any{}, nested: nested}
	for _, row := range groupRows {
		n := int(toInt(row["__level"]))
		switch {
		case row["__row"] != nil:
			t.computed[fmt.Sprint(row["__row"])+"|"+rn.currencyKey(row)] = row
		case n == 0:
			t.totals = append(t.totals, row)
		default:
			parent := keyString(rn.rowKey(row, n-1))
			t.children[parent] = append(t.children[parent], row)
		}
	}
	groups := len(rn.c.p.groups)
	for _, row := range details {
		key := keyString(rn.rowKey(row, groups))
		t.details[key] = append(t.details[key], row)
	}
	// One total per currency, in currency order.
	sort.SliceStable(t.totals, func(i, j int) bool {
		return rn.currencyKey(t.totals[i]) < rn.currencyKey(t.totals[j])
	})
	if len(t.totals) == 0 {
		// A total over no rows grouped by currency has no row; the report
		// still ends in a total.
		t.totals = []map[string]any{{"__level": int64(0)}}
	}
	if groups == 0 {
		for _, row := range details {
			rn.detail(t, row, "", 0)
		}
	}
	for _, total := range t.totals {
		if len(rn.c.p.layout) > 0 {
			rn.layoutRows(t, total)
			continue
		}
		for _, g := range t.children[rn.currencyKey(total)] {
			rn.group(t, g, 1, 0, "")
		}
		rn.push(Row{ID: "total" + rn.currencySuffix(total), Kind: KindTotal, Label: "Total", Cells: rn.cells(total)}, total)
	}
}

func (rn *runner) currencySuffix(row map[string]any) string {
	if len(rn.c.currencies) == 0 {
		return ""
	}
	return ":" + rn.currencyKey(row)
}

// push emits a row made from raw, the statement's row (nil for a row that
// shows no looked-up total), and keeps the columns whose cell on it is blank
// because a sum lookup met two currencies: an opening row's opmx<idx>, any
// other row's vmx<idx> (markMixedLookups).
func (rn *runner) push(row Row, raw map[string]any) {
	rn.res.Rows = append(rn.res.Rows, row)
	var cols []string
	for idx, cp := range rn.c.p.cols {
		helper := fmt.Sprintf("vmx%d", idx)
		if row.Kind == KindOpening {
			helper = fmt.Sprintf("opmx%d", idx)
		}
		if raw[helper] != nil {
			cols = append(cols, cp.col.ID)
		}
	}
	if len(cols) == 0 {
		return
	}
	if rn.mixedRows == nil {
		rn.mixedRows = map[string][]string{}
	}
	rn.mixedRows[row.ID] = cols
}

// isEnd is a whole-report row; a computed row inside a section belongs to
// its section.
func isEnd(r Row) bool { return r.Kind == KindTotal || (r.Kind == KindComputed && r.Section == "") }

// pageWindow keeps a grouped page's slice of the document (R9e): the rows
// after the previous page's last detail row (after its group row and opening
// row; from the start on page 1), up to and including this page's last detail
// row and its child rows (to the end on the last page). A total or computed
// row inside the slice keeps its place there (a Total per currency block, a
// layout's computed rows between sections); those after the slice follow it,
// as each page of a report without groups ends in its totals. Those before
// the slice were on an earlier page.
func (rn *runner) pageWindow() {
	if rn.page == nil {
		return
	}
	rows := rn.res.Rows
	// A row the page names is always among its rows; were one missing, the
	// page keeps the document from its start or to its end, which the
	// client's merge of pages dedupes.
	start := 0
	if rn.page.after != nil {
		id := "g:" + keyString(rn.page.after)
		if i := slices.IndexFunc(rows, func(r Row) bool { return r.ID == id }); i >= 0 {
			start = i + 1
			if start < len(rows) && rows[start].Kind == KindOpening && rows[start].Section == id {
				start++
			}
		} else {
			log.WarnContext(rn.ctx, "page start group not among the rows", "group", id)
		}
	}
	end := len(rows)
	if rn.page.last != "" {
		if i := slices.IndexFunc(rows, func(r Row) bool { return r.ID == rn.page.last }); i >= 0 {
			end = i + 1
			for end < len(rows) && rows[end].Parent != "" {
				end++
			}
		} else {
			log.WarnContext(rn.ctx, "page end row not among the rows", "row", rn.page.last)
		}
	}
	out := slices.Clone(rows[start:end])
	for _, r := range rows[end:] {
		if isEnd(r) {
			out = append(out, r)
		}
	}
	rn.res.Rows = out
}

// cutPage keeps a paged grouped run's page (J1a, R3): the rows from where the
// cursor left off, up to limit counted rows (group, opening, detail and
// subtotal; a Total or computed row is not counted), then the Totals after
// the slice, as every page ends in them. A group row and its opening row
// stay on one page. The last counted row gives page.next when a counted row
// follows it; a cursor that cannot be written fails the run (cursorAt).
func (rn *runner) cutPage(limit int) error {
	rows := rn.res.Rows
	start := rn.pageStart(rows)
	end, counted := start, 0
	for end < len(rows) {
		r := rows[end]
		if isEnd(r) {
			end++
			continue
		}
		take := 1
		if r.Kind == KindGroup && end+1 < len(rows) && rows[end+1].Kind == KindOpening && rows[end+1].Section == r.ID {
			take = 2
		}
		if counted+take > limit && counted > 0 {
			break
		}
		counted += take
		end += take
		if counted >= limit {
			break
		}
	}
	more := slices.ContainsFunc(rows[end:], func(r Row) bool { return !isEnd(r) })
	out := slices.Clone(rows[start:end])
	for _, r := range rows[end:] {
		if isEnd(r) {
			out = append(out, r)
		}
	}
	rn.res.Rows = out
	details := 0
	for _, r := range out {
		if r.Kind == KindDetail {
			details++
		}
	}
	// A run that is one page and read no detail rows keeps a nil page, as it
	// had before J1, so a run that fits one page is byte-identical (R4, the
	// ledger golden: its "collapsed" runs fit one page).
	if !more && rn.opts.Cursor == "" && !rn.readDetails {
		return nil
	}
	rn.res.Page = &Page{Size: details}
	if !more {
		return nil
	}
	last := start + lastCounted(rows[start:end])
	next, err := rn.cursorAt(rows[last], rn.heldAfter(rows, last))
	if err != nil {
		return err
	}
	rn.res.Page.Next = next
	return nil
}

// heldAfter lists the groups the next cursor holds: each group whose first
// details earlier pages showed (rn.shown) that this page did not pass,
// because it sorts after the page's last counted row (it moved later between
// pages) or is not among the rows. A group the page passed is behind the next
// cursor, or the next cursor reads on in it from its own detail values.
func (rn *runner) heldAfter(rows []Row, last int) []held {
	var out []held
	for _, h := range rn.shown {
		id := "g:" + keyString(h.Group)
		at := slices.IndexFunc(rows, func(r Row) bool { return r.ID == id })
		if at >= 0 && at+1 < len(rows) && rows[at+1].Kind == KindOpening && rows[at+1].Section == id {
			at++
		}
		if at >= 0 && at < last {
			continue
		}
		out = append(out, h)
	}
	if len(out) > maxHeld {
		out = out[len(out)-maxHeld:]
	}
	return out
}

// pageStart is where a paged run's page starts among its rows: after the
// Totals of the currency blocks before the cursor's (they were on an earlier
// page; its own block's Total may be next), then after the deepest anchor's
// group row and its opening row, which the page read again only for their
// subtotals.
func (rn *runner) pageStart(rows []Row) int {
	cur := rn.groupCur
	if cur == nil {
		return 0
	}
	d := len(cur.Path)
	own := "total"
	if len(rn.c.currencies) > 0 {
		own += ":" + keyString(cur.Group[d:])
	}
	start := 0
	for start < len(rows) && rows[start].Kind == KindTotal && rows[start].ID != own {
		start++
	}
	// A group removed or moved between pages is not read as an anchor
	// (groupKeyset): the page starts after the deepest anchor that was. A
	// moved group read at its new place by the keyset is not one: starting
	// after it would skip the rows before it.
	for n := rn.anchored; n >= 1; n-- {
		// A skip level's blank key draws no row: its children follow the
		// anchor above it.
		if rn.c.skips(n) && cur.Group[n-1] == nil {
			continue
		}
		id := "g:" + keyString(append(slices.Clone(cur.Group[:n]), cur.Group[d:]...))
		i := slices.IndexFunc(rows[start:], func(r Row) bool { return r.ID == id })
		if i < 0 {
			log.WarnContext(rn.ctx, "page start group not among the rows", "group", id)
			continue
		}
		at := start + i + 1
		if at < len(rows) && rows[at].Kind == KindOpening && rows[at].Section == id {
			at++
		}
		return at
	}
	return start
}

// lastCounted is the index of the last row a page counts (not a Total or
// computed row); a page that has a next one holds at least one.
func lastCounted(rows []Row) int {
	for i := len(rows) - 1; i > 0; i-- {
		if !isEnd(rows[i]) {
			return i
		}
	}
	return 0
}

// cursorAt is the cursor after row, the last row a page counts (R6): inside
// the group for a detail row, an expanded group row or an opening row; after
// the group's whole subtree for a closed group row or a subtotal. Its path
// holds the order values of the group and of each enclosing group, read from
// their statement rows. A missing one is an engine fault: an empty next would
// end the list early and look complete, so it is an error instead.
func (rn *runner) cursorAt(row Row, held []held) (string, error) {
	key, mode := row.GroupKey, cursorAfter
	var detail map[string]any
	switch {
	case row.Kind == KindDetail:
		detail = rn.rawDetails[row.ID]
		key, mode = rn.rowKey(rn.rawGroups[row.Section], len(rn.c.p.groups)), cursorIn
	case row.Kind == KindOpening, row.Kind == KindGroup && row.Expanded:
		mode = cursorIn
	}
	depth := len(key) - len(rn.c.currencies)
	path := make([][]any, depth)
	for i := 1; i <= depth; i++ {
		id := "g:" + keyString(append(slices.Clone(key[:i]), key[depth:]...))
		raw, ok := rn.rawGroups[id]
		if !ok {
			// Every enclosing group's row is an anchor or was read.
			return "", fmt.Errorf("reports: the page's end group %s has no statement row", id)
		}
		values := make([]any, len(rn.groupShape[i-1]))
		for j := range values {
			values[j] = raw[orderValue(i, j)]
		}
		path[i-1] = values
	}
	return groupCursorOf(rn.groupFP, rn.groupKeys, groupPos{key: key, path: path, mode: mode, held: held}, detail), nil
}

// group emits a level-n group row, its children when expanded, and its
// subtotal, at depth.
func (rn *runner) group(t *tree, row map[string]any, n, depth int, section string) {
	key := rn.rowKey(row, n)
	id := "g:" + keyString(key)
	g := rn.c.p.groups[n-1]
	label := rowLabel{text: rn.display(g.col, row[fmt.Sprintf("k%d", n-1)], g.group.Bucket != "")}
	// A key that mixes a select's values with other text reads as its group
	// column's cell shows it.
	if rn.c.groupShown[n-1] {
		label = rowLabel{text: labelText(row[fmt.Sprintf("v%d", rn.c.p.index(g.col))])}
	}
	// A record group and a labelled group are named by their label; a blank
	// one reads (none), never the record's id.
	if g.group.Label != nil || rn.c.groupTargets[n-1] != nil {
		label = rn.groupLabel(row, rn.c.labels[n-1])
	}
	rn.groupWithLabel(t, row, n, depth, section, id, label, key, g.group.Subtotals, nil)
}

// children emits the level-n groups under a parent key. A blank key at a skip
// level emits no row of its own: its children come in its place, at the same
// depth, under the same section.
func (rn *runner) children(t *tree, parent []any, n, depth int, section string) {
	for _, child := range t.children[keyString(parent)] {
		if rn.c.skips(n) && child[fmt.Sprintf("k%d", n-1)] == nil {
			key := rn.rowKey(child, n)
			// A paged run's cursor reads the order values of every enclosing
			// group, this one too (cursorAt).
			if rn.rawGroups != nil {
				rn.rawGroups["g:"+keyString(key)] = child
			}
			rn.children(t, key, n+1, depth, section)
			continue
		}
		rn.group(t, child, n, depth, section)
	}
}

// rowLabel is a group row's label: its text and, for a label that is not
// text, its type, value and currency (Row.LabelType).
type rowLabel struct {
	text     string
	typ      FieldType
	value    any
	currency string
}

// groupLabel is the label a row's __label holds: a group's, or a detail row's
// label formula. A label formula or a record label that is not text keeps its
// type, so the client formats it as a cell (R9).
func (rn *runner) groupLabel(row map[string]any, lp labelPlan) rowLabel {
	cur := ""
	if lp.typ == Money {
		cur = rn.currencyOf(row, lp.cur)
		if lp.recordCurrency {
			cur, _ = row["__labelcur"].(string)
		}
	}
	return typedLabel(row["__label"], lp.typ, cur)
}

// typedLabel is a label value of type typ as a row label: (none) for a blank,
// a no-name record's text as text, Yes or No, any other value that is not
// text typed with its plain text.
func typedLabel(v any, typ FieldType, currency string) rowLabel {
	switch {
	case v == nil:
		return rowLabel{text: blankLabel}
	case v == noName:
		return rowLabel{text: noName}
	case typ == Bool:
		yes := toInt(v) != 0
		text := "No"
		if yes {
			text = "Yes"
		}
		return rowLabel{text: text, typ: Bool, value: yes}
	case typ == Number || typ == Money || typ == Percent || typ == Date || typ == DateTime:
		out := rowLabel{text: labelText(v), typ: typ, value: v}
		if typ == Money {
			out.currency = currency
		}
		return out
	}
	return rowLabel{text: labelText(v)}
}

// currencyOf is a money value's currency on a row: its currency key's value
// or its source's fixed code.
func (rn *runner) currencyOf(row map[string]any, cur currency) string {
	switch {
	case cur.key != "":
		s, _ := row[cur.key].(string)
		return s
	case cur.fixed != nil:
		return rn.c.fixedCurrency(cur.fixed)
	}
	return ""
}

// groupWithLabel emits a group row; subtotal says whether an expanded group
// ends in a subtotal row (a layout section always does). layoutSection is the
// layout section the row draws, nil for any other group: its within rows
// come after its children, before its subtotal.
func (rn *runner) groupWithLabel(t *tree, row map[string]any, n, depth int, section, id string, label rowLabel, key []any, subtotal bool, layoutSection *LayoutItem) {
	p := rn.c.p
	opening := p.groups[n-1].group.Opening
	canOpen := n < len(p.groups) || p.pivot == nil
	// A level with an opening row opens even with nothing under it, so the
	// opening row and the subtotal show (R5d). Without one: a fill record has
	// no base rows and opens only onto the next level's fill records (a layout
	// section with no rows has no __n and keeps this rule), and a group whose
	// child groups the having hides, every one, opens onto nothing. A layout
	// section with within rows always opens onto them: they add to its total,
	// so they show even when no row of its own does.
	if opening == nil && !rn.hasWithin(layoutSection) {
		if v, ok := row["__n"]; ok && v != nil && toInt(v) == 0 && !rn.c.nextFill(n) {
			canOpen = false
		}
		if v, ok := row["__kids"]; ok && v != nil && toInt(v) == 0 {
			canOpen = false
		}
	}
	expanded := canOpen && rn.expand.has(rn.c, key, n)
	if rn.rawGroups != nil {
		rn.rawGroups[id] = row
	}
	rn.push(Row{
		ID: id, Kind: KindGroup, Depth: depth, Level: n, Label: label.text, Section: section, GroupKey: key, Expanded: expanded, CanExpand: canOpen, Cells: rn.cells(row),
		LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
	}, row)
	if expanded && opening != nil {
		rn.push(Row{ID: "opening:" + keyString(key), Kind: KindOpening, Depth: depth + 1, Label: opening.Label, Section: id, GroupKey: key, Cells: rn.openingCells(row, opening)}, row)
	}
	if expanded {
		if n < len(p.groups) {
			rn.children(t, key, n+1, depth+1, id)
		} else {
			for _, d := range t.details[keyString(key)] {
				rn.detail(t, d, id, depth+1)
			}
		}
	}
	if expanded && layoutSection != nil {
		for _, w := range p.layout {
			if w.Row == "" || w.Within != layoutSection.Section {
				continue
			}
			raw := t.computed[w.Row+"|"+rn.currencyKey(row)]
			if raw == nil {
				raw = map[string]any{}
			}
			rn.push(Row{ID: "c:" + w.Row + rn.currencySuffix(row), Kind: KindComputed, Depth: depth + 1, Label: w.Label, Section: id, Cells: rn.cells(raw)}, raw)
		}
	}
	if subtotal && (expanded || len(p.layout) > 0) {
		rn.push(Row{
			ID: "s:" + keyString(key), Kind: KindSubtotal, Depth: depth, Level: n, Label: "Total " + label.text, Section: id, GroupKey: key, Cells: rn.cells(row),
			LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
		}, row)
	}
}

// hasWithin is true when the layout has a computed row inside section s.
func (rn *runner) hasWithin(s *LayoutItem) bool {
	return s != nil && slices.ContainsFunc(rn.c.p.layout, func(w *LayoutItem) bool {
		return w.Row != "" && w.Within == s.Section
	})
}

// detail emits a detail row and the child rows nested under it.
func (rn *runner) detail(t *tree, row map[string]any, section string, depth int) {
	id := fmt.Sprintf("d:%v", row["__id"])
	if rn.rawDetails != nil {
		rn.rawDetails[id] = row
	}
	label, labelColumn := rn.firstLabel(rn.c.p.cols, row), rn.c.p.cols[0].col.ID
	if lp := rn.c.detailLabel; lp != nil {
		label, labelColumn = rn.groupLabel(row, *lp), repeatedColumn(rn.c.p.spec.Detail.Label)
	}
	rn.push(Row{
		ID: id, Kind: KindDetail, Depth: depth, Label: label.text, LabelColumn: labelColumn, Section: section, Cells: rn.detailCells(rn.cells(row)),
		LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
	}, row)
	if len(t.nested) > 0 {
		rn.nestedRows(t, 0, row["__n0"], id, section, depth+1)
	}
}

// detailCells is a detail row's cells without those of the columns blank on
// detail rows (Column.Detail): their value, currency and record label.
func (rn *runner) detailCells(cells map[string]any) map[string]any {
	for _, cp := range rn.c.p.cols {
		if cp.col.Detail != DetailBlank {
			continue
		}
		id := cp.col.ID
		delete(cells, id)
		delete(cells, id+"@currency")
		delete(cells, id+"@label")
		delete(cells, id+"@labelCurrency")
	}
	return cells
}

// repeatedColumn is the column a detail label of exactly {col} repeats: the
// row's label is that cell, shown as the column shows it. Any other label is
// no cell's value, so it repeats none.
func repeatedColumn(n *Node) string {
	if n == nil || !reflect.DeepEqual(*n, Node{Col: n.Col}) {
		return ""
	}
	return n.Col
}

// cells maps a statement row's v<idx> and pv<j> values to column ids, with each
// money column's currency. Only rows of the group statement carry __level;
// their cells have the type of the column's aggregate, not of a detail row.
func (rn *runner) cells(row map[string]any) map[string]any {
	_, grouped := row["__level"]
	out := map[string]any{}
	for idx, cp := range rn.c.p.cols {
		if cp.col.Hidden {
			continue
		}
		name := fmt.Sprintf("v%d", idx)
		if _, ok := row[name]; !ok {
			continue
		}
		out[cp.col.ID] = rn.cellValue(cp, row[name], grouped)
		// A currency the value carries (a record's or a looked-up row's own)
		// is its row's; else the column's currency key holds it. A column with
		// a fixed currency that some row carries its own on names a currency
		// cell, so every other row shows the fixed code there. A cell that is
		// not an amount on this row (a count) or is blank for currencies
		// (vmx<idx>) names none.
		if rn.typeOf(cp, grouped) == Money && row[fmt.Sprintf("vmx%d", idx)] == nil {
			rn.currencyCell(out, cp, row, row[fmt.Sprintf("vcur%d", idx)])
		}
		rn.labelCell(out, cp, row)
	}
	if pv := rn.c.p.pivot; pv != nil {
		for j := range rn.pivotValues {
			if v, ok := row[fmt.Sprintf("pv%d", j)]; ok {
				out[fmt.Sprintf("%s.p%d", pv.cell.col.ID, j)] = v
			}
		}
	}
	return out
}

// currencyCell puts a money cell's currency in <id>@currency: the one its
// value carries, else the row's currency key of the column, else the column's
// fixed code when some row carries its own.
func (rn *runner) currencyCell(out map[string]any, cp *colPlan, row map[string]any, carried any) {
	cur := rn.c.currencyOf[cp.col.ID]
	switch {
	case carried != nil:
		out[cp.col.ID+"@currency"] = carried
	case cur.key != "":
		out[cp.col.ID+"@currency"] = row[cur.key]
	case cur.fixed != nil && rn.c.carried[cp.col.ID]:
		out[cp.col.ID+"@currency"] = rn.currencyOf(row, cur)
	}
}

// openingCells are an opening row's cells: each column its level's opening
// names, from the group row's op<idx>, typed as the column's detail cells,
// with a money cell's currency as a group row's (currencyCell), none on a
// cell blank for currencies (opmx<idx>). A column it names no cell for has
// none, so it is blank.
func (rn *runner) openingCells(row map[string]any, opening *Opening) map[string]any {
	out := map[string]any{}
	for idx, cp := range rn.c.p.cols {
		if opening.Cells[cp.col.ID] == nil {
			continue
		}
		out[cp.col.ID] = rn.cellValue(cp, row[fmt.Sprintf("op%d", idx)], false)
		if row[fmt.Sprintf("opmx%d", idx)] == nil {
			rn.currencyCell(out, cp, row, row[fmt.Sprintf("opcur%d", idx)])
		}
	}
	return out
}

// labelCell puts a labelled column's record label (vl<idx>) in the cell
// <id>@label, typed as its label field is, and an amount label's currency
// (vc<idx>) in <id>@labelCurrency.
func (rn *runner) labelCell(out map[string]any, cp *colPlan, row map[string]any) {
	typ, ok := rn.c.labelTypes[cp.col.ID]
	if !ok {
		return
	}
	idx := rn.c.p.index(cp)
	v := row[fmt.Sprintf("vl%d", idx)]
	if typ == Bool && v != nil && v != noName {
		v = toInt(v) != 0
	}
	out[cp.col.ID+"@label"] = v
	if typ == Money {
		out[cp.col.ID+"@labelCurrency"] = row[fmt.Sprintf("vc%d", idx)]
	}
}

func (rn *runner) cellValue(cp *colPlan, v any, grouped bool) any {
	if v != nil && rn.typeOf(cp, grouped) == Bool {
		return toInt(v) != 0
	}
	return v
}

// typeOf is the type of a column's cell on a group, subtotal and total row
// (grouped) or on a detail row.
func (rn *runner) typeOf(cp *colPlan, grouped bool) FieldType {
	if grouped {
		return rn.c.levelTypes[cp.col.ID]
	}
	return rn.c.detailTypes[cp.col.ID]
}

// firstLabel is a detail row's label: its first column's value, or the
// record label of a first column that points at records, typed (a blank link
// reads (none)).
func (rn *runner) firstLabel(cols []*colPlan, row map[string]any) rowLabel {
	if len(cols) == 0 {
		return rowLabel{}
	}
	idx := rn.c.p.index(cols[0])
	if typ, ok := rn.c.labelTypes[cols[0].col.ID]; ok {
		cur, _ := row[fmt.Sprintf("vc%d", idx)].(string)
		return typedLabel(row[fmt.Sprintf("vl%d", idx)], typ, cur)
	}
	return rowLabel{text: rn.display(cols[0], row[fmt.Sprintf("v%d", idx)], false)}
}

// columnOptions are the options of a column whose values are a select's: its
// field's, or those of the select a formula column reads.
func (c *compiler) columnOptions(cp *colPlan) []Option {
	if cp.path != nil {
		return optionsOf(cp.path.field)
	}
	return c.formulaOptions[cp.col.ID]
}

// display is a value as a row label: a select's label, Yes or No, the
// number or text itself, or (none) for a blank.
func (rn *runner) display(cp *colPlan, v any, bucketed bool) string {
	if v == nil || v == "" {
		return blankLabel
	}
	if !bucketed {
		for _, o := range rn.c.columnOptions(cp) {
			if o.Value == v {
				return o.Label
			}
		}
	}
	if !bucketed && cp.path != nil {
		switch cp.path.field.Type {
		case Bool:
			if toInt(v) != 0 {
				return "Yes"
			}
			return "No"
		}
	}
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
