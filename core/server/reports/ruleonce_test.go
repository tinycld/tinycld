package reports

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// ruleProbe is one probe a run asked, what it was asked through, and its
// answer.
type ruleProbe struct {
	db     dbx.Builder
	sql    string
	params dbx.Params
	ok     bool
}

// observeProbes collects every rule probe until the test ends.
func observeProbes(t *testing.T) *[]ruleProbe {
	t.Helper()
	var seen []ruleProbe
	saved := ruleAnswer
	ruleAnswer = func(ctx context.Context, db dbx.Builder, probe string, params dbx.Params) (bool, error) {
		ok, err := saved(ctx, db, probe, params)
		seen = append(seen, ruleProbe{db: db, sql: probe, params: params, ok: ok})
		return ok, err
	}
	t.Cleanup(func() { ruleAnswer = saved })
	return &seen
}

// sentRule is the rule CTE of collection as a statement of user holds it
// (stmt.with, with the rules asked as a run asks them), and the ids it reads.
func sentRule(t *testing.T, z *zoo, collection, user string) (string, []string) {
	t.Helper()
	s := newStmt(z.app, z.as(user))
	s.ask = askRules(context.Background(), z.app.DB(), nil)
	name := s.rule(collection)
	with, err := s.with()
	if err != nil {
		t.Fatalf("%s as %s: %v", collection, user, err)
	}
	ids := []string{}
	if err := z.app.DB().NewQuery(with + "SELECT id FROM " + name + " ORDER BY id").Bind(s.params).Column(&ids); err != nil {
		t.Fatalf("%s as %s: %v\n%s", collection, user, err, with)
	}
	return with, ids
}

// ruleKindsZoo adds a collection for each kind of rule readsNoRow accepts
// that the zoo does not hold yet, each with one row, and the user cara, who
// has no grant row. The kinds: a grant in another collection (the ledger
// collections), a field of the reader's own user row (zoo_sites), both in one
// rule (zoo_kind_both), the request alone (zoo_kind_request, zoo_kind_nobody),
// a request header (zoo_kind_header), a comparison with every joined row
// (zoo_kind_every), which reads its row's id, a rule that joins its own
// collection (zoo_kind_self), and "@request.body.x:changed",
// which reads its row's column, alone (zoo_kind_changed) and beside a grant
// (zoo_kind_changed_grant).
func ruleKindsZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	withBalanceZoo(t, z)
	z.user(t, "cara")
	for name, r := range map[string]string{
		"zoo_kind_both":    `@request.auth.role = "member" && @collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true`,
		"zoo_kind_request": `@request.auth.id != ""`,
		"zoo_kind_nobody":  `@request.auth.id = "nobody"`,
		"zoo_kind_header":  `@request.headers.x_zoo = "open"`,
		"zoo_kind_every":   `@request.auth.id != "" && @collection.zoo_ledger_access.user = @request.auth.id`,
		"zoo_kind_self":    `@request.auth.id != "" && @collection.zoo_kind_self:other.note ?= "one"`,
		"zoo_kind_changed": `@request.auth.id != "" && @request.body.note:changed = false`,
		"zoo_kind_changed_grant": `@collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true` +
			` && @request.body.note:changed = false`,
	} {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = rule(r), rule(r)
		col.Fields.Add(&core.TextField{Name: "note"})
		z.save(t, col)
		z.rec(t, name, name, map[string]any{"note": "one"})
	}
	return z
}

// answeredKinds are the collections of ruleKindsZoo whose rule one answer
// serves; keptKinds are those whose rule stays in the statement.
var (
	answeredKinds = []string{"zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines", "zoo_sites", "zoo_kind_both"}
	keptKinds     = []string{
		"zoo_kind_request", "zoo_kind_nobody", "zoo_kind_header", "zoo_kind_every", "zoo_kind_self", "zoo_kind_changed", "zoo_kind_changed_grant",
		"zoo_keepers", "zoo_animals", "zoo_feedings", "users", "zoo_vault",
	}
)

