package excsv_test

import (
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
			path := filepath.Join(fixtureRoot, filepath.FromSlash(fx.ID))
			res, err := excsv.ParseFile(path, excsv.StrictOptions())
			fixtures.AssertExpectation(t, fx, res, err)
		})
	}
}
