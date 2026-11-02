package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// expr is one compiled formula node: its SQL and what the compiler knows
// about it.
type expr struct {
	sql string
	// typ is the value's type; "" is a NULL literal, which fits any type.
	typ FieldType
	// lo and hi hold a dateRange param's bounds; sql is empty for it.
	rng    bool
	lo, hi string
	// list is a list of values for in: sql is "(?, ?, …)".
	list bool
	// unset is a param with no value: in a filter, a condition on it
	// matches every row.
	unset bool
	agg   bool // contains an aggregate
	win   bool // contains a window function
	// cur is where a money value's currency comes from.
	cur currency
	// target is the source a relation value points at, nil for any other
	// value. A group by such a value is a record group.
	target *Source
}

// currency is where a money value's currency comes from: a currency key of w
// (the ck<n> that holds each row's currency) or a source's fixed currency.
// The zero value is a value that is not money, or a section subtotal, which
// a layout reads in one currency at a time.
type currency struct {
	key   string
	fixed *Source
}

func (a currency) none() bool { return a.key == "" && a.fixed == nil }

// same is true for one currency source: a key equals itself, a fixed
// currency the same source's.
func (a currency) same(b currency) bool {
	if a.key != b.key || (a.fixed == nil) != (b.fixed == nil) {
		return false
	}
	return a.fixed == nil || a.fixed.Key == b.fixed.Key
}

// withCurrency gives a money value its currency; any other value has none.
func withCurrency(e expr, cur currency) expr {
	if e.typ == Money {
		e.cur = cur
	}
	return e
}

// combined is the currency of an amount made of values: refused when two of
// them have different currencies, which would add EUR to USD cents.
func combined(values []expr, at string) (currency, error) {
	var out currency
	for _, v := range values {
		if v.cur.none() {
			continue
		}
		if !out.none() && !out.same(v.cur) {
			return currency{}, invalid(at, "amounts in different currencies cannot be combined")
		}
		out = v.cur
	}
	return out, nil
}

// fctx says what a formula's leaves and context-bound operators mean where
// the formula is compiled: a filter over joined rows, a detail row, a group
// row, a layout row, or the formula-table harness.
type fctx interface {
	col(id, at string) (expr, error)
	path(p Path, at string) (expr, error)
	param(id, at string) (expr, error)
	subtotal(id, at string) (expr, error)
	aggregate(op Aggregate, arg *Node, scale int, at string) (expr, error)
	window(op string, arg *Node, scale int, at string) (expr, error)
	exists(n *Node, at string) (expr, error)
	// lookup compiles a lookup node: a value from another source's rows.
	lookup(n *Node, at string) (expr, error)
	// record compiles a record leaf: path read from the linked record of
	// the group named group.
	record(group string, p Path, at string) (expr, error)
	// bind adds a value to the statement's parameters and returns its
	// placeholder.
	bind(v any) string
	// fiscalStart is the SQL of the base source's fiscal start month.
	fiscalStart() string
	// filter is true while compiling a filter.
	filter() bool
}

// aggregate and window receive the operator node's path; the value they
// total is at path + ".args.0".

// Formula limits: a tree deeper or larger than these is refused before it
// reaches SQLite.
const (
	maxFormulaDepth = 32
	maxFormulaNodes = 1000
)

func compileFormula(c fctx, n *Node, at string) (expr, error) {
	count := 0
	return compileNode(c, n, at, 0, &count)
}

