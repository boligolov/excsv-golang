package excsv

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type ImportOptions struct {
	// DelimName / QuoteName describe the output ExCSV header (and inline data encoding).
	// Input bytes are always parsed using detected dialect (sniff + SourcePath hint).
	DelimName  string
	QuoteName  string
	NoHeader   bool
	AddColumns bool
	Checksum   bool
	Strict     bool
	Sidecar    bool
	Reference  string // sidecar reference= path; default basename of SourcePath
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

	inputDelimName, err := resolveImportDelim("", lines, opts.SourcePath)
	if err != nil {
		return nil, err
	}
	inputDelim, err := resolveDelim(inputDelimName)
	if err != nil {
		return nil, err
	}

	inputQuoteName := sniffInputQuote(lines, inputDelim)
	inputQuote, inputQuoteEnabled, err := resolveQuote(inputQuoteName)
	if err != nil {
		return nil, err
	}
	inputD := Dialect{Delim: inputDelim, Quote: inputQuote, QuoteEnabled: inputQuoteEnabled}

	outputDelimName := opts.DelimName
	if outputDelimName == "" {
		outputDelimName = inputDelimName
	} else if _, err := resolveDelim(outputDelimName); err != nil {
		return nil, err
	}

	outputQuoteName := opts.QuoteName
	if outputQuoteName == "" {
		outputQuoteName = inputQuoteName
	} else if _, _, err := resolveQuote(outputQuoteName); err != nil {
		return nil, err
	}

	hasHeader := !opts.NoHeader
	var warnings []Issue
	var headerRow []string
	var dataRows [][]string
	expectedCols := 0

	if hasHeader {
		fields, err := splitCSVFields(lines[0], inputD)
		if err != nil {
			return nil, wrapLineErr(err, lineNums[0])
		}
		if err := firstFieldHashError(lines[0], fields, inputD, lineNums[0]); err != nil {
			return nil, err
		}
		headerRow = fields
		expectedCols = len(fields)
		for i := 1; i < len(lines); i++ {
			row, w, err := parseCSVRow(lines[i], lineNums[i], inputD, expectedCols, inputQuoteName, opts.Strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			dataRows = append(dataRows, row)
		}
	} else {
		for i, line := range lines {
			row, w, err := parseCSVRow(line, lineNums[i], inputD, expectedCols, inputQuoteName, opts.Strict)
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
		"delim":   outputDelimName,
		"rows":    strconv.Itoa(len(dataRows)),
	}
	if outputQuoteName != "" && outputQuoteName != "none" {
		fields["quote"] = outputQuoteName
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

	if opts.Sidecar {
		ref := opts.Reference
		if ref == "" {
			ref = filepath.Base(opts.SourcePath)
		}
		doc.Source.Profile = ProfileSidecar
		doc.Source.Reference = ref
		doc.Source.SidecarPath = opts.SourcePath
		doc.Header.Fields["reference"] = ref
		doc.Data = DataSection{}
		if opts.Checksum {
			records := splitRecords(data)
			section := extractDataSection(string(data), records, 0)
			if err := doc.SetDataChecksumFromSection(section, "sha256"); err != nil {
				return nil, err
			}
		}
	} else if opts.Checksum {
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

func sniffInputQuote(lines []string, delim rune) string {
	sample := nonEmptyLines(lines, 8)
	if len(sample) == 0 {
		return "none"
	}
	dq := Dialect{Delim: delim, Quote: '"', QuoteEnabled: true}
	dn := Dialect{Delim: delim, QuoteEnabled: false}
	doubleOK := true
	noneOK := true
	for _, line := range sample {
		if strings.Contains(line, `"`) {
			if _, err := splitCSVFields(line, dq); err != nil {
				doubleOK = false
			}
		}
		if _, err := splitCSVFields(line, dn); err != nil {
			noneOK = false
		}
	}
	if doubleOK && strings.Contains(strings.Join(sample, "\n"), `"`) {
		return "double"
	}
	if noneOK {
		return "none"
	}
	return sniffQuote(sample[0], delim)
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