// For every kind of rule readsNoRow accepts, and for the rules that read
// their row, a statement reads the rows the list API gives each reader and
// the rows the rule in full gives: a reader the rule refuses reads none.
func TestEveryRuleKindReadsTheListAPIsRows(t *testing.T) {
	z := ruleKindsZoo(t)
	mux := z.mux(t)
	// bob's grant cannot read; cara has no grant row; ann is made a guest for
	// the second pass, which the role rules refuse.
	check := func(stage string) {
		t.Helper()
		for _, user := range []string{"ann", "bob", "cara"} {
			for _, collection := range append(slices.Clone(answeredKinds), keptKinds...) {
				with, got := sentRule(t, z, collection, user)
				// The list API answers 403 for a null rule, where a report
				// reads no row.
				if collection == "zoo_vault" {
					if len(got) != 0 {
						t.Errorf("%s, as %s: a superuser-only collection gives %v", stage, user, got)
					}
					continue
				}
				if api := z.listIDs(t, mux, collection, user, ""); !reflect.DeepEqual(got, api) {
					t.Errorf("%s, %s as %s: the statement reads %v, the list API gives %v\n%s", stage, collection, user, got, api, with)
				}
				if full := ruleIDs(t, z, collection, user); !reflect.DeepEqual(got, full) {
					t.Errorf("%s, %s as %s: the statement reads %v, the rule in full gives %v", stage, collection, user, got, full)
				}
			}
		}
	}
	check("as built")
	ungranted := []string{
		"zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines", "zoo_kind_both", "zoo_kind_nobody", "zoo_kind_header",
		"zoo_kind_every", "zoo_kind_changed_grant",
	}
	refused := map[string][]string{"bob": ungranted, "cara": ungranted}
	for user, collections := range refused {
		for _, collection := range collections {
			if with, got := sentRule(t, z, collection, user); len(got) != 0 {
				t.Errorf("%s, whom the rule of %s refuses, reads %v\n%s", user, collection, got, with)
			}
		}
	}
	for _, collection := range []string{"zoo_ledger_lines", "zoo_sites", "zoo_kind_both", "zoo_kind_request", "zoo_kind_changed", "zoo_kind_changed_grant"} {
		if _, got := sentRule(t, z, collection, "ann"); len(got) == 0 {
			t.Errorf("ann reads no row of %s: the check proves nothing", collection)
		}
	}
	z.recs["ann"].Set("role", "guest")
	if err := z.app.Save(z.recs["ann"]); err != nil {
		t.Fatal(err)
	}
	check("ann a guest")
	for _, collection := range []string{"zoo_sites", "zoo_kind_both"} {
		if with, got := sentRule(t, z, collection, "ann"); len(got) != 0 {
			t.Errorf("ann, a guest, reads %s %v\n%s", collection, got, with)
		}
	}
}

// plainRule is a rule CTE that holds an answer: every row of its table, or
// none.
var plainRule = regexp.MustCompile("^WITH r0 AS \\(SELECT `(\\w+)`\\.\\* FROM `(\\w+)`( WHERE 0)?\\)\n$")

// A rule one answer serves leaves the statement: its CTE reads every row or
// no row, and the probe that answered it binds every value of the rule.
func TestARuleOneAnswerServesLeavesTheStatement(t *testing.T) {
	z := ruleKindsZoo(t)
	for _, collection := range answeredKinds {
		for user, passes := range map[string]bool{"ann": true, "bob": false} {
			if collection == "zoo_sites" {
				passes = true // both are members
			}
			probes := observeProbes(t)
			with, _ := sentRule(t, z, collection, user)
			m := plainRule.FindStringSubmatch(with)
			if m == nil || m[1] != collection || m[2] != collection {
				t.Fatalf("%s as %s: the rule CTE is not the table's rows:\n%s", collection, user, with)
			}
			if refused := m[3] != ""; refused == passes {
				t.Errorf("%s as %s: refused = %v, want %v:\n%s", collection, user, refused, !passes, with)
			}
			if len(*probes) != 1 || (*probes)[0].ok != passes {
				t.Fatalf("%s as %s: probes %+v, want one that answers %v", collection, user, *probes, passes)
			}
			probe := (*probes)[0]
			if !strings.HasPrefix(probe.sql, "SELECT EXISTS (SELECT (1) FROM (SELECT 1) `__rule` ") {
				t.Errorf("%s: the probe is not the rule on a one-row table:\n%s", collection, probe.sql)
			}
			if strings.Contains(probe.sql, z.id(user)) {
				t.Errorf("%s: the probe holds the reader's id in its text:\n%s", collection, probe.sql)
			}
			bound := false
			for _, v := range probe.params {
				bound = bound || v == z.id(user)
			}
			if !bound {
				t.Errorf("%s: the probe does not bind the reader's id: %v", collection, probe.params)
			}
		}
	}
}