func compileNode(c fctx, n *Node, at string, depth int, count *int) (expr, error) {
	if n == nil {
		return expr{}, invalid(at, "a formula node is missing")
	}
	*count++
	if depth > maxFormulaDepth || *count > maxFormulaNodes {
		return expr{}, invalid(at, fmt.Sprintf("a formula has at most %d levels and %d nodes", maxFormulaDepth, maxFormulaNodes))
	}
	if n.Op == "lookup" {
		return c.lookup(n, at)
	}
	if n.Source != "" || n.Pick != "" {
		return expr{}, invalid(at, "only lookup takes a source and a pick")
	}
	if kinds := nodeKinds(n); kinds != 1 {
		return expr{}, invalid(at, errNodeKind)
	}
	switch {
	case n.Record != "":
		return c.record(n.Record, n.Path, at)
	case n.Col != "":
		return c.col(n.Col, at)
	case n.Path != nil:
		return c.path(n.Path, at+".path")
	case n.Param != "":
		return c.param(n.Param, at)
	case n.Subtotal != "":
		return c.subtotal(n.Subtotal, at)
	case n.Value != nil:
		return literal(c, n.Value, at)
	}
	if n.Link != nil || n.Where != nil {
		if n.Op != "exists" && n.Op != "notExists" {
			return expr{}, invalid(at, "only exists and notExists take a link and a where")
		}
	}
	switch n.Op {
	case "exists", "notExists":
		e, err := c.exists(n, at)
		if err != nil || n.Op == "exists" || e.unset {
			return e, err
		}
		e.sql = "(NOT " + e.sql + ")"
		return e, nil
	case "sum", "count", "countDistinct", "avg":
		return aggregateNode(c, Aggregate(n.Op), n, at)
	case "min", "max":
		if len(n.Args) == 1 {
			return aggregateNode(c, Aggregate(n.Op), n, at)
		}
	case "runningSum", "percentOfParent", "percentOfTotal":
		if len(n.Args) != 1 {
			return expr{}, invalid(at, n.Op+" takes one value")
		}
		scale := 0
		if n.Op != "runningSum" {
			s, err := needScale(n, at)
			if err != nil {
				return expr{}, err
			}
			scale = s
		}
		e, err := c.window(n.Op, n.Args[0], scale, at)
		e.win = true
		return e, err
	}
	args := make([]expr, len(n.Args))
	for i, a := range n.Args {
		e, err := compileNode(c, a, fmt.Sprintf("%s.args.%d", at, i), depth+1, count)
		if err != nil {
			return expr{}, err
		}
		args[i] = e
	}
	e, err := operator(c, n, args, at)
	if err != nil {
		return expr{}, err
	}
	for _, a := range args {
		e.agg = e.agg || a.agg
		e.win = e.win || a.win
	}
	return e, nil
}

// errNodeKind refuses a node that is not exactly one kind of node.
const errNodeKind = "a formula node is one of op, col, path, param, value, subtotal or record"

func nodeKinds(n *Node) int {
	kinds := 0
	// A record leaf's path is part of it. (A lookup, whose value is a
	// formula, is compiled before nodes are counted.)
	path := n.Path != nil && n.Record == ""
	for _, set := range []bool{n.Op != "", n.Col != "", path, n.Param != "", n.Value != nil, n.Subtotal != "", n.Record != ""} {
		if set {
			kinds++
		}
	}
	return kinds
}

func aggregateNode(c fctx, op Aggregate, n *Node, at string) (expr, error) {
	if len(n.Args) != 1 {
		return expr{}, invalid(at, string(op)+" takes one value")
	}
	scale := 0
	if op == Avg {
		s, err := needScale(n, at)
		if err != nil {
			return expr{}, err
		}
		scale = s
	}
	e, err := c.aggregate(op, n.Args[0], scale, at)
	e.agg = true
	return e, err
}

func needScale(n *Node, at string) (int, error) {
	if n.Scale == nil || *n.Scale < 0 || *n.Scale > MaxScale {
		return 0, invalid(at+".scale", fmt.Sprintf("%s needs a scale from 0 to %d", n.Op, MaxScale))
	}
	return *n.Scale, nil
}

