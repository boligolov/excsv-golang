package excsv_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/boligolov/excsv-golang/internal/fixtures"
	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestManifestFixtures(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	manifestPath := filepath.Join(root, "test", "fixtures", "fixtures.yaml")
	m := fixtures.MustLoad(manifestPath)
	fixtureRoot := fixtures.RootDir(manifestPath)

	for _, fx := range fixtures.FilterRF(m) {
		fx := fx
		t.Run(fx.ID, func(t *testing.T) {
			if reason, ok := fixtures.UpstreamFixtureBugs[fx.ID]; ok {
				t.Skip(reason)
			}
			path := filepath.Join(fixtureRoot, filepath.FromSlash(fx.ID))
			if _, err := os.Stat(path); err != nil {
				t.Skipf("fixture not on disk (run sync-upstream / generate zip): %v", err)
			}
			opts := excsv.StrictOptions()
			opts.ExpectProfile = fx.Expect.Profile
			res, err := excsv.ParseFile(path, opts)
			fixtures.AssertExpectation(t, fx, res, err)
		})
	}
}
