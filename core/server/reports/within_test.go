package reports

import (
	"fmt"
	"reflect"
	"testing"
)

// Lions, zebras and monkeys; "extra" (a tenth of the lions) sits inside the
// zebras' section; "all" reads the zebras' total after it.
const zooWithin = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"k","type":"number","label":"K"}],
	"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"layout":[
		{"section":"cats","title":"Lions","key":"lion"},
		{"section":"stripes","title":"Zebras","key":"zebra"},
		{"section":"apes","title":"Monkeys","key":"monkey"},
		{"row":"extra","label":"Extra","within":"stripes","formula":{"op":"mul","args":[{"subtotal":"cats"},{"value":0.1}]}},
		{"row":"all","label":"All zebras","formula":{"subtotal":"stripes"}}%s],
	"sort":[{"col":"cost"}]}`

func TestWithinRowCountsInItsSection(t *testing.T) {
	z := newZoo(t)
	open := z.run(t, "ann", fmt.Sprintf(zooWithin, ""), Options{ExpandAll: true}, nil)
	requireRows(t, open, "label", "Lions", "lion", "lion", "Total Lions", "Zebras", "zebra", "zebra", "Extra", "Total Zebras", "Monkeys", "Total Monkeys", "All zebras")
	requireRows(t, open, "kind", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "computed", "subtotal", "group", "subtotal", "computed")
	// Details sort by cost (f1 1000, f2 2500; f4 300, f3 700). Extra = 3500 × 0.1;
	// the zebras' group row, subtotal and the later row add it.
	requireRows(t, open, "cost", int64(3500), int64(1000), int64(2500), int64(3500), int64(1350), int64(300), int64(700), int64(350), int64(1350), nil, nil, int64(1350))
	// Grams is a number, not money, so × 0.1 stays a float wherever it is added.
	requireRows(t, open, "grams", int64(1300), int64(500), int64(800), int64(1300), float64(530), int64(100), int64(300), float64(130), float64(530), nil, nil, float64(530))
	extra := open.Rows[7]
	if extra.ID != `c:extra:["USD"]` || extra.Depth != 1 || extra.Section != `g:["zebra","USD"]` || extra.CanExpand {
		t.Fatalf("extra = %+v", extra)
	}
}

func TestWithinRowIsAbsentWhileItsSectionIsFolded(t *testing.T) {
	z := newZoo(t)
	shut := z.run(t, "ann", fmt.Sprintf(zooWithin, ""), Options{}, nil)
	requireRows(t, shut, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Monkeys", "Total Monkeys", "All zebras")
	// The folded section's rows still add it.
	requireRows(t, shut, "cost", int64(3500), int64(3500), int64(1350), int64(1350), nil, nil, int64(1350))
}

func TestWithinRowReadsAParam(t *testing.T) {
	z := newZoo(t)
	raw := fmt.Sprintf(zooWithin, `,{"row":"twice","label":"Twice","formula":{"op":"mul","args":[{"subtotal":"cats"},{"param":"k"}]}}`)
	res := z.run(t, "ann", raw, Options{}, map[string]string{"k": `2`})
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(1350), int64(1350), nil, nil, int64(1350), int64(7000))
}

func TestWithinRowStaysInItsSectionOnAPage(t *testing.T) {
	z := newZoo(t)
	raw := fmt.Sprintf(zooWithin, "")
	whole := z.run(t, "ann", raw, Options{ExpandAll: true, Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", raw, Options{ExpandAll: true, Limit: 1})
	if merged := mergedRows(t, pages); !reflect.DeepEqual(merged, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", merged, whole.Rows)
	}
	for i, p := range pages[:len(pages)-1] {
		if last := p.Rows[len(p.Rows)-1]; last.ID == `c:extra:["USD"]` {
			t.Fatalf("page %d ends in the within row as if it were a whole-report row", i+1)
		}
	}
}

func TestWithinRefusals(t *testing.T) {
	z := newZoo(t)
	base := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"layout":[{"section":"cats","title":"Lions","key":"lion"},%s]}`
	cases := map[string]struct {
		items, at, want string
		status          int
	}{
		"reads its own section": {`{"row":"r","label":"R","within":"cats","formula":{"subtotal":"cats"}}`, "layout.1.formula", "it would read itself", 400},
		"a later section":       {`{"row":"r","label":"R","within":"stripes","formula":{"value":1}},{"section":"stripes","title":"Zebras","key":"zebra"}`, "layout.1.within", "a section earlier in the layout", 400},
		"a computed row":        {`{"row":"q","label":"Q","formula":{"value":1}},{"row":"r","label":"R","within":"q","formula":{"value":1}}`, "layout.2.within", "a section earlier in the layout", 400},
		"no such section":       {`{"row":"r","label":"R","within":"dogs","formula":{"value":1}}`, "layout.1.within", "dogs", 422},
		"on a section":          {`{"section":"stripes","title":"Zebras","key":"zebra","within":"cats"}`, "layout.1", "a section has a title and the key", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(base, c.items)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), c.status, c.at, c.want)
		})
	}
}

