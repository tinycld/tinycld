package listeners

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortResolve(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	secure := Port{Name: "acme-secure", Port: 993, AddrEnv: "ACME_SECURE_ADDR", Enabled: &EnableRule{Env: "ACME_SECURE_ENABLED", Default: true}}
	inbound := Port{Name: "acme-inbound", Port: 25, AddrEnv: "ACME_INBOUND_ADDR", Enabled: &EnableRule{Env: "ACME_INBOUND_ENABLED", Default: false}}

	if addr, on := secure.Resolve(get); !on || addr != ":993" {
		t.Fatalf("secure default = %q %v", addr, on)
	}
	if _, on := inbound.Resolve(get); on {
		t.Fatal("inbound on by default")
	}
	env["ACME_SECURE_ENABLED"] = "false"
	env["ACME_INBOUND_ENABLED"] = "true"
	env["ACME_INBOUND_ADDR"] = "0.0.0.0:2525"
	if _, on := secure.Resolve(get); on {
		t.Fatal("secure on with ACME_SECURE_ENABLED=false")
	}
	if addr, on := inbound.Resolve(get); !on || addr != "0.0.0.0:2525" {
		t.Fatalf("inbound = %q %v", addr, on)
	}
	plain := Port{Name: "x", Port: 7000}
	if addr, on := plain.Resolve(get); !on || addr != ":7000" {
		t.Fatalf("no rule = %q %v", addr, on)
	}
}

func TestReadPorts(t *testing.T) {
	dir := t.TempDir()
	if ps, err := ReadPorts(filepath.Join(dir, "missing.json")); err != nil || ps != nil {
		t.Fatalf("missing file = %v %v", ps, err)
	}
	p := filepath.Join(dir, "ports.json")
	os.WriteFile(p, []byte(`[{"slug":"acme","name":"acme-secure","port":993,"addrEnv":"ACME_SECURE_ADDR","enabled":{"env":"ACME_SECURE_ENABLED","default":true}}]`), 0o644)
	ps, err := ReadPorts(p)
	if err != nil || len(ps) != 1 || ps[0].Name != "acme-secure" || ps[0].Enabled == nil || !ps[0].Enabled.Default {
		t.Fatalf("ports = %+v %v", ps, err)
	}
}
