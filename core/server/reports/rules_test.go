package reports

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ruleIDs runs the rule SELECT as a CTE, as the compiler uses it.
func ruleIDs(t *testing.T, z *zoo, collection, user string) []string {
	t.Helper()
	col, err := z.app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	sql, params, err := ruleSelect(z.app, col, z.as(user))
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	if err := z.app.DB().NewQuery("WITH r AS (" + sql + ") SELECT id FROM r ORDER BY id").Bind(params).Column(&ids); err != nil {
		t.Fatalf("%s: %v", collection, err)
	}
	return ids
}

func TestRuleSelectMatchesTheListAPI(t *testing.T) {
	z := newZoo(t)
	mux := z.mux(t)
	// zoo_animals has a back-relation rule and zoo_feedings an @collection
	// rule: the resolver joins for both, and the joins stay inside the CTE.
	for _, user := range []string{"ann", "bob"} {
		for _, collection := range []string{"users", "zoo_sites", "zoo_animals", "zoo_keepers", "zoo_feedings"} {
			got := ruleIDs(t, z, collection, user)
			want := z.listIDs(t, mux, collection, user, "")
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s as %s: rule CTE %v, list API %v", collection, user, got, want)
			}
		}
	}
	if got, want := ruleIDs(t, z, "zoo_feedings", "ann"), z.ids("f1", "f2", "f3", "f4"); !reflect.DeepEqual(got, want) {
		t.Errorf("ann's feedings = %v, want %v", got, want)
	}
	if got, want := ruleIDs(t, z, "zoo_animals", "bob"), z.ids("zed", "mo"); !reflect.DeepEqual(got, want) {
		t.Errorf("bob's animals = %v, want %v", got, want)
	}
}

func TestRuleSelectReadsTheReadersOwnAuthFields(t *testing.T) {
	z := newZoo(t)
	if got, want := ruleIDs(t, z, "zoo_sites", "ann"), z.ids("north", "south"); !reflect.DeepEqual(got, want) {
		t.Fatalf("a member sees %v, want %v", got, want)
	}
	// The resolver joins the auth record's stored row, so the role must be saved.
	z.recs["ann"].Set("role", "guest")
	if err := z.app.Save(z.recs["ann"]); err != nil {
		t.Fatal(err)
	}
	if got := ruleIDs(t, z, "zoo_sites", "ann"); len(got) != 0 {
		t.Fatalf("a guest sees %v, want none", got)
	}
}

func TestMergeParamsRefusesAConflict(t *testing.T) {
	dst := dbx.Params{"p0": "a"}
	if err := mergeParams(dst, dbx.Params{"p0": "a", "p1": 2}); err != nil {
		t.Fatalf("same value: %v", err)
	}
	if dst["p1"] != 2 {
		t.Fatalf("p1 not merged: %v", dst)
	}
	err := mergeParams(dst, dbx.Params{"p0": "b"})
	if err == nil || !strings.Contains(err.Error(), `"p0"`) {
		t.Fatalf("conflict gave %v", err)
	}
}

func TestRuleSelectOfANullRuleIsEmpty(t *testing.T) {
	z := newZoo(t)
	if got := ruleIDs(t, z, "zoo_vault", "ann"); len(got) != 0 {
		t.Fatalf("superuser-only collection gave a reader %v", got)
	}
}

