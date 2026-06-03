package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
	"github.com/spf13/cobra"
)

func newValidateCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "validate FILE",
		Short: "Check ExCSV conformance",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			if _, err := loadDocOnly(cfg, path); err != nil {
				exitParseErr(err)
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]any{"ok": true, "path": path})
				return
			}
			fmt.Println("ok")
		},
	}
}

func newInfoCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "info FILE",
		Short: "Compact summary",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			res, err := loadDoc(cfg, args[0])
			if err != nil {
				exitParseErr(err)
			}
			doc := res.Doc
			for _, w := range res.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w.Error())
			}
			if cfg.jsonOut {
				out := map[string]any{
					"version": doc.Header.Version, "delim": doc.Header.DelimName,
					"quote": doc.Header.QuoteName, "rows": doc.RowCount(),
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
				doc.Header.Version, doc.RowCount(), len(doc.Meta.Columns), formName(doc.Form), doc.Source.Profile)
			if doc.Source.Reference != "" {
				line += fmt.Sprintf("  reference=%s", doc.Source.Reference)
			}
			fmt.Println(line)
		},
	}
}

func newCatCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "cat FILE",
		Short: "Print canonical inner ExCSV document",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDocOnly(cfg, args[0])
			if err != nil {
				exitParseErr(err)
			}
			b, err := doc.SerializeCanonical()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			_, _ = io.WriteString(os.Stdout, string(b))
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("excsv-cli %s (built %s)\n", Version, BuildTime)
		},
	}
}

func runHeaderList(cfg *config, path string) {
	doc, err := loadDocOnly(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if cfg.jsonOut {
		_ = writeJSON(doc.Header.Fields)
		return
	}
	for k, v := range doc.Header.Fields {
		fmt.Printf("%s=%s\n", k, v)
	}
}

func headerGetPath(args []string) (key, path string, listOnly bool) {
	if len(args) == 0 {
		exitUserErr("file required")
		return "", "", false
	}
	if len(args) == 1 {
		if headerArgLooksLikeFile(args[0]) {
			return "", args[0], true
		}
		exitUserErr("file required")
		return "", "", false
	}
	return args[0], args[1], false
}

func headerArgLooksLikeFile(arg string) bool {
	ext := strings.ToLower(filepath.Ext(arg))
	if ext == ".excsv" || ext == ".ecsv" || ext == ".extsv" || ext == ".csv" || ext == ".tsv" || strings.HasSuffix(strings.ToLower(arg), ".zip") {
		return true
	}
	if _, err := os.Stat(arg); err == nil {
		return true
	}
	return false
}

func newHeaderCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "header", Short: "Header line (#!excsv) operations"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list FILE", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runHeaderList(cfg, args[0])
			},
		},
		&cobra.Command{
			Use: "get [KEY] FILE", Args: cobra.RangeArgs(1, 2),
			Run: func(cmd *cobra.Command, args []string) {
				key, path, listOnly := headerGetPath(args)
				if listOnly {
					runHeaderList(cfg, path)
					return
				}
				doc, err := loadDocOnly(cfg, path)
				if err != nil {
					exitParseErr(err)
				}
				v, ok := doc.Header.Fields[key]
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown header key: %s\n", key)
					os.Exit(1)
				}
				fmt.Println(v)
			},
		},
	)
	return cmd
}

func newMetaCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "meta", Short: "File metadata (#@) operations"}
	cmd.AddCommand(&cobra.Command{
		Use: "list FILE", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDocOnly(cfg, args[0])
			if err != nil {
				exitParseErr(err)
			}
			if cfg.jsonOut {
				_ = writeJSON(doc.MetaMap())
				return
			}
			for _, kv := range doc.Meta.FileMeta {
				fmt.Printf("%s: %s\n", kv.Key, kv.Value)
			}
		},
	})
	return cmd
}

func newRowsCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "rows", Short: "Data row operations"}
	cmd.AddCommand(&cobra.Command{
		Use: "count FILE", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDocOnly(cfg, args[0])
			if err != nil {
				exitParseErr(err)
			}
			n := doc.RowCount()
			if cfg.jsonOut {
				_ = writeJSON(map[string]int{"rows": n})
				return
			}
			fmt.Println(n)
		},
	})
	return cmd
}

func newCleanCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "clean FILE",
		Short: "Strip ExCSV metadata and print plain CSV/TSV data",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			if isSidecarInputPath(path) {
				opts := cfg.parseOpts()
				opts.ResolveReference = false
				res, err := excsv.ParseFile(path, opts)
				if err != nil {
					exitParseErr(err)
				}
				if excsv.IsSidecarMetaOnly(res.Doc) {
					printSidecarCleanNotice(res.Doc, path)
					return
				}
			}
			doc, err := loadDocOnly(cfg, path)
			if err != nil {
				exitParseErr(err)
			}
			d := doc.Header.Dialect()
			if doc.Data.HasHeaderRow {
				fmt.Println(excsv.JoinCSVFields(doc.Data.HeaderRow, d))
			}
			for _, row := range doc.Data.Rows {
				fmt.Println(excsv.JoinCSVFields(row, d))
			}
		},
	}
}

func isSidecarInputPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".excsv" || ext == ".ecsv" || ext == ".extsv"
}

func printSidecarCleanNotice(doc *excsv.Document, _ string) {
	ref := doc.Header.Fields["reference"]
	if ref == "" {
		ref = doc.Source.Reference
	}
	msg := "Hey — that's a sidecar file (metadata only"
	if ref != "" {
		msg += "; data is in " + ref
	}
	msg += "). clean does nothing useful here. If you don't need it, delete it."
	fmt.Fprintln(os.Stderr, msg)
}

func newConvertCmd(cfg *config) *cobra.Command {
	var out, delim, quote, reference string
	var noHeader, addColumns, checksum, asZip, sidecar bool
	var meta []string

	c := &cobra.Command{
		Use:   "convert FILE",
		Short: "Create ExCSV from CSV/TSV (inline or sidecar)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if sidecar && asZip {
				exitUserErr("--sidecar cannot be combined with --zip")
			}
			opts := excsv.ImportOptions{
				DelimName:  delim,
				QuoteName:  quote,
				NoHeader:   noHeader,
				AddColumns: addColumns,
				Checksum:   checksum,
				Strict:     !cfg.lenient,
				Sidecar:    sidecar,
				Reference:  reference,
				SourcePath: path,
			}
			for _, m := range meta {
				key, val, ok := strings.Cut(m, ":")
				if !ok {
					fmt.Fprintf(os.Stderr, "invalid --meta %q (expected KEY:VAL)\n", m)
					os.Exit(1)
				}
				key = strings.TrimSpace(key)
				val = strings.TrimSpace(val)
				opts.FileMeta = append(opts.FileMeta, excsv.KV{Key: key, Value: val})
			}
			res, err := excsv.ImportDelimited(data, opts)
			if err != nil {
				exitParseErr(err)
			}
			for _, w := range res.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w.Error())
			}
			serialized, err := res.Doc.SerializeCanonical()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if asZip {
				base := filepath.Base(path)
				entry := strings.TrimSuffix(base, filepath.Ext(base))
				if !strings.HasSuffix(entry, ".excsv") && !strings.HasSuffix(entry, ".ecsv") {
					entry += ".excsv"
				}
				serialized, err = excsvzip.Wrap(serialized, entry, "")
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					os.Exit(3)
				}
			}
			dest := out
			if dest == "" && sidecar {
				dest = defaultSidecarPath(path)
			}
			if err := writeOutputBytes(dest, serialized); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if cfg.jsonOut {
				rows := res.Doc.RowCount()
				if res.Doc.Header.Rows != nil {
					rows = *res.Doc.Header.Rows
				}
				outJSON := map[string]any{
					"ok": true, "rows": rows, "form": formName(res.Doc.Form),
					"profile": string(res.Doc.Source.Profile),
				}
				if res.Doc.Source.Reference != "" {
					outJSON["reference"] = res.Doc.Source.Reference
				}
				_ = writeJSON(outJSON)
			}
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output path (default stdout; with --sidecar: default <basename>.excsv or .extsv)")
	c.Flags().StringVar(&delim, "delim", "", "output delimiter in #!excsv and inline data (input is auto-detected; comma, tab, pipe, semicolon, or one character)")
	c.Flags().StringVar(&quote, "quote", "", "output quoting in #!excsv and inline data (none, double, single, or one character; doubles quotes inside values)")
	c.Flags().BoolVar(&sidecar, "sidecar", false, "emit metadata-only sidecar with reference= pointing at FILE (data file unchanged)")
	c.Flags().StringVar(&reference, "reference", "", "sidecar reference= path (default: basename of FILE)")
	c.Flags().BoolVar(&noHeader, "no-header", false, "treat all rows as data (header=0)")
	c.Flags().BoolVar(&addColumns, "columns", false, "emit #column name=X type=text from header row")
	c.Flags().BoolVar(&checksum, "checksum", false, "set checksum=sha256:... on output")
	c.Flags().StringArrayVar(&meta, "meta", nil, "add #@ metadata (KEY:VAL, repeatable)")
	c.Flags().BoolVar(&asZip, "zip", false, "wrap output as .excsv.zip")
	return c
}

