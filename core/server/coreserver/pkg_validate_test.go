package coreserver

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
		err := validatePackageSpec(tc.spec)
		if (err != nil) != tc.wantErr {
			t.Errorf("validatePackageSpec(%q): got err=%v, wantErr=%v", tc.spec, err, tc.wantErr)
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
		if !isTrustedScope(s) {
			t.Errorf("isTrustedScope(%q) = false, want true", s)
		}
	}
	for _, s := range untrusted {
		if isTrustedScope(s) {
			t.Errorf("isTrustedScope(%q) = true, want false", s)
		}
	}
}
