package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

// newHeaderCmd exposes #!excsv read-only.
//
// The header line is the one place where a piecemeal edit can quietly break the
// document: change delim and the data section no longer parses, change rows or
// checksum and the file now lies about itself. So every field is written by
// whoever owns what it governs — convert for the encoding, fix for the derived
// counts, sql dialect set for sql-dialect= — and this group has no write verbs.
func newHeaderCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "header", Short: "Header line (#!excsv) — read-only"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List header fields", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runHeaderList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [KEY]", Short: "Print one header field", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				path := targetPath()
				if len(args) == 0 {
					runHeaderList(cfg, path)
					return
				}
				runHeaderGet(cfg, args[0], path)
			},
		},
		&cobra.Command{
			Use: "rows", Short: "Print the row count as a bare number", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runHeaderRows(cfg)
			},
		},
	)
	return cmd
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
	keys := make([]string, 0, len(doc.Header.Fields))
	for k := range doc.Header.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, doc.Header.Fields[k])
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

// runHeaderRows prints digits and nothing else, so $(excsv f.excsv header rows)
// is usable. rows= is MAY in the spec, so an absent field falls back to the
// counted rows rather than failing; reporting a disagreement between the two is
// validate --with-data's job.
func runHeaderRows(cfg *config) {
	doc, err := loadTableDoc(cfg, targetPath(), true)
	if err != nil {
		exitParseErr(err)
	}
	fmt.Println(doc.DeclaredOrCountedRows())
}

func newMetaCmd(cfg *config) *cobra.Command {
	var value string
	cmd := &cobra.Command{Use: "meta", Short: "File metadata (#@) operations"}
	set := &cobra.Command{
		Use:   "set KEY",
		Short: "Set #@KEY (requires --value)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if !cmd.Flags().Changed("value") {
				exitUserErr("--value is required")
			}
			runMetaSet(cfg, args[0], value)
		},
	}
	set.Flags().StringVar(&value, "value", "", "metadata value (use shell quotes for spaces)")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List #@ entries", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runMetaList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [KEY]", Short: "Print one #@ value", Args: cobra.MaximumNArgs(1),
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
			Use: "remove KEY", Short: "Remove a #@ entry", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runMetaRemove(cfg, args[0])
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

func runMetaSet(cfg *config, key, value string) {
	path := targetPath()
	doc, err := loadPackScopedDoc(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	doc.SetFileMeta(key, value)
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func runMetaRemove(cfg *config, key string) {
	path := targetPath()
	doc, err := loadPackScopedDoc(cfg, path)
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
