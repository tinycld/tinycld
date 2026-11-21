package reports

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// rowsWhere finds the WHERE of w, the report's rows, in a statement.
var rowsWhere = regexp.MustCompile(`(?s)\bw AS (?:MATERIALIZED )?\(SELECT .*?\nWHERE ([^\n]*)`)

// requireRowsWhere requires that w's WHERE starts with prefix (or, with
// starts false, does not) in every statement that has the report's rows, and
// that one or more has them.
func requireRowsWhere(t *testing.T, seen []readStatement, starts bool, prefix string) {
	t.Helper()
	found := 0
	for _, st := range seen {
		m := rowsWhere.FindStringSubmatch(st.sql)
		if m == nil {
			continue
		}
		found++
		if got := strings.HasPrefix(m[1], prefix); got != starts {
			t.Fatalf("w's WHERE starts with %q: %v, want %v:\n%s", prefix, got, starts, m[1])
		}
	}
	if found == 0 {
		t.Fatal("no statement has the report's rows")
	}
}

// ledgerAccounts is an accounts param: the ledger fixture's accounts ids.
func ledgerAccounts(ids ...int) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", ledgerID("acct", id))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// The report filter names an account list and a period: w reads the lines
// through the (account, date) index, not every line.
func TestAFilterOnALinkAndADayBoundsTheBaseTable(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	seen := observeReads(t)
	if _, err := runLedger(z, params, Options{Today: "2025-07-15"}); err != nil {
		t.Fatal(err)
	}
	requireRowsWhere(t, *seen, true, "[[t1.date]] >= ")
	db := readPool(t, z)
	// byAccountAndDay counts the statements with the report's rows, and those
	// whose plan reads the lines by account and day range.
	byAccountAndDay := func() (statements, searched int) {
		for _, st := range *seen {
			if !rowsWhere.MatchString(st.sql) {
				continue
			}
			statements++
			raw, args := rawStatement(t, st.sql, st.params)
			plan := strings.Join(queryPlan(t, db, raw, args), "\n")
			if strings.Contains(plan, "SEARCH zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date>? AND date<?)") {
				searched++
			}
		}
		return statements, searched
	}
	if n, searched := byAccountAndDay(); n == 0 || searched != n {
		t.Fatalf("%d of %d statements read the report's rows by account and day, want every one", searched, n)
	}
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.noFilterBounds = true
	t.Cleanup(func() { legacySQL.noFilterBounds = false })
	*seen = nil
	if _, err := runLedger(z, params, Options{Today: "2025-07-15"}); err != nil {
		t.Fatal(err)
	}
	requireRowsWhere(t, *seen, false, "[[t1.date]] >= ")
	if n, searched := byAccountAndDay(); n == 0 || searched != 0 {
		t.Fatalf("with the switch on, %d of %d statements read the rows by account and day, want none", searched, n)
	}
}

// Every run of the ledger fixture returns the same pages with the bounds and
// without them: with and without an account list, all dates, a mixed book,
// and a reader with no grant.
func TestFilterBoundsKeepTheResult(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for _, r := range ledgerRuns() {
			opts := r.opts
			opts.Today = "2025-07-15"
			requireSameAsLegacy(t, z, "ann", balanceSpec(), opts, r.params, &legacySQL.noFilterBounds)
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15"},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.noFilterBounds)
	}
}

// withCrates adds crates that any signed-in reader lists, each for an animal
// (which has its own rule) and packed at a day and time (a PocketBase date
// column): leo's and mo's on 2026-03-05 late in the day, one with no animal
// that day, and leo's again the day after.
func withCrates(t *testing.T, z *zoo) {
	t.Helper()
	crates := core.NewBaseCollection("zoo_crates")
	crates.ListRule, crates.ViewRule = rule(`@request.auth.id != ""`), rule(`@request.auth.id != ""`)
	crates.Fields.Add(
		&core.RelationField{Name: "animal", CollectionId: z.recs["leo"].Collection().Id, MaxSelect: 1},
		&core.DateField{Name: "packed"},
		&core.TextField{Name: "tag"},
	)
	crates.AddIndex("idx_zoo_crates_animal_packed", false, "animal, packed", "")
	z.save(t, crates)
	for _, c := range []struct{ tag, animal, packed string }{
		{"leo late", z.id("leo"), "2026-03-05 23:59:59.999Z"},
		{"mo late", z.id("mo"), "2026-03-05 23:59:59.999Z"},
		{"nobody", "", "2026-03-05 12:00:00.000Z"},
		{"leo next", z.id("leo"), "2026-03-06 00:00:00.000Z"},
	} {
		z.rec(t, c.tag, "zoo_crates", map[string]any{"tag": c.tag, "animal": c.animal, "packed": c.packed})
	}
	RegisterSource(Source{
		Key: "zoo.crates", Collection: "zoo_crates", Label: "Crates",
		Fields: []Field{
			{Key: "tag", Label: "Tag", Type: Text, Aggregates: MinMax},
			{Key: "packed", Label: "Packed", Type: Date, Aggregates: MinMax},
		},
		Links: []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
	})
}

