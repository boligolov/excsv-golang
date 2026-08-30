package excsv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// DefaultAggregations is what `convert --agg default` expands to.
var DefaultAggregations = []string{"count_nonnull", "count_null", "sum", "min", "max"}

// SetHeaderField converts the document to match KEY=VALUE: dialect changes
// rewrite the data section; derived fields are resynced. Empty value clears
// optional keys (checksum, null, sql-dialect, csvw, reference).
func (doc *Document) SetHeaderField(key, value string) error {
	if doc == nil {
		return fmt.Errorf("nil document")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("header key is required")
	}
	if doc.Header.Fields == nil {
		doc.Header.Fields = map[string]string{}
	}

	switch key {
	case "rows":
		return doc.SyncDerived()
	case "original-size":
		return fmt.Errorf("original-size= is derived on ZIP/pack write")
	case "encoding":
		enc := strings.TrimSpace(value)
		if enc != "" && !strings.EqualFold(enc, "UTF-8") && !strings.EqualFold(enc, "UTF8") {
			return fail(ErrEncodingUnsupported, 1, "header set encoding= rewrites as UTF-8 only")
		}
		if enc == "" {
			delete(doc.Header.Fields, "encoding")
			doc.Header.Encoding = "UTF-8"
		} else {
			doc.Header.Fields["encoding"] = "UTF-8"
			doc.Header.Encoding = "UTF-8"
		}
		return doc.SyncDerived()
	case "delim":
		if strings.TrimSpace(value) == "" {
			delete(doc.Header.Fields, "delim")
			if err := applyHeaderDefaults(&doc.Header); err != nil {
				return err
			}
			return doc.SyncDerived()
		}
		return doc.convertDelim(value)
	case "quote":
		return doc.convertQuote(value)
	case "header":
		return doc.convertHeaderRow(value)
	case "null":
		return doc.convertNull(value)
	case "checksum":
		return doc.convertChecksum(value)
	case "version":
		if strings.TrimSpace(value) == "" {
			return fail(ErrHeaderMissingVersion, 1, "version= cannot be empty")
		}
		doc.Header.Fields["version"] = value
		doc.Header.Version = value
		return applyHeaderDefaults(&doc.Header)
	case "layout":
		if value != "pack" && value != "columnar" && value != "" {
			return fail(ErrHeaderInvalidValue, 1, "layout must be pack or columnar")
		}
		return doc.setOptionalField("layout", value)
	case "section-size":
		if value != "" && value != "0" {
			if _, err := parseIntField(value); err != nil {
				return fail(ErrHeaderInvalidValue, 1, "invalid section-size="+value)
			}
		}
		return doc.setOptionalField("section-size", value)
	case "single-table", "table-count", "sql-dialect", "reference":
		if key == "sql-dialect" {
			doc.Header.SQLDialect = value
		}
		return doc.setOptionalField(key, value)
	default:
		if value == "" {
			delete(doc.Header.Fields, key)
			return nil
		}
		doc.Header.Fields[key] = value
		return applyHeaderDefaults(&doc.Header)
	}
}

func (doc *Document) setOptionalField(key, value string) error {
	if value == "" {
		delete(doc.Header.Fields, key)
	} else {
		doc.Header.Fields[key] = value
	}
	return applyHeaderDefaults(&doc.Header)
}

func (doc *Document) convertDelim(value string) error {
	if strings.TrimSpace(value) == "" {
		return fail(ErrHeaderInvalidValue, 1, "empty delimiter")
	}
	r, err := resolveDelim(value)
	if err != nil {
		return err
	}
	name := value
	for n, wr := range wellKnownDelims {
		if wr == r {
			name = n
			break
		}
	}
	if doc.Header.QuoteName == "none" || !doc.Header.QuoteEnabled {
		if cellsContainRune(doc, r) {
			doc.Header.Fields["quote"] = "double"
		}
	}
	doc.Header.Fields["delim"] = name
	doc.Header.DelimName = name
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return err
	}
	return doc.SyncDerived()
}

