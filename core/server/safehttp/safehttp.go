// Package safehttp makes outbound HTTP requests to caller-supplied URLs
// without becoming an SSRF vector.
//
// Lifted from calendar/server/subscription.go's ICS fetcher, which had these
// guards first. The logic is unchanged; what differs is that this package
// serves POSTs to user-authored webhook URLs, where two things matter more:
//
//   - REDIRECTS ARE NOT FOLLOWED. The ICS fetcher re-validates every hop,
//     which is right for a GET. A POST that follows a 302 re-sends its body
//     — and its signature header — to a host the caller never named.
//   - The body is a signed payload, so a redirect leaking it is a
//     confidentiality bug and not merely an access one.
package safehttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const dialTimeout = 10 * time.Second

// IsDisallowedIP reports whether an address is one we must never connect to
// on a caller's behalf: loopback, RFC 1918 private, IPv6 ULA (fc00::/7),
// link-local (IPv4 169.254.0.0/16 — including cloud metadata at
// 169.254.169.254 — and IPv6 fe80::/10), CGNAT (100.64.0.0/10), unspecified,
// and multicast.
func IsDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() {
		return true
	}
	// CGNAT: 100.64.0.0/10. Not covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

// ValidateURL rejects a URL we must not fetch: a non-HTTP scheme, a missing
// host, a host that will not resolve, or a host ANY of whose addresses is
// internal. Rejecting on any address (rather than all) means a name resolving
// to both a public and a private address cannot be used to reach the private
// one.
func ValidateURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("no URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("invalid host")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("cannot resolve host")
	}
	for _, ip := range ips {
		if IsDisallowedIP(ip) {
			return fmt.Errorf("private/internal hosts are not allowed")
		}
	}
	return nil
}

// PinnedTransport returns a transport whose dialer re-resolves the hostname,
// verifies every returned address is public, and connects to a verified IP
// itself. Verifying and dialing the SAME resolution closes the DNS-rebinding
// window a standalone pre-check leaves open. TLS still handshakes against the
// original hostname, so certificate verification is unaffected.
func PinnedTransport() *http.Transport {
	return &http.Transport{
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: dialTimeout,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("cannot resolve host")
			}
			for _, ip := range ips {
				if IsDisallowedIP(ip) {
					return nil, fmt.Errorf("private/internal hosts are not allowed")
				}
			}
			dialer := &net.Dialer{Timeout: dialTimeout}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

// PostJSON sends one JSON POST to a validated public URL and returns the
// response status. The body is read and discarded up to a small ceiling: a
// webhook's response is not data we consume, but draining lets the connection
// close cleanly.
//
// Redirects are refused rather than followed — see the package comment.
func PostJSON(
	ctx context.Context,
	rawURL string,
	body []byte,
	headers map[string]string,
	timeout time.Duration,
) (int, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("invalid URL: %w", err)
	}
	if err := ValidateURL(parsed); err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TinyCld/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: PinnedTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("redirects are not followed")
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}
