//go:build windows

package client

import "io/fs"

// dataFileMode: Windows has no umask, and Go maps the mode bits onto the
// read-only attribute alone. 0666 is the plain "writable data file" answer.
func dataFileMode() fs.FileMode { return 0o666 }
