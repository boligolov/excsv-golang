package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

func newDataCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "data", Short: "Data section operations"}
	cmd.AddCommand(
		newDataPrintCmd(cfg),
		newDataGetCmd(cfg),
		newDataAppendCmd(cfg),
		newDataSortCmd(cfg),
	)
	return cmd
}

// newDataPrintCmd is the one way to print the data section. With no flags it
// dumps the header row plus every body row verbatim.
func newDataPrintCmd(cfg *config) *cobra.Command {
	var limit, offset int
	var cols, out string
	c := &cobra.Command{
		Use:   "print",
		Short: "Print the data section as CSV/TSV in the document's own dialect",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runDataPrint(cfg, limit, offset, cols, out)
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "write to a file instead of stdout")
	c.Flags().IntVar(&limit, "limit", 0, "max body rows (0 = all)")
	c.Flags().IntVar(&offset, "offset", 0, "skip this many body rows")
	c.Flags().StringVar(&cols, "select", "", "comma-separated column names or indexes to project")
	return c
}

func newDataGetCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "get ROW [COLUMN]",
		Short: "Print one body row (0-based) or a single cell",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			row, err := strconv.Atoi(args[0])
			if err != nil {
				exitUserErr("ROW must be an integer")
			}
			col := ""
			if len(args) == 2 {
				col = args[1]
			}
			runDataGet(cfg, row, col)
		},
	}
}

func newDataAppendCmd(cfg *config) *cobra.Command {
	var rows []string
	var file string
	var skipHeader bool
	c := &cobra.Command{
		Use:   "append",
		Short: "Append rows (--row and/or --file)",
		Run: func(cmd *cobra.Command, args []string) {
			if len(rows) == 0 && file == "" {
				exitUserErr("provide --row and/or --file")
			}
			runDataAppend(cfg, rows, file, skipHeader)
		},
	}
	c.Flags().StringArrayVar(&rows, "row", nil, "one data row in the document dialect (repeatable)")
	c.Flags().StringVar(&file, "file", "", "append rows from a delimited file (document dialect)")
	c.Flags().BoolVar(&skipHeader, "skip-header", false, "skip the first line of --file")
	return c
}

func newDataSortCmd(cfg *config) *cobra.Command {
	var by []string
	var desc bool
	c := &cobra.Command{
		Use:   "sort",
		Short: "Sort body rows (--by NAME or index, optional :asc/:desc)",
		Run: func(cmd *cobra.Command, args []string) {
			if len(by) == 0 {
				exitUserErr("--by is required")
			}
			runDataSort(cfg, by, desc)
		},
	}
	c.Flags().StringArrayVar(&by, "by", nil, "column name or index, optionally name:desc (repeatable)")
	c.Flags().BoolVar(&desc, "desc", false, "descending for keys without :asc/:desc")
	return c
}

func runDataPrint(cfg *config, limit, offset int, cols, out string) {
	path := targetPath()
	if printSidecarNotice(cfg, path) {
		return
	}
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	indexes, err := projectIndexes(doc, cols)
	if err != nil {
		exitUserErr(err.Error())
	}
	header := projectFields(doc.Data.HeaderRow, indexes)
	rows := doc.Data.Rows
	if offset > 0 {
		if offset >= len(rows) {
			rows = nil
		} else {
			rows = rows[offset:]
		}
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	projected := make([][]string, len(rows))
	for i, row := range rows {
		projected[i] = projectFields(row, indexes)
	}
	if cfg.jsonOut {
		out := map[string]any{"rows": projected}
		if doc.Data.HasHeaderRow {
			out["header"] = header
		}
		_ = writeJSON(out)
		return
	}
	d := doc.Header.Dialect()
	var b strings.Builder
	if doc.Data.HasHeaderRow {
		b.WriteString(excsv.JoinCSVFields(header, d))
		b.WriteByte('\n')
	}
	for _, row := range projected {
		b.WriteString(excsv.JoinCSVFields(row, d))
		b.WriteByte('\n')
	}
	if err := writeOutputBytes(out, []byte(b.String())); err != nil {
		exitIOErr(err)
	}
}

// printSidecarNotice keeps the old strip guard: a metadata-only sidecar has no
// data of its own, so re-parse without reference resolution, explain, and exit 0.
func printSidecarNotice(cfg *config, path string) bool {
	if !isSidecarInputPath(path) {
		return false
	}
	opts := cfg.parseOpts()
	opts.ResolveReference = false
	res, err := excsv.ParseFile(path, opts)
	if err != nil {
		exitParseErr(err)
	}
	if !excsv.IsSidecarMetaOnly(res.Doc) {
		return false
	}
	ref := res.Doc.Header.Fields["reference"]
	if ref == "" {
		ref = res.Doc.Source.Reference
	}
	msg := "that's a sidecar file (metadata only"
	if ref != "" {
		msg += "; the data is in " + ref
	}
	msg += "). There is nothing to print here."
	fmt.Fprintln(os.Stderr, msg)
	return true
}

func isSidecarInputPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".excsv", ".ecsv", ".extsv":
		return true
	}
	return false
}

