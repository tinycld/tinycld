package webforms

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
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
	resetLimitersForTesting()
	app := rlstest.NewAssembledApp(t)
	defs, err := forms.ParseHosts([]byte(webFormHostJSON))
	if err != nil {
		t.Fatal(err)
	}
	forms.BindHooks(app, defs)
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