// literal binds a {value}. A whole number binds as an integer, so money
// arithmetic stays exact.
func literal(c fctx, raw json.RawMessage, at string) (expr, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return expr{}, invalid(at, "a value is not valid JSON")
	}
	switch x := v.(type) {
	case nil:
		return expr{sql: "NULL"}, nil
	case bool:
		return expr{sql: c.bind(x), typ: Bool}, nil
	case string:
		return expr{sql: c.bind(x), typ: Text}, nil
	case json.Number:
		return expr{sql: c.bind(numberValue(x)), typ: Number}, nil
	case []any:
		parts := make([]string, 0, len(x))
		var typ FieldType
		for _, item := range x {
			switch y := item.(type) {
			case string:
				parts = append(parts, c.bind(y))
				typ = Text
			case json.Number:
				parts = append(parts, c.bind(numberValue(y)))
				typ = Number
			default:
				return expr{}, invalid(at, "a value list holds text or numbers")
			}
		}
		return expr{sql: "(" + strings.Join(parts, ", ") + ")", typ: typ, list: true}, nil
	}
	return expr{}, invalid(at, "a value is text, a number, true, false, null or a list")
}

func numberValue(n json.Number) any {
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
		return i
	}
	f, _ := n.Float64()
	return f
}

func isNumeric(t FieldType) bool {
	return t == Number || t == Money || t == Percent || t == Bool || t == ""
}

func isDateLike(t FieldType) bool {
	return t == Date || t == DateTime || t == Text || t == ""
}

// num coalesces an arithmetic operand: a blank counts as zero, as in the
// forms engine.
func num(e expr) string { return "COALESCE(" + e.sql + ", 0)" }

// unsetCondition is a filter condition on an unset param: it matches every
// row, and an and, or or not over it leaves it out.
var unsetCondition = expr{sql: "1", typ: Bool, unset: true}

// operator compiles an operator node. Amounts in different currencies are
// refused. In a filter, a condition on an unset param becomes
// unsetCondition, after its arguments are checked as they are for a set
// one, so Validate refuses what a run would.
func operator(c fctx, n *Node, args []expr, at string) (expr, error) {
	e, err := operatorExpr(c, n, args, at)
	if err != nil {
		return expr{}, err
	}
	// An amount takes its operands' currency. Only amounts carry one, so a
	// condition or a count among the operands adds none.
	if e.typ == Money {
		if e.cur, err = combined(args, at); err != nil {
			return expr{}, err
		}
	}
	if c.filter() && conditionOps[n.Op] && slices.ContainsFunc(args, func(a expr) bool { return a.unset }) {
		return unsetCondition, nil
	}
	return e, nil
}

