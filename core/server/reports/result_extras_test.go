package reports

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const threeParams = `{"version":1,"source":"zoo.feedings",
	"params":[
		{"id":"period","type":"dateRange","default":{"preset":"thisMonth"}},
		{"id":"t","type":"text"},
		{"id":"n","type":"number","default":5},
		{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"a","label":"A"},{"value":"b","label":"B"}]}],
	"columns":[{"id":"note","path":[{"field":"note"}]}]}`

func TestARunEchoesEachParamAsItResolved(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", threeParams, Options{Today: "2026-02-15"}, map[string]string{"t": `"lion"`, "kinds": `["b"]`})
	want := []ResolvedParam{
		{ID: "period", Type: "dateRange", From: "2026-02-01", To: "2026-02-28"},
		{ID: "t", Type: "text", Value: "lion"},
		{ID: "n", Type: "number", Value: int64(5)},
		{ID: "kinds", Type: "choice", List: []string{"b"}},
	}
	if !reflect.DeepEqual(res.Params, want) {
		t.Fatalf("params = %+v\nwant     %+v", res.Params, want)
	}
}

func TestAParamWithNoValueAndNoDefaultIsEchoedUnset(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", threeParams, Options{Today: "2026-02-15"}, nil)
	if got := res.Params[1]; got.ID != "t" || !got.Unset || got.Value != nil {
		t.Fatalf("t = %+v", got)
	}
	if got := res.Params[3]; got.ID != "kinds" || !got.Unset || got.List != nil {
		t.Fatalf("kinds = %+v", got)
	}
}

func TestARunWithoutParamsEchoesAnEmptyList(t *testing.T) {
	z := newZoo(t)
	b, err := json.Marshal(z.run(t, "ann", feedingsByDay, Options{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"params":[]`) {
		t.Fatalf("result JSON lacks an empty params list: %s", b)
	}
}

func TestAGroupRowSaysWhetherItCanOpen(t *testing.T) {
	z := newZoo(t)
	pivot := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	if r := pivot.Rows[0]; r.Kind != KindGroup || r.CanExpand {
		t.Fatalf("a pivot's innermost group = %+v, want a group that cannot open", r)
	}
	grouped := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","subtotals":true}]}`, Options{}, nil)
	if r := grouped.Rows[0]; r.Kind != KindGroup || !r.CanExpand {
		t.Fatalf("a group with detail rows = %+v, want one that can open", r)
	}
	if r := grouped.Rows[len(grouped.Rows)-1]; r.Kind != KindTotal || r.CanExpand {
		t.Fatalf("the total = %+v", r)
	}
}
