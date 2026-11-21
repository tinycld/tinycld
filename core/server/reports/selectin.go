package reports

import "strings"

// selectIn is in of a Select field with a list of texts that are all option
// values of the field, with the same test on the stored column:
//
//	(NULLIF(c, '') IN (SELECT +value FROM json_each({:p})) AND c IN ({:p}, {:p}, NULL))
//
// A Select reads as NULLIF of its column (fieldSQL) and the list is read back
// from one bound JSON array (jsonSelect). No index on c serves that term, and
// SQLite does not see that it fails for a row with no linked row, so it keeps
// the LEFT JOIN to the linked row and scans the rows it could search. The
// second term is one a plain index on c serves and one that is never true
// for a NULL c, so the planner may start at the linked rows.
//
// The second list binds one placeholder for each distinct value. The values
// are option values, so their count is at most the count of the field's
// options, far under SQLite's 32,766 variables. No value is ever written
// into the SQL text.
//
// The NULL at the end of the second list keeps the value of the condition.
// Where c is in the list, both terms are 1. Where c is another text, the
// first term is 0. Where c is NULL (no linked row, or one the reader cannot
// see) or blank, the first term is NULL, and so is the second (x IN (…, NULL)
// is NULL, not 0, for an x the list does not hold): the condition stays
// unknown, as a column and eq with false read it. Without the NULL a blank c
// would give 0.
//
// "" when a is no such Select, or the list holds another value or no value,
// and in is written as before.
func selectIn(bind func(any) string, a, list expr) string {
	if legacySQL.noSelectIn || a.sel.col == "" || a.sel.sql != a.sql || len(list.listed) == 0 {
		return ""
	}
	seen := map[string]bool{}
	for _, v := range list.listed {
		if !storedOption(a.options, v) {
			return ""
		}
		seen[v] = true
	}
	marks := make([]string, 0, len(seen)+1)
	for _, v := range list.listed {
		if seen[v] {
			marks = append(marks, bind(v))
			delete(seen, v)
		}
	}
	marks = append(marks, "NULL")
	return "(" + a.sql + " IN " + list.sql + " AND " + a.sel.col + " IN (" + strings.Join(marks, ", ") + "))"
}
