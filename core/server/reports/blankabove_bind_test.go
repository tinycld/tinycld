package reports

import (
	"errors"
	"fmt"
	"testing"
)

// blankAboveSpec is a feedings report grouped by species, then animal; %s is
// the cost column's extra keys, %s more keys of the spec.
const blankAboveSpec = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"%s},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
	{"id":"animal","path":[{"link":"animal"}],"hidden":true}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true},{"id":"byAnimal","col":"animal","subtotals":true}]%s}`

func TestBlankAboveIsAcceptedOnAGroupedMoneyColumn(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(fmt.Sprintf(blankAboveSpec, `,"blankAbove":"byAnimal"`, "")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if spec.Columns[1].BlankAbove != "byAnimal" {
		t.Fatalf("blankAbove = %q", spec.Columns[1].BlankAbove)
	}
	if err := Validate(app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestBlankAboveRefusals(t *testing.T) {
	app := withZooRegistry(t)
	pivot := `,"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}`
	for _, c := range []struct {
		name, col, rest, at, want string
		status                    int
	}{
		{"unknown group", `,"blankAbove":"nope"`, "", "columns.1.blankAbove", `the report has no group "nope"`, 422},
		// The first switch case wins: an unknown group hides the rollup refusal.
		{"unknown group and a rollup", `,"blankAbove":"nope","rollup":{"value":0}`, "", "columns.1.blankAbove", `the report has no group "nope"`, 422},
		{"with a rollup", `,"blankAbove":"byAnimal","rollup":{"value":0}`, "", "columns.1.blankAbove", "a rollup already replaces", 400},
		{"with a rows total", `,"blankAbove":"byAnimal","total":"rows"`, "", "columns.1.blankAbove", "rows shown", 400},
		{"on a pivot's cell column", `,"blankAbove":"byAnimal"`, pivot, "columns.1.blankAbove", "pivot's cell column", 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(blankAboveSpec, c.col, c.rest)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}

	grouped := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}],"blankAbove":"g"}],
		"groups":[{"id":"g","col":"species"}]}`
	spec, _, err := Load([]byte(grouped))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.1.blankAbove", "labels that group's rows, so it cannot be blank on them")

	nested := `{"version":4,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"species","path":[{"field":"species"}]},
		{"id":"cost","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}],"blankAbove":"g"}],
		"groups":[{"id":"g","col":"species"}]}`
	spec, _, err = Load([]byte(nested))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.2.blankAbove", "child row column")
}

// A having on a group above the column's level would read blank for every
// group, so bind refuses it, naming blankAbove; on the level it is fine.
func TestBlankAboveRefusesAHavingThatReadsTheColumnAboveItsLevel(t *testing.T) {
	app := withZooRegistry(t)
	const spec = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true}],
		"groups":[{"id":"bySpecies","col":"species"%s},{"id":"byAnimal","col":"animal"%s}]}`
	having := `,"having":{"op":"gt","args":[{"col":"%s"},{"value":0}]}`
	for _, c := range []struct {
		name, col string
		at        string
	}{
		{"a column", "cost", "groups.0.having"},
		{"a column that reads it", "twice", "groups.0.having"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _, err := Load([]byte(fmt.Sprintf(spec, fmt.Sprintf(having, c.col), "")))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(app, s), 400, c.at, `having of group "bySpecies" reads column "cost", which is blank above group "byAnimal"`)
		})
	}
	s, _, err := Load([]byte(fmt.Sprintf(spec, "", fmt.Sprintf(having, "cost"))))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(app, s); err != nil {
		t.Fatalf("a having on the level itself: %v", err)
	}
}

// A percentOfSection of a column blank above the sections divides by a blank
// total, so bind refuses it.
func TestBlankAboveRefusesAPercentOfSectionOfTheColumn(t *testing.T) {
	app := withZooRegistry(t)
	const spec = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"%s"},
		{"id":"share","formula":{"op":"percentOfSection","subtotal":"cats","scale":1,"args":[{"col":"cost"}]}}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`
	s, _, err := Load([]byte(fmt.Sprintf(spec, "byAnimal")))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, s), 400, "columns.3.formula.args.0", `percentOfSection reads column "cost", which is blank above group "byAnimal": the total of section "cats" is blank`)
	// A column that reads it is blank there too.
	const derived = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
		{"id":"share","formula":{"op":"percentOfSection","subtotal":"cats","scale":1,"args":[{"col":"twice"}]}}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`
	s, _, err = Load([]byte(derived))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, s), 400, "columns.4.formula.args.0", `percentOfSection reads column "cost", which is blank above group "byAnimal": the total of section "cats" is blank`)
	// Blank only above the first group, the sections keep the figure.
	s, _, err = Load([]byte(fmt.Sprintf(spec, "g")))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(app, s); err != nil {
		t.Fatalf("a column blank above the sections' own level: %v", err)
	}
}

// R12: an unknown group is the column's own 422 problem, coded unknown_group,
// and the whole spec answers 422 through Run as through Validate.
func TestBlankAboveOnAnUnknownGroupIsA422ThroughRun(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", fmt.Sprintf(blankAboveSpec, `,"blankAbove":"nope"`, ""), Options{}, nil)
	requireSpecError(t, err, 422, "columns.1.blankAbove", `no group "nope"`)
	var se *SpecError
	if !errors.As(err, &se) || se.Problems["columns.1.blankAbove"].Code() != "unknown_group" {
		t.Fatalf("problem = %v", err)
	}
}

// A column with both a rollup and an unknown group answers the unknown group
// (422, unknown_group): that case is checked first.
func TestBlankAboveUnknownGroupWinsOverARollup(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(fmt.Sprintf(blankAboveSpec, `,"blankAbove":"nope","rollup":{"value":0}`, "")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = Validate(app, spec)
	requireSpecError(t, err, 422, "columns.1.blankAbove", `no group "nope"`)
	var se *SpecError
	if !errors.As(err, &se) || len(se.Problems) != 1 || se.Problems["columns.1.blankAbove"].Code() != "unknown_group" {
		t.Fatalf("problems = %v", err)
	}
}
