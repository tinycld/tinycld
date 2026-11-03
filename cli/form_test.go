package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func webForm(id, name, slug string) map[string]any {
	return map[string]any{"id": id, "host": "core:web_form", "name": name, "slug": slug, "order": 1,
		"definition": json.RawMessage(`{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[{"key":"name","kind":"text","page":"main","label":"Name"}]}`)}
}

func TestFormResolveByIdSlugAndName(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	d := f.deps(t)
	for _, arg := range []string{"f1", "jobs", "Jobs"} {
		out, _, err := runCLI(t, d, "form", "export", arg)
		if err != nil || !strings.Contains(out, `"version": 1`) {
			t.Fatalf("export %s: out %q err %v", arg, out, err)
		}
	}
}

func TestFormResolveRefusesAnAmbiguousName(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Leads", "leads"), map[string]any{"id": "f2", "host": "gizmos:widget", "name": "Leads", "slug": "leads"})
	_, _, err := runCLI(t, f.deps(t), "form", "export", "Leads")
	if err == nil || !strings.Contains(err.Error(), "2 forms match") || !strings.Contains(err.Error(), "f1") {
		t.Fatalf("got %v", err)
	}
}

func TestFormCreateSendsTheFileAndHost(t *testing.T) {
	f := newFakePB(t)
	d := f.deps(t)
	path := writeTemp(t, "def.json", string(exampleFile("definition.json")))
	if _, _, err := runCLI(t, d, "form", "create", "Job Applications", "--from", path); err != nil {
		t.Fatal(err)
	}
	req := f.last("POST", "/api/collections/forms/records")
	if req.Body["host"] != "core:web_form" || req.Body["slug"] != "job-applications" || req.Body["name"] != "Job Applications" {
		t.Fatalf("body = %v", req.Body)
	}
	if _, ok := req.Body["definition"].(map[string]any); !ok {
		t.Fatalf("definition is sent as JSON, got %T", req.Body["definition"])
	}
}

func TestFormCreatePrintsEveryServerProblem(t *testing.T) {
	f := newFakePB(t)
	f.failNext("POST", "forms", 400, `{"status":400,"message":"Failed to create record.","data":{
		"definition.fields.0.key":{"code":"validation_invalid_definition","message":"\"Email\" is not a valid field key."},
		"definition.fields.1.kind":{"code":"validation_invalid_definition","message":"\"phone\" is not a field kind."}}}`)
	path := writeTemp(t, "def.json", `{"version":1}`)
	_, _, err := runCLI(t, f.deps(t), "form", "create", "X", "--from", path)
	if err == nil || !strings.Contains(err.Error(), "definition.fields.0.key") || !strings.Contains(err.Error(), "definition.fields.1.kind") || !strings.Contains(err.Error(), "See: tinycld form create --help") {
		t.Fatalf("got %v", err)
	}
}

func TestFormCreateRefusesBadJSONBeforeAnyRequest(t *testing.T) {
	f := newFakePB(t)
	path := writeTemp(t, "def.json", "{\n  \"version\": 1,\n}")
	_, _, err := runCLI(t, f.deps(t), "form", "create", "X", "--from", path)
	if err == nil || !strings.Contains(err.Error(), "def.json:3:") {
		t.Fatalf("got %v", err)
	}
	if len(f.requests) != 0 {
		t.Fatalf("no request is sent: %+v", f.requests)
	}
}

func TestFormRemoveNeedsYesWithoutATerminal(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	d := f.deps(t)
	if _, _, err := runCLI(t, d, "form", "remove", "jobs"); err == nil {
		t.Fatal("refused without --yes")
	}
	if f.find("forms", "f1") == nil {
		t.Fatal("nothing is deleted")
	}
	if _, _, err := runCLI(t, d, "form", "remove", "jobs", "--yes"); err != nil {
		t.Fatal(err)
	}
	if f.find("forms", "f1") != nil {
		t.Fatal("deleted with --yes")
	}
}

func TestFormHostsTableAndJSON(t *testing.T) {
	f := newFakePB(t)
	f.hosts = `{"hosts":[{"key":"core:web_form","label":"Web form","kinds":["text","file"],"builtins":[]}]}`
	d := f.deps(t)
	out, _, err := runCLI(t, d, "form", "hosts")
	if err != nil || !strings.Contains(out, "core:web_form") || !strings.Contains(out, "text, file") {
		t.Fatalf("table: %q %v", out, err)
	}
	out, _, err = runCLI(t, d, "form", "hosts", "--json")
	if err != nil || !strings.Contains(out, `"kinds"`) {
		t.Fatalf("json: %q %v", out, err)
	}
}

func TestFormFieldsShowsPlaceholders(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	out, _, err := runCLI(t, f.deps(t), "form", "fields", "jobs")
	if err != nil || !strings.Contains(out, "{{fields.name}}") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestFormHelpExplainsTheDefinition(t *testing.T) {
	f := newFakePB(t)
	out, _, err := runCLI(t, f.deps(t), "form", "create", "--help")
	if err != nil || !strings.Contains(out, `"pages"`) || !strings.Contains(out, "tinycld form hosts") {
		t.Fatalf("%q %v", out, err)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFormViewJSONHasThePublicURL(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.put("form_publications", map[string]any{"id": "p1", "form": "f1", "token": "tok", "enabled": true})
	out, _, err := runCLI(t, f.deps(t), "form", "view", "jobs", "--json")
	if err != nil || !strings.Contains(out, `"url": "http`) || !strings.Contains(out, "/p/forms/tok") {
		t.Fatalf("%q %v", out, err)
	}
}

// A 404 from the hosts route means the server predates it.
func TestFormHostsOnAnOlderServer(t *testing.T) {
	f := newFakePB(t)
	_, _, err := runCLI(t, f.deps(t), "form", "hosts")
	if err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("got %v", err)
	}
}
