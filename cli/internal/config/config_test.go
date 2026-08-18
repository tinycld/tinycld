package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingFileIsEmptyConfig(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Current != "" || len(cfg.Contexts) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Current: "localhost:7110",
		Contexts: map[string]Context{
			"localhost:7110":   {Origin: "http://localhost:7110", User: "dev@example.com"},
			"acme.tinycld.org": {Origin: "https://acme.tinycld.org"},
		},
	}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != cfg.Current {
		t.Fatalf("current = %q, want %q", got.Current, cfg.Current)
	}
	if len(got.Contexts) != 2 {
		t.Fatalf("contexts = %+v", got.Contexts)
	}
	if got.Contexts["localhost:7110"].User != "dev@example.com" {
		t.Fatalf("user lost: %+v", got.Contexts["localhost:7110"])
	}
	if got.Contexts["acme.tinycld.org"].Origin != "https://acme.tinycld.org" {
		t.Fatalf("origin lost: %+v", got.Contexts["acme.tinycld.org"])
	}
}

func TestSaveFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	dir := t.TempDir()
	cfg := &Config{Contexts: map[string]Context{"a": {Origin: "https://a"}}}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config.toml mode = %o, want 600", perm)
	}
}

func TestResolvePrecedence(t *testing.T) {
	cfg := &Config{
		Current: "b",
		Contexts: map[string]Context{
			"a": {Origin: "https://a"},
			"b": {Origin: "https://b"},
		},
	}

	name, ctx, err := cfg.Resolve("a")
	if err != nil || name != "a" || ctx.Origin != "https://a" {
		t.Fatalf("flag resolve: %q %+v %v", name, ctx, err)
	}

	name, ctx, err = cfg.Resolve("")
	if err != nil || name != "b" || ctx.Origin != "https://b" {
		t.Fatalf("current resolve: %q %+v %v", name, ctx, err)
	}

	if _, _, err := cfg.Resolve("missing"); err == nil {
		t.Fatal("expected error for unknown context")
	}

	empty := &Config{Contexts: map[string]Context{}}
	if _, _, err := empty.Resolve(""); err == nil {
		t.Fatal("expected error for no context")
	}
}

func TestNormalizeOrigin(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "acme.tinycld.org", want: "https://acme.tinycld.org"},
		{in: "acme.tinycld.org/", want: "https://acme.tinycld.org"},
		{in: "localhost:7110", want: "http://localhost:7110"},
		{in: "127.0.0.1:7110", want: "http://127.0.0.1:7110"},
		{in: "http://acme.tinycld.org", want: "http://acme.tinycld.org"},
		{in: "https://acme.tinycld.org/path", want: "https://acme.tinycld.org"},
		{in: "", wantErr: true},
		{in: "ftp://acme.tinycld.org", wantErr: true},
	}
	for _, c := range cases {
		got, err := NormalizeOrigin(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeOrigin(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeOrigin(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeOrigin(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContextName(t *testing.T) {
	if got := ContextName("http://localhost:7110"); got != "localhost:7110" {
		t.Fatalf("ContextName = %q", got)
	}
	if got := ContextName("https://acme.tinycld.org"); got != "acme.tinycld.org" {
		t.Fatalf("ContextName = %q", got)
	}
}

func TestDirEnvOverride(t *testing.T) {
	t.Setenv("TINYCLD_CONFIG_DIR", "/tmp/custom")
	if got := Dir(); got != "/tmp/custom" {
		t.Fatalf("Dir() = %q", got)
	}
}

// A plain-http origin to a non-loopback host sends the bearer token in
// cleartext. NormalizeOrigin only picks a scheme for a BARE host, so an
// explicit http:// URL reaches the config verbatim — that is what this flags.
func TestIsInsecureOrigin(t *testing.T) {
	insecure := []string{
		"http://mail.example.com",
		"http://mail.example.com:8080",
		"http://192.168.1.10:7110",
	}
	for _, o := range insecure {
		if !IsInsecureOrigin(o) {
			t.Errorf("IsInsecureOrigin(%q) = false, want true", o)
		}
	}

	secure := []string{
		"https://mail.example.com",
		"http://localhost:7110",
		"http://127.0.0.1:7110",
		"http://[::1]:7110",
		"https://localhost",
	}
	for _, o := range secure {
		if IsInsecureOrigin(o) {
			t.Errorf("IsInsecureOrigin(%q) = true, want false", o)
		}
	}
}
