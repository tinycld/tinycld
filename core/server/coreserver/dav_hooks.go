package coreserver

import (
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"

	"tinycld.org/core/davhooks"
)

// Wiring core's generic hook-point registry to the DAV protocols' shared TS
// seam (core/davhooks).
//
// This file is the only place the two know about each other, and the direction
// is one-way: coreserver imports davhooks, never the reverse (davhooks is a
// leaf; coreserver composes the app, so the other direction would be a cycle).

// hookPointAdapter adapts a *HookPoint to davhooks.TSHookPoint. The shapes
// differ only in how the result is carried — HookResult here, a triple there —
// so this is a signature shim, not behaviour.
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

// davHostBindings returns the host surface the DAV protocol packages need to
// register TS handlers against this app's hook registry. webdav and caldav
// alias the same davhooks.HostBindings type, so one builder serves both; the
// two named constructors below are what feature packages call.
func davHostBindings() davhooks.HostBindings {
	return davhooks.HostBindings{
		Point: func(name string) davhooks.TSHookPoint {
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

// WebDAVHostBindings returns the host surface core/webdav needs to register TS
// handlers against this app's hook registry.
func WebDAVHostBindings() davhooks.HostBindings { return davHostBindings() }

// CalDAVHostBindings returns the host surface core/caldav needs to register TS
// handlers against this app's hook registry.
func CalDAVHostBindings() davhooks.HostBindings { return davHostBindings() }
