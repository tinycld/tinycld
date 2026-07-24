package coreserver

import (
	"sync"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
)

// The core→TS binding seam.
//
// The multi-org PocketBase fork calls jsvm.Config.OnInit on every JS VM (both
// the hook loader and the executor/callback pools). We use it to install a
// small, stable set of native `$`-bindings that package `.pb.ts` hooks can call
// — the ONE auditable surface untrusted tenant TS is allowed to reach. Keeping
// this surface minimal is a deliberate security goal (see the multi-org
// HANDOFF): powerful Go capabilities (raw SQL, protocol codecs, filesystem)
// stay host-side and are exposed only through these enumerable bindings, never
// as arbitrary feature Go with full $app reach.
//
// Data-plane work that fires on record events (FTS sync, thumbnails, audit) is
// deliberately NOT a binding — core registers those as its own config-driven Go
// record hooks so they never cross the VM boundary and a tenant can't skip them.
// Bindings are reserved for logic that must ORIGINATE in package TS.

// JSVMBinder installs one binding namespace onto a freshly-initialized VM.
// A core sub-package (fts, carddav, …) registers a binder via RegisterJSVMBinder
// so it can expose native funcs to package TS without coreserver importing it
// (mirrors the SetShareSessionResolver decoupling pattern). The binder should
// build its namespace object with NewBindNamespace and vm.Set it under a single
// `$name` global. It is called once per VM; keep it cheap and side-effect free
// beyond the vm.Set.
type JSVMBinder func(vm *sobek.Runtime, app *pocketbase.PocketBase) error

var (
	bindersMu sync.RWMutex
	binders   []JSVMBinder
)

// RegisterJSVMBinder adds a binder invoked on every VM via OnInit. Call it from
// a sub-package's init or its Register(app) before the first VM spins up
// (coreserver.Register wires OnInit before app.Start, so registration during
// Register is in time). Safe for concurrent registration.
func RegisterJSVMBinder(b JSVMBinder) {
	bindersMu.Lock()
	defer bindersMu.Unlock()
	binders = append(binders, b)
}

// buildJsvmOnInit returns the jsvm.Config.OnInit callback. It runs every
// registered binder against the VM. A binder error is fatal for that VM — we
// panic, matching jsvm's own MustRegister failure mode, because a VM missing a
// core binding would surface as opaque "undefined is not a function" errors in
// hook code rather than a clear boot failure.
func buildJsvmOnInit(app *pocketbase.PocketBase) func(vm *sobek.Runtime) {
	return func(vm *sobek.Runtime) {
		bindersMu.RLock()
		defer bindersMu.RUnlock()
		for _, b := range binders {
			if err := b(vm, app); err != nil {
				panic("coreserver: JS binding install failed: " + err.Error())
			}
		}
	}
}

// NewBindNamespace builds a plain JS object whose keys are the given native Go
// funcs, ready to vm.Set under a `$name` global. Values are typically
// func(...) results; sobek marshals Go funcs into callable JS functions.
//
//	obj, _ := NewBindNamespace(vm, map[string]any{
//	    "search": func(q string) ([]any, error) { ... },
//	})
//	vm.Set("$fts", obj)
func NewBindNamespace(vm *sobek.Runtime, members map[string]any) (*sobek.Object, error) {
	obj := vm.NewObject()
	for name, fn := range members {
		if err := obj.Set(name, fn); err != nil {
			return nil, err
		}
	}
	return obj, nil
}
