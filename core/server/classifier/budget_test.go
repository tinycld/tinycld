package classifier

import (
	"errors"
	"testing"
	"time"

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
