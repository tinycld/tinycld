package reports

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/pocketbase/dbx"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// pageStatementOf is the page statement (J1a) a run of the grouped report raw
// sends for user: the one that orders its rows by their paths (__p1).
func (z *zoo) pageStatementOf(t *testing.T, user, raw string, opts Options) string {
	t.Helper()
	seen := observeReads(t)
	z.run(t, user, raw, opts, nil)
	observeRead = nil
	for _, st := range *seen {
		if strings.Contains(st.sql, "__p1") {
			return st.sql
		}
	}
	t.Fatalf("no page statement among %d reads", len(*seen))
	return ""
}

var cteName = regexp.MustCompile(`(?m)(?:^WITH |,\n)(\w+) AS (?:MATERIALIZED )?\(`)

// ctesOf lists the names of a statement's CTEs.
func ctesOf(sql string) []string {
	var out []string
	for _, m := range cteName.FindAllStringSubmatch(sql, -1) {
		out = append(out, m[1])
	}
	return out
}

// dbxToken matches what dbx rewrites in a statement: {{table}}, [[column]]
// and {:param}.
var dbxToken = regexp.MustCompile(`{{(\w+)}}|\[\[([\w.]+)\]\]|{:(\w+)}`)

// rawStatement rewrites a dbx statement as dbx does for SQLite, and lists its
// arguments in placeholder order.
func rawStatement(tb testing.TB, stmt string, params dbx.Params) (string, []any) {
	tb.Helper()
	var args []any
	out := dbxToken.ReplaceAllStringFunc(stmt, func(tok string) string {
		m := dbxToken.FindStringSubmatch(tok)
		switch {
		case m[1] != "":
			return "`" + m[1] + "`"
		case m[2] != "":
			parts := strings.Split(m[2], ".")
			for i, p := range parts {
				parts[i] = "`" + p + "`"
			}
			return strings.Join(parts, ".")
		}
		v, ok := params[m[3]]
		if !ok {
			tb.Fatalf("no param %s", m[3])
		}
		args = append(args, v)
		return "?"
	})
	return out, args
}

// preparedStmt receives sqlite3_prepare_v2's statement handle. A package
// variable, because the C side writes it through a raw address and a global
// never moves. It is shared, so it is safe only while no test in this
// package calls t.Parallel (none does): two parallel firstRow calls would
// write it at once.
var preparedStmt uintptr

// reprepares is how many times SQLite prepared stmt again after its first
// prepare, on the way to its first row (SQLITE_STMTSTATUS_REPREPARE).
func reprepares(t *testing.T, db *sql.DB, stmt string, args []any) int {
	t.Helper()
	return firstRow(t, db, stmt, args).reprepares
}

// firstRowRun is how one statement reached its first row: the time of its
// prepare, the time of its first step (which includes any second prepare),
// and the number of second prepares.
type firstRowRun struct {
	prepare, step time.Duration
	reprepares    int
}

