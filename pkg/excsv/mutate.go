package excsv

import "strings"

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
