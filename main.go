// Package main is the entrypoint for the kcompass CLI.
package main

import (
	"os"

	"github.com/nonchord/kcompass/internal/cli"
)

// Set by goreleaser via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	root := cli.NewRootCommand()
	root.Version = version + " (" + commit + ")"
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
