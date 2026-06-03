package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

type config struct {
	strict             bool
	lenient            bool
	jsonOut            bool
	cleanHumanComments bool
	expectProfile      string
}

func (c config) parseOpts() excsv.ParseOptions {
	opts := excsv.StrictOptions()
	if c.lenient {
		opts.Strict = false
	}
	opts.ClearHumanComments = c.cleanHumanComments
	opts.ExpectProfile = c.expectProfile
	return opts
}

func NewRoot() *cobra.Command {
	cfg := &config{}
	root := &cobra.Command{
		Use:          "excsv",
		Short:        "CLI for ExCSV v0.2 (plain and zip)",
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&cfg.strict, "strict", true, "fail on spec violations")
	root.PersistentFlags().BoolVar(&cfg.lenient, "lenient", false, "collect warnings and continue")
	root.PersistentFlags().BoolVar(&cfg.jsonOut, "json", false, "machine-readable output")
	root.PersistentFlags().BoolVar(&cfg.cleanHumanComments, "clean-human-comments", false, "drop ## comments on read/rewrite")
	root.PersistentFlags().StringVar(&cfg.expectProfile, "expect-profile", "", "validate as stub, sidecar, or sidecar_strict (fixture/testing)")

	root.AddCommand(newValidateCmd(cfg))
	root.AddCommand(newInfoCmd(cfg))
	root.AddCommand(newCatCmd(cfg))
	root.AddCommand(newHeaderCmd(cfg))
	root.AddCommand(newMetaCmd(cfg))
	root.AddCommand(newRowsCmd(cfg))
	root.AddCommand(newCleanCmd(cfg))
	root.AddCommand(newConvertCmd(cfg))
	root.AddCommand(newSQLCmd(cfg))
	root.AddCommand(newZipCmd(cfg))
	root.AddCommand(newVersionCmd())
	return root
}

func loadDoc(cfg *config, path string) (*excsv.ParseResult, error) {
	return excsv.ParseFile(path, cfg.parseOpts())
}

func loadDocOnly(cfg *config, path string) (*excsv.Document, error) {
	res, err := loadDoc(cfg, path)
	if err != nil {
		return nil, err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w.Error())
	}
	return res.Doc, nil
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func exitParseErr(err error) {
	if pe, ok := err.(*excsv.ParseError); ok {
		fmt.Fprintf(os.Stderr, "%s\n", pe.Error())
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(3)
}

func exitUserErr(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

func formName(f excsv.Form) string {
	if f == excsv.FormZipInner {
		return "zip"
	}
	return "plain"
}
