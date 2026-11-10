package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// The layout benchmark runs a Balance-Sheet-shaped statement and a
// Trial-Balance-shaped baseline over a ledger of a given number of lines,
// listed through the balance fixture's DISTINCT access rule (withBalanceZoo).
// Beside the run time it reports what the statement costs: its size, how
// often it reads the lines' rule CTE, the Go time before the group statement
// is sent, and SQLite's prepare and execution time of that statement alone.
//
//	GOWORK=off go test ./reports/ -run '^$' -bench BenchmarkLayoutStatement -benchtime 1x
//
// LAYOUT_BENCH_LINES=300,3000 picks the sizes (default 300,3000,30000).

// benchAsOf is the As of day of every benchmark run; the prior column reads
// the same day a year earlier.
const benchAsOf = `"2026-06-30"`

// benchDetailTypes give a ledger account its class and kind, as the Books
// account types do: ranks 0 skip the level.
var benchDetailTypes = []struct {
	typ, detail string
}{
	{"asset", "bank"}, {"asset", "receivable"}, {"asset", "other_current"}, {"asset", "fixed"}, {"asset", "other_asset"},
	{"liability", "payable"}, {"liability", "card"}, {"liability", "other_current_liability"}, {"liability", "long_term"},
	{"equity", "equity"},
	{"income", "income"}, {"income", "other_income"}, {"expense", "expense"}, {"expense", "cogs"},
}

// benchPlacement is a detail type's class and kind rank (0: no place).
var benchPlacement = map[string][2]int{
	"bank": {1, 1}, "receivable": {1, 2}, "other_current": {1, 3}, "fixed": {2, 0}, "other_asset": {3, 0},
	"payable": {4, 4}, "card": {4, 5}, "other_current_liability": {4, 6}, "long_term": {5, 0},
}