func operatorExpr(c fctx, n *Node, args []expr, at string) (expr, error) {
	argc := func(min, max int) error {
		if len(args) < min || (max >= 0 && len(args) > max) {
			if min == max {
				return invalid(at, fmt.Sprintf("%s takes %d values", n.Op, min))
			}
			return invalid(at, fmt.Sprintf("%s takes at least %d values", n.Op, min))
		}
		return nil
	}
	numeric := func() error {
		for i, a := range args {
			if !isNumeric(a.typ) || a.list || a.rng {
				return invalid(fmt.Sprintf("%s.args.%d", at, i), n.Op+" needs numbers")
			}
		}
		return nil
	}
	for i, a := range args {
		if (a.rng && n.Op != "between") || (a.list && n.Op != "in") {
			return expr{}, invalid(fmt.Sprintf("%s.args.%d", at, i), "a range or a list fits only between or in")
		}
	}
	switch n.Op {
	case "add", "sub":
		max := -1
		if n.Op == "sub" {
			max = 2
		}
		if err := argc(2, max); err != nil {
			return expr{}, err
		}
		if err := numeric(); err != nil {
			return expr{}, err
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = num(a)
		}
		sep := " + "
		if n.Op == "sub" {
			sep = " - "
		}
		return wholeCents(expr{sql: "(" + strings.Join(parts, sep) + ")", typ: sumType(args)}, args), nil
	case "mul":
		if err := argc(2, -1); err != nil {
			return expr{}, err
		}
		if err := numeric(); err != nil {
			return expr{}, err
		}
		parts := make([]string, len(args))
		amounts := 0
		for i, a := range args {
			parts[i] = num(a)
			if a.typ == Money {
				amounts++
			}
		}
		product := "(" + strings.Join(parts, " * ") + ")"
		switch {
		case amounts > 1:
			return expr{}, invalid(at, "an amount times an amount is not an amount")
		case amounts == 1:
			// Whole cents: a product with a fraction of a cent rounds.
			return expr{sql: "tc_round(" + product + ", 0)", typ: Money}, nil
		}
		return expr{sql: product, typ: Number}, nil
	case "div":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if err := numeric(); err != nil {
			return expr{}, err
		}
		scale, err := needScale(n, at)
		if err != nil {
			return expr{}, err
		}
		typ := Number
		if args[0].typ == Money && args[1].typ != Money {
			if scale != 0 {
				return expr{}, invalid(at+".scale", "an amount divided by a number keeps whole cents: use scale 0")
			}
			typ = Money
		}
		return expr{sql: fmt.Sprintf("tc_div(%s, %s, %d)", num(args[0]), num(args[1]), scale), typ: typ}, nil
	case "round":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		if err := numeric(); err != nil {
			return expr{}, err
		}
		scale, err := needScale(n, at)
		if err != nil {
			return expr{}, err
		}
		typ := args[0].typ
		if typ == Bool || typ == "" {
			typ = Number
		}
		return expr{sql: fmt.Sprintf("tc_round(%s, %d)", num(args[0]), scale), typ: typ}, nil
	case "min", "max":
		if err := argc(2, -1); err != nil {
			return expr{}, err
		}
		if err := numeric(); err != nil {
			return expr{}, err
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = num(a)
		}
		return wholeCents(expr{sql: n.Op + "(" + strings.Join(parts, ", ") + ")", typ: sumType(args)}, args), nil
	case "eq", "ne":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !comparable(args[0].typ, args[1].typ) {
			return expr{}, invalid(at, fmt.Sprintf("%s compares values of one kind, not %s and %s", n.Op, args[0].typ, args[1].typ))
		}
		op := " IS "
		if n.Op == "ne" {
			op = " IS NOT "
		}
		return expr{sql: "(" + args[0].sql + op + args[1].sql + ")", typ: Bool}, nil
	case "lt", "lte", "gt", "gte":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !ordered(args[0].typ, args[1].typ) {
			return expr{}, invalid(at, fmt.Sprintf("%s compares numbers, dates or text, not %s and %s", n.Op, args[0].typ, args[1].typ))
		}
		return expr{sql: "(" + args[0].sql + " " + comparisons[n.Op] + " " + args[1].sql + ")", typ: Bool}, nil
	case "between":
		return between(args, at)
	case "in":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !args[1].list {
			return expr{}, invalid(at+".args.1", "in needs a list")
		}
		if args[1].sql == "()" {
			return expr{sql: "0", typ: Bool}, nil
		}
		return expr{sql: "(" + args[0].sql + " IN " + args[1].sql + ")", typ: Bool}, nil
	case "and", "or":
		if err := argc(2, -1); err != nil {
			return expr{}, err
		}
		parts := make([]string, 0, len(args))
		for i, a := range args {
			if a.typ != Bool && a.typ != "" {
				return expr{}, invalid(fmt.Sprintf("%s.args.%d", at, i), n.Op+" needs conditions")
			}
			if !a.unset {
				parts = append(parts, a.sql)
			}
		}
		if len(parts) == 0 {
			return unsetCondition, nil
		}
		return expr{sql: "(" + strings.Join(parts, " "+strings.ToUpper(n.Op)+" ") + ")", typ: Bool}, nil
	case "not":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		if args[0].typ != Bool {
			return expr{}, invalid(at+".args.0", "not needs a condition")
		}
		if args[0].unset {
			return unsetCondition, nil
		}
		// A condition on a blank is false, so its negation is true.
		return expr{sql: "(NOT COALESCE(" + args[0].sql + ", 0))", typ: Bool}, nil
	case "concat":
		if err := argc(1, -1); err != nil {
			return expr{}, err
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = "COALESCE(" + a.sql + ", '')"
		}
		return expr{sql: "(" + strings.Join(parts, " || ") + ")", typ: Text}, nil
	case "upper", "lower":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		return expr{sql: n.Op + "(" + args[0].sql + ")", typ: Text}, nil
	case "contains", "startsWith":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if n.Op == "contains" {
			return expr{sql: "(instr(lower(" + args[0].sql + "), lower(" + args[1].sql + ")) > 0)", typ: Bool}, nil
		}
		return expr{sql: "(substr(lower(" + args[0].sql + "), 1, length(" + args[1].sql + ")) = lower(" + args[1].sql + "))", typ: Bool}, nil
	case "year", "quarter", "month", "fiscalYear", "fiscalPeriod":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		if !isDateLike(args[0].typ) {
			return expr{}, invalid(at+".args.0", n.Op+" needs a date")
		}
		return expr{sql: datePart(c, n.Op, args[0].sql), typ: Number}, nil
	case "addDays", "addMonths":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !isDateLike(args[0].typ) || !isNumeric(args[1].typ) {
			return expr{}, invalid(at, n.Op+" needs a date and a number")
		}
		count := "CAST(" + num(args[1]) + " AS INTEGER)"
		if n.Op == "addDays" {
			return expr{sql: "date(" + args[0].sql + ", printf('%+d days', " + count + "))", typ: Date}, nil
		}
		return expr{sql: "tc_add_months(" + args[0].sql + ", " + count + ")", typ: Date}, nil
	case "diffDays":
		if err := argc(2, 2); err != nil {
			return expr{}, err
		}
		if !isDateLike(args[0].typ) || !isDateLike(args[1].typ) {
			return expr{}, invalid(at, "diffDays needs two dates")
		}
		return expr{sql: "CAST(julianday(substr(" + args[1].sql + ", 1, 10)) - julianday(substr(" + args[0].sql + ", 1, 10)) AS INTEGER)", typ: Number}, nil
	case "if":
		if err := argc(3, 3); err != nil {
			return expr{}, err
		}
		if args[0].typ != Bool {
			return expr{}, invalid(at+".args.0", "if needs a condition first")
		}
		typ, ok := commonType(args[1], args[2])
		if !ok {
			return expr{}, invalid(at, "both results of if are of one kind")
		}
		target, err := oneTarget(args[1:], "if", at)
		if err != nil {
			return expr{}, err
		}
		out := wholeCents(expr{sql: "(CASE WHEN " + args[0].sql + " THEN " + args[1].sql + " ELSE " + args[2].sql + " END)", typ: typ}, args[1:])
		out.target = target
		return out, nil
	case "case":
		if err := argc(2, -1); err != nil {
			return expr{}, err
		}
		var b strings.Builder
		b.WriteString("(CASE")
		results := []expr{}
		for i := 0; i+1 < len(args); i += 2 {
			if args[i].typ != Bool {
				return expr{}, invalid(fmt.Sprintf("%s.args.%d", at, i), "case takes condition and result pairs")
			}
			b.WriteString(" WHEN " + args[i].sql + " THEN " + args[i+1].sql)
			results = append(results, args[i+1])
		}
		if len(args)%2 == 1 {
			b.WriteString(" ELSE " + args[len(args)-1].sql)
			results = append(results, args[len(args)-1])
		}
		b.WriteString(" END)")
		typ, ok := commonType(results...)
		if !ok {
			return expr{}, invalid(at, "every result of case is of one kind")
		}
		target, err := oneTarget(results, "case", at)
		if err != nil {
			return expr{}, err
		}
		out := wholeCents(expr{sql: b.String(), typ: typ}, results)
		out.target = target
		return out, nil
	case "coalesce":
		if err := argc(1, -1); err != nil {
			return expr{}, err
		}
		typ, ok := commonType(args...)
		if !ok {
			return expr{}, invalid(at, "coalesce takes values of one kind")
		}
		target, err := oneTarget(args, "coalesce", at)
		if err != nil {
			return expr{}, err
		}
		if len(args) == 1 {
			return expr{sql: args[0].sql, typ: typ, target: target}, nil
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = a.sql
		}
		out := wholeCents(expr{sql: "COALESCE(" + strings.Join(parts, ", ") + ")", typ: typ}, args)
		out.target = target
		return out, nil
	case "isEmpty":
		if err := argc(1, 1); err != nil {
			return expr{}, err
		}
		return expr{sql: "(" + args[0].sql + " IS NULL OR " + args[0].sql + " = '')", typ: Bool}, nil
	}
	return expr{}, invalid(at, fmt.Sprintf("unknown operator %q", n.Op))
}

