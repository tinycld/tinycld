// Package fakeprovider is a deterministic Jev stand-in for unit tests, local
// dev and e2e. It answers with real Decisions JSON, so the classifier's
// parser runs exactly as in production. The record's subject names the
// label to choose; a subject without a marker gets HTTP 400 so mail that is
// not about classification is never labelled.
package fakeprovider

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"

	"tinycld.org/core/classifier"
)

const AcceptedKey = "test-key"

var (
	labelMarker = regexp.MustCompile(`\[jev:([^\]]+)\]`)
	confMarker  = regexp.MustCompile(`\[conf:([0-9.]+)\]`)
)

type Fake struct {
	Calls atomic.Int32
	// Reply, when set, replaces the marker logic (unit tests that script a
	// sequence of answers). It receives the 1-based call number.
	Reply func(n int32, payload []byte) (status int, body string)
}

func New() *Fake { return &Fake{} }

type payload struct {
	State struct {
		Record map[string]any `json:"record"`
	} `json:"state"`
	Questions struct {
		Label struct {
			Criteria map[string]struct {
				Label string `json:"label"`
			} `json:"criteria"`
		} `json:"label"`
	} `json:"questions"`
}

func (f *Fake) Decide(_ context.Context, apiKey string, raw []byte) (classifier.Response, error) {
	n := f.Calls.Add(1)
	if f.Reply != nil {
		status, body := f.Reply(n, raw)
		return classifier.Response{Status: status, Body: []byte(body)}, nil
	}
	if apiKey != AcceptedKey {
		return classifier.Response{Status: 401, Body: []byte(`{}`)}, nil
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return classifier.Response{Status: 400, Body: []byte(`{}`)}, nil
	}
	keys := make([]string, 0, len(p.Questions.Label.Criteria))
	for k := range p.Questions.Label.Criteria {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	subject, _ := p.State.Record["subject"].(string)
	choice := ""
	if m := labelMarker.FindStringSubmatch(subject); m != nil {
		for _, k := range keys {
			if p.Questions.Label.Criteria[k].Label == m[1] {
				choice = k
			}
		}
	}
	if choice == "" && subject == "Hello there" && len(keys) > 0 {
		choice = keys[0]
	}
	if choice == "" {
		return classifier.Response{Status: 400, Body: []byte(`{}`)}, nil
	}
	conf := 0.95
	if m := confMarker.FindStringSubmatch(subject); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			conf = v
		}
	}
	probs := map[string]float64{}
	for _, k := range keys {
		probs[k] = 0
	}
	probs[choice] = 1
	body, _ := json.Marshal(map[string]any{
		"model":   "fake-jev",
		"answers": map[string]any{"label": map[string]any{"choice": choice, "confidence": conf, "probabilities": probs}},
		"usage":   map[string]any{"cost": 0.00001},
	})
	return classifier.Response{Status: 200, Body: body}, nil
}

// Answer builds a Decisions body for Reply scripts.
func Answer(choice string, conf float64, probs map[string]float64) string {
	b, _ := json.Marshal(map[string]any{
		"answers": map[string]any{"label": map[string]any{"choice": choice, "confidence": conf, "probabilities": probs}},
		"usage":   map[string]any{"cost": 0.00001},
	})
	return string(b)
}
