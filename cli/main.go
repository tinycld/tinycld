package main

import (
	"fmt"
	"os"
)

func main() {
	d := defaultDeps()
	root := newRootCmd(d)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
