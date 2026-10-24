package excsv_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/boligolov/excsv-golang/internal/fixtures"
	"github.com/boligolov/excsv-golang/pkg/excsv"
)

// assertEquivalent checks that a JSON-form fixture reads as the same document
// as its plain twin (json.md § Reading): their JSON exports are identical.
func assertEquivalent(t *testing.T, res *excsv.ParseResult, twinPath string, opts excsv.ParseOptions) {
	t.Helper()
	twin, err := excsv.ParseFile(twinPath, opts)
	if err != nil {
		t.Fatalf("equivalent_to %s: %v", twinPath, err)
	}
	got, want := exportJSON(t, res.Doc), exportJSON(t, twin.Doc)
	if !bytes.Equal(got, want) {
		t.Fatalf("not equivalent to %s:\n%s\n---\n%s", twinPath, got, want)
	}
}

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
			if fx.EquivalentTo != "" {
				assertEquivalent(t, res, filepath.Join(fixtureRoot, filepath.FromSlash(fx.EquivalentTo)), opts)
			}
		})
	}
}
