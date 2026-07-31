//go:build !linux

package tenantmain

import "fmt"

// confinePackages is Linux-only. Reaching it elsewhere means the parent asked
// for confinement a non-Linux host cannot provide, and silently serving
// unconfined would be worse than refusing.
func confinePackages(dir string) error {
	return fmt.Errorf("package confinement requires Linux (mount namespaces); refusing to serve unconfined")
}
