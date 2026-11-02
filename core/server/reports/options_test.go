package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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
	requireRows(t, res, "note", "f1", "f2", nil)
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["not-an-animal"]`})
	requireRows(t, res, "note", nil)
	all := z.run(t, "ann", animalsParam, Options{}, nil)
	requireRows(t, all, "note", "f1", "f2", "f3", "f4", nil)
	many := make([]string, maxChoiceValues+1)
	for i := range many {
		many[i] = fmt.Sprintf("%q", fmt.Sprint("x", i))
	}
	_, err := z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": "[" + strings.Join(many, ",") + "]"})
	requireSpecError(t, err, 400, "params.animals", "at most 1000")
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