func writeOutputBytes(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func newSQLCmd(cfg *config) *cobra.Command {
	var verb, dialect string
	cmd := &cobra.Command{Use: "sql", Short: "SQL companion (#$) operations"}
	list := &cobra.Command{
		Use:   "list FILE",
		Short: "List #$ddl / #$dql statements",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDocOnly(cfg, args[0])
			if err != nil {
				exitParseErr(err)
			}
			var out []map[string]string
			for _, s := range doc.Meta.SQL {
				if verb != "" && s.Verb != verb {
					continue
				}
				eff := excsv.EffectiveDialect(s, doc.Header.SQLDialect)
				if dialect != "" && !dialectMatches(eff, dialect) {
					continue
				}
				entry := map[string]string{
					"verb": s.Verb, "dialect": eff, "key": s.RawKey, "sql": s.Payload,
				}
				out = append(out, entry)
				if !cfg.jsonOut {
					fmt.Printf("#$%s [%s]: %s\n", s.RawKey, eff, s.Payload)
				}
			}
			if cfg.jsonOut {
				_ = writeJSON(out)
			}
		},
	}
	list.Flags().StringVar(&verb, "verb", "", "filter: ddl or dql")
	list.Flags().StringVar(&dialect, "dialect", "", "filter by effective dialect (exact or family)")
	cmd.AddCommand(list)
	return cmd
}

func defaultSidecarPath(input string) string {
	ext := strings.ToLower(filepath.Ext(input))
	sideExt := ".excsv"
	if ext == ".tsv" {
		sideExt = ".extsv"
	}
	return strings.TrimSuffix(input, filepath.Ext(input)) + sideExt
}

func dialectMatches(effective, target string) bool {
	if effective == target {
		return true
	}
	eBase, _, _ := strings.Cut(effective, "-")
	tBase, _, _ := strings.Cut(target, "-")
	return eBase != "" && eBase == tBase
}

func newZipCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "zip", Short: "Row ZIP container operations"}
	var out string
	wrap := &cobra.Command{
		Use: "wrap INPUT", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			base := filepath.Base(args[0])
			entry := strings.TrimSuffix(base, filepath.Ext(base))
			if !strings.HasSuffix(entry, ".excsv") && !strings.HasSuffix(entry, ".ecsv") {
				entry += ".excsv"
			}
			if out == "" {
				out = entry + ".zip"
			}
			zipped, err := excsvzip.Wrap(data, entry, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if err := os.WriteFile(out, zipped, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
		},
	}
	wrap.Flags().StringVarP(&out, "output", "o", "", "output zip path")
	unwrap := &cobra.Command{
		Use: "unwrap INPUT.zip", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			ext, err := excsvzip.Extract(args[0], data)
			if err != nil {
				exitParseErr(excsv.MapZipError(err))
			}
			dest := out
			if dest == "" {
				dest = strings.TrimSuffix(filepath.Base(args[0]), ".zip")
			}
			if err := os.WriteFile(dest, ext.Inner, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
		},
	}
	unwrap.Flags().StringVarP(&out, "output", "o", "", "output plain path")
	peek := &cobra.Command{
		Use: "peek INPUT.zip", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			ext, err := excsvzip.Extract(args[0], data)
			if err != nil {
				exitParseErr(excsv.MapZipError(err))
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]string{"comment": ext.Comment})
				return
			}
			fmt.Println(ext.Comment)
		},
	}
	cmd.AddCommand(wrap, unwrap, peek)
	return cmd
}
