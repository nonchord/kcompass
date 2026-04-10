// Package main is the entrypoint for the kcompass CLI.
package main

import (
	"os"

	"github.com/nonchord/kcompass/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
