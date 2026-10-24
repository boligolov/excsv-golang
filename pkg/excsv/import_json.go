package excsv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// IsJSONForm reports whether data is the JSON form: a top-level JSON object
// with an "excsv" member (json.md § Reading — dispatch is on content, not
// on the file name).
func IsJSONForm(data []byte) bool {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	// A text document cannot start with "{" and name an "excsv" member near
	// the top, so a broken JSON document still reaches JSONToText and fails
	// with json_malformed instead of being read as CSV.
	head := trimmed[:min(len(trimmed), 4096)]
	return bytes.Contains(head, []byte(`"excsv"`))
}

// JSONToText renders a JSON-form document (.excsv.json) as the equivalent
// canonical text document. ParseBytes reads the JSON form through it, so a
// JSON document gets exactly the checks and warning codes of its text twin;
// problems that only exist in JSON fail with json_malformed.
func JSONToText(data []byte) ([]byte, error) {
	v, err := decodeOrdered(data)
	if err != nil {
		return nil, jsonFail("not valid JSON: " + err.Error())
	}
	root, ok := v.(*orderedJSON)
	if !ok {
		return nil, jsonFail("top level is not an object")
	}
	if _, ok := root.values["excsv"]; !ok {
		return nil, jsonFail(`no "excsv" member`)
	}
	if _, ok := root.values["tables"]; ok {
		return nil, jsonFail("pack documents (tables) cannot be read from the JSON form yet")
	}
	w := &jsonTextWriter{root: root}
	return w.render()
}

func jsonFail(msg string) error { return fail(ErrJSONMalformed, 0, msg) }

type jsonColumn struct {
	attrs   *orderedJSON
	index   int
	virtual bool
}

type jsonTextWriter struct {
	root    *orderedJSON
	dialect Dialect
	header  bool
}

