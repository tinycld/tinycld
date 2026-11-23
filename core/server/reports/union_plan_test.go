package reports

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// E3: a key that a union part does not map is a blank with the type of the
// column the other parts map, so SQLite flattens the union into the outer
// query and each part is read by its own indexes. The fictional package's
// union zoo.payments has two parts: zoo_posted_lines (a ledger) and
// zoo_paid_shares (the paid shares of its lines), each indexed on
// (account, day). Only the shares map the payment link and the payment's
// details (memo, status, qty, cleared, settled); a posted line reads them
// blank.
//
//	p1 posted north 2026-01-05 1000   p2 posted south 2026-02-10 2500
//	p3 posted north 2026-02-20  400
//	s1 share  north 2026-01-12  700 (leo)  s2 share south 2026-03-03 300 (zed)
//	s3 share  north 2026-02-14  150 (kit)
//
// With tags, the shares also map the key tags to a multi relation (a column
// PocketBase declares JSON, of NUMERIC affinity in SQLite): s1 tags leo and
// kit, s2 zed, s3 none.
func addZooPayments(t *testing.T, z *zoo, withTags bool) {
	t.Helper()
	find := func(name string) *core.Collection {
		col, err := z.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return col
	}
	sites, animals := find("zoo_sites"), find("zoo_animals")
	signedIn := rule(`@request.auth.id != ""`)
	part := func(name string, payment bool) {
		col := core.NewBaseCollection(name)
		col.ListRule, col.ViewRule = signedIn, signedIn
		col.Fields.Add(
			&core.RelationField{Name: "account", CollectionId: sites.Id, MaxSelect: 1},
			&core.TextField{Name: "day"},
			&core.NumberField{Name: "amount"},
			&core.TextField{Name: "currency"},
			&core.TextField{Name: "note"},
		)
		if payment {
			col.Fields.Add(
				&core.RelationField{Name: "payment", CollectionId: animals.Id, MaxSelect: 1},
				&core.TextField{Name: "memo"},
				&core.SelectField{Name: "status", Values: []string{"cleared", "pending"}, MaxSelect: 1},
				&core.NumberField{Name: "qty"},
				&core.BoolField{Name: "cleared"},
				&core.TextField{Name: "settled"},
			)
			if withTags {
				col.Fields.Add(&core.RelationField{Name: "tags", CollectionId: animals.Id, MaxSelect: 3})
			}
		}
		col.AddIndex("idx_"+name+"_account_day", false, "account, day", "")
		z.save(t, col)
	}
	part("zoo_posted_lines", false)
	part("zoo_paid_shares", true)
	posted := func(key, site, day string, amount int) {
		z.rec(t, key, "zoo_posted_lines", map[string]any{"account": z.id(site), "day": day, "amount": amount, "currency": "USD", "note": key})
	}
	shareTags := map[string][]string{"s1": {"leo", "kit"}, "s2": {"zed"}}
	share := func(key, site, day string, amount int, animal, memo, status string, qty int, cleared bool, settled string) {
		data := map[string]any{
			"account": z.id(site), "day": day, "amount": amount, "currency": "USD", "note": key,
			"payment": z.id(animal), "memo": memo, "status": status, "qty": qty, "cleared": cleared, "settled": settled,
		}
		if withTags {
			ids := []string{}
			for _, a := range shareTags[key] {
				ids = append(ids, z.id(a))
			}
			data["tags"] = ids
		}
		z.rec(t, key, "zoo_paid_shares", data)
	}
	posted("p1", "north", "2026-01-05", 1000)
	posted("p2", "south", "2026-02-10", 2500)
	posted("p3", "north", "2026-02-20", 400)
	share("s1", "north", "2026-01-12", 700, "leo", "by check", "cleared", 2, true, "2026-01-20")
	share("s2", "south", "2026-03-03", 300, "zed", "by card", "pending", 1, false, "2026-03-09")
	share("s3", "north", "2026-02-14", 150, "kit", "by wire", "cleared", 3, true, "2026-02-18")

	lineColumns := map[string]string{"account": "account", "day": "day", "amount": "amount", "currency": "currency", "note": "note"}
	shareColumns := map[string]string{
		"account": "account", "day": "day", "amount": "amount", "currency": "currency", "note": "note",
		"payment": "payment", "memo": "memo", "status": "status", "qty": "qty", "cleared": "cleared", "settled": "settled",
	}
	if withTags {
		// Not a field or a link: the union still projects the key.
		shareColumns["tags"] = "tags"
	}
	RegisterSource(Source{
		Key: "zoo.payments", Label: "Payments",
		Parts: []SourcePart{
			{Collection: "zoo_posted_lines", Columns: lineColumns},
			{Collection: "zoo_paid_shares", Columns: shareColumns},
		},
		Fields: []Field{
			{Key: "day", Label: "Day", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "amount", Label: "Amount", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax},
			{Key: "memo", Label: "Memo", Type: Text, Aggregates: MinMax},
			{Key: "status", Label: "Status", Type: Select, Options: []Option{{Value: "cleared", Label: "Cleared"}, {Value: "pending", Label: "Pending"}}},
			{Key: "qty", Label: "Qty", Type: Number, Aggregates: SumAll},
			{Key: "cleared", Label: "Cleared", Type: Bool},
			{Key: "settled", Label: "Settled", Type: Date, TextDay: true, Aggregates: MinMax},
		},
		Links: []Link{
			{Key: "account", Label: "Account", Target: "zoo.sites", Cardinality: ToOne, Field: "account"},
			{Key: "payment", Label: "Payment", Target: "zoo.animals", Cardinality: ToOne, Field: "payment"},
		},
		Currency: func(core.App, *core.RequestInfo) string { return "USD" },
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
}

// paymentsOfNorth is the payments on the North account in a period, grouped
// by the account.
const paymentsOfNorth = `{"version":4,"source":"zoo.payments",
	"params":[{"id":"period","type":"dateRange","label":"Period"}],
	"columns":[{"id":"acct","path":[{"link":"account"}]},{"id":"note","path":[{"field":"note"}]},
		{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}],
	"filter":{"op":"and","args":[
		{"op":"eq","args":[{"path":[{"link":"account","with":"required"},{"field":"name"}]},{"value":"North"}]},
		{"op":"between","args":[{"path":[{"field":"day"}]},{"param":"period"}]}]},
	"groups":[{"id":"byAcct","col":"acct","open":true}],
	"sort":[{"col":"note"}]}`

var (
	scanOfAPart = regexp.MustCompile(`SCAN (zoo_posted_lines|zoo_paid_shares)\b`)
	// The union is read as t<n>, as the rows CTE w once it is flattened, or as
	// a part by its own name; an automatic index on the open groups (gord) or
	// on a CTE of totals is not an index on the union.
	automaticOnUnion = regexp.MustCompile(`(SEARCH|SCAN) (t\d+|w|zoo_posted_lines|zoo_paid_shares) USING AUTOMATIC`)
)

// A filter on the account and a between on the day reach each part: every
// statement that reads a part reads it by its (account, day) index, and none
// builds an automatic index over the union.
func TestUnionPartBlankKeyKeepsPartIndexes(t *testing.T) {
	requirePartIndexes(t, false)
}

// A key whose mapped column PocketBase declares JSON (a multi relation) is a
// blank of that column's affinity too, so the union is still flattened.
func TestUnionPartBlankJSONKeyKeepsPartIndexes(t *testing.T) {
	requirePartIndexes(t, true)
}

func requirePartIndexes(t *testing.T, withTags bool) {
	t.Helper()
	z := newZoo(t)
	addZooPayments(t, z, withTags)
	seen := observeReads(t)
	res := z.run(t, "ann", paymentsOfNorth, Options{}, map[string]string{"period": `{"from":"2026-01-01","to":"2026-02-28"}`})
	if got := strings.Join(detailNotes(res), ","); got != "p1,p3,s1,s3" {
		t.Fatalf("the North rows of Jan-Feb are %s, want p1,p3,s1,s3", got)
	}
	plan := unionPlans(t, z, *seen)
	for _, want := range []string{
		"SEARCH zoo_posted_lines USING INDEX idx_zoo_posted_lines_account_day",
		"SEARCH zoo_paid_shares USING INDEX idx_zoo_paid_shares_account_day",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("the plan has no %q:\n%s", want, plan)
		}
	}
	if m := scanOfAPart.FindString(plan); m != "" {
		t.Errorf("the plan has %q:\n%s", m, plan)
	}
	if m := automaticOnUnion.FindString(plan); m != "" {
		t.Errorf("the plan builds an automatic index on the union (%q):\n%s", m, plan)
	}
}

// paymentsByKey lists every payment, sorted by one key then the note, with
// each blank-able key's cell, isEmpty of it, and the payment's linked name.
const paymentsByKey = `{"version":4,"source":"zoo.payments",
	"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"memo","path":[{"field":"memo"}]},
		{"id":"payment","path":[{"link":"payment"}]},
		{"id":"payName","path":[{"link":"payment"},{"field":"name"}]},
		{"id":"status","path":[{"field":"status"}]},
		{"id":"qty","path":[{"field":"qty"}]},
		{"id":"cleared","path":[{"field":"cleared"}]},
		{"id":"settled","path":[{"field":"settled"}]},
		{"id":"memoEmpty","formula":{"op":"isEmpty","args":[{"path":[{"field":"memo"}]}]}},
		{"id":"paymentEmpty","formula":{"op":"isEmpty","args":[{"path":[{"link":"payment"}]}]}},
		{"id":"statusEmpty","formula":{"op":"isEmpty","args":[{"path":[{"field":"status"}]}]}},
		{"id":"qtyEmpty","formula":{"op":"isEmpty","args":[{"path":[{"field":"qty"}]}]}},
		{"id":"clearedEmpty","formula":{"op":"isEmpty","args":[{"path":[{"field":"cleared"}]}]}},
		{"id":"settledEmpty","formula":{"op":"isEmpty","args":[{"path":[{"field":"settled"}]}]}}],
	"sort":[{"col":"%s","dir":"%s"},{"col":"note"}]}`

func paymentsSortedBy(key, dir string) string {
	return strings.Replace(paymentsByKey, `"%s","dir":"%s"`, `"`+key+`","dir":"`+dir+`"`, 1)
}

// cellsByNote is one cell of every detail row, by its note.
func cellsByNote(res *Result, col string) map[string]any {
	out := map[string]any{}
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			out[r.Cells["note"].(string)] = r.Cells[col]
		}
	}
	return out
}

