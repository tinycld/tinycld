package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"tinycld.org/cli/output"
)

// version is stamped by the build pipeline via
// -ldflags "-X main.version=<v>"; "dev" means a local build.
var version = "dev"

func newVersionCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			type info struct {
				Version string `json:"version"`
				Go      string `json:"go"`
				OS      string `json:"os"`
				Arch    string `json:"arch"`
			}
			v := info{Version: version, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
			// Only the human format is hand-written. --output csv used to fall
			// through to it, so a caller asking for CSV got a prose line that
			// no parser accepts; routing both machine formats through Write
			// keeps them in step with every other command.
			if d.out.Format != output.Table {
				return d.out.Write(cmd.OutOrStdout(),
					[]string{"VERSION", "GO", "OS", "ARCH"},
					[][]string{{v.Version, v.Go, v.OS, v.Arch}},
					v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "tinycld %s (%s %s/%s)\n", v.Version, v.Go, v.OS, v.Arch)
			return nil
		},
	}
}
