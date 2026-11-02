package reports

import (
	"encoding/json"
	"fmt"
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
		}
		if t := c.levelTypes[cp.col.ID]; t != "" && t != rc.Type && cp.col.Aggregate != "" {
			rc.TotalType = t
		}
		rn.currencyOfColumn(cp, &rc)
		out = append(out, rc)
	}
	if pv := c.p.pivot; pv != nil {
		cell := pv.cell
		for j, key := range rn.pivotValues {
			rc := ResultColumn{
				ID: fmt.Sprintf("%s.p%d", cell.col.ID, j), Label: rn.pivotLabel(key),
				Type: c.levelTypes[cell.col.ID], Aggregate: cell.col.Aggregate, Format: cell.col.Format,
				Pivot: &PivotColumn{Cell: cell.col.ID, Key: key},
			}
			rn.currencyOfColumn(cell, &rc)
			out = append(out, rc)
		}
	}
	return out
}

// currencyOfColumn names a money column's currency: the cell that holds each
// row's currency, or the fixed code. A child row column reads its own
// currency field, which nestLevel selects.
func (rn *runner) currencyOfColumn(cp *colPlan, rc *ResultColumn) {
	cur := rn.c.currencyOf[cp.col.ID]
	if cp.nest > 0 {
		cur = rn.c.p.nestCurrency(cp)
	}
	switch {
	case cur.key != "":
		rc.CurrencyCell = cp.col.ID + "@currency"
	case cur.fixed != nil:
		rc.Currency = cur.fixed.Currency(rn.app)
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
	p := rn.c.p
	key := rn.rowKey(row, n)
	id := "g:" + keyString(key)
	label := rn.display(p.groups[n-1].col, row[fmt.Sprintf("k%d", n-1)], p.groups[n-1].group.Bucket != "")
	rn.groupWithLabel(t, row, n, section, id, label, key, p.groups[n-1].group.Subtotals)
}

// groupWithLabel emits a group row; subtotal says whether an expanded group
// ends in a subtotal row (a layout section always does).
func (rn *runner) groupWithLabel(t *tree, row map[string]any, n int, section, id, label string, key []any, subtotal bool) {
	p := rn.c.p
	canOpen := n < len(p.groups) || p.pivot == nil
	expanded := canOpen && rn.expand.has(key)
	rn.push(Row{ID: id, Kind: KindGroup, Depth: n - 1, Label: label, Section: section, GroupKey: key, Expanded: expanded, CanExpand: canOpen, Cells: rn.cells(row)})
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
		rn.push(Row{ID: "s:" + keyString(key), Kind: KindSubtotal, Depth: n - 1, Label: "Total " + label, Section: id, GroupKey: key, Cells: rn.cells(row)})
	}
}

// detail emits a detail row and the child rows nested under it.
func (rn *runner) detail(t *tree, row map[string]any, section string, depth int) {
	id := fmt.Sprintf("d:%v", row["__id"])
	rn.push(Row{ID: id, Kind: KindDetail, Depth: depth, Label: rn.firstLabel(rn.c.p.cols, row), Section: section, Cells: rn.cells(row)})
	if len(t.nested) > 0 {
		rn.nestedRows(t, 0, row["__n0"], id, section, depth+1)
	}
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
		if ck := rn.c.currencyOf[cp.col.ID].key; ck != "" {
			out[cp.col.ID+"@currency"] = row[ck]
		}
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

func (rn *runner) firstLabel(cols []*colPlan, row map[string]any) string {
	if len(cols) == 0 {
		return ""
	}
	idx := rn.c.p.index(cols[0])
	return rn.display(cols[0], row[fmt.Sprintf("v%d", idx)], false)
}

// display is a value as a row label: a select's label, Yes or No, the
// number or text itself, or (none) for a blank.
func (rn *runner) display(cp *colPlan, v any, bucketed bool) string {
	if v == nil || v == "" {
		return blankLabel
	}
	if !bucketed && cp.path != nil {
		switch cp.path.field.Type {
		case Select:
			for _, o := range cp.path.field.Options {
				if o.Value == v {
					return o.Label
				}
			}
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
