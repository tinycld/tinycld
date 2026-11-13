package fakeprovider_test

import (
	"context"
	"encoding/json"
	"testing"

	"tinycld.org/core/classifier/fakeprovider"
)

func payloadFor(subject string) []byte {
	b, _ := json.Marshal(map[string]any{
		"state": map[string]any{"record": map[string]any{"subject": subject}},
		"questions": map[string]any{"label": map[string]any{"criteria": map[string]any{
			"L0": map[string]any{"label": "personal"},
			"L1": map[string]any{"label": "junk"},
		}}},
	})
	return b
}

func decide(t *testing.T, key, subject string) (int, string, float64) {
	t.Helper()
	resp, err := fakeprovider.New().Decide(context.Background(), key, payloadFor(subject))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Answers struct {
			Label struct {
				Choice     string  `json:"choice"`
				Confidence float64 `json:"confidence"`
			} `json:"label"`
		} `json:"answers"`
	}
	_ = json.Unmarshal(resp.Body, &out)
	return resp.Status, out.Answers.Label.Choice, out.Answers.Label.Confidence
}

func TestFake(t *testing.T) {
	if s, _, _ := decide(t, "wrong", "[jev:junk] x"); s != 401 {
		t.Fatalf("wrong key status = %d", s)
	}
	if s, c, conf := decide(t, fakeprovider.AcceptedKey, "[jev:junk] x"); s != 200 || c != "L1" || conf != 0.95 {
		t.Fatalf("marker: %d %s %v", s, c, conf)
	}
	if s, c, conf := decide(t, fakeprovider.AcceptedKey, "[jev:junk][conf:0.6]"); s != 200 || c != "L1" || conf != 0.6 {
		t.Fatalf("conf: %d %s %v", s, c, conf)
	}
	if s, _, _ := decide(t, fakeprovider.AcceptedKey, "plain"); s != 400 {
		t.Fatalf("no marker status = %d", s)
	}
	if s, c, _ := decide(t, fakeprovider.AcceptedKey, "Hello there"); s != 200 || c != "L0" {
		t.Fatalf("probe: %d %s", s, c)
	}
}