func (doc *Document) convertQuote(value string) error {
	if value == "" {
		value = "none"
	}
	q, enabled, err := resolveQuote(value)
	if err != nil {
		return err
	}
	_ = q
	name := value
	if _, ok := wellKnownQuotes[value]; ok {
		name = value
	}
	if !enabled && cellsContainRune(doc, doc.Header.Delim) {
		return fail(ErrQuoteNoneDelimiterInValue, 0, "quote=none but a value contains the delimiter")
	}
	doc.Header.Fields["quote"] = name
	doc.Header.QuoteName = name
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return err
	}
	return doc.SyncDerived()
}

func (doc *Document) convertHeaderRow(value string) error {
	if value != "0" && value != "1" {
		return fail(ErrHeaderInvalidValue, 1, "invalid header="+value)
	}
	want := value == "1"
	if want && !doc.Data.HasHeaderRow {
		names := doc.Data.HeaderRow
		if len(names) == 0 {
			names = columnNamesFromHeader(doc)
		}
		if len(names) == 0 {
			w := doc.columnWidth()
			names = make([]string, w)
			for i := range names {
				names[i] = fmt.Sprintf("col%d", i)
			}
		}
		doc.Data.HeaderRow = names
		doc.Data.HasHeaderRow = true
	}
	if !want {
		doc.Data.HasHeaderRow = false
	}
	doc.Header.Fields["header"] = value
	doc.Header.HeaderRow = want
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return err
	}
	return doc.SyncDerived()
}

func (doc *Document) convertNull(value string) error {
	old := doc.Header.Null
	rewriteNullCells(doc, old, value)
	if value == "" {
		delete(doc.Header.Fields, "null")
		doc.Header.Null = ""
	} else {
		doc.Header.Fields["null"] = value
		doc.Header.Null = value
	}
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return err
	}
	return doc.SyncDerived()
}

func (doc *Document) convertChecksum(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(doc.Header.Fields, "checksum")
		doc.Header.Checksum = nil
		return nil
	}
	alg := value
	if i := strings.IndexByte(value, ':'); i > 0 {
		alg = value[:i]
		if strings.TrimSpace(value[i+1:]) != "" {
			doc.Header.Fields["checksum"] = value
			cs, kind := classifyChecksumField(value)
			if kind != "" {
				return fail(kind, 1, "checksum="+value)
			}
			doc.Header.Checksum = cs
			return nil
		}
	}
	return doc.SetDataChecksum(alg)
}

func rewriteNullCells(doc *Document, old, neu string) {
	mapCell := func(s string) string {
		isNull := s == "" || (old != "" && s == old)
		if !isNull {
			return s
		}
		if neu == "" {
			return ""
		}
		return neu
	}
	for i, row := range doc.Data.Rows {
		for c, v := range row {
			doc.Data.Rows[i][c] = mapCell(v)
		}
	}
}

func cellsContainRune(doc *Document, r rune) bool {
	d := string(r)
	if d == "" {
		return false
	}
	if doc.Data.HasHeaderRow {
		for _, c := range doc.Data.HeaderRow {
			if strings.Contains(c, d) {
				return true
			}
		}
	}
	for _, row := range doc.Data.Rows {
		for _, c := range row {
			if strings.Contains(c, d) {
				return true
			}
		}
	}
	return false
}

// Tidy repairs ragged rows, canonicalizes meta order, NFC-normalizes cells,
// strips C0 controls, and resyncs derived header fields.
func (doc *Document) Tidy() error {
	if doc == nil {
		return fmt.Errorf("nil document")
	}
	width := doc.columnWidth()
	if width > 0 {
		for i, row := range doc.Data.Rows {
			doc.Data.Rows[i] = padOrTrim(row, width)
		}
		if doc.Data.HasHeaderRow {
			doc.Data.HeaderRow = padOrTrim(doc.Data.HeaderRow, width)
		}
	}
	for i, row := range doc.Data.Rows {
		for c, v := range row {
			doc.Data.Rows[i][c] = tidyCell(v)
		}
	}
	if doc.Data.HasHeaderRow {
		for i, v := range doc.Data.HeaderRow {
			doc.Data.HeaderRow[i] = tidyCell(v)
		}
	}
	doc.dedupeFileMeta()
	doc.sortMeta()
	if doc.Header.QuoteName == "none" || !doc.Header.QuoteEnabled {
		if cellsContainRune(doc, doc.Header.Delim) {
			doc.Header.Fields["quote"] = "double"
			if err := applyHeaderDefaults(&doc.Header); err != nil {
				return err
			}
		}
	}
	return doc.SyncDerived()
}

