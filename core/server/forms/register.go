package forms

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("forms")

type Options struct {
	// HostsPath is the generator's server/form_hosts.json.
	HostsPath string
	// HostsJSON is the embedded fallback for a single-binary build; used when HostsPath is absent.
	HostsJSON []byte
}

// Register reads the host registry and binds the forms hooks. With no hosts
// the hooks stay unbound (composition_parity_test). A hosts file that exists
// but cannot be read is a broken build, not a lean shell: Register panics
// rather than start with every form write unchecked.
//
// The read rules are composed on serve whatever the hosts: a package that
// registered a public-reader clause gets it applied, and with none the rule
// stays the signed-in text the migration wrote.
func Register(app *pocketbase.PocketBase, opts Options) {
	defs, err := resolveHosts(opts)
	if err != nil {
		panic(fmt.Sprintf("forms: cannot start with an unreadable host registry (%s): %v", opts.HostsPath, err))
	}
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := ApplyPublicReaders(se.App); err != nil {
			log.Error("compose the forms read rules failed", "err", err)
		}
		return se.Next()
	})
	if len(defs.Hosts) == 0 {
		return
	}
	BindHooks(app, defs)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := SeedDefaults(se.App, defs); err != nil {
			log.Error("seed default forms failed", "err", err)
		}
		return se.Next()
	})
}

// resolveHosts reads the generator's hosts file, falling back to the
// embedded copy when the file is absent or empty. A missing file with no
// embedded copy is an empty registry; a file or copy that does not parse is
// an error.
func resolveHosts(opts Options) (*Defs, error) {
	defs, err := LoadHosts(opts.HostsPath)
	if err != nil {
		return nil, err
	}
	if len(defs.Hosts) == 0 && len(opts.HostsJSON) > 0 {
		return ParseHosts(opts.HostsJSON)
	}
	return defs, nil
}

// BindHooks binds the forms and host-record hooks for defs. Register calls it,
// and a package test calls it to give a test app the hooks a served install has.
func BindHooks(app core.App, defs *Defs) {
	if len(defs.Hosts) == 0 {
		return
	}
	bindFormsHooks(app, defs)
	bindHostHooks(app, defs)
	defsKey.Set(app, defs)
	registerFilesProviders(defs)
}
