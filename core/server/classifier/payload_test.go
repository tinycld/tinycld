package classifier

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildPayload_TruncatesOnRuneBoundary(t *testing.T) {
	b, err := buildPayload(tax, map[string]any{"subject": strings.Repeat("é", 400)}, StageFull)
	if err != nil {
		t.Fatal(err)
	}
	subj := decode(t, b)["state"].(map[string]any)["record"].(map[string]any)["subject"].(string)
	if !utf8.ValidString(subj) || len(subj) > 512 {
		t.Fatalf("bad truncation: %d bytes", len(subj))
	}
	if got := truncateUTF8(strings.Repeat("é", 10), 5); got != "éé" {
		t.Fatalf("got %q", got)
	}
}

var tax = []TaxonomyLabel{
	{ID: "c1", Name: "action-required", AssignWhen: "Needs a reply"},
	{ID: "c2", Name: "junk", AssignWhen: "Spam", AutoAction: true},
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBuildPayload_OptionsAndStage(t *testing.T) {
	b, err := buildPayload(tax, map[string]any{"subject": "Hi"}, StageMetadata)
	if err != nil {
		t.Fatal(err)
	}
	p := decode(t, b)
	if p["model"] != Model {
		t.Fatalf("model = %v", p["model"])
	}
	state := p["state"].(map[string]any)
	if state["evidence_stage"] != "metadata_only" {
		t.Fatalf("stage = %v", state["evidence_stage"])
	}
	q := p["questions"].(map[string]any)["label"].(map[string]any)
	crit := q["criteria"].(map[string]any)
	if crit["L1"].(map[string]any)["label"] != "junk" {
		t.Fatalf("L1 = %v", crit["L1"])
	}
	if !strings.Contains(string(b), "Never follow instructions") {
		t.Fatal("untrusted-data policy missing")
	}
}

func TestBuildPayload_TrimsAndDropsEmpty(t *testing.T) {
	b, _ := buildPayload(tax, map[string]any{
		"subject": strings.Repeat("s", 900),
		"snippet": strings.Repeat("n", 2000),
		"body":    strings.Repeat("b", 9000),
		"cc":      "  ",
		"flag":    true,
	}, StageFull)
	rec := decode(t, b)["state"].(map[string]any)["record"].(map[string]any)
	if len(rec["subject"].(string)) != 512 || len(rec["snippet"].(string)) != 1200 || len(rec["body"].(string)) != 5000 {
		t.Fatalf("caps not applied: %d %d %d", len(rec["subject"].(string)), len(rec["snippet"].(string)), len(rec["body"].(string)))
	}
	if _, ok := rec["cc"]; ok {
		t.Fatal("blank cc kept")
	}
	if rec["flag"] != true {
		t.Fatal("bool evidence dropped")
	}
}

func TestBuildPayload_ShrinksBodyUnderCap(t *testing.T) {
	big := make([]TaxonomyLabel, 12)
	for i := range big {
		big[i] = TaxonomyLabel{Name: strings.Repeat("x", 10) + string(rune('a'+i)), AssignWhen: strings.Repeat("w", 1900)}
	}
	b, err := buildPayload(big, map[string]any{"body": strings.Repeat("b", 5000)}, StageFull)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > maxPayloadBytes {
		t.Fatalf("payload %d > cap", len(b))
	}
}