func (w *jsonTextWriter) render() ([]byte, error) {
	version, err := jsonString(w.root.values["excsv"], "excsv")
	if err != nil {
		return nil, err
	}
	csvOpts, err := jsonObject(w.root.values["csv"], "csv")
	if err != nil {
		return nil, err
	}
	fields := map[string]string{"version": version}
	w.header = true
	delimName, quoteName := "comma", ""
	if csvOpts != nil {
		if v, ok := csvOpts.values["delim"]; ok {
			if delimName, err = jsonString(v, "csv.delim"); err != nil {
				return nil, err
			}
		}
		if v, ok := csvOpts.values["quote"]; ok {
			if quoteName, err = jsonString(v, "csv.quote"); err != nil {
				return nil, err
			}
		}
		if v, ok := csvOpts.values["header"]; ok {
			b, isBool := v.(bool)
			if !isBool {
				return nil, jsonFail("csv.header is not a boolean")
			}
			w.header = b
		}
		if v, ok := csvOpts.values["null"]; ok {
			marker, err := jsonNullMarker(v)
			if err != nil {
				return nil, err
			}
			if marker != "" {
				fields["null"] = marker
			}
		}
		if v, ok := csvOpts.values["encoding"]; ok {
			enc, err := jsonString(v, "csv.encoding")
			if err != nil {
				return nil, err
			}
			if !strings.EqualFold(enc, "UTF-8") {
				fields["encoding"] = enc
			}
		}
	}
	if !w.header {
		fields["header"] = "0"
	}

	columns, err := w.columns()
	if err != nil {
		return nil, err
	}
	width := 0
	for _, c := range columns {
		if !c.virtual && c.index+1 > width {
			width = c.index + 1
		}
	}
	reference, hasRef := w.root.values["reference"]
	var rows [][]string
	if hasRef {
		ref, err := jsonString(reference, "reference")
		if err != nil {
			return nil, err
		}
		fields["reference"] = ref
	} else if rows, err = w.data(); err != nil {
		return nil, err
	}
	if width == 0 && len(rows) > 0 {
		width = len(rows[0])
	}
	headerRow, err := w.headerRow(columns, width)
	if err != nil {
		return nil, err
	}
	aggs, err := w.aggregates()
	if err != nil {
		return nil, err
	}

	// Pick the dialect: csv.delim, and csv.quote unless a value needs
	// quoting that quote=none cannot give it.
	delim, err := resolveDelim(delimName)
	if err != nil {
		return nil, jsonFail("csv.delim: " + err.Error())
	}
	if quoteName == "" || quoteName == "none" {
		quoteName = "none"
		if needsQuoting(delim, fields["null"], headerRow, rows, aggs) {
			quoteName = "double"
		}
	}
	quote, quoteOn, err := resolveQuote(quoteName)
	if err != nil {
		return nil, jsonFail("csv.quote: " + err.Error())
	}
	w.dialect = Dialect{Delim: delim, Quote: quote, QuoteEnabled: quoteOn}
	fields["delim"], fields["quote"] = delimName, quoteName

	if v, ok := w.root.values["rows"]; ok {
		n, err := jsonInt(v, "rows")
		if err != nil {
			return nil, err
		}
		fields["rows"] = strconv.Itoa(n)
	} else if !hasRef {
		fields["rows"] = strconv.Itoa(len(rows))
	}
	if v, ok := w.root.values["column_count"]; ok {
		n, err := jsonInt(v, "column_count")
		if err != nil {
			return nil, err
		}
		fields["columns"] = strconv.Itoa(n)
	}
	if v, ok := w.root.values["checksum"]; ok {
		cs, err := jsonString(v, "checksum")
		if err != nil {
			return nil, err
		}
		fields["checksum"] = cs
	}

	var b bytes.Buffer
	b.WriteString("#!excsv")
	for _, k := range canonicalHeaderOrder {
		if v, ok := fields[k]; ok {
			b.WriteString(" " + formatHeaderPair(k, v))
		}
	}
	b.WriteByte('\n')
	if err := w.writeMeta(&b); err != nil {
		return nil, err
	}
	for _, c := range columns {
		line, err := w.columnLine(c)
		if err != nil {
			return nil, err
		}
		if line != "" {
			b.WriteString(line + "\n")
		}
	}
	if err := w.writeCharts(&b); err != nil {
		return nil, err
	}
	for _, kind := range []string{"notes", "links"} {
		if err := w.writeAnchored(&b, kind); err != nil {
			return nil, err
		}
	}
	if err := w.writeSQL(&b); err != nil {
		return nil, err
	}
	for _, a := range aggs {
		b.WriteString("#%" + a.Name + ": " + joinCSVFields(a.Values, w.dialect) + "\n")
	}
	if hasRef {
		return b.Bytes(), nil
	}
	if w.header {
		b.WriteString(joinRow(headerRow, w.dialect) + "\n")
	}
	for _, row := range rows {
		line := joinRow(row, w.dialect)
		if line == "" && len(row) == 1 {
			// A lone null would be a blank line, which is no row at all.
			if line = fields["null"]; line == "" {
				line = `""`
			}
		}
		b.WriteString(line + "\n")
	}
	return b.Bytes(), nil
}

// columns reads columns[]: physical columns sorted by index, then virtual
// ones in array order.
func (w *jsonTextWriter) columns() ([]jsonColumn, error) {
	list, err := jsonArray(w.root.values["columns"], "columns")
	if err != nil {
		return nil, err
	}
	var phys, virt []jsonColumn
	seen := map[int]bool{}
	for i, item := range list {
		obj, ok := item.(*orderedJSON)
		if !ok {
			return nil, jsonFail(fmt.Sprintf("columns[%d] is not an object", i))
		}
		c := jsonColumn{attrs: obj}
		formula := obj.values["formula"] != nil
		materialized, _ := obj.values["materialized"].(bool)
		c.virtual = formula && !materialized
		idx, hasIdx := obj.values["index"]
		switch {
		case c.virtual:
			// An index on a formula column stays on its #column line, where
			// the text check reports formula_index_forbidden.
			virt = append(virt, c)
			continue
		case !hasIdx:
			return nil, jsonFail(fmt.Sprintf("columns[%d]: a physical column needs index", i))
		}
		if c.index, err = jsonInt(idx, fmt.Sprintf("columns[%d].index", i)); err != nil {
			return nil, err
		}
		if c.index < 0 || seen[c.index] {
			return nil, jsonFail(fmt.Sprintf("columns[%d]: index %d is negative or repeated", i, c.index))
		}
		seen[c.index] = true
		phys = append(phys, c)
	}
	sort.SliceStable(phys, func(a, b int) bool { return phys[a].index < phys[b].index })
	return append(phys, virt...), nil
}

