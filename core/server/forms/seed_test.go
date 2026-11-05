package forms

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func TestSeedDefaults_CreatesOneDefaultPerHostAndBackfills(t *testing.T) {
	e := newTestEnv(t)
	// Inserted before any form exists: the host hook then leaves form empty.
	widgets, _ := e.app.FindCollectionByNameOrId("gizmos_widgets")
	orphan := core.NewRecord(widgets)
	orphan.Set("name", "old")
	if err := e.app.SaveNoValidate(orphan); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	forms, err := e.app.FindRecordsByFilter("forms", "host = {:host}", "", 0, 0, dbx.Params{"host": "gizmos:widget"})
	if err != nil || len(forms) != 1 {
		t.Fatalf("forms = %d err = %v", len(forms), err)
	}
	f := forms[0]
	if !f.GetBool("is_default") || f.GetString("name") != "Widget" || f.GetString("slug") != "widget" {
		t.Fatalf("default form = %v", f.PublicExport())
	}
	def, _ := ParseDefinition([]byte(f.GetString("definition")))
	if len(def.Fields) != 6 || def.Fields[0].Key != "name" || def.Fields[0].Required != true {
		t.Fatalf("definition fields = %+v", def.Fields)
	}
	orphan, _ = e.app.FindRecordById("gizmos_widgets", orphan.Id)
	if orphan.GetString("form") != f.Id {
		t.Fatalf("backfill: form = %q", orphan.GetString("form"))
	}
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	forms, _ = e.app.FindRecordsByFilter("forms", "host = {:host}", "", 0, 0, dbx.Params{"host": "gizmos:widget"})
	if len(forms) != 1 {
		t.Fatal("seed must be idempotent")
	}
}

func TestSeedDefaults_SkipsAHostWhoseCollectionIsAbsent(t *testing.T) {
	e := newTestEnv(t)
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	absent, _ := e.app.FindRecordsByFilter("forms", "host = {:host}", "", 0, 0, dbx.Params{"host": "gizmos:other"})
	if len(absent) != 0 {
		t.Fatal("no form for a host whose collection is not installed")
	}
	present, _ := e.app.FindRecordsByFilter("forms", "host = {:host}", "", 0, 0, dbx.Params{"host": "gizmos:widget"})
	if len(present) != 1 {
		t.Fatalf("widget default forms = %d", len(present))
	}
}

// The client parses this same file with definitionSchema
// (core/lib/forms/__tests__/seeded-definition.test.ts), so a Go change that
// writes a shape the client rejects fails one of the two suites.
const seededFixturePath = "../../lib/forms/fixtures/seeded-definition.json"

