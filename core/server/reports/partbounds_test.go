package reports

import (
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// withMealIndexes gives each part of zoo.meals one index a bound inside the
// part reads through: on its animal link (kind "link") or on its day (kind
// "day"). One index a part, so the plan does not depend on which of two
// SQLite's planner prefers on a small table.
func withMealIndexes(t *testing.T, z *zoo, kind string) {
	t.Helper()
	columns := map[string]map[string]string{
		"link": {"zoo_feedings": "animal", "zoo_treats": "pet"},
		"day":  {"zoo_feedings": "fed", "zoo_treats": "given"},
	}
	for name, column := range columns[kind] {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		col.AddIndex("idx_"+name+"_"+kind, false, column, "")
		z.save(t, col)
	}
}

// unionPlans is the query plan of every statement a run read, one text.
func unionPlans(t *testing.T, z *zoo, seen []readStatement) string {
	t.Helper()
	db := readPool(t, z)
	var plans []string
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		plans = append(plans, queryPlan(t, db, raw, args)...)
	}
	return strings.Join(plans, "\n")
}

const mealsLookupSpec = `{"version":4,"source":"zoo.pets","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"pet","path":[{"field":"id"}]},
	{"id":"meals","formula":{"op":"lookup","source":"zoo.meals","pick":"sum",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
			{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]}]},
		"value":{"path":[{"field":"cost"}]}}}],
	"sort":[{"col":"name"}]}`

const mealsFilterSpec = `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[{"id":"type","path":[{"field":"kind"}]},{"id":"cost","path":[{"field":"cost"}]}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},"sort":[{"col":"type"}]}`

var mealDays = map[string]string{"days": `{"from":"2026-01-01","to":"2026-02-28"}`}

// A lookup over a union that matches a link reads each part through the index
// on the part's own link column, not every row of the union for each row of
// the report.
func TestALookupOverAUnionSearchesEachPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	seen := observeReads(t)
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	plan := unionPlans(t, z, *seen)
	for _, want := range []string{
		"SEARCH zoo_feedings USING INDEX idx_zoo_feedings_link (animal>? AND animal<?)",
		"SEARCH zoo_treats USING INDEX idx_zoo_treats_link (pet>? AND pet<?)",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("the plan has no %q:\n%s", want, plan)
		}
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, partLink) || strings.Contains(st.sql, partDay) {
			t.Fatalf("a statement keeps a part alias:\n%s", st.sql)
		}
	}
	legacySQL.noPartBounds = true
	t.Cleanup(func() { legacySQL.noPartBounds = false })
	*seen = nil
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	if plan := unionPlans(t, z, *seen); strings.Contains(plan, "idx_zoo_feedings_link") {
		t.Fatalf("with the switch on, the plan still reads a part by its link:\n%s", plan)
	}
}

// A filter on a union's day reads each part through the index on the part's
// own day column: zoo_feedings.fed and zoo_treats.given.
func TestAFilterOnAUnionSearchesEachPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "day")
	seen := observeReads(t)
	z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	plan := unionPlans(t, z, *seen)
	for _, want := range []string{
		"SEARCH zoo_feedings USING INDEX idx_zoo_feedings_day (fed>? AND fed<?)",
		"SEARCH zoo_treats USING INDEX idx_zoo_treats_day (given>? AND given<?)",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("the plan has no %q:\n%s", want, plan)
		}
	}
	// w's WHERE is still the filter alone: the bounds are inside the parts.
	requireRowsWhere(t, *seen, true, "(substr(")
}

// Every reader gets the same rows with the part bounds and without them: a
// reader of both parts, of one part, and of none; with a period and with all
// dates (no day bound).
func TestPartBoundsKeepTheResult(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, params := range []map[string]string{mealDays, nil} {
			requireSameAsLegacy(t, z, user, mealsLookupSpec, Options{}, params, &legacySQL.noPartBounds)
			requireSameAsLegacy(t, z, user, mealsFilterSpec, Options{}, params, &legacySQL.noPartBounds)
		}
	}
}

