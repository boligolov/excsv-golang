package excsv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var columnAttrDisplayOrder = []string{
	"name", "index", "title", "description", "type", "format", "unit",
	"role", "agg", "order", "separator", "enum", "pattern", "regexp_dialect",
	"min", "max", "len_min", "len_max", "unique", "required", "default",
}

// ColumnIndex resolves a header name, #column name=, or numeric index.
func (doc *Document) ColumnIndex(ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return -1, fmt.Errorf("empty column reference")
	}
	if n, err := strconv.Atoi(ref); err == nil {
		width := doc.columnWidth()
		if n < 0 || (width > 0 && n >= width) {
			return -1, fmt.Errorf("column index out of range: %d", n)
		}
		return n, nil
	}
	for i, cell := range doc.Data.HeaderRow {
		if cell == ref {
			return i, nil
		}
	}
	for i, col := range doc.Meta.Columns {
		if col.Attrs["name"] == ref {
			if v, ok := col.Attrs["index"]; ok {
				if n, err := strconv.Atoi(v); err == nil {
					return n, nil
				}
			}
			return i, nil
		}
	}
	return -1, fmt.Errorf("unknown column: %s", ref)
}

func (doc *Document) columnWidth() int {
	if doc.Data.HasHeaderRow && len(doc.Data.HeaderRow) > 0 {
		return len(doc.Data.HeaderRow)
	}
	if n := columnCountFromSchema(doc.Meta.Columns); n > 0 {
		return n
	}
	if len(doc.Data.Rows) > 0 {
		return len(doc.Data.Rows[0])
	}
	return 0
}

func (doc *Document) columnDefAt(col int) (ColumnDef, bool) {
	for i, c := range doc.Meta.Columns {
		idx := i
		if v, ok := c.Attrs["index"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				idx = n
			}
		}
		if idx == col {
			return c, true
		}
	}
	return ColumnDef{}, false
}

func (doc *Document) columnByName(name string) (int, ColumnDef, bool) {
	for i, col := range doc.Meta.Columns {
		if col.Attrs["name"] == name {
			return i, col, true
		}
	}
	return -1, ColumnDef{}, false
}

// UpsertColumn merges attributes into the named #column (creates it if missing).
func (doc *Document) UpsertColumn(name string, attrs map[string]string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("column name is required")
	}
	if strings.Contains(name, " ") {
		return fail(ErrColumnMalformedAttribute, 0, "column name must not contain spaces")
	}
	idx, col, ok := doc.columnByName(name)
	if !ok {
		merged := map[string]string{"name": name}
		for k, v := range attrs {
			if k == "" {
				continue
			}
			merged[k] = v
		}
		doc.Meta.Columns = append(doc.Meta.Columns, ColumnDef{Attrs: merged})
		return nil
	}
	if col.Attrs == nil {
		col.Attrs = map[string]string{}
	}
	col.Attrs["name"] = name
	for k, v := range attrs {
		if k == "" {
			continue
		}
		if v == "" {
			delete(col.Attrs, k)
			continue
		}
		col.Attrs[k] = v
	}
	doc.Meta.Columns[idx] = col
	return nil
}

// RemoveColumn removes a #column by name. Returns false if not found.
func (doc *Document) RemoveColumn(name string) bool {
	var out []ColumnDef
	found := false
	for _, col := range doc.Meta.Columns {
		if col.Attrs["name"] == name {
			found = true
			continue
		}
		out = append(out, col)
	}
	doc.Meta.Columns = out
	return found
}

func FormatColumnAttrs(attrs map[string]string) string {
	return formatColumnAttrParts(attrs, formatColumnAttr)
}

// FormatColumnInfoLine renders #column attributes for `info header` output:
// name: order_id, title: "Order ID", type: long
func FormatColumnInfoLine(attrs map[string]string) string {
	return formatColumnAttrParts(attrs, formatColumnInfoAttr)
}

func formatColumnAttrParts(attrs map[string]string, format func(k, v string) string) string {
	seen := map[string]bool{}
	var parts []string
	for _, k := range columnAttrDisplayOrder {
		v, ok := attrs[k]
		if !ok {
			continue
		}
		seen[k] = true
		parts = append(parts, format(k, v))
	}
	var extra []string
	for k := range attrs {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		parts = append(parts, format(k, attrs[k]))
	}
	return strings.Join(parts, ", ")
}

func formatColumnInfoAttr(k, v string) string {
	if strings.ContainsAny(v, " \t,") || strings.Contains(v, `"`) {
		return k + ": \"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
	}
	return k + ": " + v
}

func formatColumnAttr(k, v string) string {
	if strings.ContainsAny(v, " \t") {
		return k + "=\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
	}
	return k + "=" + v
}
