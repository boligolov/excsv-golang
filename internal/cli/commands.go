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
		Use:   "validate",
		Short: "Check ExCSV conformance",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if _, err := loadDocOnly(cfg, path, true); err != nil {
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
		Use:   "info",
		Short: "Compact summary",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			res, err := loadDoc(cfg, path, false)
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

func newCatCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "cat",
		Short: "Print canonical inner ExCSV document",
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDocOnly(cfg, targetPath(), true)
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
	doc, err := loadDocOnly(cfg, path, false)
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

func runHeaderGet(cfg *config, key, path string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	v, ok := doc.Header.Fields[key]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown header key: %s\n", key)
		os.Exit(1)
	}
	fmt.Println(v)
}

func newHeaderCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "header", Short: "Header line (#!excsv) operations"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List header fields",
			Run: func(cmd *cobra.Command, args []string) {
				runHeaderList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [KEY]", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				path := targetPath()
				if len(args) == 0 {
					runHeaderList(cfg, path)
					return
				}
				runHeaderGet(cfg, args[0], path)
			},
		},
	)
	return cmd
}

func runMetaList(cfg *config, path string) {
	doc, err := loadDocOnly(cfg, path, false)
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
}

func runMetaGet(cfg *config, key, path string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	v, ok := doc.MetaMap()[key]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown meta key: %s\n", key)
		os.Exit(1)
	}
	fmt.Println(v)
}

func newMetaCmd(cfg *config) *cobra.Command {
	var value string
	cmd := &cobra.Command{Use: "meta", Short: "File metadata (#@) operations"}
	set := &cobra.Command{
		Use:   "set KEY",
		Short: "Set #@KEY (requires --value)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if value == "" {
				exitUserErr("--value is required")
			}
			runMetaSet(cfg, args[0], value)
		},
	}
	set.Flags().StringVar(&value, "value", "", "metadata value (use shell quotes for spaces)")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Run: func(cmd *cobra.Command, args []string) {
				runMetaList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [KEY]", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				path := targetPath()
				if len(args) == 0 {
					runMetaList(cfg, path)
					return
				}
				runMetaGet(cfg, args[0], path)
			},
		},
		set,
		&cobra.Command{
			Use: "remove KEY", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runMetaRemove(cfg, args[0])
			},
		},
	)
	return cmd
}

