package reports

import (
	"fmt"
	"testing"
)

// zooHide groups Ann's feedings by levels (group JSON objects) above an animal
// level whose having is the second argument.
func zooHide(levels, having string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
		{"id":"side","formula":{"op":"if","args":[{"op":"in","args":[{"col":"species"},{"value":["lion","cat"]}]},{"value":"felines"},{"value":"others"}]}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[` + levels + `,{"id":"a","col":"animal","having":` + having + `}]}`
}

const costOver = `{"op":"gt","args":[{"col":"cost"},{"value":%d}]}`
const costUnder = `{"op":"lt","args":[{"col":"cost"},{"value":%d}]}`

func TestHideEmptyHidesATypeAndItsClassWhenEveryRowIsHidden(t *testing.T) {
	z := raggedZoo(t)
	// Region (hideEmpty) › species (hideEmpty) › animal: only Leo (3500) passes.
	raw := zooHide(`{"id":"r","col":"region","hideEmpty":true,"subtotals":true,"open":true},
		{"id":"s","col":"species","hideEmpty":true,"subtotals":true,"open":true}`, fmt.Sprintf(costOver, 2000))
	res := z.run(t, "ann", raw, Options{}, nil)
	// zebra (Zed hidden) goes, South with it; cat (Kit hidden) goes, (none) with it.
	requireRows(t, res, "label", "North", "lion", "Leo", "Total lion", "Total North", "Total")
	// A "rows" total adds only what shows; a recomputed one counts every row the
	// filter keeps, as with any having.
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(3500))
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(4599))
}

func TestHideEmptyKeepsAGroupWithAShownSkippedRow(t *testing.T) {
	z := raggedZoo(t)
	// Side (hideEmpty) › region (skip, hideEmpty) › animal: only Kit (99) passes.
	raw := zooHide(`{"id":"d","col":"side","hideEmpty":true,"subtotals":true,"open":true},
		{"id":"r","col":"region","blank":"skip","hideEmpty":true,"subtotals":true,"open":true}`, fmt.Sprintf(costUnder, 200))
	res := z.run(t, "ann", raw, Options{}, nil)
	// felines keeps Kit, who sits directly under it; North (Leo hidden) goes;
	// others (South: Zed hidden) goes.
	requireRows(t, res, "label", "felines", "Kit", "Total felines", "Total")
	requireRows(t, res, "depth", 0, 1, 0, 0)
}

func TestHideEmptyHidesAClassWhoseRowsAllSkipTheKindLevel(t *testing.T) {
	z := raggedZoo(t)
	// Species (hideEmpty) › region (skip, hideEmpty) › animal over 200: cat's
	// only row (Kit, no region) skips the region level, and the having hides it.
	raw := zooHide(`{"id":"s","col":"species","hideEmpty":true,"subtotals":true,"open":true},
		{"id":"r","col":"region","blank":"skip","hideEmpty":true,"subtotals":true,"open":true}`, fmt.Sprintf(costOver, 200))
	res := z.run(t, "ann", raw, Options{}, nil)
	requireRows(t, res, "label", "lion", "North", "Leo", "Total North", "Total lion", "zebra", "South", "Zed", "Total South", "Total zebra", "Total")
}

// hideEmpty above a level that hides nothing keeps every group (each has a
// shown child) and runs: no count of shown children is read.
func TestHideEmptyAboveAPlainLevelKeepsEveryGroup(t *testing.T) {
	z := raggedZoo(t)
	raw := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","hideEmpty":true,"subtotals":true,"open":true},{"id":"a","col":"animal"}],
		"sort":[{"col":"species"}]}`
	res := z.run(t, "ann", raw, Options{}, nil)
	requireRows(t, res, "label", "cat", "Kit", "Total cat", "lion", "Leo", "Total lion", "zebra", "Zed", "Total zebra", "Total")
}

func TestHideEmptyRefusedOnTheLastLevel(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"a","col":"animal","hideEmpty":true}]}`
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	requireSpecError(t, Validate(z.app, spec), 400, "groups.0.hideEmpty", "the last level")
}

// A layout's sections always show, also when empty, as with a having.
func TestHideEmptyRefusedOnALayoutsSections(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","hideEmpty":true},{"id":"a","col":"animal"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	requireSpecError(t, Validate(z.app, spec), 400, "groups.0.hideEmpty", "a layout's sections are fixed rows")
}
