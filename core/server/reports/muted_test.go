package reports

import (
	"encoding/json"
	"strings"
	"testing"
)

// notActive holds for an animal group whose record is not active (Mo).
const notActive = `{"op":"not","args":[{"record":"g","path":[{"field":"active"}]}]}`

// mutedByAnimal groups the reader's feedings by animal (open, with
// subtotals; %s is the rest of that group's JSON), then by note (%s too).
const mutedByAnimal = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal","open":true,"subtotals":true%s},{"id":"n","col":"note"%s}],
	"sort":[{"col":"note"}]}`

// mutedJSON is the row as the run endpoint writes it, read back as a map.
func mutedJSON(t *testing.T, r Row) map[string]any {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMutedMarksGroupRows(t *testing.T) {
	z := inlineZoo(t)
	isF1 := `{"op":"eq","args":[{"col":"note"},{"value":"f1"}]}`
	res := z.run(t, "ann", replaceTwo(mutedByAnimal, `,"muted":`+notActive, `,"muted":`+isF1), Options{}, nil)
	marked := map[string]bool{}
	for _, r := range res.Rows {
		m := mutedJSON(t, r)
		v, has := m["muted"]
		switch {
		case r.Muted && (!has || v != true):
			t.Fatalf("%s %s: Muted but JSON %v", r.Kind, r.Label, m)
		case !r.Muted && has:
			t.Fatalf("%s %s: not muted but JSON holds muted: %v", r.Kind, r.Label, m)
		case r.Muted && r.Kind != KindGroup:
			t.Fatalf("a %s row is muted: %+v", r.Kind, r)
		}
		if r.Muted {
			marked[r.Label] = true
		}
	}
	// Mo (not active) on the animal level; f1 on the note level, by each
	// level's own condition. Total Mo and the Total are never muted.
	if len(marked) != 2 || !marked["Mo"] || !marked["f1"] {
		t.Fatalf("muted rows %v, want Mo and f1", marked)
	}
	if mo := oneRow(t, res, KindSubtotal, "Total Mo"); mo.Muted {
		t.Fatalf("Total Mo is muted")
	}
	if lastTotal(t, res).Muted {
		t.Fatal("the Total is muted")
	}

	// A muted row of an inlined group's level keeps the mark.
	bySpecies := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","open":true,"subtotals":true,"inline":{"op":"eq","args":[{"col":"species"},{"value":"monkey"}]}},
			{"id":"g","col":"animal","muted":` + notActive + `}],
		"sort":[{"col":"species"}]}`
	res = z.run(t, "ann", bySpecies, Options{}, nil)
	mo := oneRow(t, res, KindGroup, "Mo")
	if mo.Depth != 0 || !mo.Muted || hasLabel(res, "monkey") {
		t.Fatalf("Mo = %+v (rows %v), want a muted row in the inlined monkey group's place", mo, rowsOf(res, "label"))
	}
	if leo := oneRow(t, res, KindGroup, "Leo"); leo.Muted {
		t.Fatalf("Leo (active) is muted")
	}
}

// replaceTwo fills the two %s of mutedByAnimal: the animal group's rest, then
// the note group's.
func replaceTwo(spec, animal, note string) string {
	spec = strings.Replace(spec, "%s", animal, 1)
	return strings.Replace(spec, "%s", note, 1)
}

func TestMutedRefusals(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ muted, want string }{
		"not a condition": {`{"value":3}`, "muted is a condition"},
		"one row's value": {`{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}`,
			"muted reads a group's totals, keys and records, not a value of one row"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := replaceTwo(mutedByAnimal, `,"muted":`+c.muted, "")
			spec, _, err := Load([]byte(raw))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, "groups.0.muted", c.want)
			_, err = z.try(t, "ann", raw, Options{}, nil)
			requireSpecError(t, err, 400, "groups.0.muted", c.want)
		})
	}
}

func TestMutedExportIgnoresIt(t *testing.T) {
	s := newSavedZoo(t)
	s.rec(t, "k6", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": s.id("mo")})
	plain := s.saved(t, "plain", "ann", "Feedings", replaceTwo(mutedByAnimal, "", ""), false)
	muted := s.saved(t, "muted", "ann", "Feedings", replaceTwo(mutedByAnimal, `,"muted":`+notActive, ""), false)
	// The muted report marks Mo, so the file below is of a report with a
	// muted row.
	run := s.run(t, "ann", replaceTwo(mutedByAnimal, `,"muted":`+notActive, ""), Options{}, nil)
	if !oneRow(t, run, KindGroup, "Mo").Muted {
		t.Fatal("Mo is not muted in the run")
	}
	want := s.buildCSV(t, "ann", plain.Id, `{"today":"2026-11-16"}`).Bytes
	got := s.buildCSV(t, "ann", muted.Id, `{"today":"2026-11-16"}`).Bytes
	if string(got) != string(want) {
		t.Fatalf("CSV with muted\n%q\nwithout\n%q", got, want)
	}
	if !strings.Contains(string(got), "Mo,") {
		t.Fatalf("the CSV has no Mo row: %q", got)
	}
}
