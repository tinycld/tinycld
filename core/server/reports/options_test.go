package reports

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/oauth"
)

// animalsParam is a feedings report whose Animals filter lists the animals
// the reader may see, grouped by species.
const animalsParam = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"animals","type":"choice","label":"Animals","multiple":true,
		"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]},
	"sort":[{"col":"note"}]}`

// A run checks an options-from-source value by type only.
func TestARunTakesAnyIDForAnOptionsFromSourceParam(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["` + z.id("leo") + `"]`})
	requireOrderedRows(t, animalsParam, res, "note", "f1", "f2", nil)
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["not-an-animal"]`})
	requireRows(t, res, "note", nil)
	all := z.run(t, "ann", animalsParam, Options{}, nil)
	requireOrderedRows(t, animalsParam, all, "note", "f1", "f2", "f3", "f4", nil)
	many := make([]string, maxChoiceValues+1)
	for i := range many {
		many[i] = fmt.Sprintf("%q", fmt.Sprint("x", i))
	}
	_, err := z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": "[" + strings.Join(many, ",") + "]"})
	requireSpecError(t, err, 400, "params.animals", "pick a whole group instead")
}

func TestOptionsFromIsChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"params":[%s]}`
	for _, c := range []struct {
		param, at, want string
		status          int
	}{
		{`{"id":"p","type":"text","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0", "only a choice param has options", 400},
		{`{"id":"p","type":"choice","options":[{"value":"a","label":"A"}],"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0.options", "or names the source they come from", 400},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.gone","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0.optionsFrom.source", "does not exist", 422},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"nope"}],"label":[{"field":"name"}]}}`, "params.0.optionsFrom.value.0.field", "has no field", 422},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"where":{"value":2}}}`, "params.0.optionsFrom.where", "where is a condition", 400},
		// Options are not report rows: their where reads the source's paths.
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"where":{"op":"eq","args":[{"col":"n"},{"value":"x"}]}}}`, "params.0.optionsFrom.where.args.0", "not report columns", 400},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"link":"feedings"}],"label":[{"field":"name"}]}}`, "params.0.optionsFrom.value.0", "to-many", 400},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}]}}`, "params.0.optionsFrom.label", "at least one step", 400},
	} {
		spec, _, err := Load([]byte(fmt.Sprintf(base, c.param)))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
	}
}

// optionRows runs a param's options select as user and lists each option as
// "label/group", ordered by label.
func (z *zoo) optionRows(t *testing.T, user, raw, param string) []string {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	p, specErr := bind(z.app, spec)
	if specErr != nil {
		t.Fatal(specErr)
	}
	c := newCompiler(p, z.app, z.as(user), nil)
	s := newStmt(z.app, z.as(user))
	body, err := c.optionsSelect(s, p.options[param])
	if err != nil {
		t.Fatal(err)
	}
	with, err := s.with()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := z.app.DB().NewQuery(with + body + "\nORDER BY 2, 1").Bind(s.params).Rows()
	if err != nil {
		t.Fatal(err)
	}
	read, err := scanRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, row := range read {
		out = append(out, fmt.Sprintf("%v/%v", row["l"], row["g"]))
	}
	return out
}

func requireOptions(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("options = %v, want %v", got, want)
	}
}

// Options are the rows of their source the reader may list: a value the
// reader cannot see never appears.
func TestOptionsSelectReadsUnderTheReadersRules(t *testing.T) {
	z := newZoo(t)
	requireOptions(t, z.optionRows(t, "ann", animalsParam, "animals"), "Kit/cat", "Leo/lion", "Zed/zebra")
	requireOptions(t, z.optionRows(t, "bob", animalsParam, "animals"), "Mo/monkey", "Zed/zebra")

	active := strings.Replace(animalsParam, `"group":[{"field":"species"}]`,
		`"group":[{"field":"species"}],"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}`, 1)
	requireOptions(t, z.optionRows(t, "bob", active, "animals"), "Zed/zebra")

	// A where reads the request's other params.
	bySpecies := strings.Replace(animalsParam, `"params":[`, `"params":[{"id":"kind","type":"text","default":"lion"},`, 1)
	bySpecies = strings.Replace(bySpecies, `"group":[{"field":"species"}]`,
		`"group":[{"field":"species"}],"where":{"op":"eq","args":[{"path":[{"field":"species"}]},{"param":"kind"}]}`, 1)
	requireOptions(t, z.optionRows(t, "ann", bySpecies, "animals"), "Leo/lion")

	// No reader may list the vault: its linked records are no one's options.
	vault := strings.Replace(animalsParam, `"value":[{"field":"id"}],"label":[{"field":"name"}]`,
		`"value":[{"link":"vault"}],"label":[{"link":"vault"},{"field":"name"}]`, 1)
	requireOptions(t, z.optionRows(t, "ann", vault, "animals"))
}

