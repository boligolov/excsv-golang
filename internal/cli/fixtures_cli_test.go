package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/boligolov/excsv-golang/internal/fixtures"
)

func TestCLIValidateFixtures(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	manifestPath := filepath.Join(root, "test", "fixtures", "fixtures.yaml")
	m := fixtures.MustLoad(manifestPath)
	fixtureRoot := fixtures.RootDir(manifestPath)

	for _, fx := range fixtures.FilterRF(m) {
		fx := fx
		t.Run(fx.ID, func(t *testing.T) {
			path := filepath.Join(fixtureRoot, filepath.FromSlash(fx.ID))
			if _, err := os.Stat(path); err != nil {
				t.Skipf("fixture not on disk (run sync-upstream): %v", err)
			}
			args := []string{"validate", path}
			if fx.Expect.Profile != "" {
				args = append([]string{"--expect-profile", fx.Expect.Profile}, args...)
			}
			if len(fx.Expect.Warnings) > 0 {
				args = append([]string{"--lenient"}, args...)
			}
			cmd := exec.Command(bin, args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			expectFail := fx.Expect.Parse == "fail"
			if expectFail {
				if err == nil {
					t.Fatalf("expected validate failure, got ok\n%s", out)
				}
				if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
					t.Fatalf("expected exit 2, got %v\n%s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected ok, got %v\n%s", err, out)
			}
		})
	}
}

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
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/excsv")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build excsv: %v\n%s", err, out)
	}
	return bin
}
