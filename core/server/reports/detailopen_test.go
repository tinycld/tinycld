package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// feedingsOpen lists ann's feedings by note (f1, f2 Leo; f3, f4 Zed) with
// the detail open formula given.
const feedingsOpen = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}],"detail":{"open":%s}}`

// countLogs counts, until the test ends, the package logger's records whose
// message holds msg. It replaces the package logger: never use it in a
// t.Parallel test.
func countLogs(t *testing.T, msg string) *atomic.Int32 {
	t.Helper()
	n := &atomic.Int32{}
	saved := log
	log = slog.New(&countHandler{Handler: saved.Handler(), msg: msg, n: n})
	t.Cleanup(func() { log = saved })
	return n
}

type countHandler struct {
	slog.Handler
	msg string
	n   *atomic.Int32
}

func (h *countHandler) Handle(ctx context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.msg) {
		h.n.Add(1)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *countHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &countHandler{Handler: h.Handler.WithAttrs(as), msg: h.msg, n: h.n}
}

func (h *countHandler) WithGroup(name string) slog.Handler {
	return &countHandler{Handler: h.Handler.WithGroup(name), msg: h.msg, n: h.n}
}

// opensOf lists each row's open path as "<kind> <open>".
func opensOf(res *Result) []string {
	out := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, r.Kind+" "+r.Open)
	}
	return out
}

func TestADetailRowOpensItsPath(t *testing.T) {
	z := newZoo(t)
	// Inside open a relation reads as its record's id, not its label (R2).
	spec := fmt.Sprintf(feedingsOpen, `{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"link":"animal"}]}]}`)
	res := z.run(t, "ann", spec, Options{}, nil)
	leo, zed := "detail zoo/animals/"+z.id("leo"), "detail zoo/animals/"+z.id("zed")
	if got, want := opensOf(res), []string{leo, leo, zed, zed, "total "}; !slices.Equal(got, want) {
		t.Fatalf("opens = %v, want %v", got, want)
	}
	raw, err := json.Marshal(res.Rows[len(res.Rows)-1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"open"`) {
		t.Fatalf("a row with no open path writes no open key: %s", raw)
	}
}

func TestDetailOpenDropsABadPathAndLogsOncePerRun(t *testing.T) {
	z := newZoo(t)
	for name, prefix := range map[string]string{
		"another package": "other/x/",
		"dots":            "zoo/../",
		"two slashes":     "zoo//",
		"a query":         "zoo/x?q=",
		"a scheme":        "https://zoo/",
		"a space":         "zoo/a b/",
	} {
		t.Run(name, func(t *testing.T) {
			logs := countLogs(t, "open path")
			spec := fmt.Sprintf(feedingsOpen, fmt.Sprintf(`{"op":"concat","args":[{"value":%q},{"path":[{"field":"note"}]}]}`, prefix))
			res := z.run(t, "ann", spec, Options{}, nil)
			if got, want := opensOf(res), []string{"detail ", "detail ", "detail ", "detail ", "total "}; !slices.Equal(got, want) {
				t.Fatalf("opens = %v, want none", got)
			}
			if n := logs.Load(); n != 1 {
				t.Fatalf("logged %d times, want once per run", n)
			}
		})
	}
}

func TestOpenPathKeepsOnlyAPathOfThePackage(t *testing.T) {
	for value, want := range map[string]bool{
		"zoo/animals/abc123": true,
		"zoo/a-b_c/D9":       true,
		"zoo/":               false,
		"zoo/animals/":       false,
		"zoo":                false,
		"zoos/animals/x":     false,
		"zoo/../x":           false,
		"zoo//x":             false,
		"zoo/x?y":            false,
		"zoo/x#y":            false,
		"https://zoo/x":      false,
		"/zoo/x":             false,
		"":                   false,
	} {
		if _, ok := openPath("zoo", value); ok != want {
			t.Errorf("openPath(zoo, %q) = %v, want %v", value, ok, want)
		}
	}
}

func TestNestedChildRowsOpenNothing(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","path":[{"link":"feedings","mode":"nest"},{"field":"note"}]}],
		"sort":[{"col":"name"}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"field":"name"}]}]}}}`, Options{}, nil)
	parents := 0
	for _, r := range res.Rows {
		switch {
		case r.Parent != "" && r.Open != "":
			t.Fatalf("child row %s opens %q", r.ID, r.Open)
		case r.Kind == KindDetail && r.Parent == "":
			parents++
			if r.Open != "zoo/animals/"+r.Label {
				t.Fatalf("row %s opens %q", r.Label, r.Open)
			}
		}
	}
	if parents == 0 {
		t.Fatal("no parent rows")
	}
}

func TestOnlyDetailRowsOpen(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","subtotals":true,"open":true}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/feedings/"},{"path":[{"field":"note"}]}]}}}`, Options{}, nil)
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
			if !strings.HasPrefix(r.Open, "zoo/feedings/f") {
				t.Fatalf("detail row %s opens %q", r.Label, r.Open)
			}
		} else if r.Open != "" {
			t.Fatalf("%s row %s opens %q", r.Kind, r.Label, r.Open)
		}
	}
	if details != 4 {
		t.Fatalf("detail rows = %d, want 4", details)
	}
}

