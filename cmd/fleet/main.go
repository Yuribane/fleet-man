package main

import (
	"os"

	"github.com/fleet-man/fleet-man/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