// An earlier row read by a later one has the value it shows: its section
// reads add only the within rows above it, not those after it (ruling 9).
const zooWithinOrder = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"layout":[
		{"section":"cats","title":"Lions","key":"lion"},
		{"section":"stripes","title":"Zebras","key":"zebra"},
		%s],
	"sort":[{"col":"cost"}]}`

func TestWithinEarlierRowKeepsItsOwnValue(t *testing.T) {
	z := newZoo(t)
	raw := fmt.Sprintf(zooWithinOrder, `{"row":"w1","label":"W1","within":"cats","formula":{"subtotal":"stripes"}},
		{"row":"w3","label":"W3","within":"stripes","formula":{"value":5}},
		{"row":"r","label":"R","formula":{"subtotal":"w1"}}`)
	res := z.run(t, "ann", raw, Options{ExpandAll: true}, nil)
	requireRows(t, res, "label", "Lions", "lion", "lion", "W1", "Total Lions", "Zebras", "zebra", "zebra", "W3", "Total Zebras", "R")
	// W1 = zebras before W3 = 1000; Lions = 3500 + 1000; zebras = 1000 + 5.
	requireRows(t, res, "cost", int64(4500), int64(1000), int64(2500), int64(1000), int64(4500), int64(1005), int64(300), int64(700), int64(5), int64(1005), int64(1000))
}

func TestWithinRowMayReadASectionWhoseWithinRowReadsItsOwn(t *testing.T) {
	z := newZoo(t)
	// W2 reads the lions with W1, and W1 reads the zebras before W2: no cycle.
	raw := fmt.Sprintf(zooWithinOrder, `{"row":"w1","label":"W1","within":"cats","formula":{"subtotal":"stripes"}},
		{"row":"w2","label":"W2","within":"stripes","formula":{"subtotal":"cats"}}`)
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Validate(z.app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	// W1 = 1000; lions = 4500; W2 = 4500; zebras = 1000 + 4500.
	requireRows(t, res, "cost", int64(4500), int64(4500), int64(5500), int64(5500))
}

// Every animal fails the having, so the zebras' section shows no child group;
// its within row still adds to it, so the section opens onto that row (the
// statement explains its own total).
const zooWithinHidden = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"animal","path":[{"link":"animal"},{"field":"name"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"},{"id":"a","col":"animal","having":{"op":"gt","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":5000}]}}],
	"layout":[
		{"section":"stripes","title":"Zebras","key":"zebra"},
		{"row":"extra","label":"Extra","within":"stripes","formula":{"value":5}}]}`

func TestWithinRowShowsUnderASectionWhoseRowsTheHavingHides(t *testing.T) {
	z := newZoo(t)
	open := z.run(t, "ann", zooWithinHidden, Options{ExpandAll: true}, nil)
	requireRows(t, open, "label", "Zebras", "Extra", "Total Zebras")
	requireRows(t, open, "cost", int64(1005), int64(5), int64(1005))
	if !open.Rows[0].CanExpand || !open.Rows[0].Expanded {
		t.Fatalf("section = %+v", open.Rows[0])
	}
	shut := z.run(t, "ann", zooWithinHidden, Options{}, nil)
	requireRows(t, shut, "label", "Zebras", "Total Zebras")
	if !shut.Rows[0].CanExpand {
		t.Fatalf("folded section cannot open: %+v", shut.Rows[0])
	}
	pages := groupedPages(t, z, "ann", zooWithinHidden, Options{ExpandAll: true, Limit: 1})
	if merged := mergedRows(t, pages); !reflect.DeepEqual(merged, open.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", merged, open.Rows)
	}
}
