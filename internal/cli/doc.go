package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var targetFile string

func targetPath() string { return targetFile }

func setTargetFile(path string) { targetFile = path }

func argLooksLikeFile(arg string) bool {
	ext := strings.ToLower(filepath.Ext(arg))
	if ext == ".excsv" || ext == ".ecsv" || ext == ".extsv" || ext == ".csv" || ext == ".tsv" || strings.HasSuffix(strings.ToLower(arg), ".zip") {
		return true
	}
	if _, err := os.Stat(arg); err == nil {
		return true
	}
	return false
}

func isRowZipPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
	return ext == ".zip" && (strings.HasSuffix(base, ".excsv") || strings.HasSuffix(base, ".ecsv"))
}

// Execute runs the CLI. When the first positional argument looks like a file path,
// it is bound as the document target for all subcommands (excsv FILE command ...).
func Execute() error {
	setTargetFile("")
	root := newRoot()
	root.TraverseChildren = true
	args := rewriteFileFirstArgs(os.Args[1:])
	root.SetArgs(args)
	return root.Execute()
}

func rewriteFileFirstArgs(args []string) []string {
	i := skipGlobalFlags(args, 0)
	if i >= len(args) {
		return args
	}
	if args[i] == "version" || args[i] == "help" {
		return args
	}
	if !argLooksLikeFile(args[i]) {
		return args
	}
	setTargetFile(args[i])
	out := make([]string, 0, len(args))
	out = append(out, args[:i]...)
	out = append(out, args[i+1:]...)
	return out
}

func skipGlobalFlags(args []string, i int) int {
	boolFlags := map[string]bool{
		"--strict": true, "--lenient": true, "--json": true, "--clean-human-comments": true,
	}
	for i < len(args) {
		a := args[i]
		if a == "--" {
			return i + 1
		}
		if strings.HasPrefix(a, "--") {
			if strings.Contains(a, "=") {
				i++
				continue
			}
			if boolFlags[a] {
				i++
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i += 2
				continue
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			i++
			continue
		}
		break
	}
	return i
}

func needsTargetFile(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "version", "help", "completion":
		return false
	}
	if cmd.Parent() == nil {
		return false
	}
	return true
}

func requireTargetFile(cmd *cobra.Command) {
	if !needsTargetFile(cmd) {
		return
	}
	if targetPath() == "" {
		exitUserErr("FILE required as first argument (example: excsv data.excsv validate)")
	}
}
