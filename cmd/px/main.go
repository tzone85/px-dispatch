package main

import (
	"os"

	"github.com/tzone85/px-dispatch/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
