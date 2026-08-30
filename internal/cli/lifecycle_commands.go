package cli

import (
	"fmt"
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

// largeDocumentRows is the point past which --with-data earns a cost notice.
const largeDocumentRows = 100_000

func newInfoCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Compact summary (version, rows, columns, form, profile)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			res, err := loadDoc(cfg, path, false)
			if err != nil {
				exitParseErr(err)
			}
			printWarnings(res.Warnings)
			if res.Pack != nil && cfg.table == "" && cfg.packTable == nil {
				printPackInfo(cfg, res)
				return
			}
			doc := res.Doc
			if cfg.packTable != nil {
				doc = cfg.packTable.Document()
			}
			if cfg.jsonOut {
				out := map[string]any{
					"version": doc.Header.Version, "delim": doc.Header.DelimName,
					"quote": doc.Header.QuoteName, "rows": doc.DeclaredOrCountedRows(),
					"columns": len(doc.Meta.Columns), "form": formName(doc.Form),
					"profile": string(doc.Source.Profile),
				}
				if doc.Source.Reference != "" {
					out["reference"] = doc.Source.Reference
				}
				if doc.Source.ReferencePath != "" {
					out["reference_path"] = doc.Source.ReferencePath
				}
				_ = writeJSON(out)
				return
			}
			line := fmt.Sprintf("ExCSV %s  rows=%d  columns=%d  form=%s  profile=%s",
				doc.Header.Version, doc.DeclaredOrCountedRows(), len(doc.Meta.Columns), formName(doc.Form), doc.Source.Profile)
			if doc.Source.Reference != "" {
				line += fmt.Sprintf("  reference=%s", doc.Source.Reference)
			}
			fmt.Println(line)
		},
	}
}

// newValidateCmd is the single reporter. It runs every check to completion,
// prints the full list of findings, and never writes to the file.
func newValidateCmd(cfg *config) *cobra.Command {
	var withData, schemaOnly bool
	var columns []string
	c := &cobra.Command{
		Use:   "validate",
		Short: "Read-only conformance report; never writes",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if schemaOnly && withData {
				fmt.Fprintln(os.Stderr, "warning: --schema-only conflicts with --with-data; the wider level wins")
			}
			// --column narrows the data scan, so it implies --with-data.
			level := withData || len(columns) > 0
			runValidate(cfg, level, columns)
		},
	}
	c.Flags().BoolVar(&withData, "with-data", false, "also scan the data section (cells, rows=, checksum=, #%)")
	c.Flags().BoolVar(&schemaOnly, "schema-only", false, "explicit default: declarations only, no data scan")
	c.Flags().StringArrayVar(&columns, "column", nil, "narrow the data scan to this column (repeatable; implies --with-data)")
	return c
}

func runValidate(cfg *config, withData bool, columns []string) {
	path := targetPath()
	res, err := loadDoc(cfg, path, withData)
	if err != nil {
		exitParseErr(err)
	}
	if withData {
		warnDataScanCost(res)
	}

	report := excsv.ValidateReport{WithData: withData}
	for _, w := range res.Warnings {
		if !withData && excsv.IsDataLevelWarning(w.Kind) {
			continue
		}
		report.Findings = append(report.Findings, excsv.Finding{Issue: w})
	}
	opts := excsv.ValidateOptions{WithData: withData, Columns: columns}
	for _, doc := range validationTargets(cfg, res) {
		sub := doc.Validate(opts)
		report.Findings = append(report.Findings, sub.Findings...)
	}

	if cfg.jsonOut {
		_ = writeJSON(map[string]any{
			"ok": report.OK(), "path": path, "with_data": withData,
			"findings": report.Findings, "repair": report.RepairCommand(),
		})
	} else {
		for _, f := range report.Findings {
			fmt.Fprintf(os.Stderr, "%s\n", f.Error())
		}
		if repair := report.RepairCommand(); repair != "" {
			fmt.Fprintf(os.Stderr, "repair with: excsv %s %s\n", path, repair)
		}
		if report.OK() {
			fmt.Println("ok")
		}
	}
	if !report.OK() && !cfg.lenient {
		os.Exit(2)
	}
}

