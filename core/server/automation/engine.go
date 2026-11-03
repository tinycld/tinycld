// tinycld/core/server/automation/engine.go
package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/background"
	"tinycld.org/core/readonly"
	"tinycld.org/core/shutdown"
)

const (
	maxChainDepth    = 3
	actionTimeout    = 30 * time.Second
	dispatchQueueLen = 1024
	// shutdownDrainWait bounds how long OnTerminate waits for an in-flight
	// dispatch, and the actions it started, to finish. The terminate cancels
	// every action's context first, so an action that honours it returns at
	// once; an action that ignores it must not hold up process shutdown.
	// The terminate's shared deadline (tinycld.org/core/shutdown) can cut it
	// shorter.
	shutdownDrainWait = 5 * time.Second
)

type event struct {
	TriggerRef string
	Trigger    TriggerDef
	Record     *core.Record
	Depth      int
	SourceRule string
	// RuleID targets one specific rule (scheduled dispatch — each cron job
	// has its own cadence, so a firing must hit exactly its rule). Empty
	// means all enabled rules on the trigger, the record-hook path's behavior.
	RuleID string
}

type Engine struct {
	app   core.App
	defs  *Defs
	queue chan event
	// ctx ends at OnTerminate. Every action's context derives from it, so
	// the terminate cancels the actions in flight.
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
	// actions counts action goroutines still running, including those the
	// per-action timeout gave up on. The terminate waits for them.
	actions sync.WaitGroup
	// waitWritable waits for app's read-only mode to end; a test swaps it before starting
	// a worker to learn that the worker has reached the wait.
	waitWritable func(context.Context) error
}

func NewEngine(app core.App, defs *Defs) *Engine {
	defsKey.Set(app, defs)
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		app: app, defs: defs,
		queue: make(chan event, dispatchQueueLen), done: make(chan struct{}),
		ctx: ctx, cancel: cancel,
		waitWritable: func(ctx context.Context) error { return readonly.WaitInactive(ctx, app) },
	}
}

// Start binds one hook per distinct (collection, op) named by the defs, plus
// the rules-change hooks for schedule reconciliation, and launches the worker.
// Rule execution never runs on the hook goroutine: a slow rule must not block
// an SMTP delivery.
func (e *Engine) Start() {
	if e.started {
		log.Warn("Start called more than once, ignoring")
		return
	}
	e.started = true

	shutdown.Register(e.app)
	e.app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
		e.cancel()
		ctx, cancel := shutdown.Within(e.app, shutdownDrainWait)
		defer cancel()
		e.drain(ctx)
		return te.Next()
	})
	// Bound after the drain above, so the runner stops after the worker: a
	// notice the last dispatch starts is then waited for, not dropped.
	background.For(e.app, jobsPkg)
	go e.worker(e.ctx)

	type colOp struct{ col, op string }
	seen := map[colOp]bool{}
	for _, p := range e.defs.Packages {
		for _, t := range p.Triggers {
			if t.Synthetic != "" {
				continue
			}
			key := colOp{t.Collection, t.On}
			if seen[key] {
				continue
			}
			seen[key] = true
			handler := e.recordHookHandler(key.col, key.op)
			switch key.op {
			case "create":
				e.app.OnRecordAfterCreateSuccess(key.col).BindFunc(handler)
			case "update":
				e.app.OnRecordAfterUpdateSuccess(key.col).BindFunc(handler)
			case "delete":
				e.app.OnRecordAfterDeleteSuccess(key.col).BindFunc(handler)
			}
		}
	}

	reload := func(ev *core.RecordEvent) error {
		// The auto-disable path (recordRunResult) marks an engine-write
		// sentinel before saving the rule it just disabled. Nothing else
		// ever consumes that sentinel for the "rules" collection itself, so
		// drain it here — otherwise it lingers and misattributes provenance
		// (depth/source rule) to whatever later legitimate save of the same
		// rule row happens to land next.
		takeEngineWrite(e.app, ev.Record.Id)
		e.reloadScheduleFor(ev.Record)
		return ev.Next()
	}
	e.app.OnRecordAfterCreateSuccess("rules").BindFunc(reload)
	e.app.OnRecordAfterUpdateSuccess("rules").BindFunc(reload)
	e.app.OnRecordAfterDeleteSuccess("rules").BindFunc(func(ev *core.RecordEvent) error {
		// A deleted record's fields still read as they were pre-delete (e.g.
		// enabled=true), so routing through reloadScheduleFor's field check
		// would re-Add instead of Remove. The row is gone either way — just
		// drop the job unconditionally.
		takeEngineWrite(e.app, ev.Record.Id)
		e.app.Cron().Remove(scheduleJobID(ev.Record.Id))
		return ev.Next()
	})

	e.syncSchedules()
}

