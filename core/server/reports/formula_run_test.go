package reports

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func hasAggregate(n *Node) bool {
	if n == nil {
		return false
	}
	return aggregateOp(n) || slices.ContainsFunc(n.Args, hasAggregate)
}

// colsOnlyInAggregates is true when every {col} leaf is inside an aggregate:
// a total row has no single value for a column outside one.
func colsOnlyInAggregates(n *Node, inside bool) bool {
	if n == nil {
		return true
	}
	if n.Col != "" && !inside {
		return false
	}
	inside = inside || aggregateOp(n)
	for _, a := range n.Args {
		if !colsOnlyInAggregates(a, inside) {
			return false
		}
	}
	return true
}

// sameCell compares a result cell with a table expectation. A result holds
// true or false for a Bool cell; SQLite returns 1 or 0.
func sameCell(got, want any) bool {
	if b, ok := got.(bool); ok {
		if w, ok := want.(bool); ok {
			return b == w
		}
		got = int64(0)
		if b {
			got = int64(1)
		}
	}
	return sameValue(got, want)
}

// tableSource gives a table case its own collection with a column for every
// typed name, and the source that reads it.
func (z *zoo) tableSource(t *testing.T, n int, c formulaCase, fiscal *int) (source string, names []string) {
	t.Helper()
	rows := c.Rows
	if c.Values != nil {
		rows = []map[string]any{c.Values}
	}
	collection := fmt.Sprintf("zoo_case%d", n)
	fields, names := z.caseTable(t, collection, c.Types, rows)
	source = fmt.Sprintf("zoo.case%d", n)
	var links []Link
	if c.RecordTypes != nil {
		links = append(links, z.recordLink(t, n, c, collection))
	}
	RegisterSource(Source{
		Key: source, Collection: collection, Label: "Case", Fields: fields, Links: links,
		Currency:         func(core.App) string { return "USD" },
		FiscalStartMonth: func(core.App) int { return *fiscal },
	})
	return source, names
}