// oneTarget is the source the relations among values point at; two that
// point at different sources are refused, since a record group could not
// read both.
func oneTarget(values []expr, op, at string) (*Source, error) {
	var out *Source
	for _, v := range values {
		if v.target == nil {
			continue
		}
		if out != nil && out.Key != v.target.Key {
			return nil, invalid(at, fmt.Sprintf("the results of %s link to different sources", op))
		}
		out = v.target
	}
	return out, nil
}

// conditionOps match every row in a filter when one of their values is an
// unset param, so an empty filter chip filters nothing.
var conditionOps = map[string]bool{
	"eq": true, "ne": true, "lt": true, "lte": true, "gt": true, "gte": true,
	"between": true, "in": true, "contains": true, "startsWith": true,
}

var comparisons = map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">="}

func between(args []expr, at string) (expr, error) {
	switch {
	case len(args) == 2 && args[1].rng:
		x := args[0]
		if !isDateLike(x.typ) {
			return expr{}, invalid(at+".args.0", "a date range holds dates")
		}
		day := "substr(" + x.sql + ", 1, 10)"
		return expr{sql: "(" + day + " >= " + args[1].lo + " AND " + day + " <= " + args[1].hi + ")", typ: Bool}, nil
	case len(args) == 3:
		if !ordered(args[0].typ, args[1].typ) || !ordered(args[0].typ, args[2].typ) {
			return expr{}, invalid(at, "between compares numbers, dates or text")
		}
		return expr{sql: "(" + args[0].sql + " >= " + args[1].sql + " AND " + args[0].sql + " <= " + args[2].sql + ")", typ: Bool}, nil
	}
	return expr{}, invalid(at, "between takes a value and a date range param, or a value, a low and a high")
}

