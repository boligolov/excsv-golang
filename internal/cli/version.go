package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set at link time via -ldflags -X.
var (
	Version   = "0.0.2"
	BuildTime = "unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("excsv-cli %s (built %s)\n", Version, BuildTime)
		},
	}
}