// withBenchLedger adds lines ledger lines (2 per entry) over accounts and
// entries to the balance fixture, written by SQL in one transaction so the
// largest size sets up in seconds. Dates spread over 2024–2026.
func withBenchLedger(tb testing.TB, z *zoo, lines int) {
	tb.Helper()
	accounts := 30 + lines/200
	err := z.app.RunInTransaction(func(tx core.App) error {
		db := tx.DB()
		for i := range accounts {
			dt := benchDetailTypes[i%len(benchDetailTypes)]
			if _, err := db.Insert("zoo_ledger_accounts", dbx.Params{
				"id": ledgerID("bacc", i), "number": strconv.Itoa(1000 + i*10), "full_name": fmt.Sprintf("Account %03d", i),
				"type": dt.typ, "detail_type": dt.detail, "inactive": false,
			}).Execute(); err != nil {
				return err
			}
		}
		if _, err := db.Insert("zoo_ledger_accounts", dbx.Params{
			"id": ledgerID("bacc", accounts), "number": "3900", "full_name": "Retained earnings",
			"type": "equity", "detail_type": "retained_earnings", "inactive": false,
		}).Execute(); err != nil {
			return err
		}
		for e := range lines / 2 {
			entry := ledgerID("bent", e)
			if _, err := db.Insert("zoo_ledger_entries", dbx.Params{"id": entry, "number": fmt.Sprintf("E%d", e)}).Execute(); err != nil {
				return err
			}
			a, b := e%accounts, (e*7+3)%accounts
			if a == b {
				b = (a + 1) % accounts
			}
			day := fmt.Sprintf("%d-%02d-%02d", 2024+e%3, 1+(e*5)%12, 1+(e*11)%28)
			amount := 100 + (e*37)%5000
			for j, side := range [][3]int{{a, amount, 0}, {b, 0, amount}} {
				if _, err := db.Insert("zoo_ledger_lines", dbx.Params{
					"id": ledgerID("blin", 2*e+j), "entry": entry, "account": ledgerID("bacc", side[0]), "date": day,
					"debit": side[1], "credit": side[2], "currency": "USD", "position": j + 1, "posted": e%13 != 5,
				}).Execute(); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

// benchCase is one detail type's rank at a level, as a case node over the
// account's detail type.
func benchCase(level int) string {
	var args []string
	for _, dt := range benchDetailTypes {
		p, ok := benchPlacement[dt.detail]
		if !ok {
			continue
		}
		v := "null"
		if p[level] != 0 {
			v = strconv.Itoa(p[level])
		}
		args = append(args, `{"op":"eq","args":[{"path":[{"link":"account"},{"field":"detail_type"}]},{"value":"`+dt.detail+`"}]}`, `{"value":`+v+`}`)
	}
	return `{"op":"case","args":[` + strings.Join(append(args, `{"value":null}`), ",") + `]}`
}

// benchBalanceSheet is the Books Balance Sheet's shape on the ledger
// fixture: sections by side, class and kind levels that skip, accounts with
// a having, two day columns, derived Change and % change, Retained earnings
// and Net income inside Equity by sum lookups, and a total of two sections.
func benchBalanceSheet() string {
	asOf := `{"param":"asOf"}`
	prior := `{"op":"addMonths","args":[{"param":"asOf"},{"value":-12}]}`
	typ := `{"path":[{"link":"account"},{"field":"type"}]}`
	signed := `{"op":"if","args":[{"op":"in","args":[` + typ + `,{"value":["liability","equity","income"]}]},
		{"op":"sub","args":[{"path":[{"field":"credit"}]},{"path":[{"field":"debit"}]}]},
		{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}]}`
	pl := `{"value":["income","expense"]}`
	sum := func(day, which string) string {
		return `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"lte","args":[{"path":[{"field":"date"}]},` + day + `]},` + which + `]},
			"value":{"op":"sub","args":[{"path":[{"field":"credit"}]},{"path":[{"field":"debit"}]}]},"pick":"sum"},{"value":0}]}`
	}
	retained := func(day string) string {
		return sum(day, `{"op":"or","args":[
			{"op":"eq","args":[{"path":[{"link":"account"},{"field":"detail_type"}]},{"value":"retained_earnings"}]},
			{"op":"and","args":[{"op":"in","args":[`+typ+`,`+pl+`]},
				{"op":"lt","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[`+day+`]}]}]}]}`)
	}
	netIncome := func(day string) string {
		return sum(day, `{"op":"and","args":[{"op":"in","args":[`+typ+`,`+pl+`]},
			{"op":"gte","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[`+day+`]}]}]}`)
	}
	label := func(level int, names map[int]string) string {
		var args []string
		for r := 1; r <= 6; r++ {
			if n, ok := names[r]; ok {
				args = append(args, fmt.Sprintf(`{"op":"eq","args":[{"col":"%s"},{"value":%d}]}`, map[int]string{1: "cls", 2: "knd"}[level], r), `{"value":"`+n+`"}`)
			}
		}
		return `{"op":"case","args":[` + strings.Join(append(args, `{"value":null}`), ",") + `]}`
	}
	return `{"version":4,"source":"zoo.ledger_lines",
	"params":[{"id":"asOf","type":"asOf","label":"As of"},
		{"id":"rows","type":"choice","label":"Rows","default":"nonzero","options":[{"value":"nonzero","label":"Non-zero"},{"value":"active","label":"Active"}]}],
	"filter":{"op":"and","args":[
		{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
		{"op":"lte","args":[{"path":[{"field":"date"}]},` + asOf + `]},
		{"op":"not","args":[{"op":"in","args":[` + typ + `,` + pl + `]}]},
		{"op":"ne","args":[{"path":[{"link":"account"},{"field":"detail_type"}]},{"value":"retained_earnings"}]}]},
	"columns":[
		{"id":"account","path":[{"link":"account"}]},
		{"id":"current","heading":` + asOf + `,"formula":{"op":"coalesce","args":[{"op":"sum","args":[` + signed + `]},{"value":0}]}},
		{"id":"prior","heading":` + prior + `,"formula":{"op":"coalesce","args":[{"op":"sum","args":[{"op":"if","args":[{"op":"lte","args":[{"path":[{"field":"date"}]},` + prior + `]},` + signed + `,{"value":null}]}]},{"value":0}]}},
		{"id":"change","detail":"blank","formula":{"op":"sub","args":[{"col":"current"},{"col":"prior"}]}},
		{"id":"pct","format":"percent","detail":"blank","formula":{"op":"if","args":[{"op":"eq","args":[{"col":"prior"},{"value":0}]},{"value":null},
			{"op":"mul","args":[{"op":"div","scale":4,"args":[{"col":"change"},{"op":"max","args":[{"col":"prior"},{"op":"sub","args":[{"value":0},{"col":"prior"}]}]}]},{"value":100}]}]}},
		{"id":"side","hidden":true,"formula":{"op":"case","args":[
			{"op":"eq","args":[` + typ + `,{"value":"asset"}]},{"value":"asset"},
			{"op":"eq","args":[` + typ + `,{"value":"liability"}]},{"value":"liability"},
			{"value":"equity"}]}},
		{"id":"cls","hidden":true,"formula":` + benchCase(0) + `},
		{"id":"knd","hidden":true,"formula":` + benchCase(1) + `}],
	"groups":[
		{"id":"section","col":"side","open":true,"subtotals":true},
		{"id":"class","col":"cls","blank":"skip","hideEmpty":true,"open":true,"subtotals":true,
			"label":` + label(1, map[int]string{1: "Current assets", 2: "Fixed assets", 3: "Other assets", 4: "Current liabilities", 5: "Long-term liabilities"}) + `},
		{"id":"kind","col":"knd","blank":"skip","hideEmpty":true,"open":true,"subtotals":true,
			"label":` + label(2, map[int]string{1: "Bank accounts", 2: "Accounts receivable", 3: "Other current assets", 4: "Accounts payable", 5: "Credit cards", 6: "Other current liabilities"}) + `},
		{"id":"byAccount","col":"account","label":{"record":"byAccount","path":[{"field":"full_name"}]},
			"having":{"op":"or","args":[{"op":"eq","args":[{"param":"rows"},{"value":"active"}]},
				{"op":"ne","args":[{"col":"current"},{"value":0}]},{"op":"ne","args":[{"col":"prior"},{"value":0}]}]}}],
	"layout":[
		{"section":"assets","title":"Assets","key":"asset"},
		{"section":"liabilities","title":"Liabilities","key":"liability"},
		{"section":"equity","title":"Equity","key":"equity"},
		{"row":"retainedEarnings","label":"Retained earnings","within":"equity","cells":{"current":` + retained(asOf) + `,"prior":` + retained(prior) + `}},
		{"row":"netIncome","label":"Net income","within":"equity","cells":{"current":` + netIncome(asOf) + `,"prior":` + netIncome(prior) + `}},
		{"row":"totalLE","label":"Total liabilities and equity","formula":{"op":"add","args":[{"subtotal":"liabilities"},{"subtotal":"equity"}]}}],
	"sort":[{"col":"cls"},{"col":"knd"},{"formula":{"record":"byAccount","path":[{"field":"number"}]}},
		{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`
}

// benchLevelsOnly is the Balance Sheet without its layout: its group levels
// alone, so the layout's own cost is the difference.
func benchLevelsOnly() string {
	spec := benchBalanceSheet()
	from := strings.Index(spec, `"layout":[`)
	to := strings.Index(spec, `"sort":[`)
	return spec[:from] + spec[to:]
}

// benchTrialBalance is the Books Trial Balance's shape: one level of
// accounts, debit and credit of the net as of a day.
func benchTrialBalance() string {
	net := `{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}`
	return `{"version":4,"source":"zoo.ledger_lines",
	"params":[{"id":"asOf","type":"asOf","label":"As of"}],
	"filter":{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
		{"op":"lte","args":[{"path":[{"field":"date"}]},{"param":"asOf"}]}]},
	"columns":[
		{"id":"account","path":[{"link":"account"}]},
		{"id":"number","formula":{"record":"byAccount","path":[{"field":"number"}]},"detail":"blank"},
		{"id":"debit","total":"rows","formula":{"op":"if","args":[{"op":"gte","args":[{"op":"sum","args":[` + net + `]},{"value":0}]},{"op":"sum","args":[` + net + `]},{"value":null}]}},
		{"id":"credit","total":"rows","formula":{"op":"if","args":[{"op":"lt","args":[{"op":"sum","args":[` + net + `]},{"value":0}]},{"op":"sub","args":[{"value":0},{"op":"sum","args":[` + net + `]}]},{"value":null}]}}],
	"groups":[{"id":"byAccount","col":"account","label":{"record":"byAccount","path":[{"field":"full_name"}]},
		"having":{"op":"ne","args":[{"op":"sum","args":[` + net + `]},{"value":0}]}}],
	"sort":[{"col":"number"},{"formula":{"path":[{"field":"date"}]}}]}`
}

// benchStatement is what one run sent as its group statement, measured.
type benchStatement struct {
	bytes, ruleReads  int
	goBefore          time.Duration
	prepare, run      time.Duration
	rows              int
	sql               string
	params            dbx.Params
	totalBytes, count int
}

// linesRule finds the rule CTE that reads the lines table in a statement.
var linesRule = regexp.MustCompile("\\b(r\\d+) AS (?:MATERIALIZED )?\\(SELECT `zoo_ledger_lines`")

// measureRun runs the spec once, keeps its group statement (the one with
// __level), and times SQLite's prepare and execution of that statement alone
// on the read pool.
func measureRun(tb testing.TB, z *zoo, raw string) benchStatement {
	tb.Helper()
	var out benchStatement
	start := time.Now()
	observeRead = func(sql string, params dbx.Params) {
		out.count++
		out.totalBytes += len(sql)
		if strings.Contains(sql, "__level") && len(sql) > out.bytes {
			out.bytes, out.sql, out.params = len(sql), sql, params
			out.goBefore = time.Since(start)
		}
	}
	defer func() { observeRead = nil }()
	if _, err := benchRun(z, raw); err != nil {
		tb.Fatalf("Run: %v", err)
	}
	if out.sql == "" {
		tb.Fatal("no group statement")
	}
	if m := linesRule.FindStringSubmatch(out.sql); m != nil {
		out.ruleReads = len(regexp.MustCompile(`(?:FROM|JOIN) `+m[1]+` `).FindAllString(out.sql, -1))
	}
	stmt, args := rawStatement(tb, out.sql, out.params)
	pool, ok := z.app.ConcurrentDB().(*dbx.DB)
	if !ok {
		tb.Fatalf("read pool is %T", z.app.ConcurrentDB())
	}
	db := pool.DB()
	t0 := time.Now()
	prepared, err := db.Prepare(stmt)
	if err != nil {
		tb.Fatal(err)
	}
	out.prepare = time.Since(t0)
	t1 := time.Now()
	rows, err := prepared.Query(args...)
	if err != nil {
		tb.Fatal(err)
	}
	for rows.Next() {
		out.rows++
	}
	if err := rows.Err(); err != nil {
		tb.Fatal(err)
	}
	rows.Close()
	out.run = time.Since(t1)
	prepared.Close()
	return out
}

func benchRun(z *zoo, raw string) (*Result, error) {
	return benchRunWith(z, raw, map[string]json.RawMessage{"asOf": json.RawMessage(benchAsOf)})
}

// benchRunWith runs a spec with the given params; Run refuses a param the
// spec does not declare, so a spec without asOf passes nil.
func benchRunWith(z *zoo, raw string, params map[string]json.RawMessage) (*Result, error) {
	spec, info, err := Load([]byte(raw))
	if err != nil {
		return nil, err
	}
	return Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Params: params, Options: Options{Today: "2026-07-15"}, Auth: z.recs["ann"]})
}

func benchSizes() []int {
	sizes := []int{300, 3000, 30000}
	if env := os.Getenv("LAYOUT_BENCH_LINES"); env != "" {
		sizes = nil
		for _, s := range strings.Split(env, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err == nil {
				sizes = append(sizes, n)
			}
		}
	}
	return sizes
}

func BenchmarkLayoutStatement(b *testing.B) {
	timeout := runLimits.timeout
	runLimits.timeout = 30 * time.Minute
	defer func() { runLimits.timeout = timeout }()
	for _, lines := range benchSizes() {
		for _, sc := range []struct {
			name, spec string
		}{{"balanceSheet", benchBalanceSheet()}, {"balanceSheetLevels", benchLevelsOnly()}, {"trialBalance", benchTrialBalance()}} {
			b.Run(fmt.Sprintf("%s/lines=%d", sc.name, lines), func(b *testing.B) {
				z := newZoo(b)
				withBalanceZoo(b, z)
				withBenchLedger(b, z, lines)
				m := measureRun(b, z, sc.spec)
				b.ResetTimer()
				for b.Loop() {
					if _, err := benchRun(z, sc.spec); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(m.bytes), "stmt-bytes")
				b.ReportMetric(float64(m.ruleReads), "rule-reads")
				b.ReportMetric(float64(m.goBefore.Microseconds())/1000, "go-ms")
				b.ReportMetric(float64(m.prepare.Microseconds())/1000, "prepare-ms")
				b.ReportMetric(float64(m.run.Microseconds())/1000, "sql-ms")
				b.ReportMetric(float64(m.rows), "rows")
				b.ReportMetric(float64(m.count), "stmts")
				b.ReportMetric(float64(m.totalBytes), "all-bytes")
				if dir := os.Getenv("LAYOUT_BENCH_DUMP"); dir != "" {
					name := fmt.Sprintf("%s/%s-%d.sql", dir, sc.name, lines)
					if err := os.WriteFile(name, []byte(m.sql), 0o644); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
