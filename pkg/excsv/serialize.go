package excsv

import (
	"bytes"
	"sort"
	"strings"
)

func (doc *Document) SerializeCanonical() ([]byte, error) {
	var b bytes.Buffer
	h := doc.buildCanonicalHeaderLine()
	b.WriteString(h)
	b.WriteByte('\n')
	for _, kv := range doc.Meta.FileMeta {
		b.WriteString("#@" + kv.Key + ": " + kv.Value)
		b.WriteByte('\n')
	}
	for _, t := range doc.Meta.Tables {
		b.WriteString(formatPackTableLine(t))
		b.WriteByte('\n')
	}
	for _, fk := range doc.Meta.FKs {
		b.WriteString(formatFKLine(fk))
		b.WriteByte('\n')
	}
	for _, col := range doc.Meta.Columns {
		b.WriteString("#column")
		if attrs := FormatColumnAttrs(col.Attrs); attrs != "" {
			b.WriteByte(' ')
			b.WriteString(attrs)
		}
		b.WriteByte('\n')
	}
	for _, s := range doc.Meta.SQL {
		key := s.RawKey
		if key == "" {
			key = s.Verb
			if s.Dialect != "" {
				key += "-" + s.Dialect
			}
		}
		b.WriteString("#$" + key + ": " + s.Payload)
		b.WriteByte('\n')
	}
	for _, u := range doc.Meta.Unknown {
		b.WriteString(u.Text)
		b.WriteByte('\n')
	}
	d := doc.Header.Dialect()
	for _, a := range doc.Meta.Aggregations {
		b.WriteString("#%" + a.Name + ": " + joinCSVFields(a.Values, d))
		b.WriteByte('\n')
	}
	for _, line := range doc.Meta.HumanComments {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	layout := doc.Header.Fields["layout"]
	if layout != "pack" && layout != "columnar" && doc.Source.Profile != ProfileSidecar && headerReference(doc.Header) == "" {
		if doc.Data.HasHeaderRow {
			b.WriteString(joinCSVFields(doc.Data.HeaderRow, d))
			b.WriteByte('\n')
		}
		for _, row := range doc.Data.Rows {
			b.WriteString(joinCSVFields(row, d))
			b.WriteByte('\n')
		}
	}
	return b.Bytes(), nil
}

var canonicalHeaderOrder = []string{
	"version", "layout", "delim", "quote", "header", "encoding", "null", "rows",
	"checksum", "sql-dialect", "reference", "original-size", "table-count",
	"single-table", "section-size",
}

func (doc *Document) buildCanonicalHeaderLine() string {
	// delim= and quote= are always written, defaults included: a format whose pitch
	// is "the dialect is declared inside the file" should not make the reader
	// recall a default.
	def := map[string]string{
		"header":   "1",
		"encoding": "UTF-8",
	}
	var parts []string
	emitted := map[string]bool{}
	for _, k := range canonicalHeaderOrder {
		v, ok := doc.Header.Fields[k]
		if !ok {
			switch {
			case k == "version" && doc.Header.Version != "":
				v = doc.Header.Version
			case k == "delim" && doc.Header.HasMagicLine:
				v = doc.Header.DelimName
			case k == "quote" && doc.Header.HasMagicLine:
				v = doc.Header.QuoteName
			default:
				continue
			}
		}
		emitted[k] = true
		if v == "" {
			continue
		}
		if d, isDef := def[k]; isDef && v == d {
			continue
		}
		if k == "header" && doc.Header.HeaderRow && v == "1" {
			continue
		}
		parts = append(parts, formatHeaderPair(k, v))
	}
	// Unrecognized keys MUST survive a rewrite, so trail them in a stable order.
	var extra []string
	for k := range doc.Header.Fields {
		if !emitted[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		parts = append(parts, formatHeaderPair(k, doc.Header.Fields[k]))
	}
	if len(parts) == 0 {
		return "#!excsv"
	}
	return "#!excsv " + strings.Join(parts, " ")
}

func formatHeaderPair(k, v string) string {
	if strings.Contains(v, " ") {
		return k + "=\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
	}
	return k + "=" + v
}

func (doc *Document) RowCount() int {
	return len(doc.Data.Rows)
}

// DeclaredOrCountedRows returns body row count from data when present, else rows= from header (e.g. ZIP comment metadata).
func (doc *Document) DeclaredOrCountedRows() int {
	if doc.Data.HasHeaderRow || len(doc.Data.Rows) > 0 {
		return doc.RowCount()
	}
	if doc.Header.Rows != nil {
		return *doc.Header.Rows
	}
	return 0
}

func (doc *Document) MetaMap() map[string]string {
	m := make(map[string]string, len(doc.Meta.FileMeta))
	for _, kv := range doc.Meta.FileMeta {
		m[kv.Key] = kv.Value
	}
	return m
}
