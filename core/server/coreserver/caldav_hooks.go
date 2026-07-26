package coreserver

import (
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"

	"tinycld.org/core/caldav"
)

// Wiring core's generic hook-point registry to core/caldav's TS seam.
//
// The same one-way shape as webdav_hooks.go: coreserver imports caldav, never
// the reverse (caldav is a leaf; coreserver composes the app, so the other
// direction would be a cycle). hookPointAdapter is shared with the WebDAV
// wiring — the two protocols' TSHookPoint interfaces are structurally identical
// — but each protocol keeps its own HostBindings type so their hook-name spaces
// stay separate.

// caldavHookPointAdapter adapts a *HookPoint to caldav.TSHookPoint. Separate
// from hookPointAdapter only because Go interfaces are nominal at the point of
// use: the method set is the same.
type caldavHookPointAdapter struct {
	hp *HookPoint
}

func (a caldavHookPointAdapter) Enabled() bool { return a.hp.Enabled() }

func (a caldavHookPointAdapter) Call(payload map[string]any) (any, bool, error) {
	res, err := a.hp.Call(payload)
	if err != nil {
		return nil, false, err
	}
	return res.Value, res.Handled, nil
}

// CalDAVHostBindings returns the host surface core/caldav needs to register TS
// handlers against this app's hook registry.
func CalDAVHostBindings() caldav.HostBindings {
	return caldav.HostBindings{
		Point: func(name string) caldav.TSHookPoint {
			return caldavHookPointAdapter{hp: RegisterHookPoint(name)}
		},
		AddHandler: func(name string, fn jsvm.Callable) {
			RegisterHookPoint(name).Add(fn)
		},
		RegisterLoaderBinder: func(b func(*sobek.Runtime, jsvm.Compiler, *pocketbase.PocketBase) error) {
			RegisterLoaderBinder(LoaderBinder(b))
		},
	}
}
