package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInfoExtras(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)

	aggPath := filepath.Join(root, "test", "fixtures", "plain", "valid", "011_aggregations_standard.excsv")
	zipPath := filepath.Join(root, "test", "fixtures", "zip", "valid", "004_comment_full.excsv.zip")
	if _, err := os.Stat(aggPath); err != nil {
		t.Skip("fixtures not synced")
	}

	t.Run("aggregations", func(t *testing.T) {
		out := runExcsv(t, bin, root, aggPath, "info")
		if !strings.Contains(out, "Aggregations:") || !strings.Contains(out, "sum") {
			t.Fatalf("expected aggregation names:\n%s", out)
		}
		if !strings.Contains(out, "Rows:") || !strings.Contains(out, "Delimiter:") || !strings.Contains(out, "Quote:") || !strings.Contains(out, "Null:") {
			t.Fatalf("expected dialect summary:\n%s", out)
		}
		if strings.Contains(out, "#%sum:") || strings.Contains(out, "60.00") {
			t.Fatalf("aggregation values must not appear:\n%s", out)
		}
	})

	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("zip fixtures not synced")
	}

	t.Run("sql and meta", func(t *testing.T) {
		out := runExcsv(t, bin, root, zipPath, "info")
		if !strings.Contains(out, "SQL (2):") || !strings.Contains(out, "ddl") {
			t.Fatalf("expected sql summary:\n%s", out)
		}
		if !strings.Contains(out, "SQL dialect: mysql") {
			t.Fatalf("expected sql dialect:\n%s", out)
		}
		if !strings.Contains(out, "author:") {
			t.Fatalf("expected meta:\n%s", out)
		}
	})

	t.Run("no-meta", func(t *testing.T) {
		out := runExcsv(t, bin, root, zipPath, "info", "--no-meta")
		if strings.Contains(out, "author:") {
			t.Fatalf("meta must be hidden:\n%s", out)
		}
		if !strings.Contains(out, "SQL (2):") {
			t.Fatalf("sql summary should remain:\n%s", out)
		}
	})

	t.Run("header", func(t *testing.T) {
		out := runExcsv(t, bin, root, aggPath, "info", "header")
		if !strings.Contains(out, "id,name,amount") {
			t.Fatalf("expected header row:\n%s", out)
		}
		if !strings.Contains(out, "name: id") || !strings.Contains(out, "type: decimal") {
			t.Fatalf("expected column lines:\n%s", out)
		}
	})
}

func runExcsv(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return string(out)
}
