package reports

import (
	"testing"
	"time"
)

// drilledByMonth pivots ann's grams by month of fed over the period's
// months; a pivot cell drills to its month cut to the period.
const drilledByMonth = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"},"period":{"param":"period"}}},
			"pivotCells":{"period":{"range":{"from":{"pivotBound":"from"},"to":{"pivotBound":"to"}}}}}}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams","range":"period"}}`

func TestAPivotCellDrillsToItsBucketCutToTheRange(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledByMonth, Options{}, map[string]string{"period": `{"from":"2026-03-10","to":"2026-05-20"}`})
	lion := res.Rows[0]
	if lion.Label != "lion" {
		t.Fatalf("first row = %s", lion.Label)
	}
	for col, want := range map[string]string{
		"grams.p0": `{"period":{"from":"2026-03-10","to":"2026-03-31"},"species":"lion"}`,
		"grams.p1": `{"period":{"from":"2026-04-01","to":"2026-04-30"},"species":"lion"}`,
		"grams.p2": `{"period":{"from":"2026-05-01","to":"2026-05-20"},"species":"lion"}`,
		// The cell column (the pivot's total) drills with the level's values alone.
		"grams": `{"period":{"from":"2026-03-10","to":"2026-05-20"},"species":"lion"}`,
	} {
		if got := drillJSON(t, lion, col); got != want {
			t.Errorf("%s drill = %s, want %s", col, got, want)
		}
	}
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Drill == nil {
			t.Fatalf("pivot column %s has no drill", c.ID)
		}
	}
}

func TestAPivotCellOfABlankDateHasNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "lion", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.rec(t, "f8", "zoo_feedings", map[string]any{"animal": nub.Id, "grams": 5, "cost": 1, "currency": "USD", "note": "f8"})
	// No range: the columns are the months that have rows, and "(none)".
	res := s.run(t, "ann", drilledByMonth, Options{}, nil)
	lion := res.Rows[0]
	if lion.Label != "lion" {
		t.Fatalf("first row = %s", lion.Label)
	}
	// The level's period copies the run's, which has none.
	if got, want := drillJSON(t, lion, "grams"), `{"period":null,"species":"lion"}`; got != want {
		t.Fatalf("grams drill = %s, want %s", got, want)
	}
	blankColumn, months := false, 0
	for _, c := range res.Columns {
		if c.Pivot == nil {
			continue
		}
		if c.Pivot.Key == nil {
			blankColumn = true
			for _, r := range res.Rows {
				if got := drillJSON(t, r, c.ID); got != "" {
					t.Fatalf("the blank date's column drills on %s: %s", r.Label, got)
				}
			}
			continue
		}
		// With no range, a month column drills to its whole month.
		months++
		first, last := wholeMonth(t, c.Pivot.Key)
		want := `{"period":{"from":"` + first + `","to":"` + last + `"},"species":"lion"}`
		if got := drillJSON(t, lion, c.ID); got != want {
			t.Errorf("%s drill = %s, want %s", c.ID, got, want)
		}
	}
	if !blankColumn {
		t.Fatal("no blank date column")
	}
	if months == 0 {
		t.Fatal("no month column")
	}
}

// wholeMonth is the first and last day of month key ("2006-01").
func wholeMonth(t *testing.T, key any) (string, string) {
	t.Helper()
	text, _ := key.(string)
	start, err := time.Parse("2006-01", text)
	if err != nil {
		t.Fatalf("month key %v: %v", key, err)
	}
	return start.Format(time.DateOnly), start.AddDate(0, 1, -1).Format(time.DateOnly)
}

