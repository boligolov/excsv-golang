package excsv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type SortKey struct {
	Index int
	Desc  bool
}

// SyncDerived updates rows=, checksum=, and standard #% values after a data rewrite.
func (doc *Document) SyncDerived() error {
	n := doc.RowCount()
	doc.Header.Fields["rows"] = strconv.Itoa(n)
	doc.Header.Rows = &n
	if doc.Header.Checksum != nil {
		alg := doc.Header.Checksum.Algorithm
		if alg == "" {
			alg = "sha256"
		}
		if err := doc.SetDataChecksum(alg); err != nil {
			return err
		}
	}
	for i, a := range doc.Meta.Aggregations {
		if !standardAggregations[a.Name] {
			continue
		}
		vals, err := ComputeAggregationValues(doc, a.Name)
		if err != nil {
			return err
		}
		doc.Meta.Aggregations[i].Values = vals
	}
	return nil
}

// AppendRows appends data rows. In strict mode, arity must match the document width.
func (doc *Document) AppendRows(rows [][]string, strict bool) error {
	width := doc.columnWidth()
	out := make([][]string, 0, len(rows))
	for i, row := range rows {
		if width == 0 && len(row) > 0 {
			width = len(row)
		}
		if width > 0 && len(row) != width {
			if strict {
				return fail(ErrDataRowArityMismatch, i+1, fmtRowArity(len(row), width))
			}
			row = padOrTrim(row, width)
		}
		copied := append([]string(nil), row...)
		out = append(out, copied)
	}
	doc.Data.Rows = append(doc.Data.Rows, out...)
	return doc.SyncDerived()
}

func padOrTrim(row []string, width int) []string {
	if len(row) == width {
		return row
	}
	if len(row) > width {
		return row[:width]
	}
	out := make([]string, width)
	copy(out, row)
	return out
}

// SortRows sorts body rows by the given keys (stable). Type-aware when #column type= is set.
func (doc *Document) SortRows(keys []SortKey) error {
	if len(keys) == 0 {
		return fmt.Errorf("at least one sort key is required")
	}
	width := doc.columnWidth()
	for _, k := range keys {
		if k.Index < 0 || (width > 0 && k.Index >= width) {
			return fmt.Errorf("column index out of range: %d", k.Index)
		}
	}
	sort.SliceStable(doc.Data.Rows, func(i, j int) bool {
		a, b := doc.Data.Rows[i], doc.Data.Rows[j]
		for _, key := range keys {
			cmp := compareCells(doc, key.Index, cellAt(doc, a, key.Index), cellAt(doc, b, key.Index))
			if cmp == 0 {
				continue
			}
			if key.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	return doc.SyncDerived()
}

func compareCells(doc *Document, col int, a, b string) int {
	aNull := isNullCell(doc, a)
	bNull := isNullCell(doc, b)
	if aNull && bNull {
		return 0
	}
	if aNull {
		return 1
	}
	if bNull {
		return -1
	}
	ct := columnTypeAt(doc, col)
	switch ct {
	case "int", "long", "float", "double", "decimal", "number":
		af, aErr := strconv.ParseFloat(strings.TrimSpace(a), 64)
		bf, bErr := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if aErr == nil && bErr == nil {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	case "date", "time", "datetime":
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
		return 0
	case "boolean":
		return boolOrder(a) - boolOrder(b)
	}
	return strings.Compare(a, b)
}

func boolOrder(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false":
		return 0
	default:
		return 1
	}
}
