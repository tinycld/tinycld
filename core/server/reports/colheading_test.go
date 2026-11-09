package reports

import (
	"fmt"
	"testing"
)

const zooHeadings = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"asOf","type":"asOf","label":"As of"},{"id":"note","type":"text","label":"Note"}],
	"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","label":"Current","path":[{"field":"cost"}],"aggregate":"sum","heading":{"param":"asOf"}},
	{"id":"prior","label":"Prior year","path":[{"field":"cost"}],"aggregate":"sum","heading":{"op":"addMonths","args":[{"param":"asOf"},{"value":-12}]}},
	{"id":"grams","label":"Grams","path":[{"field":"grams"}],"aggregate":"sum","heading":%s}]}`

func headingOf(res *Result, id string) *ColumnHeading {
	for _, c := range res.Columns {
		if c.ID == id {
			return c.Heading
		}
	}
	return nil
}

func TestColumnHeadingsFromParams(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooHeadings, `{"param":"note"}`), Options{}, map[string]string{"asOf": `"2026-07-31"`, "note": `"Grams fed"`})
	if h := headingOf(res, "cost"); h == nil || h.Value != "2026-07-31" || h.Type != Date {
		t.Fatalf("cost heading = %+v", h)
	}
	if h := headingOf(res, "prior"); h == nil || h.Value != "2025-07-31" || h.Type != Date {
		t.Fatalf("prior heading = %+v", h)
	}
	if h := headingOf(res, "grams"); h == nil || h.Value != "Grams fed" || h.Type != Text {
		t.Fatalf("grams heading = %+v", h)
	}
	// The label stays the column's name.
	if res.Columns[1].Label != "Current" {
		t.Fatalf("label = %q", res.Columns[1].Label)
	}
}

func TestAnUnsetParamGivesNoHeading(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooHeadings, `{"param":"note"}`), Options{}, map[string]string{"asOf": `"2026-07-31"`})
	if h := headingOf(res, "grams"); h != nil {
		t.Fatalf("grams heading = %+v, want none", h)
	}
	if headingOf(res, "note") != nil {
		t.Fatal("a column with no heading formula has a heading")
	}
}

func TestABlankTextHeadingGivesNoHeading(t *testing.T) {
	z := newZoo(t)
	for name, note := range map[string]string{"empty": `""`, "spaces": `"  "`} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(zooHeadings, `{"param":"note"}`), Options{}, map[string]string{"asOf": `"2026-07-31"`, "note": note})
			if h := headingOf(res, "grams"); h != nil {
				t.Fatalf("grams heading = %+v, want none", h)
			}
		})
	}
}

func TestHeadingRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ heading, want string }{
		"path leaf": {`{"path":[{"field":"note"}]}`, "params and values"},
		"aggregate": {`{"op":"sum","args":[{"path":[{"field":"grams"}]}]}`, "params and values"},
		"column":    {`{"col":"cost"}`, "params and values"},
		"yes or no": {`{"value":true}`, "text, a day or a number"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(zooHeadings, c.heading)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, "columns.3.heading", c.want)
		})
	}
}
