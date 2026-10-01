package excsv

import (
	"fmt"
	"strings"
)

// CurrentVersion is the ExCSV spec version this package writes.
const CurrentVersion = "0.6"

// Form is the storage form a Document was read from.
type Form int

// Storage forms.
const (
	FormPlain Form = iota
	FormZipInner
	FormPack
)

// Checksum is a parsed checksum=<algorithm>:<hex> header field.
type Checksum struct {
	Algorithm string
	Hex       string
}

// KV is one ordered key/value pair (#@ metadata, #chart / #note / #link
// attributes).
type KV struct {
	Key   string
	Value string
}

// ColumnDef is one #column declaration; Line is its 1-based source line
// (0 for a column added programmatically).
type ColumnDef struct {
	Attrs map[string]string
	Line  int
}

// Aggregation is one #%<name>: line: one value per physical column, with ""
// meaning "not applicable".
type Aggregation struct {
	Name   string
	Values []string
	Line   int
}

// SQLStatement is one #$<verb>[-<dialect>[-<version>]]: line. Qualified
// reports whether the key carried an explicit dialect suffix.
type SQLStatement struct {
	Verb      string
	Dialect   string
	Version   string
	Payload   string
	RawKey    string
	Line      int
	Qualified bool
}

// TableDecl is one #table line of a pack manifest.
type TableDecl struct {
	Name         string
	Dir          string
	Columns      int
	OriginalSize int64
	Line         int
	Attrs        map[string]string
}

// ForeignKey is one #fk from=<table.col> to=<table.col> line of a pack manifest.
type ForeignKey struct {
	From string
	To   string
	Line int
}

// PackTable is one table of a pack: its _header.excsv document plus the
// column values read from its .col files.
type PackTable struct {
	Decl        TableDecl
	Header      *Document
	ColValues   [][]string
	ColNames    []string
	ColPaths    []string
	Sectioned   bool
	SectionSize int
}

// Pack is a parsed .excsv.pack.zip container.
type Pack struct {
	Manifest   *Document
	Tables     []PackTable
	FKs        []ForeignKey
	Discovered bool
}

// Table returns the table with the given name.
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

// DefaultTable returns the table commands address when none is named: the
// only table of a single-table pack, or nil when the pack has several.
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

// Document returns the table's header document.
func (t *PackTable) Document() *Document {
	if t == nil {
		return nil
	}
	return t.Header
}

// MetaBlock holds every meta line between the header line and the data
// section, grouped by kind.
type MetaBlock struct {
	FileMeta      []KV
	Columns       []ColumnDef
	Charts        []Chart
	Notes         []Note
	Links         []Link
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

// UnknownMetaLine is a "#" meta line this version does not interpret, kept
// verbatim for round-tripping.
type UnknownMetaLine struct {
	Text string
	Line int
}

// DataSection is the parsed data: the optional header row and the body rows.
type DataSection struct {
	HasHeaderRow bool
	HeaderRow    []string
	Rows         [][]string
}

// Profile is the document shape: inline data, a sidecar referencing a
// separate CSV/TSV, or a stub with metadata and no data.
type Profile string

// Document profiles.
const (
	ProfileInline  Profile = "inline"
	ProfileSidecar Profile = "sidecar"
	ProfileStub    Profile = "stub"
)

// SourceInfo records where a Document came from.
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

// Document is one parsed ExCSV table: header line, meta block and data.
type Document struct {
	Form   Form
	Header Header
	Meta   MetaBlock
	Data   DataSection
	Source SourceInfo
}

// Header is the parsed #!excsv line. Fields holds every key verbatim;
// the typed fields are resolved from it with the spec defaults applied.
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

// ParseOptions controls a parse.
type ParseOptions struct {
	// Strict rejects a v0.5+ header without rows= (header_missing_rows);
	// otherwise it is only a warning, so repair tools can open the file.
	// Other FAIL codes are rejected either way.
	Strict bool
	// ClearHumanComments drops ## comments instead of keeping them.
	ClearHumanComments bool
	// ExpectZipInner marks the input as the primary entry of a row ZIP
	// (enforces original-size=). Set by the ZIP reader.
	ExpectZipInner bool
	// ZipUncompressedSize is the primary entry's size, checked against
	// original-size=. Set by the ZIP reader.
	ZipUncompressedSize int64
	// ZipLoadData reads a row ZIP's primary entry; when false only the ZIP
	// comment is parsed (metadata-only peek).
	ZipLoadData bool
	// ZipPassword decrypts an encrypted ZIP or pack.
	ZipPassword string
	// SourcePath is the input's path, used to resolve sidecar references and
	// extension-based checks.
	SourcePath string
	// ExpectProfile, when set ("stub", "sidecar", "sidecar_strict"), makes
	// the parse fail unless the document has that profile.
	ExpectProfile string
	// ResolveReference reads a sidecar's referenced CSV/TSV.
	ResolveReference bool
	// PackRole is "manifest" or "table" when parsing a pack member, "" for
	// a standalone document.
	PackRole string
}

// StrictOptions returns the options a conforming reader uses: every FAIL
// code rejects the file, and data and references are loaded.
func StrictOptions() ParseOptions {
	return ParseOptions{Strict: true, ResolveReference: true, ZipLoadData: true}
}

// LenientOptions is StrictOptions with Strict off (see ParseOptions.Strict).
func LenientOptions() ParseOptions {
	return ParseOptions{Strict: false, ResolveReference: true, ZipLoadData: true}
}

// ParseResult is a successful parse: a Document, or a Pack for a pack
// container, plus every WARN-level issue found.
type ParseResult struct {
	Doc      *Document
	Pack     *Pack
	Warnings []Issue
}

// OK reports whether the result holds a document or a pack.
func (r *ParseResult) OK() bool {
	return r.Doc != nil || r.Pack != nil
}

// IsPackPath reports whether path names a pack container (.pack.zip).
func IsPackPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, ".pack.zip") || strings.Contains(lower, ".pack.")
}
