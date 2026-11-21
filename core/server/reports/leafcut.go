package reports

import (
	"fmt"
	"strings"
)

// leafCut is the condition that cuts w, in a detail statement with open
// leaves, to the base rows that can be in one of them, or "" for no cut. A
// group level whose key is a to-one link of the base row is cut by the link's
// stored column, so an index on that column serves the read: the column is
// in the keys gord binds at that level, or, when an open leaf's key there is
// blank, the row's key reads blank (an empty column, or a linked row the
// reader cannot see). gord stays the one place the keys are bound.
//
// The cut keeps every result: a row whose key at a cut level is no open
// leaf's key there is in no open leaf. The shown-level CTEs (shown is true)
// total the groups of levels 1 and below over w, so only the first level's
// key is cut then: it keeps every row of each first-level group that holds an
// open leaf, and so every row a shown check of an open leaf reads. A deeper
// key would drop rows of such a group and change its totals. There is no cut
// when win is set: a value of the statement is a running value or a percent
// (expr.win, read from the detail select's frame and the shown levels), which
// reads rows outside the open leaves.
func (c *compiler) leafCut(r *rowsCTE, leaves [][]any, shown, win bool) string {
	if len(leaves) == 0 || win {
		return ""
	}
	levels := len(c.p.groups)
	if shown {
		levels = min(levels, 1)
	}
	var terms []string
	for i := range levels {
		column := c.storedLinkKey(r, i)
		if column == "" {
			continue
		}
		term := fmt.Sprintf("%s IN (SELECT g%d FROM gord WHERE g%d IS NOT NULL)", column, i, i)
		if blankAt(leaves, i) {
			term = fmt.Sprintf("(%s OR %s IS NULL)", term, r.raw[fmt.Sprintf("k%d", i)])
		}
		terms = append(terms, term)
	}
	return strings.Join(terms, " AND ")
}

// storedLinkKey is the base row's stored column of group level i's key, when
// that key is a to-one link of the base row read by the linked row's id
// (fieldSQL of the linked row's id); "" for any other key.
func (c *compiler) storedLinkKey(r *rowsCTE, i int) string {
	g := c.p.groups[i]
	cp := g.col
	if g.group.Bucket != "" || cp.path == nil || cp.tm >= 0 || cp.nest > 0 {
		return ""
	}
	p := cp.path
	if len(p.hops) != 1 || p.field == nil || p.field.Key != idField.Key {
		return ""
	}
	h := p.hops[0]
	if h.mode != "" || h.link.Cardinality != ToOne || h.link.Field == "" {
		return ""
	}
	return q(r.root.alias, h.link.Field)
}

// blankAt is true when an open leaf's key at level i is blank.
func blankAt(leaves [][]any, i int) bool {
	for _, key := range leaves {
		if i < len(key) && key[i] == nil {
			return true
		}
	}
	return false
}