// An option's label read from a link shows the record's label, and a select
// shows its option's label; the group keeps its stored value.
func TestOptionLabelsShowRecordsAndSelectOptions(t *testing.T) {
	z := newZoo(t)
	sites := strings.Replace(animalsParam, `"value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]`,
		`"value":[{"link":"site"}],"label":[{"link":"site"}],"group":[{"link":"site"},{"field":"region"}]`, 1)
	requireOptions(t, z.optionRows(t, "ann", sites, "animals"), "North/north", "South/south")

	regions := strings.Replace(animalsParam, `"value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]`,
		`"value":[{"link":"site"},{"field":"region"}],"label":[{"link":"site"},{"field":"region"}]`, 1)
	requireOptions(t, z.optionRows(t, "ann", regions, "animals"), "North/<nil>", "South/<nil>")
}

// An options source's renamed keys load as their current keys.
func TestOptionsFromPathsResolveAliases(t *testing.T) {
	withZooRegistry(t)
	spec, info, err := Load([]byte(strings.Replace(animalsParam, `"label":[{"field":"name"}],"group":[{"field":"species"}]`,
		`"label":[{"field":"mass"}],"group":[{"field":"mass"}],"where":{"op":"gt","args":[{"path":[{"field":"mass"}]},{"value":1}]}`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	of := spec.Params[0].OptionsFrom
	if of.Label[0].Field != "weight" || of.Group[0].Field != "weight" || of.Where.Args[0].Path[0].Field != "weight" || len(info.Aliased) != 3 {
		t.Fatalf("optionsFrom = %+v, aliased %v", of, info.Aliased)
	}
}

func (z *zoo) options(t *testing.T, user, spec string, req OptionsRequest) *ParamOptions {
	t.Helper()
	loaded, _, err := Load([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	req.Spec, req.Auth = loaded, z.recs[user]
	res, err := ReadParamOptions(context.Background(), z.app, req)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	return res
}

func optionLabels(res *ParamOptions) []string {
	out := []string{}
	for _, o := range res.Options {
		out = append(out, o.Label)
	}
	return out
}

// Review Focus 4: by list, search, group or values, a reader gets only rows
// their rules let them list.
func TestParamOptionsReadOnlyRowsTheReaderMayList(t *testing.T) {
	z := newZoo(t)
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Kit", "Leo", "Zed"}) {
		t.Fatalf("ann = %v", got)
	}
	if got := optionLabels(z.options(t, "bob", animalsParam, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Mo", "Zed"}) {
		t.Fatalf("bob = %v", got)
	}
	// "o" is in Leo and in bob's Mo, which ann may not list.
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Search: "o"})); !slices.Equal(got, []string{"Leo"}) {
		t.Fatalf("search = %v", got)
	}
	lion := "lion"
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Group: &lion})); !slices.Equal(got, []string{"Leo"}) {
		t.Fatalf("group = %v", got)
	}
	values := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Values: []string{z.id("leo"), z.id("mo")}})
	if got := optionLabels(values); !slices.Equal(got, []string{"Leo"}) || values.Options[0].Value != z.id("leo") || values.Options[0].Group != "lion" {
		t.Fatalf("values = %+v", values.Options)
	}
	groups := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"}).Groups
	if len(groups) != 3 || groups[0] != (OptionGroup{Value: "cat", Label: "cat"}) {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestParamOptionsPageByLabel(t *testing.T) {
	z := newZoo(t)
	first := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Limit: 2})
	if got := optionLabels(first); !slices.Equal(got, []string{"Kit", "Leo"}) || first.Next == "" {
		t.Fatalf("first = %v next %q", got, first.Next)
	}
	next := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Limit: 2, After: first.Next})
	if got := optionLabels(next); !slices.Equal(got, []string{"Zed"}) || next.Next != "" {
		t.Fatalf("next = %v next %q", got, next.Next)
	}
	if len(first.Groups) == 0 || len(next.Groups) != 0 {
		t.Fatalf("groups come with the first page only: %v then %v", first.Groups, next.Groups)
	}
}

