package coreserver

import (
	"errors"
	"testing"
)

func infos(rows ...[3]string) []VersionInfo {
	out := make([]VersionInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, VersionInfo{Slug: r[0], Current: r[1], Latest: r[2], Available: []string{r[2]}, HasUpdate: r[1] != r[2]})
	}
	return out
}

func TestNewestTargetsSkipsPrereleases(t *testing.T) {
	in := []VersionInfo{
		// Latest and HasUpdate point at a prerelease; the stable 0.6.0 is the target.
		{Slug: "mail", Current: "0.5.0", Latest: "1.0.0-rc.1", HasUpdate: true,
			Available: []string{"1.0.0-rc.1", "0.6.0", "0.5.0"}},
		// Only prereleases are newer than current: not targeted.
		{Slug: "drive", Current: "0.3.0", Latest: "0.4.0-beta.2", HasUpdate: true,
			Available: []string{"0.4.0-beta.2", "0.4.0-beta.1", "0.3.0"}},
		// A stable older than current is not a target either.
		{Slug: "calc", Current: "0.2.0", Latest: "0.3.0-rc.1", HasUpdate: true,
			Available: []string{"0.3.0-rc.1", "0.1.0"}},
	}
	got := newestTargets(in)
	if len(got) != 1 || got["mail"] != "0.6.0" {
		t.Fatalf("got %v", got)
	}
}

func TestPlanUpgradeTakesNewestIncludingMajors(t *testing.T) {
	in := infos([3]string{"mail", "0.5.0", "1.0.0"}, [3]string{"core", "0.5.4", "0.5.4"})
	p, err := planUpgrade(in, func(map[string]string) ([]compatViolation, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Target) != 1 || p.Target["mail"] != "1.0.0" || p.DroppedMajors {
		t.Fatalf("got %+v", p)
	}
}

func TestPlanUpgradeDropsMajorsOnConflict(t *testing.T) {
	in := infos([3]string{"mail", "0.5.0", "1.0.0"}, [3]string{"drive", "0.3.0", "0.3.2"})
	solve := func(c map[string]string) ([]compatViolation, error) {
		if c["mail"] == "1.0.0" {
			return []compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6", Found: "0.5.4"}}, nil
		}
		return nil, nil
	}
	p, err := planUpgrade(in, solve)
	if err != nil {
		t.Fatal(err)
	}
	if !p.DroppedMajors || p.Target["drive"] != "0.3.2" || p.Target["mail"] != "" {
		t.Fatalf("got %+v", p)
	}
}

func TestPlanUpgradePausesWhenNothingResolves(t *testing.T) {
	in := infos([3]string{"mail", "0.5.0", "0.5.1"})
	v := []compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6", Found: "0.5.4"}}
	p, err := planUpgrade(in, func(map[string]string) ([]compatViolation, error) { return v, nil })
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != nil || len(p.Violations) != 1 || p.Wanted["mail"] != "0.5.1" {
		t.Fatalf("got %+v", p)
	}
}

func TestPlanUpgradeNoUpdates(t *testing.T) {
	p, err := planUpgrade(infos([3]string{"mail", "0.5.0", "0.5.0"}), nil)
	if err != nil || p.Target != nil || p.Violations != nil {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestPlanUpgradeSolveError(t *testing.T) {
	in := infos([3]string{"mail", "0.5.0", "0.5.1"})
	if _, err := planUpgrade(in, func(map[string]string) ([]compatViolation, error) { return nil, errors.New("db") }); err == nil {
		t.Fatal("want error")
	}
}

func TestFingerprintIsStableAndOrderFree(t *testing.T) {
	a := fingerprint(map[string]string{"mail": "1.0.0", "core": "0.6.0"}, nil)
	b := fingerprint(map[string]string{"core": "0.6.0", "mail": "1.0.0"}, nil)
	c := fingerprint(map[string]string{"core": "0.6.0", "mail": "1.0.1"}, nil)
	if a != b || a == c || len(a) != 16 {
		t.Fatalf("a=%s b=%s c=%s", a, b, c)
	}
	v := []compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6", Found: "0.5.4"}}
	if fingerprint(map[string]string{"mail": "1.0.0"}, v) == fingerprint(map[string]string{"mail": "1.0.0"}, nil) {
		t.Fatal("violations must change the fingerprint")
	}
}

func TestFormatViolations(t *testing.T) {
	got := formatViolations([]compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6 <0.7", Found: "0.5.4"}})
	want := "mail needs @tinycld/core >=0.6 <0.7 (found 0.5.4)"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestFormatTarget(t *testing.T) {
	if got := formatTarget(map[string]string{"mail": "0.6.0", "core": "0.5.4"}); got != "core 0.5.4, mail 0.6.0" {
		t.Fatalf("got %q", got)
	}
}