// recordLink gives a case's record its own collection and source, and every
// row of the case's table a link "rec" to it, so the case's record leaves
// read it through a group by that link.
func (z *zoo) recordLink(t *testing.T, n int, c formulaCase, table string) Link {
	t.Helper()
	collection := fmt.Sprintf("zoo_case%d_record", n)
	fields, _ := z.caseTable(t, collection, c.RecordTypes, []map[string]any{c.Record})
	source := fmt.Sprintf("zoo.case%d_record", n)
	RegisterSource(Source{Key: source, Collection: collection, Label: "Case record", Fields: fields})
	for _, stmt := range []string{
		fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `rec`", table),
		fmt.Sprintf("UPDATE `%s` SET `rec` = 'row0'", table),
	} {
		if _, err := z.app.DB().NewQuery(stmt).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	return Link{Key: "rec", Label: "Record", Target: source, Cardinality: ToOne, Field: "rec"}
}

// lookupSource gives a case's lookupRows their own collection and source,
// which the case's lookups read in place of "table".
func (z *zoo) lookupSource(t *testing.T, n int, c formulaCase) string {
	t.Helper()
	collection := fmt.Sprintf("zoo_case%d_lookup", n)
	fields, _ := z.caseTable(t, collection, c.LookupTypes, c.LookupRows)
	source := fmt.Sprintf("zoo.case%d_lookup", n)
	RegisterSource(Source{Key: source, Collection: collection, Label: "Case lookup", Fields: fields})
	return source
}

// caseTable makes a collection that lists to everyone, with a column for
// every typed name, holding rows. PocketBase columns are NOT NULL, so the
// columns are added to the bare table to hold the blanks the table's cases
// test.
func (z *zoo) caseTable(t *testing.T, collection string, types map[string]FieldType, rows []map[string]any) (fields []Field, names []string) {
	t.Helper()
	col := core.NewBaseCollection(collection)
	col.ListRule = rule("")
	z.save(t, col)
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	fields = make([]Field, 0, len(names))
	for _, name := range names {
		stmt := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s`", collection, name)
		if _, err := z.app.DB().NewQuery(stmt).Execute(); err != nil {
			t.Fatal(err)
		}
		fields = append(fields, Field{Key: name, Label: name, Type: types[name]})
	}
	for i, row := range rows {
		params := dbx.Params{"id": fmt.Sprintf("row%d", i)}
		cols, slots := []string{"id"}, []string{"{:id}"}
		for j, name := range names {
			key := fmt.Sprintf("v%d", j)
			params[key] = jsonValue(row[name])
			cols = append(cols, "`"+name+"`")
			slots = append(slots, "{:"+key+"}")
		}
		stmt := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)", collection, strings.Join(cols, ", "), strings.Join(slots, ", "))
		if _, err := z.app.DB().NewQuery(stmt).Bind(params).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	return fields, names
}

// The shared formula table, through the production Run: a case with one row
// is a detail row's formula (rowsCtx); a case that totals rows is a total
// row's formula (levelCtx) over the rows it lists; a case that reads a record
// is both, on the group row of its record and on the row under it.
func TestFormulaCasesThroughRun(t *testing.T) {
	z := newZoo(t)
	fiscal := 1
	detailRuns, totalRuns, recordRuns := 0, 0, 0
	for n, c := range loadFormulaCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			fiscal = max(c.FiscalStart, 1)
			source, names := z.tableSource(t, n, c, &fiscal)
			node, err := json.Marshal(c.Formula)
			if err != nil {
				t.Fatal(err)
			}
			if c.LookupTypes != nil {
				node = bytes.ReplaceAll(node, []byte(`"source":"table"`), []byte(`"source":"`+z.lookupSource(t, n, c)+`"`))
			}
			columns := make([]string, 0, len(names)+1)
			for _, name := range names {
				columns = append(columns, fmt.Sprintf(`{"id":%q,"path":[{"field":%q}]}`, name, name))
			}
			columns = append(columns, `{"id":"zz_out","formula":`+string(node)+`}`)
			raw := fmt.Sprintf(`{"version":1,"source":%q,"columns":[%s]}`, source, strings.Join(columns, ","))

			if c.RecordTypes != nil {
				recordRuns++
				columns = append(columns, `{"id":"zz_rec","path":[{"link":"rec"}]}`)
				raw = fmt.Sprintf(`{"version":2,"source":%q,"columns":[%s],"groups":[{"id":"g","col":"zz_rec"}]}`, source, strings.Join(columns, ","))
				res := z.run(t, "ann", raw, Options{Expand: [][]any{{"row0"}}}, nil)
				requireRows(t, res, "kind", "group", "detail", "total")
				for _, row := range res.Rows[:2] {
					if got := row.Cells["zz_out"]; !sameCell(got, c.Expect) {
						t.Fatalf("%s row: got %#v, want %v", row.Kind, got, c.Expect)
					}
				}
				return
			}
			if c.Values != nil && c.Error == "" {
				detailRuns++
				res := z.run(t, "ann", raw, Options{}, nil)
				requireRows(t, res, "kind", "detail", "total")
				if got := res.Rows[0].Cells["zz_out"]; !sameCell(got, c.Expect) {
					t.Fatalf("detail row: got %#v, want %v", got, c.Expect)
				}
			}
			if !hasAggregate(c.Formula) {
				if c.Rows != nil {
					t.Fatalf("a case with several rows has no aggregate to total them")
				}
				return
			}
			if !colsOnlyInAggregates(c.Formula, false) {
				t.Fatalf("a case with an aggregate reads a column outside it; the total row cannot show it")
			}
			totalRuns++
			res, err := z.try(t, "ann", raw, Options{}, nil)
			if c.Error != "" {
				if !errors.Is(err, ErrAmountTooLarge) || !strings.Contains(c.Error, "integer overflow") {
					t.Fatalf("err = %v, want the table's %q", err, c.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			total := res.Rows[len(res.Rows)-1]
			if total.Kind != KindTotal {
				t.Fatalf("last row = %+v", total)
			}
			if got := total.Cells["zz_out"]; !sameCell(got, c.Expect) {
				t.Fatalf("total row: got %#v, want %v", got, c.Expect)
			}
		})
	}
	// Keep in step with the table: the cases with one row, and the cases
	// that total rows.
	if detailRuns != 59 || totalRuns != 12 || recordRuns != 2 {
		t.Fatalf("ran %d detail rows, %d totals and %d records", detailRuns, totalRuns, recordRuns)
	}
}
