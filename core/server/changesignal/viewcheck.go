package changesignal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// CheckViews compiles every registered view with SQLite's EXPLAIN and fails
// when its bytecode opens a table, or an index of a table, that the view's
// registration does not cover. Each OpenRead opcode names a root page, and
// sqlite_schema maps every root page to its table, so a read hidden in a
// subquery, an EXISTS, a CTE or a covering index is still seen. A view that
// reads a stored virtual table must declare the collections
// RegisterVirtualTable maps it to (see the package comment). Call it from a
// package's tests after the package's migrations apply.
func CheckViews(app core.App) error {
	registered := RegisteredViews()
	if len(registered) == 0 {
		return nil
	}
	s, err := readSchema(context.Background(), app)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	for _, name := range names {
		problems = append(problems, checkView(app, s, name, registered[name])...)
	}
	if len(problems) > 0 {
		return errors.New("changesignal: " + strings.Join(problems, "; "))
	}
	return nil
}

func checkView(app core.App, s *schema, view string, sources []string) []string {
	col, err := app.FindCollectionByNameOrId(view)
	if err != nil {
		return []string{fmt.Sprintf("view %q is not a collection", view)}
	}
	if !col.IsView() {
		return []string{fmt.Sprintf("view %q is a %s collection, not a view", view, col.Type)}
	}
	// Topics and writes are matched by collection name: a view keyed by an
	// id or another spelling signals a topic no client subscribes, and such
	// a source never matches a write.
	var problems []string
	if col.Name != view {
		problems = append(problems, fmt.Sprintf("view %q is not registered by its collection name %q", view, col.Name))
	}
	for _, source := range sources {
		sourceCol, err := app.FindCollectionByNameOrId(source)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("view %q declares %q, which is not a collection", view, source))
		case sourceCol.Name != source:
			problems = append(problems, fmt.Sprintf("view %q declares %q, which is not the collection name %q", view, source, sourceCol.Name))
		}
	}
	read, err := tablesRead(context.Background(), app, s, "SELECT * FROM {{"+col.Name+"}}", nil)
	if err != nil {
		return append(problems, fmt.Sprintf("view %q: %v", view, err))
	}
	covered := coveredBy(view)
	for _, table := range read {
		if !covered[table] {
			problems = append(problems, fmt.Sprintf("view %q reads %q, which RegisterView does not declare", view, table))
		}
	}
	return problems
}

// tablesRead lists, sorted, the tables of the main database that query
// opens for reading, and the collections of each stored virtual table it
// may read. params binds the query's placeholders; EXPLAIN only compiles the
// statement, so their values do not change the result.
//
// A VOpen opcode names no table. When the query reaches no stored virtual
// table by name (virtualTablesReached), every VOpen is a table-valued
// function such as json_each, which holds no stored rows and adds nothing.
// Otherwise each stored virtual table it reaches must be mapped with
// RegisterVirtualTable, and adds its collections.
func tablesRead(ctx context.Context, app core.App, s *schema, query string, params dbx.Params) ([]string, error) {
	var ops []struct {
		Opcode string `db:"opcode"`
		P2     int64  `db:"p2"`
		P3     int64  `db:"p3"`
	}
	if err := app.DB().NewQuery("EXPLAIN " + query).Bind(params).WithContext(ctx).All(&ops); err != nil {
		return nil, fmt.Errorf("explain: %w", err)
	}
	seen := map[string]bool{}
	opensVirtual := false
	for _, op := range ops {
		switch op.Opcode {
		case "VOpen":
			opensVirtual = true
		case "OpenRead":
			if op.P3 != 0 {
				continue // not the main database
			}
			table, ok := s.pages[op.P2]
			if !ok {
				return nil, fmt.Errorf("opens root page %d, which no table or index owns", op.P2)
			}
			if !strings.HasPrefix(table, "sqlite_") {
				seen[table] = true
			}
		}
	}
	if opensVirtual {
		for _, table := range s.virtualTablesReached(query) {
			collections, err := mappedCollections(app, table)
			if err != nil {
				return nil, err
			}
			for _, collection := range collections {
				seen[collection] = true
			}
		}
	}
	tables := make([]string, 0, len(seen))
	for table := range seen {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables, nil
}

// mappedCollections returns the collections RegisterVirtualTable maps a
// stored virtual table to, each checked to be a collection by its name.
func mappedCollections(app core.App, table string) ([]string, error) {
	collections, ok := virtualTableCollections(table)
	if !ok {
		return nil, fmt.Errorf("reads virtual table %q, which RegisterVirtualTable does not map to collections", table)
	}
	for _, collection := range collections {
		col, err := app.FindCachedCollectionByNameOrId(collection)
		switch {
		case err != nil:
			return nil, fmt.Errorf("virtual table %q maps %q, which is not a collection", table, collection)
		case col.Name != collection:
			return nil, fmt.Errorf("virtual table %q maps %q, which is not the collection name %q", table, collection, col.Name)
		}
	}
	return collections, nil
}
