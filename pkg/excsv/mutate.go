package excsv

import "strings"

// SetFileMeta sets or replaces a #@ metadata entry (last wins on duplicate keys).
func (doc *Document) SetFileMeta(key, value string) {
	doc.Meta.FileMeta = upsertKV(doc.Meta.FileMeta, key, value)
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