// A group read from a link shows the record's label, never its id, and a
// select group shows its option's label; the group's value stays the stored
// value the picker posts back. Options with no group value (Kit has no site)
// make one blank group, "".
func TestParamOptionGroupsShowLabels(t *testing.T) {
	z := newZoo(t)
	bySite := strings.Replace(animalsParam, `"group":[{"field":"species"}]`, `"group":[{"link":"site"}]`, 1)
	res := z.options(t, "ann", bySite, OptionsRequest{Param: "animals"})
	want := []OptionGroup{{Value: "", Label: "(none)"}, {Value: z.id("north"), Label: "North"}, {Value: z.id("south"), Label: "South"}}
	if !slices.Equal(res.Groups, want) {
		t.Fatalf("site groups = %+v, want %+v", res.Groups, want)
	}
	if res.Options[0].Label != "Kit" || res.Options[0].Group != "" || res.Options[1].Group != z.id("north") {
		t.Fatalf("site options = %+v", res.Options)
	}
	north, blank := z.id("north"), ""
	if got := optionLabels(z.options(t, "ann", bySite, OptionsRequest{Param: "animals", Group: &north})); !slices.Equal(got, []string{"Leo"}) {
		t.Fatalf("north = %v", got)
	}
	if got := optionLabels(z.options(t, "ann", bySite, OptionsRequest{Param: "animals", Group: &blank})); !slices.Equal(got, []string{"Kit"}) {
		t.Fatalf("no site = %v", got)
	}

	byRegion := strings.Replace(animalsParam, `"group":[{"field":"species"}]`, `"group":[{"link":"site"},{"field":"region"}]`, 1)
	res = z.options(t, "ann", byRegion, OptionsRequest{Param: "animals"})
	want = []OptionGroup{{Value: "", Label: "(none)"}, {Value: "north", Label: "North"}, {Value: "south", Label: "South"}}
	if !slices.Equal(res.Groups, want) {
		t.Fatalf("region groups = %+v, want %+v", res.Groups, want)
	}
	// Groups are of the options that match the search.
	res = z.options(t, "ann", byRegion, OptionsRequest{Param: "animals", Search: "e"})
	want = []OptionGroup{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}
	if !slices.Equal(res.Groups, want) {
		t.Fatalf("searched region groups = %+v, want %+v", res.Groups, want)
	}

	// A label read from a select is already its option's label.
	regions := strings.Replace(animalsParam, `"value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]`,
		`"value":[{"link":"site"},{"field":"region"}],"label":[{"link":"site"},{"field":"region"}]`, 1)
	res = z.options(t, "ann", regions, OptionsRequest{Param: "animals"})
	if got := optionLabels(res); !slices.Equal(got, []string{"North", "South"}) || res.Options[0].Value != "north" || len(res.Groups) != 0 {
		t.Fatalf("regions = %+v groups %+v", res.Options, res.Groups)
	}
}

