package excsv

import (
	"strconv"
	"strings"
)

var implementedVersions = map[string]bool{
	"0.2": true,
	"0.3": true,
	"0.4": true,
	"0.5": true,
}

var knownColumnAttrs = map[string]bool{
	"name": true, "index": true, "title": true, "description": true,
	"type": true, "format": true, "unit": true, "role": true, "agg": true,
	"order": true, "separator": true, "enum": true, "pattern": true,
	"regexp_dialect": true, "min": true, "max": true, "len_min": true,
	"len_max": true, "unique": true, "required": true, "default": true,
	"formula": true, "materialized": true,
}

func (r *ParseResult) warn(kind ErrorKind, line int, msg string) {
	if r == nil {
		return
	}
	r.Warnings = append(r.Warnings, newIssue(kind, line, msg))
}

func encodingIssue(data []byte, enc string) *Issue {
	e := strings.ToUpper(strings.TrimSpace(enc))
	if e == "" || e == "UTF-8" || e == "UTF8" {
		return nil
	}
	if strings.HasPrefix(e, "UTF-16") || strings.HasPrefix(e, "UTF16") ||
		strings.HasPrefix(e, "UTF-32") || strings.HasPrefix(e, "UTF32") {
		iss := newIssue(ErrEncodingNotASCIICompat, 0, "encoding "+enc+" is not ASCII-compatible")
		return &iss
	}
	if !known8BitEncoding(e) {
		iss := newIssue(ErrEncodingUnsupported, 0, "unsupported encoding "+enc)
		return &iss
	}
	if validUTF8(data) {
		iss := newIssue(ErrEncodingMismatch, 0, "content appears UTF-8 but encoding declares "+enc)
		return &iss
	}
	return nil
}

func known8BitEncoding(e string) bool {
	switch e {
	case "ISO-8859-1", "ISO8859-1", "LATIN1", "WINDOWS-1252", "US-ASCII", "ASCII", "CP1252":
		return true
	default:
		return false
	}
}

func collectHeaderWarnings(res *ParseResult, opts ParseOptions) {
	h := &res.Doc.Header
	if h.HasMagicLine && h.Version != "" && !implementedVersions[h.Version] {
		res.warn(ErrUnknownVersion, 1, "unknown version="+h.Version)
	}
	if cs, ok := h.Fields["checksum"]; ok && cs != "" {
		parsed, kind := classifyChecksumField(cs)
		if kind != "" {
			res.warn(kind, 1, "checksum="+cs)
			h.Checksum = nil
		} else {
			h.Checksum = parsed
		}
	}
	if opts.PackRole == "" && !opts.ExpectZipInner && h.OriginalSize != nil {
		res.warn(ErrOriginalSizeOnPlain, 1, "original-size= on a plain file")
	}
	if opts.PackRole == "" {
		for k := range h.Fields {
			if isReservedHeaderKey(k) {
				res.warn(ErrPackKeyOnPlain, 1, "pack-only key "+k+" on plain/row file")
				break
			}
		}
	}
}

func collectSQLWarnings(res *ParseResult, stmt SQLStatement) {
	if !knownSQLVerbs[stmt.Verb] {
		res.warn(ErrSQLUnknownVerb, stmt.Line, "unknown #$ verb "+stmt.Verb)
	}
	if stmt.Qualified && stmt.Dialect != "" && !isKnownDialect(stmt.Dialect) {
		res.warn(ErrSQLUnknownDialect, stmt.Line, "unknown SQL dialect "+stmt.Dialect)
	}
}

func collectMetaWarnings(res *ParseResult) {
	doc := res.Doc
	applyDuplicateColumnLastWins(res)

	for _, col := range doc.Meta.Columns {
		for k := range col.Attrs {
			if strings.HasPrefix(k, "x-") {
				continue
			}
			if !knownColumnAttrs[k] {
				res.warn(ErrColumnUnknownAttribute, col.Line, "unknown #column attribute "+k)
				break
			}
		}
	}

	width := physicalWidth(doc)
	if n := columnCountFromSchema(doc.Meta.Columns); width > 0 && n > width {
		res.warn(ErrColumnCountMismatch, 0, "schema column count exceeds physical width")
	}
	for _, col := range doc.Meta.Columns {
		if idx, ok := col.Attrs["index"]; ok && width > 0 {
			if n, err := strconv.Atoi(idx); err == nil && n >= width {
				res.warn(ErrColumnCountMismatch, col.Line, "index= exceeds physical width")
				break
			}
		}
	}

	for _, col := range doc.Meta.Columns {
		if _, ok := col.Attrs["default"]; !ok {
			continue
		}
		idx := columnIndexForDef(doc, col)
		if idx < 0 {
			continue
		}
		for _, row := range dataRows(doc) {
			if isNullCell(doc, cellAt(doc, row, idx)) {
				res.warn(ErrDefaultWithNulls, col.Line, "default= set but column contains nulls")
				break
			}
		}
	}

	expectedCols := effectiveColumnCount(doc, width)
	for _, agg := range doc.Meta.Aggregations {
		if expectedCols > 0 && len(agg.Values) > expectedCols {
			res.warn(ErrAggArityMismatch, agg.Line, fmtAggArity(len(agg.Values), expectedCols))
		}
		for col, v := range agg.Values {
			if v == "" {
				continue
			}
			ct := columnTypeAt(doc, col)
			if ct == "" {
				continue
			}
			switch agg.Name {
			case "sum", "avg", "min", "max":
				if !isMeasureColumnType(ct) {
					res.warn(ErrAggTypeIncompatible, agg.Line, agg.Name+" incompatible with column type "+ct)
				}
			case "len_min", "len_max":
				if !isStringColumnType(ct) {
					res.warn(ErrAggTypeIncompatible, agg.Line, agg.Name+" incompatible with column type "+ct)
				}
			}
		}
	}
}

