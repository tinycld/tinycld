package webforms

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/automation"
	"tinycld.org/core/forms"
	"tinycld.org/core/rlstest"
)

const webFormHostJSON = `{"hosts":[{"pkgSlug":"core","key":"core:web_form","label":"Web form",
"collection":"form_submissions","formColumn":"form","fieldsColumn":"values",
"columns":["id","form","publication","values","client_id","ip_hash","user_agent","created"],
"builtin":[],"kinds":["text","textarea","number","date","toggle","select","checklist","file","computed","table"],
"files":{"collection":"form_submission_files","recordColumn":"submission","fieldColumn":"field","sizeColumn":"size","uploaderColumn":"uploaded_by"}}]}`

type fixture struct {
	app   *tests.TestApp
	form  *core.Record
	pub   *core.Record
	token string
}

// newFixture builds an assembled app with the forms hooks for the web form
// host, the webforms routes, and one enabled publication of a form whose
// definition is def.
func newFixture(t *testing.T, def string) *fixture {
	t.Helper()
	return buildFixture(t, def, true)
}

// newFixtureWithoutHost is newFixture on an app where no package bound the
// web form host, so its hooks (validation, files) are absent.
func newFixtureWithoutHost(t *testing.T, def string) *fixture {
	t.Helper()
	return buildFixture(t, def, false)
}

func buildFixture(t *testing.T, def string, bindHost bool) *fixture {
	t.Helper()
	app := rlstest.NewAssembledApp(t)
	if bindHost {
		defs, err := forms.ParseHosts([]byte(webFormHostJSON))
		if err != nil {
			t.Fatal(err)
		}
		forms.BindHooks(app, defs)
	}
	// The action catalog the safety rule reads: core's send-email and a
	// fictional package's send action, both with a recipient param.
	automation.NewEngine(app, &automation.Defs{Packages: []automation.PackageDefs{
		{Slug: "core", Actions: []automation.ActionDef{{ID: "send-email", Kind: "native", Params: []automation.ParamDef{
			{Key: "to", Type: "text", Recipient: true}, {Key: "subject", Type: "text"},
		}}}},
		{Slug: "gizmos", Actions: []automation.ActionDef{{ID: "dispatch", Kind: "native", Params: []automation.ParamDef{
			{Key: "recipient", Type: "text", Recipient: true}, {Key: "note", Type: "text"},
		}}}},
	}})
	Register(app)

	formsCol, _ := app.FindCollectionByNameOrId("forms")
	form := core.NewRecord(formsCol)
	form.Set("host", WebFormHostKey)
	form.Set("name", "Contact")
	form.Set("slug", "contact")
	form.Set("definition", []byte(def))
	form.Set("is_default", true)
	form.Set("order", 1)
	if err := app.Save(form); err != nil {
		t.Fatal(err)
	}
	pubs, _ := app.FindCollectionByNameOrId("form_publications")
	pub := core.NewRecord(pubs)
	pub.Set("form", form.Id)
	pub.Set("enabled", true)
	if err := app.Save(pub); err != nil {
		t.Fatal(err)
	}
	return &fixture{app: app, form: form, pub: pub, token: pub.GetString("token")}
}

const contactDef = `{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[
{"key":"name","page":"main","label":"Name","kind":"text","required":true},
{"key":"cv","page":"main","label":"CV","kind":"file","accept":["application/pdf"],"maxSizeMb":1},
{"key":"docs","page":"main","label":"Docs","kind":"file","multiple":true}]}`

// storeForTest submits through the public endpoint and returns the stored row.
func storeForTest(t *testing.T, f *fixture, values map[string]any, uploads ...upload) *core.Record {
	t.Helper()
	raw, _ := json.Marshal(values)
	id := "store-" + strconv.Itoa(int(time.Now().UnixNano()%1e9))
	fields := map[string]string{"values": string(raw), "client_id": id, "stamp": oldStamp(t, f), "_hp": ""}
	submit(t, f, "store", fields, 200, []string{`"ok":true`}, uploads...)
	rec, err := f.app.FindFirstRecordByData("form_submissions", "client_id", id)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func utf8ValidString(s string) bool { return utf8.ValidString(s) }

// secondPublication publishes a second form in the fixture's app, with its
// own retention, and returns a fixture that submits to it.
func secondPublication(t *testing.T, f *fixture, retentionDays int) *fixture {
	t.Helper()
	formsCol, _ := f.app.FindCollectionByNameOrId("forms")
	form := core.NewRecord(formsCol)
	form.Set("host", WebFormHostKey)
	form.Set("name", "Second")
	form.Set("slug", "second")
	form.Set("definition", f.form.Get("definition"))
	form.Set("order", 2)
	if err := f.app.Save(form); err != nil {
		t.Fatal(err)
	}
	pubs, _ := f.app.FindCollectionByNameOrId("form_publications")
	pub := core.NewRecord(pubs)
	pub.Set("form", form.Id)
	pub.Set("enabled", true)
	pub.Set("retention_days", retentionDays)
	if err := f.app.Save(pub); err != nil {
		t.Fatal(err)
	}
	return &fixture{app: f.app, form: form, pub: pub, token: pub.GetString("token")}
}