// A drill with no pivotCells draws no pivot cell, so the pivot columns
// name no target; the cell column keeps its own.
func TestAPivotColumnHasNoDrillWithoutPivotCells(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	raw := `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"}}}}}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams","range":"period"}}`
	res := s.run(t, "ann", raw, Options{}, map[string]string{"period": `{"from":"2026-03-10","to":"2026-05-20"}`})
	pivots := 0
	for _, c := range res.Columns {
		switch {
		case c.ID == "grams" && c.Drill == nil:
			t.Fatal("the cell column has no drill")
		case c.Pivot != nil && c.Drill != nil:
			t.Fatalf("pivot column %s has a drill but no pivot cell draws one", c.ID)
		case c.Pivot != nil:
			pivots++
		}
	}
	if pivots == 0 {
		t.Fatal("no pivot column")
	}
}

// A layout section's row is a group row: its pivot cells drill to their
// bucket cut to the range, as in a grouped report.
func TestALayoutSectionsPivotCellDrills(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	raw := `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange"}],
	"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"},"period":{"param":"period"}}},
			"pivotCells":{"period":{"range":{"from":{"pivotBound":"from"},"to":{"pivotBound":"to"}}}}}}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams","range":"period"},
	"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"},
		{"row":"both","label":"Both","formula":{"op":"add","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}]}`
	res := s.run(t, "ann", raw, Options{}, map[string]string{"period": `{"from":"2026-03-10","to":"2026-05-20"}`})
	var lions, both *Row
	for i := range res.Rows {
		switch res.Rows[i].Label {
		case "Lions":
			lions = &res.Rows[i]
		case "Both":
			both = &res.Rows[i]
		}
	}
	if lions == nil || both == nil {
		t.Fatalf("no Lions or Both row in %d rows", len(res.Rows))
	}
	for col, want := range map[string]string{
		"grams.p0": `{"period":{"from":"2026-03-10","to":"2026-03-31"},"species":"lion"}`,
		"grams.p1": `{"period":{"from":"2026-04-01","to":"2026-04-30"},"species":"lion"}`,
		"grams.p2": `{"period":{"from":"2026-05-01","to":"2026-05-20"},"species":"lion"}`,
		"grams":    `{"period":{"from":"2026-03-10","to":"2026-05-20"},"species":"lion"}`,
	} {
		if got := drillJSON(t, *lions, col); got != want {
			t.Errorf("Lions %s drill = %s, want %s", col, got, want)
		}
		// The computed row carries the drill cells blank.
		if got := drillJSON(t, *both, col); got != "" {
			t.Errorf("Both %s drill = %s, want none", col, got)
		}
	}
}

func TestBucketDays(t *testing.T) {
	for _, c := range []struct {
		bucket      string
		key         any
		fiscal      int
		first, last string
		ok          bool
	}{
		{"day", "2026-03-10", 1, "2026-03-10", "2026-03-10", true},
		{"month", "2026-03", 1, "2026-03-01", "2026-03-31", true},
		{"month", "2028-02", 1, "2028-02-01", "2028-02-29", true},
		{"quarter", "2026-Q2", 1, "2026-04-01", "2026-06-30", true},
		{"year", "2026", 1, "2026-01-01", "2026-12-31", true},
		{"fiscalYear", int64(2026), 1, "2026-01-01", "2026-12-31", true},
		{"fiscalYear", int64(2027), 4, "2026-04-01", "2027-03-31", true},
		{"month", nil, 1, "", "", false},
		{"month", "March", 1, "", "", false},
		{"quarter", "2026-Q5", 1, "", "", false},
		{"fiscalYear", "2026", 1, "", "", false},
	} {
		first, last, ok := bucketDays(c.bucket, c.key, c.fiscal)
		if first != c.first || last != c.last || ok != c.ok {
			t.Errorf("bucketDays(%s, %v, %d) = %s %s %v, want %s %s %v", c.bucket, c.key, c.fiscal, first, last, ok, c.first, c.last, c.ok)
		}
	}
}

func TestPivotCellsAreRefusedOffTheCellColumn(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	raw := `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","drill":{"report":"zoo.drill_target",
			"levels":{"g":{"species":{"groupKey":"g"}}},"pivotCells":{"period":{"param":"period"}}}}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"}}`
	requireSpecError(t, z.validate(t, raw), 400, "columns.2.drill.pivotCells", "the pivot's cell column")
}
