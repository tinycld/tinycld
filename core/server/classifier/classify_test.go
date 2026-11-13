package classifier_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/classifier"
	"tinycld.org/core/classifier/classifiertest"
	"tinycld.org/core/classifier/fakeprovider"
	"tinycld.org/core/syscfg"
)

var tax = []classifier.TaxonomyLabel{
	{ID: "c1", Name: "action-required", AssignWhen: "Needs a reply"},
	{ID: "c2", Name: "junk", AssignWhen: "Spam", AutoAction: true},
}

var probs = map[string]float64{"L0": 0.2, "L1": 0.8}

func classifyApp(t *testing.T, fake *fakeprovider.Fake, extra map[string]string) core.App {
	t.Helper()
	s := map[string]string{classifier.KeyAPIKey: fakeprovider.AcceptedKey}
	for k, v := range extra {
		s[k] = v
	}
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	classifiertest.CreateCollections(t, app)
	syscfg.SetResolver(app, func(k string) string { return s[k] })
	col := core.NewBaseCollection("system_settings")
	col.Fields.Add(&core.TextField{Name: "key"}, &core.TextField{Name: "value"}, &core.BoolField{Name: "is_secret"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	classifier.SetProvider(app, fake)
	return app
}

func scripted(reply func(n int32) (int, string)) *fakeprovider.Fake {
	f := fakeprovider.New()
	f.Reply = func(n int32, _ []byte) (int, string) { return reply(n) }
	return f
}

func TestClassify_MetadataOnlyWhenConfident(t *testing.T) {
	f := scripted(func(int32) (int, string) { return 200, fakeprovider.Answer("L1", 0.9, probs) })
	app := classifyApp(t, f, nil)
	fullCalled := false
	r, err := classifier.Classify(context.Background(), app, classifier.Request{
		Taxonomy: tax, Evidence: map[string]any{"subject": "x"},
		FullEvidence: func(context.Context) (map[string]any, error) { fullCalled = true; return nil, nil },
	})
	if err != nil || r.Stage != classifier.StageMetadata || fullCalled || f.Calls.Load() != 1 {
		t.Fatalf("r=%+v err=%v full=%v calls=%d", r, err, fullCalled, f.Calls.Load())
	}
}

func TestClassify_StageTwoOnLowConfidenceOrNeedsFull(t *testing.T) {
	f := scripted(func(n int32) (int, string) {
		if n == 1 {
			return 200, fakeprovider.Answer("L1", 0.5, probs)
		}
		return 200, fakeprovider.Answer("L1", 0.95, probs)
	})
	app := classifyApp(t, f, nil)
	r, err := classifier.Classify(context.Background(), app, classifier.Request{
		Taxonomy: tax, Evidence: map[string]any{"subject": "x"},
		FullEvidence: func(context.Context) (map[string]any, error) { return map[string]any{"body": "b"}, nil },
	})
	if err != nil || r.Stage != classifier.StageFull || r.Confidence != 0.95 || r.CostUSD < 0.000019 {
		t.Fatalf("r=%+v err=%v", r, err)
	}

	g := scripted(func(int32) (int, string) { return 200, fakeprovider.Answer("L1", 0.8, probs) })
	app2 := classifyApp(t, g, nil)
	r, _ = classifier.Classify(context.Background(), app2, classifier.Request{
		Taxonomy: tax, Evidence: map[string]any{"subject": "x"},
		FullEvidence: func(context.Context) (map[string]any, error) { return map[string]any{"body": "b"}, nil },
		NeedsFull:    func(r classifier.Result) bool { return r.Label.AutoAction && r.Confidence < 0.9 },
	})
	if r.Stage != classifier.StageFull || g.Calls.Load() != 2 {
		t.Fatalf("NeedsFull ignored: %+v calls=%d", r, g.Calls.Load())
	}
}

func TestClassify_RetriesTransientOnce(t *testing.T) {
	f := scripted(func(n int32) (int, string) {
		if n == 1 {
			return 503, `{}`
		}
		return 200, fakeprovider.Answer("L0", 0.9, probs)
	})
	app := classifyApp(t, f, nil)
	if _, err := classifier.Classify(context.Background(), app, classifier.Request{Taxonomy: tax, Evidence: map[string]any{"subject": "x"}}); err != nil {
		t.Fatal(err)
	}
	if f.Calls.Load() != 2 {
		t.Fatalf("calls = %d", f.Calls.Load())
	}
}

type failingProvider struct{}

func (failingProvider) Decide(context.Context, string, []byte) (classifier.Response, error) {
	return classifier.Response{}, errors.New("dial tcp: connection refused")
}

func TestClassify_TransportErrorRetriedThenFails(t *testing.T) {
	app := classifyApp(t, fakeprovider.New(), nil)
	classifier.SetProvider(app, failingProvider{})
	_, err := classifier.Classify(context.Background(), app, classifier.Request{Taxonomy: tax, Evidence: map[string]any{"subject": "x"}})
	if classifier.ErrorCode(err) != "transport_error" {
		t.Fatalf("err = %v", err)
	}
}

func TestClassify_AuthFailurePausesFeature(t *testing.T) {
	app := classifyApp(t, fakeprovider.New(), map[string]string{classifier.KeyAPIKey: "wrong"})
	_, err := classifier.Classify(context.Background(), app, classifier.Request{Taxonomy: tax, Evidence: map[string]any{"subject": "[jev:junk]"}})
	var fl *classifier.Failure
	if !errors.As(err, &fl) || fl.Status != "auth_failed" {
		t.Fatalf("err = %v", err)
	}
	rec, ferr := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": classifier.KeyStatus})
	if ferr != nil || rec.GetString("value") != "auth_failed" {
		t.Fatalf("status not written: %v", ferr)
	}
}

func TestClassify_BudgetExceededMakesNoCall(t *testing.T) {
	f := fakeprovider.New()
	app := classifyApp(t, f, map[string]string{classifier.KeyBudget: "0"})
	_, err := classifier.Classify(context.Background(), app, classifier.Request{Taxonomy: tax, Evidence: map[string]any{"subject": "[jev:junk]"}})
	if !errors.Is(err, classifier.ErrBudgetExceeded) || f.Calls.Load() != 0 || classifier.ErrorCode(err) != "budget_exceeded" {
		t.Fatalf("err=%v calls=%d", err, f.Calls.Load())
	}
}

func TestSaveRow_Upserts(t *testing.T) {
	app := classifyApp(t, fakeprovider.New(), nil)
	row := classifier.Row{App: "posts", Collection: "posts", RecordID: "r1", User: "u1", LabelName: "a", Confidence: 0.5, Stage: classifier.StageMetadata}
	if err := classifier.SaveRow(app, row); err != nil {
		t.Fatal(err)
	}
	row.LabelName = "b"
	if err := classifier.SaveRow(app, row); err != nil {
		t.Fatal(err)
	}
	recs, _ := app.FindRecordsByFilter("classifications", "record_id = 'r1'", "", 0, 0)
	if len(recs) != 1 || recs[0].GetString("label_name") != "b" {
		t.Fatalf("rows = %d", len(recs))
	}
}

func superuserGuard(re *core.RequestEvent) error {
	if re.HasSuperuserAuth() {
		return re.Next()
	}
	return re.ForbiddenError("Admin access required", nil)
}

func verifyScenario(t *testing.T, name, key, want string, wantStatus string) {
	t.Helper()
	app := classifyApp(t, fakeprovider.New(), map[string]string{classifier.KeyAPIKey: key})
	settings, err := app.FindCollectionByNameOrId("system_settings")
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(settings)
	row.Set("key", classifier.KeyStatus)
	row.Set("value", "auth_failed")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	admin := core.NewRecord(mustSuperusers(t, app))
	admin.SetEmail("admin@test.local")
	admin.SetPassword("password123")
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	token, err := admin.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	testApp := app.(*tests.TestApp)
	classifier.Register(testApp, superuserGuard)
	sc := tests.ApiScenario{
		Name: name, Method: http.MethodPost, URL: "/api/classifier/verify",
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        200,
		ExpectedContent:       []string{want},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return testApp },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
	rec, ferr := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": classifier.KeyStatus})
	if ferr != nil || rec.GetString("value") != wantStatus {
		t.Fatalf("status = %q (%v), want %q", rec.GetString("value"), ferr, wantStatus)
	}
}

func mustSuperusers(t *testing.T, app core.App) *core.Collection {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	return col
}

func TestVerify_ClearsPauseWithGoodKey(t *testing.T) {
	verifyScenario(t, "good key", fakeprovider.AcceptedKey, `"ok":true`, "ok")
}

func TestVerify_ReportsBadKey(t *testing.T) {
	verifyScenario(t, "bad key", "wrong", `"error":"authentication_failed"`, "auth_failed")
}
