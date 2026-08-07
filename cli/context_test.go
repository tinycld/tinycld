package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"tinycld.org/cli/internal/config"
	"tinycld.org/cli/internal/keychain"
)

// runCLI executes the root command against a temp config dir and memory
// keychain, returning stdout, stderr, and the command error.
func runCLI(t *testing.T, d *deps, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	d.stdout = &stdout
	d.stderr = &stderr
	root := newRootCmd(d)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func testDeps(t *testing.T) (*deps, *keychain.MemStore) {
	t.Helper()
	store := keychain.NewMemStore()
	d := &deps{
		configDir:  t.TempDir(),
		httpClient: &http.Client{},
		isTTY:      false,
		openStore:  func(string, io.Writer) keychain.Store { return store },
		sleep:      func(time.Duration) {},
	}
	return d, store
}

func TestContextAddUseListRemove(t *testing.T) {
	d, store := testDeps(t)

	if _, _, err := runCLI(t, d, "context", "add", "dev", "localhost:7110"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, d, "context", "add", "prod", "acme.tinycld.org"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(d.configDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Contexts["dev"].Origin != "http://localhost:7110" {
		t.Fatalf("dev origin = %q", cfg.Contexts["dev"].Origin)
	}
	if cfg.Contexts["prod"].Origin != "https://acme.tinycld.org" {
		t.Fatalf("prod origin = %q", cfg.Contexts["prod"].Origin)
	}
	// first added context becomes current
	if cfg.Current != "dev" {
		t.Fatalf("current = %q", cfg.Current)
	}

	if _, _, err := runCLI(t, d, "context", "use", "prod"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(d.configDir)
	if cfg.Current != "prod" {
		t.Fatalf("current = %q", cfg.Current)
	}

	stdout, _, err := runCLI(t, d, "context", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "dev") || !strings.Contains(stdout, "prod") {
		t.Fatalf("list = %q", stdout)
	}

	stdout, _, err = runCLI(t, d, "context", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []contextRow
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("list --json parse: %v\n%s", err, stdout)
	}
	if len(listed) != 2 {
		t.Fatalf("listed = %+v", listed)
	}
	for _, row := range listed {
		if row.Current != (row.Name == "prod") {
			t.Fatalf("current flag wrong: %+v", row)
		}
	}

	// removing the current context clears current and its credential
	store.Set("prod", "token")
	if _, _, err := runCLI(t, d, "context", "remove", "prod"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(d.configDir)
	if cfg.Current != "" {
		t.Fatalf("current = %q after remove", cfg.Current)
	}
	if _, err := store.Get("prod"); err == nil {
		t.Fatal("credential should be deleted with the context")
	}
}

func TestContextAddDuplicateFails(t *testing.T) {
	d, _ := testDeps(t)
	if _, _, err := runCLI(t, d, "context", "add", "dev", "localhost:7110"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, d, "context", "add", "dev", "localhost:9999"); err == nil {
		t.Fatal("duplicate add should fail")
	}
}

func TestContextUseUnknownFails(t *testing.T) {
	d, _ := testDeps(t)
	if _, _, err := runCLI(t, d, "context", "use", "nope"); err == nil {
		t.Fatal("unknown context should fail")
	}
}

func TestQuietSuppressesInfo(t *testing.T) {
	d, _ := testDeps(t)
	stdout, _, err := runCLI(t, d, "context", "add", "dev", "localhost:7110", "--quiet")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Fatalf("quiet stdout = %q", stdout)
	}
}

func TestVersionJSON(t *testing.T) {
	d, _ := testDeps(t)
	stdout, _, err := runCLI(t, d, "version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version string `json:"version"`
		Go      string `json:"go"`
	}
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("version --json parse: %v\n%s", err, stdout)
	}
	if v.Version != "dev" || v.Go == "" {
		t.Fatalf("version = %+v", v)
	}
}
