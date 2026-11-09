package reports

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The balance-shaped report (balance_shape_test.go) over a chart of many
// accounts: its beginning balance read in the line balance, the rollup, the
// having and the opening cell, its group record read by the label, the sort
// keys and the having, and its pages. The engine's output for each run is
// pinned by a hash of every page (a set with EUR and blank currencies pins
// the mixed-currency notices too) (testdata/ledger_scale.golden.json), so a
// change to how the engine computes it cannot change a figure, a row, an
// order, a notice or a cursor.
//
// LEDGER_GOLDEN_UPDATE=1 writes the hashes again; LEDGER_GOLDEN_DUMP=<dir>
// writes each run's pages as JSON there, to diff two engines' output.

const ledgerGolden = "testdata/ledger_scale.golden.json"

// ledgerTypes are the account types of the balance fixture, in its rank order.
var ledgerTypes = []string{"asset", "liability", "equity", "income", "expense"}

// withLedgerScale adds n accounts to the balance fixture, two entries per
// account over three years (a quarter of them with a third line), every
// record with a fixed id so the output does not move between runs.
func withLedgerScale(t testing.TB, z *zoo, n int) { withLedgerLines(t, z, n, false) }

// withLedgerLines is withLedgerScale; with mixed, some entries are in EUR and
// some have no currency, so lookups and totals meet two currencies.
func withLedgerLines(t testing.TB, z *zoo, n int, mixed bool) {
	t.Helper()
	for i := range n {
		number := fmt.Sprintf("%d", 6000+i)
		if i%9 == 0 {
			number = ""
		}
		detail := ""
		if i == 2 {
			detail = "retained_earnings"
		}
		z.recWithID(t, ledgerID("acct", i), "zoo_ledger_accounts", map[string]any{
			"number": number, "full_name": fmt.Sprintf("Account %03d", i), "type": ledgerTypes[i%len(ledgerTypes)],
			"detail_type": detail, "inactive": i%7 == 3,
		})
	}
	line := 0
	for e := range 2 * n {
		entry := ledgerID("entr", e)
		z.recWithID(t, entry, "zoo_ledger_entries", map[string]any{"number": fmt.Sprintf("S%d", e)})
		a, b := e%n, (e*7+3)%n
		if a == b {
			b = (a + 1) % n
		}
		day := fmt.Sprintf("%d-%02d-%02d", 2024+e%3, 1+(e*5)%12, 1+(e*11)%28)
		amount := 100 + (e*37)%5000
		sides := []struct{ account, debit, credit int }{{a, amount, 0}, {b, 0, amount}}
		if e%4 == 1 {
			sides[1].credit = amount - 50
			sides = append(sides, struct{ account, debit, credit int }{(e*3 + 1) % n, 0, 50})
		}
		currency := "USD"
		if mixed && e%5 == 2 {
			currency = "EUR"
		} else if mixed && e%11 == 7 {
			currency = ""
		}
		for j, s := range sides {
			z.recWithID(t, ledgerID("line", line), "zoo_ledger_lines", map[string]any{
				"entry": entry, "account": ledgerID("acct", s.account), "date": day,
				"debit": s.debit, "credit": s.credit, "currency": currency, "position": j + 1, "posted": e%13 != 5,
			})
			line++
		}
	}
}

func ledgerID(prefix string, i int) string { return fmt.Sprintf("%s%011d", prefix, i) }

// recWithID saves a record with a fixed id.
func (z *zoo) recWithID(t testing.TB, id, collection string, data map[string]any) *core.Record {
	t.Helper()
	col, err := z.app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Id = id
	for k, v := range data {
		r.Set(k, v)
	}
	if err := z.app.Save(r); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
	return r
}

// ledgerRun is one run of the balance report: its params and options; a run
// with a limit reads every page.
type ledgerRun struct {
	name   string
	params map[string]string
	opts   Options
}

const ledgerPeriod = `{"from":"2025-03-01","to":"2025-08-31"}`

