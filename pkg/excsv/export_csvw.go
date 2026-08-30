package excsv

import (
	"fmt"
	"sort"
	"strings"
)

// CSVWExportOptions configures the CSVW metadata sidecar writer.
//
// URL is CSVW's one required table property: "the single URL of the CSV file
// that the table is held in". A sidecar document already knows it from
// reference=; every other shape must be told.
type CSVWExportOptions struct {
	URL           string
	EnumAsPattern bool
	Table         string
	Indent        string
}

// CSVWExportResult carries the bytes plus every attribute the mapping could
// not carry. Unlike ExportJSON, this export is lossy by nature, so Dropped is
// the normal case and must be surfaced every time.
type CSVWExportResult struct {
	Data    []byte
	Dropped []string
}

// csvwTypeMap lowers ExCSV types onto XSD datatype names. Every ExCSV type has
// a target, so type= is never dropped; uuid and binary widen and say so.
var csvwTypeMap = map[string]string{
	"string":   "string",
	"int":      "int",
	"long":     "long",
	"float":    "float",
	"double":   "double",
	"decimal":  "decimal",
	"boolean":  "boolean",
	"date":     "date",
	"time":     "time",
	"datetime": "dateTime",
	"uuid":     "string",
	"binary":   "base64Binary",
}

// csvwUncarried names the ExCSV column attributes with no CSVW counterpart.
var csvwUncarried = []string{"unit", "role", "agg", "order", "regexp_dialect"}

// ExportCSVW writes a CSVW metadata sidecar describing the document's data.
//
// Nothing reads CSVW in this tool and nothing stores it: this is a one-way
// boundary format. Serializing to a standard is total and deterministic;
// consuming one is a merge with an adversary, so there is no importer.
func (doc *Document) ExportCSVW(opts CSVWExportOptions) (*CSVWExportResult, error) {
	if doc == nil {
		return nil, errNilDocument
	}
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		url = headerReference(doc.Header)
	}
	if url == "" {
		return nil, fmt.Errorf("--url is required: an inline ExCSV document is not a CSV file, so CSVW has no url to point at")
	}
	res := &CSVWExportResult{}
	root := newOrderedJSON()
	root.set("@context", "http://www.w3.org/ns/csvw")
	doc.applyCSVWMeta(root, res)
	root.set("url", url)
	if dialect := doc.csvwDialect(); dialect.len() > 0 {
		root.set("dialect", dialect)
	}
	root.set("tableSchema", doc.csvwTableSchema(opts, res))

	data, err := marshalJSON(root, opts.Indent)
	if err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

