package reports

import (
	"reflect"
	"testing"
)

func TestBucketSeries(t *testing.T) {
	for name, c := range map[string]struct {
		bucket, from, to string
		start, max       int
		keys             []any
		total            int
	}{
		"months":          {"month", "2026-01-15", "2026-03-10", 1, 50, []any{"2026-01", "2026-02", "2026-03"}, 3},
		"across a year":   {"month", "2025-11-30", "2026-02-01", 1, 50, []any{"2025-11", "2025-12", "2026-01", "2026-02"}, 4},
		"quarters":        {"quarter", "2026-02-01", "2026-09-30", 1, 50, []any{"2026-Q1", "2026-Q2", "2026-Q3"}, 3},
		"years":           {"year", "2025-06-01", "2026-01-01", 1, 50, []any{"2025", "2026"}, 2},
		"leap days":       {"day", "2028-02-27", "2028-03-01", 1, 50, []any{"2028-02-27", "2028-02-28", "2028-02-29", "2028-03-01"}, 4},
		"one day":         {"day", "2026-01-05", "2026-01-05", 1, 50, []any{"2026-01-05"}, 1},
		"fiscal years":    {"fiscalYear", "2026-01-01", "2026-12-31", 7, 50, []any{int64(2026), int64(2027)}, 2},
		"calendar fiscal": {"fiscalYear", "2026-01-01", "2026-12-31", 1, 50, []any{int64(2026)}, 1},
		"capped":          {"month", "2026-01-01", "2026-12-31", 1, 2, []any{"2026-01", "2026-02"}, 12},
	} {
		t.Run(name, func(t *testing.T) {
			keys, total := bucketSeries(c.bucket, c.from, c.to, c.start, c.max)
			if !reflect.DeepEqual(keys, c.keys) || total != c.total {
				t.Fatalf("got %#v (%d), want %#v (%d)", keys, total, c.keys, c.total)
			}
		})
	}
}

// Two centuries of days: three keys built, every day counted.
func TestBucketSeriesCountsWithoutBuildingPastTheCap(t *testing.T) {
	keys, total := bucketSeries("day", "1900-01-01", "2100-12-31", 1, 3)
	if want := []any{"1900-01-01", "1900-01-02", "1900-01-03"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v", keys)
	}
	// 201 years of 365 days and 49 leap days (1900 and 2100 are not leap years).
	if total != 73414 {
		t.Fatalf("total = %d, want 73414", total)
	}
}

// rangePivot is a feedings pivot with the given params and pivot.
func rangePivot(params, pivot string) string {
	return `{"version":4,"source":"zoo.feedings","params":[` + params + `],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":` + pivot + `}`
}

const periodParam = `{"id":"period","type":"dateRange"}`

func TestPivotRangeRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct {
		params, pivot, want string
		status              int
	}{
		"unknown param":   {periodParam, `{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","range":"when"}`, "no param", 422},
		"not a range":     {`{"id":"day","type":"date"}`, `{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","range":"day"}`, "a date range param", 400},
		"not over a date": {periodParam, `{"path":[{"field":"note"}],"cell":"cost","range":"period"}`, "only a pivot over a date", 400},
		"no bucket":       {periodParam, `{"path":[{"field":"fed"}],"cell":"cost","range":"period"}`, "a bucket", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", rangePivot(c.params, c.pivot), Options{}, nil)
			requireSpecError(t, err, c.status, "pivot.range", c.want)
		})
	}
}
