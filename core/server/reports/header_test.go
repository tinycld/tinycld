package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// E2: a spec's own document header lines, each a text with one {value} and a
// value the run reads once (params, values and lookups). The fictional
// package's one-row source zoo.floors holds a setting only its owner, ann,
// may list: bob runs zoo.feedings too (he keeps zed) but may not read it.

// addFloors adds the zoo_floors collection with its one row and registers
// zoo.floors (newZoo's cleanup resets the registry).
func addFloors(t *testing.T, z *zoo) {
	t.Helper()
	users, err := z.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	floors := core.NewBaseCollection("zoo_floors")
	floors.ListRule = rule(`owner = @request.auth.id`)
	floors.ViewRule = floors.ListRule
	floors.Fields.Add(
		&core.TextField{Name: "name"},
		&core.NumberField{Name: "floor"},
		&core.TextField{Name: "currency"},
		&core.DateField{Name: "since"},
		&core.NumberField{Name: "count"},
		&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1},
	)
	z.save(t, floors)
	z.rec(t, "floor", "zoo_floors", map[string]any{
		"name": "Zoo floor", "floor": 200000, "currency": "USD", "since": "2026-03-05 00:00:00.000Z", "count": 1234.5, "owner": z.id("ann"),
	})
	RegisterSource(Source{
		Key: "zoo.floors", Collection: "zoo_floors", Label: "Floors",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "floor", Label: "Floor", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "since", Label: "Since", Type: Date, Aggregates: MinMax},
			{Key: "count", Label: "Count", Type: Number, Aggregates: SumAll},
		},
	})
}

// floorLookup reads field of the one floors row (pick min).
func floorLookup(field string) string {
	return `{"op":"lookup","source":"zoo.floors","value":{"path":[{"field":"` + field + `"}]},"pick":"min"}`
}

// noFloorLookup matches no floors row.
const noFloorLookup = `{"op":"lookup","source":"zoo.floors",
	"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]},
	"value":{"path":[{"field":"floor"}]},"pick":"min"}`

// headerLine is one spec header line as JSON.
func headerLine(text, value string) string {
	return fmt.Sprintf(`{"text":%q,"value":%s}`, text, value)
}

// headedSpec is basisSpec's report (a choice with a param header line) with
// the spec header lines given.
func headedSpec(lines ...string) string {
	return `{"version":4,"source":"zoo.feedings","params":[
		{"id":"basis","type":"choice","default":"accrual","header":"{label} basis",
			"options":[{"value":"accrual","label":"Accrual"},{"value":"cash","label":"Cash"}]}],
		"columns":[{"id":"note","path":[{"field":"note"}]},
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"byAnimal","col":"animal"}],
		"sort":[{"col":"note"}],
		"header":[` + strings.Join(lines, ",") + `]}`
}

func requireHeaderLines(t *testing.T, res *Result, want ...string) {
	t.Helper()
	if !slices.Equal(res.HeaderLines, want) {
		t.Fatalf("headerLines = %#v\nwant        %#v", res.HeaderLines, want)
	}
}

// The param lines come first, then each spec line with its value written as
// an export writes a cell of the value's type: an amount with its currency,
// a day in the short form, a number grouped, a text as it is.
func TestHeaderLineWithLookup(t *testing.T) {
	z := newZoo(t)
	addFloors(t, z)
	res := z.run(t, "ann", headedSpec(
		headerLine("Floor: {value}", floorLookup("floor")),
		headerLine("Name: {value}", floorLookup("name")),
		headerLine("Since: {value}", floorLookup("since")),
		headerLine("Count: {value}", floorLookup("count")),
	), Options{}, nil)
	requireHeaderLines(t, res, "Accrual basis", "Floor: $2,000.00", "Name: Zoo floor", "Since: Mar 5, 2026", "Count: 1,234.5")
}

// A value that is blank (a lookup that matches no row) gives ” in its
// place: the entry stays, so the lines after it keep their index.
func TestHeaderLineBlankValue(t *testing.T) {
	z := newZoo(t)
	addFloors(t, z)
	res := z.run(t, "ann", headedSpec(
		headerLine("Floor: {value}", noFloorLookup),
		headerLine("Name: {value}", floorLookup("name")),
	), Options{}, nil)
	requireHeaderLines(t, res, "Accrual basis", "", "Name: Zoo floor")
}

// A lookup reads under the reader's list rule, as any lookup does: bob may
// not list the floors row, so every spec line is ” for him, with no error.
func TestHeaderLineReadsUnderTheReadersRules(t *testing.T) {
	z := newZoo(t)
	addFloors(t, z)
	spec := headedSpec(
		headerLine("Floor: {value}", floorLookup("floor")),
		headerLine("Name: {value}", floorLookup("name")),
	)
	res, err := z.try(t, "bob", spec, Options{}, nil)
	if err != nil {
		t.Fatalf("bob's run: %v", err)
	}
	requireHeaderLines(t, res, "Accrual basis", "", "")
	if len(res.Rows) == 0 {
		t.Fatal("bob's run has no rows")
	}
}

