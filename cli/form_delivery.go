package main

import "encoding/json"

// Mirrors core/lib/web-forms/delivery-status.ts; both read
// core/lib/web-forms/fixtures/delivery-cases.json in their tests.
func runSucceeded(r ruleRun) bool {
	if r.Error != "" {
		return false
	}
	var results []struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(r.Results, &results)
	for _, res := range results {
		if res.Status == "error" {
			return false
		}
	}
	return true
}

func newestRun(ruleID string, runs []ruleRun) *ruleRun {
	var newest *ruleRun
	for i := range runs {
		if runs[i].Rule != ruleID {
			continue
		}
		if newest == nil || runs[i].FiredAt > newest.FiredAt {
			newest = &runs[i]
		}
	}
	return newest
}

// deliveryStatus counts only destinations with a run for the submission:
// one added later, or whose old runs were pruned, is not a failed delivery.
func deliveryStatus(destinationIDs []string, runs []ruleRun) (delivered, total int) {
	for _, id := range destinationIDs {
		run := newestRun(id, runs)
		if run == nil {
			continue
		}
		total++
		if runSucceeded(*run) {
			delivered++
		}
	}
	return delivered, total
}
