package docfonts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir_UnsetIsEmpty(t *testing.T) {
	t.Setenv(EnvVar, "")
	if got := Dir(); got != "" {
		t.Errorf("Dir() = %q", got)
	}
}

func TestDir_ReturnsAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvVar, dir)
	if got := Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}

// A path that is missing or is a file is ignored, so a typo in the setting
// degrades to "no extra fonts" instead of failing every render.
func TestDir_IgnoresAMissingPathOrAFile(t *testing.T) {
	t.Setenv(EnvVar, filepath.Join(t.TempDir(), "absent"))
	if got := Dir(); got != "" {
		t.Errorf("missing dir: Dir() = %q", got)
	}
	file := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, file)
	if got := Dir(); got != "" {
		t.Errorf("file: Dir() = %q", got)
	}
}