// A header line's lookup that names a field by an old key (the source's
// Aliases) reads the field's current key, as the same lookup in a column
// does, and the run reports the alias as it reports a column's.
func TestHeaderLineResolvesAliases(t *testing.T) {
	z := newZoo(t)
	addFloors(t, z)
	floors := *lookupRegistered("zoo.floors")
	floors.Aliases = map[string]string{"minimum": "floor"}
	RegisterSource(floors)
	aliasedOf := func(res *Result) []Alias {
		for _, n := range res.Notices {
			if n.Code == "aliased" {
				return n.Aliases
			}
		}
		return nil
	}
	column := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"fl","formula":`+floorLookup("minimum")+`}],"sort":[{"col":"note"}]}`, Options{}, nil)
	want := []Alias{{Source: "zoo.floors", From: "minimum", To: "floor"}}
	if got := aliasedOf(column); !slices.Equal(got, want) {
		t.Fatalf("the column's aliases = %+v, want %+v", got, want)
	}
	res, err := z.try(t, "ann", headedSpec(headerLine("Floor: {value}", floorLookup("minimum"))), Options{}, nil)
	if err != nil {
		t.Fatalf("a header line's lookup by an old key: %v", err)
	}
	requireHeaderLines(t, res, "Accrual basis", "Floor: $2,000.00")
	if got := aliasedOf(res); !slices.Equal(got, want) {
		t.Fatalf("the header line's aliases = %+v, want %+v (as for a column)", got, want)
	}
}

// headerSpecError is the first refusal of raw: at Load, at Validate, or at
// the run.
func headerSpecError(t *testing.T, z *zoo, raw string) error {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		return err
	}
	if err := Validate(z.app, spec); err != nil {
		return err
	}
	_, err = z.try(t, "ann", raw, Options{}, nil)
	return err
}

// requireHeaderProblem fails unless err is a 400 with a problem at at (or a
// part under it) and none at another header line.
func requireHeaderProblem(t *testing.T, err error, at string) {
	t.Helper()
	se, ok := err.(*SpecError)
	if !ok {
		t.Fatalf("err = %v, want a *SpecError at %s", err, at)
	}
	if se.Status != 400 {
		t.Fatalf("status = %d, want 400 (%v)", se.Status, se)
	}
	found := false
	for k := range se.Problems {
		switch {
		case k == at || strings.HasPrefix(k, at+"."):
			found = true
		case strings.HasPrefix(k, "header"):
			t.Fatalf("a problem at %q, want it at %s only: %v", k, at, se)
		}
	}
	if !found {
		t.Fatalf("no problem at %s: %v", at, se)
	}
}

// Each line is text with {value} exactly once and a value of params, values
// and lookups of a text, day, number or amount type: any other shape is a 400
// at header.<i>.
func TestHeaderLineRefusals(t *testing.T) {
	z := newZoo(t)
	addFloors(t, z)
	floor := floorLookup("floor")
	cases := map[string]string{
		"no {value}":  headerLine("Floor", floor),
		"two {value}": headerLine("{value} and {value}", floor),
		"a row field": headerLine("Note: {value}", `{"path":[{"field":"note"}]}`),
		"a column":    headerLine("Cost: {value}", `{"col":"cost"}`),
		// A total over a value a header line may read, so only the total
		// refusal refuses it.
		"a total":      headerLine("Floor: {value}", `{"op":"sum","args":[`+floor+`]}`),
		"a record":     headerLine("Animal: {value}", `{"record":"byAnimal","path":[{"field":"name"}]}`),
		"a yes or no":  headerLine("Open: {value}", `{"value":true}`),
		"a lookup yes": headerLine("Open: {value}", `{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"active"}]},"pick":"min"}`),
		// A line's own shape: a text that is not a string, a key a line does
		// not have.
		"a text of a number": `{"text":1,"value":` + floor + `}`,
		"another key":        `{"text":"Floor: {value}","value":` + floor + `,"label":"Floor"}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			requireHeaderProblem(t, headerSpecError(t, z, headedSpec(line)), "header.0")
		})
	}
	for name, second := range map[string]string{
		"a column":           headerLine("Cost: {value}", `{"col":"cost"}`),
		"a text of a number": `{"text":1,"value":` + floor + `}`,
		"another key":        `{"text":"Floor: {value}","value":` + floor + `,"label":"Floor"}`,
	} {
		t.Run("the second of two lines is "+name, func(t *testing.T) {
			raw := headedSpec(headerLine("Floor: {value}", floor), second)
			requireHeaderProblem(t, headerSpecError(t, z, raw), "header.1")
		})
	}
	for name, header := range map[string]string{
		"a text":           `"Floor: {value}"`,
		"an object":        `{"text":"Floor: {value}","value":{"value":1}}`,
		"a list of texts":  `["Floor: {value}"]`,
		"a list of number": `[1]`,
	} {
		t.Run("header is "+name, func(t *testing.T) {
			raw := strings.Replace(headedSpec(), `"header":[]`, `"header":`+header, 1)
			err := headerSpecError(t, z, raw)
			se, ok := err.(*SpecError)
			if !ok || se.Status != 400 {
				t.Fatalf("err = %v, want a 400 at header", err)
			}
			if _, ok := se.Problems["header"]; !ok {
				t.Fatalf("no problem at header: %v", se)
			}
		})
	}
}

