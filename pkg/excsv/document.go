package excsv

type Form int

const (
	FormPlain Form = iota
	FormZipInner
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

type MetaBlock struct {
	FileMeta      []KV
	Columns       []ColumnDef
	Aggregations  []Aggregation
	SQL           []SQLStatement
	CSVW          *string
	HumanComments []string
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
	Schema       string
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
	SourcePath          string
	ExpectProfile       string // stub, sidecar, sidecar_strict (fixture / explicit validation)
	ResolveReference    bool   // load referenced data when reference= is set (default true)
}

func StrictOptions() ParseOptions {
	return ParseOptions{Strict: true, ResolveReference: true}
}

func LenientOptions() ParseOptions {
	return ParseOptions{Strict: false, ResolveReference: true}
}

type ParseResult struct {
	Doc      *Document
	Warnings []Issue
}

func (r *ParseResult) OK() bool {
	return r.Doc != nil
}
