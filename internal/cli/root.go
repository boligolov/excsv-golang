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

func newRoot() *cobra.Command {
	cfg := &config{}
	root := &cobra.Command{
		Use:   "excsv [flags] FILE <command>",
		Short: "CLI for ExCSV v0.2 (plain and zip)",
		Long: `Operate on an ExCSV document: put FILE first, then a command.

  excsv data.excsv validate
  excsv data.excsv meta set author --value "a@b"
  excsv data.csv convert -o data.excsv
  excsv version`,
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&cfg.strict, "strict", true, "fail on spec violations")
	root.PersistentFlags().BoolVar(&cfg.lenient, "lenient", false, "collect warnings and continue")
	root.PersistentFlags().BoolVar(&cfg.jsonOut, "json", false, "machine-readable output")
	root.PersistentFlags().BoolVar(&cfg.cleanHumanComments, "clean-human-comments", false, "drop ## comments on read/rewrite")
	root.PersistentFlags().StringVar(&cfg.expectProfile, "expect-profile", "", "validate as stub, sidecar, or sidecar_strict (fixture/testing)")
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		requireTargetFile(cmd)
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(
		newValidateCmd(cfg),
		newInfoCmd(cfg),
		newCatCmd(cfg),
		newStripCmd(cfg),
		newConvertCmd(cfg),
		newWrapCmd(cfg),
		newUnwrapCmd(cfg),
		newRowsCmd(cfg),
		newHeaderCmd(cfg),
		newMetaCmd(cfg),
		newSQLCmd(cfg),
		newAggCmd(cfg),
	)
	return root
}

// NewRoot returns the root command (for tests); prefer Execute() for file-first argv handling.
func NewRoot() *cobra.Command {
	return newRoot()
}

func loadDoc(cfg *config, path string, zipLoadData bool) (*excsv.ParseResult, error) {
	opts := cfg.parseOpts()
	opts.ZipLoadData = zipLoadData
	return excsv.ParseFile(path, opts)
}

func loadDocOnly(cfg *config, path string, zipLoadData bool) (*excsv.Document, error) {
	res, err := loadDoc(cfg, path, zipLoadData)
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
