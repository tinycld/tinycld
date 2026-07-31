package tenantcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"tinycld.org/core/caldav"
	"tinycld.org/core/carddav"
	"tinycld.org/core/webdav"
)

// The .runtime/*.json file shapes. The router writes them (orgmanager); the
// tenant loads them at boot (tenantmain). Every loader treats an empty path
// and a missing file identically: the router manages nothing here, leave the
// tenant's own state alone.

// QuotaConfig is .runtime/quota.json: the org's storage ceiling and the
// storage-bearing collections it is computed over. It comes from the
// control-plane record rather than the org's own settings because each tenant
// has its own superusers: a limit stored inside the org could be raised by
// the org.
type QuotaConfig struct {
	StorageLimitBytes int64         `json:"storageLimitBytes"`
	Sources           []QuotaSource `json:"sources"`
}

// PackagesConfig is .runtime/packages.json: the org's resolved package slugs.
type PackagesConfig struct {
	Slugs []string `json:"slugs"`
}

// AppConfig is .runtime/app.json.
type AppConfig struct {
	// AppURL is adopted as Settings().Meta.AppURL — the value PB interpolates
	// into {APP_URL} in every auth email — because the tenant knows neither
	// the base domain nor the TLS mode.
	AppURL string `json:"appURL"`

	// OrgName is the org's display name from the control-plane record,
	// adopted as the tenant's Settings().Meta.AppName (the client reads it
	// back via /api/org-info). Empty = leave the tenant's stored name alone.
	OrgName string `json:"orgName"`

	// BaseDomain lets the tenant scope the cross-org switcher cookie to the
	// parent domain (Domain=.<baseDomain>). Empty = no cross-org cookie.
	BaseDomain string `json:"baseDomain"`

	// TrustedProxyHeaders is what the tenant should set as
	// Settings().TrustedProxy.Headers. The router guarantees the RIGHTMOST
	// entry of the (last) header is the best-known client IP — PocketBase's
	// default resolution order (UseLeftmostIP false).
	TrustedProxyHeaders []string `json:"trustedProxyHeaders"`
}

// WriteRuntimeFile writes body to <orgDir>/.runtime/<name> as a root-authored
// tenant config, hardened against a hostile tenant.
//
// chownTree hands the whole org tree — including .runtime and its files — to
// the tenant uid on every spawn, so a running tenant can replace .runtime (or
// a file inside it) with a symlink. The next router write, running as ROOT,
// would then follow that link onto an arbitrary host path: /etc/shadow, a
// sibling org's data.db, or the org's own app.json (whose AppURL becomes its
// auth-email hostname). Two guards close this:
//   - the .runtime directory is refused if it is a symlink (Lstat), so the
//     path is resolved beneath the org dir, not through a redirect;
//   - the file open uses O_NOFOLLOW, so a symlink planted AT the destination
//     fails the open rather than being written through.
//
// One ABI definition, used by the router's writers (orgmanager, the deployer)
// so the hardening cannot drift between them.
func WriteRuntimeFile(orgDir, name string, body []byte, mode os.FileMode) (string, error) {
	runtimeDir := filepath.Join(orgDir, ".runtime")
	if fi, err := os.Lstat(runtimeDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to write %s: .runtime is a symlink", name)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(runtimeDir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// loadJSON reads path into out. Empty path or missing file leaves out at its
// zero value and reports no error.
func loadJSON(path string, out any) error {
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(body, out)
}

// LoadQuota reads .runtime/quota.json.
func LoadQuota(path string) (QuotaConfig, error) {
	var cfg QuotaConfig
	err := loadJSON(path, &cfg)
	return cfg, err
}

// LoadPackageSlugs reads .runtime/packages.json.
func LoadPackageSlugs(path string) ([]string, error) {
	var cfg PackagesConfig
	err := loadJSON(path, &cfg)
	return cfg.Slugs, err
}

// LoadAppConfig reads .runtime/app.json.
func LoadAppConfig(path string) (AppConfig, error) {
	var cfg AppConfig
	err := loadJSON(path, &cfg)
	return cfg, err
}

// LoadCardDAV reads a materialized carddav.json into core Sources.
func LoadCardDAV(path string) ([]carddav.Source, error) {
	var wire []Source
	if err := loadJSON(path, &wire); err != nil {
		return nil, err
	}
	return Decode(wire), nil
}

// LoadWebDAV reads a materialized webdav.json into core Sources.
func LoadWebDAV(path string) ([]webdav.Source, error) {
	var wire []WebDAVSource
	if err := loadJSON(path, &wire); err != nil {
		return nil, err
	}
	return DecodeWebDAV(wire), nil
}

// LoadCalDAV reads a materialized caldav.json into core Sources.
func LoadCalDAV(path string) ([]caldav.Source, error) {
	var wire []CalDAVSource
	if err := loadJSON(path, &wire); err != nil {
		return nil, err
	}
	return DecodeCalDAV(wire), nil
}
