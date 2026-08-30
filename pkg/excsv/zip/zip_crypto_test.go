package excsvzip_test

import (
	"strings"
	"testing"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func TestWrapWithPasswordRoundTrip(t *testing.T) {
	inner := []byte("#!excsv version=0.2\n#@source: demo\nid\n1\n")
	const password = "secret"
	zipped, err := excsvzip.WrapWithPassword(inner, "data.excsv", "", password)
	if err != nil {
		t.Fatal(err)
	}
	ins, err := excsvzip.Inspect("data.excsv.zip", zipped)
	if err != nil {
		t.Fatal(err)
	}
	if !ins.Encrypted {
		t.Fatal("expected encrypted archive")
	}
	_, err = excsvzip.Extract("data.excsv.zip", zipped)
	if err == nil {
		t.Fatal("expected password required")
	}
	ext, err := excsvzip.ExtractWithPassword("data.excsv.zip", zipped, password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ext.Inner), "#@source: demo") {
		t.Fatalf("inner=%q", ext.Inner)
	}
	plain, err := excsvzip.ReWrap("data.excsv.zip", zipped, password, "")
	if err != nil {
		t.Fatal(err)
	}
	ins2, err := excsvzip.Inspect("data.excsv.zip", plain)
	if err != nil {
		t.Fatal(err)
	}
	if ins2.Encrypted {
		t.Fatal("expected decrypted archive")
	}
	ext2, err := excsvzip.Extract("data.excsv.zip", plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(ext2.Inner) != string(ext.Inner) {
		t.Fatalf("inner mismatch after decrypt")
	}
}