func detailNotes(res *Result) []string {
	var out []string
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			out = append(out, r.Cells["note"].(string))
		}
	}
	return out
}

// A key a part does not map reads as a blank of every field type: its cell is
// empty, isEmpty is true, a link through it finds no record, and a sort puts
// the blank rows last, going up and going down.
func TestUnionPartBlankKeyReadsBlank(t *testing.T) {
	z := newZoo(t)
	addZooPayments(t, z, true)
	shares := map[string]map[string]any{
		"memo":    {"s1": "by check", "s2": "by card", "s3": "by wire"},
		"payment": {"s1": z.id("leo"), "s2": z.id("zed"), "s3": z.id("kit")},
		"payName": {"s1": "Leo", "s2": "Zed", "s3": "Kit"},
		"status":  {"s1": "cleared", "s2": "pending", "s3": "cleared"},
		"qty":     {"s1": int64(2), "s2": int64(1), "s3": int64(3)},
		"cleared": {"s1": true, "s2": false, "s3": true},
		"settled": {"s1": "2026-01-20", "s2": "2026-03-09", "s3": "2026-02-18"},
	}
	// The engine sorts a blank after every value, going up and going down, as
	// it sorts NULL; the blank rows then follow the note.
	up := map[string][]string{
		"memo":    {"s2", "s1", "s3", "p1", "p2", "p3"},
		"payment": {"s3", "s1", "s2", "p1", "p2", "p3"},
		"status":  {"s1", "s3", "s2", "p1", "p2", "p3"},
		"qty":     {"s2", "s1", "s3", "p1", "p2", "p3"},
		"cleared": {"s2", "s1", "s3", "p1", "p2", "p3"},
		"settled": {"s1", "s3", "s2", "p1", "p2", "p3"},
	}
	for _, key := range []string{"memo", "payment", "status", "qty", "cleared", "settled"} {
		t.Run(key, func(t *testing.T) {
			res := z.run(t, "ann", paymentsSortedBy(key, "asc"), Options{}, nil)
			cells := cellsByNote(res, key)
			empty := cellsByNote(res, key+"Empty")
			for _, p := range []string{"p1", "p2", "p3"} {
				if cells[p] != nil {
					t.Errorf("%s of %s = %#v, want blank", key, p, cells[p])
				}
				if empty[p] != true {
					t.Errorf("isEmpty(%s) of %s = %#v, want true", key, p, empty[p])
				}
			}
			for s, want := range shares[key] {
				if cells[s] != want {
					t.Errorf("%s of %s = %#v, want %#v", key, s, cells[s], want)
				}
				if empty[s] != false {
					t.Errorf("isEmpty(%s) of %s = %#v, want false", key, s, empty[s])
				}
			}
			if notes, want := detailNotes(res), up[key]; strings.Join(notes, ",") != strings.Join(want, ",") {
				t.Errorf("sorted up by %s: %v, want %v", key, notes, want)
			}
			if notes := detailNotes(z.run(t, "ann", paymentsSortedBy(key, "desc"), Options{}, nil)); len(notes) != 6 || strings.Join(notes[3:], ",") != "p1,p2,p3" {
				t.Errorf("sorted down by %s: %v, want p1, p2, p3 last", key, notes)
			}
		})
	}
	names := cellsByNote(z.run(t, "ann", paymentsSortedBy("note", "asc"), Options{}, nil), "payName")
	for _, p := range []string{"p1", "p2", "p3"} {
		if names[p] != nil {
			t.Errorf("the payment link of %s reads %#v, want no record", p, names[p])
		}
	}
	for s, want := range shares["payName"] {
		if names[s] != want {
			t.Errorf("the payment link of %s reads %#v, want %#v", s, names[s], want)
		}
	}

	// The multi relation key, which no field reads: the union holds a NULL in
	// a posted line's row and the stored list in a share's.
	union, params, err := unionSelect(z.app, lookupRegistered("zoo.payments"), z.as("ann"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []dbx.NullStringMap
	if err := z.app.DB().NewQuery("SELECT [[id]], [[tags]] FROM (" + union + ")").Bind(params).All(&rows); err != nil {
		t.Fatalf("the union's tags: %v", err)
	}
	tagsOf := map[string]sql.NullString{}
	for _, r := range rows {
		tagsOf[r["id"].String] = r["tags"]
	}
	for _, p := range []string{"p1", "p2", "p3"} {
		if got, ok := tagsOf[z.id(p)]; !ok || got.Valid {
			t.Errorf("tags of %s = %#v (in the union: %v), want NULL", p, got, ok)
		}
	}
	for s, want := range map[string]string{
		"s1": `["` + z.id("leo") + `","` + z.id("kit") + `"]`,
		"s2": `["` + z.id("zed") + `"]`,
		"s3": `[]`,
	} {
		if got := tagsOf[z.id(s)]; !got.Valid || got.String != want {
			t.Errorf("tags of %s = %#v, want %s", s, got, want)
		}
	}
}
