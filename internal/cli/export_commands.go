package cli

import (
	"fmt"
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

// newExportCmd writes foreign representations of the whole document. It owns no
// meta line and never modifies FILE.
func newExportCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "export", Short: "Write a foreign representation; never modifies FILE"}
	cmd.AddCommand(newExportJSONCmd(cfg), newExportCSVWCmd(cfg))
	return cmd
}

func newExportJSONCmd(cfg *config) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "json",
		Short: "Write the v0.4 JSON form (.excsv.json)",
		Long: `Write the v0.4 JSON form (.excsv.json).

The text and JSON forms are a bijection by specification, so this export is
lossless with exactly one exception: free-text ## comments carry no structured
meaning and have no JSON slot. That loss is reported on stderr when it applies.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			res, err := loadDoc(cfg, targetPath(), true)
			if err != nil {
				exitParseErr(err)
			}
			printWarnings(res.Warnings)

			var exported *excsv.JSONExportResult
			opts := excsv.JSONExportOptions{Indent: "  "}
			if res.Pack != nil && cfg.packTable == nil {
				exported, err = res.Pack.ExportJSON(opts)
			} else {
				doc := res.Doc
				if cfg.packTable != nil {
					doc = cfg.packTable.Document()
				}
				exported, err = doc.ExportJSON(opts)
			}
			if err != nil {
				exitIOErr(err)
			}
			for _, d := range exported.Dropped {
				fmt.Fprintf(os.Stderr, "not represented in JSON: %s\n", d)
			}
			if err := writeOutputBytes(out, exported.Data); err != nil {
				exitIOErr(err)
			}
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output .excsv.json path (default stdout)")
	return c
}

func newExportCSVWCmd(cfg *config) *cobra.Command {
	var out, url string
	var enumAsPattern bool
	c := &cobra.Command{
		Use:   "csvw",
		Short: "Write a CSVW metadata sidecar (write-only; nothing reads CSVW)",
		Long: `Write a CSVW metadata sidecar.

CSVW is write-only in this tool. Serializing to a standard is total and
deterministic: every #column attribute either has a target or is named as
dropped. Reading CSVW would be a merge with a foreign schema that can contradict
the data, and every conflict there has a plausible default but no correct one —
so use --column-attr to declare the same things with a human behind them.

The mapping is lossy and says so: every attribute it cannot carry is named on
stderr. An inline ExCSV document is not a CSV file, so --url is required unless
the document is a sidecar that already declares reference=.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			res, err := loadDoc(cfg, targetPath(), true)
			if err != nil {
				exitParseErr(err)
			}
			printWarnings(res.Warnings)

			opts := excsv.CSVWExportOptions{URL: url, EnumAsPattern: enumAsPattern, Table: cfg.table, Indent: "  "}
			var exported *excsv.CSVWExportResult
			if res.Pack != nil && cfg.packTable == nil {
				exported, err = res.Pack.ExportCSVW(opts)
			} else {
				doc := res.Doc
				if cfg.packTable != nil {
					doc = cfg.packTable.Document()
				}
				exported, err = doc.ExportCSVW(opts)
			}
			if err != nil {
				exitUserErr(err.Error())
			}
			for _, d := range exported.SortedDropped() {
				fmt.Fprintf(os.Stderr, "dropped: %s\n", d)
			}
			if err := writeOutputBytes(out, exported.Data); err != nil {
				exitIOErr(err)
			}
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output metadata path (default stdout)")
	c.Flags().StringVar(&url, "url", "", "the CSV file the metadata describes (required unless the document is a sidecar)")
	c.Flags().BoolVar(&enumAsPattern, "enum-as-pattern", false, "encode enum= as a datatype.format regex instead of dropping it")
	return c
}