func tidyCell(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		if unicode.Is(unicode.Cc, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (doc *Document) dedupeFileMeta() {
	var out []KV
	for _, kv := range doc.Meta.FileMeta {
		out = upsertKV(out, kv.Key, kv.Value)
	}
	doc.Meta.FileMeta = out
}

func (doc *Document) sortMeta() {
	sort.SliceStable(doc.Meta.FileMeta, func(i, j int) bool {
		return doc.Meta.FileMeta[i].Key < doc.Meta.FileMeta[j].Key
	})
	sort.SliceStable(doc.Meta.Aggregations, func(i, j int) bool {
		return doc.Meta.Aggregations[i].Name < doc.Meta.Aggregations[j].Name
	})
}

// InferColumns adds #column stubs from the header row (or colN) when none exist.
func (doc *Document) InferColumns() {
	if len(doc.Meta.Columns) > 0 {
		return
	}
	titles := append([]string{}, doc.Data.HeaderRow...)
	if len(titles) == 0 {
		titles = columnNamesFromHeader(doc)
	}
	doc.InferColumnsFromHeader(titles, doc.Data.HasHeaderRow)
}

// InferColumnsFromHeader declares one #column per physical column, with an
// inferred type=. name= is a sanitized identifier and title= keeps the raw
// header text when the two differ, so "Total Sales" becomes
// `name=total_sales title="Total Sales"` rather than a conversion error.
func (doc *Document) InferColumnsFromHeader(titles []string, hasHeader bool) {
	if len(doc.Meta.Columns) > 0 {
		return
	}
	width := doc.columnWidth()
	if len(titles) < width {
		titles = append(append([]string{}, titles...), make([]string, width-len(titles))...)
	}
	used := map[string]bool{}
	for i, title := range titles {
		name := uniqueColumnName(sanitizeColumnName(title, i), used)
		attrs := map[string]string{"name": name, "type": sniffColumnType(doc, i)}
		if hasHeader && title != "" && title != name {
			attrs["title"] = title
		}
		if !hasHeader {
			attrs["index"] = strconv.Itoa(i)
		}
		doc.Meta.Columns = append(doc.Meta.Columns, ColumnDef{Attrs: attrs})
	}
}

// sanitizeColumnName maps arbitrary header text onto ^[A-Za-z_][A-Za-z0-9_-]*$.
func sanitizeColumnName(raw string, index int) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return fmt.Sprintf("col%d", index)
	}
	if c := name[0]; !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && c != '_' {
		name = "_" + name
	}
	return name
}

func uniqueColumnName(name string, used map[string]bool) string {
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s_%d", name, i)
	}
	used[candidate] = true
	return candidate
}

func sniffColumnType(doc *Document, col int) string {
	saw := false
	allInt := true
	allNum := true
	for _, row := range doc.Data.Rows {
		v := cellAt(doc, row, col)
		if isNullCell(doc, v) {
			continue
		}
		saw = true
		if _, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err != nil {
			allInt = false
		}
		if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err != nil {
			allNum = false
			break
		}
	}
	if !saw {
		return "string"
	}
	if allInt {
		return "int"
	}
	if allNum {
		return "double"
	}
	return "string"
}

func (p *Pack) Fix(opts FixOptions) (FixReport, error) {
	report := FixReport{DryRun: opts.DryRun}
	if p == nil {
		return report, fmt.Errorf("nil pack")
	}
	for i := range p.Tables {
		r, err := p.Tables[i].Header.Fix(opts)
		if err != nil {
			return report, err
		}
		report.merge(r)
		if !opts.DryRun {
			p.Tables[i].SyncFromDocument(p.Tables[i].Header)
		}
	}
	if !opts.DryRun {
		p.syncManifest()
	}
	return report, nil
}
