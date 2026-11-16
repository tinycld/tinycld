package core

// OnBeforeCommit registers fn to run inside the transaction, after the
// transaction's function returns without an error and before the commit.
// txApp is the transaction's app. An error from fn rolls the transaction
// back, and no later callback runs.
//
// It is for a write that must follow many others exactly once (a total kept
// on a parent row): a hook that runs for each of N writes registers the
// callback on the first one, and the parent is written one time, not N.
//
// Callbacks run in the order they were registered. A callback may register
// another one; that one runs too. A nested RunInTransaction joins its outer
// transaction, so its callbacks run before the outer commit.
func (a *TxAppInfo) OnBeforeCommit(fn func(txApp App) error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.beforeCommitFuncs = append(a.beforeCommitFuncs, fn)
}

// runBeforeCommitFuncs runs the callbacks, without the lock held, so a
// callback can register another one.
func (a *TxAppInfo) runBeforeCommitFuncs(txApp App) error {
	for {
		a.mu.Lock()
		if len(a.beforeCommitFuncs) == 0 {
			a.mu.Unlock()
			return nil
		}
		fn := a.beforeCommitFuncs[0]
		a.beforeCommitFuncs = a.beforeCommitFuncs[1:]
		a.mu.Unlock()

		if err := fn(txApp); err != nil {
			return err
		}
	}
}

// runTxFunc runs a transaction's function and then its OnBeforeCommit
// callbacks.
func runTxFunc(txApp *BaseApp, fn func(txApp App) error) error {
	if err := fn(txApp); err != nil {
		return err
	}
	return txApp.txInfo.runBeforeCommitFuncs(txApp)
}
