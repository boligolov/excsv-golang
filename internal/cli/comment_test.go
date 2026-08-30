package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICommentAdd(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.excsv")
	if err := os.WriteFile(path, []byte("#!excsv version=0.2\nid\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, path, "comment", "add", "--value", "my note")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	cmd = exec.Command(bin, path, "comment", "list")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(string(out), "## my note") {
		t.Fatalf("list=%q", out)
	}
}
