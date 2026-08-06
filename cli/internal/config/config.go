// Package config manages the CLI's saved contexts (servers) in
// ~/.config/tinycld/config.toml. Tokens are NOT stored here — they live in
// the OS keychain (internal/keychain), keyed by context name.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

const fileName = "config.toml"

type Context struct {
	Origin string `toml:"origin"`
	User   string `toml:"user,omitempty"`
}

type Config struct {
	Current  string             `toml:"current,omitempty"`
	Contexts map[string]Context `toml:"contexts"`
}

// Dir resolves the config directory. TINYCLD_CONFIG_DIR wins (tests,
// unusual setups); Windows uses the platform config dir; elsewhere XDG
// applies with ~/.config as the fallback.
func Dir() string {
	if v := os.Getenv("TINYCLD_CONFIG_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "tinycld")
		}
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "tinycld")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".tinycld")
	}
	return filepath.Join(home, ".config", "tinycld")
}

// Load reads dir/config.toml. A missing file is an empty config, not an
// error, so first-run commands work without setup.
func Load(dir string) (*Config, error) {
	cfg := &Config{Contexts: map[string]Context{}}
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", fileName, err)
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]Context{}
	}
	return cfg, nil
}

// Save writes atomically (temp + rename) with owner-only permissions.
func (c *Config) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, fileName+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(buf.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, fileName))
}

// Resolve picks the context to operate on: the --context flag if given,
// otherwise the saved current context.
func (c *Config) Resolve(flag string) (string, Context, error) {
	name := flag
	if name == "" {
		name = c.Current
	}
	if name == "" {
		return "", Context{}, errors.New("no context selected — run `tinycld auth login <host>` first")
	}
	ctx, ok := c.Contexts[name]
	if !ok {
		return "", Context{}, fmt.Errorf("unknown context %q — run `tinycld context list`", name)
	}
	return name, ctx, nil
}

// NormalizeOrigin turns user input like "acme.tinycld.org" or
// "localhost:7110" into a canonical origin URL. Loopback hosts default to
// http, everything else to https.
func NormalizeOrigin(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", errors.New("empty host")
	}
	if !strings.Contains(s, "://") {
		host := s
		if i := strings.IndexAny(host, ":/"); i >= 0 {
			host = host[:i]
		}
		if isLoopback(host) {
			s = "http://" + s
		} else {
			s = "https://" + s
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid host %q: %w", input, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid host %q: unsupported scheme %q", input, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid host %q", input)
	}
	return u.Scheme + "://" + u.Host, nil
}

// ContextName is the default context name for an origin: its host[:port].
func ContextName(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return origin
	}
	return u.Host
}

func isLoopback(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}