// firstRow runs stmt (and its arguments) to its first row on one pooled
// connection of the app's read pool, as the run does. The driver does not
// expose a statement's handle, so this reads the connection's sqlite3 handle
// and runs the statement through the SQLite library itself.
func firstRow(t *testing.T, db *sql.DB, stmt string, args []any) firstRowRun {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// The connection reads the current schema first, so a schema change
	// made on another connection cannot be what prepares the statement again.
	if _, err := conn.ExecContext(ctx, "SELECT count(*) FROM sqlite_master"); err != nil {
		t.Fatal(err)
	}
	run := firstRowRun{reprepares: -1}
	err = conn.Raw(func(dc any) error {
		v := reflect.ValueOf(dc).Elem()
		handle := uintptr(v.FieldByName("db").Uint())
		tls := (*libc.TLS)(v.FieldByName("tls").UnsafePointer())
		csql, err := libc.CString(stmt)
		if err != nil {
			return err
		}
		defer libc.Xfree(tls, csql)
		started := time.Now()
		if rc := sqlite3.Xsqlite3_prepare_v2(tls, handle, csql, -1, uintptr(unsafe.Pointer(&preparedStmt)), 0); rc != sqlite3.SQLITE_OK {
			return fmt.Errorf("prepare: %d", rc)
		}
		run.prepare = time.Since(started)
		pstmt := preparedStmt
		defer sqlite3.Xsqlite3_finalize(tls, pstmt)
		for i, a := range args {
			n := int32(i + 1)
			var rc int32
			switch x := a.(type) {
			case nil:
				rc = sqlite3.Xsqlite3_bind_null(tls, pstmt, n)
			case string:
				text, err := libc.CString(x)
				if err != nil {
					return err
				}
				rc = sqlite3.Xsqlite3_bind_text(tls, pstmt, n, text, -1, sqlite3.SQLITE_TRANSIENT)
				libc.Xfree(tls, text)
			case int:
				rc = sqlite3.Xsqlite3_bind_int64(tls, pstmt, n, int64(x))
			case int64:
				rc = sqlite3.Xsqlite3_bind_int64(tls, pstmt, n, x)
			case float64:
				rc = sqlite3.Xsqlite3_bind_double(tls, pstmt, n, x)
			case bool:
				b := int64(0)
				if x {
					b = 1
				}
				rc = sqlite3.Xsqlite3_bind_int64(tls, pstmt, n, b)
			default:
				return fmt.Errorf("argument %d: %T", n, a)
			}
			if rc != sqlite3.SQLITE_OK {
				return fmt.Errorf("bind %d: %d", n, rc)
			}
		}
		started = time.Now()
		if rc := sqlite3.Xsqlite3_step(tls, pstmt); rc != sqlite3.SQLITE_ROW && rc != sqlite3.SQLITE_DONE {
			return fmt.Errorf("step: %d", rc)
		}
		run.step = time.Since(started)
		run.reprepares = int(sqlite3.Xsqlite3_stmt_status(tls, pstmt, sqlite3.SQLITE_STMTSTATUS_REPREPARE, 0))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func readPool(t *testing.T, z *zoo) *sql.DB {
	t.Helper()
	db, ok := z.app.ConcurrentDB().(*dbx.DB)
	if !ok {
		t.Fatalf("read pool is %T", z.app.ConcurrentDB())
	}
	return db.DB()
}

// SQLite marks a statement whose LIMIT is a bound parameter to be prepared
// again once a value is bound (sqlite3ExprIsInteger sets the statement's
// varmask), so binding the limit costs a second prepare at the first step. A
// literal limit does not.
func TestABoundLimitPreparesTheStatementAgain(t *testing.T) {
	z := newZoo(t)
	db := readPool(t, z)
	if n := reprepares(t, db, "SELECT name FROM zoo_animals ORDER BY name LIMIT ?", []any{int64(3)}); n != 1 {
		t.Fatalf("bound limit: %d reprepares, want 1", n)
	}
	if n := reprepares(t, db, "SELECT name FROM zoo_animals ORDER BY name LIMIT 3", nil); n != 0 {
		t.Fatalf("literal limit: %d reprepares, want 0", n)
	}
}

// wideSpec is TestManyRecomputeTotalsOverOneParentUnderAHaving's report: 65
// recompute to-many totals and a "rows" column, all of one parent row (the
// site) over its animals.
func wideSpec() string {
	cols := []string{`{"id":"region","path":[{"field":"region"}]}`, `{"id":"name","path":[{"field":"name"}]}`,
		`{"id":"shown","path":[{"link":"animals","mode":"aggregate"},{"field":"weight"}],"aggregate":"sum","total":"rows"}`}
	for i := range 65 {
		cols = append(cols, fmt.Sprintf(`{"id":"w%d","path":[{"link":"animals","mode":"aggregate"},{"field":"weight"}],"aggregate":"sum"}`, i))
	}
	return `{"version":4,"source":"zoo.sites","columns":[` + strings.Join(cols, ",") + `],
		"groups":[{"id":"g","col":"region"},{"id":"h","col":"name","having":{"op":"gt","args":[{"col":"w0"},{"value":200}]}}],
		"sort":[{"col":"region"}]}`
}

// groupStatementOf is the unpaged group statement a run of a pivot, layout
// or nest report raw sends for user: the one that reads level rows (__level)
// and orders them by no path.
func (z *zoo) groupStatementOf(t *testing.T, user, raw string, opts Options) string {
	t.Helper()
	seen := observeReads(t)
	z.run(t, user, raw, opts, nil)
	observeRead = nil
	for _, st := range *seen {
		if strings.Contains(st.sql, "__level") && !strings.Contains(st.sql, "__p1") {
			return st.sql
		}
	}
	t.Fatalf("no unpaged group statement among %d reads", len(*seen))
	return ""
}

// wideLayoutSpec is wideSpec as a layout report (a section per region): its
// group rows are read whole, by the unpaged group statement (review A, NOTE 2).
func wideLayoutSpec() string {
	return strings.Replace(wideSpec(), `"sort":[{"col":"region"}]}`,
		`"sort":[{"col":"region"}],"layout":[{"section":"n","title":"North","key":"north"},{"section":"s","title":"South","key":"south"}]}`, 1)
}

// Every to-many total a level reads of one parent row is in one CTE: the
// wide report's three levels have three, and no level needs its joins split,
// in the page statement and in the unpaged group statement of a layout.
func TestToManyTotalsOfOneParentRowShareOneCTEPerLevel(t *testing.T) {
	z := newZoo(t)
	requireOneTotalsCTEPerLevel(t, z.pageStatementOf(t, "ann", wideSpec(), Options{ExpandAll: true}))
	requireOneTotalsCTEPerLevel(t, z.groupStatementOf(t, "ann", wideLayoutSpec(), Options{ExpandAll: true}))
}

func requireOneTotalsCTEPerLevel(t *testing.T, stmt string) {
	t.Helper()
	var totals, chunks []string
	for _, name := range ctesOf(stmt) {
		switch {
		case strings.HasPrefix(name, "d"):
			totals = append(totals, name)
		case regexp.MustCompile(`^g\d+_\d+$`).MatchString(name):
			chunks = append(chunks, name)
		}
	}
	if len(totals) != 3 || len(chunks) != 0 {
		t.Fatalf("to-many total CTEs %v, split joins %v; want one per level and none split", totals, chunks)
	}
}

// More side CTEs than one select may join (to-many totals of that many
// distinct parent rows) are joined maxSideJoins at a time, and every value
// arrives. The totals of one parent row share a CTE, so a report reaches
// this only with that many parent rows; the join is built directly here.
func TestSidesPastTheJoinLimitAreJoinedInChunks(t *testing.T) {
	z := newZoo(t)
	lv := &level{n: 0, keys: []string{"k0"}}
	ctes := []string{"a0 AS (SELECT 'x' AS k0, 1 AS __n)"}
	var sides []tmCol
	for i := range maxSideJoins + 10 {
		d := fmt.Sprintf("d0_%d", i)
		ctes = append(ctes, fmt.Sprintf("%s AS (SELECT 'x' AS k0, %d AS t%d)", d, i, i))
		sides = append(sides, tmCol{d: d, name: fmt.Sprintf("t%d", i), zero: true})
	}
	joined, g := lv.joinSides("a0", sides)
	if len(joined) != 2 || g != "g0" || !strings.Contains(joined[0], "g0_0 AS MATERIALIZED") {
		t.Fatalf("joins %d, result %s:\n%s", len(joined), g, strings.Join(joined, "\n"))
	}
	row := dbx.NullStringMap{}
	if err := z.app.DB().NewQuery("WITH " + strings.Join(append(ctes, joined...), ",\n") + "\nSELECT * FROM " + g).One(row); err != nil {
		t.Fatal(err)
	}
	for i := range maxSideJoins + 10 {
		if v := row[fmt.Sprintf("t%d", i)]; v.String != fmt.Sprint(i) {
			t.Fatalf("t%d = %v", i, v)
		}
	}
}

// A row's record label (vl, vc) and its carried currency (vcur) are selected
// only for a column some row has them for: the wide report's columns are
// plain numbers, so its statement selects none; a link column keeps its
// label.
func TestHelperColumnsOnlyForColumnsThatHaveThem(t *testing.T) {
	z := newZoo(t)
	const bySite = `{"version":4,"source":"zoo.animals","columns":[
		{"id":"site","path":[{"link":"site"}]},{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"site"}]%s}`
	// The page statement of a grouped report, and the unpaged group statement
	// of a layout and of a pivot (review A, NOTE 2).
	for name, c := range map[string]struct {
		wide, linked string
		statementOf  func(*testing.T, string, string, Options) string
	}{
		"page":  {wideSpec(), fmt.Sprintf(bySite, ""), z.pageStatementOf},
		"group": {wideLayoutSpec(), fmt.Sprintf(bySite, `,"pivot":{"path":[{"field":"species"}],"cell":"w"}`), z.groupStatementOf},
	} {
		stmt := c.statementOf(t, "ann", c.wide, Options{ExpandAll: true})
		if m := regexp.MustCompile(` AS (vl|vc|vcur)\d+\b`).FindString(stmt); m != "" {
			t.Fatalf("%s: the wide report's statement selects %q", name, m)
		}
		linked := c.statementOf(t, "ann", c.linked, Options{ExpandAll: true})
		if !strings.Contains(linked, " AS vl0") || strings.Contains(linked, " AS vl1") {
			t.Fatalf("%s: a link column keeps its label and a number has none:\n%s", name, linked)
		}
	}
}

// readStatement is one statement a run or an options read sent.
type readStatement struct {
	sql    string
	params dbx.Params
}

// observeReads collects every read statement until the test ends.
func observeReads(t *testing.T) *[]readStatement {
	t.Helper()
	var seen []readStatement
	observeRead = func(sql string, params dbx.Params) {
		seen = append(seen, readStatement{sql: sql, params: maps.Clone(params)})
	}
	t.Cleanup(func() { observeRead = nil })
	return &seen
}

// Every statement the engine sends (group, detail, nest, pivot, options and
// option groups), and the EXPLAIN that finds a run's change topics, is
// prepared once. The engine's own SQL binds nothing SQLite plans by; a rule
// CTE is PocketBase's SQL (TestAPocketBaseRuleLikePreparesTheStatementAgain).
func TestEveryReportStatementIsPreparedOnce(t *testing.T) {
	z := newZoo(t)
	db := readPool(t, z)
	seen := observeReads(t)
	z.run(t, "ann", `{"version":4,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`, Options{ExpandAll: true}, nil)
	z.run(t, "ann", wideSpec(), Options{ExpandAll: true}, nil)
	z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"name","path":[{"link":"animal"},{"field":"name"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"name"}],"pivot":{"path":[{"field":"note"}],"cell":"c"}}`, Options{}, nil)
	z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"})
	kinds := map[string]bool{}
	for _, st := range *seen {
		switch {
		case strings.Contains(st.sql, "__parent"):
			kinds["nest"] = true
		case strings.Contains(st.sql, "COUNT(*) OVER () AS __n"):
			kinds["pivot"] = true
		case strings.Contains(st.sql, "GROUP BY o.g"):
			kinds["option groups"] = true
		case strings.Contains(st.sql, "o.l <> ''"):
			kinds["options"] = true
		case strings.Contains(st.sql, "__level"):
			kinds["group"] = true
		default:
			kinds["detail"] = true
		}
		raw, args := rawStatement(t, st.sql, st.params)
		if n := reprepares(t, db, "EXPLAIN "+raw, args); n != 0 {
			t.Errorf("EXPLAIN %d reprepares, want 0:\n%.300s", n, st.sql)
		}
		if n := reprepares(t, db, raw, args); n != 0 {
			t.Errorf("%d reprepares, want 0:\n%.300s", n, st.sql)
		}
	}
	for _, k := range []string{"group", "detail", "nest", "pivot", "options", "option groups"} {
		if !kinds[k] {
			t.Errorf("no %s statement was read (%v)", k, kinds)
		}
	}
}

// A PocketBase list rule with ~ compiles to LIKE with a bound pattern, and
// SQLite plans a LIKE by its pattern's value (isLikeOrGlob sets the
// statement's varmask), so a report statement that holds that rule's CTE is
// prepared again at its first step. The SQL is PocketBase's; this pins the
// one known exception to TestEveryReportStatementIsPreparedOnce.
func TestAPocketBaseRuleLikePreparesTheStatementAgain(t *testing.T) {
	z := newZoo(t)
	sites, err := z.app.FindCollectionByNameOrId("zoo_sites")
	if err != nil {
		t.Fatal(err)
	}
	// A text pattern binds as a parameter (a rule that reads the reader's own
	// field joins it instead, and binds nothing SQLite plans by).
	sites.ListRule = rule(`name ~ "or"`)
	z.save(t, sites)
	sql, params, err := ruleSelect(z.app, sites, listRequestInfo(z.recs["ann"], nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "LIKE") {
		t.Fatalf("the rule compiled without LIKE: %s", sql)
	}
	raw, args := rawStatement(t, "SELECT * FROM ("+sql+")", params)
	if n := reprepares(t, readPool(t, z), raw, args); n != 1 {
		t.Fatalf("a rule LIKE: %d reprepares, want 1\n%s\n%v", n, raw, args)
	}
}
