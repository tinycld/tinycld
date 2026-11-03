package reports

import (
	"encoding/json"
	"fmt"
	"strconv"

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
	f := bound.field
	switch {
	case f.Type != Money:
		return currency{}
	case f.Currency != "" && key == "NULL":
		return currency{sql: "NULL", id: id}
	case f.Currency != "":
		return currency{sql: recordCurrencySQL(s, target, bound.hops, bound.src, f, key), id: id}
	case bound.src.Currency != nil:
		return currency{fixed: bound.src}
	}
	return currency{}
}

// recordCurrencySQL reads the currency field of the amount f (on src, after
// hops from target) of the record key points at, through the rule CTEs, so a
// record the reader cannot see has no currency. A record label's currency is
// read the same way (labelCurrencySQL).
func recordCurrencySQL(s *stmt, target *Source, hops []hopPlan, src *Source, f *Field, key string) string {
	return recordSQL(s, target, &pathPlan{hops: hops, field: src.field(f.Currency), src: src}, key)
}

// recordSQL reads a group record's field: a subquery on the target's rule
// CTE, keyed by the group key, so a record the reader cannot see reads
// blank.
func recordSQL(s *stmt, target *Source, bound *pathPlan, key string) string {
	sc := newScope(s, target)
	v := sc.value(bound)
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", v.sql, sc.render(), q(sc.alias, "id"), key)
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
