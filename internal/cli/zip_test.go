package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIZipPasswordWrapUnwrap(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	plain := filepath.Join(dir, "data.excsv")
	if err := os.WriteFile(plain, []byte("#!excsv version=0.2\nid\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipped := filepath.Join(dir, "data.excsv.zip")
	const password = "testpass"

	cmd := exec.Command(bin, plain, "zip", "wrap", "-o", zipped, "--password", password)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrap: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, zipped, "zip", "unwrap", "-o", filepath.Join(dir, "out.excsv"), "--password", password)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unwrap: %v\n%s", err, out)
	}

	outData, err := os.ReadFile(filepath.Join(dir, "out.excsv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outData), "#!excsv") {
		t.Fatalf("unwrap output=%q", outData)
	}
}

func TestCLIZipPasswordRemove(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	plain := filepath.Join(dir, "data.excsv")
	if err := os.WriteFile(plain, []byte("#!excsv version=0.2\nid\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipped := filepath.Join(dir, "data.excsv.zip")
	const password = "testpass"

	cmd := exec.Command(bin, plain, "zip", "wrap", "-o", zipped, "--password", password)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrap: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, zipped, "zip", "password", "remove", "--password", password)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, zipped, "zip", "unwrap", "-o", filepath.Join(dir, "out.excsv"))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unwrap without password: %v\n%s", err, out)
	}
}
