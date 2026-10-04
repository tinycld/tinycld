//go:build !unix

package coreserver

import "github.com/pocketbase/pocketbase/core"

// registerSupervised binds nothing: there is no supervisor on this platform.
func registerSupervised(core.App) {}
