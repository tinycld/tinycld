package reports

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)

// ruleSelect returns the SELECT of the rows of col that the reader's list
// rule allows, built by PocketBase's own rule resolver exactly as the list
// API builds it: the rule's expression, the joins it needs (a back-relation,
// an @collection reference) and DISTINCT when it joined. The compiler puts it
// in a CTE, and every reference to the collection reads that CTE.
//
// A null rule (superusers only) selects nothing: a report never runs as a
// superuser. An empty rule (public) selects every row.
func ruleSelect(app core.App, col *core.Collection, info *core.RequestInfo) (string, dbx.Params, error) {
	q := app.DB().Select("{{" + col.Name + "}}.*").From(col.Name)
	switch {
	case col.ListRule == nil:
		q.AndWhere(dbx.NewExp("1=0"))
	case *col.ListRule != "":
		resolver := core.NewRecordFieldResolver(app, col, info, true)
		expr, err := search.FilterData(*col.ListRule).BuildExpr(resolver)
		if err != nil {
			return "", nil, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
		q.AndWhere(expr)
		if err := resolver.UpdateQuery(q); err != nil {
			return "", nil, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
	}
	built := q.Build()
	if !q.Info().Distinct {
		return built.SQL(), built.Params(), nil
	}
	// A rule that joined is a DISTINCT select, which SQLite cannot read in
	// place: it copies the rows first, and a subquery run for every report
	// row then reads that copy whole, with no index of the table. The ids
	// the rule allows, read once, keep the same rows (id is the key) and
	// leave the table itself to be read through its indexes. The unary +
	// keeps SQLite from driving a read by that id list, which would visit
	// every allowed row where an index of the table finds the few it needs.
	table := app.DB().QuoteSimpleTableName(col.Name)
	id := app.DB().QuoteSimpleColumnName("id")
	return fmt.Sprintf("SELECT %s.* FROM %s WHERE +%s.%s IN (SELECT %s FROM (%s))", table, table, table, id, id, built.SQL()), built.Params(), nil
}

// mergeParams copies src into dst for a statement that holds several rule
// CTEs. Each rule is built with its own {:p0}, so the same name with another
// value would silently change one rule; it is an error instead.
func mergeParams(dst, src dbx.Params) error {
	for k, v := range src {
		if old, ok := dst[k]; ok && !reflect.DeepEqual(old, v) {
			return fmt.Errorf("query parameter %q is set twice with different values", k)
		}
		dst[k] = v
	}
	return nil
}

// listRequestInfo is the request a list rule sees for a report: the reader,
// the request's headers, and a GET with no query or body, as a list request
// from the same client would carry.
func listRequestInfo(auth *core.Record, headers map[string]string) *core.RequestInfo {
	if headers == nil {
		headers = map[string]string{}
	}
	return &core.RequestInfo{
		Auth:    auth,
		Headers: headers,
		Query:   map[string]string{},
		Body:    map[string]any{},
		Method:  http.MethodGet,
		Context: core.RequestInfoContextDefault,
	}
}
