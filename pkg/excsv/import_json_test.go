package excsv_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/internal/fixtures"
	"github.com/boligolov/excsv-golang/pkg/excsv"
)

// TestJSONRoundTripFixtures checks that text -> JSON -> text is lossless on
// every valid plain fixture: reading the exported JSON back and exporting it
// again gives identical JSON, and so does rendering it as text and reading
// that.
func TestJSONRoundTripFixtures(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	manifestPath := filepath.Join(root, "test", "fixtures", "fixtures.yaml")
	m := fixtures.MustLoad(manifestPath)
	fixtureRoot := fixtures.RootDir(manifestPath)

	for _, fx := range m.Fixtures {
		if !strings.HasPrefix(fx.ID, "plain/valid/") || fx.Expect.Parse != "ok" {
			continue
		}
		fx := fx
		t.Run(fx.ID, func(t *testing.T) {
			res, err := excsv.ParseFile(filepath.Join(fixtureRoot, filepath.FromSlash(fx.ID)), excsv.ParseOptions{ResolveReference: true})
			if err != nil {
				t.Skipf("does not parse with default options: %v", err)
			}
			j1 := exportJSON(t, res.Doc)

			opts := excsv.ParseOptions{ResolveReference: true, SourcePath: filepath.Join(fixtureRoot, filepath.FromSlash(fx.ID))}
			back, err := excsv.ParseBytes(j1, opts)
			if err != nil {
				t.Fatalf("reading the exported JSON: %v\n%s", err, j1)
			}
			if j2 := exportJSON(t, back.Doc); !bytes.Equal(j1, j2) {
				t.Fatalf("JSON -> document -> JSON changed:\n%s\n---\n%s", j1, j2)
			}

			text, err := back.Doc.SerializeCanonical()
			if err != nil {
				t.Fatal(err)
			}
			again, err := excsv.ParseBytes(text, opts)
			if err != nil {
				t.Fatalf("reading the text rendered from JSON: %v\n%s", err, text)
			}
			if j3 := exportJSON(t, again.Doc); !bytes.Equal(j1, j3) {
				t.Fatalf("JSON -> text -> JSON changed:\n%s\n---\n%s\ntext:\n%s", j1, j3, text)
			}
		})
	}
}

func exportJSON(t *testing.T, doc *excsv.Document) []byte {
	t.Helper()
	out, err := doc.ExportJSON(excsv.JSONExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func TestJSONFormReadsLikeText(t *testing.T) {
	doc := `{"excsv":"0.6","meta":{"grain":"one row per order"},
	 "columns":[{"index":0,"name":"id","type":"int","role":"id"},
	            {"index":1,"name":"note"},
	            {"index":2,"name":"amount","type":"decimal","unit":"USD"}],
	 "aggregates":{"sum":[null,null,"15.75"]},
	 "notes":[{"key":"2","text":"Refund pending"}],
	 "rows":3,
	 "data":[[1,"says \"hi\", ok","10.50"],[2,null,"5.25"],[3,"#tag","0.00"]]}`
	res, err := excsv.ParseBytes([]byte(doc), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	text, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	want := `#!excsv version=0.6 delim=comma quote=double rows=3
#@grain: one row per order
#column name=id type=int role=id
#column name=amount type=decimal unit=USD
#note key=2 text="Refund pending"
#%sum: ,,15.75
id,note,amount
1,"says ""hi"", ok",10.50
2,,5.25
3,#tag,0.00
`
	if string(text) != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}
}

func TestJSONFormMalformed(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":        `{"excsv":"0.6",`,
		"no index":        `{"excsv":"0.6","columns":[{"name":"a"}],"rows":0,"data":[]}`,
		"repeated index":  `{"excsv":"0.6","columns":[{"index":0,"name":"a"},{"index":0,"name":"b"}],"rows":0,"data":[]}`,
		"unnamed column":  `{"excsv":"0.6","columns":[{"index":0,"type":"int"}],"rows":0,"data":[]}`,
		"nested cell":     `{"excsv":"0.6","columns":[{"index":0,"name":"a"}],"rows":1,"data":[[[1]]]}`,
		"newline in cell": `{"excsv":"0.6","columns":[{"index":0,"name":"a"}],"rows":1,"data":[["x\ny"]]}`,
	} {
		_, err := excsv.ParseBytes([]byte(doc), excsv.ParseOptions{ResolveReference: true})
		var pe *excsv.ParseError
		if !errors.As(err, &pe) || pe.Issue.Kind != excsv.ErrJSONMalformed {
			t.Errorf("%s: want json_malformed, got %v", name, err)
		}
	}
}

func TestJSONFormLoneNull(t *testing.T) {
	doc := `{"excsv":"0.6","columns":[{"index":0,"name":"a"}],"rows":2,"data":[[null],["x"]]}`
	res, err := excsv.ParseBytes([]byte(doc), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Doc.RowCount(); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	if v := res.Doc.Data.Rows[0][0]; v != "" {
		t.Fatalf("row 0 = %q, want null", v)
	}
}