// cratesSpec lists the crates of a list of animals, packed in a day range and
// up to a day.
func cratesSpec(z *zoo) string {
	return fmt.Sprintf(`{"version":4,"source":"zoo.crates",
	"params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}},
		{"id":"pets","type":"choice","label":"Pets","multiple":true,"options":[{"value":%q,"label":"Leo"},{"value":%q,"label":"Mo"}]},
		{"id":"day","type":"date","label":"Day"},
		{"id":"since","type":"date","label":"Since"},
		{"id":"before","type":"date","label":"Before"}],
	"columns":[{"id":"tag","path":[{"field":"tag"}]},{"id":"pet","path":[{"link":"animal"}]}],
	"filter":{"op":"and","args":[
		{"op":"between","args":[{"path":[{"field":"packed"}]},{"param":"days"}]},
		{"op":"in","args":[{"col":"pet"},{"param":"pets"}]},
		{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"pets"}]},
		{"op":"lte","args":[{"path":[{"field":"packed"}]},{"param":"day"}]},
		{"op":"gte","args":[{"path":[{"field":"packed"}]},{"param":"since"}]},
		{"op":"lt","args":[{"path":[{"field":"packed"}]},{"param":"before"}]}]},
	"sort":[{"col":"tag"}]}`, z.id("leo"), z.id("mo"))
}

// A day with a time after it on the last day of a range stays in. A crate of
// an animal the reader cannot see, and a crate with no animal, pass or fail
// as the filter decides, bounded or not. An unset param bounds nothing.
func TestFilterBoundsKeepADayWithATimeAndLeaveTheFilterToDecide(t *testing.T) {
	z := newZoo(t)
	withCrates(t, z)
	spec := cratesSpec(z)
	both := fmt.Sprintf(`[%q,%q]`, z.id("leo"), z.id("mo"))
	const fifth = `{"from":"2026-03-05","to":"2026-03-05"}`
	seen := observeReads(t)
	// ann sees leo and not mo: the bound keeps mo's crate, the filter drops it.
	res := z.run(t, "ann", spec, Options{}, map[string]string{"days": fifth, "pets": both})
	// The last row of each result is the total row, which has no tag.
	requireRows(t, res, "tag", "leo late", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] >= ")
	// With no list, the crate with no animal and the hidden animal's show.
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"days": fifth}), "tag", "leo late", "mo late", "nobody", nil)
	// lte alone: up to the end of the fifth.
	requireRows(t, z.run(t, "bob", spec, Options{}, map[string]string{"day": `"2026-03-05"`, "pets": both}), "tag", "mo late", nil)
	// gte alone bounds the low side only; lt alone, the high side only.
	*seen = nil
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"since": `"2026-03-06"`}), "tag", "leo next", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] >= {:")
	*seen = nil
	requireRows(t, z.run(t, "ann", spec, Options{}, map[string]string{"before": `"2026-03-06"`}), "tag", "leo late", "mo late", "nobody", nil)
	requireRowsWhere(t, *seen, true, "[[t1.packed]] <= (")
	for _, user := range []string{"ann", "bob"} {
		for _, params := range []map[string]string{
			nil,
			{"since": `"2026-03-05"`},
			{"since": `"2026-03-06"`, "pets": both},
			{"before": `"2026-03-06"`},
			{"before": `"2026-03-05"`, "pets": both},
			{"since": `"2026-03-05"`, "before": `"2026-03-06"`},
			{"days": fifth},
			{"days": fifth, "pets": both},
			{"pets": both},
			// An empty list.
			{"pets": `[]`, "days": fifth},
			{"pets": both, "day": `"2026-03-05"`},
			{"days": `{"from":"2026-03-06","to":"2026-03-31"}`, "day": `"2026-03-06"`},
		} {
			requireSameAsLegacy(t, z, user, spec, Options{}, params, &legacySQL.noFilterBounds)
		}
	}
}

