package coreserver

import (
	"strings"
	"testing"
)

// The assemble behavior itself is covered in pkgbuild/assemble_test.go. What
// stays here is the host MemberSource's contract: an unchanged member the
// current build doesn't actually carry must fail the rebuild loudly, not
// produce a silently-incomplete build.
func TestHostMemberSource_CopyCurrentMissingMemberFailsLoudly(t *testing.T) {
	err := hostMemberSource{}.CopyCurrent(
		MemberSpec{Slug: "definitely-absent-member", FromCurrent: true}, t.TempDir())
	if err == nil {
		t.Fatal("expected copy of a member absent from the current build to fail")
	}
	if !strings.Contains(err.Error(), "current build missing member") ||
		!strings.Contains(err.Error(), "definitely-absent-member") {
		t.Fatalf("error should name the missing member, got: %v", err)
	}
}
