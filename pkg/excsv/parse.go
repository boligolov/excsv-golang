package excsv

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

func ParseBytes(data []byte, opts ParseOptions) (*ParseResult, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	enc := "UTF-8"
	if firstLine := firstLineBytes(data); strings.HasPrefix(firstLine, "#!excsv") {
		if fields, err := parseHeaderLine(firstLine); err == nil {
			if e, ok := fields["encoding"]; ok {
				enc = e
			}
		}
	}

	if strings.EqualFold(enc, "UTF-8") || enc == "" {
		if !utf8.Valid(data) {
			return nil, fail(ErrInvalidUTF8, 0, "invalid UTF-8")
		}
	} else if encodingMismatch(data, enc) {
		return nil, fail(ErrEncodingMismatch, 0, "content appears UTF-8 but encoding declares "+enc)
	}

	if len(data) == 0 {
		doc := &Document{Form: FormPlain, Header: Header{Fields: map[string]string{}}}
		if err := applyHeaderDefaults(&doc.Header); err != nil {
			return nil, err
		}
		return &ParseResult{Doc: doc}, nil
	}

	records := splitRecords(data)
	return parseRecords(records, string(data), opts)
}

func firstLineBytes(data []byte) string {
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return string(data)
	}
	line := data[:i]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return string(line)
}

type record struct {
	num   int
	text  string
	start int
	end   int // exclusive, content only
}

func splitRecords(data []byte) []record {
	var out []record
	lineNo := 1
	i := 0
	for i <= len(data) {
		start := i
		j := i
		for j < len(data) && data[j] != '\n' && data[j] != '\r' {
			j++
		}
		text := string(data[start:j])
		out = append(out, record{num: lineNo, text: text, start: start, end: j})
		lineNo++
		if j >= len(data) {
			break
		}
		if data[j] == '\r' {
			j++
		}
		if j < len(data) && data[j] == '\n' {
			j++
		}
		i = j
	}
	return out
}

