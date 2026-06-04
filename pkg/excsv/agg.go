package excsv

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

var standardAggregations = map[string]bool{
	"count_nonnull":  true,
	"count_null":     true,
	"count_distinct": true,
	"sum":            true,
	"avg":            true,
	"min":            true,
	"max":            true,
	"len_min":        true,
	"len_max":        true,
}

// ComputeAggregationValues calculates per-column #% values for a standard aggregation name.
func ComputeAggregationValues(doc *Document, name string) ([]string, error) {
	if !standardAggregations[name] {
		return nil, fmt.Errorf("unknown aggregation: %s", name)
	}
	n := columnCountForAgg(doc)
	if n == 0 {
		return nil, fmt.Errorf("no columns to aggregate")
	}
	out := make([]string, n)
	for col := 0; col < n; col++ {
		v, err := computeAggColumn(doc, name, col)
		if err != nil {
			return nil, err
		}
		out[col] = v
	}
	return out, nil
}

func columnCountForAgg(doc *Document) int {
	width := 0
	if doc.Data.HasHeaderRow {
		width = len(doc.Data.HeaderRow)
	}
	return effectiveColumnCount(doc, width)
}

func columnTypeAt(doc *Document, col int) string {
	for i, c := range doc.Meta.Columns {
		idx := i
		if v, ok := c.Attrs["index"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				idx = n
			}
		}
		if idx == col {
			return strings.ToLower(strings.TrimSpace(c.Attrs["type"]))
		}
	}
	return ""
}

func isMeasureColumnType(t string) bool {
	switch t {
	case "float", "double", "decimal", "number":
		return true
	default:
		return false
	}
}

func isStringColumnType(t string) bool {
	switch t {
	case "string", "text":
		return true
	default:
		return t == ""
	}
}

func cellAt(doc *Document, row []string, col int) string {
	if col >= len(row) {
		return ""
	}
	return row[col]
}

func isNullCell(doc *Document, s string) bool {
	if s == "" {
		return true
	}
	if doc.Header.Null != "" && s == doc.Header.Null {
		return true
	}
	return false
}

func dataRows(doc *Document) [][]string {
	if doc.Data.HasHeaderRow {
		return doc.Data.Rows
	}
	return doc.Data.Rows
}

func computeAggColumn(doc *Document, name string, col int) (string, error) {
	ct := columnTypeAt(doc, col)
	switch name {
	case "count_nonnull":
		return formatInt(countNonNull(doc, col)), nil
	case "count_null":
		return formatInt(countNull(doc, col)), nil
	case "count_distinct":
		return formatInt(countDistinct(doc, col)), nil
	case "sum", "avg", "min", "max":
		if !isMeasureColumnType(ct) {
			return "", nil
		}
		return computeNumericAgg(doc, name, col)
	case "len_min", "len_max":
		if !isStringColumnType(ct) {
			return "", nil
		}
		return computeLenAgg(doc, name, col)
	default:
		return "", fmt.Errorf("unknown aggregation: %s", name)
	}
}

func countNonNull(doc *Document, col int) int {
	n := 0
	for _, row := range dataRows(doc) {
		if !isNullCell(doc, cellAt(doc, row, col)) {
			n++
		}
	}
	return n
}

func countNull(doc *Document, col int) int {
	n := 0
	for _, row := range dataRows(doc) {
		if isNullCell(doc, cellAt(doc, row, col)) {
			n++
		}
	}
	return n
}

func countDistinct(doc *Document, col int) int {
	seen := make(map[string]struct{})
	for _, row := range dataRows(doc) {
		v := cellAt(doc, row, col)
		if isNullCell(doc, v) {
			continue
		}
		seen[v] = struct{}{}
	}
	return len(seen)
}

func computeNumericAgg(doc *Document, name string, col int) (string, error) {
	var sum float64
	var count int
	minV := math.MaxFloat64
	maxV := -math.MaxFloat64
	for _, row := range dataRows(doc) {
		v := cellAt(doc, row, col)
		if isNullCell(doc, v) {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return "", fmt.Errorf("non-numeric value %q in column %d", v, col)
		}
		sum += f
		count++
		if f < minV {
			minV = f
		}
		if f > maxV {
			maxV = f
		}
	}
	if count == 0 {
		return "", nil
	}
	switch name {
	case "sum":
		return formatDecimal(sum), nil
	case "avg":
		return formatDecimal(sum / float64(count)), nil
	case "min":
		return formatDecimal(minV), nil
	case "max":
		return formatDecimal(maxV), nil
	default:
		return "", nil
	}
}

func computeLenAgg(doc *Document, name string, col int) (string, error) {
	minLen := -1
	maxLen := -1
	for _, row := range dataRows(doc) {
		v := cellAt(doc, row, col)
		if isNullCell(doc, v) {
			continue
		}
		l := len(v)
		if minLen < 0 || l < minLen {
			minLen = l
		}
		if l > maxLen {
			maxLen = l
		}
	}
	if minLen < 0 {
		return "", nil
	}
	if name == "len_min" {
		return formatInt(minLen), nil
	}
	return formatInt(maxLen), nil
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

func formatDecimal(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if strings.Contains(s, ".") {
		for strings.HasSuffix(s, "0") {
			s = strings.TrimSuffix(s, "0")
		}
		s = strings.TrimSuffix(s, ".")
	}
	if !strings.Contains(s, ".") {
		return s + ".00"
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts[1]) == 1 {
		return s + "0"
	}
	return s
}
