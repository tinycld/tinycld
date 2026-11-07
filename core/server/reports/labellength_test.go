package reports

import (
	"strings"
	"testing"
)

// cjk is n characters of three bytes each: a label is counted in characters,
// as the client's spec schema counts it, not in bytes.
func cjk(n int) string { return strings.Repeat("帳", n) }

// Every label limit counts characters: 200 CJK characters (600 bytes) are
// accepted, 201 are refused.
func TestALabelLimitCountsCharactersNotBytes(t *testing.T) {
	app := withZooRegistry(t)
	column := func(label string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[{"id":"note","label":"` + label + `","path":[{"field":"note"}]}]}`
	}
	opening := func(label string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"animal","opening":{"label":"` + label + `"}}]}`
	}
	layout := func(title string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species"}],
			"layout":[{"section":"cats","title":"` + title + `","key":"lion"}]}`
	}
	cases := map[string]struct {
		spec     func(string) string
		at, want string
	}{
		"column label":  {column, "columns.0", "a label has at most 200 characters"},
		"opening label": {opening, "groups.0.opening.label", "an opening row has a label of 1 to 200 characters"},
		"layout title":  {layout, "layout.0", "a title or label has at most 200 characters"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(app, loadSpec(t, c.spec(cjk(maxLabel)))); err != nil {
				t.Fatalf("200 characters refused: %v", err)
			}
			requireSpecError(t, Validate(app, loadSpec(t, c.spec(cjk(maxLabel+1)))), 400, c.at, c.want)
		})
	}
	t.Run("group pick text", func(t *testing.T) {
		region := Path{{Field: "region"}}
		if err := checkParamDefs([]ParamDef{groupedTeams("{group}"+cjk(maxLabel-len("{group}")), region)}); err != nil {
			t.Fatalf("200 characters refused: %v", err)
		}
		requireSpecError(t, checkParamDefs([]ParamDef{groupedTeams("{group}"+cjk(maxLabel), region)}), 400,
			"params.0.optionsFrom.groupLabel", "at most 200")
	})
}
