package main

import (
	"embed"
	"strings"
)

//go:embed form_help/*.txt
var formHelpFS embed.FS

//go:embed form_examples/*.json
var formExamplesFS embed.FS

// helpText is one embedded long help text. The texts are files, not Go
// strings, so a core test can check them against the server's own lists.
func helpText(name string) string {
	b, err := formHelpFS.ReadFile("form_help/" + name)
	if err != nil {
		panic("form help " + name + " is not embedded")
	}
	return strings.TrimRight(string(b), "\n")
}

func exampleFile(name string) []byte {
	b, err := formExamplesFS.ReadFile("form_examples/" + name)
	if err != nil {
		panic("form example " + name + " is not embedded")
	}
	return b
}
