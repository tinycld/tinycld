package main

import (
	"fmt"
	"os"

	"tinycld.org/cli/client"
)

func main() {
	d := defaultDeps()
	root := newRootCmd(d)
	// Package commands (generated cli_extensions.go) share one lazy client:
	// the active context resolves on the first actual request, so --help and
	// completion work with no context configured.
	registerPackageCommands(root, client.NewLazy(d.resolveClientContext, d.httpClient))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
