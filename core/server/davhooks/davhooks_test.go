package davhooks

import "testing"

// Method shorthand stringifies to a method DEFINITION, which is not an
// expression and fails to compile standalone. This is the drive-migration bug
// that unit tests missed.
func TestNormalizeHandlerSourceFixesMethodShorthand(t *testing.T) {
	got := NormalizeHandlerSource("beforeWrite", "beforeWrite(e) { return true }")
	if got != "function beforeWrite(e) { return true }" {
		t.Errorf("shorthand not normalized: %q", got)
	}
}

func TestNormalizeHandlerSourceLeavesValidExpressions(t *testing.T) {
	for _, src := range []string{
		"function (e) { return true }",
		"(e) => { return true }",
		"e => true",
	} {
		if got := NormalizeHandlerSource("beforeWrite", src); got != src {
			t.Errorf("NormalizeHandlerSource rewrote a valid expression %q → %q", src, got)
		}
	}
}

func TestIsKnown(t *testing.T) {
	names := []string{"beforeWrite", "beforeDelete", "canRead", "filterList"}
	for _, name := range names {
		if !isKnown(names, name) {
			t.Errorf("%q should be a known hook", name)
		}
	}
	for _, name := range []string{"beforeMove", "beforeRead", "", "canWrite"} {
		if isKnown(names, name) {
			t.Errorf("%q should not be a known hook", name)
		}
	}
}
