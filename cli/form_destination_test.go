package main

import (
	"strings"
	"testing"
)

func TestDestinationAddSetsTheFormTrigger(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	path := writeTemp(t, "rule.json", string(exampleFile("destination.json")))
	if _, _, err := runCLI(t, f.deps(t), "form", "destination", "add", "jobs", "--from", path); err != nil {
		t.Fatal(err)
	}
	req := f.last("POST", "/api/collections/rules/records")
	cfg, _ := req.Body["trigger_config"].(map[string]any)
	if req.Body["trigger"] != "core:form-submitted" || cfg["form"] != "f1" || req.Body["scope"] != "org" ||
		req.Body["owner"] != "me123" || req.Body["enabled"] != true || req.Body["name"] != "Jobs destination" {
		t.Fatalf("body = %v", req.Body)
	}
	if _, ok := req.Body["actions"].([]any); !ok {
		t.Fatalf("actions sent as a list: %T", req.Body["actions"])
	}
}

func TestDestinationAddPrintsServerProblemsAndTheHint(t *testing.T) {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.failNext("POST", "rules", 400, `{"status":400,"message":"Failed","data":{"actions.0.ref":{"code":"validation_invalid_action","message":"no installed action \"x:y\"."}}}`)
	path := writeTemp(t, "rule.json", `{"actions":[{"ref":"x:y","params":{}}]}`)
	_, _, err := runCLI(t, f.deps(t), "form", "destination", "add", "jobs", "--from", path)
	if err == nil || !strings.Contains(err.Error(), "actions.0.ref") || !strings.Contains(err.Error(), "See: tinycld form destination actions") {
		t.Fatalf("got %v", err)
	}
}

func TestDestinationActionsListsParams(t *testing.T) {
	f := newFakePB(t)
	f.put("automation_catalog", map[string]any{"id": "c1", "kind": "action", "available": true, "ref": "core:notify", "label": "Send me a notification",
		"definition": map[string]any{"params": []any{map[string]any{"key": "title", "label": "Title", "required": true, "field": map[string]any{"type": "text"}}}}})
	out, _, err := runCLI(t, f.deps(t), "form", "destination", "actions")
	if err != nil || !strings.Contains(out, "core:notify") || !strings.Contains(out, "title:text*") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestDestinationEnableDisableAndRemove(t *testing.T) {
	f := newFakePB(t)
	f.put("rules", map[string]any{"id": "r1", "name": "Card", "scope": "org", "trigger": "core:form-submitted", "trigger_config": map[string]any{"form": "f1"}, "enabled": true})
	d := f.deps(t)
	if _, _, err := runCLI(t, d, "form", "destination", "disable", "Card"); err != nil || f.find("rules", "r1")["enabled"] != false {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := runCLI(t, d, "form", "destination", "remove", "r1"); err == nil {
		t.Fatal("remove needs --yes")
	}
	if _, _, err := runCLI(t, d, "form", "destination", "remove", "r1", "--yes"); err != nil || f.find("rules", "r1") != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestDestinationHelpExplainsTheFile(t *testing.T) {
	f := newFakePB(t)
	out, _, err := runCLI(t, f.deps(t), "form", "destination", "add", "--help")
	if err != nil || !strings.Contains(out, `"actions"`) || !strings.Contains(out, "within_last_days") {
		t.Fatalf("%q %v", out, err)
	}
}
