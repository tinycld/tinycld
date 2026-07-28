package webdav

import (
	"tinycld.org/core/davhooks"
)

// Registering the TS hook points.
//
// The five points a package can hook are declared here, named per source slug
// so two packages serving trees never share a handler. The JS-facing binding is
// a single `webdavHook({...})` call that takes an object of handlers, which
// keeps the surface one auditable function rather than five globals.
//
// The wiring is deliberately indirect. core/webdav must not import coreserver
// (coreserver composes the whole app, so that would be a cycle), and coreserver
// must not know each protocol's hook names. So the host injects the small
// davhooks.HostBindings surface and the shared machinery in core/davhooks does
// the rest — including the validate-before-compile order that stops a typo'd
// hook name from partially registering.

// hookPointNames are the JS keys a handler object may carry, in the order the
// request path encounters them.
var hookPointNames = []string{
	"beforeWrite",
	"beforeDelete",
	"beforeMove",
	"canRead",
	"filterList",
}

// HostBindings is the minimal host surface this package needs in order to
// register TS handlers, shared with the other DAV protocols. coreserver
// supplies it (WebDAVHostBindings).
type HostBindings = davhooks.HostBindings

// RegisterTSHooks installs the `webdavHook` JS binding and returns the TSHooks
// wired to this source's points, ready to hand to FileSystem.SetTSHooks.
//
// The returned TSHooks reference the points whether or not any handler is ever
// registered; each point reports Enabled() == false until one is, which is what
// keeps the fast path free.
func RegisterTSHooks(host HostBindings, src Source) TSHooks {
	point := func(hook string) TSHookPoint {
		return host.Point(davhooks.PointName("webdav", src.Slug, hook))
	}
	hooks := TSHooks{
		BeforeWrite:  point("beforeWrite"),
		BeforeDelete: point("beforeDelete"),
		BeforeMove:   point("beforeMove"),
		CanRead:      point("canRead"),
		FilterList:   point("filterList"),
	}

	davhooks.RegisterBinding(host, "webdavHook", "webdav", src.Slug, hookPointNames)

	return hooks
}
