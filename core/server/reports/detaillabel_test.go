package reports

import (
	"strings"
	"testing"
)

func TestADetailLabelIsAFormula(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"sort":[{"col":"note"}]`
	plain := z.run(t, "ann", spec+`}`, Options{}, nil)
	requireOrderedRows(t, spec+`}`, plain, "label", "f1", "f2", "f3", "f4", "Total")
	if plain.Rows[0].LabelColumn != "note" {
		t.Fatalf("labelColumn = %q", plain.Rows[0].LabelColumn)
	}
	withLabel := spec + `,"detail":{"label":{"op":"concat","args":[{"path":[{"field":"fed"}]},{"value":" · "},{"path":[{"field":"note"}]}]}}}`
	labelled := z.run(t, "ann", withLabel, Options{}, nil)
	requireOrderedRows(t, withLabel, labelled, "label", "2026-01-05 · f1", "2026-02-10 · f2", "2026-01-20 · f3", "2026-03-03 · f4", "Total")
	if labelled.Rows[0].LabelColumn != "" {
		t.Fatalf("a label that is no cell names none: %q", labelled.Rows[0].LabelColumn)
	}
}

// A label that is exactly a column repeats that column's cell, so the row
// names it; a formula over the column repeats no cell.
func TestADetailLabelThatIsAColumnNamesIt(t *testing.T) {
	z := newZoo(t)
	spec := withDetailLabel(`{"col":"animal"}`)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Leo", "Leo", "Zed", "Zed", "Total")
	if res.Rows[0].LabelColumn != "animal" {
		t.Fatalf("labelColumn = %q, want animal", res.Rows[0].LabelColumn)
	}
	spec = withDetailLabel(`{"op":"concat","args":[{"col":"note"},{"value":"!"}]}`)
	wrapped := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, wrapped, "label", "f1!", "f2!", "f3!", "f4!", "Total")
	if wrapped.Rows[0].LabelColumn != "" {
		t.Fatalf("a formula over a column names none: %q", wrapped.Rows[0].LabelColumn)
	}
}

func TestADetailNeedsItsLabel(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"detail":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "detail", "detail has a label")
}

// detailLabelsOf lists the detail rows' typed labels: type, value and currency.
func detailLabelsOf(res *Result) [][3]any {
	var out [][3]any
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			out = append(out, [3]any{r.LabelType, r.LabelValue, r.LabelCurrency})
		}
	}
	return out
}

func requireLabels(t *testing.T, res *Result, want ...[3]any) {
	t.Helper()
	got := detailLabelsOf(res)
	if len(got) != len(want) {
		t.Fatalf("labels = %v\nwant     %v", got, want)
	}
	for i := range want {
		// A cents value reads back as int64 or float64; compare its text.
		if got[i][0] != want[i][0] || toText(got[i][1]) != toText(want[i][1]) || got[i][2] != want[i][2] {
			t.Fatalf("labels = %v\nwant     %v", got, want)
		}
	}
}

func toText(v any) string {
	if v == nil {
		return "<nil>"
	}
	return labelText(v)
}

const feedingsByNote = `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},
	{"id":"animal","path":[{"link":"animal"}]}],"sort":[{"col":"note"}],"detail":{"label":LABEL}}`

func withDetailLabel(label string) string { return strings.Replace(feedingsByNote, "LABEL", label, 1) }

// An amount label is typed, in its currency, though no column reads the
// amount or its currency (F5: the label's path adds its currency key).
func TestADetailLabelThatIsAnAmountIsTyped(t *testing.T) {
	z := newZoo(t)
	spec := withDetailLabel(`{"path":[{"field":"cost"}]}`)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "1000", "2500", "700", "300", "Total")
	requireLabels(t, res,
		[3]any{Money, 1000, "USD"}, [3]any{Money, 2500, "USD"}, [3]any{Money, 700, "USD"}, [3]any{Money, 300, "USD"})
	if res.Rows[0].LabelColumn != "" {
		t.Fatalf("labelColumn = %q", res.Rows[0].LabelColumn)
	}
}

func TestADetailLabelThatIsADateOrAYesNoIsTyped(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", withDetailLabel(`{"path":[{"link":"animal"},{"field":"born"}]}`), Options{}, nil)
	requireLabels(t, res,
		[3]any{Date, "2020-05-01", ""}, [3]any{Date, "2020-05-01", ""}, [3]any{Date, "2019-02-10", ""}, [3]any{Date, "2019-02-10", ""})
	yesSpec := withDetailLabel(`{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":800}]}`)
	yes := z.run(t, "ann", yesSpec, Options{}, nil)
	requireOrderedRows(t, yesSpec, yes, "label", "Yes", "Yes", "No", "No", "Total")
	requireLabels(t, yes, [3]any{Bool, true, ""}, [3]any{Bool, true, ""}, [3]any{Bool, false, ""}, [3]any{Bool, false, ""})
}

