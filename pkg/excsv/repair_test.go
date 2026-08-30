package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestSetHeaderFieldConvertsDelim(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.3\n#column name=id\n#column name=n\nid,n\n1,a\n2,b\n")
	if err := doc.SetHeaderField("delim", "tab"); err != nil {
		t.Fatal(err)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "delim=tab") {
		t.Fatalf("header: %s", text)
	}
	if !strings.Contains(text, "1\ta") {
		t.Fatalf("data not converted:\n%s", text)
	}
	if strings.Contains(text, "1,a") {
		t.Fatalf("old delim remains:\n%s", text)
	}
}

func TestSetHeaderFieldHeaderZero(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.3\n#column name=id\n#column name=n\nid,n\n1,a\n")
	if err := doc.SetHeaderField("header", "0"); err != nil {
		t.Fatal(err)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "header=0") {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, "\nid,n\n") {
		t.Fatalf("header row still emitted:\n%s", text)
	}
	if !strings.Contains(text, "1,a") {
		t.Fatalf("body lost:\n%s", text)
	}
}

func TestSetHeaderFieldNullRemap(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.3\n#column name=id\n#column name=n\nid,n\n1,\n")
	if err := doc.SetHeaderField("null", "NA"); err != nil {
		t.Fatal(err)
	}
	if doc.Data.Rows[0][1] != "NA" {
		t.Fatalf("cell=%q", doc.Data.Rows[0][1])
	}
}

func TestTidyPadsRows(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.3\n#column name=a\n#column name=b\na,b\n1,2\n")
	doc.Data.Rows[0] = []string{"1"}
	if err := doc.Tidy(); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data.Rows[0]) != 2 {
		t.Fatalf("row=%v", doc.Data.Rows[0])
	}
}

func TestFixInfersAndChecksums(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.4\nid,amount\n1,10.5\n2,3\n")
	report, err := doc.Fix(excsv.FixOptions{Only: []string{excsv.FixColumns, excsv.FixChecksum, excsv.FixStamp}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) == 0 {
		t.Fatal("expected changes")
	}
	if len(doc.Meta.Columns) != 2 {
		t.Fatalf("columns=%d", len(doc.Meta.Columns))
	}
	if got := doc.Meta.Columns[1].Attrs["type"]; got != "double" && got != "decimal" && got != "int" {
		t.Fatalf("amount type=%q", got)
	}
	if doc.Header.Checksum == nil {
		t.Fatal("missing checksum")
	}
	got := ""
	for _, kv := range doc.Meta.FileMeta {
		if kv.Key == "exported" {
			got = kv.Value
		}
	}
	if got == "" {
		t.Fatal("missing #@exported")
	}
	if len(doc.Meta.Aggregations) != 0 {
		t.Fatal("fix must not invent aggregations")
	}
}

func TestValidateEscalatesRowsMismatch(t *testing.T) {
	src := []byte("#!excsv version=0.4 rows=99\n#column name=a\na\n1\n")
	res, err := excsv.ParseBytes(src, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	report := res.Doc.Validate(excsv.ValidateOptions{WithData: true})
	if report.OK() {
		t.Fatal("expected rows_mismatch failure")
	}
	if report.Findings[0].Kind != excsv.ErrRowsMismatch {
		t.Fatalf("kind=%s", report.Findings[0].Kind)
	}
}

func TestQuoteNoneRejectsDelimiterInValue(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.3 quote=double\n#column name=a\n#column name=b\na,b\n\"x,y\",z\n")
	if err := doc.SetHeaderField("quote", "none"); err == nil {
		t.Fatal("expected fail")
	}
}
