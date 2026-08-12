// tinycld/core/server/automation/engine.go
package automation

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const (
	maxChainDepth    = 3
	actionTimeout    = 30 * time.Second
	dispatchQueueLen = 1024
	// shutdownDrainWait bounds how long OnTerminate waits for an in-flight
	// dispatch to finish. Long enough for the common case — a dispatch mid-
	// flight finishes its current DB call — but a hung action must not hold
	// up process shutdown, mirroring the abandon-don't-kill design of the
	// per-action timeout below.
	shutdownDrainWait = 5 * time.Second
)

type event struct {
	TriggerRef string
	Trigger    TriggerDef
	Record     *core.Record
	Depth      int
	SourceRule string
}

type Engine struct {
	app     core.App
	defs    *Defs
	queue   chan event
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
}

func NewEngine(app core.App, defs *Defs) *Engine {
	return &Engine{app: app, defs: defs, queue: make(chan event, dispatchQueueLen), done: make(chan struct{})}
}

// Start binds one hook per distinct (collection, op) named by the defs, plus
// the rules-change hooks for schedule reconciliation, and launches the worker.
// Rule execution never runs on the hook goroutine: a slow rule must not block
// an SMTP delivery.
func (e *Engine) Start() {
	if e.started {
		e.app.Logger().Warn("automation: Start called more than once, ignoring")
		return
	}
	e.started = true

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
		cancel()
		// Wait for the worker to actually exit before letting teardown
		// proceed: cancelling only stops it from picking up the NEXT event —
		// a dispatch already in flight is still touching the DB, and a test
		// app's Cleanup() tears the DB down right after this hook returns.
		// Bounded rather than unconditional: a dispatch can run N rules × M
		// actions × a 30s per-action timeout plus prune I/O, and a hung
		// action must not hold up process shutdown — same abandon-don't-kill
		// tradeoff as the per-action timeout itself.
		select {
		case <-e.done:
		case <-time.After(shutdownDrainWait):
			e.app.Logger().Warn("automation: worker did not drain before terminate; abandoning in-flight dispatch")
		}
		return te.Next()
	})
	go e.worker(ctx)

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
		takeEngineWrite(ev.Record.Id)
		e.reloadScheduleFor(ev.Record)
		return ev.Next()
	}
	e.app.OnRecordAfterCreateSuccess("rules").BindFunc(reload)
	e.app.OnRecordAfterUpdateSuccess("rules").BindFunc(reload)
	e.app.OnRecordAfterDeleteSuccess("rules").BindFunc(reload)
}

// reloadScheduleFor is completed in the schedule task; the hook binding above
// must exist from the start so no rules write is missed between tasks.
func (e *Engine) reloadScheduleFor(rule *core.Record) {}

func (e *Engine) recordHookHandler(col, op string) func(*core.RecordEvent) error {
	return func(ev *core.RecordEvent) error {
		depth := 0
		source := ""
		if w, ok := takeEngineWrite(ev.Record.Id); ok {
			depth = w.Depth + 1
			source = w.RuleID
		}
		if depth > maxChainDepth {
			e.app.Logger().Warn("automation: chain depth exceeded", "collection", col, "record", ev.Record.Id, "sourceRule", source)
			if source != "" {
				if rule, err := e.app.FindRecordById("rules", source); err == nil {
					WriteRun(e.app, rule, ev.Record, TriggerDef{}, RunOutcome{Err: "chain-depth-exceeded"})
				}
			}
			return ev.Next()
		}
		for _, qt := range e.defs.TriggersFor(col, op) {
			if op == "update" && !WatchChanged(ev.Record, qt.Def.Watch) {
				continue
			}
			e.enqueue(event{TriggerRef: qt.Ref, Trigger: qt.Def, Record: ev.Record, Depth: depth, SourceRule: source})
		}
		return ev.Next()
	}
}

func (e *Engine) enqueue(ev event) {
	select {
	case e.queue <- ev:
	default:
		// A dropped dispatch is recoverable (the data is in the DB); a blocked
		// write path is not. Log loudly and move on.
		e.app.Logger().Error("automation: dispatch queue full, dropping event", "trigger", ev.TriggerRef)
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
			e.dispatch(ev)
		}
	}
}

// DispatchForTest runs one event synchronously.
func (e *Engine) DispatchForTest(ev event) { e.dispatch(ev) }

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

func (e *Engine) dispatch(ev event) {
	rules, err := e.app.FindRecordsByFilter(
		"rules", "trigger = {:ref} && enabled = true", "order", 0, 0,
		map[string]any{"ref": ev.TriggerRef},
	)
	if err != nil || len(rules) == 0 {
		return
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

	stopped := false
	for _, rule := range rules {
		if stopped {
			break
		}
		if rule.Id == ev.SourceRule {
			continue // a rule never re-fires on its own write
		}
		if rule.GetString("scope") == "personal" && !ownerSet[rule.GetString("owner")] {
			continue
		}
		start := time.Now()

		ast, err := DecodeConditions(rule.Get("conditions"))
		if err != nil {
			WriteRun(e.app, rule, ev.Record, ev.Trigger, RunOutcome{Err: "invalid conditions: " + err.Error(), Duration: time.Since(start)})
			continue
		}
		if !EvaluateConditions(ast, ev.Record) {
			WriteRun(e.app, rule, ev.Record, ev.Trigger, RunOutcome{Matched: false, Duration: time.Since(start)})
			continue
		}

		actions, err := decodeActions(rule.Get("actions"))
		if err != nil {
			WriteRun(e.app, rule, ev.Record, ev.Trigger, RunOutcome{Matched: true, Err: "invalid actions: " + err.Error(), Duration: time.Since(start)})
			continue
		}
		results := make([]ActionResult, 0, len(actions))
		failed := 0
		for _, a := range actions {
			res := ActionResult{Ref: a.Ref, Status: "ok"}
			if err := e.runActionWithTimeout(a, rule, ev); err != nil {
				res.Status = "error"
				res.Message = err.Error()
				failed++
			}
			results = append(results, res)
		}
		WriteRun(e.app, rule, ev.Record, ev.Trigger, RunOutcome{Matched: true, Results: results, Duration: time.Since(start)})
		recordRunResult(e.app, rule, len(actions) > 0 && failed == len(actions))

		if rule.GetBool("stop_processing") {
			if rule.GetString("scope") == "org" {
				stopped = true // org stop halts everything downstream
			} else {
				stopped = true // personal stop halts later personal rules — org already ran (org-first ordering)
			}
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string { return "action timed out" }

// runActionWithTimeout abandons (does not kill) an overrunning action: the PB
// SDK has no context-cancellable Save, so the goroutine finishes on its own
// while the run is recorded as timed out.
func (e *Engine) runActionWithTimeout(a storedAction, rule *core.Record, ev event) error {
	done := make(chan error, 1)
	go func() {
		done <- ExecuteAction(e.app, e.defs, a.Ref, a.Params, rule, ev.Trigger, ev.Record, ev.Depth)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(actionTimeout):
		return timeoutErr{}
	}
}
