package automation

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// validateRecordRun: an org rule is retried by an admin, a personal rule by
// its owner, and only a record trigger has a record to run for.
func validateRecordRun(rule *core.Record, trigger TriggerDef, caller *core.Record) error {
	if caller == nil {
		return fmt.Errorf("authentication required")
	}
	if trigger.Synthetic != "" || trigger.Collection == "" {
		return fmt.Errorf("this rule has no record to run for")
	}
	if rule.GetString("scope") == "org" {
		if !isAdmin(caller) {
			return fmt.Errorf("only an admin may run an organization rule")
		}
		return nil
	}
	if rule.GetString("owner") != caller.Id {
		return fmt.Errorf("not your rule")
	}
	return nil
}

// retryEvent targets one rule (the RuleID path skips the enabled filter, as
// a manual run does) with one record; dispatch still applies the rule filter
// and the personal-owner check.
func retryEvent(triggerRef string, trigger TriggerDef, rule, record *core.Record) event {
	return event{TriggerRef: triggerRef, Trigger: trigger, Record: record, RuleID: rule.Id}
}

func bindRetryRoute(se *core.ServeEvent, engine *Engine) {
	se.Router.POST("/api/automation/rules/{id}/records/{recordId}/run", func(re *core.RequestEvent) error {
		rule, err := re.App.FindRecordById("rules", re.Request.PathValue("id"))
		if err != nil {
			return re.NotFoundError("rule not found", err)
		}
		ref := rule.GetString("trigger")
		trigger, _, ok := engine.defs.Trigger(ref)
		if !ok {
			return re.BadRequestError("the rule's trigger is not installed", nil)
		}
		if err := validateRecordRun(rule, trigger, re.Auth); err != nil {
			return re.BadRequestError(err.Error(), nil)
		}
		record, err := re.App.FindRecordById(trigger.Collection, re.Request.PathValue("recordId"))
		if err != nil {
			return re.NotFoundError("record not found", err)
		}
		engine.enqueue(retryEvent(ref, trigger, rule, record))
		return re.JSON(http.StatusOK, map[string]any{"queued": true})
	}).BindFunc(requireAuth)
}