// delimitedData renders the whole data section in the document's own dialect.
func delimitedData(doc *excsv.Document) string {
	d := doc.Header.Dialect()
	var b strings.Builder
	if doc.Data.HasHeaderRow {
		b.WriteString(excsv.JoinCSVFields(doc.Data.HeaderRow, d))
		b.WriteByte('\n')
	}
	for _, row := range doc.Data.Rows {
		b.WriteString(excsv.JoinCSVFields(row, d))
		b.WriteByte('\n')
	}
	return b.String()
}

func runDataGet(cfg *config, row int, colRef string) {
	doc, err := loadTableDoc(cfg, targetPath(), true)
	if err != nil {
		exitParseErr(err)
	}
	if row < 0 || row >= len(doc.Data.Rows) {
		exitUserErr(fmt.Sprintf("row index out of range: %d", row))
	}
	fields := doc.Data.Rows[row]
	if colRef != "" {
		idx, err := doc.ColumnIndex(colRef)
		if err != nil {
			exitUserErr(err.Error())
		}
		val := ""
		if idx < len(fields) {
			val = fields[idx]
		}
		if cfg.jsonOut {
			_ = writeJSON(map[string]any{"row": row, "column": colRef, "value": val})
			return
		}
		fmt.Println(val)
		return
	}
	if cfg.jsonOut {
		_ = writeJSON(fields)
		return
	}
	fmt.Println(excsv.JoinCSVFields(fields, doc.Header.Dialect()))
}

func runDataAppend(cfg *config, rowArgs []string, file string, skipHeader bool) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	d := doc.Header.Dialect()
	var rows [][]string
	for _, line := range rowArgs {
		fields, err := excsv.SplitCSVFields(line, d)
		if err != nil {
			exitParseErr(err)
		}
		rows = append(rows, fields)
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(3)
		}
		res, err := excsv.ImportDelimited(data, excsv.ImportOptions{
			Strict:     !cfg.lenient,
			NoHeader:   true,
			SourcePath: file,
		})
		if err != nil {
			exitParseErr(err)
		}
		fileRows := res.Doc.Data.Rows
		if len(fileRows) > 0 && shouldSkipAppendHeader(doc, fileRows[0], skipHeader) {
			fileRows = fileRows[1:]
		}
		rows = append(rows, fileRows...)
	}
	if err := doc.AppendRows(rows, !cfg.lenient); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, "", map[string]any{"rows": doc.RowCount()})
}

func runDataSort(cfg *config, by []string, descDefault bool) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	var keys []excsv.SortKey
	for _, spec := range by {
		ref, desc := parseSortSpec(spec, descDefault)
		idx, err := doc.ColumnIndex(ref)
		if err != nil {
			exitUserErr(err.Error())
		}
		keys = append(keys, excsv.SortKey{Index: idx, Desc: desc})
	}
	if err := doc.SortRows(keys); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, "", map[string]any{"rows": doc.RowCount()})
}

func parseSortSpec(spec string, descDefault bool) (string, bool) {
	ref, rest, ok := strings.Cut(spec, ":")
	if ok {
		switch strings.ToLower(rest) {
		case "desc":
			return ref, true
		case "asc":
			return ref, false
		}
	}
	return spec, descDefault
}

func projectIndexes(doc *excsv.Document, cols string) ([]int, error) {
	if strings.TrimSpace(cols) == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(cols, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, err := doc.ColumnIndex(part)
		if err != nil {
			return nil, err
		}
		out = append(out, idx)
	}
	return out, nil
}

func projectFields(fields []string, indexes []int) []string {
	if indexes == nil {
		return fields
	}
	out := make([]string, len(indexes))
	for i, idx := range indexes {
		if idx >= 0 && idx < len(fields) {
			out[i] = fields[idx]
		}
	}
	return out
}

func shouldSkipAppendHeader(doc *excsv.Document, first []string, skip bool) bool {
	if skip {
		return true
	}
	if !doc.Data.HasHeaderRow {
		return false
	}
	if len(first) != len(doc.Data.HeaderRow) {
		return false
	}
	for i := range first {
		if first[i] != doc.Data.HeaderRow[i] {
			return false
		}
	}
	return true
}
