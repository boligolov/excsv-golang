package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

type config struct {
	strict             bool
	lenient            bool
	jsonOut            bool
	cleanHumanComments bool
	expectProfile      string
	zipPassword        string
	table              string
	pack               *excsv.Pack
	packTable          *excsv.PackTable
}

func (c config) parseOpts() excsv.ParseOptions {
	opts := excsv.StrictOptions()
	if c.lenient {
		opts.Strict = false
	}
	opts.ClearHumanComments = c.cleanHumanComments
	opts.ExpectProfile = c.expectProfile
	opts.ZipPassword = c.zipPassword
	return opts
}

func newRoot() *cobra.Command {
	cfg := &config{}
	root := &cobra.Command{
		Use:   "excsv [flags] FILE <group> <command>",
		Short: "CLI for ExCSV v0.4 (plain, sidecar, row ZIP, pack)",
		Long: `Operate on an ExCSV document: put FILE first, then a command.

  excsv data.csv convert -o data.excsv
  excsv data.excsv validate --with-data
  excsv data.excsv fix --only format
  excsv data.excsv data print -o data.csv
  excsv data.excsv export json -o data.excsv.json
  excsv version`,
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&cfg.strict, "strict", true, "findings make the command exit 2")
	root.PersistentFlags().BoolVar(&cfg.lenient, "lenient", false, "print the same findings but exit 0")
	root.PersistentFlags().BoolVar(&cfg.jsonOut, "json", false, "machine-readable output")
	root.PersistentFlags().BoolVar(&cfg.cleanHumanComments, "clean-human-comments", false, "drop ## comments on read/rewrite")
	root.PersistentFlags().StringVar(&cfg.expectProfile, "expect-profile", "", "validate as stub, sidecar, or sidecar_strict (fixture/testing)")
	root.PersistentFlags().StringVar(&cfg.zipPassword, "zip-password", "", "password for encrypted ZIP or pack")
	root.PersistentFlags().StringVar(&cfg.table, "table", "", "pack table name")
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		requireTargetFile(cmd)
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(
		// Whole-document lifecycle stays flat: these are the most-typed commands.
		newInfoCmd(cfg),
		newValidateCmd(cfg),
		newFixCmd(cfg),
		newConvertCmd(cfg),
		// Noun groups, one per meta line the format defines.
		newDataCmd(cfg),
		newHeaderCmd(cfg),
		newMetaCmd(cfg),
		newColumnCmd(cfg),
		newAggCmd(cfg),
		newSQLCmd(cfg),
		newCommentCmd(cfg),
		newExportCmd(cfg),
		newPackCmd(cfg),
		newZipCmd(cfg),
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
	res, err := excsv.ParseFile(path, opts)
	if err != nil {
		return nil, err
	}
	cfg.pack = nil
	cfg.packTable = nil
	if res != nil && res.Pack != nil {
		cfg.pack = res.Pack
		if cfg.table != "" {
			t, err := res.Pack.Table(cfg.table)
			if err != nil {
				return nil, err
			}
			cfg.packTable = t
		}
	}
	return res, nil
}

func loadDocOnly(cfg *config, path string, zipLoadData bool) (*excsv.Document, error) {
	res, err := loadDoc(cfg, path, zipLoadData)
	if err != nil {
		return nil, err
	}
	printWarnings(res.Warnings)
	if cfg.packTable != nil {
		return cfg.packTable.Document(), nil
	}
	return res.Doc, nil
}

func loadTableDoc(cfg *config, path string, zipLoadData bool) (*excsv.Document, error) {
	res, err := loadDoc(cfg, path, zipLoadData)
	if err != nil {
		return nil, err
	}
	printWarnings(res.Warnings)
	if res.Pack == nil {
		return res.Doc, nil
	}
	if cfg.packTable != nil {
		return cfg.packTable.Document(), nil
	}
	if t := res.Pack.DefaultTable(); t != nil {
		cfg.packTable = t
		return t.Document(), nil
	}
	exitUserErr("--table is required for multi-table packs")
	return nil, nil
}

func printWarnings(warnings []excsv.Issue) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w.Error())
	}
}

func printPackInfo(cfg *config, res *excsv.ParseResult, noMeta bool) {
	p := res.Pack
	names := make([]string, 0, len(p.Tables))
	for _, t := range p.Tables {
		names = append(names, t.Decl.Name)
	}
	extras := collectInfoExtras(res.Doc, noMeta)
	if cfg.jsonOut {
		out := map[string]any{
			"version": res.Doc.Header.Version,
			"form":    "pack",
			"layout":  res.Doc.Header.Fields["layout"],
			"tables":  names,
			"fk":      len(p.FKs),
		}
		applyInfoExtrasJSON(out, extras)
		_ = writeJSON(out)
		return
	}
	fmt.Printf("ExCSV %s\n", res.Doc.Header.Version)
	fmt.Printf("Form: pack\n")
	fmt.Printf("Tables: %d (%s)\n", len(p.Tables), strings.Join(names, ", "))
	if len(p.FKs) > 0 {
		fmt.Printf("Foreign keys: %d\n", len(p.FKs))
	}
	printInfoExtrasText(extras)
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

func exitIOErr(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(3)
}

func formName(f excsv.Form) string {
	switch f {
	case excsv.FormZipInner:
		return "zip"
	case excsv.FormPack:
		return "pack"
	default:
		return "plain"
	}
}

func writeOutputBytes(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
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

func mapZipCLIError(err error) error {
	return excsv.MapZipError(err)
}

// stderrIsTerminal keeps advisory notices out of CI logs.
func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
