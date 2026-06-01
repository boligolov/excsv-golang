package excsv

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ImportOptions struct {
	DelimName  string // empty = sniff
	QuoteName  string // empty = sniff
	NoHeader   bool
	AddColumns bool
	Checksum   bool
	Strict     bool
	FileMeta   []KV
	SourcePath string
}

type ImportResult struct {
	Doc      *Document
	Warnings []Issue
}

var columnNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func ImportDelimited(data []byte, opts ImportOptions) (*ImportResult, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	if len(data) > 0 && !utf8.Valid(data) {
		return nil, fail(ErrInvalidUTF8, 0, "invalid UTF-8")
	}

	if len(data) == 0 {
		return minimalImportDoc()
	}

	lines, lineNums := splitImportLines(data)
	lines, lineNums = trimTrailingEmptyLine(lines, lineNums)

	if allLinesEmpty(lines) {
		return minimalImportDoc()
	}

	delimName, err := resolveImportDelim(opts.DelimName, lines, opts.SourcePath)
	if err != nil {
		return nil, err
	}
	delim, err := resolveDelim(delimName)
	if err != nil {
		return nil, err
	}

	quoteName := opts.QuoteName
	if quoteName == "" {
		quoteName = sniffQuote(lines[0], delim)
	}
	quote, quoteEnabled, err := resolveQuote(quoteName)
	if err != nil {
		return nil, err
	}
	d := Dialect{Delim: delim, Quote: quote, QuoteEnabled: quoteEnabled}

	hasHeader := !opts.NoHeader
	var warnings []Issue
	var headerRow []string
	var dataRows [][]string
	expectedCols := 0

	if hasHeader {
		fields, err := splitCSVFields(lines[0], d)
		if err != nil {
			return nil, wrapLineErr(err, lineNums[0])
		}
		if len(fields) > 0 && strings.HasPrefix(fields[0], "#") && !isFirstFieldQuoted(lines[0], d) {
			return nil, fail(ErrFirstFieldHashUnquoted, lineNums[0], "first field starts with # unquoted")
		}
		headerRow = fields
		expectedCols = len(fields)
		for i := 1; i < len(lines); i++ {
			row, w, err := parseImportRow(lines[i], lineNums[i], d, expectedCols, opts.Strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			dataRows = append(dataRows, row)
		}
	} else {
		for i, line := range lines {
			if expectedCols == 0 {
				fields, err := splitCSVFields(line, d)
				if err != nil {
					return nil, wrapLineErr(err, lineNums[i])
				}
				if !d.QuoteEnabled && len(fields) > 0 && strings.HasPrefix(fields[0], "#") {
					return nil, fail(ErrFirstFieldHashUnquoted, lineNums[i], "first field starts with # unquoted")
				}
				expectedCols = len(fields)
				dataRows = append(dataRows, fields)
				continue
			}
			row, w, err := parseImportRow(line, lineNums[i], d, expectedCols, opts.Strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			dataRows = append(dataRows, row)
		}
	}

	fields := map[string]string{
		"version": "0.2",
		"delim":   delimName,
		"rows":    itoa(len(dataRows)),
	}
	if quoteName != "" && quoteName != "none" {
		fields["quote"] = quoteName
	}
	if !hasHeader {
		fields["header"] = "0"
	}

	doc := &Document{
		Form: FormPlain,
		Header: Header{
			Fields:       fields,
			HasMagicLine: true,
			Version:      "0.2",
		},
		Data: DataSection{
			HasHeaderRow: hasHeader,
			HeaderRow:    headerRow,
			Rows:         dataRows,
		},
	}
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return nil, err
	}

	for _, kv := range opts.FileMeta {
		doc.Meta.FileMeta = upsertKV(doc.Meta.FileMeta, kv.Key, kv.Value)
	}

	if opts.AddColumns && hasHeader {
		for i, name := range headerRow {
			if !columnNameRE.MatchString(name) {
				return nil, fail(ErrColumnMalformedAttribute, lineNums[0], "invalid column name "+name)
			}
			doc.Meta.Columns = append(doc.Meta.Columns, ColumnDef{
				Attrs: map[string]string{"name": name, "type": "text"},
				Line:  lineNums[0],
			})
			_ = i
		}
	}

	if opts.Checksum {
		if err := doc.SetDataChecksum("sha256"); err != nil {
			return nil, err
		}
	}

	return &ImportResult{Doc: doc, Warnings: warnings}, nil
}

func parseImportRow(line string, lineNo int, d Dialect, expectedCols int, strict bool) ([]string, []Issue, error) {
	fields, err := splitCSVFields(line, d)
	if err != nil {
		return nil, nil, wrapLineErr(err, lineNo)
	}
	if !d.QuoteEnabled && len(fields) > 0 && strings.HasPrefix(fields[0], "#") {
		return nil, nil, fail(ErrFirstFieldHashUnquoted, lineNo, "first field starts with # unquoted")
	}
	if expectedCols == 0 {
		return fields, nil, nil
	}
	if len(fields) == expectedCols {
		return fields, nil, nil
	}
	if strict {
		if !d.QuoteEnabled && len(fields) > expectedCols {
			return nil, nil, fail(ErrQuoteNoneDelimiterInValue, lineNo, "delimiter in unquoted value")
		}
		return nil, nil, fail(ErrDataRowArityMismatch, lineNo, fmtRowArity(len(fields), expectedCols))
	}
	var warnings []Issue
	if len(fields) < expectedCols {
		warnings = append(warnings, newIssue(ErrDataRowArityMismatch, lineNo,
			"padded row from "+itoa(len(fields))+" to "+itoa(expectedCols)+" fields"))
		for len(fields) < expectedCols {
			fields = append(fields, "")
		}
	} else {
		warnings = append(warnings, newIssue(ErrDataRowArityMismatch, lineNo,
			"truncated row from "+itoa(len(fields))+" to "+itoa(expectedCols)+" fields"))
		fields = fields[:expectedCols]
	}
	return fields, warnings, nil
}

func splitImportLines(data []byte) ([]string, []int) {
	var lines []string
	var nums []int
	lineNo := 1
	i := 0
	for i <= len(data) {
		start := i
		j := i
		for j < len(data) && data[j] != '\n' && data[j] != '\r' {
			j++
		}
		lines = append(lines, string(data[start:j]))
		nums = append(nums, lineNo)
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
	return lines, nums
}

func trimTrailingEmptyLine(lines []string, nums []int) ([]string, []int) {
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1], nums[:len(nums)-1]
	}
	return lines, nums
}