// A part holds a bound only when its collection stores every key the bound
// names: a link as a single relation, a day as text or a date. A part with no
// column for a key, or a column of another type (a multi-select relation
// holds a JSON array, never an id), reads its rows without that bound; a part
// that is a view (its column may hold NULL) or an unknown collection reads
// its rows without any.
func TestAPartHoldsOnlyTheBoundsItStores(t *testing.T) {
	z := newUnionZoo(t)
	signedIn := rule(`@request.auth.id != ""`)
	boxes := core.NewBaseCollection("zoo_boxes")
	boxes.ListRule, boxes.ViewRule = signedIn, signedIn
	boxes.Fields.Add(
		&core.RelationField{Name: "animals", CollectionId: z.recs["leo"].Collection().Id, MaxSelect: 5},
		&core.DateField{Name: "packed"},
	)
	z.save(t, boxes)
	view := core.NewViewCollection("zoo_treat_days")
	view.ListRule, view.ViewRule = signedIn, signedIn
	view.ViewQuery = `SELECT t.id, t.pet, t.given FROM zoo_treats t`
	z.save(t, view)
	link := q(partLink, "animal") + " >= 'a'"
	day := q(partDay, "fed") + " >= 'b'"
	part := func(collection string, columns map[string]string) SourcePart {
		return SourcePart{Collection: collection, Columns: columns}
	}
	for _, c := range []struct {
		name   string
		part   SourcePart
		bounds []string
		want   string
	}{
		{"both keys stored", part("zoo_treats", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, "[[p.pet]] >= 'a' AND [[p.given]] >= 'b'"},
		{"no column for the day", part("zoo_treats", map[string]string{"animal": "pet"}), []string{link, day}, "[[p.pet]] >= 'a'"},
		{"a day in a number column", part("zoo_treats", map[string]string{"animal": "pet", "fed": "price"}), []string{link, day}, "[[p.pet]] >= 'a'"},
		{"a link in a text column", part("zoo_treats", map[string]string{"animal": "label", "fed": "given"}), []string{link, day}, "[[p.given]] >= 'b'"},
		{"a link in a multi-select relation", part("zoo_boxes", map[string]string{"animal": "animals", "fed": "packed"}), []string{link, day}, "[[p.packed]] >= 'b'"},
		{"a view", part("zoo_treat_days", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, ""},
		{"one bound of two keys, one not stored", part("zoo_treats", map[string]string{"animal": "pet"}), []string{link + " AND " + day}, ""},
		{"an unknown collection", part("zoo_nothing", map[string]string{"animal": "pet", "fed": "given"}), []string{link, day}, ""},
	} {
		if got := partWhere(z.app, c.part, c.bounds); got != c.want {
			t.Errorf("%s: the part holds %q, want %q", c.name, got, c.want)
		}
	}
}

// An eq under or, or under not, holds for rows outside its range: it bounds
// no part, and the rows are the rows with no bound.
func TestAnEqUnderOrOrNotBoundsNoPart(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "link")
	eq := `{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]}`
	days := `{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]}`
	partBound := regexp.MustCompile(`FROM r\d+ p WHERE `)
	for name, where := range map[string]string{
		"or":  `{"op":"or","args":[` + eq + `,` + days + `]}`,
		"not": `{"op":"not","args":[` + eq + `]}`,
	} {
		spec := strings.Replace(mealsLookupSpec, `{"op":"and","args":[`+"\n\t\t\t"+eq+`,`+"\n\t\t\t"+days+`]}`, where, 1)
		if spec == mealsLookupSpec {
			t.Fatalf("%s: the where was not replaced", name)
		}
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{}, mealDays)
		for _, st := range *seen {
			if partBound.MatchString(st.sql) {
				t.Fatalf("%s: a part is bounded:\n%s", name, st.sql)
			}
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, mealDays, &legacySQL.noPartBounds)
	}
	// The same test finds the bound of the plain and.
	seen := observeReads(t)
	z.run(t, "ann", mealsLookupSpec, Options{}, mealDays)
	bounded := false
	for _, st := range *seen {
		bounded = bounded || partBound.MatchString(st.sql)
	}
	if !bounded {
		t.Fatal("the and of the eq and the days bounds no part")
	}
}

// The topics of a run that reads a union through its parts name each part's
// collection, as a read of the union's rule CTE does.
func TestPartBoundsKeepTheTopics(t *testing.T) {
	z := newUnionZoo(t)
	withMealIndexes(t, z, "day")
	bounded := z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	legacySQL.noPartBounds = true
	t.Cleanup(func() { legacySQL.noPartBounds = false })
	whole := z.run(t, "ann", mealsFilterSpec, Options{}, mealDays)
	if got, want := strings.Join(bounded.Topics, ","), strings.Join(whole.Topics, ","); got != want {
		t.Fatalf("topics with part bounds %q, without %q", got, want)
	}
	if !strings.Contains(strings.Join(bounded.Topics, ","), "zoo_treats") {
		t.Fatalf("the topics name no part: %v", bounded.Topics)
	}
}
