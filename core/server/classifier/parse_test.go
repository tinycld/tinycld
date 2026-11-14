package classifier

import (
	"net/http"
	"testing"
)

func ok(body string) (Result, *Failure) {
	return parseResponse(200, http.Header{}, []byte(body), tax, 0.0001)
}

func TestParse_Success(t *testing.T) {
	r, f := ok(`{"model":"jev-1","answers":{"label":{"choice":"L1","confidence":0.92,"probabilities":{"L0":0.1,"L1":0.9}}},"usage":{"cost":0.00002}}`)
	if f != nil {
		t.Fatal(f)
	}
	if r.Label.Name != "junk" || r.Confidence != 0.92 || r.CostUSD != 0.00002 || r.ArgmaxMismatch {
		t.Fatalf("%+v", r)
	}
	if r.Probabilities["junk"] != 0.9 {
		t.Fatalf("probabilities keyed by name: %v", r.Probabilities)
	}
}

func TestParse_CostFallbacks(t *testing.T) {
	r, _ := ok(`{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":0.6,"L1":0.4}}},"usage":{"input_tokens":1000000}}`)
	if r.CostUSD != 0.042 {
		t.Fatalf("token cost = %v", r.CostUSD)
	}
	r, _ = ok(`{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":0.6,"L1":0.4}}}}`)
	if r.CostUSD != 0.0001 {
		t.Fatalf("estimate cost = %v", r.CostUSD)
	}
}

func TestParse_ArgmaxMismatch(t *testing.T) {
	r, _ := ok(`{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":0.3,"L1":0.7}}}}`)
	if !r.ArgmaxMismatch {
		t.Fatal("mismatch not flagged")
	}
}

func TestParse_Failures(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		code       string
		retry      bool
		fatal      string
	}{
		{"401", `{}`, 401, "authentication_failed", false, "auth_failed"},
		{"402", `{}`, 402, "insufficient_credits", false, "no_credits"},
		{"403", `{}`, 403, "access_denied", false, "access_denied"},
		{"429", `{}`, 429, "provider_unavailable", true, ""},
		{"503", `{}`, 503, "provider_unavailable", true, ""},
		{"400", `{}`, 400, "request_rejected", false, ""},
		{"bad json", `nope`, 200, "invalid_json", false, ""},
		{"no answer", `{"answers":{}}`, 200, "missing_answer", false, ""},
		{"bad choice", `{"answers":{"label":{"choice":"L9","confidence":0.5,"probabilities":{"L0":0.5,"L1":0.5}}}}`, 200, "invalid_choice", false, ""},
		{"bad confidence", `{"answers":{"label":{"choice":"L0","confidence":2,"probabilities":{"L0":0.5,"L1":0.5}}}}`, 200, "invalid_confidence", false, ""},
		{"missing prob", `{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":1}}}}`, 200, "invalid_probabilities", false, ""},
		{"extra prob", `{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":0.5,"L1":0.4,"L7":0.1}}}}`, 200, "invalid_probabilities", false, ""},
		{"sum", `{"answers":{"label":{"choice":"L0","confidence":0.5,"probabilities":{"L0":0.5,"L1":0.2}}}}`, 200, "invalid_probabilities", false, ""},
	}
	for _, c := range cases {
		_, f := parseResponse(c.status, http.Header{}, []byte(c.body), tax, 0.0001)
		if f == nil || f.Code != c.code || f.Retryable != c.retry || f.Status != c.fatal {
			t.Errorf("%s: got %+v", c.name, f)
		}
	}
}

func TestParse_RetryAfter(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "3")
	_, f := parseResponse(429, h, []byte(`{}`), tax, 0)
	if f.RetryAfter.Seconds() != 3 {
		t.Fatalf("retry after = %v", f.RetryAfter)
	}
}
