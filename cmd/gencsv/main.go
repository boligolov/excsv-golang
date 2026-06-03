package main

import (
	"fmt"
	"os"

	"github.com/boligolov/excsv-golang/internal/gencsv"
)

func main() {
	if err := gencsv.NewRoot().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
