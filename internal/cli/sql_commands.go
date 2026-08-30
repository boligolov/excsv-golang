package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

func newSQLCmd(cfg *config) *cobra.Command {
	var verb, dialect, value string
	cmd := &cobra.Command{Use: "sql", Short: "SQL companion (#$) operations"}

	list := &cobra.Command{
		Use: "list", Short: "List #$ statements", Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSQLList(cfg, targetPath(), verb, dialect)
		},
	}
	list.Flags().StringVar(&verb, "verb", "", "filter: ddl or dql")
	list.Flags().StringVar(&dialect, "dialect", "", "filter by effective dialect (exact or family)")

	get := &cobra.Command{
		Use: "get [KEY]", Short: "Print one #$ payload", Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if len(args) == 0 {
				runSQLList(cfg, path, verb, dialect)
				return
			}
			runSQLGet(cfg, args[0], path, verb, dialect)
		},
	}
	get.Flags().StringVar(&verb, "verb", "", "filter: ddl or dql")
	get.Flags().StringVar(&dialect, "dialect", "", "filter by effective dialect (exact or family)")

	set := &cobra.Command{
		Use:   "set KEY",
		Short: "Set #$KEY payload (requires --value)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if !cmd.Flags().Changed("value") {
				exitUserErr("--value is required")
			}
			runSQLSet(cfg, args[0], value)
		},
	}
	set.Flags().StringVar(&value, "value", "", "SQL payload (use shell quotes for spaces)")

	cmd.AddCommand(list, get, set,
		&cobra.Command{
			Use: "remove KEY", Short: "Remove a #$ statement", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runSQLRemove(cfg, args[0])
			},
		},
		newSQLDialectCmd(cfg),
	)
	return cmd
}

// newSQLDialectCmd owns sql-dialect=, the default for unsuffixed #$ lines. The
// field governs #$, so it belongs to sql rather than to a generic header setter.
//
// Note the deliberate collision: `sql list --dialect postgres` filters lines,
// `sql dialect set postgres` sets the default.
func newSQLDialectCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "dialect", Short: "Default SQL dialect (sql-dialect=) for unsuffixed #$ lines"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "get", Short: "Print the default dialect, or ansi when unset", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				doc, err := loadDocOnly(cfg, targetPath(), false)
				if err != nil {
					exitParseErr(err)
				}
				// ansi is what the resolver actually uses when the field is
				// absent, so that is what we print. Never JSON: this exists to
				// be captured by a shell.
				if d := doc.Header.SQLDialect; d != "" {
					fmt.Println(d)
					return
				}
				fmt.Println("ansi")
			},
		},
		&cobra.Command{
			Use: "set DIALECT", Short: "Set sql-dialect=", Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				runSQLDialectSet(cfg, args[0])
			},
		},
		&cobra.Command{
			Use: "remove", Short: "Remove sql-dialect= (#$ lines fall back to ansi)", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				runSQLDialectSet(cfg, "")
			},
		},
	)
	return cmd
}

func runSQLDialectSet(cfg *config, dialect string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if err := doc.SetHeaderField("sql-dialect", dialect); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, "sql-dialect", map[string]any{"value": dialect})
}

func runSQLList(cfg *config, path, verb, dialect string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	var out []map[string]string
	for _, s := range doc.Meta.SQL {
		if verb != "" && s.Verb != verb {
			continue
		}
		eff := excsv.EffectiveDialect(s, doc.Header.SQLDialect)
		if dialect != "" && !dialectMatches(eff, dialect) {
			continue
		}
		out = append(out, map[string]string{
			"verb": s.Verb, "dialect": eff, "key": s.RawKey, "sql": s.Payload,
		})
		if !cfg.jsonOut {
			fmt.Printf("#$%s [%s]: %s\n", s.RawKey, eff, s.Payload)
		}
	}
	if cfg.jsonOut {
		_ = writeJSON(out)
	}
}

func runSQLGet(cfg *config, key, path, verb, dialect string) {
	doc, err := loadTableDoc(cfg, path, true)
	if err != nil {
		exitParseErr(err)
	}
	var matches []excsv.SQLStatement
	for _, s := range doc.Meta.SQL {
		if s.RawKey != key {
			continue
		}
		if verb != "" && s.Verb != verb {
			continue
		}
		eff := excsv.EffectiveDialect(s, doc.Header.SQLDialect)
		if dialect != "" && !dialectMatches(eff, dialect) {
			continue
		}
		matches = append(matches, s)
	}
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "unknown sql key: %s\n", key)
		os.Exit(1)
	}
	if len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "ambiguous sql key: %s\n", key)
		os.Exit(1)
	}
	fmt.Println(matches[0].Payload)
}

func runSQLSet(cfg *config, key, value string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if err := doc.SetSQL(key, value); err != nil {
		exitParseErr(err)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func runSQLRemove(cfg *config, key string) {
	path := targetPath()
	doc, err := loadDocForMutation(cfg, path)
	if err != nil {
		exitParseErr(err)
	}
	if !doc.RemoveSQL(key) {
		fmt.Fprintf(os.Stderr, "unknown sql key: %s\n", key)
		os.Exit(1)
	}
	if err := saveDocument(cfg, doc, path); err != nil {
		exitParseErr(err)
	}
	printMutationOK(cfg, path, key, nil)
}

func dialectMatches(effective, target string) bool {
	if effective == target {
		return true
	}
	eBase, _, _ := strings.Cut(effective, "-")
	tBase, _, _ := strings.Cut(target, "-")
	return eBase != "" && eBase == tBase
}