func TestParamOptionsWhereAndRefusals(t *testing.T) {
	z := newZoo(t)
	active := strings.Replace(animalsParam, `"group":[{"field":"species"}]`,
		`"group":[{"field":"species"}],"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}`, 1)
	if got := optionLabels(z.options(t, "bob", active, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Zed"}) {
		t.Fatalf("bob active = %v", got)
	}
	loaded, _, _ := Load([]byte(animalsParam))
	for _, c := range []struct {
		req      OptionsRequest
		at, want string
	}{
		{OptionsRequest{Param: "nope"}, "param", "no choice param"},
		{OptionsRequest{Param: "animals", Search: strings.Repeat("x", 201)}, "search", "at most 200"},
		{OptionsRequest{Param: "animals", Limit: 1001}, "limit", "1 to 1000"},
		{OptionsRequest{Param: "animals", After: "garbage"}, "after", "not one this list gave"},
		{OptionsRequest{Param: "animals", Values: make([]string, 1001)}, "values", "at most 1000"},
		{OptionsRequest{Param: "animals", Today: "soon"}, "today", "YYYY-MM-DD"},
		{OptionsRequest{Param: "animals", Values: []string{"x"}, Limit: 5}, "values", "labelled alone"},
		{OptionsRequest{Param: "animals", Values: []string{"x"}, Search: "x"}, "values", "labelled alone"},
	} {
		c.req.Spec, c.req.Auth = loaded, z.recs["ann"]
		_, err := ReadParamOptions(context.Background(), z.app, c.req)
		requireSpecError(t, err, 400, c.at, c.want)
	}
	ungrouped, _, _ := Load([]byte(strings.Replace(animalsParam, `,"group":[{"field":"species"}]`, ``, 1)))
	lion := "lion"
	_, err := ReadParamOptions(context.Background(), z.app, OptionsRequest{Spec: ungrouped, Param: "animals", Group: &lion, Auth: z.recs["ann"]})
	requireSpecError(t, err, 400, "group", "have no groups")
	_, err = ReadParamOptions(context.Background(), z.app, OptionsRequest{Spec: loaded, Param: "animals"})
	if !errors.Is(err, ErrNoReader) {
		t.Fatalf("no reader: %v", err)
	}
}

func TestParamOptionsEndpoint(t *testing.T) {
	s := newSavedZoo(t)
	o := s.saved(t, "O", "ann", "O", animalsParam, false)
	const url = "/api/reports/param-options"
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals"}`, http.StatusOK, `"label":"Kit"`, `"groups":[`)
	s.expect(t, "ann", http.MethodPost, url, `{"reportId":"`+o.Id+`","param":"animals","search":"ze"}`, http.StatusOK, `"label":"Zed"`)
	s.expect(t, "ot", http.MethodPost, url, `{"reportId":"`+o.Id+`","param":"animals"}`, http.StatusForbidden, `"reportId":{"code":"no_access"`)
	s.expect(t, "", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals"}`, http.StatusUnauthorized)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals","sql":"x"}`, http.StatusBadRequest, `unknown field`)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"reportId":"`+o.Id+`","param":"animals"}`, http.StatusBadRequest, `not both`)
	s.expect(t, "ann", http.MethodPost, url, `{"param":"animals"}`, http.StatusBadRequest, `no spec and no reportId`)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals"} {}`, http.StatusBadRequest, `after the first object`)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"nope"}`, http.StatusBadRequest, `"param":{"code":"invalid"`)
	if got := oauth.ScopeForRoute(http.MethodPost, url); len(got) != 0 {
		t.Fatalf("param-options resolved to scopes %v, want default-deny", got)
	}
}