// headerRow names each physical position: title, else name.
func (w *jsonTextWriter) headerRow(columns []jsonColumn, width int) ([]string, error) {
	if !w.header {
		return nil, nil
	}
	row := make([]string, width)
	named := make([]bool, width)
	for _, c := range columns {
		if c.virtual || c.index >= width {
			continue
		}
		cell := ""
		for _, k := range []string{"title", "name"} {
			if v, ok := c.attrs.values[k]; ok {
				s, err := jsonString(v, "columns."+k)
				if err != nil {
					return nil, err
				}
				cell = s
				break
			}
		}
		row[c.index], named[c.index] = cell, cell != ""
	}
	for i, ok := range named {
		if !ok {
			return nil, jsonFail(fmt.Sprintf("column %d has no name, but csv.header is true", i))
		}
	}
	return row, nil
}

// columnLine renders one column object as a #column line, or "" for a bare
// object (index and name only), which carries no schema.
func (w *jsonTextWriter) columnLine(c jsonColumn) (string, error) {
	attrs := map[string]string{}
	for _, k := range c.attrs.keys {
		if k == "index" && !c.virtual {
			continue
		}
		s, err := jsonColumnAttrText(k, c.attrs.values[k])
		if err != nil {
			return "", err
		}
		attrs[k] = s
	}
	if len(attrs) == 0 || (w.header && len(attrs) == 1 && attrs["name"] != "") {
		return "", nil
	}
	if !w.header && !c.virtual {
		attrs["index"] = strconv.Itoa(c.index)
	}
	return "#column " + FormatColumnAttrs(attrs), nil
}

func jsonColumnAttrText(key string, v any) (string, error) {
	switch t := v.(type) {
	case bool:
		if t {
			return "1", nil
		}
		return "0", nil
	case []any:
		if key != "enum" {
			return "", jsonFail("columns." + key + " is an array")
		}
		parts := make([]string, len(t))
		for i, p := range t {
			s, err := jsonScalarText(p, "columns.enum")
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, "|"), nil
	}
	return jsonScalarText(v, "columns."+key)
}

func (w *jsonTextWriter) writeMeta(b *bytes.Buffer) error {
	meta, err := jsonObject(w.root.values["meta"], "meta")
	if err != nil || meta == nil {
		return err
	}
	for _, k := range meta.keys {
		v := meta.values[k]
		var s string
		if list, ok := v.([]any); ok && k == "tags" {
			parts := make([]string, len(list))
			for i, p := range list {
				if parts[i], err = jsonScalarText(p, "meta.tags"); err != nil {
					return err
				}
			}
			s = strings.Join(parts, ",")
		} else if s, err = jsonScalarText(v, "meta."+k); err != nil {
			return err
		}
		if strings.ContainsAny(s, "\r\n") {
			return jsonFail("meta." + k + " contains a line break")
		}
		b.WriteString("#@" + k + ": " + s + "\n")
	}
	return nil
}

