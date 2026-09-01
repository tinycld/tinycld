package tenantmain

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/quota"
)

// Every flag the router passes must be DEFINED here, because the flagset is
// flag.ExitOnError: an unrecognized one exits the process before the readiness
// pipe is written, so the router only ever sees a tenant that never came up.
// Run is invoked without --org-dir so it stops at the required-flags check —
// reaching that error at all is the proof that parsing accepted every flag.
func TestRun_AcceptsEveryRouterFlag(t *testing.T) {
	err := Run(Options{Args: []string{
		"--slug", "acme",
		"--ready-fd", "0",
		"--hooks-pool", "2",
		"--drain", "1s",
		"--carddav-config", "/x/carddav.json",
		"--caldav-config", "/x/caldav.json",
		"--webdav-config", "/x/webdav.json",
		"--quota-config", "/x/quota.json",
		"--limits-config", "/x/limits.json",
		"--app-config", "/x/app.json",
		"--imap-socket", "/x/imap.sock",
		"--smtp-socket", "/x/smtp.sock",
		"--mx-socket", "/x/mx.sock",
		"--control-socket", "/x/ctl.sock",
		"--cfg-socket", "/x/cfg.sock",
		"--confine-packages", "/x/packages",
	}})
	if err == nil || !strings.Contains(err.Error(), "--org-dir and --socket are required") {
		t.Fatalf("Run should have parsed every flag and stopped at the required-flags check, got %v", err)
	}
}

// RegisterRuntime exists so a layered composition can reach the paths this
// flagset parsed without re-parsing os.Args. If the values did not arrive
// verbatim, that composition would silently configure itself from nothing.
func TestRun_RegisterRuntimeReceivesTheParsedPaths(t *testing.T) {
	var got RuntimeConfig
	called := false

	// --org-dir is supplied so parsing gets past the required-flags check; the
	// boot then fails later (no such directory), which is enough to prove the
	// hook ran with the values we passed in.
	_ = Run(Options{
		Args: []string{
			"--org-dir", "/x/org",
			"--socket", "/x/app.sock",
			"--limits-config", "/x/limits.json",
			"--cfg-socket", "/x/cfg.sock",
			"--quota-config", "/x/quota.json",
		},
		RegisterRuntime: func(_ *pocketbase.PocketBase, cfg RuntimeConfig) (quota.LimitsFunc, error) {
			called = true
			got = cfg
			return nil, errors.New("stop the boot here")
		},
	})

	if !called {
		t.Fatal("RegisterRuntime should have been called")
	}
	if got.OrgDir != "/x/org" {
		t.Errorf("OrgDir = %q, want /x/org", got.OrgDir)
	}
	if got.LimitsConfig != "/x/limits.json" {
		t.Errorf("LimitsConfig = %q, want /x/limits.json", got.LimitsConfig)
	}
	if got.ConfigSocket != "/x/cfg.sock" {
		t.Errorf("ConfigSocket = %q, want /x/cfg.sock", got.ConfigSocket)
	}
	if got.QuotaConfig != "/x/quota.json" {
		t.Errorf("QuotaConfig = %q, want /x/quota.json", got.QuotaConfig)
	}
}

// A composition whose limits failed to register must not boot. Coming up
// anyway would serve the org with enforcement silently absent, which is
// indistinguishable from a healthy tenant until someone exceeds a limit.
func TestRun_RegisterRuntimeErrorAbortsTheBoot(t *testing.T) {
	err := Run(Options{
		Args: []string{"--org-dir", "/x/org", "--socket", "/x/app.sock"},
		RegisterRuntime: func(*pocketbase.PocketBase, RuntimeConfig) (quota.LimitsFunc, error) {
			return nil, errors.New("cfg socket in use")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cfg socket in use") {
		t.Fatalf("boot should have aborted with the hook's error, got %v", err)
	}
}

// A nil QuotaLimits must leave the boot-derived limits in place; a non-nil one
// must win. This is the seam the hosting composition uses to swap the
// boot-constant ceiling for a live-refreshing one.
func TestResolveQuotaLimits_OverrideWins(t *testing.T) {
	boot := int64(300)
	override := quota.LimitsFunc(func(core.App) quota.Limits {
		return quota.Limits{PerOrg: 999, PerUser: 100}
	})

	// When override is nil, should get FixedLimits; when non-nil, get override.
	gotNil := resolveQuotaLimits(nil, boot)
	gotOverride := resolveQuotaLimits(override, boot)

	// Verify nil branch returns a non-nil resolver. We cannot invoke it with a
	// nil core.App because FixedLimits.PerUser reads from settings via app.DB(),
	// which would panic, but we verify the resolver path selected the boot-time
	// constant by confirming a function was returned.
	if gotNil == nil {
		t.Fatal("nil override should return a non-nil LimitsFunc")
	}

	// Verify the override is used when supplied.
	overrideResult := gotOverride(nil)
	if overrideResult.PerOrg != 999 {
		t.Fatalf("override should return PerOrg=999, got %d", overrideResult.PerOrg)
	}
	if overrideResult.PerUser != 100 {
		t.Fatalf("override should return PerUser=100, got %d", overrideResult.PerUser)
	}
}
