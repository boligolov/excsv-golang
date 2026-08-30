package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"github.com/spf13/cobra"
)

func newPackCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "pack", Short: "Pack container (.excsv.pack.zip) operations"}
	cmd.AddCommand(newPackCreateCmd(cfg), newPackTableCmd(cfg), newPackFKCmd(cfg))
	return cmd
}

func newPackCreateCmd(cfg *config) *cobra.Command {
	var out, table string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a single-table pack from a plain ExCSV file",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if excsv.IsPackPath(path) {
				exitUserErr("pack create requires a plain .excsv file")
			}
			doc, err := loadDocOnly(cfg, path, true)
			if err != nil {
				exitParseErr(err)
			}
			name := table
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				name = strings.ReplaceAll(name, ".", "_")
			}
			pack := excsv.PackFromDocument(doc, name)
			zipped, err := pack.Serialize()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			dest := out
			if dest == "" {
				dest = path + ".pack.zip"
				if strings.HasSuffix(strings.ToLower(path), ".excsv") {
					dest = strings.TrimSuffix(path, filepath.Ext(path)) + ".excsv.pack.zip"
				}
			}
			if err := os.WriteFile(dest, zipped, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]any{"ok": true, "path": dest, "table": name})
				return
			}
			fmt.Println("ok")
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output .excsv.pack.zip path")
	c.Flags().StringVar(&table, "name", "", "table name (default: source filename)")
	return c
}

func newPackTableCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "table", Short: "Manage tables inside a pack"}
	var from, out string
	add := &cobra.Command{
		Use:   "add NAME",
		Short: "Add a table from a plain ExCSV file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if from == "" {
				exitUserErr("--from is required")
			}
			path := targetPath()
			res, err := loadDoc(cfg, path, true)
			if err != nil {
				exitParseErr(err)
			}
			if res.Pack == nil {
				exitUserErr("table add requires a pack file")
			}
			src, err := excsv.ParseFile(from, cfg.parseOpts())
			if err != nil {
				exitParseErr(err)
			}
			res.Pack.AddTable(src.Doc, args[0])
			zipped, err := res.Pack.Serialize()
			if err != nil {
				exitParseErr(err)
			}
			if err := os.WriteFile(path, zipped, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			fmt.Println("ok")
		},
	}
	add.Flags().StringVar(&from, "from", "", "plain .excsv to import as the new table")
	drop := &cobra.Command{
		Use:  "drop NAME",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			res, err := loadDoc(cfg, path, true)
			if err != nil {
				exitParseErr(err)
			}
			if res.Pack == nil {
				exitUserErr("table drop requires a pack file")
			}
			if err := res.Pack.DropTable(args[0]); err != nil {
				exitUserErr(err.Error())
			}
			zipped, err := res.Pack.Serialize()
			if err != nil {
				exitParseErr(err)
			}
			if err := os.WriteFile(path, zipped, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			fmt.Println("ok")
		},
	}
	extract := &cobra.Command{
		Use:  "extract NAME",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			res, err := loadDoc(cfg, path, true)
			if err != nil {
				exitParseErr(err)
			}
			if res.Pack == nil {
				exitUserErr("table extract requires a pack file")
			}
			t, err := res.Pack.Table(args[0])
			if err != nil {
				exitUserErr(err.Error())
			}
			doc := t.ExtractDocument()
			b, err := doc.SerializeCanonical()
			if err != nil {
				exitParseErr(err)
			}
			dest := out
			if dest == "" {
				dest = args[0] + ".excsv"
			}
			if err := os.WriteFile(dest, b, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			fmt.Println("ok")
		},
	}
	extract.Flags().StringVarP(&out, "output", "o", "", "output .excsv path")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List tables in the pack",
			Run: func(cmd *cobra.Command, args []string) {
				res, err := loadDoc(cfg, targetPath(), true)
				if err != nil {
					exitParseErr(err)
				}
				if res.Pack == nil {
					exitUserErr("table list requires a pack file")
				}
				if cfg.jsonOut {
					var names []string
					for _, t := range res.Pack.Tables {
						names = append(names, t.Decl.Name)
					}
					_ = writeJSON(map[string]any{"tables": names})
					return
				}
				for _, t := range res.Pack.Tables {
					fmt.Println(t.Decl.Name)
				}
			},
		},
		add, drop, extract,
	)
	return cmd
}

func newPackFKCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "fk", Short: "Pack foreign-key (#fk) declarations"}
	cmd.AddCommand(&cobra.Command{
		Use: "list", Short: "List #fk lines",
		Run: func(cmd *cobra.Command, args []string) {
			res, err := loadDoc(cfg, targetPath(), true)
			if err != nil {
				exitParseErr(err)
			}
			if res.Pack == nil {
				exitUserErr("fk list requires a pack file")
			}
			if cfg.jsonOut {
				var out []map[string]string
				for _, fk := range res.Pack.FKs {
					out = append(out, map[string]string{"from": fk.From, "to": fk.To})
				}
				_ = writeJSON(out)
				return
			}
			for _, fk := range res.Pack.FKs {
				fmt.Printf("from=%s to=%s\n", fk.From, fk.To)
			}
		},
	})
	return cmd
}