func (w *jsonTextWriter) writeCharts(b *bytes.Buffer) error {
	list, err := jsonArray(w.root.values["charts"], "charts")
	if err != nil {
		return err
	}
	for i, item := range list {
		obj, ok := item.(*orderedJSON)
		if !ok {
			return jsonFail(fmt.Sprintf("charts[%d] is not an object", i))
		}
		// {"<engine>": payload} is a #chart-<engine>: escape line.
		if len(obj.keys) == 1 && obj.keys[0] != "type" {
			engine := obj.keys[0]
			payload := obj.values[engine]
			text, isString := payload.(string)
			if !isString {
				raw, err := json.Marshal(payload)
				if err != nil {
					return jsonFail(fmt.Sprintf("charts[%d]: %v", i, err))
				}
				text = string(raw)
			}
			b.WriteString("#chart-" + engine + ": " + text + "\n")
			continue
		}
		c := Chart{}
		for _, k := range obj.keys {
			s, err := jsonChartAttrText(k, obj.values[k])
			if err != nil {
				return err
			}
			c.Attrs = append(c.Attrs, KV{Key: k, Value: s})
		}
		b.WriteString(c.Text() + "\n")
	}
	return nil
}

func jsonChartAttrText(key string, v any) (string, error) {
	switch t := v.(type) {
	case bool:
		if t {
			return "1", nil
		}
		return "0", nil
	case []any:
		parts := make([]string, len(t))
		for i, p := range t {
			s, err := jsonScalarText(p, "charts."+key)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, ","), nil
	}
	return jsonScalarText(v, "charts."+key)
}

func (w *jsonTextWriter) writeAnchored(b *bytes.Buffer, kind string) error {
	list, err := jsonArray(w.root.values[kind], kind)
	if err != nil {
		return err
	}
	prefix, quoted := "#note", "text"
	if kind == "links" {
		prefix, quoted = "#link", "href"
	}
	for i, item := range list {
		obj, ok := item.(*orderedJSON)
		if !ok {
			return jsonFail(fmt.Sprintf("%s[%d] is not an object", kind, i))
		}
		var attrs []KV
		for _, k := range obj.keys {
			s, err := jsonScalarText(obj.values[k], kind+"."+k)
			if err != nil {
				return err
			}
			if strings.ContainsAny(s, "\r\n") {
				return jsonFail(fmt.Sprintf("%s[%d].%s contains a line break", kind, i, k))
			}
			attrs = append(attrs, KV{Key: k, Value: s})
		}
		b.WriteString(formatAnchoredLine(prefix, attrs, quoted) + "\n")
	}
	return nil
}

func (w *jsonTextWriter) writeSQL(b *bytes.Buffer) error {
	sql, err := jsonObject(w.root.values["sql"], "sql")
	if err != nil || sql == nil {
		return err
	}
	for _, verb := range sql.keys {
		list, err := jsonArray(sql.values[verb], "sql."+verb)
		if err != nil {
			return err
		}
		for i, item := range list {
			obj, ok := item.(*orderedJSON)
			if !ok {
				return jsonFail(fmt.Sprintf("sql.%s[%d] is not an object", verb, i))
			}
			key := verb
			for _, part := range []string{"dialect", "version"} {
				if v, ok := obj.values[part]; ok {
					s, err := jsonString(v, "sql."+verb+"."+part)
					if err != nil {
						return err
					}
					key += "-" + s
				}
			}
			stmt, err := jsonString(obj.values["stmt"], "sql."+verb+".stmt")
			if err != nil {
				return err
			}
			if strings.ContainsAny(stmt, "\r\n") {
				return jsonFail("sql." + verb + " statement contains a line break")
			}
			b.WriteString("#$" + key + ": " + stmt + "\n")
		}
	}
	return nil
}

