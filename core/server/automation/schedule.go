// tinycld/core/server/automation/schedule.go
package automation

import (
	"encoding/json"
)

type scheduleConfig struct {
	Cron string `json:"cron"`
}

func decodeScheduleConfig(raw any) scheduleConfig {
	var cfg scheduleConfig
	if raw == nil {
		return cfg
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

func scheduleJobID(ruleID string) string { return "automation:" + ruleID }

// syncSchedules registers cron jobs for every enabled core:schedule rule.
// Called once from Start; per-rule changes reconcile via reloadScheduleFor.
func (e *Engine) syncSchedules() {
	rules, err := e.app.FindRecordsByFilter(
		"rules", "trigger = 'core:schedule' && enabled = true", "", 0, 0,
	)
	if err != nil {
		log.Error("load schedule rules failed", "err", err)
		return
	}
	for _, r := range rules {
		e.reloadScheduleFor(r)
	}
}