// A rule that reads its row, a rule that joins nothing, and the comparison
// with every joined row (which reads the row's id) are never asked outside
// the statement: the statement holds the rule ruleSelect writes.
func TestARuleThatReadsItsRowStaysInTheStatement(t *testing.T) {
	z := ruleKindsZoo(t)
	names := regexp.MustCompile(`\{:\w+\}|\{\{__sm\w+\}\}|\[\[__sm\w+\.`)
	for _, collection := range keptKinds {
		probes := observeProbes(t)
		with, _ := sentRule(t, z, collection, "ann")
		if len(*probes) != 0 {
			t.Errorf("%s: the rule was asked outside the statement: %+v", collection, *probes)
		}
		// PocketBase names the placeholders anew at each build.
		want := "WITH r0 AS (" + ruleSQL(t, z, collection, "ann") + ")\n"
		if got := names.ReplaceAllString(with, "?"); got != names.ReplaceAllString(want, "?") {
			t.Errorf("%s: the statement holds\n%s\nwant the rule in full\n%s", collection, with, want)
		}
	}
	for collection, form := range map[string]string{
		"zoo_animals": " IN (SELECT", "zoo_feedings": " IN (SELECT", "zoo_kind_every": "WHERE EXISTS (", "zoo_kind_changed_grant": "WHERE EXISTS (",
		"zoo_keepers": "WHERE ", "zoo_vault": "1=0",
	} {
		if with, _ := sentRule(t, z, collection, "ann"); !strings.Contains(with, form) {
			t.Errorf("%s: the statement lost %q:\n%s", collection, form, with)
		}
	}
	// Through a run: zoo_feedings reads its row's animal.
	probes := observeProbes(t)
	seen := observeReads(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}]}`, Options{}, nil)
	if n := detailRows(res); n != 4 {
		t.Fatalf("ann reads %d feedings, want 4", n)
	}
	if len(*probes) != 0 {
		t.Errorf("the run asked a rule that reads its row: %+v", *probes)
	}
	held := false
	for _, st := range *seen {
		held = held || strings.Contains(st.sql, "FROM `zoo_feedings` WHERE +`zoo_feedings`.`id` IN (SELECT")
	}
	if !held {
		t.Error("no statement of the run holds the feedings' rule")
	}
}

func detailRows(res *Result) int {
	n := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			n++
		}
	}
	return n
}

// A run asks each distinct rule one time, for its own reader, and keeps no
// answer for the next run: a grant taken away or given shows in the next run.
func TestARuleAnswerBelongsToOneRunAndOneReader(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	probes := observeProbes(t)
	runAs := func(user string) int {
		t.Helper()
		seen := observeReads(t)
		n := detailRows(z.run(t, user, entryLinkSpec, Options{}, nil))
		if len(*seen) == 0 {
			t.Fatal("the run sent no statement")
		}
		for _, st := range *seen {
			if strings.Contains(st.sql, "__rule") || strings.Contains(st.sql, "zoo_ledger_access") {
				t.Fatalf("a statement of %s's run holds the grant rule:\n%s", user, st.sql)
			}
			if want := user != "ann"; strings.Contains(st.sql, " WHERE 0)") != want && strings.Contains(st.sql, "zoo_ledger_lines") {
				t.Fatalf("a statement of %s's run: refused = %v, want %v:\n%s", user, !want, want, st.sql)
			}
		}
		return n
	}
	// asked is the readers of the probes so far, by the id each one binds.
	asked := func() []string {
		out := make([]string, len(*probes))
		for i, p := range *probes {
			for _, name := range []string{"ann", "bob"} {
				for _, v := range p.params {
					if v == z.id(name) {
						out[i] = name
					}
				}
			}
		}
		return out
	}
	if n := runAs("ann"); n == 0 {
		t.Fatal("ann reads no line")
	}
	// The lines and the entries share the rule's text: one probe for both.
	if got := asked(); !slices.Equal(got, []string{"ann"}) {
		t.Fatalf("ann's run asked %v, want one probe of ann's", got)
	}
	if n := runAs("bob"); n != 0 {
		t.Fatalf("bob, whose grant cannot read, reads %d lines after ann's run", n)
	}
	if n := runAs("ann"); n == 0 {
		t.Fatal("ann reads no line after bob's run")
	}
	if got := asked(); !slices.Equal(got, []string{"ann", "bob", "ann"}) {
		t.Fatalf("three runs asked %v, want each run to ask for its own reader", got)
	}
	if err := z.app.Delete(z.recs["annLedger"]); err != nil {
		t.Fatal(err)
	}
	if n := runAs("bob"); n != 0 {
		t.Fatalf("bob reads %d lines", n)
	}
	// ann's grant is gone: her next run reads nothing. runAs expects her
	// statements to read every row, so this run is checked here.
	if n := detailRows(z.run(t, "ann", entryLinkSpec, Options{}, nil)); n != 0 {
		t.Fatalf("ann reads %d lines after her grant was deleted", n)
	}
	z.recs["bobLedger"].Set("can_read", true)
	if err := z.app.Save(z.recs["bobLedger"]); err != nil {
		t.Fatal(err)
	}
	if n := detailRows(z.run(t, "bob", entryLinkSpec, Options{}, nil)); n == 0 {
		t.Fatal("bob reads no line after his grant was given")
	}
	if got := asked(); !slices.Equal(got, []string{"ann", "bob", "ann", "bob", "ann", "bob"}) {
		t.Fatalf("six runs asked %v", got)
	}
	// Each probe is asked through its run's read transaction, so the answer
	// and the rows are of one state of the database; no two runs share one.
	txs := map[dbx.Builder]bool{}
	for i, p := range *probes {
		tx, ok := p.db.(*dbx.Tx)
		if !ok {
			t.Fatalf("probe %d was asked through %T, want the run's read transaction", i, p.db)
		}
		txs[tx] = true
	}
	if len(txs) != len(*probes) {
		t.Errorf("%d probes were asked through %d transactions, want one for each run", len(*probes), len(txs))
	}
}

// One map of answers never gives one reader another's: the key holds the
// values the probe binds, whatever PocketBase named them.
func TestAProbeKeyHoldsItsValues(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	lines, err := z.app.FindCollectionByNameOrId("zoo_ledger_lines")
	if err != nil {
		t.Fatal(err)
	}
	key := func(user string) string {
		f, err := ruleFormOf(z.app, lines, z.as(user))
		if err != nil || f.probe == "" {
			t.Fatalf("no probe: %v", err)
		}
		return probeKey(f.probe, f.params)
	}
	if key("ann") != key("ann") {
		t.Error("two builds of one rule for one reader have two keys")
	}
	if key("ann") == key("bob") {
		t.Error("ann and bob share a key")
	}
	if a, b := probeKey("SELECT {:a} = {:b}", dbx.Params{"a": "1", "b": 1}), probeKey("SELECT {:x} = {:y}", dbx.Params{"x": 1, "y": "1"}); a == b {
		t.Error("the text 1 and the number 1 share a key")
	}
	// With one shared map, as a run holds: each reader is asked and answered
	// for himself.
	answers := map[string]bool{}
	probes := observeProbes(t)
	for user, want := range map[string]bool{"ann": true, "bob": false} {
		f, err := ruleFormOf(z.app, lines, z.as(user))
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			ok, err := askRules(context.Background(), z.app.DB(), answers)(f.probe, f.params)
			if err != nil || ok != want {
				t.Errorf("%s: answer %v, %v, want %v", user, ok, err, want)
			}
		}
	}
	if len(*probes) != 2 {
		t.Errorf("%d probes for two readers asked two times each, want 2", len(*probes))
	}
}

// A probe that fails shows no rows: the run fails, and no later statement of
// the run is sent.
func TestAFailedRuleProbeFailsTheRun(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	fault := errors.New("the probe failed")
	saved := ruleAnswer
	ruleAnswer = func(context.Context, dbx.Builder, string, dbx.Params) (bool, error) { return false, fault }
	t.Cleanup(func() { ruleAnswer = saved })
	seen := observeReads(t)
	res, err := z.try(t, "ann", entryLinkSpec, Options{}, nil)
	if !errors.Is(err, fault) || res != nil {
		t.Fatalf("Run = %+v, %v, want the probe's error and no result", res, err)
	}
	if len(*seen) != 0 {
		t.Fatalf("the run sent %d statements after the failed probe", len(*seen))
	}
}

// A probe is under the run time limit: one that does not answer in time ends
// the run with ErrTimeout, and no statement is sent.
func TestARuleProbeIsUnderTheRunTimeLimit(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	limit, saved := runLimits.timeout, ruleAnswer
	runLimits.timeout = 20 * time.Millisecond
	ruleAnswer = func(ctx context.Context, _ dbx.Builder, _ string, _ dbx.Params) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(10 * time.Second):
			return true, nil
		}
	}
	t.Cleanup(func() { runLimits.timeout, ruleAnswer = limit, saved })
	seen := observeReads(t)
	res, err := z.try(t, "ann", entryLinkSpec, Options{}, nil)
	if !errors.Is(err, ErrTimeout) || res != nil {
		t.Fatalf("Run = %+v, %v, want ErrTimeout and no result", res, err)
	}
	if len(*seen) != 0 {
		t.Fatalf("the run sent %d statements after the probe ran out of time", len(*seen))
	}
}

// The topics of a run are those of the rule in full, for a reader the rule
// passes and for one it refuses: a refused reader's report still follows the
// grant collection, so a grant given later reaches it.
func TestAnAnsweredRuleKeepsTheRunsTopics(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	topicsOf := func(collections ...string) []string {
		out := make([]string, len(collections))
		for i, c := range collections {
			out[i] = changesignal.Topic(c)
		}
		slices.Sort(out)
		return out
	}
	// fullTopics is what the rules in full read, found as before the rules
	// were answered outside the statement: by explaining each rule select.
	fullTopics := func(user string, collections ...string) []string {
		var out []string
		for _, c := range collections {
			col, err := z.app.FindCollectionByNameOrId(c)
			if err != nil {
				t.Fatal(err)
			}
			sql, params, err := ruleSelect(z.app, col, z.as(user))
			if err != nil {
				t.Fatal(err)
			}
			read, err := changesignal.TopicsRead(context.Background(), z.app, sql, params)
			if err != nil {
				t.Fatal(err)
			}
			for _, topic := range read {
				if !slices.Contains(out, topic) {
					out = append(out, topic)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	sitesSpec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}]}`
	for _, user := range []string{"ann", "bob"} {
		got := z.run(t, user, entryLinkSpec, Options{}, nil).Topics
		if want := topicsOf("zoo_ledger_access", "zoo_ledger_entries", "zoo_ledger_lines"); !slices.Equal(got, want) {
			t.Errorf("%s: topics %v, want %v", user, got, want)
		}
		if want := fullTopics(user, "zoo_ledger_lines", "zoo_ledger_entries"); !slices.Equal(got, want) {
			t.Errorf("%s: topics %v, the rules in full read %v", user, got, want)
		}
		got = z.run(t, user, sitesSpec, Options{}, nil).Topics
		if want := topicsOf("users", "zoo_sites"); !slices.Equal(got, want) {
			t.Errorf("%s: sites topics %v, want %v", user, got, want)
		}
		if want := fullTopics(user, "zoo_sites"); !slices.Equal(got, want) {
			t.Errorf("%s: sites topics %v, the rule in full reads %v", user, got, want)
		}
	}
}

