package reports

import (
	"strings"
	"testing"
)

// A text column that holds YYYY-MM-DD reads like a date: compared, bucketed
// and grouped by its day.
func TestATextDayReadsLikeADate(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"due","path":[{"field":"due"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"filter":{"op":"lte","args":[{"path":[{"field":"due"}]},{"value":"2026-02-10"}]},
		"groups":[{"id":"m","col":"due","bucket":"month"}]}`, Options{}, nil)
	requireRows(t, res, "label", "2026-01", "2026-02", "Total")
	requireRows(t, res, "cost", int64(1700), int64(2500), int64(4200))
	if res.Columns[0].Type != Date {
		t.Fatalf("due column = %+v", res.Columns[0])
	}
}

func TestCheckSourcesWantsATextColumnForATextDay(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.bad_day", Collection: "zoo_feedings", Label: "Bad",
		Fields: []Field{{Key: "grams", Label: "Grams", Type: Date, TextDay: true}}})
	err := CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `column "grams" is a number field`) {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyADateFieldReadsATextDay(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "only a date field reads a text day") {
			t.Fatalf("recover = %v", r)
		}
	}()
	RegisterSource(Source{Key: "zoo.bad_text", Collection: "zoo_feedings", Label: "Bad",
		Fields: []Field{{Key: "note", Label: "Note", Type: Text, TextDay: true}}})
}
