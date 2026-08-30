package cli_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func ensureExcsvBinary(t *testing.T, root string) string {
	t.Helper()
	name := "excsv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(root, "bin", name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/excsv")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build excsv: %v\n%s", err, out)
	}
	return bin
}