// A union's rule CTE holds the answer of a part whose rule one answer serves
// and the rule in full of a part whose rule reads its row; its topics are
// those of each part's rule in full.
func TestAUnionPartsRuleIsAnswered(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	RegisterSource(Source{
		Key: "zoo.entries_and_animals", Label: "Entries and animals",
		Parts: []SourcePart{
			{Collection: "zoo_ledger_entries", Columns: map[string]string{"name": "number"}},
			{Collection: "zoo_animals", Columns: map[string]string{"name": "name"}},
		},
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	if lookupSource(z.app, "zoo.entries_and_animals") == nil {
		t.Fatalf("the union is refused: %v", Refused(z.app)["zoo.entries_and_animals"])
	}
	spec := `{"version":4,"source":"zoo.entries_and_animals","columns":[{"id":"name","path":[{"field":"name"}]}]}`
	// ann: four entries, and the animals she owns or keeps (Leo, Kit, Zed).
	// bob: no entry (his grant cannot read), and Zed and Mo.
	for user, want := range map[string]int{"ann": 7, "bob": 2} {
		probes := observeProbes(t)
		seen := observeReads(t)
		res := z.run(t, user, spec, Options{}, nil)
		if n := detailRows(res); n != want {
			t.Errorf("%s reads %d rows of the union, want %d", user, n, want)
		}
		if len(*probes) != 1 {
			t.Errorf("%s: %d probes, want 1 (the entries' grant rule)", user, len(*probes))
		}
		for _, st := range *seen {
			if strings.Contains(st.sql, "__rule") {
				t.Errorf("%s: a statement holds the grant rule:\n%s", user, st.sql)
			}
			if !strings.Contains(st.sql, "FROM `zoo_animals` WHERE +`zoo_animals`.`id` IN (SELECT") {
				t.Errorf("%s: a statement lost the animals' rule, which reads its row:\n%s", user, st.sql)
			}
		}
		for _, c := range []string{"zoo_ledger_access", "zoo_ledger_entries", "zoo_animals", "zoo_keepers"} {
			if !slices.Contains(res.Topics, changesignal.Topic(c)) {
				t.Errorf("%s: topics %v lack %s", user, res.Topics, c)
			}
		}
	}
}

// A rule is asked outside its statement only when its filter does not name
// the collection's own table: the name as a whole word, in any of the forms
// dbx and PocketBase write it. A longer name that holds it is another table.
func TestNamesTable(t *testing.T) {
	for sql, want := range map[string]bool{
		"LEFT JOIN `zoo_ledger_access` `__collection_alias_me` WHERE [[__collection_alias_me.user]] = {:p}": false,
		"LEFT JOIN `zoo_ledger` `__collection_zoo_ledger`":                                                  true,
		"WHERE `__mm_zoo_ledger`.`id` = `zoo_ledger`.`id`":                                                  true,
		"WHERE [[zoo_ledger.note]] IS NOT NULL":                                                             true,
		"FROM {{zoo_ledger}}":                                                                               true,
		"WHERE zoo_ledger.id = 1":                                                                           true,
		"WHERE [[__collection_zoo_ledger.note]] = {:zoo_ledger_p}":                                          false,
	} {
		if got := namesTable(sql, "zoo_ledger"); got != want {
			t.Errorf("namesTable(%q) = %v, want %v", sql, got, want)
		}
	}
}

// grantedOptions lists the ledger lines of the picked accounts; its options
// are the ledger accounts, read under the grant rule.
const grantedOptions = `{"version":4,"source":"zoo.ledger_lines",
	"params":[{"id":"accounts","type":"choice","label":"Accounts","multiple":true,
		"optionsFrom":{"source":"zoo.ledger_accounts","value":[{"field":"id"}],"label":[{"field":"full_name"}]}}],
	"columns":[{"id":"pos","path":[{"field":"position"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"account"}]},{"param":"accounts"}]}}`

// A param's options are read outside a run, each statement asking for
// itself: a reader the grant rule refuses gets no option.
func TestParamOptionsUnderAnAnsweredRule(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	probes := observeProbes(t)
	seen := observeReads(t)
	if got := optionLabels(z.options(t, "ann", grantedOptions, OptionsRequest{Param: "accounts"})); len(got) != 8 || got[0] != "Bank" {
		t.Fatalf("ann's options = %v, want the 8 accounts from Bank", got)
	}
	if got := optionLabels(z.options(t, "bob", grantedOptions, OptionsRequest{Param: "accounts"})); len(got) != 0 {
		t.Fatalf("bob, whose grant cannot read, gets the options %v", got)
	}
	if got := optionLabels(z.options(t, "bob", grantedOptions, OptionsRequest{Param: "accounts", Values: []string{z.id("cash")}})); len(got) != 0 {
		t.Fatalf("bob gets the option of an id he names: %v", got)
	}
	// An answer is not kept past a read outside a run: bob's grant, given
	// now, shows in his next options call.
	z.recs["bobLedger"].Set("can_read", true)
	if err := z.app.Save(z.recs["bobLedger"]); err != nil {
		t.Fatal(err)
	}
	if got := optionLabels(z.options(t, "bob", grantedOptions, OptionsRequest{Param: "accounts"})); len(got) != 8 {
		t.Fatalf("bob's options after his grant = %v, want the 8 accounts", got)
	}
	if len(*seen) == 0 || len(*probes) == 0 {
		t.Fatalf("%d statements, %d probes: nothing was checked", len(*seen), len(*probes))
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, "__rule") {
			t.Errorf("an options statement holds the grant rule:\n%s", st.sql)
		}
	}
	answers := map[bool]int{}
	for _, p := range *probes {
		answers[p.ok]++
	}
	if answers[true] == 0 || answers[false] == 0 {
		t.Errorf("probe answers %v, want ann's true and bob's false", answers)
	}
}

// withList splits "WITH a AS (..),\nb AS MATERIALIZED (..)\n<body>" into its
// CTE bodies by name and the statement's body.
func withList(t *testing.T, sql string) (map[string]string, string) {
	t.Helper()
	rest, ok := strings.CutPrefix(sql, "WITH ")
	if !ok {
		t.Fatalf("the statement has no WITH:\n%.200s", sql)
	}
	head := regexp.MustCompile(`^(\w+) AS (?:MATERIALIZED )?\(`)
	bodies := map[string]string{}
	for {
		m := head.FindStringSubmatch(rest)
		if m == nil {
			t.Fatalf("no CTE at:\n%.200s", rest)
		}
		rest = rest[len(m[0]):]
		depth, quoted, end := 1, false, -1
		for i := 0; i < len(rest) && end < 0; i++ {
			switch c := rest[i]; {
			case c == '\'':
				quoted = !quoted
			case quoted:
			case c == '(':
				depth++
			case c == ')':
				if depth--; depth == 0 {
					end = i
				}
			}
		}
		if end < 0 {
			t.Fatalf("CTE %s does not close", m[1])
		}
		bodies[m[1]] = rest[:end]
		rest = rest[end+1:]
		if next, more := strings.CutPrefix(rest, ",\n"); more {
			rest = next
			continue
		}
		return bodies, strings.TrimPrefix(rest, "\n")
	}
}

var cteRead = regexp.MustCompile(`\b(?:FROM|JOIN) (\w+)\b`)

// expandedSize is the size of a statement with each read of a CTE (FROM or
// JOIN) replaced by the CTE's own expanded text: what SQLite prepares, since
// it copies a CTE at every reference.
func expandedSize(bodies map[string]string, text string, memo map[string]int) int {
	size := len(text)
	for _, m := range cteRead.FindAllStringSubmatch(text, -1) {
		body, ok := bodies[m[1]]
		if !ok {
			continue
		}
		n, done := memo[m[1]]
		if !done {
			n = expandedSize(bodies, body, memo)
			memo[m[1]] = n
		}
		size += n
	}
	return size
}

// ruleCopies is how many copies of a rule CTE (r<n>) the expanded statement
// holds: each read of a rule CTE, counted once for each copy of the CTE that
// reads it.
func ruleCopies(bodies map[string]string, text string, memo map[string]int) int {
	copies := 0
	for _, m := range cteRead.FindAllStringSubmatch(text, -1) {
		body, ok := bodies[m[1]]
		if !ok {
			continue
		}
		if ruleName.MatchString(m[1]) {
			copies++
			continue
		}
		n, done := memo[m[1]]
		if !done {
			n = ruleCopies(bodies, body, memo)
			memo[m[1]] = n
		}
		copies += n
	}
	return copies
}

var ruleName = regexp.MustCompile(`^r\d+$`)

// opcodes is the length of the program SQLite prepares for a statement.
func opcodes(t *testing.T, z *zoo, sql string, params dbx.Params) int {
	t.Helper()
	raw, args := rawStatement(t, sql, params)
	rows, err := readPool(t, z).Query("EXPLAIN "+raw, args...)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// SQLite copies a CTE at every reference, so the Balance-Sheet-shaped layout
// prepares its rule CTEs hundreds of times. With the grant rule answered by
// one probe, each copy is the table's rows and holds no join: the statement
// SQLite prepares is smaller, in expanded text and in program, than the same
// statement with the rule in each copy.
func TestABalanceSheetShapedStatementHoldsNoRule(t *testing.T) {
	z := balanceSheetZoo(t, 40)
	probes := observeProbes(t)
	m := measureRun(t, z, benchBalanceSheet())
	if len(*probes) != 1 {
		t.Fatalf("the run asked %d probes, want 1: its collections share one rule", len(*probes))
	}
	if strings.Contains(m.sql, "__rule") || strings.Contains(m.sql, "zoo_ledger_access") {
		t.Fatalf("the group statement holds the grant rule:\n%.2000s", m.sql)
	}
	bodies, body := withList(t, m.sql)
	table := regexp.MustCompile("^SELECT `(\\w+)`\\.\\* FROM `(\\w+)`$")
	// held is the statement as it was sent before: the rule in full in each
	// rule CTE.
	held, heldSQL, heldParams := map[string]string{}, m.sql, dbx.Params{}
	if err := mergeParams(heldParams, m.params); err != nil {
		t.Fatal(err)
	}
	rules := 0
	for name, text := range bodies {
		held[name] = text
		if !ruleName.MatchString(name) {
			continue
		}
		tm := table.FindStringSubmatch(text)
		if tm == nil || tm[1] != tm[2] {
			t.Fatalf("rule CTE %s is not its table's rows: %s", name, text)
		}
		col, err := z.app.FindCollectionByNameOrId(tm[1])
		if err != nil {
			t.Fatal(err)
		}
		full, params, err := ruleSelect(z.app, col, z.as("ann"))
		if err != nil {
			t.Fatal(err)
		}
		if err := mergeParams(heldParams, params); err != nil {
			t.Fatal(err)
		}
		rules++
		held[name] = full
		heldSQL = strings.Replace(heldSQL, name+" AS ("+text+")", name+" AS ("+full+")", 1)
	}
	if rules < 2 || !strings.Contains(heldSQL, "__rule") {
		t.Fatalf("%d rule CTEs put back, want the lines and the accounts", rules)
	}
	copies := ruleCopies(bodies, body, map[string]int{})
	if copies < 100 {
		t.Fatalf("the expanded statement holds %d copies of a rule CTE: the fixture no longer shows the cost", copies)
	}
	sentSize, heldSize := expandedSize(bodies, body, map[string]int{}), expandedSize(held, body, map[string]int{})
	sentOps, heldOps := opcodes(t, z, m.sql, m.params), opcodes(t, z, heldSQL, heldParams)
	t.Logf("statement %d bytes; %d copies of a rule CTE; expanded %d bytes (%d with the rule in each copy); %d opcodes (%d with the rule)",
		len(m.sql), copies, sentSize, heldSize, sentOps, heldOps)
	if saved := heldSize - sentSize; saved < copies*100 {
		t.Errorf("the expanded statement is %d bytes smaller than with the rule in each of %d copies, want at least 100 bytes for each", saved, copies)
	}
	if sentOps >= heldOps {
		t.Errorf("the statement prepares to %d opcodes, with the rule in each copy %d: want fewer", sentOps, heldOps)
	}
}
