package automation

import "github.com/pocketbase/pocketbase/core"

// RunObserver sees each run the engine logs for a trigger, after the run row
// is written. Web forms use it to tell admins a destination failed.
type RunObserver func(app core.App, rule, record *core.Record, outcome RunOutcome)

func RegisterRunObserver(triggerRef string, o RunObserver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	runObservers[triggerRef] = append(runObservers[triggerRef], o)
}

// Failed reports a run in which the rule errored or any action failed.
func (o RunOutcome) Failed() bool {
	if o.Err != "" {
		return true
	}
	for _, r := range o.Results {
		if r.Status == "error" {
			return true
		}
	}
	return false
}

func notifyRunObservers(app core.App, triggerRef string, rule, record *core.Record, outcome RunOutcome) {
	registryMu.RLock()
	obs := append([]RunObserver(nil), runObservers[triggerRef]...)
	registryMu.RUnlock()
	for _, o := range obs {
		o(app, rule, record, outcome)
	}
}
