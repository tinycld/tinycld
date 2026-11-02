package reports

import "testing"

// valueText writes a value as the report shell's cell formatters do
// (core/reports/lib/format.ts and money.ts, en-US).
func TestValueTextMatchesTheShellsCells(t *testing.T) {
	cases := []struct {
		v    any
		typ  FieldType
		code string
		want string
	}{
		{int64(500000), Money, "USD", "$5,000.00"},
		{int64(-1234567), Money, "USD", "-$12,345.67"},
		{int64(150), Money, "eur", "€1.50"},
		{int64(150), Money, "CHF", "CHF 1.50"},
		{int64(150), Money, "dollars", "1.50 dollars"},
		{int64(150), Money, "", "1.50"},
		{-0.4, Money, "USD", "$0.00"},
		{1000.5, Money, "USD", "$10.01"},
		{int64(1), Bool, "", "Yes"},
		{int64(0), Bool, "", "No"},
		{int64(1234567), Number, "", "1,234,567"},
		{-1234.5, Number, "", "-1,234.5"},
		{0.1234567, Number, "", "0.123457"},
		{12.345, Percent, "", "12.3%"},
		{"2026-01-05 00:00:00.000Z", DateTime, "", "Jan 5, 2026"},
		{"2026-12-31", Date, "", "Dec 31, 2026"},
		{"1800-01-01", Date, "", "1800-01-01"},
		{"soon", Date, "", "soon"},
	}
	for _, c := range cases {
		if got := valueText(c.v, c.typ, c.code); got != c.want {
			t.Errorf("valueText(%v, %s, %q) = %q, want %q", c.v, c.typ, c.code, got, c.want)
		}
	}
}
