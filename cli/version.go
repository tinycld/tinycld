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
			if d.out.Format == output.JSON {
				return d.out.Write(cmd.OutOrStdout(), nil, nil, v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "tinycld %s (%s %s/%s)\n", v.Version, v.Go, v.OS, v.Arch)
			return nil
		},
	}
}