// A multi-select relation column holds a JSON array, so "column IN list" would
// drop rows the filter keeps. CheckSources refuses a to-one link over such a
// column (relationCheck: "holds many values"); this source is registered with
// no check, as withCrates does, so the bound's own guard is what is tested.
func TestFilterBoundsLeaveAMultiSelectLinkAlone(t *testing.T) {
	z := newZoo(t)
	boxes := core.NewBaseCollection("zoo_boxes")
	boxes.ListRule, boxes.ViewRule = rule(`@request.auth.id != ""`), rule(`@request.auth.id != ""`)
	boxes.Fields.Add(
		&core.RelationField{Name: "animals", CollectionId: z.recs["leo"].Collection().Id, MaxSelect: 5},
		&core.TextField{Name: "tag"},
	)
	z.save(t, boxes)
	z.rec(t, "one", "zoo_boxes", map[string]any{"tag": "one", "animals": []string{z.id("leo")}})
	z.rec(t, "two", "zoo_boxes", map[string]any{"tag": "two", "animals": []string{z.id("leo"), z.id("mo")}})
	RegisterSource(Source{
		Key: "zoo.boxes", Collection: "zoo_boxes", Label: "Boxes",
		Fields: []Field{{Key: "tag", Label: "Tag", Type: Text, Aggregates: MinMax}},
		Links:  []Link{{Key: "animals", Label: "Animals", Target: "zoo.animals", Cardinality: ToOne, Field: "animals"}},
	})
	spec := fmt.Sprintf(`{"version":4,"source":"zoo.boxes",
		"params":[{"id":"pets","type":"choice","label":"Pets","multiple":true,"options":[{"value":%q,"label":"Leo"}]}],
		"columns":[{"id":"tag","path":[{"field":"tag"}]},{"id":"pet","path":[{"link":"animals"}]}],
		"filter":{"op":"in","args":[{"path":[{"link":"animals"}]},{"param":"pets"}]},
		"sort":[{"col":"tag"}]}`, z.id("leo"))
	params := map[string]string{"pets": fmt.Sprintf(`[%q]`, z.id("leo"))}
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{}, params)
	requireRowsWhere(t, *seen, false, "[[t1.animals]] IN ")
	requireSameAsLegacy(t, z, "ann", spec, Options{}, params, &legacySQL.noFilterBounds)
}

// A dry compile has no database and no reader: it writes no bound and reads
// no statement. A bound would read the collection through the app, which a
// bare app has no database for.
func TestADryCompileOfAFilteredSpecWritesNoBound(t *testing.T) {
	z := newZoo(t)
	withCrates(t, z)
	app := bareApp(t)
	seen := observeReads(t)
	spec, _, err := Load([]byte(cratesSpec(z)))
	if err != nil {
		t.Fatal(err)
	}
	p, se := bind(app, spec)
	if se != nil {
		t.Fatal(se)
	}
	c := newCompiler(p, nil, nil, nil)
	if !c.dry() {
		t.Fatal("a compiler with no app and no reader is not dry")
	}
	r, err := c.rows(newStmt(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if r.bounds != "" {
		t.Fatalf("a dry compile wrote the bound %q", r.bounds)
	}
	if err := Validate(app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(*seen) != 0 {
		t.Fatalf("a dry compile read the database: %v", *seen)
	}
}

// A union has no one stored column to bound in w's WHERE, which stays the
// filter alone: its bounds are inside the parts (partbounds_test.go).
func TestFilterBoundsStayOutOfAUnionsWhere(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
		"columns":[{"id":"type","path":[{"field":"kind"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},"sort":[{"col":"type"}]}`
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{}, map[string]string{"days": `{"from":"2026-01-01","to":"2026-12-31"}`})
	requireRowsWhere(t, *seen, true, "(substr(")
}
