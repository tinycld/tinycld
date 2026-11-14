package classifier

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/classifier/classifiertest"
	"tinycld.org/core/syscfg"
)

func newApp(t *testing.T, settings map[string]string) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	classifiertest.CreateCollections(t, app)
	syscfg.SetResolver(app, func(k string) string { return settings[k] })
	return app
}

func TestLoadConfig_Defaults(t *testing.T) {
	app := newApp(t, map[string]string{KeyAPIKey: "k"})
	cfg := LoadConfig(app)
	if cfg.Budget != 5 || cfg.FullThreshold != 0.7 {
		t.Fatalf("%+v", cfg)
	}
	if !Available(app) {
		t.Fatal("should be available with key and no status")
	}
}

func TestLoadConfig_RejectsNaNBudget(t *testing.T) {
	for _, v := range []string{"NaN", "Inf", "-Inf"} {
		app := newApp(t, map[string]string{KeyAPIKey: "k", KeyBudget: v})
		if got := LoadConfig(app).Budget; got != 5 {
			t.Fatalf("%s -> %v", v, got)
		}
	}
}

func TestLoadConfig_AutoActionThreshold(t *testing.T) {
	cases := map[string]float64{
		"": 0.9, "0.4": 0.9, "0.5": 0.5, "0.75": 0.75, "1": 1, "1.1": 0.9,
		"abc": 0.9, "NaN": 0.9, "Inf": 0.9, "-Inf": 0.9,
	}
	for in, want := range cases {
		app := newApp(t, map[string]string{KeyAPIKey: "k", KeyAutoActionThreshold: in})
		if got := LoadConfig(app).AutoActionThreshold; got != want {
			t.Errorf("threshold %q = %v, want %v", in, got, want)
		}
	}
}

// A taxonomy row's label relation is required and non-cascade, so PocketBase
// must refuse to delete a label a taxonomy row still points at.
func TestTaxonomyLabel_CannotBeDeletedWhileReferenced(t *testing.T) {
	app := newApp(t, nil)
	labels, err := app.FindCollectionByNameOrId("labels")
	if err != nil {
		t.Fatal(err)
	}
	label := core.NewRecord(labels)
	label.Set("name", "Junk")
	if err := app.Save(label); err != nil {
		t.Fatal(err)
	}
	tax, err := app.FindCollectionByNameOrId("classifier_labels")
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(tax)
	row.Set("app", "mail")
	row.Set("name", "Junk")
	row.Set("label", label.Id)
	row.Set("assign_when", "junk")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(label); err == nil {
		t.Fatal("deleting a referenced label succeeded")
	}
	if _, err := app.FindRecordById("classifier_labels", row.Id); err != nil {
		t.Fatalf("taxonomy row gone: %v", err)
	}
}

func TestBudget_FailsClosedOnLookupError(t *testing.T) {
	app := newApp(t, map[string]string{KeyAPIKey: "k"})
	if _, err := app.DB().NewQuery("DROP TABLE classifier_usage").Execute(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := checkBudget(app, LoadConfig(app), 0.1, now); err == nil || errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want lookup error, got %v", err)
	}
	if err := addUsage(app, 0.1, now); err == nil {
		t.Fatal("addUsage should fail")
	}
}

func TestAvailable_FalseWithoutKeyOrWithBadStatus(t *testing.T) {
	if Available(newApp(t, map[string]string{})) {
		t.Fatal("no key")
	}
	if Available(newApp(t, map[string]string{KeyAPIKey: "k", KeyStatus: "no_credits"})) {
		t.Fatal("bad status")
	}
}

func TestBudget(t *testing.T) {
	app := newApp(t, map[string]string{KeyAPIKey: "k", KeyBudget: "0.001"})
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	cfg := LoadConfig(app)
	if err := checkBudget(app, cfg, 0.0005, now); err != nil {
		t.Fatal(err)
	}
	if err := addUsage(app, 0.0008, now); err != nil {
		t.Fatal(err)
	}
	if err := addUsage(app, 0.0001, now); err != nil {
		t.Fatal(err)
	}
	cost, calls := MonthSpend(app, now)
	if calls != 2 || cost < 0.00089 || cost > 0.00091 {
		t.Fatalf("spend %v %d", cost, calls)
	}
	if err := checkBudget(app, cfg, 0.0005, now); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want budget exceeded, got %v", err)
	}
	if cost, _ := MonthSpend(app, now.AddDate(0, 1, 0)); cost != 0 {
		t.Fatal("new month should start at zero")
	}
}
