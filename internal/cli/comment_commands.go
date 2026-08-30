package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func newCommentCmd(cfg *config) *cobra.Command {
	var value string
	cmd := &cobra.Command{Use: "comment", Short: "Human comment (##) operations"}
	add := &cobra.Command{
		Use:   "add",
		Short: "Add a ## human comment (requires --value)",
		Run: func(cmd *cobra.Command, args []string) {
			if value == "" {
				exitUserErr("--value is required")
			}
			runCommentAdd(cfg, value)
		},
	}
	add.Flags().StringVar(&value, "value", "", "comment text (## prefix added if missing)")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List ## human comments",
			Run: func(cmd *cobra.Command, args []string) {
				runCommentList(cfg, targetPath())
			},
		},
		add,
		&cobra.Command{
			Use:   "remove INDEX",
			Short: "Remove ## comment by 0-based index (see comment list)",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				idx, err := strconv.Atoi(args[0])
				if err != nil {
					exitUserErr("INDEX must be an integer")
				}
				runCommentRemove(cfg, idx)
			},
		},
	)
	return cmd
}

func runCommentList(cfg *config, path string) {
	doc, err := loadDocOnly(cfg, path, false)
	if err != nil {
		exitParseErr(err)
	}
	if cfg.jsonOut {
		_ = writeJSON(doc.Meta.HumanComments)
		return
	}
	for i, line := range doc.Meta.HumanComments {
		fmt.Printf("%d\t%s\n", i, line)
	}
}

func runCommentAdd(cfg *config, text string) {
	path := targetPath()
	doc, err := loadPackScopedDoc(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	doc.AddHumanComment(text)
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, "", map[string]any{"index": len(doc.Meta.HumanComments) - 1})
}

func runCommentRemove(cfg *config, index int) {
	path := targetPath()
	doc, err := loadPackScopedDoc(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveHumanComment(index) {
		fmt.Fprintf(os.Stderr, "unknown comment index: %d\n", index)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, strconv.Itoa(index), nil)
}