func TestDefaultDefinition_MatchesTheSeededFixture(t *testing.T) {
	defs, err := LoadHosts(filepath.Join("testdata", "form_hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(defaultDefinition(defs.Host("gizmos:widget")))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(seededFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := json.Compact(&want, raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("seeded definition drifted from the fixture\n got: %s\nwant: %s", got, want.Bytes())
	}
}

// The client seeds the same host from the same files
// (core/lib/forms/__tests__/layout-definition.test.ts), so a host layout the
// two seeds read differently fails one of the two suites.
const (
	layoutHostsFixturePath      = "../../lib/forms/fixtures/layout-hosts.json"
	layoutDefinitionFixturePath = "../../lib/forms/fixtures/layout-definition.json"
)

func TestDefaultDefinition_MatchesTheLayoutFixture(t *testing.T) {
	rawHosts, err := os.ReadFile(layoutHostsFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := ParseHosts(rawHosts)
	if err != nil {
		t.Fatal(err)
	}
	host := defs.Host("gizmos:gadget")
	if host == nil {
		t.Fatal("the layout fixture has no gizmos:gadget host")
	}
	got, err := json.Marshal(defaultDefinition(host))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(layoutDefinitionFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := json.Compact(&want, raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("the seed of a host with a layout drifted from the fixture\n got: %s\nwant: %s", got, want.Bytes())
	}
}

func TestSeedDefaults_ReconcilesBuiltinsWithTheHost(t *testing.T) {
	e := newTestEnv(t)
	var older []Builtin
	for _, b := range e.host.Builtins {
		if b.Key != "owner" {
			older = append(older, b)
		}
	}
	older = append(older, Builtin{Key: "legacy", Label: "Legacy", Kind: "text", Storage: "column"})
	var form *core.Record
	e.withBuiltins(t, older, func() {
		def := strings.Replace(widgetDefault, `{"key":"owner","kind":"builtin","page":"main","label":"Owner","width":"full","required":false,"hidden":false},`,
			`{"key":"legacy","kind":"builtin","page":"main","label":"Legacy","width":"half","required":false,"hidden":false},`, 1)
		def = strings.Replace(def, `"rules":[]`, `"rules":[`+
			`{"key":"reads_legacy","match":"all","when":[{"field":"legacy","op":"is_empty"}],"actions":[{"action":"hide","field":"due"}]},`+
			`{"key":"targets_legacy","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"hide","field":"legacy"},{"action":"hide","field":"notes"}]}]`, 1)
		def = strings.Replace(def, `"defaults":{}`, `"defaults":{"legacy":"x"}`, 1)
		def = strings.Replace(def, `"submit":{"required":[]}`, `"submit":{"required":["legacy","due"]}`, 1)
		form = e.newForm(t, "widget", def, true)
	})
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	form, _ = e.app.FindRecordById("forms", form.Id)
	def, err := ParseDefinition([]byte(form.GetString("definition")))
	if err != nil {
		t.Fatal(err)
	}
	if def.Field("legacy") != nil {
		t.Fatal("a built-in the host no longer declares must go")
	}
	owner := def.Field("owner")
	if owner == nil || owner.Kind != "builtin" || owner.Page != "main" || owner.Label != "Owner" {
		t.Fatalf("a built-in the host now declares must be added, got %+v", owner)
	}
	if len(def.Rules) != 1 || def.Rules[0].Key != "targets_legacy" || len(def.Rules[0].Actions) != 1 || def.Rules[0].Actions[0].Field != "notes" {
		t.Fatalf("rules = %+v", def.Rules)
	}
	if _, ok := def.Defaults["legacy"]; ok {
		t.Fatal("the default of a removed built-in must go")
	}
	if !reflect.DeepEqual(def.Submit.Required, []string{"due"}) {
		t.Fatalf("submit.required = %v", def.Submit.Required)
	}
	if !strings.Contains(form.GetString("definition"), `"width":"full"`) {
		t.Fatal("properties the server does not model must survive")
	}
	if errs := ValidateDefinition(def, e.host); errs != nil {
		t.Fatalf("a reconciled form must be valid: %v", errs)
	}
}

func TestSeedDefaults_ContinuesPastAFailingHost(t *testing.T) {
	e := newTestEnv(t)
	// Not registered with the hooks, so its default form is refused.
	broken := Host{Key: "gizmos:broken", Label: "Broken", Collection: "gizmos_widgets", FormColumn: "form", FieldsColumn: "fields",
		Builtins: []Builtin{{Key: "name", Label: "Name", Kind: "text", Storage: "column", Required: true}}}
	defs := &Defs{Hosts: []Host{broken, *e.host}}
	err := SeedDefaults(e.app, defs)
	if err == nil || !strings.Contains(err.Error(), "gizmos:broken") {
		t.Fatalf("the failing host's error must be returned, got %v", err)
	}
	widgetForms, _ := e.app.FindRecordsByFilter("forms", "host = {:host}", "", 0, 0, dbx.Params{"host": "gizmos:widget"})
	if len(widgetForms) != 1 {
		t.Fatalf("a later host must still be seeded, got %d forms", len(widgetForms))
	}
}

func TestSeedDefaults_PromotesANonArchivedForm(t *testing.T) {
	e := newTestEnv(t)
	a := e.newForm(t, "a", widgetDefault, false)
	a.Set("archived", true)
	if err := e.app.Save(a); err != nil {
		t.Fatal(err)
	}
	b := e.newForm(t, "b", widgetDefault, false)
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	b, _ = e.app.FindRecordById("forms", b.Id)
	if !b.GetBool("is_default") {
		t.Fatal("the non-archived form must be promoted")
	}
}

func TestDefaultDefinition_WritesALayoutDescriptionOnItsFieldOnly(t *testing.T) {
	rawHosts, err := os.ReadFile(layoutHostsFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := ParseHosts(rawHosts)
	if err != nil {
		t.Fatal(err)
	}
	def := defaultDefinition(defs.Host("gizmos:gadget"))
	for _, f := range def.Fields {
		want := ""
		if f.Key == "notes" {
			want = "Seen by the whole team."
		}
		if f.Description != want {
			t.Fatalf("field %s description = %q, want %q", f.Key, f.Description, want)
		}
	}
}

func TestSeedDefaults_AppendsABuiltinWithItsLayoutDescription(t *testing.T) {
	e := newTestEnv(t)
	var older []Builtin
	for _, b := range e.host.Builtins {
		if b.Key != "owner" && b.Key != "due" {
			older = append(older, b)
		}
	}
	var form *core.Record
	e.withBuiltins(t, older, func() {
		def := strings.Replace(widgetDefault, `{"key":"owner","kind":"builtin","page":"main","label":"Owner","width":"full","required":false,"hidden":false},`, "", 1)
		def = strings.Replace(def, `{"key":"due","kind":"builtin","page":"main","label":"Due","width":"full","required":false,"hidden":false},`, "", 1)
		form = e.newForm(t, "widget", def, true)
	})
	e.host.Layout = &HostLayout{
		Pages:  []Page{{Key: "main", Title: "Widget"}},
		Fields: map[string]LayoutField{"owner": {Description: "Who answers for it."}},
		Rules:  []Rule{},
	}
	if err := SeedDefaults(e.app, e.defs); err != nil {
		t.Fatal(err)
	}
	form, _ = e.app.FindRecordById("forms", form.Id)
	var def struct {
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(form.GetString("definition")), &def); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]map[string]any{}
	for _, f := range def.Fields {
		key, _ := f["key"].(string)
		byKey[key] = f
	}
	owner, due := byKey["owner"], byKey["due"]
	if owner == nil || due == nil {
		t.Fatalf("both built-ins must be appended, got fields %v", def.Fields)
	}
	if owner["description"] != "Who answers for it." {
		t.Fatalf("owner description = %v, want the layout's", owner["description"])
	}
	if _, has := due["description"]; has {
		t.Fatalf("a built-in with no layout description must have no description key, got %v", due)
	}
}
