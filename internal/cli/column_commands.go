package cli

import (
	"fmt"
	"os"
	"path/filepath"
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

	var materializeOut string
	materialize := &cobra.Command{
		Use:   "materialize NAME",
		Short: "Write a computed column's formula= output into the data (sets materialized=1)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runColumnMaterialize(cfg, args[0], materializeOut)
		},
	}
	materialize.Flags().StringVarP(&materializeOut, "output", "o", "",
		"output path; required for a sidecar (materialize always writes a new inline file there)")

	var dematerializeOut string
	dematerialize := &cobra.Command{
		Use:   "dematerialize NAME",
		Short: "Drop a materialized computed column's cached data (keeps formula=)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runColumnDematerialize(cfg, args[0], dematerializeOut)
		},
	}
	dematerialize.Flags().StringVarP(&dematerializeOut, "output", "o", "",
		"output path; required for a sidecar (dematerialize always writes a new inline file there)")

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
		materialize,
		dematerialize,
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

// runColumnMaterialize and runColumnDematerialize share one container rule:
// plain, row-ZIP, and pack all rewrite FILE (or -o) in place through the
// normal save path — MaterializeColumn/DematerializeColumn only touch
// doc.Data/doc.Meta, and saveDocument already resyncs the container-specific
// bytes (zip re-wrap, pack .col files) from that. A sidecar is the one
// exception the spec calls out: the reference is never rewritten, so
// materializing/dematerializing one instead writes a brand-new inline file.
func runColumnMaterialize(cfg *config, name, out string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if doc.Source.Profile == excsv.ProfileSidecar {
		writeMaterializedSidecarCopy(cfg, doc, path, out, name, doc.MaterializeColumn)
		return
	}
	if err := doc.MaterializeColumn(name); err != nil {
		exitParseErr(err)
	}
	if err := saveDocumentTo(cfg, doc, path, out); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}

func runColumnDematerialize(cfg *config, name, out string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if doc.Source.Profile == excsv.ProfileSidecar {
		writeMaterializedSidecarCopy(cfg, doc, path, out, name, doc.DematerializeColumn)
		return
	}
	if err := doc.DematerializeColumn(name); err != nil {
		exitParseErr(err)
	}
	if err := saveDocumentTo(cfg, doc, path, out); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}

// writeMaterializedSidecarCopy applies mutate to doc (already loaded with its
// reference resolved) and writes the result as a brand-new inline file: the
// sidecar at path, and the file it references, are never touched.
func writeMaterializedSidecarCopy(cfg *config, doc *excsv.Document, path, out, name string, mutate func(string) error) {
	if out == "" {
		out = defaultSidecarMaterializeOutput(path)
	}
	if out == path || (doc.Source.ReferencePath != "" && out == doc.Source.ReferencePath) {
		exitUserErr("-o must name a new file, distinct from the sidecar and its reference")
	}
	if err := mutate(name); err != nil {
		exitParseErr(err)
	}
	if err := doc.SetHeaderField("reference", ""); err != nil {
		exitParseErr(err)
	}
	// The output is now a self-contained inline file, not a sidecar:
	// SerializeCanonical only emits the data section once both the
	// reference= field AND the in-memory sidecar profile say so.
	doc.Source.Profile = excsv.ProfileInline
	doc.Source.Reference = ""
	doc.Source.ReferencePath = ""
	doc.Source.SidecarPath = ""
	serialized, err := doc.SerializeCanonical()
	if err != nil {
		exitParseErr(err)
	}
	if err := os.WriteFile(out, serialized, 0o644); err != nil {
		exitIOErr(err)
	}
	printMutationOK(cfg, out, name, nil)
}

func defaultSidecarMaterializeOutput(sidecarPath string) string {
	ext := filepath.Ext(sidecarPath)
	stem := strings.TrimSuffix(sidecarPath, ext)
	return stem + ".materialized" + ext
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
