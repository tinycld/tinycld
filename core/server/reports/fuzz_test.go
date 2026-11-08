package reports

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"testing"
)

// The rule-parity fuzz test: for a random reader, source and filter, the
// engine's detail rows are exactly the rows PocketBase's list API returns
// for the same reader and filter, and a to-one linked value is the one the
// list API expands. A link to a row the reader cannot see (zoo_vault, which
// only superusers list) gives an empty cell in the engine and no expand entry
// in the list API. Each seed builds one case; `go test -fuzz FuzzRuleParity`
// explores further seeds.

var fuzzWords = []string{"hay", "fish", "meat", "fruit", "seeds"}

// fuzzLink is a to-one link the fuzzer expands and the field it reads there.
type fuzzLink struct{ link, field string }

// fuzzSource is a source the fuzzer filters: its collection, the fields a
// filter compares and the to-one links to expand.
type fuzzSource struct {
	key, collection string
	numbers, texts  []string
	links           []fuzzLink
}

var fuzzSources = []fuzzSource{
	{"zoo.animals", "zoo_animals", []string{"weight", "price"}, []string{"name", "species"}, []fuzzLink{{"site", "name"}, {"vault", "name"}}},
	{"zoo.feedings", "zoo_feedings", []string{"cost", "grams"}, []string{"note", "currency"}, []fuzzLink{{"animal", "name"}}},
	{"zoo.keepers", "zoo_keepers", nil, nil, []fuzzLink{{"animal", "species"}}},
	{"zoo.sites", "zoo_sites", nil, []string{"name"}, nil},
}

// growZoo adds random animals, keepers and feedings for three readers.
func growZoo(t testing.TB, z *zoo, rng *rand.Rand) {
	carol := z.user(t, "carol")
	// zoo_sites lists only members: a guest reader is excluded from it.
	carol.Set("role", "guest")
	if err := z.app.Save(carol); err != nil {
		t.Fatalf("save carol: %v", err)
	}
	readers := []string{z.id("ann"), z.id("bob"), carol.Id, ""}
	sites := []string{z.id("north"), z.id("south"), ""}
	vaults := []string{""}
	for i := range 6 {
		v := z.rec(t, fmt.Sprintf("vv%d", i), "zoo_vault", map[string]any{"name": fuzzWords[rng.IntN(len(fuzzWords))]})
		vaults = append(vaults, v.Id)
	}
	var animals []string
	for i := range 20 {
		a := z.rec(t, fmt.Sprintf("a%d", i), "zoo_animals", map[string]any{
			"name": fuzzWords[rng.IntN(len(fuzzWords))], "species": fuzzWords[rng.IntN(len(fuzzWords))],
			"site": sites[rng.IntN(len(sites))], "owner": readers[rng.IntN(len(readers))],
			"vault":  vaults[rng.IntN(len(vaults))],
			"weight": rng.IntN(400), "price": rng.IntN(100000),
		})
		animals = append(animals, a.Id)
	}
	for i := range 25 {
		user := readers[rng.IntN(3)]
		z.rec(t, fmt.Sprintf("kk%d", i), "zoo_keepers", map[string]any{"user": user, "animal": animals[rng.IntN(len(animals))]})
	}
	for i := range 60 {
		z.rec(t, fmt.Sprintf("ff%d", i), "zoo_feedings", map[string]any{
			"animal": animals[rng.IntN(len(animals))], "cost": rng.IntN(5000), "grams": rng.IntN(900),
			"currency": []string{"USD", "EUR"}[rng.IntN(2)], "note": fuzzWords[rng.IntN(len(fuzzWords))],
			"fed": fmt.Sprintf("2026-%02d-%02d 00:00:00.000Z", 1+rng.IntN(12), 1+rng.IntN(28)),
		})
	}
}

// fuzzFilter builds one random condition as a spec node and as the same
// PocketBase filter.
func fuzzFilter(rng *rand.Rand, src fuzzSource, depth int) (string, string) {
	leaves := len(src.numbers) + len(src.texts)
	if leaves == 0 {
		return "", ""
	}
	if depth > 0 && rng.IntN(3) == 0 {
		a, pa := fuzzFilter(rng, src, depth-1)
		b, pb := fuzzFilter(rng, src, depth-1)
		op, pop := "and", "&&"
		if rng.IntN(2) == 0 {
			op, pop = "or", "||"
		}
		return fmt.Sprintf(`{"op":%q,"args":[%s,%s]}`, op, a, b), "(" + pa + " " + pop + " " + pb + ")"
	}
	i := rng.IntN(leaves)
	if i < len(src.numbers) {
		field := src.numbers[i]
		ops := [][2]string{{"eq", "="}, {"ne", "!="}, {"gt", ">"}, {"lt", "<"}, {"gte", ">="}, {"lte", "<="}}
		o := ops[rng.IntN(len(ops))]
		v := rng.IntN(5000)
		return fmt.Sprintf(`{"op":%q,"args":[{"path":[{"field":%q}]},{"value":%d}]}`, o[0], field, v), fmt.Sprintf("%s %s %d", field, o[1], v)
	}
	field := src.texts[i-len(src.numbers)]
	words := append(append([]string{}, fuzzWords...), "USD", "EUR", "North")
	v := words[rng.IntN(len(words))]
	if rng.IntN(2) == 0 {
		return fmt.Sprintf(`{"op":"eq","args":[{"path":[{"field":%q}]},{"value":%q}]}`, field, v), fmt.Sprintf("%s = %q", field, v)
	}
	return fmt.Sprintf(`{"op":"ne","args":[{"path":[{"field":%q}]},{"value":%q}]}`, field, v), fmt.Sprintf("%s != %q", field, v)
}

