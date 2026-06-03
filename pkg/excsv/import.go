package excsv

import (
	"regexp"
	"strconv"
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
	data = stripUTF8BOM(data)
	if len(data) > 0 && !utf8.Valid(data) {
		return nil, fail(ErrInvalidUTF8, 0, "invalid UTF-8")
	}

	if len(data) == 0 {
		return minimalImportDoc()
	}

	lines, lineNums := linesFromRecords(splitRecords(data))

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
		if err := firstFieldHashError(lines[0], fields, d, lineNums[0]); err != nil {
			return nil, err
		}
		headerRow = fields
		expectedCols = len(fields)
		for i := 1; i < len(lines); i++ {
			row, w, err := parseCSVRow(lines[i], lineNums[i], d, expectedCols, quoteName, opts.Strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			dataRows = append(dataRows, row)
		}
	} else {
		for i, line := range lines {
			row, w, err := parseCSVRow(line, lineNums[i], d, expectedCols, quoteName, opts.Strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			dataRows = append(dataRows, row)
			if expectedCols == 0 && len(row) > 0 {
				expectedCols = len(row)
			}
		}
	}

	fields := map[string]string{
		"version": "0.2",
		"delim":   delimName,
		"rows":    strconv.Itoa(len(dataRows)),
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
		for _, name := range headerRow {
			if !columnNameRE.MatchString(name) {
				return nil, fail(ErrColumnMalformedAttribute, lineNums[0], "invalid column name "+name)
			}
			doc.Meta.Columns = append(doc.Meta.Columns, ColumnDef{
				Attrs: map[string]string{"name": name, "type": "text"},
				Line:  lineNums[0],
			})
		}
	}

	if opts.Checksum {
		if err := doc.SetDataChecksum("sha256"); err != nil {
			return nil, err
		}
	}

	return &ImportResult{Doc: doc, Warnings: warnings}, nil
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
	preferred := delimNameForPath(sourcePath)

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
