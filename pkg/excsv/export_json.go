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

// ExportJSON writes the JSON form of the document (implementation/json.md).
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
	res := &JSONExportResult{Dropped: doc.appendTableJSON(root)}
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
		for _, d := range tdoc.appendTableJSON(entry) {
			res.Dropped = append(res.Dropped, d+" in table "+p.Tables[i].Decl.Name)
		}
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
func (doc *Document) appendTableJSON(into *orderedJSON) (dropped []string) {
	cols, dropped := doc.jsonColumns()
	if len(cols) > 0 {
		into.set("columns", cols)
	}
	if charts := doc.jsonCharts(); len(charts) > 0 {
		into.set("charts", charts)
	}
	if notes := doc.jsonAnchored(noteAttrs(doc.Meta.Notes), noteJSONKeys); len(notes) > 0 {
		into.set("notes", notes)
	}
	if links := doc.jsonAnchored(linkAttrs(doc.Meta.Links), linkJSONKeys); len(links) > 0 {
		into.set("links", links)
	}
	sql, sqlDropped := doc.jsonSQL()
	dropped = append(dropped, sqlDropped...)
	if sql.len() > 0 {
		into.set("sql", sql)
	}
	if cs := doc.Header.Fields["checksum"]; cs != "" {
		// The schema admits only <algo>:<lowercase hex>; a malformed value
		// (checksum_malformed) has no JSON slot.
		if _, kind := classifyChecksumField(cs); kind != ErrChecksumMalformed {
			into.set("checksum", cs)
		} else {
			dropped = append(dropped, "checksum="+cs+": malformed")
		}
	}
	into.set("rows", doc.DeclaredOrCountedRows())
	if n, ok := parseAttrInt(doc.Header.Fields["columns"]); ok {
		into.set("column_count", n)
	}
	if agg := doc.jsonAggregates(); agg.len() > 0 {
		into.set("aggregates", agg)
	}
	if ref := headerReference(doc.Header); ref != "" {
		into.set("reference", ref)
		return dropped
	}
	into.set("data", doc.jsonData())
	return dropped
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
		// The schema types csv.null as an array of markers.
		csv.set("null", []string{doc.Header.Null})
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

// jsonColumns writes one object per physical column, in index order, then
// the virtual computed columns (json.md § Column order). index comes from
// the header cell a #column binds to, never its line order; a physical
// column no #column describes becomes a bare {index, name} object so its
// header name survives. A #column bound to no column is reported in dropped.
func (doc *Document) jsonColumns() (out []any, dropped []string) {
	width := doc.PhysicalColumnCount()
	bound := make([]int, width) // declaration index per physical column, -1 if none
	for i := range bound {
		bound[i] = -1
	}
	var virtual []ColumnDef
	for i, col := range doc.Meta.Columns {
		if isVirtualColumn(col) {
			virtual = append(virtual, col)
			continue
		}
		p := doc.declPhysIndex(i)
		if p < 0 || p >= width {
			dropped = append(dropped, "#column "+col.Attrs["name"]+": matches no column")
			continue
		}
		if bound[p] >= 0 {
			dropped = append(dropped, "#column "+doc.Meta.Columns[bound[p]].Attrs["name"]+": duplicate, last one wins")
		}
		bound[p] = i
	}
	if width == 0 && len(virtual) == 0 {
		return nil, dropped
	}
	for p, i := range bound {
		if i < 0 {
			entry := newOrderedJSON()
			entry.set("index", p)
			if doc.Header.HeaderRow && p < len(doc.Data.HeaderRow) {
				entry.set("name", doc.Data.HeaderRow[p])
			}
			out = append(out, entry)
			continue
		}
		out = append(out, jsonColumnEntry(doc.Meta.Columns[i], p))
	}
	for _, col := range virtual {
		out = append(out, jsonColumnEntry(col, -1))
	}
	return out, dropped
}

// jsonColumnEntry renders one #column; index < 0 means virtual (no index).
func jsonColumnEntry(col ColumnDef, index int) *orderedJSON {
	entry := newOrderedJSON()
	if index >= 0 {
		entry.set("index", index)
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
	return entry
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

// jsonCharts mirrors #chart lines (charts.md § JSON mirror): a compact line
// becomes a flat object of its attributes, a #chart-<engine>: line becomes
// {"<engine>": <parsed payload>}. File order is preserved.
func (doc *Document) jsonCharts() []any {
	if len(doc.Meta.Charts) == 0 {
		return nil
	}
	out := make([]any, 0, len(doc.Meta.Charts))
	for _, c := range doc.Meta.Charts {
		entry := newOrderedJSON()
		if c.IsEscape() {
			var payload any = c.Payload
			if json.Valid([]byte(c.Payload)) {
				payload = json.RawMessage(c.Payload)
			}
			entry.set(c.Engine, payload)
		} else {
			for _, kv := range c.Attrs {
				entry.set(kv.Key, jsonChartAttr(kv))
			}
		}
		out = append(out, entry)
	}
	return out
}

// jsonChartAttr types a compact #chart attribute per schema/excsv.schema.json.
func jsonChartAttr(kv KV) any {
	switch kv.Key {
	case "tooltip":
		if refs := chartChannelValues(kv); len(refs) > 1 {
			out := make([]any, len(refs))
			for i, r := range refs {
				out[i] = r
			}
			return out
		}
	case "bin":
		// bin=1 switches binning on; any other count is the bin count.
		if kv.Value == "1" {
			return true
		}
		if n, ok := parseAttrInt(kv.Value); ok {
			return n
		}
	case "stack":
		switch kv.Value {
		case "1":
			return true
		case "0":
			return false
		}
	case "limit":
		if n, ok := parseAttrInt(kv.Value); ok {
			return n
		}
	case "hole":
		if f, err := strconv.ParseFloat(kv.Value, 64); err == nil {
			return f
		}
	}
	return kv.Value
}

var (
	noteJSONKeys = stringSet([]string{"col", "row", "key", "text", "author", "created"})
	linkJSONKeys = stringSet([]string{"col", "row", "key", "href"})
)

func noteAttrs(notes []Note) [][]KV {
	out := make([][]KV, len(notes))
	for i, n := range notes {
		out[i] = n.Attrs
	}
	return out
}

func linkAttrs(links []Link) [][]KV {
	out := make([][]KV, len(links))
	for i, l := range links {
		out[i] = l.Attrs
	}
	return out
}

// jsonAnchored mirrors #note / #link lines (notes.md § JSON mirror): one flat
// object per line, file order preserved. row is an integer, col a name or
// (header=0) an integer index, key always the raw id text. Unknown attributes
// are dropped, except the x- ones the schema admits.
func (doc *Document) jsonAnchored(lines [][]KV, known map[string]bool) []any {
	if len(lines) == 0 {
		return nil
	}
	declared := map[string]bool{}
	for _, col := range doc.Meta.Columns {
		if name := col.Attrs["name"]; name != "" {
			declared[name] = true
		}
	}
	out := make([]any, 0, len(lines))
	for _, attrs := range lines {
		entry := newOrderedJSON()
		for _, kv := range attrs {
			if !known[kv.Key] && !strings.HasPrefix(kv.Key, "x-") {
				continue
			}
			var v any = kv.Value
			switch kv.Key {
			case "row":
				if n, ok := parseAttrInt(kv.Value); ok {
					v = n
				}
			case "col":
				if n, ok := parseAttrInt(kv.Value); ok && !doc.Header.HeaderRow && !declared[kv.Value] {
					v = n
				}
			}
			entry.set(kv.Key, v)
		}
		out = append(out, entry)
	}
	return out
}

// jsonSQL mirrors #$ddl / #$dql lines. The JSON sql object only has those two
// verbs, so a line with any other verb (sql_unknown_verb) is reported.
func (doc *Document) jsonSQL() (*orderedJSON, []string) {
	sql := newOrderedJSON()
	byVerb := map[string][]any{}
	var verbs, dropped []string
	for _, stmt := range doc.Meta.SQL {
		if stmt.Verb != "ddl" && stmt.Verb != "dql" {
			dropped = append(dropped, "#$"+stmt.Verb+": unknown SQL verb")
			continue
		}
		entry := newOrderedJSON()
		if stmt.Qualified && stmt.Dialect != "" {
			entry.set("dialect", stmt.Dialect)
			if stmt.Version != "" {
				entry.set("version", stmt.Version)
			}
		} else if d := strings.TrimSpace(doc.Header.Fields["sql-dialect"]); d != "" {
			// JSON has no sql-dialect slot: the header default moves onto
			// each unqualified statement, which keeps its meaning.
			dialect, version := splitDialectVersion(strings.ToLower(d))
			entry.set("dialect", dialect)
			if version != "" {
				entry.set("version", version)
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
	return sql, dropped
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
	width := doc.PhysicalColumnCount()
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
