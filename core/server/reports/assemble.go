package reports

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// columns describes the result's columns, pivot cell columns last.
func (rn *runner) columns() []ResultColumn {
	c := rn.c
	out := make([]ResultColumn, 0, len(c.p.cols))
	for _, cp := range c.p.cols {
		rc := ResultColumn{ID: cp.col.ID, Label: labelOf(cp), Aggregate: cp.col.Aggregate, Format: cp.col.Format}
		rc.Type = c.detailTypes[cp.col.ID]
		if rc.Type == "" {
			rc.Type = c.levelTypes[cp.col.ID]
		}
		if cp.path != nil {
			rc.Type = cp.path.field.Type
			if cp.tm >= 0 {
				rc.Type = aggregateType(cp.col.Aggregate, cp.path.field.Type, cp.scale)
			}
			if cp.path.field.Type == Select {
				rc.Options = cp.path.field.Options
			}
		} else if rc.Type == Select {
			rc.Options = c.formulaOptions[cp.col.ID]
		}
		if t := c.levelTypes[cp.col.ID]; t != "" && t != rc.Type && cp.col.Aggregate != "" {
			rc.TotalType = t
		}
		if typ, ok := c.labelTypes[cp.col.ID]; ok {
			rc.LabelCell, rc.LabelType = cp.col.ID+"@label", typ
			if typ == Money {
				rc.LabelCurrencyCell = cp.col.ID + "@labelCurrency"
			}
		}
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
			rn.group(t, g, 1, "")
		}
		rn.push(Row{ID: "total" + rn.currencySuffix(total), Kind: KindTotal, Label: "Total", Cells: rn.cells(total)})
	}
}

func (rn *runner) currencySuffix(row map[string]any) string {
	if len(rn.c.currencies) == 0 {
		return ""
	}
	return ":" + rn.currencyKey(row)
}

func (rn *runner) push(row Row) { rn.res.Rows = append(rn.res.Rows, row) }

// group emits a level-n group row, its children when expanded, and its
// subtotal.
func (rn *runner) group(t *tree, row map[string]any, n int, section string) {
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
	rn.groupWithLabel(t, row, n, section, id, label, key, g.group.Subtotals)
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
// ends in a subtotal row (a layout section always does).
func (rn *runner) groupWithLabel(t *tree, row map[string]any, n int, section, id string, label rowLabel, key []any, subtotal bool) {
	p := rn.c.p
	canOpen := n < len(p.groups) || p.pivot == nil
	// A fill record has no base rows: it opens only onto the next level's fill
	// records. A layout section with no rows has no __n and keeps this rule.
	if v, ok := row["__n"]; ok && v != nil && toInt(v) == 0 && !rn.c.nextFill(n) {
		canOpen = false
	}
	// A group whose child groups the having hides, every one, opens onto nothing.
	if v, ok := row["__kids"]; ok && v != nil && toInt(v) == 0 {
		canOpen = false
	}
	expanded := canOpen && rn.expand.has(key, n)
	rn.push(Row{
		ID: id, Kind: KindGroup, Depth: n - 1, Label: label.text, Section: section, GroupKey: key, Expanded: expanded, CanExpand: canOpen, Cells: rn.cells(row),
		LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
	})
	if expanded {
		if n < len(p.groups) {
			for _, child := range t.children[keyString(key)] {
				rn.group(t, child, n+1, id)
			}
		} else {
			for _, d := range t.details[keyString(key)] {
				rn.detail(t, d, id, n)
			}
		}
	}
	if subtotal && (expanded || len(p.layout) > 0) {
		rn.push(Row{
			ID: "s:" + keyString(key), Kind: KindSubtotal, Depth: n - 1, Label: "Total " + label.text, Section: id, GroupKey: key, Cells: rn.cells(row),
			LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
		})
	}
}

// detail emits a detail row and the child rows nested under it.
func (rn *runner) detail(t *tree, row map[string]any, section string, depth int) {
	id := fmt.Sprintf("d:%v", row["__id"])
	label, labelColumn := rn.firstLabel(rn.c.p.cols, row), rn.c.p.cols[0].col.ID
	if lp := rn.c.detailLabel; lp != nil {
		label, labelColumn = rn.groupLabel(row, *lp), repeatedColumn(rn.c.p.spec.Detail.Label)
	}
	rn.push(Row{
		ID: id, Kind: KindDetail, Depth: depth, Label: label.text, LabelColumn: labelColumn, Section: section, Cells: rn.detailCells(rn.cells(row)),
		LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
	})
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
		name := fmt.Sprintf("v%d", idx)
		if _, ok := row[name]; !ok {
			continue
		}
		out[cp.col.ID] = rn.cellValue(cp, row[name], grouped)
		// A currency the value carries (a record's or a looked-up row's own)
		// is its row's; else the column's currency key holds it. A column with
		// a fixed currency that some row carries its own on names a currency
		// cell, so every other row shows the fixed code there.
		cur := rn.c.currencyOf[cp.col.ID]
		switch v := row[fmt.Sprintf("vcur%d", idx)]; {
		case v != nil:
			out[cp.col.ID+"@currency"] = v
		case cur.key != "":
			out[cp.col.ID+"@currency"] = row[cur.key]
		case cur.fixed != nil && rn.c.carried[cp.col.ID]:
			out[cp.col.ID+"@currency"] = rn.currencyOf(row, cur)
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
