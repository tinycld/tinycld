package coreserver

import (
	"reflect"
	"testing"
)

func TestClassifySpec(t *testing.T) {
	cases := []struct {
		spec       string
		wantSource pkgSource
		wantKey    string
	}{
		{"@tinycld/mail", sourceNpm, "@tinycld/mail"},
		{"@tinycld/mail@1.2.3", sourceNpm, "@tinycld/mail"},
		{"mail", sourceNpm, "mail"},
		{"mail@latest", sourceNpm, "mail"},
		{"github:tinycld/todo", sourceGit, "github:tinycld/todo"},
		{"git+https://github.com/tinycld/todo.git", sourceGit, "git+https://github.com/tinycld/todo.git"},
		{"git+file:///workspace/base-remote.git", sourceGit, "git+file:///workspace/base-remote.git"},
		{"git+file:///workspace/base-remote.git#v0.0.5", sourceGit, "git+file:///workspace/base-remote.git"},
		{"", sourceUnknown, ""},
	}
	for _, c := range cases {
		gotSrc, gotKey := classifySpec(c.spec)
		if gotSrc != c.wantSource || gotKey != c.wantKey {
			t.Errorf("classifySpec(%q) = (%q, %q), want (%q, %q)",
				c.spec, gotSrc, gotKey, c.wantSource, c.wantKey)
		}
	}
}

func TestStripNpmVersion(t *testing.T) {
	cases := map[string]string{
		"@tinycld/mail@1.2.3": "@tinycld/mail",
		"@tinycld/mail":       "@tinycld/mail",
		"mail@1":              "mail",
		"mail":                "mail",
	}
	for in, want := range cases {
		if got := stripNpmVersion(in); got != want {
			t.Errorf("stripNpmVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortVersionsDesc(t *testing.T) {
	in := []string{"1.0.0", "v2.1.0", "1.2.0", "not-a-version", "0.9.0"}
	got := sortVersionsDesc(in)
	want := []string{"v2.1.0", "1.2.0", "1.0.0", "0.9.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortVersionsDesc = %v, want %v (newest first, junk dropped)", got, want)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"1.2.0", "1.0.0", true},
		{"1.0.0", "1.2.0", false},
		{"1.0.0", "1.0.0", false},
		{"v2.0.0", "1.9.9", true},
		{"garbage", "1.0.0", false},
		{"1.0.0", "garbage", false},
	}
	for _, c := range cases {
		if got := isNewer(c.candidate, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestGitRemoteURL(t *testing.T) {
	cases := map[string]string{
		"github:tinycld/todo":                 "https://github.com/tinycld/todo.git",
		"gitlab:org/repo":                     "https://gitlab.com/org/repo.git",
		"bitbucket:org/repo":                  "https://bitbucket.org/org/repo.git",
		"tinycld/todo":                        "https://github.com/tinycld/todo.git",
		"git+https://example.com/x.git":       "https://example.com/x.git",
		"https://github.com/tinycld/todo.git": "https://github.com/tinycld/todo.git",
	}
	for in, want := range cases {
		if got := gitRemoteURL(in); got != want {
			t.Errorf("gitRemoteURL(%q) = %q, want %q", in, got, want)
		}
	}
}