// reloadScheduleFor reconciles one rule's cron registration after any rules
// write — the base_backup.go loadJob shape: Add replaces by id, Remove is a
// no-op for unknown ids, so this is idempotent for every transition
// (create/enable/disable/delete/edit-expression).
func (e *Engine) reloadScheduleFor(rule *core.Record) {
	jobID := scheduleJobID(rule.Id)
	if rule.GetString("trigger") != "core:schedule" || !rule.GetBool("enabled") {
		e.app.Cron().Remove(jobID)
		return
	}
	cfg := decodeScheduleConfig(rule.Get("trigger_config"))
	trigger, _, _ := e.defs.Trigger("core:schedule")
	ruleID := rule.Id
	err := e.app.Cron().Add(jobID, cfg.Cron, func() {
		if !appIsLive(e.app) {
			return
		}
		e.enqueue(event{TriggerRef: "core:schedule", Trigger: trigger, Record: nil, RuleID: ruleID})
	})
	if err != nil {
		// Info, not warn: the cron expression is user input on the rule, and the
		// failure is already surfaced to that user through the rule_runs row below.
		log.Info("invalid schedule", "ruleID", rule.Id, "cron", cfg.Cron, "err", err)
		WriteRun(e.app, rule, nil, TriggerDef{}, RunOutcome{Err: "invalid schedule: " + cfg.Cron})
	}
}

func (e *Engine) recordHookHandler(col, op string) func(*core.RecordEvent) error {
	return func(ev *core.RecordEvent) error {
		depth := 0
		source := ""
		if w, ok := takeEngineWrite(e.app, ev.Record.Id); ok {
			depth = w.Depth + 1
			source = w.RuleID
		}
		if depth > maxChainDepth {
			// Info, not warn: rules that trigger each other are a user
			// misconfiguration, recorded to rule_runs for that user, and a loop
			// would hit this on every write in the cycle.
			log.Info("chain depth exceeded", "collection", col, "recordID", ev.Record.Id, "sourceRuleID", source)
			if source != "" {
				if rule, err := e.app.FindRecordById("rules", source); err == nil {
					// nil record, not ev.Record: an empty TriggerDef has no
					// declared field allowlist, so triggerSummary would fall
					// into the "open trigger" branch and snapshot EVERY column
					// of the record — including curated-away/hidden fields.
					// The error string is the payload here; the record adds
					// nothing but a leak.
					WriteRun(e.app, rule, nil, TriggerDef{}, RunOutcome{Err: "chain-depth-exceeded"})
				}
			}
			return ev.Next()
		}
		for _, qt := range e.defs.TriggersFor(col, op) {
			if op == "update" && !WatchChanged(ev.Record, qt.Def.Watch) {
				continue
			}
			if !triggerAllowed(e.app, qt.Ref, ev.Record) {
				continue
			}
			e.enqueue(event{TriggerRef: qt.Ref, Trigger: qt.Def, Record: ev.Record, Depth: depth, SourceRule: source})
		}
		return ev.Next()
	}
}

// drain waits, at most wait in all, for the worker to exit and then for every
// action goroutine to return. The caller has cancelled e.ctx, so the worker
// starts no new action and each action's context has ended. The wait is
// needed because a dispatch in flight still touches the DB, and a test app's
// Cleanup() tears the DB down right after the terminate hook returns.
//
// Bounded rather than unconditional: an action that ignores its context, or
// a DB call that cannot be cancelled, must not hold up process shutdown. Such
// an action is abandoned; its next query fails against the closed database
// (the panic is recovered) and that write is lost — the risk ActionHandler's
// contract documents.
// drain waits, until ctx ends, for the worker to exit and then for the
// actions it started.
func (e *Engine) drain(ctx context.Context) {
	select {
	case <-e.done:
	case <-ctx.Done():
		log.Warn("worker did not drain before terminate; abandoning in-flight dispatch",
			"cause", context.Cause(ctx))
		return
	}
	// Only after the worker has exited: it is the one goroutine that adds to
	// e.actions, and a WaitGroup must not see an Add race its Wait.
	actionsDone := make(chan struct{})
	go func() {
		e.actions.Wait()
		close(actionsDone)
	}()
	select {
	case <-actionsDone:
	case <-ctx.Done():
		log.Warn("actions did not return after cancel before terminate; abandoning them",
			"cause", context.Cause(ctx))
	}
}