func parseRecords(records []record, full string, opts ParseOptions) (*ParseResult, error) {
	doc := &Document{
		Form:   FormPlain,
		Header: Header{Fields: map[string]string{}},
	}
	idx := 0

	// skip trailing empty record from final newline for logic, but keep for checksum
	for len(records) > 0 && records[len(records)-1].text == "" && len(records) > 1 {
		records = records[:len(records)-1]
	}

	if len(records) == 0 {
		if err := applyHeaderDefaults(&doc.Header); err != nil {
			return nil, err
		}
		return &ParseResult{Doc: doc}, nil
	}

	first := records[0].text
	if strings.HasPrefix(first, "#!excsv") {
		fields, err := parseHeaderLine(first)
		if err != nil {
			return nil, err
		}
		doc.Header.Fields = fields
		doc.Header.HasMagicLine = true
		if v, ok := fields["version"]; ok {
			doc.Header.Version = v
		}
		idx = 1
	}

	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return nil, err
	}

	if opts.ExpectZipInner {
		if doc.Header.OriginalSize == nil {
			return nil, fail(ErrZipMissingOriginalSize, 1, "missing original-size in zipped inner file")
		}
		if opts.ZipUncompressedSize > 0 && *doc.Header.OriginalSize != opts.ZipUncompressedSize {
			return nil, fail(ErrZipOriginalSizeMismatch, 1, "original-size does not match ZIP entry size")
		}
	}

	d := doc.Header.Dialect()
	metaStart := idx
	lastWasSQL := false
	for idx < len(records) {
		line := records[idx].text
		ln := records[idx].num
		if line == "" {
			idx++
			continue
		}
		if !strings.HasPrefix(line, "#") {
			if lastWasSQL && !strings.ContainsRune(line, d.Delim) {
				return nil, fail(ErrSQLEmbeddedNewline, ln, "SQL statement spans multiple lines")
			}
			break
		}
		if strings.HasPrefix(line, "#!excsv") {
			return nil, fail(ErrHeaderMalformedMagic, ln, "duplicate header line")
		}
		if strings.HasPrefix(line, "##") {
			if opts.PreserveHumanComments {
				doc.Meta.HumanComments = append(doc.Meta.HumanComments, line)
			}
			lastWasSQL = false
			idx++
			continue
		}
		if strings.HasPrefix(line, "#excsv") {
			return nil, fail(ErrHeaderMalformedMagic, ln, "malformed header magic in meta section")
		}
		if isReservedMetaPrefix(line) {
			lastWasSQL = false
			idx++
			continue
		}
		switch {
		case strings.HasPrefix(line, "#@"):
			key, val, ok := skipColonValue(line, "#@")
			if ok {
				doc.Meta.FileMeta = upsertKV(doc.Meta.FileMeta, key, val)
			}
			lastWasSQL = false
		case strings.HasPrefix(line, "#column"):
			attrs, err := parseKVLine(strings.TrimPrefix(line, "#column "), ln)
			if err != nil {
				return nil, err
			}
			doc.Meta.Columns = append(doc.Meta.Columns, ColumnDef{Attrs: attrs, Line: ln})
			lastWasSQL = false
		case strings.HasPrefix(line, "#%"):
			rest := line[2:]
			colon := strings.IndexByte(rest, ':')
			if colon < 0 {
				idx++
				continue
			}
			name := rest[:colon]
			payload := rest[colon+1:]
			if len(payload) > 0 && payload[0] == ' ' {
				payload = payload[1:]
			}
			vals, err := parseAggPayload(payload, d)
			if err != nil {
				if pe, ok := err.(*ParseError); ok {
					pe.Issue.Line = ln
					return nil, pe
				}
				return nil, err
			}
			doc.Meta.Aggregations = append(doc.Meta.Aggregations, Aggregation{Name: name, Values: vals, Line: ln})
			lastWasSQL = false
		case strings.HasPrefix(line, "#csvw"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "#csvw"))
			doc.Meta.CSVW = &payload
			lastWasSQL = false
		case strings.HasPrefix(line, "#$"):
			stmt, err := parseSQLLine(line, ln)
			if err != nil {
				return nil, err
			}
			doc.Meta.SQL = append(doc.Meta.SQL, *stmt)
			lastWasSQL = true
		default:
			lastWasSQL = false
		}
		idx++
	}

	dataRecords := records[idx:]
	// drop single trailing empty line from data for row parsing but keep for checksum
	parseRecords := dataRecords
	if len(parseRecords) > 0 && parseRecords[len(parseRecords)-1].text == "" {
		parseRecords = parseRecords[:len(parseRecords)-1]
	}

	colCount := 0
	rowStart := 0
	if doc.Header.HeaderRow && len(parseRecords) > 0 {
		fields, err := splitCSVFields(parseRecords[0].text, d)
		if err != nil {
			return nil, wrapLineErr(err, parseRecords[0].num)
		}
		if len(fields) > 0 && strings.HasPrefix(fields[0], "#") && !isFirstFieldQuoted(parseRecords[0].text, d) {
			return nil, fail(ErrFirstFieldHashUnquoted, parseRecords[0].num, "first field starts with # unquoted")
		}
		doc.Data.HasHeaderRow = true
		doc.Data.HeaderRow = fields
		colCount = len(fields)
		rowStart = 1
	} else if !doc.Header.HeaderRow {
		colCount = columnCountFromSchema(doc.Meta.Columns)
	}

	if err := validateColumns(doc, colCount); err != nil {
		return nil, err
	}

	expectedCols := effectiveColumnCount(doc, colCount)
	for _, agg := range doc.Meta.Aggregations {
		if expectedCols > 0 && len(agg.Values) != expectedCols {
			return nil, fail(ErrAggArityMismatch, agg.Line, fmtAggArity(len(agg.Values), expectedCols))
		}
	}

	for i := rowStart; i < len(parseRecords); i++ {
		dl := parseRecords[i]
		fields, err := splitCSVFields(dl.text, d)
		if err != nil {
			return nil, wrapLineErr(err, dl.num)
		}
		if !d.QuoteEnabled && len(fields) > 0 && strings.HasPrefix(fields[0], "#") {
			return nil, fail(ErrFirstFieldHashUnquoted, dl.num, "first field starts with # unquoted")
		}
		if expectedCols > 0 {
			if len(fields) != expectedCols {
				if !d.QuoteEnabled && len(fields) > expectedCols && doc.Header.Fields["quote"] == "none" {
					return nil, fail(ErrQuoteNoneDelimiterInValue, dl.num, "delimiter in unquoted value")
				}
				return nil, fail(ErrDataRowArityMismatch, dl.num, fmtRowArity(len(fields), expectedCols))
			}
		}
		doc.Data.Rows = append(doc.Data.Rows, fields)
	}

	if doc.Header.Rows != nil && len(doc.Data.Rows) != *doc.Header.Rows {
		return nil, fail(ErrHeaderInvalidValue, 1, "rows= does not match data row count")
	}

	if doc.Header.Checksum != nil && len(dataRecords) > 0 {
		dataSection := extractDataSection(full, records, idx)
		if err := verifyChecksum(dataSection, doc.Header.Checksum); err != nil {
			return nil, err
		}
	}

	_ = metaStart
	return &ParseResult{Doc: doc}, nil
}