func applyDuplicateColumnLastWins(res *ParseResult) {
	cols := res.Doc.Meta.Columns
	if len(cols) < 2 {
		return
	}
	seenName := map[string]struct{}{}
	seenIndex := map[string]struct{}{}
	kept := make([]ColumnDef, 0, len(cols))
	var dups []ColumnDef
	for i := len(cols) - 1; i >= 0; i-- {
		col := cols[i]
		name := col.Attrs["name"]
		idx := col.Attrs["index"]
		dup := false
		if name != "" {
			if _, ok := seenName[name]; ok {
				dup = true
			}
		}
		if idx != "" {
			if _, ok := seenIndex[idx]; ok {
				dup = true
			}
		}
		if dup {
			dups = append(dups, col)
			continue
		}
		if name != "" {
			seenName[name] = struct{}{}
		}
		if idx != "" {
			seenIndex[idx] = struct{}{}
		}
		kept = append(kept, col)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	res.Doc.Meta.Columns = kept
	for _, col := range dups {
		res.warn(ErrDuplicateColumn, col.Line, "duplicate #column; last-wins")
	}
}

func physicalWidth(doc *Document) int {
	if doc.Data.HasHeaderRow {
		return len(doc.Data.HeaderRow)
	}
	if len(doc.Data.Rows) > 0 {
		return len(doc.Data.Rows[0])
	}
	return 0
}

func columnIndexForDef(doc *Document, col ColumnDef) int {
	if v, ok := col.Attrs["index"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return -1
		}
		return n
	}
	name := col.Attrs["name"]
	if name == "" {
		return -1
	}
	for i, cell := range doc.Data.HeaderRow {
		if cell == name || cell == col.Attrs["title"] {
			return i
		}
	}
	for i, c := range doc.Meta.Columns {
		if c.Attrs["name"] == name {
			if v, ok := c.Attrs["index"]; ok {
				if n, err := strconv.Atoi(v); err == nil {
					return n
				}
			}
			return i
		}
	}
	return -1
}

func appendExtsvWarning(res *ParseResult, path string) {
	if res == nil || res.Doc == nil {
		return
	}
	if sidecarExtMismatch(path, res.Doc.Header) {
		for _, w := range res.Warnings {
			if w.Kind == ErrExtsvDelimMismatch {
				return
			}
		}
		res.warn(ErrExtsvDelimMismatch, 1, ".extsv should declare delim=tab")
	}
}

func applyChecksumWarning(res *ParseResult, dataSection string, sidecar bool) {
	if res.Doc.Header.Checksum == nil {
		return
	}
	if err := verifyChecksum(dataSection, res.Doc.Header.Checksum); err != nil {
		kind := ErrChecksumMismatch
		if sidecar {
			kind = ErrSidecarChecksumMismatch
		}
		if pe, ok := err.(*ParseError); ok {
			if pe.Issue.Kind == ErrChecksumUnknownAlgorithm {
				kind = ErrChecksumUnknownAlgorithm
			}
		}
		res.warn(kind, 1, err.Error())
	}
}

func applyRowsMismatchWarning(res *ParseResult) {
	if res.Doc.Header.Rows == nil {
		return
	}
	// No data section was read (sidecar metadata-only, row-ZIP comment preview,
	// stub) — comparing rows= against zero body rows is not meaningful yet.
	if !res.Doc.Data.HasHeaderRow && len(res.Doc.Data.Rows) == 0 {
		return
	}
	if res.Doc.RowCount() != *res.Doc.Header.Rows {
		res.warn(ErrRowsMismatch, 1, "rows= does not match data row count")
	}
}
