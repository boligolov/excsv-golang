package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func parseComputedDoc(t *testing.T, src string) *excsv.Document {
	t.Helper()
	res, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return res.Doc
}

const computedSrc = "#!excsv version=0.5 header=1 rows=2\n" +
	"#column name=price type=decimal\n" +
	"#column name=quantity type=int\n" +
	"#column name=total type=decimal formula=\"price * quantity\"\n" +
	"price,quantity\n" +
	"10.00,3\n" +
	"2.50,4\n"

func TestMaterializeColumn(t *testing.T) {
	doc := parseComputedDoc(t, computedSrc)
	if err := doc.MaterializeColumn("total"); err != nil {
		t.Fatal(err)
	}
	if got := doc.Data.HeaderRow; len(got) != 3 || got[2] != "total" {
		t.Fatalf("header row = %v", got)
	}
	if doc.Data.Rows[0][2] != "30.00" || doc.Data.Rows[1][2] != "10.00" {
		t.Fatalf("rows = %v", doc.Data.Rows)
	}
	idx, err := doc.ColumnIndex("total")
	if err != nil {
		t.Fatalf("ColumnIndex(total): %v", err)
	}
	if idx != 2 {
		t.Fatalf("total physical index = %d, want 2", idx)
	}
	report := doc.Validate(excsv.ValidateOptions{WithData: true})
	if !report.OK() {
		t.Fatalf("validate after materialize: %+v", report.Findings)
	}

	if err := doc.DematerializeColumn("total"); err != nil {
		t.Fatal(err)
	}
	if len(doc.Data.HeaderRow) != 2 {
		t.Fatalf("header row after dematerialize = %v", doc.Data.HeaderRow)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "materialized") {
		t.Fatalf("materialized= should be cleared:\n%s", out)
	}
}

func TestMaterializeColumnRejectsNonFormula(t *testing.T) {
	doc := parseComputedDoc(t, computedSrc)
	if err := doc.MaterializeColumn("price"); err == nil {
		t.Fatal("expected error materializing a plain stored column")
	}
}

func TestMaterializeColumnRejectsAlreadyMaterialized(t *testing.T) {
	doc := parseComputedDoc(t, computedSrc)
	if err := doc.MaterializeColumn("total"); err != nil {
		t.Fatal(err)
	}
	if err := doc.MaterializeColumn("total"); err == nil {
		t.Fatal("expected error re-materializing an already-materialized column")
	}
}

func TestDematerializeColumnRejectsVirtual(t *testing.T) {
	doc := parseComputedDoc(t, computedSrc)
	if err := doc.DematerializeColumn("total"); err == nil {
		t.Fatal("expected error dematerializing a virtual (never materialized) column")
	}
}

// TestVirtualColumnDeclaredBetweenStoredColumns is the regression test for
// the declaration-order-vs-physical-position bug: a virtual computed column
// declared between two stored #column lines must not shift the physical
// index of the stored column that follows it.
func TestVirtualColumnDeclaredBetweenStoredColumns(t *testing.T) {
	src := "#!excsv version=0.5 header=1 rows=1\n" +
		"#column name=price type=decimal\n" +
		"#column name=total type=decimal formula=\"price * qty\"\n" +
		"#column name=qty type=int\n" +
		"price,qty\n" +
		"10.00,3\n"
	doc := parseComputedDoc(t, src)
	idx, err := doc.ColumnIndex("qty")
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("qty physical index = %d, want 1 (price=0, qty=1, total is virtual)", idx)
	}
	report := doc.Validate(excsv.ValidateOptions{WithData: true})
	if !report.OK() {
		t.Fatalf("validate: %+v", report.Findings)
	}
	if err := doc.MaterializeColumn("total"); err != nil {
		t.Fatal(err)
	}
	if doc.Data.Rows[0][2] != "30.00" {
		t.Fatalf("materialized total = %v", doc.Data.Rows[0])
	}
}

func TestFormulaValidationErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want excsv.ErrorKind
	}{
		{
			name: "unknown reference",
			src: "#!excsv version=0.5 header=1 rows=1\n#column name=price type=decimal\n" +
				"#column name=total type=decimal formula=\"price * missing\"\nprice\n1\n",
			want: excsv.ErrFormulaUnknownReference,
		},
		{
			name: "references computed",
			src: "#!excsv version=0.5 header=1 rows=1\n#column name=price type=decimal\n" +
				"#column name=a type=decimal formula=\"price * 2\"\n" +
				"#column name=b type=decimal formula=\"a * 2\"\nprice\n1\n",
			want: excsv.ErrFormulaReferencesComputed,
		},
		{
			name: "parse error",
			src: "#!excsv version=0.5 header=1 rows=1\n#column name=price type=decimal\n" +
				"#column name=total type=decimal formula=\"price *\"\nprice\n1\n",
			want: excsv.ErrFormulaParseError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseComputedDoc(t, tc.src)
			report := doc.Validate(excsv.ValidateOptions{})
			found := false
			for _, f := range report.Findings {
				if f.Kind == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("findings=%+v, want %s", report.Findings, tc.want)
			}
		})
	}
}

func TestFormulaIndexForbiddenFailsAtParse(t *testing.T) {
	src := "#!excsv version=0.5 header=1 rows=1\n#column name=price type=decimal\n" +
		"#column name=total index=2 type=decimal formula=\"price * 2\"\nprice\n1\n"
	_, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	if err == nil {
		t.Fatal("expected parse failure for formula= with index=")
	}
	pe, ok := err.(*excsv.ParseError)
	if !ok || pe.Issue.Kind != excsv.ErrFormulaIndexForbidden {
		t.Fatalf("err=%v", err)
	}
}

func TestFormulaRequiresHeaderFailsAtParse(t *testing.T) {
	src := "#!excsv version=0.5 header=0 rows=1\n#column index=0 name=price type=decimal\n" +
		"#column index=1 name=total type=decimal formula=\"price * 2\"\n1.00\n"
	_, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	if err == nil {
		t.Fatal("expected parse failure for formula= under header=0")
	}
	pe, ok := err.(*excsv.ParseError)
	if !ok || pe.Issue.Kind != excsv.ErrFormulaRequiresHeader {
		t.Fatalf("err=%v", err)
	}
}