func runMetaRemove(cfg *config, key string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveFileMeta(key) {
		fmt.Fprintf(os.Stderr, "unknown meta key: %s\n", key)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func runMetaSet(cfg *config, key, value string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	doc.SetFileMeta(key, value)
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func newRowsCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "rows",
		Short: "Print rows= from header (alias for header get rows)",
		Run: func(cmd *cobra.Command, args []string) {
			runHeaderGet(cfg, "rows", targetPath())
		},
	}
}

func newStripCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "strip",
		Short: "Remove ExCSV metadata and print plain CSV/TSV data",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if isSidecarInputPath(path) {
				opts := cfg.parseOpts()
				opts.ResolveReference = false
				res, err := excsv.ParseFile(path, opts)
				if err != nil {
					exitParseErr(err)
				}
				if excsv.IsSidecarMetaOnly(res.Doc) {
					printSidecarStripNotice(res.Doc, path)
					return
				}
			}
			doc, err := loadDocOnly(cfg, path, true)
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

func printSidecarStripNotice(doc *excsv.Document, _ string) {
	ref := doc.Header.Fields["reference"]
	if ref == "" {
		ref = doc.Source.Reference
	}
	msg := "Hey — that's a sidecar file (metadata only"
	if ref != "" {
		msg += "; data is in " + ref
	}
	msg += "). strip does nothing useful here. If you don't need it, delete it."
	fmt.Fprintln(os.Stderr, msg)
}

func newConvertCmd(cfg *config) *cobra.Command {
	var out, delim, quote, reference string
	var noHeader, addColumns, checksum, asZip, sidecar bool
	var meta []string

	c := &cobra.Command{
		Use:   "convert",
		Short: "Create ExCSV from CSV/TSV (inline or sidecar)",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
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

func runSQLList(cfg *config, path, verb, dialect string) {
	doc, err := loadDocOnly(cfg, path, false)
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
}

func newSQLCmd(cfg *config) *cobra.Command {
	var verb, dialect, value string
	cmd := &cobra.Command{Use: "sql", Short: "SQL companion (#$) operations"}
	list := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			runSQLList(cfg, targetPath(), verb, dialect)
		},
	}
	list.Flags().StringVar(&verb, "verb", "", "filter: ddl or dql")
	list.Flags().StringVar(&dialect, "dialect", "", "filter by effective dialect (exact or family)")
	get := &cobra.Command{
		Use: "get [KEY]", Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if len(args) == 0 {
				runSQLList(cfg, path, verb, dialect)
				return
			}
			runSQLGet(cfg, args[0], path, verb, dialect)
		},
	}
	get.Flags().StringVar(&verb, "verb", "", "filter: ddl or dql")
	get.Flags().StringVar(&dialect, "dialect", "", "filter by effective dialect (exact or family)")
	set := &cobra.Command{
		Use:   "set KEY",
		Short: "Set #$KEY payload (requires --value)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if value == "" {
				exitUserErr("--value is required")
			}
			runSQLSet(cfg, args[0], value)
		},
	}
	set.Flags().StringVar(&value, "value", "", "SQL payload (use shell quotes for spaces)")
	cmd.AddCommand(list, get, set, &cobra.Command{
		Use: "remove KEY", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runSQLRemove(cfg, args[0])
		},
	})
	return cmd
}

func runSQLRemove(cfg *config, key string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveSQL(key) {
		fmt.Fprintf(os.Stderr, "unknown sql key: %s\n", key)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func runSQLSet(cfg *config, key, value string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if err := doc.SetSQL(key, value); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func runSQLGet(cfg *config, key, path, verb, dialect string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	var matches []excsv.SQLStatement
	for _, s := range doc.Meta.SQL {
		if s.RawKey != key {
			continue
		}
		if verb != "" && s.Verb != verb {
			continue
		}
		eff := excsv.EffectiveDialect(s, doc.Header.SQLDialect)
		if dialect != "" && !dialectMatches(eff, dialect) {
			continue
		}
		matches = append(matches, s)
	}
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "unknown sql key: %s\n", key)
		os.Exit(1)
	}
	if len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "ambiguous sql key: %s\n", key)
		os.Exit(1)
	}
	fmt.Println(matches[0].Payload)
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

func newWrapCmd(cfg *config) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "wrap",
		Short: "Wrap plain ExCSV as row ZIP",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if isRowZipPath(path) {
				exitUserErr("wrap requires a plain .excsv or .ecsv file, not a zip")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			base := filepath.Base(path)
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
	c.Flags().StringVarP(&out, "output", "o", "", "output zip path")
	return c
}

func newUnwrapCmd(cfg *config) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "unwrap",
		Short: "Extract inner plain ExCSV from row ZIP",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if !isRowZipPath(path) {
				exitUserErr("unwrap requires a .excsv.zip or .ecsv.zip file")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			ext, err := excsvzip.Extract(path, data)
			if err != nil {
				exitParseErr(excsv.MapZipError(err))
			}
			dest := out
			if dest == "" {
				dest = strings.TrimSuffix(filepath.Base(path), ".zip")
			}
			if err := os.WriteFile(dest, ext.Inner, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output plain path")
	return c
}

func printMutationOK(cfg *config, path, key string, extra map[string]any) {
	if cfg.jsonOut {
		out := map[string]any{"ok": true, "path": path}
		if key != "" {
			out["key"] = key
		}
		for k, v := range extra {
			out[k] = v
		}
		_ = writeJSON(out)
		return
	}
	fmt.Println("ok")
}
