package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
