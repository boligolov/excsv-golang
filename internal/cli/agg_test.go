package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAggUpdate(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	src := filepath.Join(root, "test", "fixtures", "plain", "valid", "011_aggregations_standard.excsv")
	dir := t.TempDir()
	path := filepath.Join(dir, "agg.excsv")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	// Drop existing #% lines to test update.
	text := strings.ReplaceAll(string(data), "#%sum: ,,60.00\n", "")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, path, "agg", "update", "sum")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "agg", "get", "sum")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(string(out), "60") {
		t.Fatalf("sum=%q", out)
	}
}

func TestCLIMetaRemove(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	src := filepath.Join(root, "test", "fixtures", "plain", "valid", "008_meta_duplicate_last_wins.excsv")
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.excsv")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, path, "meta", "remove", "author")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "meta", "get", "author")
	cmd.Dir = root
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected missing key")
	}
}