func extractDataSection(full string, records []record, dataIdx int) string {
	if dataIdx >= len(records) {
		return ""
	}
	start := records[dataIdx].start
	end := len(full)
	if len(records) > 0 {
		last := records[len(records)-1]
		end = last.end
		// include trailing newline after last data line if present in file
		if end < len(full) {
			if full[end] == '\r' {
				if end+1 < len(full) && full[end+1] == '\n' {
					end += 2
				} else {
					end++
				}
			} else if full[end] == '\n' {
				end++
			}
		}
	}
	return full[start:end]
}

func wrapLineErr(err error, line int) error {
	if pe, ok := err.(*ParseError); ok {
		pe.Issue.Line = line
		return pe
	}
	return err
}

func upsertKV(list []KV, key, val string) []KV {
	for i := range list {
		if list[i].Key == key {
			list[i].Value = val
			return list
		}
	}
	return append(list, KV{Key: key, Value: val})
}

func columnCountFromSchema(cols []ColumnDef) int {
	max := -1
	for _, c := range cols {
		if idx, ok := c.Attrs["index"]; ok {
			n := 0
			okNum := true
			for _, ch := range idx {
				if ch < '0' || ch > '9' {
					okNum = false
					break
				}
				n = n*10 + int(ch-'0')
			}
			if okNum && n > max {
				max = n
			}
		}
	}
	if max >= 0 {
		return max + 1
	}
	if len(cols) > 0 {
		return len(cols)
	}
	return 0
}

func effectiveColumnCount(doc *Document, headerWidth int) int {
	if doc.Header.HeaderRow && headerWidth > 0 {
		return headerWidth
	}
	n := columnCountFromSchema(doc.Meta.Columns)
	if n > 0 {
		return n
	}
	return headerWidth
}

func validateColumns(doc *Document, headerWidth int) error {
	for i, col := range doc.Meta.Columns {
		if doc.Header.HeaderRow {
			name, hasName := col.Attrs["name"]
			if !hasName || name == "" {
				return fail(ErrColumnMissingName, col.Line, "missing name= when header=1")
			}
			title, hasTitle := col.Attrs["title"]
			if headerWidth > 0 && len(doc.Data.HeaderRow) > i {
				cell := doc.Data.HeaderRow[i]
				if hasTitle && cell != title {
					return fail(ErrColumnTitleHeaderMismatch, col.Line, "title does not match header row")
				}
				if !hasTitle && cell != name {
					return fail(ErrColumnNameHeaderMismatch, col.Line, "name does not match header row")
				}
			}
		} else if _, ok := col.Attrs["index"]; !ok {
			return fail(ErrColumnMissingIndex, col.Line, "missing index= when header=0")
		}
	}
	return nil
}

func isFirstFieldQuoted(line string, d Dialect) bool {
	if !d.QuoteEnabled {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(line), string(d.Quote))
}

func fmtAggArity(got, want int) string {
	return fmt.Sprintf("aggregation has %d values, expected %d", got, want)
}

func fmtRowArity(got, want int) string {
	return fmt.Sprintf("row has %d fields, expected %d", got, want)
}