func datePart(c fctx, op, day string) string {
	switch op {
	case "year":
		return "CAST(substr(" + day + ", 1, 4) AS INTEGER)"
	case "month":
		return "CAST(substr(" + day + ", 6, 2) AS INTEGER)"
	case "quarter":
		return "((CAST(substr(" + day + ", 6, 2) AS INTEGER) + 2) / 3)"
	case "fiscalYear":
		return "tc_fiscal_year(" + day + ", " + c.fiscalStart() + ")"
	}
	return "tc_fiscal_period(" + day + ", " + c.fiscalStart() + ")"
}

// sumType is the type of a sum, difference, min or max: money when any
// operand is money, else percent when any is a percent, else number.
func sumType(args []expr) FieldType {
	typ := Number
	for _, a := range args {
		if a.typ == Money {
			return Money
		}
		if a.typ == Percent {
			typ = Percent
		}
	}
	return typ
}

// wholeCents rounds an amount that mixes in a non-amount operand: such an
// operand can carry a fraction of a cent. tc_round leaves an integer as it
// is and keeps NULL, so amounts alone stay exact.
func wholeCents(e expr, operands []expr) expr {
	if e.typ != Money {
		return e
	}
	for _, o := range operands {
		if o.typ != Money && o.typ != "" {
			e.sql = "tc_round(" + e.sql + ", 0)"
			return e
		}
	}
	return e
}