// A label that points at a record shows the record's label, never its id.
func TestADetailLabelThatReadsALinkShowsItsRecord(t *testing.T) {
	z := newZoo(t)
	spec := withDetailLabel(`{"path":[{"link":"animal"}]}`)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Leo", "Leo", "Zed", "Zed", "Total")
	// Through a column, by that column's label field.
	species := strings.Replace(withDetailLabel(`{"col":"animal"}`), `{"link":"animal"}]`, `{"link":"animal"}],"labelField":"species"`, 1)
	requireOrderedRows(t, species, z.run(t, "ann", species, Options{}, nil), "label", "lion", "lion", "zebra", "zebra", "Total")
}

// A looked-up amount carries its own row's currency into the label (Leo's fee
// is in EUR, Zed's in USD).
const animalFee = `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},"value":{"path":[{"field":"fee"}]}}`

func TestADetailLabelCarriesALookedUpCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", withDetailLabel(animalFee), Options{}, nil)
	requireLabels(t, res,
		[3]any{Money, 700, "EUR"}, [3]any{Money, 700, "EUR"}, [3]any{Money, 300, "USD"}, [3]any{Money, 300, "USD"})
}

// A running sum of looked-up amounts adds each currency on its own: the label
// reads the currency inside a total, so it is a currency key (Task 5a).
func TestADetailLabelRunsAPerCurrencySum(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", withDetailLabel(`{"op":"runningSum","args":[`+animalFee+`]}`), Options{}, nil)
	requireLabels(t, res,
		[3]any{Money, 700, "EUR"}, [3]any{Money, 1400, "EUR"}, [3]any{Money, 300, "USD"}, [3]any{Money, 600, "USD"})
}

func TestValidateChecksTheDetailLabel(t *testing.T) {
	app := withZooRegistry(t)
	requireSpecError(t, Validate(app, loadSpec(t, withDetailLabel(`{"path":[{"field":"nope"}]}`))), 422, "detail.label.path.0.field", "nope")
	big := strings.TrimSuffix(chainSpec(8, doubling), "}") + `,"detail":{"label":{"op":"add","args":[{"col":"c8"},{"col":"c8"}]}}}`
	requireSpecError(t, Validate(app, loadSpec(t, big)), 400, "detail.label", "with the formula columns it reads")
	badLookup := `{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"fee"}],"x":1}}`
	requireSpecError(t, Validate(app, loadSpec(t, withDetailLabel(badLookup))), 400, "detail.label.value", "a lookup's value is a formula")
	if err := Validate(app, loadSpec(t, withDetailLabel(animalFee))); err != nil {
		t.Fatalf("a lookup label: %v", err)
	}
}

// Every detail row names the column its label repeats, a nested child row
// its own level's first column, so the client leaves out exactly that cell.
func TestNestedChildRowsNameTheirLabelColumn(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	var got []string
	for _, r := range res.Rows {
		got = append(got, r.LabelColumn)
	}
	want := []string{"site", "animal", "fed", "fed", "site", "animal", "fed", "fed", ""}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("labelColumns = %v\nwant           %v", got, want)
	}
}
