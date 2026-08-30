package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestImportDelimited_MinimalCSV(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("a,b\n1,2\n"), excsv.ImportOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	doc := res.Doc
	if doc.Header.Version != "0.4" {
		t.Fatalf("version=%q", doc.Header.Version)
	}
	if doc.Header.DelimName != "comma" {
		t.Fatalf("delim=%q", doc.Header.DelimName)
	}
	if !doc.Data.HasHeaderRow || len(doc.Data.HeaderRow) != 2 {
		t.Fatalf("header=%v", doc.Data.HeaderRow)
	}
	if len(doc.Data.Rows) != 1 || doc.Data.Rows[0][0] != "1" {
		t.Fatalf("rows=%v", doc.Data.Rows)
	}
	if doc.Header.Rows == nil || *doc.Header.Rows != 1 {
		t.Fatalf("rows field=%v", doc.Header.Rows)
	}

	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := excsv.ParseBytes(out, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Doc.RowCount() != 1 {
		t.Fatalf("round-trip rows=%d", parsed.Doc.RowCount())
	}
}

func TestImportDelimited_TSVSniff(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("a\tb\n1\t2\n"), excsv.ImportOptions{
		Strict:     true,
		SourcePath: "data.tsv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.DelimName != "tab" {
		t.Fatalf("delim=%q", res.Doc.Header.DelimName)
	}
}

func TestImportDelimited_QuotedCSV(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte(`"a,b",c`+"\n"+`"d,e",f`+"\n"), excsv.ImportOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.QuoteName != "double" {
		t.Fatalf("quote=%q", res.Doc.Header.QuoteName)
	}
	if res.Doc.Data.HeaderRow[0] != "a,b" {
		t.Fatalf("header=%v", res.Doc.Data.HeaderRow)
	}
}

func TestImportDelimited_NoHeader(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("1,2\n3,4\n"), excsv.ImportOptions{Strict: true, NoHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.HeaderRow {
		t.Fatal("expected header=0")
	}
	if len(res.Doc.Data.Rows) != 2 {
		t.Fatalf("rows=%d", len(res.Doc.Data.Rows))
	}
}

func TestImportDelimited_Columns(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("id,name\n1,alice\n"), excsv.ImportOptions{
		Strict: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Meta.Columns) != 2 {
		t.Fatalf("columns=%d", len(res.Doc.Meta.Columns))
	}
	names := map[string]bool{}
	for _, col := range res.Doc.Meta.Columns {
		names[col.Attrs["name"]] = true
	}
	if !names["id"] || !names["name"] {
		t.Fatalf("column names=%v", names)
	}
}

func TestImportDelimited_SanitizedColumnName(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("Total Sales,x\n1,2\n"), excsv.ImportOptions{
		Strict: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Meta.Columns) != 2 {
		t.Fatalf("columns=%d", len(res.Doc.Meta.Columns))
	}
	if res.Doc.Meta.Columns[0].Attrs["name"] != "Total_Sales" {
		t.Fatalf("name=%q", res.Doc.Meta.Columns[0].Attrs["name"])
	}
	if res.Doc.Meta.Columns[0].Attrs["title"] != "Total Sales" {
		t.Fatalf("title=%q", res.Doc.Meta.Columns[0].Attrs["title"])
	}
}

func TestImportDelimited_Checksum(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("a,b\n1,2\n"), excsv.ImportOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.Checksum == nil {
		t.Fatal("missing checksum")
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := excsv.ParseBytes(out, excsv.StrictOptions()); err != nil {
		t.Fatal(err)
	}
}

func TestImportDelimited_RaggedStrict(t *testing.T) {
	_, err := excsv.ImportDelimited([]byte("a,b\n1,2,3\n"), excsv.ImportOptions{Strict: true})
	if err == nil {
		t.Fatal("expected strict error")
	}
}

func TestImportDelimited_RaggedLenient(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("a,b\n1\n3,4,5\n"), excsv.ImportOptions{Strict: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 2 {
		t.Fatalf("warnings=%d", len(res.Warnings))
	}
	if len(res.Doc.Data.Rows[0]) != 2 || res.Doc.Data.Rows[0][1] != "" {
		t.Fatalf("padded row=%v", res.Doc.Data.Rows[0])
	}
	if len(res.Doc.Data.Rows[1]) != 2 {
		t.Fatalf("truncated row=%v", res.Doc.Data.Rows[1])
	}
}

func TestImportDelimited_Empty(t *testing.T) {
	res, err := excsv.ImportDelimited(nil, excsv.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "#!excsv version=0.4 delim=comma quote=none" {
		t.Fatalf("got %q", string(out))
	}
}

func TestImportDelimited_RoundTrip(t *testing.T) {
	input := "name,age\nalice,30\nbob,25\n"
	res, err := excsv.ImportDelimited([]byte(input), excsv.ImportOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := excsv.ParseBytes(out, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	d := parsed.Doc.Header.Dialect()
	var lines []string
	if parsed.Doc.Data.HasHeaderRow {
		lines = append(lines, excsv.JoinCSVFields(parsed.Doc.Data.HeaderRow, d))
	}
	for _, row := range parsed.Doc.Data.Rows {
		lines = append(lines, excsv.JoinCSVFields(row, d))
	}
	got := strings.Join(lines, "\n") + "\n"
	if got != input {
		t.Fatalf("round-trip mismatch:\nwant %q\ngot  %q", input, got)
	}
}

func TestImportDelimited_OutputDelimReencode(t *testing.T) {
	input := "a,b\n\"1,2\",3\n"
	res, err := excsv.ImportDelimited([]byte(input), excsv.ImportOptions{
		Strict:    true,
		DelimName: "pipe",
		QuoteName: "double",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.DelimName != "pipe" {
		t.Fatalf("header delim=%q", res.Doc.Header.DelimName)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "delim=pipe") {
		t.Fatalf("missing delim=pipe in %q", string(out))
	}
	d := res.Doc.Header.Dialect()
	gotHeader := excsv.JoinCSVFields(res.Doc.Data.HeaderRow, d)
	if gotHeader != "a|b" {
		t.Fatalf("header row=%q", gotHeader)
	}
	gotRow := excsv.JoinCSVFields(res.Doc.Data.Rows[0], d)
	if gotRow != "1,2|3" {
		t.Fatalf("data row=%q", gotRow)
	}
}

func TestImportDelimited_OutputQuoteSingle(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("id,msg\n1,\"a,b\"\n"), excsv.ImportOptions{
		Strict:    true,
		DelimName: "comma",
		QuoteName: "single",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Header.QuoteName != "single" {
		t.Fatalf("quote=%q", res.Doc.Header.QuoteName)
	}
	d := res.Doc.Header.Dialect()
	got := excsv.JoinCSVFields(res.Doc.Data.Rows[0], d)
	if got != `1,'a,b'` {
		t.Fatalf("data row=%q", got)
	}
}

func TestImportDelimited_Sidecar(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("id,v\n1,2\n"), excsv.ImportOptions{
		Strict:     true,
		Sidecar:    true,
		SourcePath: "dir/data.csv",
		Reference:  "data.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.Source.Profile != excsv.ProfileSidecar {
		t.Fatalf("profile=%q", res.Doc.Source.Profile)
	}
	if res.Doc.Source.Reference != "data.csv" {
		t.Fatalf("reference=%q", res.Doc.Source.Reference)
	}
	if res.Doc.RowCount() != 0 {
		t.Fatalf("sidecar should not embed rows, got %d", res.Doc.RowCount())
	}
	if res.Doc.Header.Rows == nil || *res.Doc.Header.Rows != 1 {
		t.Fatalf("rows=%v", res.Doc.Header.Rows)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	if strings.Contains(body, "id,v") || strings.Contains(body, "\n1,2") {
		t.Fatalf("sidecar must not contain data section: %q", body)
	}
	if !strings.Contains(body, "reference=data.csv") {
		t.Fatalf("missing reference: %q", body)
	}
}

func TestImportDelimited_FileMeta(t *testing.T) {
	res, err := excsv.ImportDelimited([]byte("a\n1\n"), excsv.ImportOptions{
		Strict:   true,
		FileMeta: []excsv.KV{{Key: "source", Value: "import.csv"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc.MetaMap()["source"] != "import.csv" {
		t.Fatalf("meta=%v", res.Doc.MetaMap())
	}
}
