package embedpolicy

import (
	"fmt"
	"net/url"
	"strings"
)

// MaxOrigins bounds a stored list: its only destination is a response header.
const MaxOrigins = 10

// ParseOrigins normalizes a space- or newline-separated list of origins that
// may frame a page, and returns the canonical space-separated form to store.
// An empty result means none.
//
// Narrow on purpose: scheme + host + optional port only. The value is written
// verbatim into a frame-ancestors directive and CSP has no quoting, so a stray
// space, semicolon or quote would change which directives the browser sees.
// No wildcards: one compromised subdomain must not become a framing grant.
func ParseOrigins(raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) > MaxOrigins {
		return "", fmt.Errorf("at most %d websites", MaxOrigins)
	}
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		origin, err := parseOrigin(f)
		if err != nil {
			return "", err
		}
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	return strings.Join(out, " "), nil
}

func parseOrigin(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		return "", fmt.Errorf("%q must include a scheme, e.g. https://%s", raw, raw)
	}
	if strings.ContainsAny(raw, " \t\r\n;,'\"") {
		return "", fmt.Errorf("%q contains an unsupported character", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid origin", raw)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("%q must use http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%q is missing a host", raw)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%q must be an origin only, with no path or credentials", raw)
	}
	if strings.Contains(u.Host, "*") {
		return "", fmt.Errorf("%q may not use a wildcard", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}
