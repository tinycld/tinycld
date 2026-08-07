package output

import "fmt"

// FormatBytes renders a byte count exactly the way the server's
// quota.FormatBytes does, so CLI sizes read identically to the app.
// Reimplemented rather than imported: tinycld.org/core is PocketBase-hook
// code, and pulling it into the CLI workspace would drag the fork replace
// into every member cli build (see gen-cli.ts). bytes_test.go pins parity.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}
