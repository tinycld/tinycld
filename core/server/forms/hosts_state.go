package forms

import (
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/appstate"
)

var defsKey = appstate.NewKey[*Defs]("forms.defs")

// HostFor returns the host bound for app, or nil. A caller that writes host
// records relies on the host hooks; nil means they are not bound.
func HostFor(app core.App, key string) *Host {
	defs, ok := defsKey.GetOk(app)
	if !ok || defs == nil {
		return nil
	}
	return defs.Host(key)
}
