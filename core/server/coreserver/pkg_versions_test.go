package coreserver

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
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

// ---------- concurrent discovery fan-out ----------

// registryRows builds n unsaved pkg_registry records, each with a distinct slug
// and spec, for driving versionInfosForRows without touching the network.
func registryRows(t *testing.T, n int) []*core.Record {
	t.Helper()
	app := newRegistryOnlyApp(t)
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatalf("pkg_registry collection not found: %v", err)
	}
	rows := make([]*core.Record, n)
	for i := range rows {
		r := core.NewRecord(col)
		r.Set("slug", fmt.Sprintf("pkg%02d", i))
		r.Set("npm_package", fmt.Sprintf("@tinycld/pkg%02d", i))
		r.Set("version", "1.0.0")
		rows[i] = r
	}
	return rows
}

// The fan-out writes results by index, so the caller's ordering survives even
// when discovery finishes in the opposite order. Without index-keyed writes
// (e.g. appending as each goroutine completes) the rows would come back
// shuffled and every version picker would attach to the wrong package.
func TestVersionInfosForRows_PreservesOrderDespiteCompletionOrder(t *testing.T) {
	rows := registryRows(t, 8)

	// Invert the delay by index: the LAST row resolves first.
	discover := func(spec string) (pkgSource, []string, string) {
		var idx int
		if _, err := fmt.Sscanf(spec, "@tinycld/pkg%d", &idx); err != nil {
			t.Errorf("unexpected spec %q", spec)
		}
		time.Sleep(time.Duration(len(rows)-idx) * time.Millisecond)
		return sourceNpm, []string{"2.0.0"}, ""
	}

	infos := versionInfosForRows(rows, discover)

	if len(infos) != len(rows) {
		t.Fatalf("got %d infos, want %d", len(infos), len(rows))
	}
	for i, info := range infos {
		want := fmt.Sprintf("pkg%02d", i)
		if info.Slug != want {
			t.Errorf("infos[%d].Slug = %q, want %q — fan-out lost registry ordering",
				i, info.Slug, want)
		}
		if info.Latest != "2.0.0" || !info.HasUpdate {
			t.Errorf("infos[%d] = %+v, want Latest 2.0.0 with HasUpdate", i, info)
		}
	}
}

// Discovery shells out to git/npm per row, so the fan-out must stay bounded —
// a large registry must not fork one subprocess per row simultaneously.
func TestVersionInfosForRows_RespectsFanoutLimit(t *testing.T) {
	rows := registryRows(t, versionsFanoutLimit*3)

	var inFlight, peak int64
	var mu sync.Mutex
	discover := func(string) (pkgSource, []string, string) {
		cur := atomic.AddInt64(&inFlight, 1)
		mu.Lock()
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		// Hold the slot long enough that the limiter, not scheduling luck, is
		// what caps concurrency.
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return sourceNpm, []string{"1.0.0"}, ""
	}

	versionInfosForRows(rows, discover)

	mu.Lock()
	got := peak
	mu.Unlock()
	if got > versionsFanoutLimit {
		t.Errorf("peak concurrency %d exceeded versionsFanoutLimit %d", got, versionsFanoutLimit)
	}
	// Guard against the fan-out silently degrading to serial: with 24 rows and a
	// limit of 8 we expect real overlap.
	if got < 2 {
		t.Errorf("peak concurrency %d — discovery ran serially, not fanned out", got)
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