// Options are read like a run: an OAuth token is refused by default-deny,
// and the same reader's session token reads them.
func TestParamOptionsRefuseAnOAuthTokenThroughTheMiddleware(t *testing.T) {
	z := newZoo(t)
	oauth.Register(z.app)
	Register(z.app)
	client, err := oauth.FindClientByClientID(z.app, "tinycld-cli")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := oauth.NewGrant(z.app, z.id("ann"), client.Id, oauth.AllScopes(), "active")
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.MintAccessToken(z.app, z.recs["ann"], grant, oauth.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"spec":` + animalsParam + `,"param":"animals"}`
	for _, c := range []struct {
		name, token, want string
		status            int
	}{
		{"an OAuth token", "Bearer " + token, `not available to API tokens`, http.StatusForbidden},
		{"a session token", z.token(t, "ann"), `"label":"Kit"`, http.StatusOK},
	} {
		sc := tests.ApiScenario{
			Name: c.name, Method: http.MethodPost, URL: "/api/reports/param-options",
			Body: strings.NewReader(body), Headers: map[string]string{"Authorization": c.token},
			ExpectedStatus: c.status, ExpectedContent: []string{c.want},
			TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
			DisableTestAppCleanup: true,
		}
		sc.Test(t)
	}
}

// withGroup is animalsParam with its options grouped by group (a path).
func withGroup(group string) string {
	return strings.Replace(animalsParam, `"group":[{"field":"species"}]`, `"group":`+group, 1)
}

func groupsOf(res *ParamOptions) []OptionGroup { return res.Groups }

// A search matches any case of any script: Unicode case folding, not
// SQLite's ASCII-only lower().
func TestParamOptionsSearchFoldsUnicodeCase(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "emile", "zoo_animals", map[string]any{"name": "Émile", "species": "emu", "owner": z.id("ann")})
	z.rec(t, "strasse", "zoo_animals", map[string]any{"name": "Straße", "species": "stork", "owner": z.id("ann")})
	for search, want := range map[string]string{"émile": "Émile", "ÉMILE": "Émile", "STRASSE": "Straße", "straße": "Straße"} {
		if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Search: search})); !slices.Equal(got, []string{want}) {
			t.Fatalf("search %q = %v, want %s", search, got, want)
		}
	}
}

// Two options with one label page by value: a page boundary between them
// loses neither and repeats neither.
func TestParamOptionsPageSplitsADuplicateLabel(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "leo2", "zoo_animals", map[string]any{"name": "Leo", "species": "lion", "owner": z.id("ann")})
	var labels, values []string
	req := OptionsRequest{Param: "animals", Limit: 1}
	for range 10 {
		res := z.options(t, "ann", animalsParam, req)
		for _, o := range res.Options {
			labels, values = append(labels, o.Label), append(values, o.Value)
		}
		if res.Next == "" {
			break
		}
		req.After = res.Next
	}
	leos := []string{z.id("leo"), z.id("leo2")}
	slices.Sort(leos)
	if !slices.Equal(labels, []string{"Kit", "Leo", "Leo", "Zed"}) || !slices.Equal(values, []string{z.id("kit"), leos[0], leos[1], z.id("zed")}) {
		t.Fatalf("pages = %v %v", labels, values)
	}
}

// A groups list stops at maxOptionGroups and says so.
func TestParamOptionGroupsSayWhenCut(t *testing.T) {
	z := newZoo(t)
	if z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"}).GroupsCut {
		t.Fatal("three groups are not cut")
	}
	for i := range maxOptionGroups {
		z.rec(t, fmt.Sprint("a", i), "zoo_animals", map[string]any{"name": fmt.Sprint("A", i), "species": fmt.Sprintf("s%03d", i), "owner": z.id("ann")})
	}
	res := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"})
	if len(res.Groups) != maxOptionGroups || !res.GroupsCut {
		t.Fatalf("groups = %d, cut %v", len(res.Groups), res.GroupsCut)
	}
}

// A value whose rows differ keeps its set group over a blank one, and a link
// to a record the reader cannot see joins the one blank group.
func TestParamOptionGroupsKeepTheSetGroupAndMergeHiddenLinks(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "nobody", "zoo_animals", map[string]any{"name": "Nobody", "site": z.id("north"), "owner": z.id("ann")})
	bySite := strings.Replace(animalsParam, `"value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]`,
		`"value":[{"link":"site"}],"label":[{"link":"site"}],"group":[{"field":"species"}]`, 1)
	res := z.options(t, "ann", bySite, OptionsRequest{Param: "animals"})
	if len(res.Options) != 2 || res.Options[0].Label != "North" || res.Options[0].Group != "lion" {
		t.Fatalf("site options = %+v", res.Options)
	}

	// No reader may list the vault: Leo's vault reads as no vault.
	res = z.options(t, "ann", withGroup(`[{"link":"vault"}]`), OptionsRequest{Param: "animals"})
	if want := []OptionGroup{{Value: "", Label: "(none)"}}; !slices.Equal(res.Groups, want) {
		t.Fatalf("vault groups = %+v, want %+v", res.Groups, want)
	}
}

