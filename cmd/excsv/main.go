package main

import (
	"os"

	"github.com/boligolov/excsv-golang/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
