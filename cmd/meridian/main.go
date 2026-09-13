package main

import (
	"os"

	"github.com/sadisticbrew/meridian/internal/cli"
)

func main() {
	if code := cli.Execute(); code != 0 {
		os.Exit(code)
	}
}
