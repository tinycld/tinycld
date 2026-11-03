package automation

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// errRunNotAllowed marks a refusal about who is asking, which the route
// answers with 403 rather than 400.
var errRunNotAllowed = errors.New("you may not run this rule")

// validateRecordRun: an org rule is retried by an admin, a personal rule by
// its owner, and only a record trigger has a record to run for.
func validateRecordRun(rule *core.Record, trigger TriggerDef, caller *core.Record) error {
	if caller == nil {
		return fmt.Errorf("authentication required: %w", errRunNotAllowed)
	}
	if trigger.Synthetic != "" || trigger.Collection == "" {
		return fmt.Errorf("this rule has no record to run for")
	}
	if rule.GetString("scope") == "org" {
		if !isAdmin(caller) {
			return fmt.Errorf("only an admin may run an organization rule: %w", errRunNotAllowed)
		}
	} else if rule.GetString("owner") != caller.Id {
		return fmt.Errorf("not your rule: %w", errRunNotAllowed)
	}
	// Checked after the caller: a retry of a turned-off rule would otherwise
	// run it, an auto-reply the email safety rule allowed only while it was off.
	if !rule.GetBool("enabled") {
		// Shown to the person as is, so it reads as a sentence.
		return errors.New("Turn this rule on to run it again.")
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
			if errors.Is(err, errRunNotAllowed) {
				return re.ForbiddenError(err.Error(), nil)
			}
			return re.BadRequestError(err.Error(), nil)
		}
		record, err := re.App.FindRecordById(trigger.Collection, re.Request.PathValue("recordId"))
		if err != nil {
			return re.NotFoundError("record not found", err)
		}
		// Answered here, not left to dispatch: the caller is told the run was
		// queued only when the rule will run for this record.
		if !ruleAllowed(re.App, ref, ruleConfig(rule), record) {
			return re.NotFoundError("This rule does not run for that record.", nil)
		}
		engine.enqueue(retryEvent(ref, trigger, rule, record))
		return re.JSON(http.StatusOK, map[string]any{"queued": true})
	}).BindFunc(requireAuth)
}
