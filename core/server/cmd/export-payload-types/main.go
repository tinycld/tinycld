// Command export-payload-types emits TypeScript declarations from one
// feature package's Go payload package (its manifest `payloads: { package }`
// dir) and exits.
//
// Invoked per adopting package by app/scripts/gen-payload-types.ts (a step of
// scripts/generate.ts, so it runs on every `pnpm install` and dev-loop
// regeneration). Like cmd/export-types it is pure Go with no CGO and imports
// only core packages — payload sources are parsed as text with go/ast, never
// imported — so it builds inside the lean web-builder Docker stage.
//
// Usage:
//
//	export-payload-types --src <payload package dir> --out <file.ts>
//
// Both flags are required. Any payload construct the generator cannot map
// faithfully is a hard error and nothing is written.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"tinycld.org/core/payloadgen"
)

func main() {
	src := flag.String("src", "", "Go payload package directory to parse")
	out := flag.String("out", "", "TypeScript file to write")
	flag.Parse()

	if *src == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}

	ts, err := payloadgen.Generate(*src)
	if err != nil {
		log.Fatalf("export-payload-types: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("export-payload-types: mkdir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(ts), 0o644); err != nil {
		log.Fatalf("export-payload-types: write %s: %v", *out, err)
	}
	log.Printf("export-payload-types: wrote %s", *out)
}
