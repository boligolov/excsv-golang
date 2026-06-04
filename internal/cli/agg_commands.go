package cli

import (
	"fmt"
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

func newAggCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "agg", Short: "Aggregation (#%) operations"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list",
			Run: func(cmd *cobra.Command, args []string) {
				runAggList(cfg, targetPath())
			},
		},
		&cobra.Command{
			Use: "get [NAME]", Args: cobra.MaximumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				path := targetPath()
				if len(args) == 0 {
					runAggList(cfg, path)
					return
				}
				runAggGet(cfg, args[0], path)
			},
		},
		&cobra.Command{
			Use: "add NAME", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runAggAdd(cfg, args[0])
			},
		},
		&cobra.Command{
			Use: "update NAME", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runAggUpdate(cfg, args[0])
			},
		},
		&cobra.Command{
			Use: "remove NAME", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runAggRemove(cfg, args[0])
			},
		},
	)
	return cmd
}

func runAggList(cfg *config, path string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	if cfg.jsonOut {
		var out []map[string]any
		for _, a := range doc.Meta.Aggregations {
			out = append(out, map[string]any{"name": a.Name, "values": a.Values})
		}
		_ = writeJSON(out)
		return
	}
	d := doc.Header.Dialect()
	for _, a := range doc.Meta.Aggregations {
		fmt.Printf("#%%%s: %s\n", a.Name, excsv.JoinCSVFields(a.Values, d))
	}
}

func runAggGet(cfg *config, name, path string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	a, ok := doc.AggregationByName(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown aggregation: %s\n", name)
		os.Exit(1)
	}
	if cfg.jsonOut {
		_ = writeJSON(map[string]any{"name": a.Name, "values": a.Values})
		return
	}
	fmt.Println(excsv.JoinCSVFields(a.Values, doc.Header.Dialect()))
}

func runAggAdd(cfg *config, name string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	added, err := doc.AddAggregation(name)
	if err != nil {
		exitParseErr(err)
	}
	if !added {
		printMutationOK(cfg, path, name, map[string]any{"added": false})
		return
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, map[string]any{"added": true})
}

func runAggUpdate(cfg *config, name string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if err := doc.UpdateAggregation(name); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}

func runAggRemove(cfg *config, name string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveAggregation(name) {
		fmt.Fprintf(os.Stderr, "unknown aggregation: %s\n", name)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, name, nil)
}
