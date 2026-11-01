//go:build !linux && !darwin

package pkgbuild

import "os/exec"

// killGroupOnCancel keeps exec.CommandContext's own cancel, which kills only
// the direct child: the process group handling (procgroup) is built for
// linux and darwin only.
func killGroupOnCancel(cmd *exec.Cmd) (wait func() error) { return cmd.Wait }
