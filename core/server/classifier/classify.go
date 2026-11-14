package classifier

import (
	"context"
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("classifier")

type Request struct {
	Taxonomy     []TaxonomyLabel
	Evidence     map[string]any
	FullEvidence func(ctx context.Context) (map[string]any, error)
	// NeedsFull adds an app-specific reason to run the second stage, such as
	// an automatic action that needs stronger evidence.
	NeedsFull func(r Result) bool
}

func estimateCost(payload []byte) float64 {
	return float64(len(payload)) / 4 * inputRateUSDPerMillion / 1_000_000
}

// Classify asks Jev for one decision, first from Evidence, then from
// Evidence plus FullEvidence when the first answer is not confident enough.
func Classify(ctx context.Context, app core.App, req Request) (Result, error) {
	cfg := LoadConfig(app)
	if cfg.APIKey == "" || cfg.Status != StatusOK {
		return Result{}, ErrUnavailable
	}
	first, err := call(ctx, app, cfg, req.Taxonomy, req.Evidence, StageMetadata, true)
	if err != nil {
		return Result{}, err
	}
	wantFull := first.Confidence < cfg.FullThreshold || (req.NeedsFull != nil && req.NeedsFull(first))
	if req.FullEvidence == nil || !wantFull {
		return first, nil
	}
	extra, err := req.FullEvidence(ctx)
	if err != nil {
		log.WarnContext(ctx, "full evidence unavailable; keeping metadata decision", "err", err)
		return first, nil
	}
	merged := make(map[string]any, len(req.Evidence)+len(extra))
	for k, v := range req.Evidence {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	second, err := call(ctx, app, cfg, req.Taxonomy, merged, StageFull, true)
	if err != nil {
		// The metadata decision stands; any automatic action still has to
		// clear its own threshold against it.
		log.WarnContext(ctx, "full-evidence call failed; keeping metadata decision", "err", err)
		return first, nil
	}
	second.CostUSD += first.CostUSD
	return second, nil
}

func call(ctx context.Context, app core.App, cfg Config, taxonomy []TaxonomyLabel, evidence map[string]any, stage string, enforceBudget bool) (Result, error) {
	payload, err := buildPayload(taxonomy, evidence, stage)
	if err != nil {
		return Result{}, err
	}
	estimate := estimateCost(payload)
	if enforceBudget {
		if err := checkBudget(app, cfg, estimate, time.Now()); err != nil {
			return Result{}, err
		}
	}
	for attempt := 0; ; attempt++ {
		res, fail := post(ctx, app, cfg, payload, taxonomy, estimate)
		if fail != nil && fail.CostUSD > 0 || fail == nil {
			cost := res.CostUSD
			if fail != nil {
				cost = fail.CostUSD
			}
			if err := addUsage(app, cost, time.Now()); err != nil {
				log.ErrorContext(ctx, "recording classifier usage failed", "err", err)
			}
		}
		if fail == nil {
			res.Stage = stage
			return res, nil
		}
		if fail.Status != "" {
			if err := setStatus(app, fail.Status); err != nil {
				log.ErrorContext(ctx, "writing classifier status failed", "err", err)
			}
			return Result{}, fail
		}
		if !fail.Retryable || attempt >= 1 {
			return Result{}, fail
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(max(fail.RetryAfter, 500*time.Millisecond)):
		}
	}
}

func post(ctx context.Context, app core.App, cfg Config, payload []byte, taxonomy []TaxonomyLabel, estimate float64) (Result, *Failure) {
	resp, err := providerFor(app).Decide(ctx, cfg.APIKey, payload)
	if err != nil {
		// The key travels in a header, never in the error text.
		log.WarnContext(ctx, "classifier transport error", "err", err)
		// Nothing proves the request was not billed, so the estimate counts.
		return Result{}, &Failure{Code: "transport_error", Retryable: true, CostUSD: estimate}
	}
	return parseResponse(resp.Status, resp.Header, resp.Body, taxonomy, estimate)
}

// ErrorCode is the value an app stores in classifications.error.
func ErrorCode(err error) string {
	var f *Failure
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrBudgetExceeded):
		return "budget_exceeded"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	case errors.Is(err, ErrPayloadTooLarge):
		return "payload_too_large"
	case errors.As(err, &f):
		return f.Code
	default:
		return "internal_error"
	}
}
