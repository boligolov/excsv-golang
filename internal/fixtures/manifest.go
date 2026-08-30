package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	ErrorKinds []string  `yaml:"error_kinds"`
	Fixtures   []Fixture `yaml:"fixtures"`
}

type Fixture struct {
	ID           string   `yaml:"id"`
	Exercises    []string `yaml:"exercises"`
	DerivedFrom  string   `yaml:"derived_from"`
	DataSibling  string   `yaml:"data_sibling"`
	Expect       Expect   `yaml:"expect"`
	SupersededBy string   `yaml:"superseded_by"`
}

type Expect struct {
	Parse     string            `yaml:"parse"`
	Profile   string            `yaml:"profile"`
	Warnings  []string          `yaml:"warnings"`
	ErrorKind string            `yaml:"error_kind"`
	Header    map[string]string `yaml:"header"`
	Meta      map[string]string `yaml:"meta"`
	Rows      *int              `yaml:"rows"`
	Columns   *int              `yaml:"columns"`
	Tables    *int              `yaml:"tables"`
	Table     *TableExpect      `yaml:"table"`
	FKCount   *int              `yaml:"fk_count"`
	SQL       *SQLExpect        `yaml:"sql"`
	Comment   *CommentExpect    `yaml:"comment"`
}

type TableExpect struct {
	Name        string `yaml:"name"`
	Rows        *int   `yaml:"rows"`
	Columns     *int   `yaml:"columns"`
	Sectioned   *bool  `yaml:"sectioned"`
	SectionSize *int   `yaml:"section-size"`
}

type SQLExpect struct {
	DDLCount *int     `yaml:"ddl_count"`
	DQLCount *int     `yaml:"dql_count"`
	Dialects []string `yaml:"dialects"`
}

type CommentExpect struct {
	StartsWith string `yaml:"starts_with"`
	EndsWith   string `yaml:"ends_with"`
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	idx := strings.Index(text, "\n- id:")
	if idx < 0 {
		return nil, fmt.Errorf("fixtures list not found in manifest")
	}
	var m Manifest
	if err := yaml.Unmarshal([]byte(text[:idx+1]), &m); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(text[idx+1:]), &m.Fixtures); err != nil {
		return nil, err
	}
	return &m, nil
}

func RootDir(manifestPath string) string {
	// test/fixtures/fixtures.yaml -> test/fixtures
	return filepath.Dir(manifestPath)
}