// A group read from a yes/no, number, amount or date is labelled as a run
// labels a group of that type: Yes or No, or a typed value the client
// formats; numbers order by value.
func TestParamOptionGroupsOfTypedFields(t *testing.T) {
	z := newZoo(t)
	for _, c := range []struct {
		group, user string
		want        []OptionGroup
	}{
		{`[{"field":"active"}]`, "bob", []OptionGroup{{Value: "0", Label: "No"}, {Value: "1", Label: "Yes"}}},
		{`[{"field":"weight"}]`, "ann", []OptionGroup{{Value: "4", Label: "4", Type: Number}, {Value: "190", Label: "190", Type: Number}, {Value: "300", Label: "300", Type: Number}}},
		// The animals' prices are in their source's fixed currency.
		{`[{"field":"price"}]`, "ann", []OptionGroup{{Value: "5000 USD", Label: "5000", Type: Money, Currency: "USD"}, {Value: "120000 USD", Label: "120000", Type: Money, Currency: "USD"}, {Value: "500000 USD", Label: "500000", Type: Money, Currency: "USD"}}},
		{`[{"field":"born"}]`, "ann", []OptionGroup{{Value: "", Label: "(none)"}, {Value: "2019-02-10", Label: "2019-02-10", Type: Date}, {Value: "2020-05-01", Label: "2020-05-01", Type: Date}}},
	} {
		if got := groupsOf(z.options(t, c.user, withGroup(c.group), OptionsRequest{Param: "animals"})); !slices.Equal(got, c.want) {
			t.Fatalf("%s groups = %+v, want %+v", c.group, got, c.want)
		}
	}
}

// An amount group with a currency field groups by amount and currency: 300
// USD and 300 EUR are two groups, and a group filter names both.
func TestParamOptionAmountGroupsKeepTheirCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "otto", "zoo_animals", map[string]any{"name": "Otto", "species": "otter", "owner": z.id("ann"), "fee": 300, "fee_currency": "EUR"})
	byFee := withGroup(`[{"field":"fee"}]`)
	res := z.options(t, "ann", byFee, OptionsRequest{Param: "animals"})
	want := []OptionGroup{
		{Value: "50 USD", Label: "50", Type: Money, Currency: "USD"},
		{Value: "300 EUR", Label: "300", Type: Money, Currency: "EUR"},
		{Value: "300 USD", Label: "300", Type: Money, Currency: "USD"},
		{Value: "700 EUR", Label: "700", Type: Money, Currency: "EUR"},
	}
	if !slices.Equal(res.Groups, want) {
		t.Fatalf("fee groups = %+v, want %+v", res.Groups, want)
	}
	eur := "300 EUR"
	if got := optionLabels(z.options(t, "ann", byFee, OptionsRequest{Param: "animals", Group: &eur})); !slices.Equal(got, []string{"Otto"}) {
		t.Fatalf("300 EUR = %v", got)
	}
}

// An option label read from a yes/no, number, amount or date is typed as a
// group label is: Yes or No, or a typed value (an amount with its currency)
// the client formats; numbers order and page by value. The options are not
// grouped: grouped options come group by group (grouporder_test.go).
func TestParamOptionLabelsOfTypedFields(t *testing.T) {
	z := newZoo(t)
	labelled := func(label string) string {
		return strings.Replace(animalsParam, `"label":[{"field":"name"}],"group":[{"field":"species"}]`, `"label":`+label, 1)
	}
	strip := func(res *ParamOptions) []ParamOption {
		out := []ParamOption{}
		for _, o := range res.Options {
			out = append(out, ParamOption{Label: o.Label, Type: o.Type, Currency: o.Currency})
		}
		return out
	}
	for _, c := range []struct {
		label, user string
		want        []ParamOption
	}{
		{`[{"field":"active"}]`, "bob", []ParamOption{{Label: "No"}, {Label: "Yes"}}},
		{`[{"field":"weight"}]`, "ann", []ParamOption{{Label: "4", Type: Number}, {Label: "190", Type: Number}, {Label: "300", Type: Number}}},
		{`[{"field":"fee"}]`, "ann", []ParamOption{{Label: "50", Type: Money, Currency: "USD"}, {Label: "300", Type: Money, Currency: "USD"}, {Label: "700", Type: Money, Currency: "EUR"}}},
		{`[{"field":"born"}]`, "ann", []ParamOption{{Label: "(none)"}, {Label: "2019-02-10", Type: Date}, {Label: "2020-05-01", Type: Date}}},
	} {
		if got := strip(z.options(t, c.user, labelled(c.label), OptionsRequest{Param: "animals"})); !slices.Equal(got, c.want) {
			t.Fatalf("%s options = %+v, want %+v", c.label, got, c.want)
		}
	}
	// A yes/no is searched as it is shown.
	if got := optionLabels(z.options(t, "bob", labelled(`[{"field":"active"}]`), OptionsRequest{Param: "animals", Search: "yes"})); !slices.Equal(got, []string{"Yes"}) {
		t.Fatalf("search yes = %v", got)
	}
	// Pages of numbers follow the number order.
	var pages []string
	req := OptionsRequest{Param: "animals", Limit: 1}
	for range 5 {
		res := z.options(t, "ann", labelled(`[{"field":"weight"}]`), req)
		pages = append(pages, optionLabels(res)...)
		if res.Next == "" {
			break
		}
		req.After = res.Next
	}
	if !slices.Equal(pages, []string{"4", "190", "300"}) {
		t.Fatalf("weight pages = %v", pages)
	}
}

