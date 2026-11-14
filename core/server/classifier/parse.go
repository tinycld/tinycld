package classifier

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	inputRateUSDPerMillion  = 0.042
	probabilitySumTolerance = 0.01
	maxRetryAfter           = 10 * time.Second
)

// Failure is a classification that produced no usable decision. Status is
// set only for errors that make every later call fail the same way, so the
// caller pauses the feature until an admin re-verifies the key.
type Failure struct {
	Code       string
	Retryable  bool
	Status     string
	RetryAfter time.Duration
	CostUSD    float64
}

func (f *Failure) Error() string { return "classifier: " + f.Code }

type jevResponse struct {
	Model   string `json:"model"`
	Answers map[string]struct {
		Choice        string             `json:"choice"`
		Confidence    *float64           `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
	} `json:"answers"`
	Usage struct {
		Cost        *float64 `json:"cost"`
		InputTokens *float64 `json:"input_tokens"`
	} `json:"usage"`
}

func retryAfter(h http.Header) time.Duration {
	raw := h.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs >= 0 {
		return min(time.Duration(secs)*time.Second, maxRetryAfter)
	}
	if at, err := http.ParseTime(raw); err == nil {
		return min(max(time.Until(at), 0), maxRetryAfter)
	}
	return 0
}

func parseResponse(status int, header http.Header, body []byte, taxonomy []TaxonomyLabel, estimate float64) (Result, *Failure) {
	var resp jevResponse
	jsonErr := json.Unmarshal(body, &resp)
	cost := estimate
	switch {
	case jsonErr == nil && resp.Usage.Cost != nil && *resp.Usage.Cost >= 0:
		cost = *resp.Usage.Cost
	case jsonErr == nil && resp.Usage.InputTokens != nil && *resp.Usage.InputTokens >= 0:
		cost = *resp.Usage.InputTokens * inputRateUSDPerMillion / 1_000_000
	}
	fail := func(code string, retryable bool, fatal string) (Result, *Failure) {
		return Result{}, &Failure{Code: code, Retryable: retryable, Status: fatal, RetryAfter: retryAfter(header), CostUSD: cost}
	}

	switch {
	case status == 401:
		return fail("authentication_failed", false, "auth_failed")
	case status == 402:
		return fail("insufficient_credits", false, "no_credits")
	case status == 403:
		return fail("access_denied", false, "access_denied")
	case status == 408 || status == 409 || status == 425 || status == 429 || status >= 500:
		return fail("provider_unavailable", true, "")
	case status < 200 || status >= 300:
		return fail("request_rejected", false, "")
	case jsonErr != nil:
		return fail("invalid_json", false, "")
	}

	answer, found := resp.Answers["label"]
	if !found {
		return fail("missing_answer", false, "")
	}
	idx := -1
	for i := range taxonomy {
		if optionKey(i) == answer.Choice {
			idx = i
		}
	}
	if idx < 0 {
		return fail("invalid_choice", false, "")
	}
	if answer.Confidence == nil || math.IsNaN(*answer.Confidence) || *answer.Confidence < 0 || *answer.Confidence > 1 {
		return fail("invalid_confidence", false, "")
	}
	if len(answer.Probabilities) != len(taxonomy) {
		return fail("invalid_probabilities", false, "")
	}
	probs := make(map[string]float64, len(taxonomy))
	sum, best, bestKey := 0.0, -1.0, ""
	for i, l := range taxonomy {
		p, has := answer.Probabilities[optionKey(i)]
		if !has || math.IsNaN(p) || p < 0 || p > 1 {
			return fail("invalid_probabilities", false, "")
		}
		probs[l.Name] = p
		sum += p
		if p > best {
			best, bestKey = p, optionKey(i)
		}
	}
	if math.Abs(sum-1) > probabilitySumTolerance {
		return fail("invalid_probabilities", false, "")
	}
	model := resp.Model
	if model == "" {
		model = Model
	}
	return Result{
		Label:          taxonomy[idx],
		Confidence:     *answer.Confidence,
		Probabilities:  probs,
		ArgmaxMismatch: bestKey != answer.Choice && answer.Probabilities[answer.Choice]+1e-9 < best,
		Model:          model,
		CostUSD:        cost,
	}, nil
}
