package pkgbuild

import "testing"

func TestValidatePackageSpec(t *testing.T) {
	cases := []struct {
		spec    string
		wantErr bool
	}{
		// bare npm names (existing behavior preserved)
		{"mail", false},
		{"@tinycld/mail", false},
		{"@tinycld/google-takeout-import", false},
		// git specs npm pack understands natively
		{"github:tinycld/todo", false},
		{"gitlab:acme/widget", false},
		{"bitbucket:acme/widget", false},
		{"tinycld/todo", false},
		{"https://github.com/tinycld/todo", false},
		{"https://github.com/tinycld/todo.git", false},
		{"git+https://github.com/tinycld/todo.git", false},
		{"git+ssh://git@github.com/tinycld/todo.git", false},
		// git specs pinned to a tag/ref via #ref (npm pack clones at the ref)
		{"github:tinycld/todo#v1.0.0", false},
		{"github:tinycld/todo#2.0.0", false},
		{"tinycld/todo#v1.0.0", false},
		{"https://github.com/tinycld/todo.git#v1.0.0", false},
		{"git+https://github.com/tinycld/todo.git#1.2.3-beta.1", false},
		// rejected #ref forms: only git specs may pin, ref must be a safe token
		{"mail#v1.0.0", true},          // npm name can't carry a #ref
		{"@tinycld/mail#v1.0.0", true}, // scoped npm name can't carry a #ref
		{"github:tinycld/todo#-flag", true},
		{"github:tinycld/todo#", true}, // empty ref
		{"github:tinycld/todo#v1.0.0#extra", true},
		// versioned npm specs (npm pack name@version)
		{"mail@1.2.3", false},
		{"@tinycld/mail@1.2.3", false},
		{"mail@latest", false},
		// tightened bare owner/repo shorthand — no path traversal
		{"../etc", true},
		{"..%2f/etc", true},
		// rejected: arg injection / shell metacharacters / empty
		{"", true},
		{"-rf", true},
		{"--registry=evil", true},
		{"; rm -rf /", true},
		{"$(whoami)", true},
		{"foo bar", true},
		{"foo`id`", true},
		{"foo|bar", true},
		{"foo\nbar", true},
	}
	for _, tc := range cases {
		err := ValidatePackageSpec(tc.spec)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidatePackageSpec(%q): got err=%v, wantErr=%v", tc.spec, err, tc.wantErr)
		}
	}
}

func TestValidateManifest(t *testing.T) {
	base := func() *ParsedManifest {
		return &ParsedManifest{Name: "Cal Slots", Slug: "calendar-slots", Version: "0.1.0"}
	}
	cases := []struct {
		name        string
		m           *ParsedManifest
		allowServer bool
		bundled     map[string]bool
		wantErr     bool
	}{
		// A slot-only contributor (no nav, no routes) is valid — it contributes
		// purely via sidebarContributions. This is the regression that the old
		// unconditional `nav` requirement wrongly rejected.
		{"slot-only no nav no routes", base(), false, nil, false},
		// Full-featured package with nav + routes.
		{
			"nav and routes",
			&ParsedManifest{
				Name: "Cal", Slug: "calendar", Version: "1.0.0",
				Routes: &ManifestRoutes{Directory: "screens"},
				Nav:    &ManifestNav{Label: "Calendar", Icon: "calendar"},
			},
			false, nil, false,
		},
		// Identity fields are still required.
		{"missing name", &ParsedManifest{Slug: "x", Version: "1.0.0"}, false, nil, true},
		{"missing slug", &ParsedManifest{Name: "X", Version: "1.0.0"}, false, nil, true},
		{"missing version", &ParsedManifest{Name: "X", Slug: "x"}, false, nil, true},
		// Slug shape is enforced (feeds path construction).
		{"bad slug", &ParsedManifest{Name: "X", Slug: "Bad_Slug", Version: "1.0.0"}, false, nil, true},
		// Path traversal in an optional routes dir is rejected.
		{
			"routes traversal",
			&ParsedManifest{Name: "X", Slug: "x", Version: "1.0.0", Routes: &ManifestRoutes{Directory: "../etc"}},
			false, nil, true,
		},
		// Env gate: a server package can't install where the Go toolchain is absent.
		{
			"server rejected without toolchain",
			&ParsedManifest{Name: "X", Slug: "x", Version: "1.0.0", HasServer: true, Server: &ManifestServer{Package: "server", Module: "tinycld.org/x"}},
			false, nil, true,
		},
		{
			"server allowed with toolchain",
			&ParsedManifest{Name: "X", Slug: "x", Version: "1.0.0", HasServer: true, Server: &ManifestServer{Package: "server", Module: "tinycld.org/x"}},
			true, nil, false,
		},
		// Env gate: slug collision with a bundled package.
		{"bundled slug collision", base(), false, map[string]bool{"calendar-slots": true}, true},
	}
	for _, tc := range cases {
		err := ValidateManifest(tc.m, tc.allowServer, tc.bundled)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: ValidateManifest got err=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestIsTrustedScope(t *testing.T) {
	trusted := []string{"@tinycld/mail", "@tinycld/todo"}
	untrusted := []string{
		"mail",
		"github:tinycld/todo",
		"https://github.com/tinycld/todo",
		"@acme/widget",
	}
	for _, s := range trusted {
		if !IsTrustedScope(s) {
			t.Errorf("IsTrustedScope(%q) = false, want true", s)
		}
	}
	for _, s := range untrusted {
		if IsTrustedScope(s) {
			t.Errorf("IsTrustedScope(%q) = true, want false", s)
		}
	}
}
