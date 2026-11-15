package reports

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional book of two unions: orders and tickets
// (two document collections), each with its lines (one line per document and
// rate), a line's site standing for the agency and its animal for the rate.
//
//	o1 order  2026-10-02 posted INV-1 (ann): north/leo 10000·10000·290, north/zed 10000·10000·100, south/kit 10000·8000·400
//	k1 ticket 2026-10-03 posted SR-1 (no customer): north/leo 2000·2000·58
//	o2 order  2026-10-05 posted INV-2 (ann): north/leo 5000·0·0 (nothing taxable)
//	o3 order  2026-10-06 void   INV-3 (ann): north/leo 9999·9999·999 (drops out)
//
// (gross · taxable · tax, in cents). North's two rates both carry o1, so its
// sales summed above the rate would count o1 twice.
func addZooSales(t testing.TB, z *zoo) {
	t.Helper()
	find := func(name string) *core.Collection {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return col
	}
	users, sites, animals := find("users"), find("zoo_sites"), find("zoo_animals")
	signedIn := rule(`@request.auth.id != ""`)
	document := func(name string) *core.Collection {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.TextField{Name: "day"},
			&core.SelectField{Name: "status", Values: []string{"posted", "void"}, MaxSelect: 1},
			&core.TextField{Name: "number"},
			&core.NumberField{Name: "sequence"},
			&core.NumberField{Name: "subtotal"},
			&core.TextField{Name: "currency"},
			&core.RelationField{Name: "customer", CollectionId: users.Id, MaxSelect: 1},
		)
		z.save(t, col)
		return col
	}
	line := func(name, docField string, doc *core.Collection) {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.RelationField{Name: docField, CollectionId: doc.Id, MaxSelect: 1},
			&core.RelationField{Name: "site", CollectionId: sites.Id, MaxSelect: 1},
			&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
			&core.NumberField{Name: "gross"},
			&core.NumberField{Name: "taxable"},
			&core.NumberField{Name: "amount"},
			&core.TextField{Name: "currency"},
		)
		z.save(t, col)
	}
	line("zoo_order_lines", "order", document("zoo_orders"))
	line("zoo_ticket_lines", "ticket", document("zoo_tickets"))

	doc := func(key, collection, day, status, number string, seq int, customer string) {
		z.rec(t, key, collection, map[string]any{"day": day, "status": status, "number": number, "sequence": seq, "currency": "USD", "customer": customer})
	}
	ann := z.id("ann")
	doc("o1", "zoo_orders", "2026-10-02", "posted", "INV-1", 1, ann)
	doc("o2", "zoo_orders", "2026-10-05", "posted", "INV-2", 2, ann)
	doc("o3", "zoo_orders", "2026-10-06", "void", "INV-3", 3, ann)
	doc("k1", "zoo_tickets", "2026-10-03", "posted", "SR-1", 1, "")
	ln := func(collection, docField, docKey, site, animal string, gross, taxable, amount int) {
		z.rec(t, docKey+site+animal, collection, map[string]any{
			docField: z.id(docKey), "site": z.id(site), "animal": z.id(animal),
			"gross": gross, "taxable": taxable, "amount": amount, "currency": "USD",
		})
	}
	ln("zoo_order_lines", "order", "o1", "north", "leo", 10000, 10000, 290)
	ln("zoo_order_lines", "order", "o1", "north", "zed", 10000, 10000, 100)
	ln("zoo_order_lines", "order", "o1", "south", "kit", 10000, 8000, 400)
	ln("zoo_ticket_lines", "ticket", "k1", "north", "leo", 2000, 2000, 58)
	ln("zoo_order_lines", "order", "o2", "north", "leo", 5000, 0, 0)
	ln("zoo_order_lines", "order", "o3", "north", "leo", 9999, 9999, 999)
}