func TestALayoutComputedRowOpensNothing(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"},
			{"row":"both","label":"Both","formula":{"op":"add","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/feedings/"},{"path":[{"field":"note"}]}]}}}`, Options{ExpandAll: true}, nil)
	computed, details := 0, 0
	for _, r := range res.Rows {
		switch r.Kind {
		case KindComputed:
			computed++
			if r.Open != "" {
				t.Fatalf("computed row %s opens %q", r.Label, r.Open)
			}
		case KindDetail:
			details++
			if !strings.HasPrefix(r.Open, "zoo/feedings/f") {
				t.Fatalf("detail row %s opens %q", r.Label, r.Open)
			}
		default:
			if r.Open != "" {
				t.Fatalf("%s row %s opens %q", r.Kind, r.Label, r.Open)
			}
		}
	}
	if computed != 1 || details != 4 {
		t.Fatalf("computed rows = %d, detail rows = %d, want 1 and 4", computed, details)
	}
}

func TestDetailOpenIsRefusedOnAPivotAndWhenNotText(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ spec, at, contains string }{
		"a pivot": {`{"version":4,"source":"zoo.feedings","columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species"}],
			"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"},
			"detail":{"open":{"value":"zoo/x"}}}`, "detail.open", "no detail rows to open"},
		"a number":   {fmt.Sprintf(feedingsOpen, `{"path":[{"field":"grams"}]}`), "detail.open", "open is a text"},
		"a relation": {fmt.Sprintf(feedingsOpen, `{"path":[{"link":"animal"}]}`), "detail.open", "open is a text"},
		"empty":      {`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"detail":{}}`, "detail", "detail has a label or an open"},
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, c.at, c.contains)
		})
	}
}

func TestDetailOpenReadsARelationAsItsIDInsideAnAggregateAndALookup(t *testing.T) {
	z := newZoo(t)
	leo, zed := "detail zoo/animals/"+z.id("leo"), "detail zoo/animals/"+z.id("zed")
	for name, open := range map[string]string{
		"an aggregate": `{"op":"max","args":[{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"link":"animal"}]}]}]}`,
		"a lookup": `{"op":"lookup","source":"zoo.feedings",
			"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"outer":[{"field":"note"}]}]},
			"value":{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"link":"animal"}]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(feedingsOpen, open), Options{}, nil)
			if got, want := opensOf(res), []string{leo, leo, zed, zed, "total "}; !slices.Equal(got, want) {
				t.Fatalf("opens = %v, want %v", got, want)
			}
		})
	}
}

func TestDetailLabelAndOpenShareOneRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"sort":[{"col":"note"}],
		"detail":{"label":{"op":"concat","args":[{"value":"Feeding "},{"path":[{"field":"note"}]}]},
			"open":{"op":"concat","args":[{"value":"zoo/animals/"},{"path":[{"link":"animal"}]}]}}}`, Options{}, nil)
	leo, zed := "zoo/animals/"+z.id("leo"), "zoo/animals/"+z.id("zed")
	want := map[string][2]string{"Feeding f1": {"Feeding f1", leo}, "Feeding f2": {"Feeding f2", leo}, "Feeding f3": {"Feeding f3", zed}, "Feeding f4": {"Feeding f4", zed}}
	seen := 0
	for _, r := range res.Rows {
		if r.Kind != KindDetail {
			continue
		}
		seen++
		if w, ok := want[r.Label]; !ok || r.Open != w[1] {
			t.Fatalf("row %q opens %q, want %v", r.Label, r.Open, w)
		}
	}
	if seen != 4 {
		t.Fatalf("detail rows = %d, want 4", seen)
	}
}

func TestDetailOpenReadsAColumnFormula(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"who","formula":{"path":[{"link":"animal"}]}},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"sort":[{"col":"note"}],
		"detail":{"open":{"op":"concat","args":[{"value":"zoo/animals/"},{"col":"who"}]}}}`, Options{}, nil)
	leo, zed := "detail zoo/animals/"+z.id("leo"), "detail zoo/animals/"+z.id("zed")
	if got, want := opensOf(res), []string{leo, leo, zed, zed, "total "}; !slices.Equal(got, want) {
		t.Fatalf("opens = %v, want %v", got, want)
	}
}
