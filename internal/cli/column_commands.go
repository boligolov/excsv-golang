package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

func newColumnCmd(cfg *config) *cobra.Command {
	var attrs []string
	cmd := &cobra.Command{Use: "column", Short: "Column schema (#column) operations", Aliases: []string{"col"}}
	set := &cobra.Command{
		Use:   "set NAME",
		Short: "Create or update #column NAME (repeatable --attr key=val)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runColumnSet(cfg, args[0], attrs)
		},
	}
	set.Flags().StringArrayVar(&attrs, "attr", nil, "attribute key=val (repeatable)")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List #column declarations",
			Run: func(cmd *cobra.Command, args []string) {
				runColumnList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [NAME]", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				path := targetPath()
				if len(args) == 0 {
					runColumnList(cfg, path)
					return
				}
				runColumnGet(cfg, args[0], path)
			},
		},
		set,
		// There is no `column check`: checking cell values against #column is
		// `validate --with-data [--column NAME]`, the single reporter.
		&cobra.Command{
			Use: "remove NAME", Short: "Remove a #column declaration", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runColumnRemove(cfg, args[0])
			},
		},
	)
	return cmd
}

func runColumnList(cfg *config, path string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	if cfg.jsonOut {
		var out []map[string]string
		for _, col := range doc.Meta.Columns {
			out = append(out, col.Attrs)
		}
		_ = writeJSON(out)
		return
	}
	for _, col := range doc.Meta.Columns {
		fmt.Printf("#column %s\n", excsv.FormatColumnAttrs(col.Attrs))
	}
}

func runColumnGet(cfg *config, name, path string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	for _, col := range doc.Meta.Columns {
		if col.Attrs["name"] == name {
			if cfg.jsonOut {
				_ = writeJSON(col.Attrs)
				return
			}
			fmt.Println(excsv.FormatColumnAttrs(col.Attrs))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "unknown column: %s\n", name)
	os.Exit(1)
}

func runColumnSet(cfg *config, name string, attrs []string) {
	parsed := map[string]string{}
	for _, a := range attrs {
		k, v, ok := strings.Cut(a, "=")
		if !ok {
			exitUserErr("invalid --attr " + a + " (expected key=val)")
		}
		parsed[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if err := doc.UpsertColumn(name, parsed); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}

func runColumnRemove(cfg *config, name string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveColumn(name) {
		fmt.Fprintf(os.Stderr, "unknown column: %s\n", name)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}
