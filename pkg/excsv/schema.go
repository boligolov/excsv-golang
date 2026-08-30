package excsv

import (
	"encoding/base64"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	decimalRE = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$`)
	uuidRE    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	dateRE    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRE    = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(?:\.\d+)?$`)
)

var datetimeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
}

// CheckSchema checks cell values against #column type= and constraints.
// unique= is a hint and is not enforced. Unknown types are skipped (forward compatible).
func (doc *Document) CheckSchema() []Issue {
	if len(doc.Meta.Columns) == 0 {
		return nil
	}
	var issues []Issue
	width := doc.columnWidth()
	for col := 0; col < width; col++ {
		def, ok := doc.columnDefAt(col)
		if !ok {
			continue
		}
		issues = append(issues, checkColumnValues(doc, col, def)...)
	}
	return issues
}

func checkColumnValues(doc *Document, col int, def ColumnDef) []Issue {
	var issues []Issue
	required := def.Attrs["required"] == "1"
	colName := def.Attrs["name"]
	if colName == "" {
		colName = strconv.Itoa(col)
	}
	sep := def.Attrs["separator"]
	pattern, patErr := compileColumnPattern(def)
	if patErr != nil {
		issues = append(issues, newIssue(ErrColumnMalformedAttribute, def.Line, patErr.Error()))
	}
	enumSet := splitEnum(def.Attrs["enum"])

	for r, row := range dataRows(doc) {
		raw := cellAt(doc, row, col)
		line := r + 1
		if doc.Data.HasHeaderRow {
			line++
		}
		if isNullCell(doc, raw) {
			if required {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line,
					"column "+colName+": required value is null"))
			}
			continue
		}
		parts := []string{raw}
		if sep != "" {
			parts = strings.Split(raw, sep)
		}
		for _, part := range parts {
			if msg := checkTypedValue(def, part); msg != "" {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line, "column "+colName+": "+msg))
				continue
			}
			if len(enumSet) > 0 && !enumAllows(def, enumSet, part) {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line,
					"column "+colName+": value not in enum"))
			}
			if pattern != nil && !pattern.MatchString(part) {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line,
					"column "+colName+": value does not match pattern"))
			}
			if msg := checkBounds(def, part); msg != "" {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line, "column "+colName+": "+msg))
			}
			if msg := checkLength(def, part); msg != "" {
				issues = append(issues, newIssue(ErrColumnValueInvalid, line, "column "+colName+": "+msg))
			}
		}
	}
	return issues
}

func compileColumnPattern(def ColumnDef) (*regexp.Regexp, error) {
	pat := def.Attrs["pattern"]
	if pat == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	return re, nil
}

func splitEnum(s string) map[string]bool {
	if s == "" {
		return nil
	}
	out := map[string]bool{}
	for _, p := range strings.Split(s, "|") {
		out[p] = true
	}
	return out
}

func enumAllows(def ColumnDef, set map[string]bool, v string) bool {
	if set[v] {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(def.Attrs["type"]))
	if isMeasureColumnType(ct) {
		for allowed := range set {
			if numericEqual(allowed, v) {
				return true
			}
		}
	}
	return false
}

func numericEqual(a, b string) bool {
	af, aErr := strconv.ParseFloat(strings.TrimSpace(a), 64)
	bf, bErr := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return aErr == nil && bErr == nil && af == bf
}

func checkTypedValue(def ColumnDef, v string) string {
	ct := strings.ToLower(strings.TrimSpace(def.Attrs["type"]))
	switch ct {
	case "", "string", "text":
		return ""
	case "int":
		if _, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32); err != nil {
			return "not an int"
		}
	case "long":
		if _, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err != nil {
			return "not a long"
		}
	case "float":
		if _, err := strconv.ParseFloat(strings.TrimSpace(v), 32); err != nil {
			return "not a float"
		}
	case "double", "number":
		if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err != nil {
			return "not a double"
		}
	case "decimal":
		if !decimalRE.MatchString(strings.TrimSpace(v)) {
			return "not a decimal"
		}
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "false", "1", "0":
		default:
			return "not a boolean"
		}
	case "date":
		if !dateRE.MatchString(v) {
			return "not a date (YYYY-MM-DD)"
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return "not a date (YYYY-MM-DD)"
		}
	case "time":
		if !timeRE.MatchString(v) {
			return "not a time (HH:MM:SS)"
		}
	case "datetime":
		if !parseDateTime(v) {
			return "not a datetime (ISO 8601)"
		}
	case "uuid":
		if !uuidRE.MatchString(v) {
			return "not a uuid"
		}
	case "binary":
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			return "not base64"
		}
	default:
		return ""
	}
	return ""
}

func parseDateTime(v string) bool {
	for _, layout := range datetimeLayouts {
		if _, err := time.Parse(layout, v); err == nil {
			return true
		}
	}
	return false
}

func checkBounds(def ColumnDef, v string) string {
	minV, hasMin := def.Attrs["min"]
	maxV, hasMax := def.Attrs["max"]
	if !hasMin && !hasMax {
		return ""
	}
	ct := strings.ToLower(strings.TrimSpace(def.Attrs["type"]))
	switch ct {
	case "date", "time", "datetime":
		if hasMin && v < minV {
			return "below min"
		}
		if hasMax && v > maxV {
			return "above max"
		}
		return ""
	case "int", "long", "float", "double", "decimal", "number":
		if hasMin && compareNumeric(v, minV) < 0 {
			return "below min"
		}
		if hasMax && compareNumeric(v, maxV) > 0 {
			return "above max"
		}
		return ""
	}
	if hasMin && v < minV {
		return "below min"
	}
	if hasMax && v > maxV {
		return "above max"
	}
	return ""
}

func compareNumeric(a, b string) int {
	ar, aOK := new(big.Rat).SetString(strings.TrimSpace(a))
	br, bOK := new(big.Rat).SetString(strings.TrimSpace(b))
	if aOK && bOK {
		return ar.Cmp(br)
	}
	return strings.Compare(a, b)
}

func checkLength(def ColumnDef, v string) string {
	n := utf8.RuneCountInString(v)
	if s, ok := def.Attrs["len_min"]; ok && s != "" {
		if min, err := strconv.Atoi(s); err == nil && n < min {
			return "shorter than len_min"
		}
	}
	if s, ok := def.Attrs["len_max"]; ok && s != "" {
		if max, err := strconv.Atoi(s); err == nil && n > max {
			return "longer than len_max"
		}
	}
	return ""
}
