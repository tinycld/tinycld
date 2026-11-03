package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryStatusMatchesTheAppFixture(t *testing.T) {
	raw, err := os.ReadFile("../core/lib/web-forms/fixtures/delivery-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name             string
		Destinations     []string
		Runs             []ruleRun
		Delivered, Total int
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		d, n := deliveryStatus(c.Destinations, c.Runs)
		if d != c.Delivered || n != c.Total {
			t.Errorf("%s: got %d of %d, want %d of %d", c.Name, d, n, c.Delivered, c.Total)
		}
	}
}

func submissionFixture(t *testing.T) *fakePB {
	f := newFakePB(t)
	f.put("forms", webForm("f1", "Jobs", "jobs"))
	f.put("form_submissions",
		map[string]any{"id": "s1", "form": "f1", "values": map[string]any{"name": "Ada"}, "created": "2026-11-02 10:00:00.000Z"},
		map[string]any{"id": "s2", "form": "f1", "values": map[string]any{"name": "Bo"}, "created": "2026-10-01 10:00:00.000Z"})
	f.put("rules",
		map[string]any{"id": "r1", "name": "Card", "scope": "org", "trigger": "core:form-submitted", "trigger_config": map[string]any{"form": "f1"}, "enabled": true},
		map[string]any{"id": "r2", "name": "Notify", "scope": "org", "trigger": "core:form-submitted", "trigger_config": map[string]any{"form": "f1"}, "enabled": true})
	f.put("rule_runs",
		map[string]any{"id": "x1", "rule": "r1", "record_id": "s1", "fired_at": "2026-11-02 10:00:01.000Z", "error": "", "results": []any{map[string]any{"status": "ok"}}},
		map[string]any{"id": "x2", "rule": "r2", "record_id": "s1", "fired_at": "2026-11-02 10:00:01.000Z", "error": "boom", "results": []any{}},
		map[string]any{"id": "x3", "rule": "r1", "record_id": "s2", "fired_at": "2026-10-01 10:00:01.000Z", "error": "", "results": []any{map[string]any{"status": "ok"}}})
	return f
}

func TestSubmissionListShowsDeliveryAndFilters(t *testing.T) {
	f := submissionFixture(t)
	d := f.deps(t)
	out, _, err := runCLI(t, d, "form", "submission", "list", "jobs")
	if err != nil || !strings.Contains(out, "1 of 2") || !strings.Contains(out, "1 of 1") {
		t.Fatalf("%q %v", out, err)
	}
	out, _, err = runCLI(t, d, "form", "submission", "list", "jobs", "--failed")
	if err != nil || !strings.Contains(out, "s1") || strings.Contains(out, "s2") {
		t.Fatalf("--failed: %q %v", out, err)
	}
	out, _, err = runCLI(t, d, "form", "submission", "list", "jobs", "--since", "2026-11-01")
	if err != nil || strings.Contains(out, "s2") {
		t.Fatalf("--since: %q %v", out, err)
	}
}

func TestSubmissionRetryRetriesOnlyTheFailedDestinations(t *testing.T) {
	f := submissionFixture(t)
	if _, _, err := runCLI(t, f.deps(t), "form", "submission", "retry", "s1"); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, r := range f.requests {
		if strings.HasPrefix(r.Path, "/api/automation/rules/") {
			paths = append(paths, r.Path)
		}
	}
	if len(paths) != 1 || paths[0] != "/api/automation/rules/r2/records/s1/run" {
		t.Fatalf("retries = %v, want only r2", paths)
	}
}

func TestSubmissionDownloadUsesSafeUniqueNames(t *testing.T) {
	f := submissionFixture(t)
	f.put("form_submission_files",
		map[string]any{"id": "fa", "submission": "s1", "field": "cv", "file": "cv_a.pdf", "name": "../../.ssh/authorized_keys", "size": 3},
		map[string]any{"id": "fb", "submission": "s1", "field": "docs", "file": "cv_b.pdf", "name": "cv.pdf", "size": 3},
		map[string]any{"id": "fc", "submission": "s1", "field": "docs", "file": "cv_c.pdf", "name": "cv.pdf", "size": 3})
	f.files["/api/files/form_submission_files/fa/cv_a.pdf"] = []byte("one")
	f.files["/api/files/form_submission_files/fb/cv_b.pdf"] = []byte("two")
	f.files["/api/files/form_submission_files/fc/cv_c.pdf"] = []byte("three")
	dir := t.TempDir()
	if _, _, err := runCLI(t, f.deps(t), "form", "submission", "download", "s1", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"authorized_keys", "cv.pdf", "cv (2).pdf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSubmissionRemoveNeedsYes(t *testing.T) {
	f := submissionFixture(t)
	d := f.deps(t)
	if _, _, err := runCLI(t, d, "form", "submission", "remove", "s1"); err == nil || f.find("form_submissions", "s1") == nil {
		t.Fatal("refused without --yes, nothing deleted")
	}
	if _, _, err := runCLI(t, d, "form", "submission", "remove", "s1", "--yes"); err != nil || f.find("form_submissions", "s1") != nil {
		t.Fatalf("deleted with --yes: %v", err)
	}
}
