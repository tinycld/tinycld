package coreserver

import (
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"

	"tinycld.org/core/webdav"
)

// Wiring core's generic hook-point registry to core/webdav's TS seam.
//
// This file is the only place the two know about each other, and the direction
// is one-way: coreserver imports webdav, never the reverse (webdav is a leaf;
// coreserver composes the app, so the other direction would be a cycle).

// hookPointAdapter adapts a *HookPoint to webdav.TSHookPoint. The shapes differ
// only in how the result is carried — HookResult here, a triple there — so this
// is a signature shim, not behaviour.
type hookPointAdapter struct {
	hp *HookPoint
}

func (a hookPointAdapter) Enabled() bool { return a.hp.Enabled() }

func (a hookPointAdapter) Call(payload map[string]any) (any, bool, error) {
	res, err := a.hp.Call(payload)
	if err != nil {
		return nil, false, err
	}
	return res.Value, res.Handled, nil
}

// WebDAVHostBindings returns the host surface core/webdav needs to register TS
// handlers against this app's hook registry.
func WebDAVHostBindings() webdav.HostBindings {
	return webdav.HostBindings{
		Point: func(name string) webdav.TSHookPoint {
			return hookPointAdapter{hp: RegisterHookPoint(name)}
		},
		AddHandler: func(name string, fn jsvm.Callable) {
			RegisterHookPoint(name).Add(fn)
		},
		RegisterLoaderBinder: func(b func(*sobek.Runtime, jsvm.Compiler, *pocketbase.PocketBase) error) {
			RegisterLoaderBinder(LoaderBinder(b))
		},
	}
}
