//go:build windows

package backup

import "errors"

// Windows has no Statfs. The precheck treats an error as "cannot tell" and lets
// the run proceed, which is the right outcome: a deployment that cannot be
// measured must not be refused a backup.
func statfsAvailable(string) (int64, error) {
	return 0, errors.New("backup: free space is not measurable on this platform")
}
