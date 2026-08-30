package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func parseDoc(t *testing.T, src string) *excsv.Document {
	t.Helper()
	res, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	return res.Doc
}

func TestAppendAndSort(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.2\n#column name=id type=int role=id\n#column name=amount type=decimal role=measure\nid,amount\n2,20\n1,10\n")
	if err := doc.AppendRows([][]string{{"3", "5"}}, true); err != nil {
		t.Fatal(err)
	}
	if doc.RowCount() != 3 {
		t.Fatalf("rows=%d", doc.RowCount())
	}
	id, err := doc.ColumnIndex("amount")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SortRows([]excsv.SortKey{{Index: id, Desc: false}}); err != nil {
		t.Fatal(err)
	}
	if doc.Data.Rows[0][0] != "3" || doc.Data.Rows[1][0] != "1" {
		t.Fatalf("sorted=%v", doc.Data.Rows)
	}
	if doc.Header.Rows == nil || *doc.Header.Rows != 3 {
		t.Fatalf("rows= %v", doc.Header.Rows)
	}
}

func TestCheckSchema(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.2\n#column name=id type=int required=1\n#column name=status type=string enum=ok|bad\nid,status\n1,ok\nx,nope\n")
	issues := doc.CheckSchema()
	if len(issues) < 2 {
		t.Fatalf("issues=%v", issues)
	}
	var kinds []string
	for _, iss := range issues {
		kinds = append(kinds, string(iss.Kind)+":"+iss.Message)
	}
	joined := strings.Join(kinds, " | ")
	if !strings.Contains(joined, "not an int") || !strings.Contains(joined, "enum") {
		t.Fatalf("issues=%v", kinds)
	}
}

func TestCheckSchemaOK(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.2\n#column name=id type=int\n#column name=when type=date\nid,when\n1,2026-01-02\n")
	if issues := doc.CheckSchema(); len(issues) != 0 {
		t.Fatalf("issues=%v", issues)
	}
}

func TestUpsertColumn(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.2\nid,amount\n1,10\n")
	if err := doc.UpsertColumn("amount", map[string]string{"type": "decimal", "unit": "USD"}); err != nil {
		t.Fatal(err)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "#column name=amount type=decimal unit=USD") {
		t.Fatalf("serialize:\n%s", text)
	}
}

func TestNumericAggSkipsIDRole(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.2\n#column name=id type=int role=id\n#column name=amount type=decimal\nid,amount\n1,10.00\n2,20.00\n")
	got, err := excsv.ComputeAggregationValues(doc, "sum")
	if err != nil {
		t.Fatal(err)
	}
	have := excsv.JoinCSVFields(got, doc.Header.Dialect())
	if have != ",30.00" {
		t.Fatalf("sum=%q", have)
	}

	doc = parseDoc(t, "#!excsv version=0.2\n#column name=id type=int role=id\n#column name=qty type=int role=measure\nid,qty\n1,2\n2,3\n")
	got, err = excsv.ComputeAggregationValues(doc, "sum")
	if err != nil {
		t.Fatal(err)
	}
	have = excsv.JoinCSVFields(got, doc.Header.Dialect())
	if have != ",5.00" {
		t.Fatalf("int measure sum=%q", have)
	}
}