// listExpanded reads id → the expanded links' fields through the list API.
func (z *zoo) listExpanded(t *testing.T, mux http.Handler, src fuzzSource, user, filter string) map[string]any {
	t.Helper()
	q := url.Values{"perPage": {"500"}, "skipTotal": {"1"}}
	if filter != "" {
		q.Set("filter", filter)
	}
	expand := ""
	for _, l := range src.links {
		if expand != "" {
			expand += ","
		}
		expand += l.link
	}
	if expand != "" {
		q.Set("expand", expand)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+src.collection+"/records?"+q.Encode(), nil)
	req.Header.Set("Authorization", z.token(t, user))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s %q: %d %s", src.collection, filter, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, item := range body.Items {
		linked := make([]any, len(src.links))
		if expanded, ok := item["expand"].(map[string]any); ok {
			for i, l := range src.links {
				if rec, ok := expanded[l.link].(map[string]any); ok {
					linked[i] = rec[l.field]
				}
			}
		}
		out[item["id"].(string)] = linked
	}
	return out
}

func (z *zoo) engineExpanded(t *testing.T, src fuzzSource, user, filter string) map[string]any {
	t.Helper()
	cols := `{"id":"id","path":[{"field":"id"}]}`
	for i, l := range src.links {
		cols += fmt.Sprintf(`,{"id":"linked%d","path":[{"link":%q},{"field":%q}]}`, i, l.link, l.field)
	}
	spec := fmt.Sprintf(`{"version":4,"source":%q,"columns":[%s]`, src.key, cols)
	if filter != "" {
		spec += `,"filter":` + filter
	}
	res := z.run(t, user, spec+"}", Options{Limit: 1000}, nil)
	out := map[string]any{}
	for _, row := range res.Rows {
		if row.Kind == KindDetail {
			linked := make([]any, len(src.links))
			for i := range src.links {
				linked[i] = row.Cells[fmt.Sprintf("linked%d", i)]
			}
			out[row.Cells["id"].(string)] = linked
		}
	}
	return out
}

func FuzzRuleParity(f *testing.F) {
	for seed := range uint64(40) {
		f.Add(seed)
	}
	// One grown zoo serves every seed this process runs; a seed only reads.
	z := newZoo(f)
	growZoo(f, z, rand.New(rand.NewPCG(1, 2)))
	mux := z.mux(f)
	f.Fuzz(func(t *testing.T, seed uint64) {
		rng := rand.New(rand.NewPCG(seed, 7))
		src := fuzzSources[rng.IntN(len(fuzzSources))]
		user := []string{"ann", "bob", "carol"}[rng.IntN(3)]
		filter, pbFilter := fuzzFilter(rng, src, 2)
		got := z.engineExpanded(t, src, user, filter)
		want := z.listExpanded(t, mux, src, user, pbFilter)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: %s as %s, filter %s\nengine: %v\nlist:   %v", seed, src.key, user, pbFilter, keys(got), keys(want))
		}
		z.lookupParity(t, mux, user)
		z.fillParity(t, mux, user)
		z.sumLookupParity(t, mux, user)
		z.pagedGroupParity(t, rng, user, pagedShapes[seed%uint64(len(pagedShapes))], 1+rng.IntN(5))
	})
}

// lookupParity: a lookup reads exactly what the list API lists for the reader.
func (z *zoo) lookupParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	res := z.run(t, user, `{"version":4,"source":"zoo.feedings","columns":[{"id":"id","path":[{"field":"id"}]},
		{"id":"lk","formula":{"op":"lookup","source":"zoo.sites","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"North"}]},"value":{"path":[{"field":"id"}]}}}]}`,
		Options{Limit: 1000}, nil)
	// pick defaults to min: the smallest id among the sites the reader lists.
	var want any
	if ids := z.listIDs(t, mux, "zoo_sites", user, `name="North"`); len(ids) > 0 {
		want = slices.Min(ids)
	}
	details := 0
	for _, row := range res.Rows {
		if row.Kind != KindDetail {
			continue
		}
		details++
		if row.Cells["lk"] != want {
			t.Fatalf("%s: lookup = %v, list API = %v", user, row.Cells["lk"], want)
		}
	}
	if details == 0 {
		t.Fatalf("%s: no detail rows to check the lookup on", user)
	}
}

