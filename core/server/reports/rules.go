package reports

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
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
//
// ruleSelect is the rule in full. A statement of a run holds a rule that one
// answer serves as every row or no row (ruleFormOf, ruleForm.sent).
func ruleSelect(app core.App, col *core.Collection, info *core.RequestInfo) (string, dbx.Params, error) {
	f, err := ruleFormOf(app, col, info)
	if err != nil {
		return "", nil, err
	}
	return f.sql, f.params, nil
}

// ruleForm is a collection's rule select in full (ruleSelect) and, for a rule
// that one answer serves, the statement that gives the answer.
type ruleForm struct {
	ruleRead
	// probe is "SELECT EXISTS (<the rule on a one-row table>)", bound by the
	// rule's params: 1 when the reader's rule passes. It is set only for a
	// rule that reads no row, joined, and does not name the collection's own
	// table; every other rule has none and stays in its statement.
	probe string
	// table is the collection's table, quoted.
	table string
}

// ruleAsk answers a rule's probe for the statement being written: true when
// the reader's rule passes.
type ruleAsk func(probe string, params dbx.Params) (bool, error)

// sent is the rule select a statement holds. A rule with a probe is asked one
// time, outside the statement, and the statement reads every row of the table
// or none: SQLite copies a CTE at each reference, so a rule left in a
// statement that reads its CTE thousands of times is prepared thousands of
// times. With no ask (a caller that only builds), or for a rule with no
// probe, the statement holds the rule in full. An ask that fails gives no
// select: a reader is never shown rows on a failed answer.
func (f ruleForm) sent(ask ruleAsk) (ruleRead, error) {
	if f.probe == "" || ask == nil {
		return f.ruleRead, nil
	}
	ok, err := ask(f.probe, f.params)
	if err != nil {
		return ruleRead{}, err
	}
	sql := fmt.Sprintf("SELECT %s.* FROM %s", f.table, f.table)
	if !ok {
		sql += " WHERE 0"
	}
	return ruleRead{sql: sql, params: dbx.Params{}}, nil
}

