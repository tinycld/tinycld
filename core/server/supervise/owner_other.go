//go:build !unix

package supervise

import "os"

// matchOwner is a no-op where files carry no unix owner.
func matchOwner(string, os.FileInfo) error { return nil }
