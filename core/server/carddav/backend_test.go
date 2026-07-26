package carddav

import "testing"

func TestExtractVCardUID(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/carddav/u/ab/default/urn:uuid:abc.vcf", "urn:uuid:abc"},
		{"/carddav/u/ab/default/", ""},
		{"/carddav/u/ab/default/plain.vcf", "plain"},
	}
	for _, tc := range cases {
		if got := extractVCardUID(tc.path); got != tc.want {
			t.Errorf("extractVCardUID(%q) = %q, want %q", tc.path, got, tc.want)
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
