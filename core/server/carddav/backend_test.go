package carddav

import "testing"

func TestExtractOrgSlug(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/carddav/u/ab/acme/", "acme"},
		{"/carddav/u/ab/acme/urn:uuid:x.vcf", "acme"},
		{"/carddav/u/ab/", ""},
		{"/carddav/", ""},
	}
	for _, tc := range cases {
		if got := extractOrgSlug(tc.path); got != tc.want {
			t.Errorf("extractOrgSlug(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestExtractVCardUID(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/carddav/u/ab/acme/urn:uuid:abc.vcf", "urn:uuid:abc"},
		{"/carddav/u/ab/acme/", ""},
		{"/carddav/u/ab/acme/plain.vcf", "plain"},
	}
	for _, tc := range cases {
		if got := extractVCardUID(tc.path); got != tc.want {
			t.Errorf("extractVCardUID(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestExtractOrgSlugFromHost(t *testing.T) {
	cases := []struct{ host, want string }{
		{"acme.localhost:8100", "acme"},
		{"acme.localhost", "acme"},
		{"acme.tinycld.com", "acme"},
		{"tinycld.com", ""},    // two labels — no subdomain
		{"127.0.0.1", ""},      // IPv4
		{"127.0.0.1:8090", ""}, // IPv4 with port
		{"localhost", ""},      // bare host
	}
	for _, tc := range cases {
		if got := extractOrgSlugFromHost(tc.host); got != tc.want {
			t.Errorf("extractOrgSlugFromHost(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/carddav", true},
		{"/carddav/", true},
		{"/carddav/u/ab/default/", true},
		{"/.well-known/carddav", true},
		{"/api/health", false},
		{"/", false},
		{"/carddavx", false},
	}
	for _, tc := range cases {
		if got := HasPrefix(tc.path); got != tc.want {
			t.Errorf("HasPrefix(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
