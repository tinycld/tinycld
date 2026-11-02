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
}

// pivotPass is the pivot's first pass: its distinct values in order, at
// most maxColumns of them. The main pass's row cap never applies before
// it, so a pivot column never goes missing because of how many rows read
// it.
func (rn *runner) pivotPass() ([]any, error) {
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
	// A pivot over a link names its columns by the records' labels and reads
	// them in label order, as record groups do.
	label, order := "NULL", "(pk IS NULL), pk"
	if t := pv.path.target; t != nil {
		label = labelValueSQL(s, t, labelFieldOf(t, ""), "pk")
		order = "(pk IS NULL), (__label IS NULL), __label, pk"
	}
	body := fmt.Sprintf("SELECT pk, __label, COUNT(*) OVER () AS __n FROM (SELECT pk, %s AS __label FROM (SELECT DISTINCT pk FROM w)) ORDER BY %s LIMIT %s",
		label, order, s.bind(int64(pv.max+1)))
	rows, err := rn.query(s, []string{r.render()}, body)
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, len(rows))
	rn.pivotLabels = make([]any, 0, len(rows))
	for i, row := range rows {
		if i < pv.max {
			values = append(values, row["pk"])
			rn.pivotLabels = append(rn.pivotLabels, row["__label"])
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
func (c *compiler) pivotCells(lv *level, values []any) ([]string, error) {
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
	out := make([]string, 0, len(values))
	for j, v := range values {
		match := fmt.Sprintf("CASE WHEN w.pk IS %s THEN %s END", lv.r.s.bind(v), arg)
		ref := lv.agg(fmt.Sprintf("q%d", j), aggregateSQL(cell.col.Aggregate, match, cell.scale))
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

// pivotLabel is the j-th pivot value as a column label: a record's label for
// a pivot over a link, else the value as a row label shows it.
func (rn *runner) pivotLabel(j int, key any) string {
	pv := rn.c.p.pivot
	if pv.path.target != nil {
		return labelText(rn.pivotLabels[j])
	}
	return rn.display(&colPlan{col: &Column{}, path: pv.path}, key, pv.pivot.Bucket != "")
}
