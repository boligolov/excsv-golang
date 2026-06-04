package excsv

import (
	"bytes"
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
	for _, col := range doc.Meta.Columns {
		b.WriteString("#column")
		for k, v := range col.Attrs {
			b.WriteByte(' ')
			if strings.Contains(v, " ") {
				b.WriteString(k + "=\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\"")
			} else {
				b.WriteString(k + "=" + v)
			}
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
	if doc.Meta.CSVW != nil {
		b.WriteString("#csvw " + *doc.Meta.CSVW)
		b.WriteByte('\n')
	}
	d := doc.Header.Dialect()
	for _, a := range doc.Meta.Aggregations {
		b.WriteString("#%" + a.Name + ": " + joinCSVFields(a.Values, d))
		b.WriteByte('\n')
	}
	if doc.Source.Profile != ProfileSidecar && headerReference(doc.Header) == "" {
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

func (doc *Document) buildCanonicalHeaderLine() string {
	order := []string{"version", "delim", "quote", "header", "encoding", "null", "rows", "checksum", "schema", "csvw", "sql-dialect", "reference", "original-size"}
	def := map[string]string{
		"delim":    "comma",
		"quote":    "none",
		"header":   "1",
		"encoding": "UTF-8",
		"schema":   "excsv",
	}
	var parts []string
	for _, k := range order {
		v, ok := doc.Header.Fields[k]
		if !ok {
			if k == "version" && doc.Header.Version != "" {
				v = doc.Header.Version
			} else {
				continue
			}
		}
		if d, isDef := def[k]; isDef && v == d {
			continue
		}
		if k == "header" && doc.Header.HeaderRow && v == "1" {
			continue
		}
		if strings.Contains(v, " ") {
			parts = append(parts, k+"=\""+strings.ReplaceAll(v, "\"", "\"\"")+"\"")
		} else {
			parts = append(parts, k+"="+v)
		}
	}
	if len(parts) == 0 {
		return "#!excsv"
	}
	return "#!excsv " + strings.Join(parts, " ")
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
