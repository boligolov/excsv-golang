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
	SQL       *SQLExpect        `yaml:"sql"`
	Comment   *CommentExpect    `yaml:"comment"`
}

type SQLExpect struct {
	DDLCount int `yaml:"ddl_count"`
	DQLCount int `yaml:"dql_count"`
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
			if ddl != fx.Expect.SQL.DDLCount {
				t.Fatalf("ddl_count: got %d want %d", ddl, fx.Expect.SQL.DDLCount)
			}
			if dql != fx.Expect.SQL.DQLCount {
				t.Fatalf("dql_count: got %d want %d", dql, fx.Expect.SQL.DQLCount)
			}
		}
		if fx.Expect.Comment != nil {
			c := doc.Source.Comment
			if fx.Expect.Comment.StartsWith != "" && !strings.HasPrefix(c, fx.Expect.Comment.StartsWith) {
				t.Fatalf("comment starts_with: got %q", c)
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
		return doc.Header.Schema
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
		if strings.HasPrefix(fx.ID, "plain/") || strings.HasPrefix(fx.ID, "zip/") {
			out = append(out, fx)
		}
	}
	return out
}

func MustLoad(path string) *Manifest {
	m, err := Load(path)
	if err != nil {
		panic(fmt.Sprintf("load fixtures: %v", err))
	}
	return m
}
