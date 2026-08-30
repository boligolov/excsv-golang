package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIDataAppendSort(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	src := "#!excsv version=0.2\n#column name=id type=int\n#column name=amount type=decimal\nid,amount\n2,20\n1,10\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, path, "data", "append", "--row", "3,5")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("append: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "data", "sort", "--by", "amount")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sort: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "data", "print")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	got := strings.ReplaceAll(strings.TrimSpace(string(out)), "\r\n", "\n")
	want := "id,amount\n3,5\n1,10\n2,20"
	if got != want {
		t.Fatalf("print=%q want %q", got, want)
	}

	cmd = exec.Command(bin, path, "data", "get", "0", "id")
	cmd.Dir = root
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(string(out)) != "3" {
		t.Fatalf("get=%q", out)
	}
}

func TestCLIValidateColumn(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	src := "#!excsv version=0.4\n#column name=id type=int\nid\nnot-int\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, path, "validate", "--with-data", "--column", "id")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		t.Fatal("expected schema failure")
	}

	cmd = exec.Command(bin, path, "column", "set", "id", "--attr", "type=string")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "validate", "--with-data", "--column", "id")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("validate after set: %v\n%s", err, out)
	}
}

func TestCLIConvertRoundTrip(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "t.csv")
	exPath := filepath.Join(dir, "t.excsv")
	back := filepath.Join(dir, "back.csv")
	if err := os.WriteFile(csvPath, []byte("id,name\n1,alice\n2,bob\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, csvPath, "convert", "-o", exPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("convert: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, exPath, "data", "print", "-o", back)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("data print: %v\n%s", err, out)
	}
	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "id,name\n1,alice\n2,bob\n" {
		t.Fatalf("round-trip csv=%q", got)
	}

	cmd = exec.Command(bin, exPath, "column", "list")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("column list: %v", err)
	}
	if !strings.Contains(string(out), "type=string") {
		t.Fatalf("columns=%q", out)
	}
}