func (w *jsonTextWriter) aggregates() ([]Aggregation, error) {
	agg, err := jsonObject(w.root.values["aggregates"], "aggregates")
	if err != nil || agg == nil {
		return nil, err
	}
	var out []Aggregation
	for _, name := range agg.keys {
		list, err := jsonArray(agg.values[name], "aggregates."+name)
		if err != nil {
			return nil, err
		}
		a := Aggregation{Name: name, Values: make([]string, len(list))}
		for i, v := range list {
			if v == nil {
				continue
			}
			if a.Values[i], err = jsonScalarText(v, "aggregates."+name); err != nil {
				return nil, err
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func (w *jsonTextWriter) data() ([][]string, error) {
	list, err := jsonArray(w.root.values["data"], "data")
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(list))
	for r, item := range list {
		cells, ok := item.([]any)
		if !ok {
			return nil, jsonFail(fmt.Sprintf("data[%d] is not an array", r))
		}
		row := make([]string, len(cells))
		for i, v := range cells {
			if v == nil {
				continue
			}
			s, err := jsonScalarText(v, fmt.Sprintf("data[%d][%d]", r, i))
			if err != nil {
				return nil, err
			}
			if strings.ContainsAny(s, "\r\n") {
				return nil, jsonFail(fmt.Sprintf("data[%d][%d] contains a line break; text rows are single-line", r, i))
			}
			row[i] = s
		}
		rows[r] = row
	}
	return rows, nil
}

// needsQuoting reports whether some value cannot be written with quote=none:
// it contains the delimiter, starts a row with "#", or is the lone null of a
// one-column row with no null= marker to stand for it (written as "").
func needsQuoting(delim rune, nullMarker string, header []string, rows [][]string, aggs []Aggregation) bool {
	d := string(delim)
	check := func(fields []string) bool {
		if len(fields) == 1 && fields[0] == "" && nullMarker == "" {
			return true
		}
		for i, f := range fields {
			if strings.Contains(f, d) || (i == 0 && strings.HasPrefix(f, "#")) {
				return true
			}
		}
		return false
	}
	if check(header) {
		return true
	}
	for _, r := range rows {
		if check(r) {
			return true
		}
	}
	for _, a := range aggs {
		for _, v := range a.Values {
			if strings.Contains(v, d) {
				return true
			}
		}
	}
	return false
}

// joinRow renders one data or header record. A first field starting with
// "#" is quoted so the line is not read as a meta line.
func joinRow(fields []string, d Dialect) string {
	line := joinCSVFields(fields, d)
	if d.QuoteEnabled && len(fields) > 0 && strings.HasPrefix(fields[0], "#") && !strings.HasPrefix(line, string(d.Quote)) {
		q := string(d.Quote)
		line = q + strings.ReplaceAll(fields[0], q, q+q) + q + line[len(fields[0]):]
	}
	return line
}

func jsonNullMarker(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []any:
		if len(t) == 0 {
			return "", nil
		}
		return jsonString(t[0], "csv.null")
	}
	return "", jsonFail("csv.null is neither a string nor an array of strings")
}

// jsonScalarText renders a JSON scalar as its text-form value. Numbers keep
// their exact lexical form (the decoder runs with UseNumber).
func jsonScalarText(v any, what string) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case nil:
		return "", nil
	}
	return "", jsonFail(what + " is not a scalar")
}

func jsonString(v any, what string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", jsonFail(what + " is not a string")
	}
	return s, nil
}

func jsonInt(v any, what string) (int, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, jsonFail(what + " is not a number")
	}
	i, err := strconv.Atoi(n.String())
	if err != nil {
		return 0, jsonFail(what + " is not an integer")
	}
	return i, nil
}

func jsonObject(v any, what string) (*orderedJSON, error) {
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(*orderedJSON)
	if !ok {
		return nil, jsonFail(what + " is not an object")
	}
	return obj, nil
}

func jsonArray(v any, what string) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, jsonFail(what + " is not an array")
	}
	return list, nil
}

// decodeOrdered decodes JSON keeping object member order (objects become
// *orderedJSON) and numbers as json.Number.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the top-level value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := newOrderedJSON()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyTok.(string)
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj.set(key, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			list := []any{}
			for dec.More() {
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return list, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	}
	return tok, nil
}