// fillParity: a fill adds exactly the animals the list API lists for the reader.
func (z *zoo) fillParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	res := z.run(t, user, `{"version":4,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],"groups":[{"id":"g","col":"a","fill":{}}]}`, Options{}, nil)
	got := map[string]bool{}
	for _, row := range res.Rows {
		if row.Kind == KindGroup && row.GroupKey[0] != nil {
			got[row.GroupKey[0].(string)] = true
		}
	}
	want := z.listIDs(t, mux, "zoo_animals", user, "")
	if len(got) != len(want) {
		t.Fatalf("%s: fill keys %v, list API %v", user, got, want)
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("%s: fill left out %s", user, id)
		}
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(out)
	return out
}

// listItems reads every field of a collection's rows through the list API
// as user.
func (z *zoo) listItems(t *testing.T, mux http.Handler, collection, user string) []map[string]any {
	t.Helper()
	q := url.Values{"perPage": {"500"}, "skipTotal": {"1"}}
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collection+"/records?"+q.Encode(), nil)
	req.Header.Set("Authorization", z.token(t, user))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s as %s: %d %s", collection, user, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}

// sumLookupParity: per feeding, a sum lookup of the grams of its animal's
// feedings, and a count of them, equal the sum and the count over the
// feedings the list API lists for the reader.
func (z *zoo) sumLookupParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	const sameAnimal = `{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]}`
	res := z.run(t, user, `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"total","formula":{"op":"lookup","source":"zoo.feedings","where":`+sameAnimal+`,"value":{"path":[{"field":"grams"}]},"pick":"sum"}},
		{"id":"n","formula":{"op":"lookup","source":"zoo.feedings","where":`+sameAnimal+`,"value":{"path":[{"field":"id"}]},"pick":"count"}}]}`,
		Options{Limit: 1000}, nil)
	sums, counts := map[string]int64{}, map[string]int64{}
	for _, item := range z.listItems(t, mux, "zoo_feedings", user) {
		animal, _ := item["animal"].(string)
		grams, _ := item["grams"].(float64)
		sums[animal] += int64(grams)
		counts[animal]++
	}
	for _, row := range res.Rows {
		if row.Kind != KindDetail {
			continue
		}
		animal, _ := row.Cells["animal"].(string)
		if !reflect.DeepEqual(row.Cells["total"], sums[animal]) || !reflect.DeepEqual(row.Cells["n"], counts[animal]) {
			t.Fatalf("%s: animal %s: sum %v count %v, list API %d and %d", user, animal, row.Cells["total"], row.Cells["n"], sums[animal], counts[animal])
		}
	}
}

// pagedAnimals groups feedings by animal (under their species when levels is
// 2), each open animal group starting with the sum of its animal's grams;
// running restarts in each group.
func pagedAnimals(levels int) string {
	species, bySpecies := "", ""
	if levels == 2 {
		species = `{"id":"species","path":[{"link":"animal"},{"field":"species"}]},`
		bySpecies = `{"id":"s","col":"species","open":true,"subtotals":true},`
	}
	return `{"version":4,"source":"zoo.feedings","columns":[` + species + `
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"running","formula":{"op":"runningSum","args":[{"col":"grams"}]}}],
		"groups":[` + bySpecies + `{"id":"g","col":"animal","open":true,"subtotals":true,"opening":{"label":"All feedings","cells":{"grams":
			{"op":"lookup","source":"zoo.feedings","where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
			"value":{"path":[{"field":"grams"}]},"pick":"sum"}}}}],
		"sort":[{"col":"note"}]}`
}

// pagedShape is one grouped report the paging parity runs: its group levels
// and its expansion state. "open" sends none (the open levels expand), "all"
// expands every group, "none" shuts every group, and "some" expands a random
// half of the groups, so seams fall next to shut groups.
type pagedShape struct {
	levels int
	expand string
}

// pagedShapes are taken in turn by seed, so the 40 seeds run each one five
// times: with two levels a seam falls between the levels.
var pagedShapes = []pagedShape{
	{1, "open"}, {1, "all"}, {1, "none"}, {1, "some"},
	{2, "open"}, {2, "all"}, {2, "none"}, {2, "some"},
}

// pagedGroupParity: the pages of a grouped report, joined as the client joins
// them (mergedRows: an earlier page's trailing totals dropped, no row on two
// pages), are exactly the one-page run.
func (z *zoo) pagedGroupParity(t *testing.T, rng *rand.Rand, user string, shape pagedShape, limit int) {
	t.Helper()
	raw := pagedAnimals(shape.levels)
	var opts Options
	switch shape.expand {
	case "all":
		opts.ExpandAll = true
	case "none":
		opts.Expand = [][]any{}
	case "some":
		opts.Expand = [][]any{}
		for _, row := range z.run(t, user, raw, Options{ExpandAll: true, Limit: 1000}, nil).Rows {
			if row.Kind == KindGroup && rng.IntN(2) == 0 {
				opts.Expand = append(opts.Expand, row.GroupKey)
			}
		}
	}
	opts.Limit = 1000
	whole := z.run(t, user, raw, opts, nil)
	opts.Limit = limit
	if merged := mergedRows(t, groupedPages(t, z, user, raw, opts)); !reflect.DeepEqual(merged, whole.Rows) {
		t.Fatalf("%s, %+v, limit %d: pages\n%v\nwhole\n%v", user, shape, limit, merged, whole.Rows)
	}
}
