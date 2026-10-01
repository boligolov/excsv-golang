package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

const renameDoc = "#!excsv version=0.6 header=1 rows=2\n" +
	"#column name=id type=int role=id link=\"https://x.example.com/{$}?c={qty}&lit={{qty}\"\n" +
	"#column name=qty type=int\n" +
	"#column name=price type=decimal\n" +
	"#column name=total type=decimal formula=\"qty * price + coalesce(qty, 0)\"\n" +
	"#column name=label type=string formula=\"concat('qty', ' ', label_src)\"\n" +
	"#column name=label_src type=string\n" +
	"#chart type=bar name=c x=id y=qty tooltip=qty,price\n" +
	"#chart-vega: {\"encoding\":{\"x\":{\"field\":\"qty\"}}}\n" +
	"#note row=0 col=qty text=\"check\"\n" +
	"#link key=1 col=qty href=\"https://y.example.com\"\n" +
	"#$ddl: CREATE TABLE t (id INT, qty INT)\n" +
	"id,qty,price,label_src\n" +
	"1,2,3.00,a\n" +
	"2,5,1.50,b\n"

func TestRenameColumnRewritesReferences(t *testing.T) {
	doc := parseDoc(t, renameDoc)
	notices, err := doc.RenameColumn("qty", "quantity")
	if err != nil {
		t.Fatal(err)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"#column name=quantity type=int\n",
		`formula="quantity * price + coalesce(quantity, 0)"`,
		`formula="concat('qty', ' ', label_src)"`, // string literal untouched
		"x=id y=quantity tooltip=quantity,price",
		`#note row=0 col=quantity text="check"`,
		`#link key=1 col=quantity href=`,
		"link=https://x.example.com/{$}?c={quantity}&lit={{qty}", // {{ is a literal, not a placeholder
		"\nid,quantity,price,label_src\n",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("output lacks %q:\n%s", want, s)
		}
	}
	joined := strings.Join(notices, "\n")
	if !strings.Contains(joined, "#chart-vega") || !strings.Contains(joined, "#$ddl") {
		t.Fatalf("notices = %v, want the vega payload and the DDL reported", notices)
	}
	// The result is still a valid document with every reference resolving.
	res, err := excsv.ParseBytes(out, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings after rename: %v", res.Warnings)
	}
}

func TestRenameColumnRejects(t *testing.T) {
	doc := parseDoc(t, renameDoc)
	for _, tc := range []struct{ old, new string }{
		{"missing", "x"},
		{"qty", "price"},     // already exists
		{"qty", "two words"}, // spaces
		{"qty", "1qty"},      // used in a formula, not a formula identifier
		{"qty", "case"},      // reserved formula keyword
	} {
		if _, err := doc.RenameColumn(tc.old, tc.new); err == nil {
			t.Errorf("RenameColumn(%q, %q) succeeded, want an error", tc.old, tc.new)
		}
	}
	if _, _, ok := findColumn(doc, "qty"); !ok {
		t.Fatal("a rejected rename must leave the document unchanged")
	}
}

func TestRenameColumnUpdatesChecksum(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.6 header=1 rows=1\n#column name=a type=int\na\n1\n")
	if err := doc.SetDataChecksum("sha256"); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.RenameColumn("a", "b"); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.SerializeCanonical()
	res, err := excsv.ParseBytes(out, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range res.Warnings {
		if w.Kind == excsv.ErrChecksumMismatch {
			t.Fatalf("checksum not refreshed after renaming the header cell:\n%s", out)
		}
	}
}

func findColumn(doc *excsv.Document, name string) (int, excsv.ColumnDef, bool) {
	for i, c := range doc.Meta.Columns {
		if c.Attrs["name"] == name {
			return i, c, true
		}
	}
	return -1, excsv.ColumnDef{}, false
}
