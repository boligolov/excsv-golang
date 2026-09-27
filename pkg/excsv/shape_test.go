package excsv_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestHeaderMissingRows(t *testing.T) {
	src := []byte("#!excsv version=0.5\nid\n1\n")

	_, err := excsv.ParseBytes(src, excsv.StrictOptions())
	var pe *excsv.ParseError
	if !errors.As(err, &pe) || pe.Issue.Kind != excsv.ErrHeaderMissingRows {
		t.Fatalf("strict: err = %v, want header_missing_rows", err)
	}

	// Lenient reads warn instead, so `fix --only rows` can open the file.
	res, err := excsv.ParseBytes(src, excsv.LenientOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Kind != excsv.ErrHeaderMissingRows {
		t.Fatalf("lenient warnings = %v", res.Warnings)
	}
	if _, err := res.Doc.Fix(excsv.FixOptions{Only: []string{excsv.FixRows}}); err != nil {
		t.Fatal(err)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := excsv.ParseBytes(out, excsv.StrictOptions()); err != nil {
		t.Fatalf("fixed output still rejected: %v\n%s", err, out)
	}

	// rows= only became MUST in 0.5.
	if _, err := excsv.ParseBytes([]byte("#!excsv version=0.4\nid\n1\n"), excsv.StrictOptions()); err != nil {
		t.Fatalf("0.4 without rows=: %v", err)
	}
}

func TestCanonicalWriterAlwaysEmitsRows(t *testing.T) {
	res, err := excsv.ParseBytes([]byte("#!excsv version=0.5\nid\n1\n2\n"), excsv.LenientOptions())
	if err != nil {
		t.Fatal(err)
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(firstLine(out), " rows=2") {
		t.Fatalf("header = %q, want rows=2", firstLine(out))
	}
}

func TestColumnsMismatch(t *testing.T) {
	res, err := excsv.ParseBytes([]byte("#!excsv version=0.5 rows=1 columns=5\nid\n1\n"), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Kind != excsv.ErrColumnsMismatch {
		t.Fatalf("warnings = %v", res.Warnings)
	}

	// Virtual computed columns are not physical; columns=2 is right here.
	res, err = excsv.ParseBytes([]byte("#!excsv version=0.5 rows=1 columns=2\n"+
		"#column name=p type=int\n#column name=q type=int\n#column name=t type=int formula=\"p * q\"\n"+
		"p,q\n2,3\n"), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v", res.Warnings)
	}

	// Materializing adds a physical column, so a declared columns= follows.
	if err := res.Doc.MaterializeColumn("t"); err != nil {
		t.Fatal(err)
	}
	if got := res.Doc.Header.Fields["columns"]; got != "3" {
		t.Fatalf("columns= after materialize = %s, want 3", got)
	}
	if err := res.Doc.DematerializeColumn("t"); err != nil {
		t.Fatal(err)
	}
	if got := res.Doc.Header.Fields["columns"]; got != "2" {
		t.Fatalf("columns= after dematerialize = %s, want 2", got)
	}
}

func TestComputedMaterializedMismatchFailsParse(t *testing.T) {
	src := "#!excsv version=0.5 header=1 rows=1\n#column name=p type=int\n#column name=q type=int\n" +
		"#column name=t type=int formula=\"p * q\" materialized=1\np,q\n2,3\n"
	_, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	var pe *excsv.ParseError
	if !errors.As(err, &pe) || pe.Issue.Kind != excsv.ErrComputedMaterializedMismatch {
		t.Fatalf("err = %v, want computed_materialized_mismatch", err)
	}
}

func firstLine(b []byte) string {
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
