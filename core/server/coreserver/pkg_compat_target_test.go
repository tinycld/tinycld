package coreserver

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/pkgbuild/pkgbuildtest"
)

// The behavior of the verify itself is covered in pkgbuild/compat_verify_test.go.
// What must stay HERE is the production wiring: rebuild()'s dependency set must
// bind verifyCompat to the real checker. Without this, the orchestrator tests
// could pass with verifyCompat left nil in production and the whole gate
// silently absent.
func TestProductionRebuildDeps_WireVerifyCompat(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.4")
	pkgbuildtest.WriteBuildMember(t, buildDir, "mail", "2.0.0",
		map[string]string{corePackageKey: ">=0.5.0 <0.6.0"})
	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		{Slug: baseMemberSlug, FromCurrent: true},
		{Slug: "mail"},
	}}

	deps := productionRebuildDeps(pocketbase.New(), &installJob{ID: "j"}, m, nil)
	if deps.verifyCompat == nil {
		t.Fatal("production deps must wire verifyCompat")
	}
	err := deps.verifyCompat(m, buildDir)
	if err == nil || !strings.Contains(err.Error(), "mail") {
		t.Fatalf("production verifyCompat must refuse the violating build, got: %v", err)
	}
}
