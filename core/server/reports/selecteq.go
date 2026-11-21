package reports

import "slices"

// storedSelect is the stored column (col) a Select field is read from as
// sql; a copy of the expr whose sql changed no longer reads it.
type storedSelect struct{ col, sql string }

// selectEq is eq of a Select field with a text literal that is one of its
// option values, with the same test on the stored column:
//
//	(NULLIF(c, '') IS {:p} AND c = {:p})
//
// A Select reads as the first term (fieldSQL), which no index on c serves,
// so SQLite cannot tell that the condition is selective and scans the rows
// it could search. The second term is one a plain index on c serves. It
// never changes the value: a NULL c (no linked row, or one the reader cannot
// see) gives 0 AND NULL, which is 0; a blank c gives 0 and 0; any other c
// gives the same answer two times. Both terms bind the value; no value is
// ever written into the SQL text. "" when neither side is such a pair, and
// eq is written as before.
func selectEq(args []expr) string {
	for i, a := range args[:2] {
		v := args[1-i]
		if a.sel.col == "" || a.sel.sql != a.sql || v.text == nil || !storedOption(a.options, *v.text) {
			continue
		}
		return "(" + a.sql + " IS " + v.sql + " AND " + a.sel.col + " = " + v.sql + ")"
	}
	return ""
}

// storedOption is true for a text that is one of a Select's option values.
func storedOption(options []Option, text string) bool {
	return slices.ContainsFunc(options, func(o Option) bool { return o.Value == text })
}
