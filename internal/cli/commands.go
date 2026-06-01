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
		Use:   "validate [file]",
		Short: "Check ExCSV conformance",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := fileArg(args)
			if _, err := loadDoc(cfg, path); err != nil {
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
		Use:   "info [file]",
		Short: "Compact summary",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDoc(cfg, fileArg(args))
			if err != nil {
				exitParseErr(err)
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]any{
					"version": doc.Header.Version, "delim": doc.Header.DelimName,
					"quote": doc.Header.QuoteName, "rows": doc.RowCount(),
					"columns": len(doc.Meta.Columns), "form": formName(doc.Form),
				})
				return
			}
			fmt.Printf("ExCSV %s  rows=%d  columns=%d  form=%s\n",
				doc.Header.Version, doc.RowCount(), len(doc.Meta.Columns), formName(doc.Form))
		},
	}
}

func newCatCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "cat [file]",
		Short: "Print canonical inner ExCSV document",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDoc(cfg, fileArg(args))
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

func newHeaderCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "header", Short: "Header line (#!excsv) operations"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list [file]", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				doc, err := loadDoc(cfg, fileArg(args))
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
			},
		},
		&cobra.Command{
			Use: "get KEY [file]", Args: cobra.RangeArgs(1, 2),
			Run: func(cmd *cobra.Command, args []string) {
				path := "-"
				if len(args) > 1 {
					path = args[1]
				}
				doc, err := loadDoc(cfg, path)
				if err != nil {
					exitParseErr(err)
				}
				v, ok := doc.Header.Fields[args[0]]
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown header key: %s\n", args[0])
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
		Use: "list [file]", Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDoc(cfg, fileArg(args))
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
		Use: "count [file]", Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDoc(cfg, fileArg(args))
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
		Use:   "clean [file]",
		Short: "Strip ExCSV metadata and print plain CSV/TSV data",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := loadDoc(cfg, fileArg(args))
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

func newConvertCmd(cfg *config) *cobra.Command {
	var out, delim, quote string
	var noHeader, addColumns, checksum, asZip bool
	var meta []string

	c := &cobra.Command{
		Use:   "convert [file]",
		Short: "Create ExCSV from CSV/TSV",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := fileArg(args)
			data, err := readInputBytes(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			opts := excsv.ImportOptions{
				DelimName:  delim,
				QuoteName:  quote,
				NoHeader:   noHeader,
				AddColumns: addColumns,
				Checksum:   checksum,
				Strict:     !cfg.lenient,
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
				entry := "data.excsv"
				if path != "-" {
					base := filepath.Base(path)
					entry = strings.TrimSuffix(base, filepath.Ext(base)) + ".excsv"
				}
				serialized, err = excsvzip.Wrap(serialized, entry, "")
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					os.Exit(3)
				}
			}
			if err := writeOutputBytes(out, serialized); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]any{
					"ok": true, "rows": res.Doc.RowCount(), "form": formName(res.Doc.Form),
				})
			}
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output path (default stdout)")
	c.Flags().StringVar(&delim, "delim", "", "delimiter: comma, tab, pipe, semicolon, or single character")
	c.Flags().StringVar(&quote, "quote", "", "quote style: none, double, single, or single character")
	c.Flags().BoolVar(&noHeader, "no-header", false, "treat all rows as data (header=0)")
	c.Flags().BoolVar(&addColumns, "columns", false, "emit #column name=X type=text from header row")
	c.Flags().BoolVar(&checksum, "checksum", false, "set checksum=sha256:... on output")
	c.Flags().StringArrayVar(&meta, "meta", nil, "add #@ metadata (KEY:VAL, repeatable)")
	c.Flags().BoolVar(&asZip, "zip", false, "wrap output as .excsv.zip")
	return c
}

func readInputBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func writeOutputBytes(path string, data []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
