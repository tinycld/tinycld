package listeners

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
)

// Port is one port a package declares in its manifest's ports field,
// generated into server/ports.json for the supervisor to read.
type Port struct {
	Slug    string      `json:"slug"`
	Name    string      `json:"name"`
	Port    int         `json:"port"`
	AddrEnv string      `json:"addrEnv,omitempty"`
	Enabled *EnableRule `json:"enabled,omitempty"`
}

// EnableRule says whether a Port is on, based on one environment
// variable. Default true: on unless Env is exactly "false". Default
// false: on only if Env is exactly "true".
type EnableRule struct {
	Env     string `json:"env"`
	Default bool   `json:"default"`
}

// ReadPorts reads a generated ports.json. A missing file is not an error:
// it means the build was generated before this feature, or declares no
// ports, so the caller should treat it as "no ports" rather than fail.
func ReadPorts(path string) ([]Port, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listeners: read %s: %w", path, err)
	}
	var ports []Port
	if err := json.Unmarshal(data, &ports); err != nil {
		return nil, fmt.Errorf("listeners: parse %s: %w", path, err)
	}
	return ports, nil
}

// Resolve applies p's EnableRule (if any) and AddrEnv override against
// getenv, returning the address to bind and whether the port is on at
// all. A Port with no EnableRule is always on.
func (p Port) Resolve(getenv func(string) string) (addr string, on bool) {
	on = true
	if p.Enabled != nil {
		v := getenv(p.Enabled.Env)
		if p.Enabled.Default {
			on = v != "false"
		} else {
			on = v == "true"
		}
	}

	addr = net.JoinHostPort("", strconv.Itoa(p.Port))
	if p.AddrEnv != "" {
		if override := getenv(p.AddrEnv); override != "" {
			addr = override
		}
	}
	return addr, on
}
