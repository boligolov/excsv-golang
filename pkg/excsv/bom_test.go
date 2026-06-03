package excsv

import (
	"bytes"
	"testing"
)

func TestStripUTF8BOM(t *testing.T) {
	t.Helper()
	bom := []byte{0xEF, 0xBB, 0xBF}
	body := []byte("#!excsv version=0.2\n")
	with := append(append([]byte(nil), bom...), body...)
	got := stripUTF8BOM(with)
	if !bytes.Equal(got, body) {
		t.Fatalf("strip failed: %q", got)
	}
	if !bytes.Equal(stripUTF8BOM(body), body) {
		t.Fatal("must not strip when absent")
	}
}
