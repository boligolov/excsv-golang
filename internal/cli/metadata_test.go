package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIZipMetadataCommands(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	zipPath := filepath.Join(root, "test", "fixtures", "zip", "valid", "004_comment_full.excsv.zip")
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("zip fixtures not synced")
	}

	cases := []struct {
		name      string
		args      []string
		want      string
		wantValue bool
	}{
		{"header list", []string{zipPath, "header", "list"}, "rows=", false},
		{"header get rows", []string{zipPath, "header", "get", "rows"}, "", true},
		{"rows alias", []string{zipPath, "rows"}, "", true},
		{"meta list", []string{zipPath, "meta", "list"}, "author:", false},
		{"meta get key", []string{zipPath, "meta", "get", "author"}, "author@example.com", false},
		{"meta get file only", []string{zipPath, "meta", "get"}, "author:", false},
		{"sql list", []string{zipPath, "sql", "list"}, "#$ddl", false},
		{"sql get key", []string{zipPath, "sql", "get", "ddl"}, "CREATE TABLE", false},
		{"info", []string{zipPath, "info"}, "Rows:", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			text := string(out)
			if tc.want != "" && !strings.Contains(text, tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, text)
			}
			if tc.wantValue && strings.TrimSpace(text) == "" {
				t.Fatalf("expected non-empty output, got %q", text)
			}
		})
	}
}

func TestCLIZipSubcommands(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	cmd := exec.Command(bin, "-h")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	text := string(out)
	if strings.Contains(text, "peek") {
		t.Fatalf("zip peek should be removed:\n%s", text)
	}
	if !strings.Contains(text, "zip") {
		t.Fatalf("expected zip command group:\n%s", text)
	}
}
