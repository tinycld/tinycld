package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEmbedCodeMatchesTheAppFixture(t *testing.T) {
	raw, err := os.ReadFile("../core/lib/web-forms/fixtures/embed-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Origin, Token, Title, URL, Code string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := publicFormURL(c.Origin, c.Token); got != c.URL {
			t.Errorf("url = %q, want %q", got, c.URL)
		}
		if got := embedCode(c.Origin, c.Token, c.Title); got != c.Code {
			t.Errorf("code = %q, want %q", got, c.Code)
		}
	}
}

func TestPublishCreatesThenUpdates(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	d := f.deps(t)
	if _, _, err := runCLI(t, d, "form", "publish", "jobs", "--domain", "https://a.example,https://b.example", "--pow"); err != nil {
		t.Fatal(err)
	}
	create := f.last("POST", "/api/collections/form_publications/records")
	if create.Body["form"] != "f1" || create.Body["enabled"] != true || create.Body["require_pow"] != true ||
		create.Body["allowed_domains"] != "https://a.example https://b.example" {
		t.Fatalf("create body = %v", create.Body)
	}
	if _, _, err := runCLI(t, d, "form", "publish", "jobs", "--retention-days", "30"); err != nil {
		t.Fatal(err)
	}
	update := f.last("PATCH", "/api/collections/form_publications/records/")
	if update.Body["retention_days"] != float64(30) || update.Body["enabled"] != true {
		t.Fatalf("update body = %v", update.Body)
	}
	if _, ok := update.Body["require_pow"]; ok {
		t.Fatal("a flag left out keeps its stored value")
	}
}

func TestPublishRefusesAPackageHostBeforeAnyRequest(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", map[string]any{"id": "f2", "host": "gizmos:widget", "name": "Widget", "slug": "widget"})
	_, _, err := runCLI(t, f.deps(t), "form", "publish", "widget")
	if err == nil || !strings.Contains(err.Error(), "only web forms can be published") {
		t.Fatalf("got %v", err)
	}
	for _, r := range f.requests {
		if strings.Contains(r.Path, "form_publications") {
			t.Fatalf("no publication request: %+v", r)
		}
	}
}

func TestUnpublishKeepsTheToken(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.put("form_publications", map[string]any{"id": "p1", "form": "f1", "token": "keep", "enabled": true})
	if _, _, err := runCLI(t, f.deps(t), "form", "unpublish", "jobs"); err != nil {
		t.Fatal(err)
	}
	req := f.last("PATCH", "/api/collections/form_publications/records/p1")
	if req.Body["enabled"] != false {
		t.Fatalf("body = %v", req.Body)
	}
	if _, ok := req.Body["token"]; ok {
		t.Fatal("unpublish does not touch the token")
	}
}

func TestRegenerateLinkNeedsYesThenSendsAnEmptyToken(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.put("form_publications", map[string]any{"id": "p1", "form": "f1", "token": "old", "enabled": true})
	d := f.deps(t)
	if _, _, err := runCLI(t, d, "form", "regenerate-link", "jobs"); err == nil {
		t.Fatal("refused without --yes")
	}
	if f.find("form_publications", "p1")["token"] != "old" {
		t.Fatal("nothing changed")
	}
	out, _, err := runCLI(t, d, "form", "regenerate-link", "jobs", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if f.last("PATCH", "/api/collections/form_publications/records/p1").Body["token"] != "" {
		t.Fatal("an empty token asks the server for a new one")
	}
	if !strings.Contains(out, "/p/forms/regenerated") {
		t.Fatalf("prints the new URL: %q", out)
	}
}

func TestEmbedPrintsTheSnippet(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.put("form_publications", map[string]any{"id": "p1", "form": "f1", "token": "tok", "enabled": true})
	out, _, err := runCLI(t, f.deps(t), "form", "embed", "jobs")
	if err != nil || !strings.Contains(out, `/p/forms/tok" data-tinycld-form title="Jobs"`) {
		t.Fatalf("%q %v", out, err)
	}
}
