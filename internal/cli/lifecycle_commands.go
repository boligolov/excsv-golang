package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

// largeDocumentRows is the point past which --with-data earns a cost notice.
const largeDocumentRows = 100_000

func newInfoCmd(cfg *config) *cobra.Command {
	var noMeta bool
	c := &cobra.Command{
		Use:   "info",
		Short: "Document summary and column header views",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runInfo(cfg, noMeta)
		},
	}
	c.Flags().BoolVar(&noMeta, "no-meta", false, "omit #@ file metadata from the output")
	c.AddCommand(&cobra.Command{
		Use:   "header",
		Short: "Column schema from #column lines",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runInfoHeader(cfg)
		},
	})
	return c
}

type infoExtras struct {
	Aggregations []string
	SQLCount     int
	SQLKeys      []string
	Meta         map[string]string
}

func runInfo(cfg *config, noMeta bool) {
	path := targetPath()
	res, err := loadDoc(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	printWarnings(res.Warnings)
	if res.Pack != nil && cfg.table == "" && cfg.packTable == nil {
		printPackInfo(cfg, res, noMeta)
		return
	}
	doc := res.Doc
	if cfg.packTable != nil {
		doc = cfg.packTable.Document()
	}
	printDocumentInfo(cfg, doc, noMeta)
}

func runInfoHeader(cfg *config) {
	doc, err := loadTableDoc(cfg, targetPath(), false)
	if err != nil {
		exitParseErr(err)
	}
	printInfoHeader(cfg, doc)
}

func columnNames(doc *excsv.Document) []string {
	names := make([]string, 0, len(doc.Meta.Columns))
	for _, col := range doc.Meta.Columns {
		names = append(names, col.Attrs["name"])
	}
	return names
}

func printInfoHeader(cfg *config, doc *excsv.Document) {
	names := columnNames(doc)
	if cfg.jsonOut {
		cols := make([]map[string]string, 0, len(doc.Meta.Columns))
		for _, col := range doc.Meta.Columns {
			cols = append(cols, col.Attrs)
		}
		_ = writeJSON(map[string]any{
			"header":  names,
			"columns": cols,
		})
		return
	}
	fmt.Println(strings.Join(names, ","))
	for _, col := range doc.Meta.Columns {
		fmt.Println(excsv.FormatColumnInfoLine(col.Attrs))
	}
}

func collectInfoExtras(doc *excsv.Document, noMeta bool) infoExtras {
	var out infoExtras
	for _, a := range doc.Meta.Aggregations {
		out.Aggregations = append(out.Aggregations, a.Name)
	}
	sort.Strings(out.Aggregations)

	for _, s := range doc.Meta.SQL {
		out.SQLKeys = append(out.SQLKeys, s.RawKey)
	}
	sort.Strings(out.SQLKeys)
	out.SQLCount = len(out.SQLKeys)

	if !noMeta {
		meta := doc.MetaMap()
		if len(meta) > 0 {
			out.Meta = meta
		}
	}
	return out
}

func printDocumentInfo(cfg *config, doc *excsv.Document, noMeta bool) {
	extras := collectInfoExtras(doc, noMeta)
	if cfg.jsonOut {
		out := documentInfoJSON(doc)
		applyInfoExtrasJSON(out, extras)
		_ = writeJSON(out)
		return
	}
	printDocumentInfoText(doc, extras)
}

func documentInfoJSON(doc *excsv.Document) map[string]any {
	out := map[string]any{
		"version": doc.Header.Version,
		"rows":    doc.DeclaredOrCountedRows(),
		"columns": len(doc.Meta.Columns),
		"form":    formName(doc.Form),
		"profile": string(doc.Source.Profile),
		"delim":   doc.Header.DelimName,
		"quote":   infoQuoteLabel(doc.Header),
		"null":    infoNullLabel(doc.Header.Null),
	}
	if doc.Header.SQLDialect != "" {
		out["sql_dialect"] = doc.Header.SQLDialect
	}
	if doc.Source.Reference != "" {
		out["reference"] = doc.Source.Reference
	}
	if doc.Source.ReferencePath != "" {
		out["reference_path"] = doc.Source.ReferencePath
	}
	return out
}

func infoQuoteLabel(h excsv.Header) string {
	if !h.QuoteEnabled || h.QuoteName == "none" {
		return "none (fields not quoted)"
	}
	return h.QuoteName
}

func infoNullLabel(null string) string {
	if null == "" {
		return "(empty string)"
	}
	return null
}

func printDocumentInfoText(doc *excsv.Document, extras infoExtras) {
	fmt.Printf("ExCSV %s\n", doc.Header.Version)
	fmt.Printf("Rows: %d\n", doc.DeclaredOrCountedRows())
	fmt.Printf("Columns: %d\n", len(doc.Meta.Columns))
	fmt.Printf("Form: %s\n", formName(doc.Form))
	fmt.Printf("Profile: %s\n", doc.Source.Profile)
	fmt.Printf("Delimiter: %s\n", doc.Header.DelimName)
	fmt.Printf("Quote: %s\n", infoQuoteLabel(doc.Header))
	fmt.Printf("Null: %s\n", infoNullLabel(doc.Header.Null))
	if doc.Header.SQLDialect != "" {
		fmt.Printf("SQL dialect: %s\n", doc.Header.SQLDialect)
	}
	if doc.Source.Reference != "" {
		fmt.Printf("Reference: %s\n", doc.Source.Reference)
	}
	printInfoExtrasText(extras)
}

func applyInfoExtrasJSON(out map[string]any, extras infoExtras) {
	if len(extras.Aggregations) > 0 {
		out["aggregations"] = extras.Aggregations
	}
	if extras.SQLCount > 0 {
		out["sql"] = map[string]any{"count": extras.SQLCount, "keys": extras.SQLKeys}
	}
	if len(extras.Meta) > 0 {
		out["meta"] = extras.Meta
	}
}

func printInfoExtrasText(extras infoExtras) {
	if len(extras.Aggregations) > 0 {
		fmt.Printf("Aggregations: %s\n", strings.Join(extras.Aggregations, ", "))
	}
	if extras.SQLCount > 0 {
		fmt.Printf("SQL (%d): %s\n", extras.SQLCount, strings.Join(extras.SQLKeys, ", "))
	}
	if len(extras.Meta) > 0 {
		keys := make([]string, 0, len(extras.Meta))
		for k := range extras.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s: %s\n", k, extras.Meta[k])
		}
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
