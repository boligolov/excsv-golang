package gencsv

import (
	"fmt"
	"strings"
)

// ColumnType is a dummy-data kind for one output column.
type ColumnType int

const (
	TypeInt ColumnType = iota
	TypeString
	TypeDate
	TypeFloat
	TypeBoolean
	TypeNull
)

// ColumnSpec describes one generated column.
type ColumnSpec struct {
	Name  string
	Type  ColumnType
	Nulls bool // emit empty cells on some rows
}

// ParseColumnSpec parses "name,type" or "name,type,nulls".
func ParseColumnSpec(raw string) (ColumnSpec, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return ColumnSpec{}, fmt.Errorf("column %q: expected name,type[,nulls]", raw)
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return ColumnSpec{}, fmt.Errorf("column %q: empty name", raw)
	}
	typ, err := parseColumnType(strings.TrimSpace(parts[1]))
	if err != nil {
		return ColumnSpec{}, fmt.Errorf("column %q: %w", raw, err)
	}
	nulls := false
	if len(parts) >= 3 {
		nulls = parseNullsFlag(strings.TrimSpace(parts[2]))
	}
	if typ == TypeNull {
		nulls = false
	}
	return ColumnSpec{Name: name, Type: typ, Nulls: nulls}, nil
}

func parseColumnType(s string) (ColumnType, error) {
	switch strings.ToLower(s) {
	case "int", "integer":
		return TypeInt, nil
	case "string", "str", "text":
		return TypeString, nil
	case "date":
		return TypeDate, nil
	case "float", "double", "number":
		return TypeFloat, nil
	case "bool", "boolean":
		return TypeBoolean, nil
	case "null":
		return TypeNull, nil
	default:
		return 0, fmt.Errorf("unknown type %q (int, string, date, float, boolean, null)", s)
	}
}

func parseNullsFlag(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(s) {
	case "0", "false", "no", "off", "none":
		return false
	default:
		return true
	}
}
