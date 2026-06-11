package coreserver

import (
	"path/filepath"
	"testing"
)

func TestResolveStateDir_EnvOverride(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", "/var/lib/tinycld")
	if got := resolveStateDir(); got != "/var/lib/tinycld" {
		t.Fatalf("resolveStateDir() = %q, want /var/lib/tinycld", got)
	}
}

func TestResolveStateDir_DefaultsToServerDir(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", "")
	// With no override, state dir must equal the server dir so existing
	// deployments (state under appDir) keep working until the mounts move.
	if got, want := resolveStateDir(), resolveServerDir(); got != want {
		t.Fatalf("resolveStateDir() = %q, want %q (resolveServerDir)", got, want)
	}
}

func TestStateDataDir_JoinsPbData(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", "/state")
	if got, want := statePbDataDir(), filepath.Join("/state", "pb_data"); got != want {
		t.Fatalf("statePbDataDir() = %q, want %q", got, want)
	}
}
