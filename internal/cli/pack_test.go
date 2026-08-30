package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIPackCreateListExtract(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	plain := filepath.Join(dir, "contacts.excsv")
	if err := os.WriteFile(plain, []byte("#!excsv version=0.3\n#column name=id type=int\n#column name=n type=string\nid,n\n1,a\n2,b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(dir, "contacts.excsv.pack.zip")
	cmd := exec.Command(bin, plain, "pack", "create", "-o", pack, "--name", "contacts")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, pack, "pack", "table", "list")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "contacts") {
		t.Fatalf("list=%q", out)
	}
	cmd = exec.Command(bin, pack, "validate")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	extracted := filepath.Join(dir, "out.excsv")
	cmd = exec.Command(bin, pack, "pack", "table", "extract", "contacts", "-o", extracted)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extract: %v\n%s", err, out)
	}
	data, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "1,a") {
		t.Fatalf("extract=%s", data)
	}

	cmd = exec.Command(bin, pack, "--table", "contacts", "column", "list")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("col list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "name=id") {
		t.Fatalf("col list=%q", out)
	}

	cmd = exec.Command(bin, pack, "--table", "contacts", "header", "rows")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rows: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "2" {
		t.Fatalf("rows=%q", out)
	}

	products := filepath.Join(dir, "products.excsv")
	if err := os.WriteFile(products, []byte("#!excsv version=0.4\n#column name=sku type=string\nsku\nA1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, pack, "pack", "table", "add", "products", "--from", products)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, pack, "pack", "table", "list")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list2: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "products") {
		t.Fatalf("list after add=%q", out)
	}
	cmd = exec.Command(bin, pack, "pack", "fk", "list")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fk list: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, pack, "pack", "table", "drop", "products")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("drop: %v\n%s", err, out)
	}
}
