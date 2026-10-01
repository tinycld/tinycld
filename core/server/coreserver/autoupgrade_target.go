package coreserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type solveFunc func(changes map[string]string) ([]compatViolation, error)

// upgradePlan is one scheduler decision. Target nil + Violations set means the
// newest set cannot resolve even without majors: the run pauses.
type upgradePlan struct {
	Target        map[string]string
	Wanted        map[string]string
	Violations    []compatViolation
	DroppedMajors bool
}

func newestTargets(infos []VersionInfo) map[string]string {
	out := map[string]string{}
	for _, in := range infos {
		if in.HasUpdate && in.Latest != "" {
			out[in.Slug] = in.Latest
		}
	}
	return out
}

func isMajorBump(from, to string) bool {
	f, err1 := semver.NewVersion(from)
	t, err2 := semver.NewVersion(to)
	if err1 != nil || err2 != nil {
		return true // unknown shape: treat as the risky case
	}
	return t.Major() != f.Major()
}

func withoutMajors(target, current map[string]string) map[string]string {
	out := map[string]string{}
	for slug, to := range target {
		if !isMajorBump(current[slug], to) {
			out[slug] = to
		}
	}
	return out
}

func planUpgrade(infos []VersionInfo, solve solveFunc) (upgradePlan, error) {
	wanted := newestTargets(infos)
	if len(wanted) == 0 {
		return upgradePlan{}, nil
	}
	violations, err := solve(wanted)
	if err != nil {
		return upgradePlan{}, err
	}
	if len(violations) == 0 {
		return upgradePlan{Target: wanted, Wanted: wanted}, nil
	}

	current := map[string]string{}
	for _, in := range infos {
		current[in.Slug] = in.Current
	}
	minor := withoutMajors(wanted, current)
	if len(minor) > 0 && len(minor) < len(wanted) {
		v2, err := solve(minor)
		if err != nil {
			return upgradePlan{}, err
		}
		if len(v2) == 0 {
			return upgradePlan{Target: minor, Wanted: wanted, DroppedMajors: true}, nil
		}
	}
	return upgradePlan{Wanted: wanted, Violations: violations}, nil
}

func sortedPairs(target map[string]string) []string {
	pairs := make([]string, 0, len(target))
	for slug, v := range target {
		pairs = append(pairs, slug+"@"+v)
	}
	sort.Strings(pairs)
	return pairs
}

func fingerprint(target map[string]string, violations []compatViolation) string {
	parts := sortedPairs(target)
	vs := make([]string, 0, len(violations))
	for _, v := range violations {
		vs = append(vs, v.Package+"|"+v.Requires+"|"+v.Range+"|"+v.Found)
	}
	sort.Strings(vs)
	sum := sha256.Sum256([]byte(strings.Join(parts, ",") + "#" + strings.Join(vs, ",")))
	return hex.EncodeToString(sum[:8])
}

func formatViolations(v []compatViolation) string {
	lines := make([]string, 0, len(v))
	for _, x := range v {
		lines = append(lines, fmt.Sprintf("%s needs %s %s (found %s)", x.Package, x.Requires, x.Range, x.Found))
	}
	return strings.Join(lines, "\n")
}

func formatTarget(target map[string]string) string {
	pairs := sortedPairs(target)
	for i, p := range pairs {
		pairs[i] = strings.Replace(p, "@", " ", 1)
	}
	return strings.Join(pairs, ", ")
}