// ExportCSVW writes a pack as a CSVW TableGroup, one tables[] entry per table.
func (p *Pack) ExportCSVW(opts CSVWExportOptions) (*CSVWExportResult, error) {
	if p == nil {
		return nil, errNilDocument
	}
	res := &CSVWExportResult{}
	root := newOrderedJSON()
	root.set("@context", "http://www.w3.org/ns/csvw")
	tables := make([]any, 0, len(p.Tables))
	for i := range p.Tables {
		name := p.Tables[i].Decl.Name
		if opts.Table != "" && opts.Table != name {
			continue
		}
		tdoc := p.Tables[i].Document()
		url := strings.TrimSpace(opts.URL)
		if url == "" {
			url = name + ".csv"
		}
		entry := newOrderedJSON()
		entry.set("url", url)
		entry.set("dc:title", name)
		entry.set("tableSchema", tdoc.csvwTableSchema(opts, res))
		tables = append(tables, entry)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no tables to export")
	}
	root.set("tables", tables)
	data, err := marshalJSON(root, opts.Indent)
	if err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

// applyCSVWMeta maps the #@ keys that have Dublin Core counterparts and drops
// the rest.
func (doc *Document) applyCSVWMeta(root *orderedJSON, res *CSVWExportResult) {
	dublinCore := map[string]string{
		"title":       "dc:title",
		"description": "dc:description",
		"comment":     "dc:description",
		"author":      "dc:creator",
		"license":     "dc:license",
		"created":     "dc:created",
	}
	var dropped []string
	for _, kv := range doc.Meta.FileMeta {
		target, ok := dublinCore[kv.Key]
		if !ok {
			dropped = append(dropped, "#@"+kv.Key)
			continue
		}
		root.set(target, kv.Value)
	}
	if len(dropped) > 0 {
		res.drop("no Dublin Core counterpart: " + strings.Join(dropped, ", "))
	}
	if len(doc.Meta.Aggregations) > 0 {
		res.drop("#% aggregations have no CSVW analogue")
	}
	if len(doc.Meta.SQL) > 0 {
		res.drop("#$ SQL companions have no CSVW analogue")
	}
}

func (doc *Document) csvwDialect() *orderedJSON {
	d := newOrderedJSON()
	if r := doc.Header.Delim; r != 0 {
		d.set("delimiter", string(r))
	}
	d.set("header", doc.Header.HeaderRow)
	if doc.Header.QuoteEnabled {
		d.set("quoteChar", string(doc.Header.Quote))
	} else {
		d.set("quoteChar", nil)
	}
	if enc := doc.Header.Encoding; enc != "" {
		d.set("encoding", enc)
	}
	return d
}

func (doc *Document) csvwTableSchema(opts CSVWExportOptions, res *CSVWExportResult) *orderedJSON {
	schema := newOrderedJSON()
	columns := make([]any, 0, len(doc.Meta.Columns))
	for _, col := range doc.Meta.Columns {
		columns = append(columns, doc.csvwColumn(col, opts, res))
	}
	schema.set("columns", columns)
	return schema
}

func (doc *Document) csvwColumn(col ColumnDef, opts CSVWExportOptions, res *CSVWExportResult) *orderedJSON {
	out := newOrderedJSON()
	label := col.Attrs["name"]
	if label == "" {
		label = col.Attrs["index"]
	}
	if v := col.Attrs["name"]; v != "" {
		out.set("name", v)
	}
	if v := col.Attrs["title"]; v != "" {
		out.set("titles", v)
	}
	if v := col.Attrs["description"]; v != "" {
		out.set("dc:description", v)
	}

	datatype := newOrderedJSON()
	rawType := strings.ToLower(strings.TrimSpace(col.Attrs["type"]))
	if rawType != "" {
		base, ok := csvwTypeMap[rawType]
		if !ok {
			base = "string"
			res.drop("column " + label + ": unknown type=" + rawType + " widened to xsd:string")
		}
		datatype.set("base", base)
		switch rawType {
		case "uuid":
			datatype.set("format", "[0-9a-fA-F-]{36}")
			res.drop("column " + label + ": type=uuid approximated as xsd:string with a format regex (XSD has no UUID type)")
		case "binary":
			res.drop("column " + label + ": type=binary approximated as xsd:base64Binary")
		}
	}
	if v := col.Attrs["min"]; v != "" {
		datatype.set("minimum", v)
	}
	if v := col.Attrs["max"]; v != "" {
		datatype.set("maximum", v)
	}
	if n, ok := parseAttrInt(col.Attrs["len_min"]); ok {
		datatype.set("minLength", n)
	}
	if n, ok := parseAttrInt(col.Attrs["len_max"]); ok {
		datatype.set("maxLength", n)
	}
	if v := col.Attrs["pattern"]; v != "" {
		datatype.set("format", v)
	}
	if v := col.Attrs["enum"]; v != "" {
		if opts.EnumAsPattern {
			parts := strings.Split(v, "|")
			quoted := make([]string, 0, len(parts))
			for _, p := range parts {
				quoted = append(quoted, regexpQuoteMeta(p))
			}
			datatype.set("format", "^(?:"+strings.Join(quoted, "|")+")$")
		} else {
			res.drop("column " + label + ": enum= dropped (CSVW has no enumeration facet; --enum-as-pattern encodes it as a regex)")
		}
	}
	if datatype.len() > 0 {
		out.set("datatype", datatype)
	}

	if col.Attrs["required"] == "1" {
		out.set("required", true)
	}
	if v := col.Attrs["default"]; v != "" {
		out.set("default", v)
	}
	if v := col.Attrs["null"]; v != "" {
		out.set("null", v)
	}
	if v := col.Attrs["separator"]; v != "" {
		out.set("separator", v)
	}

	// unique= is dropped rather than translated. CSVW's only uniqueness
	// construct is schema-level primaryKey, which asserts that the *combination*
	// of the named columns is unique — strictly weaker than two independent
	// unique= constraints. Silently weakening a constraint while appearing to
	// preserve it is worse than dropping it.
	if col.Attrs["unique"] == "1" {
		res.drop("column " + label + ": unique= dropped (CSVW primaryKey asserts a weaker, combination-wide constraint)")
	}
	for _, attr := range csvwUncarried {
		if col.Attrs[attr] != "" {
			res.drop("column " + label + ": " + attr + "= has no CSVW counterpart")
		}
	}
	return out
}

func (r *CSVWExportResult) drop(msg string) {
	for _, existing := range r.Dropped {
		if existing == msg {
			return
		}
	}
	r.Dropped = append(r.Dropped, msg)
}

func regexpQuoteMeta(s string) string {
	const meta = `\.+*?()|[]{}^$`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SortedDropped returns the loss report in a stable order.
func (r *CSVWExportResult) SortedDropped() []string {
	out := append([]string(nil), r.Dropped...)
	sort.Strings(out)
	return out
}
