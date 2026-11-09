package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// A report grouped and sorted by a column it never shows (G7).
const zooHidden = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"sp","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"s","col":"sp","subtotals":true,"open":true},{"id":"a","col":"animal"}],
	"sort":[{"col":"sp","dir":"desc"}]%s}`

func TestHiddenColumnGroupsAndSortsButIsNotInTheResult(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooHidden, ""), Options{}, nil)
	ids := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		ids[i] = c.ID
	}
	if !slices.Equal(ids, []string{"animal", "cost"}) {
		t.Fatalf("columns = %v", ids)
	}
	// The groups and the descending sort still use the hidden species.
	requireRows(t, res, "label", "zebra", "Zed", "Total zebra", "lion", "Leo", "Total lion", "Total")
	// Group rows carry their figures too.
	requireRows(t, res, "cost", int64(1000), int64(1000), int64(1000), int64(3500), int64(3500), int64(3500), int64(4500))
	for _, r := range res.Rows {
		for key := range r.Cells {
			if key == "sp" || strings.HasPrefix(key, "sp@") {
				t.Fatalf("row %s carries the hidden cell %q", r.ID, key)
			}
		}
	}
}

func TestHiddenColumnRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ spec, at, want string }{
		"first column": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}],"hidden":true},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}]}`, "columns.0.hidden", "the first column labels the rows"},
		"display.columns": {fmt.Sprintf(zooHidden, `,"display":{"columns":{"sp":{"minWidth":"auto"}}}`), "display.columns.sp", "a hidden column is never shown"},
		"display.layouts": {fmt.Sprintf(zooHidden, `,"display":{"layouts":[{"width":"mobile","tracks":2,"cells":[{"col":"animal","line":1},{"col":"sp","line":2}]}]}`), "display.layouts.0.cells.1.col", "a hidden column is never shown"},
		"pivot cell": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","hidden":true}],
			"groups":[{"id":"a","col":"animal"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`, "columns.1.hidden", "a pivot's cell column shows as the pivot's columns"},
		// The first column of each level of child rows labels those rows.
		"first nested column": {strings.Replace(sitesWithAnimals, `{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"name"}]}`,
			`{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"name"}],"hidden":true}`, 1), "columns.1.hidden", "labels the child rows"},
		"first column of a deeper nest": {strings.Replace(sitesWithAnimals, `{"field":"fed"}]}`, `{"field":"fed"}],"hidden":true}`, 1),
			"columns.3.hidden", "labels the child rows"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, c.at, c.want)
		})
	}
}

func TestViewerSortNamingAHiddenColumnIsRefused(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", fmt.Sprintf(zooHidden, ""), Options{Sort: []ViewSort{{Col: "sp"}}}, nil)
	requireSpecError(t, err, 422, "options.sort.0.col", "sp")
}

func TestHiddenColumnOpeningCellIsRefused(t *testing.T) {
	z := newZoo(t)
	spec := strings.Replace(gramsByAnimal(`{"grams":`+beforePeriod+`}`, ""),
		`{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}`,
		`{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","hidden":true}`, 1)
	s, _, err := Load([]byte(spec))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	requireSpecError(t, Validate(z.app, s), 400, "groups.0.opening.cells.grams", "a hidden column is never shown")
}

func TestHiddenNestedColumnLeavesOutItsCells(t *testing.T) {
	z := newZoo(t)
	spec := strings.Replace(sitesWithAnimals, `{"id":"owner","path":[{"link":"animals","mode":"nest"},{"link":"owner"},{"field":"name"}]}`,
		`{"id":"owner","path":[{"link":"animals","mode":"nest"},{"link":"owner"},{"field":"name"}],"hidden":true}`, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	for _, r := range res.Rows {
		for key := range r.Cells {
			if key == "owner" || strings.HasPrefix(key, "owner@") {
				t.Fatalf("row %s carries the hidden cell %q", r.ID, key)
			}
		}
	}
}
