package excsv

import (
	"fmt"
	"strings"
)

const CurrentVersion = "0.5"

type Form int

const (
	FormPlain Form = iota
	FormZipInner
	FormPack
)

type Checksum struct {
	Algorithm string
	Hex       string
}

type KV struct {
	Key   string
	Value string
}

type ColumnDef struct {
	Attrs map[string]string
	Line  int
}

type Aggregation struct {
	Name   string
	Values []string
	Line   int
}

type SQLStatement struct {
	Verb      string
	Dialect   string
	Version   string
	Payload   string
	RawKey    string
	Line      int
	Qualified bool
}

type TableDecl struct {
	Name         string
	Dir          string
	Columns      int
	OriginalSize int64
	Line         int
	Attrs        map[string]string
}

type ForeignKey struct {
	From string
	To   string
	Line int
}

type PackTable struct {
	Decl        TableDecl
	Header      *Document
	ColValues   [][]string
	ColNames    []string
	ColPaths    []string
	Sectioned   bool
	SectionSize int
}

type Pack struct {
	Manifest   *Document
	Tables     []PackTable
	FKs        []ForeignKey
	Discovered bool
}

func (p *Pack) Table(name string) (*PackTable, error) {
	if p == nil {
		return nil, fmt.Errorf("not a pack")
	}
	for i := range p.Tables {
		if p.Tables[i].Decl.Name == name {
			return &p.Tables[i], nil
		}
	}
	return nil, fmt.Errorf("unknown table: %s", name)
}

func (p *Pack) DefaultTable() *PackTable {
	if p == nil || len(p.Tables) == 0 || p.Manifest == nil {
		if p != nil && len(p.Tables) == 1 {
			return &p.Tables[0]
		}
		return nil
	}
	if name := strings.TrimSpace(p.Manifest.Header.Fields["single-table"]); name != "" && len(p.Tables) == 1 {
		if t, err := p.Table(name); err == nil {
			return t
		}
	}
	if len(p.Tables) == 1 {
		return &p.Tables[0]
	}
	return nil
}

func (t *PackTable) Document() *Document {
	if t == nil {
		return nil
	}
	return t.Header
}

type MetaBlock struct {
	FileMeta      []KV
	Columns       []ColumnDef
	Aggregations  []Aggregation
	SQL           []SQLStatement
	HumanComments []string
	Tables        []TableDecl
	FKs           []ForeignKey
	// Unknown holds every "#" meta line this version cannot interpret, verbatim.
	// The spec says such lines MUST be ignored; for a writer, ignoring means
	// carrying them through, so they are re-emitted by SerializeCanonical.
	Unknown []UnknownMetaLine
}

type UnknownMetaLine struct {
	Text string
	Line int
}

type DataSection struct {
	HasHeaderRow bool
	HeaderRow    []string
	Rows         [][]string
}

type Profile string

const (
	ProfileInline  Profile = "inline"
	ProfileSidecar Profile = "sidecar"
	ProfileStub    Profile = "stub"
)

type SourceInfo struct {
	Path          string
	ZipPath       string
	Comment       string
	PrimaryName   string
	Reference     string
	ReferencePath string
	SidecarPath   string
	Profile       Profile
}

type Document struct {
	Form   Form
	Header Header
	Meta   MetaBlock
	Data   DataSection
	Source SourceInfo
}

type Header struct {
	Fields       map[string]string
	Version      string
	DelimName    string
	Delim        rune
	QuoteName    string
	Quote        rune
	QuoteEnabled bool
	Null         string
	Encoding     string
	SQLDialect   string
	HeaderRow    bool
	Rows         *int
	Checksum     *Checksum
	OriginalSize *int64
	HasMagicLine bool
}

type ParseOptions struct {
	Strict              bool
	ClearHumanComments  bool
	ExpectZipInner      bool
	ZipUncompressedSize int64
	ZipLoadData         bool
	ZipPassword         string
	SourcePath          string
	ExpectProfile       string
	ResolveReference    bool
	PackRole            string // "", "manifest", "table"
}

func StrictOptions() ParseOptions {
	return ParseOptions{Strict: true, ResolveReference: true, ZipLoadData: true}
}

func LenientOptions() ParseOptions {
	return ParseOptions{Strict: false, ResolveReference: true, ZipLoadData: true}
}

type ParseResult struct {
	Doc      *Document
	Pack     *Pack
	Warnings []Issue
}

func (r *ParseResult) OK() bool {
	return r.Doc != nil || r.Pack != nil
}

func IsPackPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, ".pack.zip") || strings.Contains(lower, ".pack.")
}