// A group reference picks every option of the group the reader may list, at
// run time: no cap, and it follows the reader's rules.
func TestAGroupReferenceExpandsOnTheServer(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":"zebra"}]`})
	requireOrderedRows(t, animalsParam, res, "note", "f3", "f4", nil)
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["` + z.id("leo") + `",{"group":"zebra"}]`})
	requireOrderedRows(t, animalsParam, res, "note", "f1", "f2", "f3", "f4", nil)
	if p := res.Params[0]; len(p.Groups) != 1 || p.Groups[0] != "zebra" || len(p.List) != 1 {
		t.Fatalf("resolved = %+v", p)
	}
	_, err := z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":7}]`})
	requireSpecError(t, err, 400, "params.animals", "a group is")

	// A group beside the most explicit values a list holds: past SQLite's 500
	// terms of a compound select.
	values := []string{`"` + z.id("leo") + `"`}
	for i := 1; i < maxChoiceValues; i++ {
		values = append(values, fmt.Sprintf("%q", fmt.Sprint("x", i)))
	}
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":"zebra"},` + strings.Join(values, ",") + "]"})
	requireOrderedRows(t, animalsParam, res, "note", "f1", "f2", "f3", "f4", nil)
}

// A group expands to the options the reader may list, never to a value the
// reader cannot see; the posted group is a bound value, never SQL.
func TestAGroupReferenceReadsUnderTheReadersRules(t *testing.T) {
	z := newZoo(t)
	// Every member lists every site; the options are the sites of the animals
	// the reader may list, grouped by species.
	sitesOfAnimals := `{"version":2,"source":"zoo.sites",
		"params":[{"id":"sites","type":"choice","multiple":true,
			"optionsFrom":{"source":"zoo.animals","value":[{"link":"site"}],"label":[{"link":"site"}],"group":[{"field":"species"}]}}],
		"columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"sites"}]},
		"sort":[{"col":"name"}]}`
	lions := map[string]string{"sites": `[{"group":"lion"}]`}
	requireRows(t, z.run(t, "ann", sitesOfAnimals, Options{}, lions), "name", "North", nil)
	// Bob cannot list Leo, the one lion: the group is empty for him.
	requireRows(t, z.run(t, "bob", sitesOfAnimals, Options{}, lions), "name", nil)
	injected := map[string]string{"sites": `[{"group":"lion' OR 1=1 --"}]`}
	requireRows(t, z.run(t, "ann", sitesOfAnimals, Options{}, injected), "name", nil)

	// Leo's vault is hidden, so Leo is in the blank group, and the vault's id
	// is no group at all.
	byVault := withGroup(`[{"link":"vault"}]`)
	requireOrderedRows(t, byVault, z.run(t, "ann", byVault, Options{}, map[string]string{"animals": `[{"group":""}]`}), "note", "f1", "f2", "f3", "f4", nil)
	requireRows(t, z.run(t, "ann", byVault, Options{}, map[string]string{"animals": `[{"group":"` + z.id("v1") + `"}]`}), "note", nil)
}

