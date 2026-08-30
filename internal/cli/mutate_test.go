package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIMetaSet(t *testing.T) {
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

	cmd := exec.Command(bin, path, "meta", "set", "note", "--value", "hello world")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "meta", "get", "note")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello world" {
		t.Fatalf("note=%q", out)
	}
}

func TestCLISQLSet(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	src := filepath.Join(root, "test", "fixtures", "plain", "valid", "012_sql_multi_dialect.excsv")
	dir := t.TempDir()
	path := filepath.Join(dir, "sql.excsv")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	newDDL := "CREATE TABLE t (id INT PRIMARY KEY)"
	cmd := exec.Command(bin, path, "sql", "set", "ddl", "--value", newDDL)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "sql", "get", "ddl")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(string(out)) != newDDL {
		t.Fatalf("ddl=%q", out)
	}
}