// ruleFormOf builds the list rule of col for the reader of info.
func ruleFormOf(app core.App, col *core.Collection, info *core.RequestInfo) (ruleForm, error) {
	q := app.DB().Select("{{" + col.Name + "}}.*").From(col.Name)
	switch {
	case col.ListRule == nil:
		q.AndWhere(dbx.NewExp("1=0"))
	case *col.ListRule != "":
		resolver := &ruleNames{RecordFieldResolver: core.NewRecordFieldResolver(app, col, info, true)}
		expr, err := search.FilterData(*col.ListRule).BuildExpr(resolver)
		if err != nil {
			return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
		if resolver.readsNoRow() {
			// The joins of the rule hang on a one-row table in place of the
			// collection, and EXISTS keeps a row once however many joined rows
			// meet the rule, as DISTINCT did.
			inner := app.DB().Select("(1)").From("(SELECT 1) __rule").AndWhere(expr)
			if err := resolver.UpdateQuery(inner); err != nil {
				return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
			}
			if inner.Info().Distinct {
				inner.Distinct(false)
				built := inner.Build()
				table := app.DB().QuoteSimpleTableName(col.Name)
				form := ruleForm{ruleRead: ruleRead{
					sql:    fmt.Sprintf("SELECT %s.* FROM %s WHERE EXISTS (%s)", table, table, built.SQL()),
					params: built.Params(),
				}}
				// The two shapes of readsNoRow that still read the row name
				// the collection's table in the filter; they keep the EXISTS
				// form, which SQLite runs for each row.
				if !namesTable(built.SQL(), col.Name) {
					form.probe = "SELECT EXISTS (" + built.SQL() + ")"
					form.table = table
				}
				return form, nil
			}
		}
		// A rule that reads no row and joined nothing reaches here after the
		// first UpdateQuery, which added no join, so nothing is added twice.
		q.AndWhere(expr)
		if err := resolver.UpdateQuery(q); err != nil {
			return ruleForm{}, fmt.Errorf("the list rule of %q cannot be built: %w", col.Name, err)
		}
	}
	built := q.Build()
	if !q.Info().Distinct {
		return ruleForm{ruleRead: ruleRead{sql: built.SQL(), params: built.Params()}}, nil
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
	return ruleForm{ruleRead: ruleRead{
		sql:    fmt.Sprintf("SELECT %s.* FROM %s WHERE +%s.%s IN (SELECT %s FROM (%s))", table, table, table, id, id, built.SQL()),
		params: built.Params(),
	}}, nil
}

// namesTable reports whether sql names the table as a whole word, quoted or
// not. A longer name that holds it (zoo_ledger in zoo_ledger_access) is not
// the table.
func namesTable(sql, table string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(table) + `\b`).MatchString(sql)
}

// ruleAnswer runs a rule's probe through db and returns its answer; a test
// wraps it to see what a run asks.
var ruleAnswer = func(ctx context.Context, db dbx.Builder, probe string, params dbx.Params) (bool, error) {
	var ok bool
	err := db.NewQuery(probe).Bind(params).WithContext(ctx).Row(&ok)
	return ok, err
}

// askRules is the ruleAsk of the statements of one run: each distinct probe
// is run one time through db, under the run time limit, and answers holds
// what it gave. The answers belong to the one run that made the map (one
// reader, one read transaction): no run reads another's. With no map, each
// statement asks for itself.
func askRules(ctx context.Context, db dbx.Builder, answers map[string]bool) ruleAsk {
	if answers == nil {
		answers = map[string]bool{}
	}
	return func(probe string, params dbx.Params) (bool, error) {
		key := probeKey(probe, params)
		if ok, asked := answers[key]; asked {
			return ok, nil
		}
		ctx, cancel := context.WithTimeout(ctx, runLimits.timeout)
		defer cancel()
		ok, err := ruleAnswer(ctx, db, probe, params)
		if err != nil {
			return false, mapRunError(err)
		}
		answers[key] = ok
		return ok, nil
	}
}

var probeParam = regexp.MustCompile(`\{:(\w+)\}`)

// probeKey names a probe by its text and the values it binds, in the order
// the text reads them. PocketBase gives a rule's placeholders new names at
// every build, so the names are numbered here: the collections of one package
// that share a rule text then share one answer, and two readers never do (the
// reader's id is one of the values).
func probeKey(probe string, params dbx.Params) string {
	index := map[string]int{}
	var values []any
	text := probeParam.ReplaceAllStringFunc(probe, func(tok string) string {
		name := tok[2 : len(tok)-1]
		i, ok := index[name]
		if !ok {
			i = len(index)
			index[name] = i
			values = append(values, params[name])
		}
		return "{:" + strconv.Itoa(i) + "}"
	})
	return text + "\x00" + fmt.Sprintf("%#v", values)
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

// unionSelect is a union source's rule CTE in full: each part's rule select
// (ruleSelect, the part's own list rule), projected to the source's keys and
// joined by UNION ALL. A key a part does not map reads NULL there; a part's
// constants are bound, under names that hold the source key, so two unions in
// one statement never bind one name twice.
func unionSelect(app core.App, src *Source, info *core.RequestInfo) (string, dbx.Params, error) {
	full, _, err := unionForms(app, src, info, nil)
	return full.sql, full.params, err
}

// unionForms is a union source's rule CTE in full (unionSelect) and as a
// statement holds it: each part's rule as ruleForm.sent writes it for ask.
func unionForms(app core.App, src *Source, info *core.RequestInfo, ask ruleAsk) (full, sent ruleRead, err error) {
	keys := src.unionKeys()
	full.params, sent.params = dbx.Params{}, dbx.Params{}
	fulls := make([]string, 0, len(src.Parts))
	sents := make([]string, 0, len(src.Parts))
	for i, part := range src.Parts {
		col, err := app.FindCachedCollectionByNameOrId(part.Collection)
		if err != nil {
			return ruleRead{}, ruleRead{}, fmt.Errorf("reports: collection %q: %w", part.Collection, err)
		}
		form, err := ruleFormOf(app, col, info)
		if err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		partSent, err := form.sent(ask)
		if err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		if err := mergeParams(full.params, form.params); err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		if err := mergeParams(sent.params, partSent.params); err != nil {
			return ruleRead{}, ruleRead{}, err
		}
		fulls = append(fulls, "SELECT "+unionPartCols(src, i, keys, full.params)+" FROM ("+form.sql+") p")
		sents = append(sents, "SELECT "+unionPartCols(src, i, keys, sent.params)+" FROM ("+partSent.sql+") p")
	}
	full.sql = strings.Join(fulls, "\nUNION ALL\n")
	sent.sql = strings.Join(sents, "\nUNION ALL\n")
	return full, sent, nil
}

// unionPartCols is the select list of part i of a union source, read as p:
// the row's id, then each of the source's keys. A constant is bound in params.
func unionPartCols(src *Source, i int, keys []string, params dbx.Params) string {
	part := src.Parts[i]
	stem := "u_" + strings.ReplaceAll(src.Key, ".", "_")
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
	return strings.Join(cols, ", ")
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
