package excsv

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

var errNilDocument = errors.New("nil document")

// JSONExportOptions configures the .excsv.json writer.
type JSONExportOptions struct {
	Indent string
}

// JSONExportResult carries the bytes plus whatever the profile could not
// represent. The text ⇄ JSON mapping is a bijection except for ## comments, so
// Dropped being non-empty is expected only for those.
type JSONExportResult struct {
	Data    []byte
	Dropped []string
}

// ExportJSON writes the v0.5 JSON form of the document (implementation/json.md).
//
// The one documented loss is the spec's own: free-text ## comments carry no
// structured meaning and have no JSON slot.
func (doc *Document) ExportJSON(opts JSONExportOptions) (*JSONExportResult, error) {
	if doc == nil {
		return nil, errNilDocument
	}
	root := newOrderedJSON()
	version := doc.Header.Version
	if version == "" {
		version = CurrentVersion
	}
	root.set("excsv", version)
	root.set("layout", string(doc.jsonLayout()))
	root.set("csv", doc.jsonDialect())
	if meta := doc.jsonMeta(); meta.len() > 0 {
		root.set("meta", meta)
	}
	doc.appendTableJSON(root)

	res := &JSONExportResult{}
	if len(doc.Meta.HumanComments) > 0 {
		res.Dropped = append(res.Dropped, "## human comments (the spec's one intentional non-round-trip)")
	}
	if len(doc.Meta.Unknown) > 0 {
		res.Dropped = append(res.Dropped, "unrecognized # meta lines (the JSON root schema is closed)")
	}

	data, err := marshalJSON(root, opts.Indent)
	if err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

// ExportJSON writes the pack as layout:"pack" with a tables array and root fk.
func (p *Pack) ExportJSON(opts JSONExportOptions) (*JSONExportResult, error) {
	if p == nil {
		return nil, errNilDocument
	}
	root := newOrderedJSON()
	version := CurrentVersion
	if p.Manifest != nil && p.Manifest.Header.Version != "" {
		version = p.Manifest.Header.Version
	}
	root.set("excsv", version)
	root.set("layout", "pack")
	res := &JSONExportResult{}
	if p.Manifest != nil {
		if meta := p.Manifest.jsonMeta(); meta.len() > 0 {
			root.set("meta", meta)
		}
	}
	tables := make([]any, 0, len(p.Tables))
	for i := range p.Tables {
		tdoc := p.Tables[i].Document()
		entry := newOrderedJSON()
		entry.set("name", p.Tables[i].Decl.Name)
		tdoc.appendTableJSON(entry)
		tables = append(tables, entry)
		if len(tdoc.Meta.HumanComments) > 0 {
			res.Dropped = append(res.Dropped, "## human comments in table "+p.Tables[i].Decl.Name)
		}
	}
	root.set("tables", tables)
	if len(p.FKs) > 0 {
		fks := make([]any, 0, len(p.FKs))
		for _, fk := range p.FKs {
			e := newOrderedJSON()
			e.set("from", fk.From)
			e.set("to", fk.To)
			fks = append(fks, e)
		}
		root.set("fk", fks)
	}
	data, err := marshalJSON(root, opts.Indent)
	if err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

// appendTableJSON writes the members shared by a single-table document and one
// entry of a pack's tables array.
func (doc *Document) appendTableJSON(into *orderedJSON) {
	if cols := doc.jsonColumns(); len(cols) > 0 {
		into.set("columns", cols)
	}
	if sql := doc.jsonSQL(); sql.len() > 0 {
		into.set("sql", sql)
	}
	if cs := doc.Header.Fields["checksum"]; cs != "" {
		into.set("checksum", cs)
	}
	into.set("rows", doc.DeclaredOrCountedRows())
	if agg := doc.jsonAggregates(); agg.len() > 0 {
		into.set("aggregates", agg)
	}
	if ref := headerReference(doc.Header); ref != "" {
		into.set("reference", ref)
		return
	}
	into.set("data", doc.jsonData())
}

func (doc *Document) jsonLayout() Profile {
	if headerReference(doc.Header) != "" {
		return ProfileSidecar
	}
	return ProfileInline
}

func (doc *Document) jsonDialect() *orderedJSON {
	csv := newOrderedJSON()
	csv.set("delim", doc.Header.DelimName)
	csv.set("quote", doc.Header.QuoteName)
	csv.set("header", doc.Header.HeaderRow)
	enc := doc.Header.Encoding
	if enc == "" {
		enc = "UTF-8"
	}
	csv.set("encoding", enc)
	if doc.Header.Null != "" {
		csv.set("null", doc.Header.Null)
	}
	return csv
}

func (doc *Document) jsonMeta() *orderedJSON {
	meta := newOrderedJSON()
	for _, kv := range doc.Meta.FileMeta {
		if kv.Key == "tags" {
			parts := strings.Split(kv.Value, ",")
			tags := make([]any, 0, len(parts))
			for _, p := range parts {
				if p = strings.TrimSpace(p); p != "" {
					tags = append(tags, p)
				}
			}
			meta.set(kv.Key, tags)
			continue
		}
		meta.set(kv.Key, kv.Value)
	}
	return meta
}

func (doc *Document) jsonColumns() []any {
	if len(doc.Meta.Columns) == 0 {
		return nil
	}
	out := make([]any, 0, len(doc.Meta.Columns))
	phys := 0
	for _, col := range doc.Meta.Columns {
		entry := newOrderedJSON()
		// index is REQUIRED on every physical column object in the JSON
		// form, even though the text form only requires it when header=0 —
		// but MUST NOT be present on a virtual computed column, which has
		// no physical position at all.
		if !isVirtualColumn(col) {
			idx := phys
			if v, ok := parseAttrInt(col.Attrs["index"]); ok {
				idx = v
			}
			entry.set("index", idx)
			phys++
		}
		colType := col.Attrs["type"]
		for _, k := range columnAttrDisplayOrder {
			v, ok := col.Attrs[k]
			if !ok || k == "index" {
				continue
			}
			entry.set(k, jsonColumnAttr(k, v, colType))
		}
		var extra []string
		for k := range col.Attrs {
			if !knownColumnAttrs[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range extra {
			entry.set(k, col.Attrs[k])
		}
		out = append(out, entry)
	}
	return out
}

func jsonColumnAttr(key, value, columnType string) any {
	switch key {
	case "unique", "required", "materialized":
		return value == "1"
	case "enum":
		parts := strings.Split(value, "|")
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			out = append(out, jsonScalar(p, columnType))
		}
		return out
	case "len_min", "len_max":
		if n, ok := parseAttrInt(value); ok {
			return n
		}
	}
	return value
}

func (doc *Document) jsonSQL() *orderedJSON {
	sql := newOrderedJSON()
	byVerb := map[string][]any{}
	var verbs []string
	for _, stmt := range doc.Meta.SQL {
		entry := newOrderedJSON()
		if stmt.Qualified && stmt.Dialect != "" {
			entry.set("dialect", stmt.Dialect)
			if stmt.Version != "" {
				entry.set("version", stmt.Version)
			}
		}
		entry.set("stmt", stmt.Payload)
		if _, seen := byVerb[stmt.Verb]; !seen {
			verbs = append(verbs, stmt.Verb)
		}
		byVerb[stmt.Verb] = append(byVerb[stmt.Verb], entry)
	}
	for _, verb := range verbs {
		sql.set(verb, byVerb[verb])
	}
	return sql
}

func (doc *Document) jsonAggregates() *orderedJSON {
	agg := newOrderedJSON()
	for _, a := range doc.Meta.Aggregations {
		values := make([]any, len(a.Values))
		for i, v := range a.Values {
			if v == "" {
				values[i] = nil
				continue
			}
			values[i] = jsonAggValue(a.Name, v, columnTypeAt(doc, i))
		}
		agg.set(a.Name, values)
	}
	return agg
}

func jsonAggValue(name, value, columnType string) any {
	switch name {
	case "count_nonnull", "count_null", "count_distinct", "len_min", "len_max":
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return n
		}
		return value
	}
	return jsonScalar(value, columnType)
}

func (doc *Document) jsonData() []any {
	rows := make([]any, 0, len(doc.Data.Rows))
	width := doc.columnWidth()
	for _, row := range doc.Data.Rows {
		n := width
		if n == 0 {
			n = len(row)
		}
		cells := make([]any, n)
		for i := 0; i < n; i++ {
			raw := cellAt(doc, row, i)
			if isNullCell(doc, raw) {
				cells[i] = nil
				continue
			}
			cells[i] = jsonScalar(raw, columnTypeAt(doc, i))
		}
		rows = append(rows, cells)
	}
	return rows
}

// jsonScalar encodes one cell. decimal and long stay strings: the column type
// is authoritative and JSON numbers would silently lose precision.
func jsonScalar(raw, columnType string) any {
	switch strings.ToLower(strings.TrimSpace(columnType)) {
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "1":
			return true
		case "false", "0":
			return false
		}
		return raw
	case "int":
		if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && strconv.FormatInt(n, 10) == raw {
			return n
		}
	case "float", "double", "number":
		// A JSON number is only safe when it reproduces the cell exactly:
		// 500.00 must not come back as 500, or the round-trip stops being one.
		if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && strconv.FormatFloat(f, 'f', -1, 64) == raw {
			return f
		}
	}
	return raw
}

// orderedJSON preserves member order, which a Go map cannot.
type orderedJSON struct {
	keys   []string
	values map[string]any
}

func newOrderedJSON() *orderedJSON {
	return &orderedJSON{values: map[string]any{}}
}

func (o *orderedJSON) set(key string, value any) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *orderedJSON) len() int { return len(o.keys) }

func (o *orderedJSON) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		val, err := json.Marshal(o.values[k])
		if err != nil {
			return nil, err
		}
		b.Write(val)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func marshalJSON(v any, indent string) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if indent == "" {
		return append(raw, '\n'), nil
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", indent); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}