// A value that reads a param prints the param's value, and follows it; an
// unset param prints no line.
func TestHeaderLineParamValue(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","params":[
		{"id":"note","type":"text","label":"Note"},
		{"id":"min","type":"number","label":"At least"}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}],
		"header":[` + headerLine("Note: {value}", `{"param":"note"}`) + `,` + headerLine("At least {value}", `{"param":"min"}`) + `]}`
	res := z.run(t, "ann", spec, Options{}, map[string]string{"note": `"fed daily"`, "min": `2500`})
	requireHeaderLines(t, res, "Note: fed daily", "At least 2,500")
	res = z.run(t, "ann", spec, Options{}, map[string]string{"note": `"hay only"`, "min": `3`})
	requireHeaderLines(t, res, "Note: hay only", "At least 3")
	res = z.run(t, "ann", spec, Options{}, map[string]string{"min": `3`})
	requireHeaderLines(t, res, "", "At least 3")
}

// exportedSpec is the feedings over a period with the spec header lines given
// (none: no header key at all).
func exportedSpec(lines ...string) string {
	header := ""
	if len(lines) > 0 {
		header = `,"header":[` + strings.Join(lines, ",") + `]`
	}
	return `{"version":4,"source":"zoo.feedings","params":[
		{"id":"period","type":"dateRange","label":"Period"}],
		"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		"sort":[{"col":"note"}]` + header + `}`
}

// The PDF and the Excel file print each spec line that is not blank directly
// under the period line; the CSV prints no header and is the same file as
// the report's with no header.
func TestHeaderLinesInExports(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	addFloors(t, s.zoo)
	headed := s.saved(t, "headed", "ann", "Floor Costs", exportedSpec(
		headerLine("Floor: {value}", floorLookup("floor")),
		headerLine("Blank: {value}", noFloorLookup),
		headerLine("Name: {value}", floorLookup("name")),
	), false)
	plain := s.saved(t, "plain", "ann", "Floor Costs", exportedSpec(), false)
	params := `{"period":{"from":"2026-01-01","to":"2026-03-31"}}`
	options := `{"today":"2026-11-16","timeZone":"UTC"}`

	sheet := sheetOf(t, buildExportAs(t, s, "ann", headed.Id, "xlsx", params, options).Bytes)
	for row, want := range map[int]string{
		3: "January 1 – March 31, 2026",
		4: "Floor: $2,000.00",
		5: "Name: Zoo floor",
	} {
		if got := sheetString(t, sheet, row, 1); got != want {
			t.Fatalf("Excel row %d = %q, want %q", row, got, want)
		}
	}
	for row := 1; row <= len(sheet.Cells); row++ {
		if c := sheetCell(sheet, row, 1); strings.HasPrefix(c.Value.S, "Blank:") {
			t.Fatalf("Excel row %d prints the blank line %q", row, c.Value.S)
		}
	}

	// The PDF's text extractor reads the table's label column before the page
	// header, so the order is read from the page the PDF is made of.
	var page string
	pdfWriter := exportWriters["pdf"]
	t.Cleanup(func() { exportWriters["pdf"] = pdfWriter })
	capture := pdfWriter
	capture.Write = func(ctx context.Context, d *exportData) ([]byte, error) {
		page = pdfHTML(t, d)
		return pdfWriter.Write(ctx, d)
	}
	exportWriters["pdf"] = capture
	text, _ := pdfTextOf(t, buildExportAs(t, s, "ann", headed.Id, "pdf", params, options).Bytes)
	requireInOrder(t, page, "January 1 – March 31, 2026", "Floor: $2,000.00", "Name: Zoo floor", "<table")
	if strings.Contains(page, "Blank:") || strings.Contains(text, "Blank:") {
		t.Fatalf("the PDF prints the blank line:\n%s", page)
	}

	headedCSV := buildExportAs(t, s, "ann", headed.Id, "csv", params, options).Bytes
	plainCSV := buildExportAs(t, s, "ann", plain.Id, "csv", params, options).Bytes
	if string(headedCSV) != string(plainCSV) {
		t.Fatalf("the CSV with header lines differs from the one without:\n%q\n%q", headedCSV, plainCSV)
	}
}

// Pin: a spec with no header loads to JSON with no header key, and its run
// gives the param lines it gives today.
func TestSpecWithoutHeaderIsUnchanged(t *testing.T) {
	z := newZoo(t)
	spec, _, err := Load([]byte(basisSpec("")))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["header"]; ok {
		t.Fatalf("a spec with no header has a header key: %s", body)
	}
	requireHeaderLines(t, z.run(t, "ann", basisSpec(""), Options{}, nil), "Accrual basis")
	requireHeaderLines(t, z.run(t, "ann", feedingsByDay, Options{}, nil))
}
