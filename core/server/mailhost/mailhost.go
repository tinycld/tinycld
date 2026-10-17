// Package mailhost names the host mail apps connect to for IMAP and SMTP.
//
// By default it is the web host: a deployment that sets nothing serves mail on
// the same name as the web app. A deployment can name another host with the
// Key system setting. This package does not know who sets it.
package mailhost

import (
	"net"
	"net/url"
	"strings"

	"tinycld.org/core/syscfg"
)

// Key is the system setting that names the host mail apps connect to.
const Key = "mail.client_host"

// Configured returns the host the deployment set for mail apps, or "" when it
// set none or the value is not a host name.
func Configured() string {
	return normalize(syscfg.Get(Key))
}

// Host returns the host mail apps connect to: the configured host, else the
// host name in fallback (a URL or a host[:port]), else "".
func Host(fallback string) string {
	if host := Configured(); host != "" {
		return host
	}
	return normalize(fallback)
}

// normalize turns a URL, a host:port or a bare host into a lower-case host
// name. A value that is not a host name gives "", so a typo in the setting
// falls back to the web host instead of sending mail apps to a bad name.
func normalize(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil {
			return ""
		}
		v = u.Host
	}
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	}
	v = strings.TrimSuffix(strings.ToLower(v), ".")
	if !isHostname(v) {
		return ""
	}
	return v
}

func isHostname(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
