package safehttp

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIsDisallowedIP_RejectsInternalRanges(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1",       // loopback
		"10.0.0.5",        // RFC 1918
		"172.16.0.1",      // RFC 1918
		"192.168.1.1",     // RFC 1918
		"169.254.169.254", // link-local — cloud metadata
		"100.64.0.1",      // CGNAT
		"::1",             // IPv6 loopback
		"fc00::1",         // IPv6 ULA
		"fe80::1",         // IPv6 link-local
		"0.0.0.0",         // unspecified
	} {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("test bug: %q is not an IP", raw)
		}
		if !IsDisallowedIP(ip) {
			t.Errorf("IsDisallowedIP(%s) = false, want true", raw)
		}
	}
}

func TestIsDisallowedIP_AllowsPublic(t *testing.T) {
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		ip := net.ParseIP(raw)
		if IsDisallowedIP(ip) {
			t.Errorf("IsDisallowedIP(%s) = true, want false", raw)
		}
	}
}

func TestValidateURL_RejectsNonHTTPSchemes(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("test bug: %v", err)
		}
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%s) = nil, want an error", raw)
		}
	}
}

func TestValidateURL_RejectsInternalHost(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:8090/hook")
	if err := ValidateURL(u); err == nil {
		t.Error("ValidateURL(loopback) = nil, want an error")
	}
}

func TestValidateURL_RejectsBlankHost(t *testing.T) {
	u, _ := url.Parse("http:///nohost")
	if err := ValidateURL(u); err == nil {
		t.Error("ValidateURL(blank host) = nil, want an error")
	}
}

func TestPostJSON_RejectsInternalTargetBeforeDialing(t *testing.T) {
	_, err := PostJSON(context.Background(), "http://169.254.169.254/", []byte("{}"), nil, time.Second)
	if err == nil {
		t.Fatal("expected an SSRF rejection")
	}
	if !strings.Contains(err.Error(), "private/internal") {
		t.Errorf("error = %v, want a private/internal rejection", err)
	}
}
