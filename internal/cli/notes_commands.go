package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

// anchorFlags are the address flags shared by `note add` and `link set`.
type anchorFlags struct {
	col string
	row int
	key string
}

func (a *anchorFlags) register(c *cobra.Command, colHelp string) {
	c.Flags().StringVar(&a.col, "col", "", colHelp)
	c.Flags().IntVar(&a.row, "row", -1, "zero-based data row; written as key= when the table has an id column")
	c.Flags().StringVar(&a.key, "key", "", "row's value in the id column (role=id, else unique=1)")
}

// attrs builds col= / row= / key= for doc, checking that the address resolves:
// writing a note or link that points nowhere is a usage error, not a WARN.
func (a *anchorFlags) attrs(doc *excsv.Document, rowSet, keySet bool) []excsv.KV {
	if rowSet && keySet {
		exitUserErr("--row and --key are mutually exclusive")
	}
	// Row before column, as the spec's own examples write it.
	var out []excsv.KV
	switch {
	case rowSet:
		anchor, err := doc.RowAnchor(a.row)
		if err != nil {
			exitUserErr(err.Error())
		}
		out = append(out, anchor)
	case keySet:
		out = append(out, excsv.KV{Key: "key", Value: a.key})
	}
	if a.col != "" {
		if !doc.ResolveColumn(a.col) {
			exitUserErr("unknown column: " + a.col)
		}
		out = append(out, excsv.KV{Key: "col", Value: a.col})
	}
	return out
}

func newNoteCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "note", Short: "Notes (#note) on a cell, row, column, or the table"}

	var anchor anchorFlags
	var text, author, created string
	add := &cobra.Command{
		Use:   "add",
		Short: "Add a #note (requires --text; --col/--row/--key pick the target)",
		Long: `Add a #note. The address flags present decide what it is attached to:
--col with --row/--key is a cell, --col alone a column, --row/--key alone a
row, none the table. --row is written as key= when the table has an id column,
so the note survives sorting.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if text == "" {
				exitUserErr("--text is required")
			}
			runNoteAdd(cfg, &anchor, cmd.Flags().Changed("row"), cmd.Flags().Changed("key"), text, author, created)
		},
	}
	anchor.register(add, "column name (or index with header=0)")
	add.Flags().StringVar(&text, "text", "", "the note (plain, single-line text)")
	add.Flags().StringVar(&author, "author", "", "who wrote it, e.g. an email address")
	add.Flags().StringVar(&created, "created", "", "when it was written (ISO 8601 datetime)")

	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List #note lines with their resolved targets", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runNoteList(cfg, targetPath())
			},
		},
		add,
		&cobra.Command{
			Use:   "remove INDEX",
			Short: "Remove a #note by 0-based index (see note list)",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runNoteRemove(cfg, parseIndexArg(args[0]))
			},
		},
	)
	return cmd
}

func runNoteList(cfg *config, path string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	notes := doc.ResolveNotes()
	if cfg.jsonOut {
		out := make([]map[string]any, 0, len(notes))
		for i, n := range notes {
			entry := map[string]any{"index": i, "target": n.Target, "resolved": n.Resolved, "text": n.Text()}
			for _, kv := range n.Attrs {
				if kv.Key != "text" {
					entry[kv.Key] = kv.Value
				}
			}
			if n.Row >= 0 {
				entry["resolved_row"] = n.Row
			}
			out = append(out, entry)
		}
		_ = writeJSON(out)
		return
	}
	// INDEX, target, resolved data row ("-" for none, "unresolved"), line.
	for i, n := range notes {
		row := "-"
		switch {
		case !n.Resolved:
			row = "unresolved"
		case n.Row >= 0:
			row = "row " + strconv.Itoa(n.Row)
		}
		fmt.Printf("%d\t%s\t%s\t%s\n", i, n.Target, row, n.MetaLine())
	}
}

func runNoteAdd(cfg *config, anchor *anchorFlags, rowSet, keySet bool, text, author, created string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	attrs := anchor.attrs(doc, rowSet, keySet)
	if author != "" {
		attrs = append(attrs, excsv.KV{Key: "author", Value: author})
	}
	if created != "" {
		attrs = append(attrs, excsv.KV{Key: "created", Value: created})
	}
	attrs = append(attrs, excsv.KV{Key: "text", Value: text})
	if err := doc.AddNote(attrs); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	note := doc.Meta.Notes[len(doc.Meta.Notes)-1]
	printMutationOK(cfg, path, "", map[string]any{"index": len(doc.Meta.Notes) - 1, "line": note.MetaLine()})
}

func runNoteRemove(cfg *config, index int) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveNote(index) {
		fmt.Fprintf(os.Stderr, "unknown note index: %d\n", index)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, strconv.Itoa(index), nil)
}

func newLinkCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "One-off cell links (#link); column templates are `column set NAME --attr link=…`",
	}

	var anchor anchorFlags
	var href string
	set := &cobra.Command{
		Use:   "set",
		Short: "Link one cell (--col plus --row or --key) to --href, replacing an existing #link there",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			rowSet, keySet := cmd.Flags().Changed("row"), cmd.Flags().Changed("key")
			if href == "" || anchor.col == "" || (!rowSet && !keySet) {
				exitUserErr("--href, --col and one of --row / --key are required")
			}
			runLinkSet(cfg, &anchor, rowSet, keySet, href)
		},
	}
	anchor.register(set, "column name (or index with header=0) of the cell")
	set.Flags().StringVar(&href, "href", "", "the URL; rendered as a link only for http, https and mailto")

	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List #link lines", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runLinkList(cfg, targetPath())
			},
		},
		set,
		&cobra.Command{
			Use:   "remove INDEX",
			Short: "Remove a #link by 0-based index (see link list)",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runLinkRemove(cfg, parseIndexArg(args[0]))
			},
		},
		&cobra.Command{
			Use:   "check",
			Short: "Report unsafe, unresolved, duplicate, and broken-template links",
			Long: `Resolve every link= template and #link line against the data and report
what will not render as a link: schemes other than http/https/mailto
(link_unsafe_scheme), addresses that resolve nowhere (link_unresolved), cells
linked twice (link_duplicate), and templates naming no stored column or with
an unterminated { (link_unknown_column, link_template_malformed). Never
follows or fetches a link.`,
			Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runLinkCheck(cfg, targetPath())
			},
		},
	)
	return cmd
}

func runLinkList(cfg *config, path string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	if cfg.jsonOut {
		out := make([]map[string]any, 0, len(doc.Meta.Links))
		for i, l := range doc.Meta.Links {
			entry := map[string]any{"index": i}
			for _, kv := range l.Attrs {
				entry[kv.Key] = kv.Value
			}
			out = append(out, entry)
		}
		_ = writeJSON(out)
		return
	}
	for i, l := range doc.Meta.Links {
		fmt.Printf("%d\t%s\n", i, l.MetaLine())
	}
}

func runLinkSet(cfg *config, anchor *anchorFlags, rowSet, keySet bool, href string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	attrs := append(anchor.attrs(doc, rowSet, keySet), excsv.KV{Key: "href", Value: href})
	if !excsv.IsSafeLinkScheme(href) {
		fmt.Fprintln(os.Stderr, "warning: href is not http/https/mailto; it will be shown as plain text")
	}
	if err := doc.SetLink(attrs); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, "", map[string]any{"line": excsv.Link{Attrs: attrs}.MetaLine()})
}

func runLinkRemove(cfg *config, index int) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveLink(index) {
		fmt.Fprintf(os.Stderr, "unknown link index: %d\n", index)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, strconv.Itoa(index), nil)
}

func runLinkCheck(cfg *config, path string) {
	res, err := loadDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	var findings []excsv.Issue
	links := 0
	for _, doc := range validationTargets(cfg, res) {
		links += doc.ResolveLinks().Count()
		for _, iss := range doc.CheckNotesLinks() {
			if strings.HasPrefix(string(iss.Kind), "link_") {
				findings = append(findings, iss)
			}
		}
	}
	if cfg.jsonOut {
		_ = writeJSON(map[string]any{"ok": len(findings) == 0, "path": path, "links": links, "findings": findings})
	} else {
		for _, f := range findings {
			fmt.Fprintln(os.Stderr, f.Error())
		}
		if len(findings) == 0 {
			fmt.Printf("ok (%d linked cells)\n", links)
		}
	}
	if len(findings) > 0 && !cfg.lenient {
		os.Exit(2)
	}
}

func parseIndexArg(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		exitUserErr("INDEX must be an integer")
	}
	return n
}
