package excsv

import (
	"fmt"
	"strings"
)

// SetFileMeta sets or replaces a #@ metadata entry (last wins on duplicate keys).
func (doc *Document) SetFileMeta(key, value string) {
	doc.Meta.FileMeta = upsertKV(doc.Meta.FileMeta, key, value)
}

// RemoveFileMeta removes a #@ entry. Returns false if the key was not present.
func (doc *Document) RemoveFileMeta(key string) bool {
	var out []KV
	found := false
	for _, kv := range doc.Meta.FileMeta {
		if kv.Key == key {
			found = true
			continue
		}
		out = append(out, kv)
	}
	doc.Meta.FileMeta = out
	return found
}

// SetSQL sets or replaces the payload for #$KEY (KEY is the raw key, e.g. ddl or ddl-mysql).
func (doc *Document) SetSQL(rawKey, payload string) error {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return fail(ErrSQLMissingColon, 0, "empty SQL key")
	}
	stmt, err := parseSQLLine("#$"+rawKey+": "+payload, 0)
	if err != nil {
		return err
	}
	found := false
	for i := range doc.Meta.SQL {
		if doc.Meta.SQL[i].RawKey == rawKey {
			doc.Meta.SQL[i] = *stmt
			found = true
		}
	}
	if !found {
		doc.Meta.SQL = append(doc.Meta.SQL, *stmt)
	}
	return nil
}

// RemoveSQL removes a #$ statement by raw key. Returns false if not found.
func (doc *Document) RemoveSQL(rawKey string) bool {
	var out []SQLStatement
	found := false
	for _, s := range doc.Meta.SQL {
		if s.RawKey == rawKey {
			found = true
			continue
		}
		out = append(out, s)
	}
	doc.Meta.SQL = out
	return found
}

// AggregationByName returns the aggregation with the given name, if any.
func (doc *Document) AggregationByName(name string) (Aggregation, bool) {
	for _, a := range doc.Meta.Aggregations {
		if a.Name == name {
			return a, true
		}
	}
	return Aggregation{}, false
}

// AddAggregation adds a computed #% line. If the name already exists, returns added=false without change.
func (doc *Document) AddAggregation(name string) (added bool, err error) {
	if _, ok := doc.AggregationByName(name); ok {
		return false, nil
	}
	vals, err := ComputeAggregationValues(doc, name)
	if err != nil {
		return false, err
	}
	doc.Meta.Aggregations = append(doc.Meta.Aggregations, Aggregation{Name: name, Values: vals})
	return true, nil
}

// UpdateAggregation recomputes and replaces an aggregation (or appends if missing).
func (doc *Document) UpdateAggregation(name string) error {
	vals, err := ComputeAggregationValues(doc, name)
	if err != nil {
		return err
	}
	for i := range doc.Meta.Aggregations {
		if doc.Meta.Aggregations[i].Name == name {
			doc.Meta.Aggregations[i].Values = vals
			return nil
		}
	}
	doc.Meta.Aggregations = append(doc.Meta.Aggregations, Aggregation{Name: name, Values: vals})
	return nil
}

// AddHumanComment appends a ## human comment line (adds ## prefix when missing).
func (doc *Document) AddHumanComment(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "##") {
		text = "## " + text
	}
	doc.Meta.HumanComments = append(doc.Meta.HumanComments, text)
}

// RemoveHumanComment removes the ## comment at index. Returns false if out of range.
func (doc *Document) RemoveHumanComment(index int) bool {
	if index < 0 || index >= len(doc.Meta.HumanComments) {
		return false
	}
	doc.Meta.HumanComments = append(doc.Meta.HumanComments[:index], doc.Meta.HumanComments[index+1:]...)
	return true
}