func ledgerRuns() []ledgerRun {
	period := map[string]string{"period": ledgerPeriod}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{"period": ledgerPeriod}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	accounts := fmt.Sprintf(`[%q,%q,%q]`, ledgerID("acct", 0), ledgerID("acct", 2), ledgerID("acct", 4))
	return []ledgerRun{
		{"default", period, Options{}},
		{"pages", period, Options{Limit: 40}},
		{"all", with(map[string]string{"rows": `"all"`}), Options{Limit: 60}},
		{"accounts", with(map[string]string{"accounts": accounts}), Options{Limit: 7}},
		{"collapsed", period, Options{Expand: [][]any{}}},
		{"someOpen", period, Options{Expand: [][]any{{ledgerID("acct", 4), "USD"}, {ledgerID("acct", 13), "USD"}}, Limit: 5}},
		{"viewSort", period, Options{ExpandAll: true, Limit: 50, Sort: []ViewSort{{Col: "debit", Dir: "desc"}}}},
		{"allDates", map[string]string{"period": `{"preset":"allDates"}`}, Options{Limit: 100}},
	}
}

// ledgerPages runs a ledger run and every page after it.
func ledgerPages(t testing.TB, z *zoo, r ledgerRun) []*Result {
	t.Helper()
	opts := r.opts
	opts.Today = "2025-07-15"
	var pages []*Result
	for {
		res, err := runLedger(z, r.params, opts)
		if err != nil {
			t.Fatalf("%s: Run: %v", r.name, err)
		}
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		if len(pages) > 200 {
			t.Fatalf("%s: more than 200 pages", r.name)
		}
		opts.Cursor = res.Page.Next
	}
}

func runLedger(z *zoo, params map[string]string, opts Options) (*Result, error) {
	spec, info, err := Load([]byte(balanceSpec()))
	if err != nil {
		return nil, err
	}
	given := map[string]json.RawMessage{}
	for k, v := range params {
		given[k] = json.RawMessage(v)
	}
	return Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Params: given, Options: opts, Auth: z.recs["ann"]})
}

// ledgerCanonical writes a run's pages as JSON with each cursor decoded and
// each record id of the fixture's random ids written as its fixture key, so
// two runs on two databases read the same.
func ledgerCanonical(t testing.TB, z *zoo, pages []*Result) string {
	t.Helper()
	type page struct {
		Result *Result
		Cursor string
	}
	out := make([]page, len(pages))
	for i, p := range pages {
		out[i] = page{Result: p}
		if p.Page != nil && p.Page.Next != "" {
			raw, err := base64.RawURLEncoding.DecodeString(p.Page.Next)
			if err != nil {
				t.Fatal(err)
			}
			out[i].Cursor = string(raw)
		}
	}
	raw, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	keys := make([]string, 0, len(z.recs))
	for k := range z.recs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		text = strings.ReplaceAll(text, z.recs[k].Id, "<"+k+">")
	}
	return text
}

func readLedgerGolden(t testing.TB) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(ledgerGolden)
	if os.IsNotExist(err) {
		return map[string]string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestABalanceShapedReportOverManyAccountsKeepsItsOutput(t *testing.T) {
	golden := readLedgerGolden(t)
	update := os.Getenv("LEDGER_GOLDEN_UPDATE") != ""
	dump := os.Getenv("LEDGER_GOLDEN_DUMP")
	for _, set := range []struct {
		n     int
		mixed bool
	}{{60, false}, {200, false}, {45, true}} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, set.n, set.mixed)
		for _, r := range ledgerRuns() {
			name := fmt.Sprintf("n%d/%s", set.n, r.name)
			if set.mixed {
				name = fmt.Sprintf("mixed%d/%s", set.n, r.name)
			}
			text := ledgerCanonical(t, z, ledgerPages(t, z, r))
			sum := sha256.Sum256([]byte(text))
			got := hex.EncodeToString(sum[:])
			if dump != "" {
				file := filepath.Join(dump, strings.ReplaceAll(name, "/", "-")+".json")
				if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if update {
				golden[name] = got
				continue
			}
			if want, ok := golden[name]; !ok || want != got {
				t.Errorf("%s: output hash %s, want %s (LEDGER_GOLDEN_DUMP=<dir> writes the pages to diff)", name, got, want)
			}
		}
	}
	if update {
		raw, err := json.MarshalIndent(golden, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(ledgerGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ledgerGolden, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// BenchmarkLedgerRun measures the default ledger run on 60 and 200 accounts.
func BenchmarkLedgerRun(b *testing.B) {
	for _, n := range []int{60, 200} {
		b.Run(fmt.Sprintf("accounts=%d", n), func(b *testing.B) {
			z := newZoo(b)
			withBalanceZoo(b, z)
			withLedgerScale(b, z, n)
			b.ResetTimer()
			for b.Loop() {
				if _, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
