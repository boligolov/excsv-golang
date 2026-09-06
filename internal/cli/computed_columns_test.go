package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const computedFixture = "#!excsv version=0.5 header=1 rows=2\n" +
	"#column name=price type=decimal\n" +
	"#column name=quantity type=int\n" +
	"#column name=total type=decimal formula=\"price * quantity\"\n" +
	"price,quantity\n" +
	"10.00,3\n" +
	"2.50,4\n"

func runCLI(t *testing.T, root, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("excsv %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestCLIColumnMaterializePlain(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	if err := os.WriteFile(path, []byte(computedFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	runCLI(t, root, bin, path, "validate")
	runCLI(t, root, bin, path, "column", "materialize", "total")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "materialized=1") {
		t.Fatalf("missing materialized=1:\n%s", text)
	}
	if !strings.Contains(text, "price,quantity,total") {
		t.Fatalf("missing total header cell:\n%s", text)
	}
	if !strings.Contains(text, "10.00,3,30.00") {
		t.Fatalf("missing computed value:\n%s", text)
	}
	runCLI(t, root, bin, path, "validate", "--with-data")

	runCLI(t, root, bin, path, "column", "dematerialize", "total")
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "materialized") {
		t.Fatalf("materialized= should be gone:\n%s", data)
	}
}

func TestCLIColumnMaterializeZip(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	if err := os.WriteFile(path, []byte(computedFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "t.excsv.zip")
	runCLI(t, root, bin, path, "zip", "wrap", "-o", zipPath)

	runCLI(t, root, bin, zipPath, "column", "materialize", "total")
	runCLI(t, root, bin, zipPath, "validate")
	runCLI(t, root, bin, zipPath, "validate", "--with-data")
	runCLI(t, root, bin, zipPath, "info")

	unwrapped := filepath.Join(dir, "unwrapped.excsv")
	runCLI(t, root, bin, zipPath, "zip", "unwrap", "-o", unwrapped)
	data, err := os.ReadFile(unwrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "10.00,3,30.00") {
		t.Fatalf("unwrapped=%s", data)
	}

	runCLI(t, root, bin, zipPath, "column", "dematerialize", "total")
	runCLI(t, root, bin, zipPath, "validate", "--with-data")
}

func TestCLIColumnMaterializePack(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.excsv")
	if err := os.WriteFile(path, []byte(computedFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(dir, "t.excsv.pack.zip")
	runCLI(t, root, bin, path, "pack", "create", "-o", packPath, "--name", "tbl")

	before := runCLI(t, root, bin, packPath, "pack", "table", "list")
	if !strings.Contains(before, "tbl") {
		t.Fatalf("table list before=%q", before)
	}

	runCLI(t, root, bin, packPath, "column", "materialize", "total")
	runCLI(t, root, bin, packPath, "validate", "--with-data")

	colList := runCLI(t, root, bin, packPath, "column", "list")
	if !strings.Contains(colList, "materialized=1") {
		t.Fatalf("column list after materialize=%q", colList)
	}

	runCLI(t, root, bin, packPath, "column", "dematerialize", "total")
	runCLI(t, root, bin, packPath, "validate", "--with-data")
	colList = runCLI(t, root, bin, packPath, "column", "list")
	if strings.Contains(colList, "materialized") {
		t.Fatalf("column list after dematerialize=%q", colList)
	}
}

func TestCLIColumnMaterializeSidecar(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "sales.csv")
	if err := os.WriteFile(csvPath, []byte("price,quantity\n10.00,3\n2.50,4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(dir, "sales.excsv")
	sidecar := "#!excsv version=0.5 header=1 rows=2 reference=sales.csv\n" +
		"#column name=price type=decimal\n" +
		"#column name=quantity type=int\n" +
		"#column name=total type=decimal formula=\"price * quantity\"\n"
	if err := os.WriteFile(sidecarPath, []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarBefore, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	csvBefore, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}

	runCLI(t, root, bin, sidecarPath, "column", "materialize", "total")

	sidecarAfter, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	csvAfter, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sidecarBefore) != string(sidecarAfter) {
		t.Fatalf("sidecar was rewritten:\nbefore=%s\nafter=%s", sidecarBefore, sidecarAfter)
	}
	if string(csvBefore) != string(csvAfter) {
		t.Fatalf("referenced csv was rewritten:\nbefore=%s\nafter=%s", csvBefore, csvAfter)
	}

	outPath := filepath.Join(dir, "sales.materialized.excsv")
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected default output file: %v", err)
	}
	text := string(out)
	if strings.Contains(text, "reference=") {
		t.Fatalf("materialized output must not carry reference=:\n%s", text)
	}
	if !strings.Contains(text, "10.00,3,30.00") {
		t.Fatalf("materialized output missing computed values:\n%s", text)
	}
	runCLI(t, root, bin, outPath, "validate", "--with-data")
}
