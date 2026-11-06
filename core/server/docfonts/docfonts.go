// Package docfonts names the directory of extra font files that server
// renderers (printed documents, PDF files) use for characters their bundled
// fonts lack, such as Chinese, Japanese, Korean or Devanagari. The server
// operator sets it with the TINYCLD_FONTS_DIR environment variable, as other
// server paths are set (TINYCLD_STATE_DIR, TINYCLD_TYPES_DIR). Unset means the
// renderers use their bundled fonts and the fonts installed on the machine.
package docfonts

import (
	"os"

	"tinycld.org/core/logging"
)

// EnvVar names the fonts directory.
const EnvVar = "TINYCLD_FONTS_DIR"

var log = logging.ForPackage("docfonts")

// Dir returns the fonts directory, or "" when EnvVar is unset or does not name
// a directory. A set but unusable path is logged, because the operator meant it.
func Dir() string {
	dir := os.Getenv(EnvVar)
	if dir == "" {
		return ""
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Warn("the fonts directory is not a directory; using bundled and installed fonts only", "env", EnvVar, "path", dir, "error", err)
		return ""
	}
	return dir
}
