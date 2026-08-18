//go:build !windows

package client

import (
	"io/fs"
	"sync"
	"syscall"
)

// dataFileMode is the mode a newly downloaded FILE should get: the
// conventional 0666 reduced by the process umask, the same result an ordinary
// `open(..., 0666)` produces in curl or scp.
//
// os.CreateTemp hardcodes 0600, which is right for a credential and wrong for
// user data — a downloaded spreadsheet the user cannot share with their own
// group is a surprise no other tool produces.
//
// Umask can only be READ by setting it, so this samples once and restores
// immediately. Done under a sync.Once at first use rather than in an init:
// the swap is momentarily visible to other goroutines, so it must happen as
// few times as possible.
var (
	umaskOnce  sync.Once
	cachedMode fs.FileMode
)

func dataFileMode() fs.FileMode {
	umaskOnce.Do(func() {
		mask := syscall.Umask(0)
		syscall.Umask(mask)
		cachedMode = fs.FileMode(0o666 &^ mask)
	})
	return cachedMode
}
