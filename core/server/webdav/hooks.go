package webdav

import (
	"context"
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
)

// The opt-in TS hook points.
//
// Everything below is gated on TSHooks.<point>.Enabled() — one atomic load —
// before any payload is built or any VM is borrowed. An org that registers no
// handlers runs the pure-Go path and never touches the JS runtime pool. That is
// deliberate: the pool falls back to constructing a whole new sobek.Runtime
// once every executor is busy, which is exactly what a PROPFIND fan-out would
// trigger. Orgs that customize pay for it; orgs that don't pay nothing.
//
// Wiring is injected rather than imported: core/webdav does not depend on
// coreserver (which would be an import cycle, since coreserver composes the
// app). The host sets TSHooks on the FileSystem; see coreserver's registration.

// TSHookPoint is the subset of coreserver.HookPoint this package needs. Keeping
// it an interface here inverts the dependency — coreserver knows about webdav,
// not the reverse.
type TSHookPoint interface {
	Enabled() bool
	Call(payload map[string]any) (value any, handled bool, err error)
}

// TSHooks are the optional TS interception points, in the order a request
// encounters them. A nil point is simply skipped.
type TSHooks struct {
	// BeforeWrite fires on PUT and MKCOL before anything is created. A thrown
	// error rejects the operation.
	BeforeWrite TSHookPoint

	// BeforeDelete fires on DELETE. A thrown error rejects the operation.
	BeforeDelete TSHookPoint

	// BeforeMove fires on MOVE. A thrown error rejects the operation.
	BeforeMove TSHookPoint

	// CanRead is consulted per entry AFTER Go's own authorization has already
	// approved it. It can only narrow the result — returning true for an entry
	// Go denied has no effect, because Go's check runs first and short-circuits.
	CanRead TSHookPoint

	// FilterList fires once per directory listing with the whole batch of
	// already-authorized names, and may return a subset. Batching is what keeps
	// this affordable: one VM borrow per directory, not one per entry.
	FilterList TSHookPoint
}

// SetTSHooks installs the TS hook points on the FileSystem. Call before serving.
func (fs *FileSystem) SetTSHooks(h TSHooks) { fs.ts = h }

// hookBeforeWrite runs the BeforeWrite point. A handler error becomes a
// permission denial, which x/net/webdav surfaces as a 403 on MKCOL and 405 on
// PUT (the framework's own mapping, not something we can choose).
func (fs *FileSystem) hookBeforeWrite(ctx context.Context, user *core.Record, name, fullPath string, isCreate bool) error {
	hp := fs.ts.BeforeWrite
	if hp == nil || !hp.Enabled() {
		return nil
	}
	_, _, err := hp.Call(map[string]any{
		"slug":     fs.src.Slug,
		"op":       "write",
		"name":     name,
		"path":     fullPath,
		"userId":   user.Id,
		"isCreate": isCreate,
	})
	if err != nil {
		return rejected(err)
	}
	return nil
}

func (fs *FileSystem) hookBeforeDelete(ctx context.Context, user *core.Record, record *core.Record, fullPath string) error {
	hp := fs.ts.BeforeDelete
	if hp == nil || !hp.Enabled() {
		return nil
	}
	_, _, err := hp.Call(map[string]any{
		"slug":   fs.src.Slug,
		"op":     "delete",
		"id":     record.Id,
		"name":   record.GetString(fs.src.Fields.Name),
		"path":   fullPath,
		"userId": user.Id,
	})
	if err != nil {
		return rejected(err)
	}
	return nil
}

func (fs *FileSystem) hookBeforeMove(ctx context.Context, user *core.Record, record *core.Record, from, to string) error {
	hp := fs.ts.BeforeMove
	if hp == nil || !hp.Enabled() {
		return nil
	}
	_, _, err := hp.Call(map[string]any{
		"slug":   fs.src.Slug,
		"op":     "move",
		"id":     record.Id,
		"name":   record.GetString(fs.src.Fields.Name),
		"from":   from,
		"to":     to,
		"userId": user.Id,
	})
	if err != nil {
		return rejected(err)
	}
	return nil
}

// hookCanRead narrows an already-granted read. It is called ONLY for entries
// Go has already authorized, so it can restrict and never widen — a handler
// returning true for a denied entry is never even consulted.
//
// A handler error, or an explicit false, denies. Any other return (including
// undefined, from a handler that only inspects) leaves the Go decision intact.
func (fs *FileSystem) hookCanRead(userID, name, id string) bool {
	hp := fs.ts.CanRead
	if hp == nil || !hp.Enabled() {
		return true
	}
	v, handled, err := hp.Call(map[string]any{
		"slug":   fs.src.Slug,
		"id":     id,
		"name":   name,
		"userId": userID,
	})
	if err != nil {
		return false
	}
	if !handled {
		return true
	}
	if allowed, ok := v.(bool); ok {
		return allowed
	}
	return true
}

// hookFilterList offers the whole already-authorized batch to TS and accepts a
// returned subset. Entries the handler adds that were not in the input are
// discarded — the point can hide, never reveal.
func (fs *FileSystem) hookFilterList(ctx context.Context, userID string, infos []os.FileInfo) ([]os.FileInfo, error) {
	// Per-entry CanRead first, so a caller registering only CanRead still gets
	// it applied to listings.
	if hp := fs.ts.CanRead; hp != nil && hp.Enabled() {
		kept := infos[:0]
		for _, info := range infos {
			id := ""
			if rec, ok := info.Sys().(*core.Record); ok && rec != nil {
				id = rec.Id
			}
			if fs.hookCanRead(userID, info.Name(), id) {
				kept = append(kept, info)
			}
		}
		infos = kept
	}

	hp := fs.ts.FilterList
	if hp == nil || !hp.Enabled() {
		return infos, nil
	}

	names := make([]any, len(infos))
	for i, info := range infos {
		names[i] = info.Name()
	}

	v, handled, err := hp.Call(map[string]any{
		"slug":   fs.src.Slug,
		"userId": userID,
		"items":  names,
	})
	if err != nil {
		return nil, rejected(err)
	}
	if !handled || v == nil {
		return infos, nil
	}

	returned, ok := v.([]any)
	if !ok {
		// A handler that returned something other than a list is a bug in the
		// hook, not a reason to leak the unfiltered listing — but neither is it
		// a reason to fail the request. Keep the Go answer and say so.
		fs.app.Logger().Warn("webdav: filterList handler returned a non-list; ignoring",
			"slug", fs.src.Slug, "type", fmt.Sprintf("%T", v))
		return infos, nil
	}

	allowed := make(map[string]bool, len(returned))
	for _, n := range returned {
		if s, ok := n.(string); ok {
			allowed[s] = true
		}
	}

	// Intersect rather than trust: a name the handler invented was never
	// authorized by Go and must not appear.
	kept := make([]os.FileInfo, 0, len(infos))
	for _, info := range infos {
		if allowed[info.Name()] {
			kept = append(kept, info)
		}
	}
	return kept, nil
}

// rejected turns a TS veto into the filesystem error the DAV handler
// understands, preserving the handler's message for the Logger callback.
func rejected(err error) error {
	return &os.PathError{Op: "webdav", Err: fmt.Errorf("%w: %s", os.ErrPermission, err.Error())}
}
