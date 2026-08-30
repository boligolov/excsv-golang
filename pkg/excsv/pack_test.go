package excsv_test

import (
	"archive/zip"
	"bytes"
	"testing"

	yekazip "github.com/yeka/zip"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestPackSectionPartitionError(t *testing.T) {
	data := packZip(t, []zipEntry{
		{"_manifest.excsv", "#!excsv version=0.3 layout=pack table-count=1 original-size=1\n#table name=items dir=items/ columns=2 original-size=1\n"},
		{"items/_header.excsv", "#!excsv version=0.3 layout=columnar rows=5 section-size=2\n#column name=id type=int\n#column name=name type=string\n"},
		{"items/00-id/0.col", "1\n2\n"},
		{"items/00-id/2.col", "3\n4\n"},
		{"items/00-id/4.col", "5\n6\n"},
		{"items/01-name/0.col", "a\nb\n"},
		{"items/01-name/2.col", "c\nd\n"},
		{"items/01-name/4.col", "e\n"},
	})
	_, err := excsv.ParsePath("x.excsv.pack.zip", data, excsv.StrictOptions())
	if err == nil {
		t.Fatal("expected fail")
	}
	pe := err.(*excsv.ParseError)
	if pe.Issue.Kind != excsv.ErrPackSectionPartition {
		t.Fatalf("got %s want pack_section_partition_error", pe.Issue.Kind)
	}
}

func TestPackSectionBoundaryMismatch(t *testing.T) {
	data := packZip(t, []zipEntry{
		{"_manifest.excsv", "#!excsv version=0.3 layout=pack table-count=1 original-size=1\n#table name=items dir=items/ columns=2 original-size=1\n"},
		{"items/_header.excsv", "#!excsv version=0.3 layout=columnar rows=5 section-size=2\n#column name=id type=int\n#column name=name type=string\n"},
		{"items/00-id/0.col", "1\n2\n"},
		{"items/00-id/2.col", "3\n4\n"},
		{"items/00-id/4.col", "5\n"},
		{"items/01-name/0.col", "a\nb\n"},
		{"items/01-name/2.col", "c\n"},
		{"items/01-name/4.col", "e\n"},
	})
	_, err := excsv.ParsePath("x.excsv.pack.zip", data, excsv.StrictOptions())
	if err == nil {
		t.Fatal("expected fail")
	}
	pe := err.(*excsv.ParseError)
	if pe.Issue.Kind != excsv.ErrPackSectionBoundary {
		t.Fatalf("got %s want pack_section_boundary_mismatch", pe.Issue.Kind)
	}
}

func TestPackCreateRoundTrip(t *testing.T) {
	src := []byte("#!excsv version=0.3\n#column name=id type=int\n#column name=n type=string\nid,n\n1,a\n2,b\n")
	res, err := excsv.ParseBytes(src, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	pack := excsv.PackFromDocument(res.Doc, "contacts")
	zipped, err := pack.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	got, err := excsv.ParsePath("out.excsv.pack.zip", zipped, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got.Pack == nil || len(got.Pack.Tables) != 1 {
		t.Fatalf("tables=%v", got.Pack)
	}
	if got.Pack.Tables[0].Decl.Name != "contacts" {
		t.Fatalf("name=%s", got.Pack.Tables[0].Decl.Name)
	}
	if got.Pack.Tables[0].Header.RowCount() != 2 {
		t.Fatalf("rows=%d", got.Pack.Tables[0].Header.RowCount())
	}
}

func TestPackEncryptedPassword(t *testing.T) {
	var buf bytes.Buffer
	zw := yekazip.NewWriter(&buf)
	w, err := zw.Encrypt("_manifest.excsv", "secret", yekazip.AES256Encryption)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("#!excsv version=0.3 layout=pack table-count=0 original-size=0\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	enc := buf.Bytes()
	_, err = excsv.ParsePath("x.excsv.pack.zip", enc, excsv.StrictOptions())
	if err == nil {
		t.Fatal("expected zip_encrypted")
	}
	pe := err.(*excsv.ParseError)
	if pe.Issue.Kind != excsv.ErrZipEncrypted {
		t.Fatalf("got %s", pe.Issue.Kind)
	}
	opts := excsv.StrictOptions()
	opts.ZipPassword = "secret"
	got, err := excsv.ParsePath("x.excsv.pack.zip", enc, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pack == nil {
		t.Fatal("expected pack")
	}
}

type zipEntry struct{ name, body string }

func packZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