func (e *Engine) enqueue(ev event) {
	select {
	case e.queue <- ev:
	default:
		// A dropped dispatch is recoverable (the data is in the DB); a blocked
		// write path is not. Log loudly and move on.
		log.Error("dispatch queue full, dropping event", "trigger", ev.TriggerRef)
	}
}

func (e *Engine) worker(ctx context.Context) {
	defer close(e.done)
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-e.queue:
			if !appIsLive(e.app) {
				return
			}
			// A dispatch writes records outside any request, so the read-only
			// middleware cannot refuse it. Hold the event until the mode ends;
			// later events wait in the queue meanwhile, and a full queue drops
			// them in enqueue as it always has. ctx ends at OnTerminate.
			if err := e.waitWritable(ctx); err != nil {
				log.Warn("shutting down while read-only: dropping held events",
					"dropped", 1+len(e.queue))
				return
			}
			e.dispatchSafely(ctx, ev)
		}
	}
}

// dispatchSafely contains a panic to the one event that caused it. The worker
// is the only consumer of the queue, so an unwinding panic here would take the
// process down and leave every later trigger unserved — a single malformed rule
// must not become a crash loop.
func (e *Engine) dispatchSafely(ctx context.Context, ev event) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("dispatch panicked, dropping event",
				"trigger", ev.TriggerRef, "rule", ev.RuleID, "panic", r,
				"stack", string(debug.Stack()))
		}
	}()
	e.dispatch(ctx, ev)
}

// DispatchForTest runs one event synchronously.
func (e *Engine) DispatchForTest(ev event) { e.dispatch(e.ctx, ev) }

type storedAction struct {
	Ref    string         `json:"ref"`
	Params map[string]any `json:"params"`
}

