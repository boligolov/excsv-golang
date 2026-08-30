package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
	"github.com/spf13/cobra"
)

// Output shapes for convert. They replace the old mutually exclusive
// --sidecar / --zip booleans, and add pack.
const (
	formatInline  = "inline"
	formatSidecar = "sidecar"
	formatZip     = "zip"
	formatPack    = "pack"
)

type convertFlags struct {
	format      string
	out         string
	delim       string
	quote       string
	encoding    string
	null        string
	reference   string
	table       string
	noHeader    bool
	noChecksum  bool
	agg         string
	meta        []string
	comments    []string
	sql         []string
	columnAttrs []string
}

func newConvertCmd(cfg *config) *cobra.Command {
	f := &convertFlags{}
	c := &cobra.Command{
		Use:   "convert",
		Short: "Import CSV/TSV into ExCSV, or re-encode an existing ExCSV document",
		Long: `Import CSV/TSV into ExCSV, or re-encode an existing ExCSV document.

With a CSV/TSV source, convert generates the full derived metadata set:
#!excsv with delim, quote, rows and checksum, one #column per column with an
inferred type, and #@created / #@source / #@tool. It never generates #%, #$ or
## lines — those are authorial choices, so they stay opt-in.

With an ExCSV source, convert re-encodes: every existing metadata line is
preserved and nothing is regenerated. Only the requested dialect, output shape
and enrichment flags are applied.

No -o always means stdout, in both modes. In-place is -o with the input path.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runConvert(cfg, f)
		},
	}
	c.Flags().StringVar(&f.format, "format", formatInline, "output shape: inline, sidecar, zip, or pack")
	c.Flags().StringVarP(&f.out, "output", "o", "", "output path (default: stdout for inline, derived for the binary shapes)")
	c.Flags().StringVar(&f.delim, "delim", "", "output delimiter (comma, tab, pipe, semicolon, or one character)")
	c.Flags().StringVar(&f.quote, "quote", "", "output quoting (none, double, single, or one character)")
	c.Flags().StringVar(&f.encoding, "encoding", "", "output encoding= (UTF-8 only)")
	c.Flags().StringVar(&f.null, "null", "", "null= token; rewrites null cells to it")
	c.Flags().BoolVar(&f.noHeader, "no-header", false, "treat every row as data (header=0)")
	c.Flags().BoolVar(&f.noChecksum, "no-checksum", false, "skip checksum= (emitted by default)")
	c.Flags().StringVar(&f.reference, "reference", "", "sidecar reference= target (--format sidecar only)")
	c.Flags().StringVar(&f.table, "table", "", "pack table name (--format pack only)")
	c.Flags().StringVar(&f.agg, "agg", "", "add #% aggregations: a name list, or the shorthands default / all")
	c.Flags().StringArrayVar(&f.meta, "meta", nil, "add #@KEY: VAL (repeatable)")
	c.Flags().StringArrayVar(&f.comments, "comment", nil, "add a ## line (repeatable)")
	c.Flags().StringArrayVar(&f.sql, "sql", nil, "add #$KEY: SQL (repeatable)")
	c.Flags().StringArrayVar(&f.columnAttrs, "column-attr", nil, "extra #column attribute as COLUMN.ATTR=VALUE (repeatable)")
	return c
}

func runConvert(cfg *config, f *convertFlags) {
	path := targetPath()
	switch f.format {
	case formatInline, formatSidecar, formatZip, formatPack:
	default:
		exitUserErr(fmt.Sprintf("unknown --format %q (want inline, sidecar, zip, or pack)", f.format))
	}
	// reference= MUST NOT appear on an inline document, so --reference outside
	// the sidecar shape is a usage error rather than a header edit.
	if f.reference != "" && f.format != formatSidecar {
		exitUserErr("--reference requires --format sidecar")
	}
	if f.table != "" && f.format != formatPack {
		exitUserErr("--table on convert requires --format pack")
	}

	var doc *excsv.Document
	if isExcsvInput(path) {
		doc = reencodeDocument(cfg, f, path)
	} else {
		doc = importDocument(cfg, f, path)
	}
	emitConverted(cfg, f, path, doc)
}

// isExcsvInput decides between the import and re-encode modes.
func isExcsvInput(path string) bool {
	if excsv.IsPackPath(path) || isRowZipPath(path) {
		return true
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 16)
	n, _ := file.Read(head)
	return strings.HasPrefix(strings.TrimPrefix(string(head[:n]), "\ufeff"), "#!excsv")
}

// importDocument builds a new ExCSV document from a delimited source.
func importDocument(cfg *config, f *convertFlags, path string) *excsv.Document {
	data, err := os.ReadFile(path)
	if err != nil {
		exitIOErr(err)
	}
	opts := excsv.ImportOptions{
		DelimName:    f.delim,
		QuoteName:    f.quote,
		NoHeader:     f.noHeader,
		Encoding:     f.encoding,
		Null:         f.null,
		NoChecksum:   f.noChecksum,
		Strict:       !cfg.lenient,
		Sidecar:      f.format == formatSidecar,
		Reference:    f.reference,
		SourcePath:   path,
		Aggregations: excsv.ExpandAggregationList(f.agg),
		Comments:     f.comments,
	}
	opts.FileMeta = parseKeyValueFlags(cfg, "--meta", f.meta)
	opts.SQL = parseKeyValueFlags(cfg, "--sql", f.sql)
	for _, spec := range f.columnAttrs {
		attr, err := excsv.ParseColumnAttr(spec)
		if err != nil {
			exitUserErr(err.Error())
		}
		opts.ColumnAttrs = append(opts.ColumnAttrs, attr)
	}
	res, err := excsv.ImportDelimited(data, opts)
	if err != nil {
		exitParseErr(err)
	}
	printWarnings(res.Warnings)
	return res.Doc
}

// reencodeDocument preserves every existing metadata line and regenerates
// nothing: only the requested dialect and enrichment are applied.
func reencodeDocument(cfg *config, f *convertFlags, path string) *excsv.Document {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	// Order matters: the header row switch reshapes the data section, and null
	// rewrites cells, so both run before anything derived is recomputed.
	if f.noHeader {
		applyHeaderField(doc, "header", "0")
	}
	if f.delim != "" {
		applyHeaderField(doc, "delim", f.delim)
	}
	if f.quote != "" {
		applyHeaderField(doc, "quote", f.quote)
	}
	if f.encoding != "" {
		applyHeaderField(doc, "encoding", f.encoding)
	}
	if f.null != "" {
		applyHeaderField(doc, "null", f.null)
	}
	if f.noChecksum {
		applyHeaderField(doc, "checksum", "")
	}
	for _, kv := range parseKeyValueFlags(cfg, "--meta", f.meta) {
		doc.SetFileMeta(kv.Key, kv.Value)
	}
	for _, kv := range parseKeyValueFlags(cfg, "--sql", f.sql) {
		if err := doc.SetSQL(kv.Key, kv.Value); err != nil {
			exitParseErr(err)
		}
	}
	for _, spec := range f.columnAttrs {
		attr, err := excsv.ParseColumnAttr(spec)
		if err != nil {
			exitUserErr(err.Error())
		}
		if err := doc.UpsertColumn(attr.Column, map[string]string{attr.Attr: attr.Value}); err != nil {
			exitParseErr(err)
		}
	}
	for _, name := range excsv.ExpandAggregationList(f.agg) {
		if _, err := doc.AddAggregation(name); err != nil {
			exitParseErr(err)
		}
	}
	for _, text := range f.comments {
		doc.AddHumanComment(text)
	}
	if f.format == formatSidecar {
		applySidecarShape(doc, f, path)
	}
	if err := doc.SyncDerived(); err != nil {
		exitParseErr(err)
	}
	return doc
}

// applySidecarShape detaches the data section into a sibling delimited file and
// points reference= at it.
func applySidecarShape(doc *excsv.Document, f *convertFlags, path string) {
	ref := f.reference
	if ref == "" {
		ext := ".csv"
		if doc.Header.DelimName == "tab" {
			ext = ".tsv"
		}
		ref = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ext
	}
	dataPath := filepath.Join(filepath.Dir(path), ref)
	if err := os.WriteFile(dataPath, []byte(delimitedData(doc)), 0o644); err != nil {
		exitIOErr(err)
	}
	if err := doc.SetDataChecksumFromSection(doc.SerializeDataSection(), "sha256"); err != nil {
		exitParseErr(err)
	}
	doc.Header.Fields["reference"] = ref
	doc.Source.Profile = excsv.ProfileSidecar
	doc.Source.Reference = ref
	doc.Data = excsv.DataSection{}
}

func applyHeaderField(doc *excsv.Document, key, value string) {
	if err := doc.SetHeaderField(key, value); err != nil {
		exitParseErr(err)
	}
}

func emitConverted(cfg *config, f *convertFlags, srcPath string, doc *excsv.Document) {
	serialized, err := doc.SerializeCanonical()
	if err != nil {
		exitIOErr(err)
	}
	dest := f.out
	switch f.format {
	case formatInline:
		// stdout by default; -o with the input path is the in-place spelling.

	case formatSidecar:
		if dest == "" {
			dest = defaultSidecarPath(srcPath)
		}

	case formatZip:
		entry := zipEntryName(srcPath)
		serialized, err = excsvzip.WrapWithPassword(serialized, entry, "", cfg.zipPassword)
		if err != nil {
			exitIOErr(err)
		}
		if dest == "" {
			dest = entry + ".zip"
		}

	case formatPack:
		name := f.table
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
			name = strings.ReplaceAll(name, ".", "_")
		}
		serialized, err = excsv.PackFromDocument(doc, name).Serialize()
		if err != nil {
			exitIOErr(err)
		}
		if dest == "" {
			dest = trimExcsvExt(srcPath) + ".excsv.pack.zip"
		}
	}

	if err := writeOutputBytes(dest, serialized); err != nil {
		exitIOErr(err)
	}
	if cfg.jsonOut {
		out := map[string]any{
			"ok": true, "rows": doc.DeclaredOrCountedRows(), "format": f.format,
			"profile": string(doc.Source.Profile),
		}
		if dest != "" {
			out["path"] = dest
		}
		if ref := doc.Header.Fields["reference"]; ref != "" {
			out["reference"] = ref
		}
		_ = writeJSON(out)
	}
}

func parseKeyValueFlags(cfg *config, flag string, specs []string) []excsv.KV {
	var out []excsv.KV
	for _, spec := range specs {
		key, val, ok := strings.Cut(spec, ":")
		if !ok {
			exitUserErr(fmt.Sprintf("invalid %s %q (expected KEY:VALUE)", flag, spec))
		}
		out = append(out, excsv.KV{Key: strings.TrimSpace(key), Value: strings.TrimSpace(val)})
	}
	return out
}

func zipEntryName(path string) string {
	base := filepath.Base(path)
	entry := strings.TrimSuffix(base, filepath.Ext(base))
	if !strings.HasSuffix(entry, ".excsv") && !strings.HasSuffix(entry, ".ecsv") && !strings.HasSuffix(entry, ".extsv") {
		entry += ".excsv"
	}
	return entry
}

func trimExcsvExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".excsv", ".ecsv", ".extsv", ".csv", ".tsv":
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
	return path
}

func defaultSidecarPath(input string) string {
	sideExt := ".excsv"
	if strings.EqualFold(filepath.Ext(input), ".tsv") {
		sideExt = ".extsv"
	}
	return trimExcsvExt(input) + sideExt
}