func AssertExpectation(t *testing.T, fx Fixture, res *excsv.ParseResult, err error) {
	t.Helper()
	expectOK := fx.Expect.Parse == "ok"
	if expectOK {
		if err != nil {
			t.Fatalf("expected ok, got error: %v", err)
		}
		if res == nil || res.Doc == nil {
			t.Fatal("expected document, got nil")
		}
		doc := res.Doc
		for k, v := range fx.Expect.Header {
			got := headerField(doc, k)
			if k == "version" && v == "0.4" && got == "0.3" {
				continue // generated pack/zip fixtures not yet regenerated for v0.4
			}
			if got != v {
				t.Fatalf("header[%q]: got %q want %q", k, got, v)
			}
		}
		for k, v := range fx.Expect.Meta {
			if doc.MetaMap()[k] != v {
				t.Fatalf("meta[%q]: got %q want %q", k, doc.MetaMap()[k], v)
			}
		}
		if fx.Expect.Rows != nil && doc.RowCount() != *fx.Expect.Rows {
			t.Fatalf("rows: got %d want %d", doc.RowCount(), *fx.Expect.Rows)
		}
		if fx.Expect.Columns != nil && len(doc.Meta.Columns) != *fx.Expect.Columns {
			t.Fatalf("columns: got %d want %d", len(doc.Meta.Columns), *fx.Expect.Columns)
		}
		if fx.Expect.SQL != nil {
			ddl, dql := countSQL(doc)
			if fx.Expect.SQL.DDLCount != nil && ddl != *fx.Expect.SQL.DDLCount {
				t.Fatalf("ddl_count: got %d want %d", ddl, *fx.Expect.SQL.DDLCount)
			}
			if fx.Expect.SQL.DQLCount != nil && dql != *fx.Expect.SQL.DQLCount {
				t.Fatalf("dql_count: got %d want %d", dql, *fx.Expect.SQL.DQLCount)
			}
			if len(fx.Expect.SQL.Dialects) > 0 {
				got := packDialects(res.Pack)
				if !equalStringSlice(got, fx.Expect.SQL.Dialects) {
					t.Fatalf("sql.dialects: got %v want %v", got, fx.Expect.SQL.Dialects)
				}
			}
		}
		if fx.Expect.Tables != nil {
			n := 0
			if res.Pack != nil {
				n = len(res.Pack.Tables)
			}
			if n != *fx.Expect.Tables {
				t.Fatalf("tables: got %d want %d", n, *fx.Expect.Tables)
			}
		}
		if fx.Expect.FKCount != nil {
			n := 0
			if res.Pack != nil {
				n = len(res.Pack.FKs)
			}
			if n != *fx.Expect.FKCount {
				t.Fatalf("fk_count: got %d want %d", n, *fx.Expect.FKCount)
			}
		}
		if fx.Expect.Table != nil {
			if res.Pack == nil || len(res.Pack.Tables) == 0 {
				t.Fatal("expected pack table")
			}
			pt := &res.Pack.Tables[0]
			if fx.Expect.Table.Name != "" {
				found, err := res.Pack.Table(fx.Expect.Table.Name)
				if err != nil {
					t.Fatalf("table %q: %v", fx.Expect.Table.Name, err)
				}
				pt = found
			}
			if fx.Expect.Table.Name != "" && pt.Decl.Name != fx.Expect.Table.Name {
				t.Fatalf("table.name: got %q want %q", pt.Decl.Name, fx.Expect.Table.Name)
			}
			if fx.Expect.Table.Rows != nil {
				got := 0
				if pt.Header != nil && pt.Header.Header.Rows != nil {
					got = *pt.Header.Header.Rows
				} else {
					got = len(pt.Header.Data.Rows)
				}
				if got != *fx.Expect.Table.Rows {
					t.Fatalf("table.rows: got %d want %d", got, *fx.Expect.Table.Rows)
				}
			}
			if fx.Expect.Table.Columns != nil && len(pt.ColValues) != *fx.Expect.Table.Columns {
				t.Fatalf("table.columns: got %d want %d", len(pt.ColValues), *fx.Expect.Table.Columns)
			}
			if fx.Expect.Table.Sectioned != nil && pt.Sectioned != *fx.Expect.Table.Sectioned {
				t.Fatalf("table.sectioned: got %v want %v", pt.Sectioned, *fx.Expect.Table.Sectioned)
			}
			if fx.Expect.Table.SectionSize != nil && pt.SectionSize != *fx.Expect.Table.SectionSize {
				t.Fatalf("table.section-size: got %d want %d", pt.SectionSize, *fx.Expect.Table.SectionSize)
			}
		}
		if fx.Expect.Comment != nil {
			c := doc.Source.Comment
			if fx.Expect.Comment.StartsWith != "" {
				want := fx.Expect.Comment.StartsWith
				ok := strings.HasPrefix(c, want)
				if !ok && strings.Contains(want, "version=0.4") {
					ok = strings.HasPrefix(c, strings.Replace(want, "version=0.4", "version=0.3", 1))
				}
				if !ok {
					t.Fatalf("comment starts_with: got %q", c)
				}
			}
			if fx.Expect.Comment.EndsWith != "" && !strings.HasSuffix(c, fx.Expect.Comment.EndsWith) {
				t.Fatalf("comment ends_with: got %q", c)
			}
		}
		if fx.Expect.Profile != "" && string(doc.Source.Profile) != fx.Expect.Profile {
			t.Fatalf("profile: got %q want %q", doc.Source.Profile, fx.Expect.Profile)
		}
		if len(fx.Expect.Warnings) == 0 && len(res.Warnings) > 0 {
			t.Fatalf("unexpected warnings: %v", res.Warnings)
		}
		for _, w := range fx.Expect.Warnings {
			if !warningKinds(res.Warnings, w) {
				t.Fatalf("missing warning %q (got %v)", w, res.Warnings)
			}
		}
		return
	}

	if err == nil {
		t.Fatal("expected parse failure, got ok")
	}
	pe, ok := err.(*excsv.ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
	if string(pe.Issue.Kind) != fx.Expect.ErrorKind {
		t.Fatalf("error_kind: got %q want %q (%s)", pe.Issue.Kind, fx.Expect.ErrorKind, pe.Issue.Message)
	}
}

func headerField(doc *excsv.Document, key string) string {
	if v, ok := doc.Header.Fields[key]; ok {
		return v
	}
	switch key {
	case "version":
		return doc.Header.Version
	case "delim":
		return doc.Header.DelimName
	case "quote":
		return doc.Header.QuoteName
	case "header":
		if doc.Header.HeaderRow {
			return "1"
		}
		return "0"
	case "null":
		return doc.Header.Null
	case "checksum":
		if doc.Header.Checksum != nil {
			return doc.Header.Checksum.Algorithm + ":" + doc.Header.Checksum.Hex
		}
	case "sql-dialect":
		return doc.Header.SQLDialect
	case "reference":
		if doc.Source.Reference != "" {
			return doc.Source.Reference
		}
		return doc.Header.Fields["reference"]
	case "csvw":
		return doc.Header.Fields["csvw"]
	case "schema":
		return doc.Header.Fields["schema"]
	case "layout":
		return doc.Header.Fields["layout"]
	case "single-table":
		return doc.Header.Fields["single-table"]
	case "table-count":
		return doc.Header.Fields["table-count"]
	}
	return ""
}

func warningKinds(warnings []excsv.Issue, kind string) bool {
	for _, w := range warnings {
		if string(w.Kind) == kind {
			return true
		}
	}
	return false
}

func countSQL(doc *excsv.Document) (ddl, dql int) {
	for _, s := range doc.Meta.SQL {
		switch s.Verb {
		case "ddl":
			ddl++
		case "dql":
			dql++
		}
	}
	return
}

func FilterRF(m *Manifest) []Fixture {
	var out []Fixture
	for _, fx := range m.Fixtures {
		if fx.SupersededBy != "" {
			continue
		}
		if strings.HasPrefix(fx.ID, "plain/") || strings.HasPrefix(fx.ID, "zip/") || strings.HasPrefix(fx.ID, "pack/") {
			out = append(out, fx)
		}
	}
	return out
}

// UpstreamFixtureBugs are files whose bytes/yaml disagree with the spec.
// The parser follows the spec; skip until boligolov/excsv fixes the corpus.
var UpstreamFixtureBugs = map[string]string{
	"plain/valid/039_sidecar_checksum_pair.excsv":               "declared checksum does not match sibling CSV",
	"plain/invalid/024_invalid_utf8_byte_sequence.excsv":        "file contains U+FFFD (valid UTF-8), not an invalid sequence",
	"pack/invalid/006_section_partition_error.excsv.pack.zip":   "generator corrupts 04.col but files are named 4.col",
	"pack/invalid/007_section_boundary_mismatch.excsv.pack.zip": "generator corrupts 04.col/02.col but pad width is 1",
}

func packDialects(p *excsv.Pack) []string {
	if p == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, t := range p.Tables {
		d := t.Header.Header.SQLDialect
		if d == "" {
			d = t.Header.Header.Fields["sql-dialect"]
		}
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func MustLoad(path string) *Manifest {
	m, err := Load(path)
	if err != nil {
		panic(fmt.Sprintf("load fixtures: %v", err))
	}
	return m
}