func decodeActions(raw any) ([]storedAction, error) {
	var out []storedAction
	if raw == nil {
		return out, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || string(b) == "null" || string(b) == `""` {
		return out, nil
	}
	return out, json.Unmarshal(b, &out)
}

func (e *Engine) dispatch(ctx context.Context, ev event) {
	var rules []*core.Record
	if ev.RuleID != "" {
		// Manual-run/scheduled-run targets one specific rule directly, bypassing
		// the enabled filter: the endpoint already replied {"queued": true}
		// regardless of enabled, so a disabled rule must still run here or the
		// dispatch silently no-ops after promising the caller it was queued.
		rule, err := e.app.FindRecordById("rules", ev.RuleID)
		if err != nil {
			return
		}
		if rule.GetString("trigger") != ev.TriggerRef {
			return
		}
		rules = []*core.Record{rule}
	} else {
		found, err := e.app.FindRecordsByFilter(
			"rules", "trigger = {:ref} && enabled = true", "order", 0, 0,
			map[string]any{"ref": ev.TriggerRef},
		)
		if err != nil || len(found) == 0 {
			return
		}
		rules = found
	}
	// Org rules first, then personal — order preserved within each tier.
	sort.SliceStable(rules, func(i, j int) bool {
		si, sj := rules[i].GetString("scope"), rules[j].GetString("scope")
		if si != sj {
			return si == "org"
		}
		return false
	})

	owners := ResolveOwners(e.app, ev.TriggerRef, ev.Trigger, ev.Record)
	ownerSet := map[string]bool{}
	for _, id := range owners {
		ownerSet[id] = true
	}

	// An org rule's stop halts everything downstream; a personal rule's stop
	// halts only that owner's later rules. A shared-audience trigger (a team
	// mailbox, a board everyone watches) dispatches one event to every member's
	// personal rules, so a global stop would let whoever happens to sort first
	// silently switch off everybody else's automation.
	stopped := false
	stoppedOwners := map[string]bool{}
	for _, rule := range rules {
		if stopped {
			break
		}
		// The app is terminating: the rules not yet run wait for the next
		// trigger, as they would had the event arrived a moment later.
		if ctx.Err() != nil {
			return
		}
		if rule.Id == ev.SourceRule {
			continue // a rule never re-fires on its own write
		}
		if stoppedOwners[rule.GetString("owner")] {
			continue
		}
		// No record → no owner to resolve (ResolveOwners returns nil); the
		// rule firing IS the owner's context (e.g. a scheduled rule), so the
		// personal-owner filter doesn't apply.
		if ev.Record != nil && rule.GetString("scope") == "personal" && !ownerSet[rule.GetString("owner")] {
			continue
		}
		config := ruleConfig(rule)
		if !ruleAllowed(e.app, ev.TriggerRef, config, ev.Record) {
			continue // not this rule's record (another form): no run row
		}
		fields := dynamicFields(e.app, ev.TriggerRef, config, ev.Record)
		dyn := dynamicValues(fields)
		start := time.Now()

		ast, err := DecodeConditions(rule.Get("conditions"))
		if err != nil {
			e.logRun(rule, ev, RunOutcome{Err: "invalid conditions: " + err.Error(), Duration: time.Since(start), Fields: dyn})
			continue
		}
		if !EvaluateConditionsWith(ast, ev.Record, ev.Trigger, dyn) {
			e.logRun(rule, ev, RunOutcome{Matched: false, Duration: time.Since(start), Fields: dyn})
			continue
		}

		actions, err := decodeActions(rule.Get("actions"))
		if err != nil {
			e.logRun(rule, ev, RunOutcome{Matched: true, Err: "invalid actions: " + err.Error(), Duration: time.Since(start), Fields: dyn})
			continue
		}
		results := make([]ActionResult, 0, len(actions))
		failed := 0
		for _, a := range actions {
			res := actionResult(a.Ref, e.runActionWithTimeout(ctx, a, rule, ev, fields))
			if res.Status == "error" {
				failed++
			}
			results = append(results, res)
		}
		// The run row is written last: a reader that sees it (the rules UI,
		// a test waiting on rule_runs) must also see the failure streak and
		// any auto-disable this run caused. A run the terminate cut short is
		// not the rule failing, so it does not count toward auto-disable.
		if ctx.Err() == nil {
			recordRunResult(e.app, rule, len(actions) > 0 && failed == len(actions))
		}
		e.logRun(rule, ev, RunOutcome{Matched: true, Results: results, Duration: time.Since(start), Fields: dyn})

		if rule.GetBool("stop_processing") {
			if rule.GetString("scope") == "org" {
				stopped = true // org stop halts everything downstream
			} else {
				// Personal stop: only this owner's later rules. Org rules have
				// already run (org-first ordering), so nothing of theirs is
				// skipped by stopping here.
				stoppedOwners[rule.GetString("owner")] = true
			}
		}
	}
}

// logRun writes one run row and then tells the trigger's run observers.
func (e *Engine) logRun(rule *core.Record, ev event, outcome RunOutcome) {
	WriteRun(e.app, rule, ev.Record, ev.Trigger, outcome)
	notifyRunObservers(e.app, ev.TriggerRef, rule, ev.Record, outcome)
}

// actionResult is the run-history entry for one action's outcome. A skip is
// its own status, not a failure, so a rule that only skips is never
// auto-disabled.
func actionResult(ref string, err error) ActionResult {
	if err == nil {
		return ActionResult{Ref: ref, Status: "ok"}
	}
	if reason, ok := skipReason(err); ok {
		return ActionResult{Ref: ref, Status: "skipped", Message: reason}
	}
	return ActionResult{Ref: ref, Status: "error", Message: err.Error()}
}

type timeoutErr struct{}

func (timeoutErr) Error() string { return "action timed out" }

var errShuttingDown = errors.New("action cancelled: server is shutting down")

// runActionWithTimeout runs one action under a context that ends at
// actionTimeout or when ctx (the engine's) ends at terminate. It returns when
// the action does or when that context ends, whichever is first.
//
// Cancelling is all it can do: an action that ignores its context keeps
// running after this returns, against ev.Record (and whatever else it
// touches) while the dispatch loop moves on with that same *core.Record. The
// record-op path and MarkEngineWrite refuse a write after the context ends,
// which closes that gap for every write that goes through them. The goroutine
// is counted in e.actions, so the terminate waits for it.
func (e *Engine) runActionWithTimeout(ctx context.Context, a storedAction, rule *core.Record, ev event, fields []DynamicField) error {
	if ctx.Err() != nil {
		return errShuttingDown
	}
	actionCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	done := make(chan error, 1)
	e.actions.Add(1)
	go func() {
		defer e.actions.Done()
		// A handler panic must surface as a failed action on this rule's run,
		// not as a dead process: this goroutine is separate from the worker's,
		// so the worker's recover cannot see it.
		defer func() {
			if r := recover(); r != nil {
				log.Error("action panicked",
					"action", a.Ref, "rule", rule.Id, "panic", r,
					"stack", string(debug.Stack()))
				done <- fmt.Errorf("action %s panicked: %v", a.Ref, r)
			}
		}()
		done <- ExecuteActionContext(actionCtx, e.app, e.defs, a.Ref, a.Params, rule, ev.Trigger, ev.Record, ev.Depth, fields)
	}()
	select {
	case err := <-done:
		return err
	case <-actionCtx.Done():
		if ctx.Err() != nil {
			return errShuttingDown
		}
		return timeoutErr{}
	}
}