// MaterializeColumn writes a virtual computed column's formula output into
// the data as an ordinary trailing column (and header cell), and sets
// materialized=1. formula= is kept either way. Errors if name is not a
// currently-virtual formula column, or if the formula can't be evaluated.
func (doc *Document) MaterializeColumn(name string) error {
	idx, col, ok := doc.columnByName(name)
	if !ok {
		return fmt.Errorf("unknown column: %s", name)
	}
	expr := col.Attrs["formula"]
	if expr == "" {
		return fmt.Errorf("column %s has no formula=, nothing to materialize", name)
	}
	if col.Attrs["materialized"] == "1" {
		return fmt.Errorf("column %s is already materialized", name)
	}
	if !doc.Data.HasHeaderRow {
		return fail(ErrFormulaRequiresHeader, col.Line, "formula= requires header=1")
	}

	node, err := parseFormula(expr)
	if err != nil {
		return fail(ErrFormulaParseError, col.Line, err.Error())
	}
	refs := formulaReferencedNames(node)
	refIdx := make(map[string]int, len(refs))
	refType := make(map[string]string, len(refs))
	for _, ref := range refs {
		_, refDef, ok := doc.columnByName(ref)
		if !ok {
			return fail(ErrFormulaUnknownReference, col.Line, "formula references unknown column "+ref)
		}
		if refDef.Attrs["formula"] != "" {
			return fail(ErrFormulaReferencesComputed, col.Line,
				"formula references computed column "+ref+" (chaining is not supported)")
		}
		physIdx, err := doc.ColumnIndex(ref)
		if err != nil {
			return err
		}
		refIdx[ref] = physIdx
		refType[ref] = refDef.Attrs["type"]
	}

	results := make([]string, len(doc.Data.Rows))
	for r, row := range doc.Data.Rows {
		env := make(map[string]formulaValue, len(refs))
		for _, ref := range refs {
			raw := cellAt(doc, row, refIdx[ref])
			if isNullCell(doc, raw) {
				env[ref] = fvNullVal()
			} else {
				env[ref] = formulaValueFromCell(raw, refType[ref])
			}
		}
		v, err := node.eval(env)
		if err != nil {
			return fmt.Errorf("materialize %s: row %d: %w", name, r+1, err)
		}
		cell, err := formatFormulaResult(col.Attrs["type"], col.Attrs["format"], v, doc.Header.Null)
		if err != nil {
			return fmt.Errorf("materialize %s: row %d: %w", name, r+1, err)
		}
		results[r] = cell
	}

	for r := range doc.Data.Rows {
		doc.Data.Rows[r] = append(doc.Data.Rows[r], results[r])
	}
	headerCell := col.Attrs["name"]
	if t := col.Attrs["title"]; t != "" {
		headerCell = t
	}
	doc.Data.HeaderRow = append(doc.Data.HeaderRow, headerCell)

	// The #column line's place in the meta block SHOULD match its new
	// physical position (append-at-end), which is also what the corrected
	// declaration-order-as-physical-position logic (columnDefAt) relies on.
	doc.Meta.Columns[idx].Attrs["materialized"] = "1"
	moved := doc.Meta.Columns[idx]
	doc.Meta.Columns = append(doc.Meta.Columns[:idx], doc.Meta.Columns[idx+1:]...)
	doc.Meta.Columns = append(doc.Meta.Columns, moved)

	return doc.SyncDerived()
}

// DematerializeColumn removes a materialized computed column's physical
// data (the header cell and every row's field) and clears materialized=
// back to absent. formula= is kept. Errors if name is not currently a
// materialized formula column.
func (doc *Document) DematerializeColumn(name string) error {
	idx, col, ok := doc.columnByName(name)
	if !ok {
		return fmt.Errorf("unknown column: %s", name)
	}
	if col.Attrs["formula"] == "" {
		return fmt.Errorf("column %s has no formula=, nothing to dematerialize", name)
	}
	if col.Attrs["materialized"] != "1" {
		return fmt.Errorf("column %s is not materialized", name)
	}
	physIdx, err := doc.ColumnIndex(name)
	if err != nil {
		return err
	}
	if doc.Data.HasHeaderRow && physIdx < len(doc.Data.HeaderRow) {
		doc.Data.HeaderRow = append(doc.Data.HeaderRow[:physIdx], doc.Data.HeaderRow[physIdx+1:]...)
	}
	for r, row := range doc.Data.Rows {
		if physIdx < len(row) {
			doc.Data.Rows[r] = append(row[:physIdx], row[physIdx+1:]...)
		}
	}
	delete(doc.Meta.Columns[idx].Attrs, "materialized")
	return doc.SyncDerived()
}

// RemoveAggregation removes a #% line. Returns false if not found.
func (doc *Document) RemoveAggregation(name string) bool {
	var out []Aggregation
	found := false
	for _, a := range doc.Meta.Aggregations {
		if a.Name == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	doc.Meta.Aggregations = out
	return found
}
