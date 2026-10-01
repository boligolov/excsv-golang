package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tryExcsv runs the CLI and returns its output and error, for cases that
// are expected to fail.
func tryExcsv(t *testing.T, bin, root string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCLINoteAddListRemove(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	path := filepath.Join(t.TempDir(), "orders.excsv")
	src := "#!excsv version=0.5 header=1 rows=2\n#column name=id type=int role=id\n#column name=amount type=decimal\nid,amount\n7,1.00\n9,2.00\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := tryExcsv(t, bin, root, path, "note", "add", "--row", "1", "--col", "amount", "--text", "Refund pending"); err != nil {
		t.Fatalf("note add: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	// --row is written as key= because the table has an id column, and the
	// file now uses a v0.6 construct.
	if !strings.Contains(text, `#note key=9 col=amount text="Refund pending"`) || !strings.HasPrefix(text, "#!excsv version=0.6") {
		t.Fatalf("after note add:\n%s", text)
	}

	out, err := tryExcsv(t, bin, root, path, "note", "list")
	if err != nil || !strings.Contains(out, "0\tcell\trow 1\t#note") {
		t.Fatalf("note list: %v\n%s", err, out)
	}
	if out, err := tryExcsv(t, bin, root, path, "note", "add", "--col", "nope", "--text", "x"); err == nil {
		t.Fatalf("note add on an unknown column must fail\n%s", out)
	}
	if out, err := tryExcsv(t, bin, root, path, "note", "remove", "0"); err != nil {
		t.Fatalf("note remove: %v\n%s", err, out)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "#note") {
		t.Fatalf("note not removed:\n%s", data)
	}
}

func TestCLILinkSetAndCheck(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	path := filepath.Join(t.TempDir(), "links.excsv")
	src := "#!excsv version=0.6 header=1 rows=2\n#column name=label type=string\n#column name=url type=string link=\"{$}\"\nlabel,url\nok,https://example.com\nbad,javascript:alert(1)\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := tryExcsv(t, bin, root, path, "link", "check")
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 || !strings.Contains(out, "link_unsafe_scheme") {
		t.Fatalf("link check must report the javascript: link with exit 2: %v\n%s", err, out)
	}

	if out, err := tryExcsv(t, bin, root, path, "link", "set", "--row", "0", "--col", "label", "--href", "https://docs.example.com"); err != nil {
		t.Fatalf("link set: %v\n%s", err, out)
	}
	out, err = tryExcsv(t, bin, root, path, "link", "list")
	if err != nil || !strings.Contains(out, `#link row=0 col=label href="https://docs.example.com"`) {
		t.Fatalf("link list: %v\n%s", err, out)
	}
	if out, err := tryExcsv(t, bin, root, path, "link", "set", "--col", "label", "--href", "https://x"); err == nil {
		t.Fatalf("link set without --row/--key must fail\n%s", out)
	}
}

func TestCLISortMovesNoteAnchors(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	path := filepath.Join(t.TempDir(), "sort.excsv")
	src := "#!excsv version=0.6 header=1 rows=3\n#column name=n type=int\n#note row=0 col=n text=\"was first\"\nn\n3\n1\n2\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := tryExcsv(t, bin, root, path, "data", "sort", "--by", "n"); err != nil {
		t.Fatalf("data sort: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `#note row=2 col=n text="was first"`) {
		t.Fatalf("row= must follow the row (value 3 is now last):\n%s", data)
	}
}

func TestCLIColumnRenameInline(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	path := filepath.Join(t.TempDir(), "r.excsv")
	src := "#!excsv version=0.6 header=1 rows=1\n#column name=id type=int role=id\n#column name=amt type=decimal\n#note key=1 col=amt text=x\n#$ddl: CREATE TABLE t (amt DECIMAL)\nid,amt\n1,2.50\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := tryExcsv(t, bin, root, path, "column", "rename", "amt", "amount")
	if err != nil || !strings.Contains(out, "SQL is not rewritten") {
		t.Fatalf("rename: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"#column name=amount type=decimal", `#note key=1 col=amount text="x"`, "\nid,amount\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("after rename lacks %q:\n%s", want, data)
		}
	}
	if out, err := tryExcsv(t, bin, root, path, "column", "rename", "nope", "x"); err == nil {
		t.Fatalf("renaming an unknown column must fail\n%s", out)
	}
}

func TestCLIColumnRenameSidecarKeepsData(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	csv := "id,amt\n1,2.50\n"
	if err := os.WriteFile(filepath.Join(dir, "s.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	side := filepath.Join(dir, "s.excsv")
	if err := os.WriteFile(side, []byte("#!excsv version=0.6 header=1 rows=1 reference=s.csv\n#column name=id type=int\n#column name=amt type=decimal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := tryExcsv(t, bin, root, side, "column", "rename", "amt", "amount"); err != nil {
		t.Fatalf("rename: %v\n%s", err, out)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "s.csv")); string(data) != csv {
		t.Fatalf("referenced CSV was rewritten:\n%s", data)
	}
	data, _ := os.ReadFile(side)
	if !strings.Contains(string(data), "#column name=amount title=amt type=decimal") {
		t.Fatalf("sidecar after rename:\n%s", data)
	}
	if out, err := tryExcsv(t, bin, root, side, "validate", "--with-data"); err != nil {
		t.Fatalf("validate after rename: %v\n%s", err, out)
	}
}

func TestCLIColumnRenamePackFK(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	src := filepath.Join(root, "test", "fixtures", "pack", "valid", "004_two_tables_with_fk.excsv.pack.zip")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skip("fixtures not synced")
	}
	path := filepath.Join(t.TempDir(), "p.excsv.pack.zip")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	fks, err := tryExcsv(t, bin, root, path, "pack", "fk", "list")
	if err != nil {
		t.Fatalf("fk list: %v\n%s", err, fks)
	}
	// Rename the column on the "to" side of the first #fk.
	line := strings.Fields(strings.TrimSpace(strings.Split(fks, "\n")[0]))
	var to string
	for _, f := range line {
		if strings.HasPrefix(f, "to=") {
			to = strings.TrimPrefix(f, "to=")
		}
	}
	table, col, ok := strings.Cut(to, ".")
	if !ok {
		t.Fatalf("unexpected fk list output: %q", fks)
	}
	if out, err := tryExcsv(t, bin, root, "--table", table, path, "column", "rename", col, col+"_renamed"); err != nil {
		t.Fatalf("rename: %v\n%s", err, out)
	}
	after, err := tryExcsv(t, bin, root, path, "pack", "fk", "list")
	if err != nil || !strings.Contains(after, "to="+table+"."+col+"_renamed") {
		t.Fatalf("fk not updated: %v\n%s", err, after)
	}
	if out, err := tryExcsv(t, bin, root, path, "validate", "--with-data"); err != nil {
		t.Fatalf("validate after rename: %v\n%s", err, out)
	}
}