func TestTwoRuleCTEsShareOneStatement(t *testing.T) {
	z := newZoo(t)
	feedings, _ := z.app.FindCollectionByNameOrId("zoo_feedings")
	animals, _ := z.app.FindCollectionByNameOrId("zoo_animals")
	fSQL, fParams, err := ruleSelect(z.app, feedings, z.as("bob"))
	if err != nil {
		t.Fatal(err)
	}
	aSQL, aParams, err := ruleSelect(z.app, animals, z.as("bob"))
	if err != nil {
		t.Fatal(err)
	}
	params := dbx.Params{}
	for _, p := range []dbx.Params{fParams, aParams} {
		if err := mergeParams(params, p); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	err = z.app.DB().NewQuery("WITH r_f AS (" + fSQL + "), r_a AS (" + aSQL + `)
		SELECT a.name FROM r_f f JOIN r_a a ON a.id = f.animal ORDER BY f.cost`).Bind(params).Column(&names)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Mo", "Zed", "Zed"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

// ruleSQL is the rule select of collection as user, as ruleSelect writes it.
func ruleSQL(t *testing.T, z *zoo, collection, user string) string {
	t.Helper()
	col, err := z.app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := ruleSelect(z.app, col, z.as(user))
	if err != nil {
		t.Fatal(err)
	}
	return sql
}

// grantRule is the list rule withBalanceZoo gives its ledger collections: a
// grant row of the reader in another collection, and no column of the row.
const grantRule = `@request.auth.id != "" && @collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true`

// A joined rule that names only the request and other collections (the
// grant rule; the reader's own role) gives one answer for every row: it is
// one EXISTS filter, with no DISTINCT and no id list. The grant rule's
// constant true is no name of the row.
func TestARuleThatReadsNoRowIsOneFilter(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	for _, collection := range []string{"zoo_ledger_entries", "zoo_sites"} {
		sql := ruleSQL(t, z, collection, "ann")
		if !strings.Contains(sql, "WHERE EXISTS (") {
			t.Errorf("%s: the rule is not one EXISTS filter:\n%s", collection, sql)
		}
		if strings.Contains(sql, "DISTINCT") {
			t.Errorf("%s: the rule is still DISTINCT:\n%s", collection, sql)
		}
		if strings.Contains(sql, " IN (SELECT") {
			t.Errorf("%s: the rule is still an id list:\n%s", collection, sql)
		}
	}
}

// A rule that names a column of its row keeps the id list when it joins, and
// stays in place when it does not join; a rule that reads no row and joins
// nothing is not wrapped either.
func TestARuleThatReadsItsRowKeepsItsForm(t *testing.T) {
	z := newZoo(t)
	gate := core.NewBaseCollection("zoo_gate")
	gate.ListRule = rule(`@request.auth.id != ""`)
	gate.Fields.Add(&core.TextField{Name: "name"})
	z.save(t, gate)
	// zoo_animals reads its owner and a back-relation; zoo_feedings compares
	// an @collection field with its own animal, the last name of a comparison.
	for _, collection := range []string{"zoo_animals", "zoo_feedings"} {
		sql := ruleSQL(t, z, collection, "ann")
		if !strings.Contains(sql, "DISTINCT") || !strings.Contains(sql, " IN (SELECT") {
			t.Errorf("%s: a joined rule that reads its row is not an id list:\n%s", collection, sql)
		}
		if strings.Contains(sql, "EXISTS (SELECT (1)") {
			t.Errorf("%s: a rule that reads its row is written as one filter:\n%s", collection, sql)
		}
	}
	for _, collection := range []string{"zoo_keepers", "zoo_gate"} {
		sql := ruleSQL(t, z, collection, "ann")
		for _, form := range []string{"DISTINCT", " IN (SELECT", "EXISTS"} {
			if strings.Contains(sql, form) {
				t.Errorf("%s: a rule with no join holds %q:\n%s", collection, form, sql)
			}
		}
	}
}

// Every form gives the rows the list API gives: a reader with two grant rows
// sees each row once, a reader whose grant cannot read sees none, and a view
// and a union part over the view read the same rows.
func TestAGrantRuleMatchesTheListAPI(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	z.rec(t, "annLedger2", "zoo_ledger_access", map[string]any{"user": z.id("ann"), "can_read": true})
	view := core.NewViewCollection("zoo_entry_view")
	view.ListRule, view.ViewRule = rule(grantRule), rule(grantRule)
	view.ViewQuery = `SELECT id, number FROM zoo_ledger_entries`
	z.save(t, view)
	RegisterSource(Source{
		Key: "zoo.entries_twice", Label: "Entries twice",
		Parts: []SourcePart{
			{Collection: "zoo_ledger_entries", Columns: map[string]string{"number": "number"}},
			{Collection: "zoo_entry_view", Columns: map[string]string{"number": "number"}},
		},
		Fields: []Field{{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	union := lookupSource(z.app, "zoo.entries_twice")
	if union == nil {
		t.Fatalf("the union is refused: %v", Refused(z.app)["zoo.entries_twice"])
	}
	mux := z.mux(t)
	ledger := []string{"zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines"}
	for _, user := range []string{"ann", "bob"} {
		for _, collection := range append(slices.Clone(ledger), "zoo_entry_view") {
			got := ruleIDs(t, z, collection, user)
			want := z.listIDs(t, mux, collection, user, "")
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s as %s: rule CTE %v, list API %v", collection, user, got, want)
			}
		}
		sql, params, err := unionSelect(z.app, union, z.as(user))
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		if err := z.app.DB().NewQuery("SELECT id FROM (" + sql + ") ORDER BY id").Bind(params).Column(&got); err != nil {
			t.Fatalf("union as %s: %v", user, err)
		}
		want := append(z.listIDs(t, mux, "zoo_ledger_entries", user, ""), z.listIDs(t, mux, "zoo_entry_view", user, "")...)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("union as %s: %v, list API %v", user, got, want)
		}
	}
	entries := ruleIDs(t, z, "zoo_ledger_entries", "ann")
	if want := z.ids("J0", "J1", "J2", "J3"); !reflect.DeepEqual(entries, want) {
		t.Errorf("ann's entries through two grant rows = %v, want each of %v once", entries, want)
	}
	for _, collection := range ledger {
		if got := ruleIDs(t, z, collection, "bob"); len(got) != 0 {
			t.Errorf("bob, whose grant cannot read, sees %s %v", collection, got)
		}
	}
	for _, key := range []string{"annLedger", "annLedger2"} {
		if err := z.app.Delete(z.recs[key]); err != nil {
			t.Fatal(err)
		}
	}
	for _, collection := range ledger {
		got := ruleIDs(t, z, collection, "ann")
		if len(got) != 0 {
			t.Errorf("ann with no grant sees %s %v", collection, got)
		}
		if want := z.listIDs(t, mux, collection, "ann", ""); !reflect.DeepEqual(got, want) {
			t.Errorf("%s as ann with no grant: rule CTE %v, list API %v", collection, got, want)
		}
	}
}

// A comparison of an @collection field with no "?" holds for every joined
// row: PocketBase reads it through a subquery on the row's id, which the
// one-filter form keeps.
func TestARuleOverEveryJoinedRowMatchesTheListAPI(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	notes := core.NewBaseCollection("zoo_ledger_notes")
	notes.ListRule = rule(`@request.auth.id != "" && @collection.zoo_ledger_access.user = @request.auth.id`)
	notes.ViewRule = notes.ListRule
	notes.Fields.Add(&core.TextField{Name: "note"})
	z.save(t, notes)
	z.rec(t, "n1", "zoo_ledger_notes", map[string]any{"note": "one"})
	sql := ruleSQL(t, z, "zoo_ledger_notes", "ann")
	if !strings.Contains(sql, "WHERE EXISTS (") || !strings.Contains(sql, "__mm_") {
		t.Errorf("the every-row rule is not one filter with its subquery:\n%s", sql)
	}
	mux := z.mux(t)
	match := func(stage string, want map[string][]string) {
		t.Helper()
		for _, user := range []string{"ann", "bob"} {
			got := ruleIDs(t, z, "zoo_ledger_notes", user)
			if api := z.listIDs(t, mux, "zoo_ledger_notes", user, ""); !reflect.DeepEqual(got, api) {
				t.Errorf("%s, as %s: rule CTE %v, list API %v", stage, user, got, api)
			}
			if !reflect.DeepEqual(got, want[user]) {
				t.Errorf("%s, as %s: rule CTE %v, want %v", stage, user, got, want[user])
			}
		}
	}
	// Ann's and bob's grant rows: no user is on every row.
	match("two grant rows", map[string][]string{"ann": {}, "bob": {}})
	if err := z.app.Delete(z.recs["bobLedger"]); err != nil {
		t.Fatal(err)
	}
	match("ann's grant row alone", map[string][]string{"ann": z.ids("n1"), "bob": {}})
}

// entryLinkSpec lists the ledger lines with the number of each line's entry,
// a link read through the entries' rule CTE.
const entryLinkSpec = `{"version":4,"source":"zoo.ledger_lines","columns":[{"id":"pos","path":[{"field":"position"}]},{"id":"entry","path":[{"link":"entry"},{"field":"number"}]}],"sort":[{"col":"pos"}]}`

// planSteps is a plan's steps trimmed of the indent under their parents.
func planSteps(plan []string) []string {
	out := make([]string, 0, len(plan))
	for _, step := range plan {
		out = append(out, strings.TrimSpace(step))
	}
	return out
}

// A link to a collection whose rule reads no row finds the linked row by its
// key: the rule never reads the collection whole.
func TestALinkToARuleThatReadsNoRowSearchesByKey(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	db := readPool(t, z)
	entries, err := z.app.FindCollectionByNameOrId("zoo_ledger_entries")
	if err != nil {
		t.Fatal(err)
	}
	sql, ruleParams, err := ruleSelect(z.app, entries, z.as("ann"))
	if err != nil {
		t.Fatal(err)
	}
	params := dbx.Params{"probe_id": z.id("J0")}
	if err := mergeParams(params, ruleParams); err != nil {
		t.Fatal(err)
	}
	raw, args := rawStatement(t, "WITH r AS ("+sql+") SELECT number FROM r WHERE id = {:probe_id}", params)
	plan := queryPlan(t, db, raw, args)
	byKey := false
	for _, step := range planSteps(plan) {
		if strings.HasPrefix(step, "SCAN zoo_ledger_entries") {
			t.Errorf("a read by id scans the entries by %q:\n%s", step, strings.Join(plan, "\n"))
		}
		if strings.HasPrefix(step, "SEARCH zoo_ledger_entries USING INDEX sqlite_autoindex_zoo_ledger_entries_1 (id=?)") {
			byKey = true
		}
	}
	if !byKey {
		t.Errorf("a read by id does not search the entries by their key:\n%s", strings.Join(plan, "\n"))
	}

	seen := observeReads(t)
	res := z.run(t, "ann", entryLinkSpec, Options{}, nil)
	j0 := false
	for _, r := range res.Rows {
		if r.Kind == KindDetail && r.Cells["entry"] == "J0" {
			j0 = true
		}
	}
	if !j0 {
		t.Errorf("no detail row reads entry J0: %+v", res.Rows)
	}
	checked := 0
	for _, st := range *seen {
		raw, args := rawStatement(t, st.sql, st.params)
		readsEntries := false
		for _, m := range ruleCTE.FindAllStringSubmatch(raw, -1) {
			if m[2] == "zoo_ledger_entries" {
				readsEntries = true
			}
		}
		if !readsEntries {
			continue
		}
		checked++
		plan := queryPlan(t, db, raw, args)
		searched := false
		for _, step := range planSteps(plan) {
			if strings.HasPrefix(step, "SCAN zoo_ledger_entries") {
				t.Errorf("the run scans the entries by %q:\n%s\n%s", step, raw, strings.Join(plan, "\n"))
			}
			if strings.HasPrefix(step, "SEARCH zoo_ledger_entries USING") {
				searched = true
			}
		}
		if !searched {
			t.Errorf("the run never searches the entries:\n%s\n%s", raw, strings.Join(plan, "\n"))
		}
	}
	if checked == 0 {
		t.Fatal("no statement of the run holds the entries' rule CTE: the plan check checked nothing")
	}
}

// A run finds its change topics from the rule selects it reads, the grant
// collection's too; a reader whose grant cannot read gets no row.
func TestARunThroughAGrantRuleKeepsItsTopics(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := z.run(t, "ann", entryLinkSpec, Options{}, nil)
	for _, topic := range []string{"collection/zoo_ledger_access", "collection/zoo_ledger_entries", "collection/zoo_ledger_lines"} {
		if !slices.Contains(res.Topics, topic) {
			t.Errorf("topics %v lack %s", res.Topics, topic)
		}
	}
	for _, r := range z.run(t, "bob", entryLinkSpec, Options{}, nil).Rows {
		if r.Kind == KindDetail {
			t.Errorf("bob, whose grant cannot read, gets the detail row %+v", r)
		}
	}
}
