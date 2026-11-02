package reports

import "fmt"

// pivotKey puts the pivot value of each base row in w as pk.
func (r *rowsCTE) pivotKey() {
	pv := r.c.p.pivot
	if pv == nil {
		return
	}
	key := r.root.value(pv.path).sql
	if pv.pivot.Bucket != "" {
		key = bucketSQL(r.join(false), pv.pivot.Bucket, key)
	}
	// A blank text and a missing linked row both read as (none): one column,
	// not two.
	if pv.path.field.Type == Text {
		key = "COALESCE(" + key + ", '')"
	}
	r.add("pk", key)
	if pv.byCurrency() {
		r.add("pkc", fieldSQL(r.root.join(pv.path.hops), pv.path.src.field(pv.path.field.Currency)))
	}
}

// byCurrency is true for a pivot over an amount with its own currency field:
// it pivots on the amount and its currency (pkc), so 300 USD and 300 EUR are
// two columns.
func (pv *pivotPlan) byCurrency() bool {
	return pv.path.field.Type == Money && pv.path.field.Currency != ""
}

// pivotValue is one pivot column: its key, and for a pivot by currency the
// key's currency.
type pivotValue struct {
	key, currency any
}

// pivotPass is the pivot's first pass: its distinct values in order, at
// most maxColumns of them. The main pass's row cap never applies before
// it, so a pivot column never goes missing because of how many rows read
// it.
func (rn *runner) pivotPass() ([]pivotValue, error) {
	c := rn.c
	pv := c.p.pivot
	if pv == nil {
		return nil, nil
	}
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	// A pivot over a link names its columns by the records' labels (its
	// label field's typed values) and reads them in label order, then by id,
	// as record groups do.
	// An amount is its own typed label with its currency; one with a currency
	// field reads in currency order, then by amount.
	label, cur, order, distinct := "NULL", "NULL", "(pk IS NULL), pk", "pk"
	typ := labelTypeOf(nil)
	switch t := pv.path.target; {
	case t != nil:
		f := labelFieldOf(t, pv.pivot.LabelField)
		label, cur = labelValueSQL(s, t, f, "pk"), labelCurrencySQL(s, t, f, "pk")
		key := labelOrder(s, "__label")
		order = "(pk IS NULL), (" + key + " IS NULL), " + key + ", pk"
		typ = labelTypeOf(f)
	case pv.byCurrency():
		label, cur, distinct, typ = "pk", "pkc", "pk, pkc", Money
		order = "(__labelcur IS NULL), __labelcur, (pk IS NULL), pk"
	case pv.path.field.Type == Money:
		label, typ = "pk", Money
		if pv.path.src.Currency != nil {
			cur = s.bind(pv.path.src.Currency(rn.app))
		}
	}
	body := fmt.Sprintf("SELECT pk, __label, __labelcur, COUNT(*) OVER () AS __n FROM (SELECT pk, %s AS __label, %s AS __labelcur FROM (SELECT DISTINCT %s FROM w)) ORDER BY %s LIMIT %s",
		label, cur, distinct, order, s.bind(int64(pv.max+1)))
	rows, err := rn.query(s, []string{r.render()}, body)
	if err != nil {
		return nil, err
	}
	values := make([]pivotValue, 0, len(rows))
	rn.pivotLabels = make([]rowLabel, 0, len(rows))
	for i, row := range rows {
		if i < pv.max {
			v := pivotValue{key: row["pk"]}
			if pv.byCurrency() {
				v.currency = row["__labelcur"]
			}
			values = append(values, v)
			cur, _ := row["__labelcur"].(string)
			rn.pivotLabels = append(rn.pivotLabels, typedLabel(row["__label"], typ, cur))
		}
	}
	if len(rows) > pv.max {
		left := int(toInt(rows[0]["__n"])) - pv.max
		rn.noticeOnce(Notice{Code: "pivotCap", Message: fmt.Sprintf("The pivot shows its first %d columns; %d more are left out.", pv.max, left), LeftOut: left})
	}
	rn.pivotValues = values
	return values, nil
}

// pivotCells adds one aggregate per pivot value to the level: the cell
// column's aggregate over the rows with that value.
func (c *compiler) pivotCells(lv *level, values []pivotValue) ([]string, error) {
	pv := c.p.pivot
	if pv == nil {
		return nil, nil
	}
	cell := pv.cell
	arg, typ, err := lv.cellArg(cell)
	if err != nil {
		return nil, err
	}
	if err := checkAggregate(cell.col.Aggregate, typ, cell.scale, "pivot.cell"); err != nil {
		return nil, err
	}
	// A "rows" cell adds the cells of the shown rows directly under it, as the
	// column's own cell does: the next level's pivot cells, or on the
	// innermost group its detail cells with the pivot value.
	agg, scale := cell.col.Aggregate, cell.scale
	if cell.col.Total == TotalRows && lv.n == len(c.p.groups) {
		d, err := lv.detailCell(cell, cell.at+".total")
		if err != nil {
			return nil, err
		}
		arg, agg, scale = d.sql, Sum, 0
	}
	out := make([]string, 0, len(values))
	for j, v := range values {
		if cell.col.Total == TotalRows && lv.n < len(c.p.groups) {
			out = append(out, fmt.Sprintf("%s AS pv%d", lv.childSum(fmt.Sprintf("pv%d", j)), j))
			continue
		}
		cond := "w.pk IS " + lv.r.s.bind(v.key)
		if pv.byCurrency() {
			cond += " AND w.pkc IS " + lv.r.s.bind(v.currency)
		}
		match := fmt.Sprintf("CASE WHEN %s THEN %s END", cond, arg)
		ref := lv.agg(fmt.Sprintf("q%d", j), aggregateSQL(agg, match, scale))
		out = append(out, fmt.Sprintf("%s AS pv%d", ref, j))
	}
	return out, nil
}

// cellArg is what a column with an aggregate aggregates, as a value of w.
func (lv *level) cellArg(cp *colPlan) (string, FieldType, error) {
	if cp.path != nil {
		return fmt.Sprintf("w.c%d", lv.c.p.index(cp)), cp.path.field.Type, nil
	}
	inner := &rowsCtx{r: lv.r, inAggregate: true, visiting: map[string]bool{}}
	e, err := compileFormula(inner, cp.col.Formula, cp.at+".formula")
	return e.sql, e.typ, err
}

// pivotLabel is the j-th pivot value as a column label: a record's typed
// label for a pivot over a link, an amount typed with its currency, else the
// value as a row label shows it.
func (rn *runner) pivotLabel(j int, key any) rowLabel {
	pv := rn.c.p.pivot
	if pv.path.target != nil || pv.path.field.Type == Money {
		return rn.pivotLabels[j]
	}
	return rowLabel{text: rn.display(&colPlan{col: &Column{}, path: pv.path}, key, pv.pivot.Bucket != "")}
}