func validationTargets(cfg *config, res *excsv.ParseResult) []*excsv.Document {
	if res.Pack == nil {
		return []*excsv.Document{res.Doc}
	}
	if cfg.packTable != nil {
		return []*excsv.Document{cfg.packTable.Document()}
	}
	out := make([]*excsv.Document, 0, len(res.Pack.Tables))
	for i := range res.Pack.Tables {
		out = append(out, res.Pack.Tables[i].Document())
	}
	return out
}

func warnDataScanCost(res *excsv.ParseResult) {
	if !stderrIsTerminal() {
		return
	}
	rows := 0
	container := ""
	if res.Pack != nil {
		container = "pack"
		for i := range res.Pack.Tables {
			rows += res.Pack.Tables[i].Document().DeclaredOrCountedRows()
		}
	} else if res.Doc != nil {
		rows = res.Doc.DeclaredOrCountedRows()
		if res.Doc.Form == excsv.FormZipInner {
			container = "zip"
		}
	}
	if rows < largeDocumentRows && container == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "note: scanning the data section (%d rows%s); this reads the whole document\n",
		rows, map[bool]string{true: ", " + container + " decompression required", false: ""}[container != ""])
}

// newFixCmd is the single repairer. It writes in place and never reports
// conformance; that is validate's job.
func newFixCmd(cfg *config) *cobra.Command {
	var only string
	var columns []string
	var dryRun bool
	c := &cobra.Command{
		Use:   "fix",
		Short: "Repair derived metadata in place (format, columns, agg, checksum, rows, stamp)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			targets, err := excsv.ParseFixTargets(only)
			if err != nil {
				exitUserErr(err.Error())
			}
			runFix(cfg, excsv.FixOptions{Only: targets, Columns: columns, DryRun: dryRun})
		},
	}
	c.Flags().StringVar(&only, "only", "", "comma-separated targets: format,columns,agg,checksum,rows,stamp (default: all)")
	c.Flags().StringArrayVar(&columns, "column", nil, "narrow per-column targets to this column (repeatable)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change and write nothing")
	return c
}

func runFix(cfg *config, opts excsv.FixOptions) {
	path := targetPath()
	if excsv.IsPackPath(path) {
		runFixPack(cfg, path, opts)
		return
	}
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	report, err := doc.Fix(opts)
	if err != nil {
		exitParseErr(err)
	}
	if !opts.DryRun {
		if err := saveDocument(cfg, doc, path); err != nil {
			exitParseErr(err)
		}
	}
	printFixReport(cfg, path, report)
}

func runFixPack(cfg *config, path string, opts excsv.FixOptions) {
	res, err := loadDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	printWarnings(res.Warnings)
	if cfg.table != "" || cfg.packTable != nil {
		doc, err := loadTableDoc(cfg, path, true)
		if err != nil {
			exitParseErr(err)
		}
		report, err := doc.Fix(opts)
		if err != nil {
			exitParseErr(err)
		}
		if !opts.DryRun {
			if err := saveDocument(cfg, doc, path); err != nil {
				exitParseErr(err)
			}
		}
		printFixReport(cfg, path, report)
		return
	}
	report, err := res.Pack.Fix(opts)
	if err != nil {
		exitParseErr(err)
	}
	if !opts.DryRun {
		if err := saveDocument(cfg, res.Doc, path); err != nil {
			exitParseErr(err)
		}
	}
	printFixReport(cfg, path, report)
}

func printFixReport(cfg *config, path string, report excsv.FixReport) {
	if cfg.jsonOut {
		changed := report.Changed
		if changed == nil {
			changed = []string{}
		}
		_ = writeJSON(map[string]any{
			"ok": true, "path": path, "changed": changed, "dry_run": report.DryRun,
		})
		return
	}
	if len(report.Changed) == 0 {
		fmt.Println("ok (nothing to repair)")
		return
	}
	verb := "repaired"
	if report.DryRun {
		verb = "would repair"
	}
	for _, target := range report.Changed {
		fmt.Printf("%s: %s\n", verb, target)
	}
}
