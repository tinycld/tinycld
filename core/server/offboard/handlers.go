package offboard

import (
	"sort"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Handler settles ownership that a flat (collection, field) rewrite cannot
// express. RegisterReassignable rewrites every matching row to the successor
// in one UPDATE, which is wrong for membership-style tables: the successor
// would inherit every role the leaver held (viewer on other people's things
// included), and a unique (resource, user) index makes the UPDATE fail when
// the successor is already a member. A Handler gets the whole picture and
// decides row by row.
//
// OffboardUser calls every registered Handler inside its transaction, after
// the reassignable refs are settled and before the users record is
// anonymized, in every mode (ModeReassign, ModeDeleteMyData and ModeKeep):
//
//   - txApp is the transaction app. Use it for every read and write, so a
//     failure rolls back the whole offboard.
//   - leaver is the users record being offboarded, not yet anonymized.
//   - plan is the validated plan. In ModeReassign, plan.SuccessorUserID names
//     an existing users record other than the leaver. In the other modes
//     there is no successor.
//   - actorUserID is the user who started the offboard: an admin on
//     /api/admin/users/offboard, the leaver on /api/account/delete, or "" for
//     system use. actorUserID == leaver.Id therefore means a self-delete, with
//     nobody else to hand ownership to.
//
// A Handler must not assume a heir exists. With no successor and no actor
// other than the leaver (a self-delete in ModeDeleteMyData or ModeKeep), a
// resource the leaver solely owns and other people use has nobody to go to:
// refuse with a wrapped ErrInvalidPlan that tells the user to transfer
// ownership or delete the resource first.
//
// A non-nil error aborts the offboard and rolls back every write, including
// the other handlers' writes. Wrap ErrInvalidPlan to reject the plan for this
// user's data; the endpoints return that as a 400 with the message, so the
// caller can show what to do instead.
type Handler func(txApp core.App, leaver *core.Record, plan Plan, actorUserID string) error

type namedHandler struct {
	name    string
	handler Handler
}

var (
	handlersMu sync.RWMutex
	handlers   []namedHandler
)

// RegisterHandler adds an offboard Handler under a unique name, conventionally
// the package slug. Idempotent by name: a second registration under the same
// name is a no-op and the first Handler stays, so re-running a package's
// Register() on a dev reload does not run its work twice. Handlers run in name
// order, so the order does not depend on package load order.
func RegisterHandler(name string, h Handler) {
	if name == "" || h == nil {
		return
	}
	handlersMu.Lock()
	defer handlersMu.Unlock()
	for _, existing := range handlers {
		if existing.name == name {
			return
		}
	}
	handlers = append(handlers, namedHandler{name: name, handler: h})
	sort.Slice(handlers, func(i, j int) bool { return handlers[i].name < handlers[j].name })
}

// registeredHandlers returns a snapshot, so a handler that registers another
// handler (it should not) cannot deadlock the offboard.
func registeredHandlers() []namedHandler {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	out := make([]namedHandler, len(handlers))
	copy(out, handlers)
	return out
}

// ResetHandlersForTesting clears the handler registry.
func ResetHandlersForTesting() {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers = nil
}
