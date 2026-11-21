package reports

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// boundParam matches a whole SQL fragment that is one bound parameter of the
// engine, and captures its name.
var boundParam = regexp.MustCompile(`^\{:(rp\d+)\}$`)

// pctScales lists the last argument of every tc_pct call in sql: the scale.
func pctScales(t *testing.T, sql string) []string {
	t.Helper()
	var out []string
	for rest := sql; ; {
		at := strings.Index(rest, "tc_pct(")
		if at < 0 {
			return out
		}
		rest = rest[at+len("tc_pct("):]
		depth, last, end := 0, 0, -1
		for i, r := range rest {
			switch {
			case r == '(':
				depth++
			case r == ')' && depth == 0:
				end = i
			case r == ')':
				depth--
			case r == ',' && depth == 0:
				last = i + 1
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			t.Fatalf("a tc_pct call does not close:\n%.200s", rest)
		}
		out = append(out, strings.TrimSpace(rest[last:end]))
	}
}

// A percent's scale is a bound parameter of its statement, never written
// into the text: the share of the Total (2 places) and the share of the
// parent (0 places) of clientShares. TestPercentColumnCarriesItsScale and
// TestPercentExportsPrintTheScale hold the figures.
func TestAPercentBindsItsScale(t *testing.T) {
	z := newZoo(t)
	withClientZoo(t, z)
	seen := observeReads(t)
	z.run(t, "ann", clientShares, Options{}, nil)
	scales := map[int64]bool{}
	for _, st := range *seen {
		for _, arg := range pctScales(t, st.sql) {
			m := boundParam.FindStringSubmatch(arg)
			if m == nil {
				t.Fatalf("a percent's scale is written into the statement: tc_pct(…, %s)", arg)
			}
			scale, ok := st.params[m[1]].(int64)
			if !ok {
				t.Fatalf("the scale's param %s = %#v, want an int64", m[1], st.params[m[1]])
			}
			scales[scale] = true
		}
	}
	if len(scales) != 2 || !scales[0] || !scales[2] {
		t.Fatalf("the bound scales = %v, want 0 and 2", scales)
	}
}

// monthOfFed lists ann's feedings by day, each with the first day of its
// month.
const monthOfFed = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"month","formula":{"op":"startOfMonth","args":[{"col":"fed"}]}}],
	"sort":[{"col":"fed"}]}`

// addedDay captures the day startOfMonth appends to a day's year and month.
var addedDay = regexp.MustCompile(`NULLIF\(substr\([^)]*, 1, 8\), ''\) \|\| (\S+)\)`)

// startOfMonth binds the day it adds to a day's year and month, and its
// figures stay: the first day of each feeding's month, blank for a feeding
// with no day.
func TestStartOfMonthBindsItsDay(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", monthOfFed, Options{}, nil)
	var got []string
	for _, r := range res.Rows {
		got = append(got, fmt.Sprint(r.Cells["fed"], "→", r.Cells["month"]))
	}
	want := "2026-01-05→2026-01-01 2026-01-20→2026-01-01 2026-02-10→2026-02-01 2026-03-03→2026-03-01 <nil>→<nil>"
	if strings.Join(got, " ") != want {
		t.Fatalf("the months = %q, want %q", got, want)
	}
	bound := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "'01'") {
			t.Fatalf("the day is written into the statement:\n%s", st.sql)
		}
		for _, m := range addedDay.FindAllStringSubmatch(st.sql, -1) {
			p := boundParam.FindStringSubmatch(m[1])
			if p == nil || st.params[p[1]] != "01" {
				t.Fatalf("the day after || is %s (%#v), want a param bound to \"01\"", m[1], st.params)
			}
			bound++
		}
	}
	if bound == 0 {
		t.Fatal("no statement holds startOfMonth's day")
	}
}

// The bounds of a blank date's pivot cell are bound like a bucket's days,
// never written into the text. TestAPivotCellOfABlankDateHasNoDrill holds
// the result: the blank column draws no drill.
func TestABlankPivotBoundIsBound(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "lion", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.rec(t, "f8", "zoo_feedings", map[string]any{"animal": nub.Id, "grams": 5, "cost": 1, "currency": "USD", "note": "f8"})
	seen := observeReads(t)
	s.run(t, "ann", drilledByMonth, Options{}, nil)
	bound := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, placeholderDay) {
			t.Fatalf("the placeholder day is written into the statement:\n%s", st.sql)
		}
		for _, v := range st.params {
			if v == placeholderDay {
				bound++
			}
		}
	}
	if bound != 2 {
		t.Fatalf("%d params hold the placeholder day, want 2 (the blank column's from and to)", bound)
	}
}
