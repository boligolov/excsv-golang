package gencsv

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version   = "0.1.0"
	BuildTime = "unknown"
)

// NewRoot returns the gencsv cobra command tree.
func NewRoot() *cobra.Command {
	var (
		rows       int
		format     string
		outPath    string
		noHeader   bool
		seed       int64
		columnArgs []string
	)

	root := &cobra.Command{
		Use:   "gencsv",
		Short: "Generate plain CSV/TSV with dummy data for testing",
		Long: `Generate plain delimited files from column specs.

Column flag: --column=name,type[,nulls]
  Types: int, string, date, float, boolean, null
  Third field (e.g. "nulls") enables sparse empty cells (~10%).

Example:
  gencsv --rows=10000 --column=a,int --column=b,string --column=c,date,nulls
  gencsv --rows=5 --format=tsv --column=id,int --column=note,string -o sample.tsv`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(columnArgs) == 0 {
				return fmt.Errorf("at least one --column is required")
			}
			cols := make([]ColumnSpec, 0, len(columnArgs))
			for _, raw := range columnArgs {
				spec, err := ParseColumnSpec(raw)
				if err != nil {
					return err
				}
				cols = append(cols, spec)
			}
			f, err := ParseFormat(format)
			if err != nil {
				return err
			}
			opts := Options{
				Rows:    rows,
				Columns: cols,
				Format:  f,
				Header:  !noHeader,
			}
			if cmd.Flags().Changed("seed") {
				opts.Seed = &seed
			}

			var w *os.File
			if outPath == "" {
				w = os.Stdout
			} else {
				w, err = os.Create(outPath)
				if err != nil {
					return err
				}
				defer w.Close()
			}
			return Write(w, opts)
		},
	}

	root.Flags().IntVar(&rows, "rows", 100, "number of data rows to emit")
	root.Flags().StringVar(&format, "format", "csv", "output format: csv or tsv")
	root.Flags().StringArrayVar(&columnArgs, "column", nil, "column spec name,type[,nulls] (repeatable)")
	root.Flags().StringVarP(&outPath, "output", "o", "", "write to file (default stdout)")
	root.Flags().BoolVar(&noHeader, "no-header", false, "omit header row")
	root.Flags().Int64Var(&seed, "seed", 0, "random seed for sparse null placement")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gencsv %s (built %s)\n", Version, BuildTime)
		},
	})

	return root
}