var sniffDelims = []struct {
	name string
	r    rune
}{
	{"comma", ','},
	{"tab", '\t'},
	{"semicolon", ';'},
	{"pipe", '|'},
}

func resolveImportDelim(explicit string, lines []string, sourcePath string) (string, error) {
	if explicit != "" {
		if _, err := resolveDelim(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	preferred := ""
	lower := strings.ToLower(filepath.Base(sourcePath))
	switch {
	case strings.HasSuffix(lower, ".tsv"):
		preferred = "tab"
	case strings.HasSuffix(lower, ".csv"):
		preferred = "comma"
	}

	sample := nonEmptyLines(lines, 5)
	if len(sample) == 0 {
		return "comma", nil
	}

	bestName := "comma"
	bestScore := -1
	for _, cand := range sniffDelims {
		score := scoreDelimiter(sample, cand.r)
		if preferred == cand.name {
			score++
		}
		if score > bestScore {
			bestScore = score
			bestName = cand.name
		}
	}
	if bestScore <= 0 {
		if preferred != "" {
			return preferred, nil
		}
		return "comma", nil
	}
	return bestName, nil
}

func nonEmptyLines(lines []string, max int) []string {
	var out []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}

func scoreDelimiter(lines []string, delim rune) int {
	ds := string(delim)
	found := false
	for _, line := range lines {
		if strings.Contains(line, ds) {
			found = true
			break
		}
	}
	if !found {
		return -1
	}
	d := Dialect{Delim: delim, QuoteEnabled: false}
	var counts []int
	for _, line := range lines {
		fields, err := splitCSVFields(line, d)
		if err != nil || len(fields) < 1 {
			return -1
		}
		counts = append(counts, len(fields))
	}
	if len(counts) == 0 {
		return 0
	}
	first := counts[0]
	for _, c := range counts[1:] {
		if c != first {
			return 0
		}
	}
	return first * len(counts)
}

func sniffQuote(line string, delim rune) string {
	dQuote := Dialect{Delim: delim, Quote: '"', QuoteEnabled: true}
	if fields, err := splitCSVFields(line, dQuote); err == nil {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `"`) {
			return "double"
		}
		dNone := Dialect{Delim: delim, QuoteEnabled: false}
		if unquoted, err := splitCSVFields(line, dNone); err == nil && len(fields) > len(unquoted) {
			return "double"
		}
	}
	return "none"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func minimalImportDoc() (*ImportResult, error) {
	doc := &Document{
		Form:   FormPlain,
		Header: Header{Fields: map[string]string{"version": "0.2"}, HasMagicLine: true, Version: "0.2"},
	}
	if err := applyHeaderDefaults(&doc.Header); err != nil {
		return nil, err
	}
	return &ImportResult{Doc: doc}, nil
}

func allLinesEmpty(lines []string) bool {
	for _, line := range lines {
		if line != "" {
			return false
		}
	}
	return true
}
