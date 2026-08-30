package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIConvertReencodesDelim(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	if err := os.WriteFile(path, []byte("#!excsv version=0.4 delim=comma quote=none\n#column name=id\n#column name=n\nid,n\n1,a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "t.extsv")
	cmd := exec.Command(bin, path, "convert", "--delim", "tab", "-o", out)
	cmd.Dir = root
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("convert: %v\n%s", err, outBytes)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "delim=tab") || !strings.Contains(text, "1\ta") {
		t.Fatalf("converted=%s", data)
	}
}

func TestCLIFixValidate(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	if err := os.WriteFile(path, []byte("#!excsv version=0.4\nid,n\n1,a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, path, "fix", "--only", "format,columns,checksum,stamp")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fix: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, path, "validate", "--with-data")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "checksum=sha256:") {
		t.Fatalf("fix output=%s", data)
	}
	if !strings.Contains(text, "#column") {
		t.Fatalf("no columns:\n%s", text)
	}

	bad := filepath.Join(dir, "bad.excsv")
	if err := os.WriteFile(bad, []byte("#!excsv version=0.4 rows=99\n#column name=a\na\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, bad, "validate", "--with-data")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected validate fail\n%s", out)
	}
	if !strings.Contains(string(out), "rows_mismatch") {
		t.Fatalf("validate out=%s", out)
	}
}
