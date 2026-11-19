package reports

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)

// ruleSelect returns the SELECT of the rows of col that the reader's list
// rule allows, from the expression and joins of PocketBase's own rule
// resolver. The list API builds the rule as that expression, the joins it
// needs (a back-relation, an @collection reference) and DISTINCT when it
// joined; the paragraphs below name the form core writes for a rule that
// joined. The compiler puts it in a CTE, and every reference to the
// collection reads that CTE.
//
// A null rule (superusers only) selects nothing: a report never runs as a
// superuser. An empty rule (public) selects every row.
//
// A rule that joins is written in one of two forms. A rule that reads no row
// of the collection (every name it resolves is the request's or another
// collection's, as a grant rule is) is one EXISTS filter with the rule's
// joins on a one-row table in place of the collection: SQLite computes it one
// time for the statement and reads the collection through its indexes, where
// the id list of such a rule is every row or none, read whole for each read
// of the CTE. A comparison with every joined row (no "?") carries a subquery
// on the row's id, which the EXISTS form runs for each row it reads. A rule
// that joins and reads its row (a column of the row, a relation path, a
// back-relation) keeps the id-list form below. A rule that joins nothing is
// a plain condition in place.
func ruleSelect(app core.App, col *core.Collection, info *core.RequestInfo) (string, dbx.Params, error) {
	q := app.DB().Select("{{" + col.Name + "}}.*").From(col.Name)
	switch {
	case col.ListRule == nil:
		q.AndWhere(dbx.NewExp("1=0"))
	case *col.ListRule != "":
		resolver := &ruleNames{RecordFieldResolver: core.NewRecordFieldResolver(app, col, info, true)}
		expr, err := search.FilterData(*col.ListRule).BuildExpr(resolver)
		if err != nil {
			return "", nil, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
		if resolver.readsNoRow() {
			// The joins of the rule hang on a one-row table in place of the
			// collection, and EXISTS keeps a row once however many joined rows
			// meet the rule, as DISTINCT did.
			inner := app.DB().Select("(1)").From("(SELECT 1) __rule").AndWhere(expr)
			if err := resolver.UpdateQuery(inner); err != nil {
				return "", nil, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
			}
			if inner.Info().Distinct {
				inner.Distinct(false)
				built := inner.Build()
				table := app.DB().QuoteSimpleTableName(col.Name)
				return fmt.Sprintf("SELECT %s.* FROM %s WHERE EXISTS (%s)", table, table, built.SQL()), built.Params(), nil
			}
		}
		// A rule that reads no row and joined nothing reaches here after the
		// first UpdateQuery, which added no join, so nothing is added twice.
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

// ruleNames is PocketBase's rule resolver, recording each name of the rule
// that it resolves. The filter builder asks it for every identifier of the
// rule, one at a time, so "@collection.x.f ?= col" records col by itself. It
// also asks for true, false and null, which the resolver refuses (the
// builder then uses its own constant): a refused name is not a name of the
// row and is not recorded.
type ruleNames struct {
	*core.RecordFieldResolver
	names []string
}

func (r *ruleNames) Resolve(field string) (*search.ResolverResult, error) {
	res, err := r.RecordFieldResolver.Resolve(field)
	if err == nil && res != nil && res.Identifier != "" {
		r.names = append(r.names, field)
	}
	return res, err
}

// readsNoRow reports whether the rule names only the request and other
// collections: no column of the collection's own row, no relation path and
// no back-relation. Such a rule gives one answer for every row. Two shapes
// pass and still read the row: a comparison with every joined row (no "?")
// reads its id, and "@request.body.x:changed" is built by PocketBase's own
// inner resolver. Their rows stay right, because the EXISTS form may read
// the outer row; it is then run for each row.
func (r *ruleNames) readsNoRow() bool {
	for _, name := range r.names {
		if !strings.HasPrefix(name, "@request.") && !strings.HasPrefix(name, "@collection.") {
			return false
		}
	}
	return true
}

// unionSelect is a union source's rule CTE: each part's rule select
// (ruleSelect, the part's own list rule), projected to the source's keys and
// joined by UNION ALL. A key a part does not map reads NULL there; a part's
// constants are bound, under names that hold the source key, so two unions in
// one statement never bind one name twice.
func unionSelect(app core.App, src *Source, info *core.RequestInfo) (string, dbx.Params, error) {
	keys := src.unionKeys()
	stem := "u_" + strings.ReplaceAll(src.Key, ".", "_")
	params := dbx.Params{}
	selects := make([]string, 0, len(src.Parts))
	for i, part := range src.Parts {
		col, err := app.FindCachedCollectionByNameOrId(part.Collection)
		if err != nil {
			return "", nil, fmt.Errorf("reports: collection %q: %w", part.Collection, err)
		}
		ruleSQL, ruleParams, err := ruleSelect(app, col, info)
		if err != nil {
			return "", nil, err
		}
		if err := mergeParams(params, ruleParams); err != nil {
			return "", nil, err
		}
		cols := make([]string, 0, len(keys)+1)
		cols = append(cols, "[[p.id]] AS [[id]]")
		for _, key := range keys {
			column, mapped := part.Columns[key]
			value, constant := part.Values[key]
			switch {
			case mapped:
				cols = append(cols, fmt.Sprintf("[[p.%s]] AS [[%s]]", column, key))
			case constant:
				name := fmt.Sprintf("%s_%d_%s", stem, i, key)
				params[name] = value
				cols = append(cols, fmt.Sprintf("{:%s} AS [[%s]]", name, key))
			default:
				cols = append(cols, fmt.Sprintf("NULL AS [[%s]]", key))
			}
		}
		selects = append(selects, "SELECT "+strings.Join(cols, ", ")+" FROM ("+ruleSQL+") p")
	}
	return strings.Join(selects, "\nUNION ALL\n"), params, nil
}

// mergeParams copies src into dst for a statement that holds several rule
// CTEs. PocketBase picks the placeholder names of a rule's filter literals
// itself, so two rule builds must never bind one name to different values: the
// later value would silently change the earlier rule. That is an error instead.
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
