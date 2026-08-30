package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
	"github.com/spf13/cobra"
)

func newZipCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{Use: "zip", Short: "Row ZIP container operations"}
	cmd.AddCommand(
		newZipWrapCmd(cfg),
		newZipUnwrapCmd(cfg),
		newZipPasswordCmd(cfg),
	)
	return cmd
}

func newZipWrapCmd(cfg *config) *cobra.Command {
	var out, password string
	c := &cobra.Command{
		Use:   "wrap",
		Short: "Wrap plain ExCSV as row ZIP",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if isRowZipPath(path) {
				exitUserErr("zip wrap requires a plain .excsv or .ecsv file, not a zip")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			base := filepath.Base(path)
			entry := strings.TrimSuffix(base, filepath.Ext(base))
			if !strings.HasSuffix(entry, ".excsv") && !strings.HasSuffix(entry, ".ecsv") {
				entry += ".excsv"
			}
			dest := out
			if dest == "" {
				dest = entry + ".zip"
			}
			zipped, err := excsvzip.WrapWithPassword(data, entry, "", password)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if err := os.WriteFile(dest, zipped, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if cfg.jsonOut {
				out := map[string]any{"ok": true, "path": dest, "encrypted": password != ""}
				_ = writeJSON(out)
				return
			}
			fmt.Println("ok")
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output zip path")
	c.Flags().StringVar(&password, "password", "", "encrypt primary entry with this password")
	return c
}

func newZipUnwrapCmd(cfg *config) *cobra.Command {
	var out, password string
	c := &cobra.Command{
		Use:   "unwrap",
		Short: "Extract inner plain ExCSV from row ZIP",
		Run: func(cmd *cobra.Command, args []string) {
			path := targetPath()
			if !isRowZipPath(path) {
				exitUserErr("zip unwrap requires a .excsv.zip or .ecsv.zip file")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if password == "" {
				password = cfg.zipPassword
			}
			ext, err := excsvzip.ExtractWithPassword(path, data, password)
			if err != nil {
				exitParseErr(mapZipCLIError(err))
			}
			dest := out
			if dest == "" {
				dest = strings.TrimSuffix(filepath.Base(path), ".zip")
			}
			if err := os.WriteFile(dest, ext.Inner, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(3)
			}
			if cfg.jsonOut {
				_ = writeJSON(map[string]any{"ok": true, "path": dest})
				return
			}
			fmt.Println("ok")
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "output plain path")
	c.Flags().StringVar(&password, "password", "", "password for encrypted zip (or use --zip-password)")
	return c
}

func newZipPasswordCmd(cfg *config) *cobra.Command {
	var current, newPassword string
	cmd := &cobra.Command{Use: "password", Short: "ZIP entry password operations"}
	set := &cobra.Command{
		Use:   "set",
		Short: "Set or change ZIP entry password (requires --password)",
		Run: func(cmd *cobra.Command, args []string) {
			if newPassword == "" {
				exitUserErr("--password is required")
			}
			runZipPasswordSet(cfg, current, newPassword)
		},
	}
	set.Flags().StringVar(&current, "current-password", "", "current password when archive is already encrypted")
	set.Flags().StringVar(&newPassword, "password", "", "new password")
	remove := &cobra.Command{
		Use:   "remove",
		Short: "Remove ZIP entry password (requires --password for encrypted archives)",
		Run: func(cmd *cobra.Command, args []string) {
			runZipPasswordRemove(cfg, current)
		},
	}
	remove.Flags().StringVar(&current, "password", "", "current password (or use --zip-password)")
	cmd.AddCommand(set, remove)
	return cmd
}

func runZipPasswordSet(cfg *config, current, newPassword string) {
	path := targetPath()
	if !isRowZipPath(path) {
		exitUserErr("zip password set requires a .excsv.zip or .ecsv.zip file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
	if current == "" {
		current = cfg.zipPassword
	}
	out, err := excsvzip.ReWrap(path, data, current, newPassword)
	if err != nil {
		exitParseErr(mapZipCLIError(err))
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
	printMutationOK(cfg, path, "", map[string]any{"encrypted": true})
}

func runZipPasswordRemove(cfg *config, password string) {
	path := targetPath()
	if !isRowZipPath(path) {
		exitUserErr("zip password remove requires a .excsv.zip or .ecsv.zip file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
	if password == "" {
		password = cfg.zipPassword
	}
	ins, err := excsvzip.Inspect(path, data)
	if err != nil {
		exitParseErr(mapZipCLIError(err))
	}
	if !ins.Encrypted {
		exitUserErr("zip is not encrypted")
	}
	out, err := excsvzip.ReWrap(path, data, password, "")
	if err != nil {
		exitParseErr(mapZipCLIError(err))
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
	printMutationOK(cfg, path, "", map[string]any{"encrypted": false})
}