func registerZooSalesSources() {
	docColumns := map[string]string{"day": "day", "status": "status", "number": "number", "sequence": "sequence", "subtotal": "subtotal", "currency": "currency", "customer": "customer"}
	RegisterSource(Source{
		Key: "zoo.documents", Label: "Documents",
		Parts: []SourcePart{
			{Collection: "zoo_orders", Columns: docColumns, Values: map[string]any{"kind": "order"}},
			{Collection: "zoo_tickets", Columns: docColumns, Values: map[string]any{"kind": "ticket"}},
		},
		Fields: []Field{
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "order", Label: "Order"}, {Value: "ticket", Label: "Ticket"}}},
			{Key: "day", Label: "Day", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "status", Label: "Status", Type: Select, Options: []Option{{Value: "posted", Label: "Posted"}, {Value: "void", Label: "Void"}}},
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "sequence", Label: "Sequence", Type: Number, Aggregates: MinMax},
			{Key: "subtotal", Label: "Subtotal", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
		},
		Links: []Link{{Key: "customer", Label: "Customer", Target: "zoo.people", Cardinality: ToOne, Field: "customer"}},
	})
	lineColumns := func(docField string) map[string]string {
		return map[string]string{"document": docField, "site": "site", "animal": "animal", "gross": "gross", "taxable": "taxable", "amount": "amount", "currency": "currency"}
	}
	RegisterSource(Source{
		Key: "zoo.lines", Label: "Lines",
		Parts: []SourcePart{
			{Collection: "zoo_order_lines", Columns: lineColumns("order")},
			{Collection: "zoo_ticket_lines", Columns: lineColumns("ticket")},
		},
		Fields: []Field{
			{Key: "gross", Label: "Gross", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "taxable", Label: "Taxable", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "amount", Label: "Tax", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
		},
		Links: []Link{
			{Key: "document", Label: "Document", Target: "zoo.documents", Cardinality: ToOne, Field: "document"},
			{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"},
			{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"},
		},
	})
}

const salesTaxShape = `{"version":4,"source":"zoo.lines",
	"columns":[
		{"id":"name","path":[{"link":"document"},{"field":"number"}]},
		{"id":"siteKey","path":[{"link":"site"}],"hidden":true},
		{"id":"animalKey","path":[{"link":"animal"}],"hidden":true},
		{"id":"gross","path":[{"field":"gross"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"nontaxable","formula":{"op":"sub","args":[{"col":"gross"},{"col":"taxable"}]},"blankAbove":"byAnimal"},
		{"id":"taxable","path":[{"field":"taxable"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"tax","path":[{"field":"amount"}],"aggregate":"sum"}],
	"filter":{"op":"eq","args":[{"path":[{"link":"document","with":"required"},{"field":"status"}]},{"value":"posted"}]},
	"groups":[{"id":"bySite","col":"siteKey","open":true,"subtotals":true},{"id":"byAnimal","col":"animalKey"}],
	"detail":{"label":{"op":"concat","args":[
		{"path":[{"link":"document"},{"field":"day"}]},
		{"value":" · "},
		{"op":"if","args":[{"op":"eq","args":[{"path":[{"link":"document"},{"field":"kind"}]},{"value":"order"}]},{"value":"Order"},{"value":"Ticket"}]},
		{"value":" "},
		{"path":[{"link":"document"},{"field":"number"}]},
		{"op":"if","args":[
			{"op":"isEmpty","args":[{"path":[{"link":"document"},{"link":"customer"},{"field":"name"}]}]},
			{"value":""},
			{"op":"concat","args":[{"value":" · "},{"path":[{"link":"document"},{"link":"customer"},{"field":"name"}]}]}]}]}},
	"sort":[{"formula":{"path":[{"link":"document"},{"field":"day"}]}},{"formula":{"path":[{"link":"document"},{"field":"sequence"}]}}]}`

func TestATwoUnionReportTotalsOnlyTheUnblankedColumnAboveTheInnerLevel(t *testing.T) {
	z := newZoo(t)
	addZooSales(t, z)
	registerZooSalesSources()
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	res := z.run(t, "ann", salesTaxShape, Options{ExpandAll: true}, nil)

	money := []string{"gross", "nontaxable", "taxable", "tax"}
	figures := func(r Row) string {
		out := make([]any, len(money))
		for i, id := range money {
			out[i] = r.Cells[id]
		}
		return fmt.Sprint(out)
	}
	var rates, sites []string
	var underLeo, leoFigures, openSites []string
	var openTax, openWant []int64
	innerTax, allInnerTax := int64(0), int64(0)
	for _, r := range res.Rows {
		switch {
		case r.Kind == KindGroup && r.Level == 1:
			// An open site shows the same tax as its subtotal, and nothing else.
			for _, id := range []string{"gross", "nontaxable", "taxable"} {
				requireNoCell(t, r, id)
			}
			openSites = append(openSites, r.Label)
			openTax = append(openTax, r.Cells["tax"].(int64))
			openWant = append(openWant, 0)
		case r.Kind == KindGroup && r.Level == 2:
			rates = append(rates, r.Label+" "+figures(r))
			innerTax += r.Cells["tax"].(int64)
			openWant[len(openWant)-1] += r.Cells["tax"].(int64)
		case r.Kind == KindDetail && len(rates) == 1:
			underLeo = append(underLeo, r.Label)
			leoFigures = append(leoFigures, figures(r))
		case r.Kind == KindSubtotal || r.Kind == KindTotal:
			// Above the rate only the tax shows, and it adds the rate rows above it.
			for _, id := range []string{"gross", "nontaxable", "taxable"} {
				requireNoCell(t, r, id)
			}
			want := innerTax
			if r.Kind == KindTotal {
				want = allInnerTax
			} else {
				allInnerTax += innerTax
				innerTax = 0
			}
			if r.Cells["tax"] != want {
				t.Fatalf("%s tax = %v, want the rate rows' %d", r.Label, r.Cells["tax"], want)
			}
			sites = append(sites, r.Label+" "+fmt.Sprint(r.Cells["tax"]))
		}
	}
	wantRates := []string{
		"Leo [17000 5000 12000 348]",
		"Zed [10000 0 10000 100]",
		"Kit [10000 2000 8000 400]",
	}
	if fmt.Sprint(rates) != fmt.Sprint(wantRates) {
		t.Fatalf("rate rows = %v\nwant      %v", rates, wantRates)
	}
	if fmt.Sprint(sites) != fmt.Sprint([]string{"Total North 448", "Total South 400", "Total 848"}) {
		t.Fatalf("totals = %v", sites)
	}
	wantLeo := []string{"2026-10-02 · Order INV-1 · ann", "2026-10-03 · Ticket SR-1", "2026-10-05 · Order INV-2 · ann"}
	if fmt.Sprint(underLeo) != fmt.Sprint(wantLeo) {
		t.Fatalf("Leo's documents = %v\nwant              %v", underLeo, wantLeo)
	}
	// gross, nontaxable, taxable, tax of each document's Leo line.
	wantLeoFigures := []string{"[10000 0 10000 290]", "[2000 0 2000 58]", "[5000 5000 0 0]"}
	if fmt.Sprint(leoFigures) != fmt.Sprint(wantLeoFigures) {
		t.Fatalf("Leo's figures = %v\nwant           %v", leoFigures, wantLeoFigures)
	}
	if fmt.Sprint(openSites) != "[North South]" || fmt.Sprint(openTax) != "[448 400]" || fmt.Sprint(openTax) != fmt.Sprint(openWant) {
		t.Fatalf("open sites = %v tax %v, rates' tax %v, want [North South] [448 400]", openSites, openTax, openWant)
	}
}
