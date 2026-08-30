package excsvzip_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func TestInspectWithoutDecompress(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	path := filepath.Join(root, "test", "fixtures", "zip", "valid", "004_comment_full.excsv.zip")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("fixtures not synced")
	}
	ins, err := excsvzip.Inspect(path, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ins.Comment, "#!excsv") {
		t.Fatalf("expected #!excsv comment, got %q", ins.Comment)
	}
	if ins.UncompressedSize <= 0 {
		t.Fatal("expected positive uncompressed size")
	}
}