func comparable(a, b FieldType) bool {
	if a == "" || b == "" || a == b {
		return true
	}
	if isNumeric(a) && isNumeric(b) {
		return true
	}
	return isDateLike(a) && isDateLike(b) || textual(a) && textual(b)
}

func textual(t FieldType) bool {
	return t == Text || t == Select || t == Relation
}

func ordered(a, b FieldType) bool {
	if isNumeric(a) && isNumeric(b) {
		return true
	}
	return (isDateLike(a) || textual(a)) && (isDateLike(b) || textual(b))
}

// commonType is the type every non-NULL value shares.
func commonType(values ...expr) (FieldType, bool) {
	var typ FieldType
	for _, v := range values {
		switch {
		case v.typ == "":
		case typ == "":
			typ = v.typ
		case !comparable(typ, v.typ):
			return "", false
		case v.typ == Money:
			typ = Money
		}
	}
	return typ, true
}

// aggregateSQL is an aggregate of a value over the rows of a group. A sum of
// no rows is 0, as in the forms engine; SQLite's sum() keeps integers exact
// and raises "integer overflow" instead of rounding. An average divides with
// tc_div, so money averages to whole cents.
func aggregateSQL(op Aggregate, arg string, scale int) string {
	switch op {
	case Sum:
		return "COALESCE(sum(" + arg + "), 0)"
	case Count:
		return "count(" + arg + ")"
	case CountDistinct:
		return "count(DISTINCT " + arg + ")"
	case Min, Max:
		return string(op) + "(" + arg + ")"
	}
	return fmt.Sprintf("(CASE WHEN count(%[1]s) = 0 THEN NULL ELSE tc_div(sum(%[1]s), count(%[1]s), %[2]d) END)", arg, scale)
}

// aggregateType is the type of an aggregate of a value of type t.
func aggregateType(op Aggregate, t FieldType, scale int) FieldType {
	switch op {
	case Count, CountDistinct:
		return Number
	case Avg:
		if t == Money && scale == 0 {
			return Money
		}
		return Number
	}
	if t == Bool {
		return Number
	}
	return t
}

// checkAggregate refuses an aggregate a value of type t cannot have.
func checkAggregate(op Aggregate, t FieldType, scale int, at string) error {
	switch op {
	case Sum, Avg:
		if !isNumeric(t) {
			return invalid(at, fmt.Sprintf("%s needs numbers", op))
		}
		if op == Avg && t == Money && scale != 0 {
			return invalid(at, "an average of amounts keeps whole cents: use scale 0")
		}
	case Count, CountDistinct, Min, Max:
	default:
		return invalid(at, fmt.Sprintf("unknown aggregate %q", op))
	}
	return nil
}

// ctxBase is what every formula context shares: a subtotal is refused (only
// a layout row reads one), an exists belongs in the filter, and an
// aggregate's argument is compiled on a row and checked the same way.
type ctxBase struct{}

func (ctxBase) subtotal(_, at string) (expr, error) {
	return expr{}, invalid(at, "only a computed layout row reads a subtotal")
}

func (ctxBase) exists(n *Node, at string) (expr, error) {
	return expr{}, invalid(at, n.Op+" belongs in the filter")
}

func (ctxBase) lookup(_ *Node, at string) (expr, error) {
	return expr{}, invalid(at, "a lookup is read in a column, the filter, a group, a sort or a label")
}

func (ctxBase) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, invalid(at, "a record field is read on a group's rows, the rows below them, a sort, a label or a having")
}

// aggregateArg compiles an aggregate's argument over one base row and
// checks that op accepts it.
func (ctxBase) aggregateArg(r *rowsCTE, visiting map[string]bool, op Aggregate, arg *Node, scale int, at string) (expr, error) {
	inner := &rowsCtx{r: r, inAggregate: true, visiting: visiting}
	e, err := compileFormula(inner, arg, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	if err := checkAggregate(op, e.typ, scale, at+".args.0"); err != nil {
		return expr{}, err
	}
	return e, nil
}
