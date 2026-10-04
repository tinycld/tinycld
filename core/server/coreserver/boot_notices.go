package coreserver

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// bootMailFn emails a notice to every administrator. ctx ends the sends.
type bootMailFn func(ctx context.Context, app core.App, n notice) error

// bootNoticeStopBound is how long a terminating app waits for the boot
// notices after it cancels them. The cancel ends every send; what is left is
// a few database writes. It must be short: the supervisor gives a stopping
// process 10 s in all, and the other terminate waits run in the same chain
// (the sum is set out at supervise's options.stopBound).
const bootNoticeStopBound = 2 * time.Second

// startBootNotices tells administrators what the last boot found (a backup a
// rollback could not restore, an automatic upgrade that was rolled back) in a
// goroutine. A send can wait on a mail server or a push service for many
// seconds, and the supervisor waits only 60 s for ready, which is sent after
// the OnServe chain: a send in the chain could turn a good boot into a
// failed one.
//
// The goroutine writes to the database, so the app must not close it while
// the goroutine runs. The OnTerminate handler bound here cancels the work
// and waits for it before the database closes. It is bound at serve, with
// the work it waits for, and only in the composition that serves itself.
func startBootNotices(app core.App, mail bootMailFn) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		cancel()
		select {
		case <-done:
		case <-time.After(bootNoticeStopBound):
			srvLog.Warn("closing the app under boot notices that did not stop in time", "bound", bootNoticeStopBound)
		}
		return e.Next()
	})
	go func() {
		defer close(done)
		reportUnrestored(ctx, app, mail)
		reconcileAutoUpgradeResults(app, time.Now(), func(n notice) {
			// send logs each failure; a blocked-upgrade notice is not retried.
			_ = mail(ctx, app, n)
		})
	}()
}

// mailAdmins is the production bootMailFn.
func mailAdmins(ctx context.Context, app core.App, n notice) error {
	return notifyAdmins(app, func(name, email, subj, html, text string) error {
		return sendContext(ctx, app, name, email, subj, html, text)
	}, n)
}
