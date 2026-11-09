package reports

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// groupIndex is the index of the group with id, or -1.
func (c *compiler) groupIndex(id string) int {
	for i, g := range c.p.groups {
		if g.group.ID == id {
			return i
		}
	}
	return -1
}

// recordOf finds the group a record leaf names and the source its key
// links to.
func (c *compiler) recordOf(group, at string) (int, *Source, error) {
	gi := c.groupIndex(group)
	if gi < 0 {
		return 0, nil, unknownRef(at, "unknown_group", fmt.Sprintf("the report has no group %q", group))
	}
	if gi >= len(c.groupTargets) || c.groupTargets[gi] == nil {
		return 0, nil, invalid(at, fmt.Sprintf("the group %q is not grouped by a link: it has no record", group))
	}
	return gi, c.groupTargets[gi], nil
}

// bindRecordPath binds a record leaf's path on the group's target: to-one
// links, then a field. An amount with its own currency field carries that
// currency (recordCurrency).
func bindRecordPath(app core.App, target *Source, p Path, at string) (*pathPlan, error) {
	if len(p) == 0 {
		return nil, invalid(at, "a record leaf names a field of the record")
	}
	return bindValuePath(app, target, p, at+".path", pathValue)
}

// recordExpr types a record leaf; its context fills in sql and cur.
func recordExpr(bound *pathPlan) expr {
	return pathExpr("", bound)
}

// recordCurrencyID names a record amount's currency for combining.
func recordCurrencyID(group string, p Path) string {
	raw, _ := json.Marshal(p)
	return "rec:" + group + ":" + string(raw)
}

// recordCurrency is a record amount's currency read beside it: its record's
// currency field, keyed like the value (key NULL: no record, no currency), or
// the source's fixed currency.
func recordCurrency(s *stmt, target *Source, bound *pathPlan, key, id string) currency {
	return recordCurrencyBy(bound, key, id, func(p *pathPlan) string { return recordSQL(s, target, p, key) })
}

// recordCurrencyBy is recordCurrency with the record's currency field read by
// read.
func recordCurrencyBy(bound *pathPlan, key, id string, read func(*pathPlan) string) currency {
	f := bound.field
	switch {
	case f.Type != Money:
		return currency{}
	case f.Currency != "" && key == "NULL":
		return currency{sql: "NULL", id: id}
	case f.Currency != "":
		return currency{sql: read(currencyPath(bound.hops, bound.src, f)), id: id}
	case bound.src.Currency != nil:
		return currency{fixed: bound.src}
	}
	return currency{}
}

// currencyPath is the path to the currency field of the amount f, on src
// after hops.
func currencyPath(hops []hopPlan, src *Source, f *Field) *pathPlan {
	return &pathPlan{hops: hops, field: src.field(f.Currency), src: src}
}

// recordCurrencySQL reads the currency field of the amount f (on src, after
// hops from target) of the record key points at, through the rule CTEs, so a
// record the reader cannot see has no currency. A record label's currency is
// read the same way (labelCurrencySQL).
func recordCurrencySQL(s *stmt, target *Source, hops []hopPlan, src *Source, f *Field, key string) string {
	return recordSQL(s, target, currencyPath(hops, src, f), key)
}

// recordSQL reads a group record's field: a subquery on the target's rule
// CTE, keyed by the group key, so a record the reader cannot see reads
// blank.
func recordSQL(s *stmt, target *Source, bound *pathPlan, key string) string {
	sc := newScope(s, target)
	v := sc.value(bound)
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", v.sql, sc.render(), q(sc.alias, "id"), key)
}

// recordJoins reads the group records of a select's rows (alias, whose
// k<i> holds group i's key) by joining each group's record once, by its key,
// under the select: SQLite runs a correlated subquery again at every place a
// row reads it, and a formula reads one record field many times (a rank by
// the record's type, its normal side). The join reads the record through its
// rule CTE, so a record the reader cannot see reads blank, as recordSQL's
// subquery does; every hop is a left join, so a missing linked row blanks the
// value read through it and nothing else, as one subquery per read does.
type recordJoins struct {
	s      *stmt
	alias  string
	groups []int
	scopes map[int]*scope
	cols   []string
	as     map[string]string
}

func newRecordJoins(s *stmt, alias string) *recordJoins {
	return &recordJoins{s: s, alias: alias, scopes: map[int]*scope{}, as: map[string]string{}}
}

// column reads group gi's record field bound and returns its reference on
// the rows: one column per field, however many places read it.
func (j *recordJoins) column(gi int, target *Source, bound *pathPlan) string {
	sc, ok := j.scopes[gi]
	if !ok {
		sc = newScope(j.s, target)
		j.scopes[gi] = sc
		j.groups = append(j.groups, gi)
	}
	sql := sc.value(bound).sql
	name, ok := j.as[sql]
	if !ok {
		name = fmt.Sprintf("rc%d", len(j.as))
		j.as[sql] = name
		j.cols = append(j.cols, sql+" AS "+name)
	}
	return j.alias + "." + name
}

// reader reads group gi's record fields as columns, for recordCurrencyBy.
func (j *recordJoins) reader(gi int, target *Source) func(*pathPlan) string {
	return func(p *pathPlan) string { return j.column(gi, target, p) }
}

func (j *recordJoins) any() bool { return j != nil && len(j.cols) > 0 }

// from is from (rows read as alias) with the record columns added.
func (j *recordJoins) from(from string) string {
	joins := make([]string, 0, len(j.groups))
	for _, gi := range j.groups {
		sc := j.scopes[gi]
		joins = append(joins, fmt.Sprintf("LEFT JOIN %s ON %s IS %s.k%d", sc.from, q(sc.alias, "id"), j.alias, gi))
		for _, join := range sc.sql {
			if strings.HasPrefix(join, "JOIN ") {
				join = "LEFT " + join
			}
			joins = append(joins, join)
		}
	}
	return fmt.Sprintf("(SELECT %s.*, %s FROM %s %s) %s", j.alias, strings.Join(j.cols, ", "), from, strings.Join(joins, "\n"), j.alias)
}

// labelText is a label value as row text: (none) for a blank.
func labelText(v any) string {
	switch x := v.(type) {
	case nil:
		return blankLabel
	case string:
		if x == "" {
			return blankLabel
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}
