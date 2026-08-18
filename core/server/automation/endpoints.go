// tinycld/core/server/automation/endpoints.go
package automation

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func isAdmin(user *core.Record) bool {
	if user == nil {
		return false
	}
	role := user.GetString("role")
	return role == "owner" || role == "admin"
}

func validateManualRun(rule *core.Record, caller *core.Record) error {
	if caller == nil {
		return fmt.Errorf("authentication required")
	}
	if rule.GetString("owner") != caller.Id && !isAdmin(caller) {
		return fmt.Errorf("not your rule")
	}
	switch rule.GetString("trigger") {
	case "core:manual", "core:schedule":
		return nil
	default:
		return fmt.Errorf("only manual/scheduled rules can be run directly")
	}
}

type dryRunMatch struct {
	ID      string         `json:"id"`
	Summary map[string]any `json:"summary"`
}

type dryRunResult struct {
	Total   int           `json:"total"`
	Matches []dryRunMatch `json:"matches"`
}

// ownerFilterFor builds the caller-scoping filter for dry runs from the same
// owner-field detection the dispatcher uses. ok=false when the collection has
// no resolvable owner field.
//
// Known gap (deferred to Phase 3): this only detects a direct user/owner/author
// relation column via autoOwnerFields/OwnerField. A trigger whose owner is
// resolved dynamically via RegisterOwnerResolver (e.g. mail's shared-mailbox
// resolver) has no such column, so dry-run treats it as unscoped/admin-only
// even though ResolveOwners could, in principle, scope it per caller. Fixing
// this requires threading a per-record OwnerResolver call into the dry-run
// filter/query path, not just a static field-name lookup.
func (e *Engine) ownerFilterFor(trigger TriggerDef) (string, bool) {
	col, err := e.app.FindCollectionByNameOrId(trigger.Collection)
	if err != nil {
		return "", false
	}
	usersCol, err := e.app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		return "", false
	}
	candidates := autoOwnerFields
	if trigger.OwnerField != "" {
		candidates = []string{trigger.OwnerField}
	}
	for _, name := range candidates {
		if rel, ok := col.Fields.GetByName(name).(*core.RelationField); ok && rel.CollectionId == usersCol.Id {
			return name + " = {:caller}", true
		}
	}
	return "", false
}

func (e *Engine) dryRun(caller *core.Record, triggerRef string, ast ConditionsAST) (dryRunResult, error) {
	// Non-nil so zero matches marshals to [] rather than null: the client maps
	// over this array, and a null crashes the dry-run panel on the very common
	// "conditions match nothing yet" case.
	out := dryRunResult{Matches: []dryRunMatch{}}
	trigger, _, ok := e.defs.Trigger(triggerRef)
	if !ok || trigger.Synthetic != "" {
		return out, fmt.Errorf("unknown or synthetic trigger %q", triggerRef)
	}
	filter, scoped := e.ownerFilterFor(trigger)
	params := map[string]any{}
	if scoped {
		params["caller"] = caller.Id
	} else {
		if !isAdmin(caller) {
			return out, fmt.Errorf("this trigger's records cannot be scoped to you; ask an admin to test it")
		}
		filter = "id != ''"
	}
	records, err := e.app.FindRecordsByFilter(trigger.Collection, filter, "-created", 50, 0, params)
	if err != nil {
		return out, err
	}
	out.Total = len(records)
	for _, r := range records {
		if !triggerAllowed(e.app, triggerRef, r) {
			continue
		}
		if EvaluateConditions(ast, r, trigger) {
			out.Matches = append(out.Matches, dryRunMatch{ID: r.Id, Summary: triggerSummary(r, trigger)})
		}
	}
	return out, nil
}

func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}

func registerEndpoints(app core.App, engine *Engine) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/automation/rules/{id}/run", func(re *core.RequestEvent) error {
			rule, err := re.App.FindRecordById("rules", re.Request.PathValue("id"))
			if err != nil {
				return re.NotFoundError("rule not found", err)
			}
			if err := validateManualRun(rule, re.Auth); err != nil {
				return re.BadRequestError(err.Error(), nil)
			}
			trigger, _, _ := engine.defs.Trigger(rule.GetString("trigger"))
			engine.enqueue(event{TriggerRef: rule.GetString("trigger"), Trigger: trigger, RuleID: rule.Id})
			return re.JSON(http.StatusOK, map[string]any{"queued": true})
		}).BindFunc(requireAuth)

		se.Router.POST("/api/automation/dry-run", func(re *core.RequestEvent) error {
			var body struct {
				Trigger    string        `json:"trigger"`
				Conditions ConditionsAST `json:"conditions"`
			}
			if err := re.BindBody(&body); err != nil {
				return re.BadRequestError("invalid body", err)
			}
			res, err := engine.dryRun(re.Auth, body.Trigger, body.Conditions)
			if err != nil {
				return re.BadRequestError(err.Error(), nil)
			}
			return re.JSON(http.StatusOK, res)
		}).BindFunc(requireAuth)

		return se.Next()
	})
}