// A typed group compares by its stored value, and an amount group names its
// amount and its currency: 300 USD and 300 EUR are two groups.
func TestAGroupReferenceOfATypedGroup(t *testing.T) {
	z := newZoo(t)
	otto := z.rec(t, "otto", "zoo_animals", map[string]any{"name": "Otto", "species": "otter", "owner": z.id("ann"), "fee": 300, "fee_currency": "EUR"})
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": otto.Id})
	z.feeding(t, "f7", otto, 40, "2026-04-01", 10)
	for _, c := range []struct {
		group, value string
		want         []any
	}{
		{`[{"field":"fee"}]`, "300 USD", []any{"f3", "f4", nil}},
		{`[{"field":"fee"}]`, "300 EUR", []any{"f7", nil}},
		{`[{"field":"fee"}]`, "300", []any{nil}},
		{`[{"field":"weight"}]`, "190", []any{"f1", "f2", nil}},
		{`[{"field":"born"}]`, "2019-02-10", []any{"f3", "f4", nil}},
		// Otto's yes/no is unset, which is stored as no.
		{`[{"field":"active"}]`, "0", []any{"f7", nil}},
		{`[{"field":"active"}]`, "1", []any{"f1", "f2", "f3", "f4", nil}},
	} {
		spec := withGroup(c.group)
		res := z.run(t, "ann", spec, Options{}, map[string]string{"animals": `[{"group":"` + c.value + `"}]`})
		requireOrderedRows(t, spec, res, "note", c.want...)
	}
}

// Every place a param is read reads the expanded group: a column, a having
// and a lookup's where, as well as the filter.
func TestAGroupReferenceInEveryContext(t *testing.T) {
	z := newZoo(t)
	zebras := map[string]string{"animals": `[{"group":"zebra"}]`}
	picked := strings.Replace(animalsParam, `"columns":[{"id":"note","path":[{"field":"note"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]},`,
		`"columns":[{"id":"note","path":[{"field":"note"}]},
			{"id":"picked","formula":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]}},
			{"id":"name","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"animals"}]},"value":{"path":[{"field":"name"}]}}}],`, 1)
	res := z.run(t, "ann", picked, Options{}, zebras)
	requireOrderedRows(t, picked, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, picked, res, "picked", false, false, true, true, nil)
	requireRows(t, res, "name", "Zed", "Zed", "Zed", "Zed", "Zed")

	having := strings.Replace(animalsParam, `"columns":[{"id":"note","path":[{"field":"note"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]},`,
		`"columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],
		"groups":[{"id":"g","col":"animal","having":{"op":"in","args":[{"col":"animal"},{"param":"animals"}]}}],`, 1)
	res = z.run(t, "ann", having, Options{}, zebras)
	requireRows(t, res, "kind", "group", "total")
	requireRows(t, res, "animal", z.id("zed"), nil)
}

// A group reference needs grouped options, at most maxOptionGroups of them,
// and options that do not read their own group.
func TestAGroupReferenceRefusals(t *testing.T) {
	z := newZoo(t)
	ungrouped := strings.Replace(animalsParam, `,"group":[{"field":"species"}]`, ``, 1)
	_, err := z.try(t, "ann", ungrouped, Options{}, map[string]string{"animals": `[{"group":"zebra"}]`})
	requireSpecError(t, err, 400, "params.animals", "have no groups")

	// Options with no groups give no hint to pick a group.
	tooMany := make([]string, maxChoiceValues+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("%q", fmt.Sprint("x", i))
	}
	_, err = z.try(t, "ann", ungrouped, Options{}, map[string]string{"animals": "[" + strings.Join(tooMany, ",") + "]"})
	requireSpecError(t, err, 400, "params.animals", "at most 1000 choices")
	var se *SpecError
	if errors.As(err, &se) && strings.Contains(se.Problems["params.animals"].Error(), "group") {
		t.Fatalf("ungrouped over-cap = %v", se)
	}

	many := make([]string, maxOptionGroups+1)
	for i := range many {
		many[i] = fmt.Sprintf(`{"group":"g%d"}`, i)
	}
	_, err = z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": "[" + strings.Join(many, ",") + "]"})
	requireSpecError(t, err, 400, "params.animals", "at most 200 groups")

	_, err = z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":"zebra","x":1}]`})
	requireSpecError(t, err, 400, "params.animals", "a group is")

	// Options whose where reads their own param are fine with values, but a
	// group of them would need the options to read themselves.
	selfish := withGroup(`[{"field":"species"}],"where":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"animals"}]}`)
	res := z.run(t, "ann", selfish, Options{}, map[string]string{"animals": `["` + z.id("leo") + `"]`})
	requireOrderedRows(t, selfish, res, "note", "f1", "f2", nil)
	_, err = z.try(t, "ann", selfish, Options{}, map[string]string{"animals": `[{"group":"zebra"}]`})
	requireSpecError(t, err, 400, "params.0.optionsFrom.where", "read a group picked from them")
}
