package reports

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

// groupRowCellsPath is the case table core/reports/lib/__tests__/rows.test.ts
// runs through visibleRows. The same file here keeps exportRows and the view
// from drifting: a group-only column's cell shows on the same rows in a file
// as on screen.
const groupRowCellsPath = "../../reports/lib/fixtures/group-row-cells.json"

type groupRowCase struct {
	Name    string
	Columns []ResultColumn
	Groups  []Group
	Layout  []LayoutItem
	Rows    []Row
	Shown   map[string][]string
}

func TestExportRows_GroupOnlyCases(t *testing.T) {
	raw, err := os.ReadFile(groupRowCellsPath)
	if err != nil {
		t.Fatalf("read %s: %v", groupRowCellsPath, err)
	}
	var table struct{ Cases []groupRowCase }
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("decode %s: %v", groupRowCellsPath, err)
	}
	if len(table.Cases) == 0 {
		t.Fatalf("%s has no cases", groupRowCellsPath)
	}
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			spec := &Spec{Groups: c.Groups, Layout: c.Layout}
			got := map[string][]string{}
			for _, r := range exportRows(c.Rows, spec, c.Columns) {
				keys := make([]string, 0, len(r.Cells))
				for key := range r.Cells {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				got[r.Row.ID] = keys
			}
			if !reflect.DeepEqual(got, c.Shown) {
				t.Fatalf("cell keys by row:\n got  %v\n want %v", got, c.Shown)
			}
		})
	}
}
