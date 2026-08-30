package excsv

import "strings"

var knownSQLVerbs = map[string]bool{
	"ddl": true,
	"dql": true,
}

var dialectAliases = map[string]string{
	"postgresql": "postgres",
	"pg":         "postgres",
	"sqlserver":  "mssql",
}

var wellKnownDialects = []string{
	"clickhouse", "postgresql", "snowflake", "sqlserver", "bigquery",
	"mariadb", "postgres", "sqlite", "oracle", "mysql", "mssql",
	"duckdb", "ansi", "db2", "pg",
}

func parseSQLKey(raw string) (verb, dialect, version string, qualified bool) {
	dash := strings.IndexByte(raw, '-')
	if dash < 0 {
		return raw, "", "", false
	}
	verb = raw[:dash]
	suffix := strings.ToLower(raw[dash+1:])
	dialect, version = splitDialectVersion(suffix)
	return verb, dialect, version, true
}

func splitDialectVersion(suffix string) (dialect, version string) {
	best := ""
	for _, d := range wellKnownDialects {
		if suffix == d || strings.HasPrefix(suffix, d+"-") {
			if len(d) > len(best) {
				best = d
			}
		}
	}
	if best == "" {
		return suffix, ""
	}
	if suffix == best {
		return normalizeDialect(best), ""
	}
	return normalizeDialect(best), suffix[len(best)+1:]
}

func normalizeDialect(d string) string {
	d = strings.ToLower(d)
	if a, ok := dialectAliases[d]; ok {
		return a
	}
	return d
}

func isKnownDialect(d string) bool {
	if d == "" {
		return true
	}
	d = strings.ToLower(d)
	if _, ok := dialectAliases[d]; ok {
		return true
	}
	for _, k := range wellKnownDialects {
		if d == k || d == normalizeDialect(k) {
			return true
		}
	}
	return false
}

func effectiveDialect(stmt SQLStatement, headerSQLDialect string) string {
	if stmt.Qualified && stmt.Dialect != "" {
		if stmt.Version != "" {
			return stmt.Dialect + "-" + stmt.Version
		}
		return stmt.Dialect
	}
	if headerSQLDialect != "" {
		return headerSQLDialect
	}
	return "ansi"
}

func sqlPayloadUnclosed(payload string) bool {
	n := 0
	for _, c := range payload {
		switch c {
		case '(':
			n++
		case ')':
			if n > 0 {
				n--
			}
		}
	}
	return n > 0
}

func parseSQLLine(line string, lineNo int) (*SQLStatement, error) {
	if !strings.HasPrefix(line, "#$") {
		return nil, fail(ErrSQLMissingColon, lineNo, "expected #$ prefix")
	}
	rest := line[2:]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return nil, fail(ErrSQLMissingColon, lineNo, "missing colon in #$ line")
	}
	rawKey := rest[:colon]
	payload := rest[colon+1:]
	if len(payload) > 0 && payload[0] == ' ' {
		payload = payload[1:]
	}
	if strings.ContainsAny(payload, "\r\n") {
		return nil, fail(ErrSQLEmbeddedNewline, lineNo, "embedded newline in SQL payload")
	}
	verb, dialect, version, qualified := parseSQLKey(rawKey)
	return &SQLStatement{
		Verb:      verb,
		Dialect:   dialect,
		Version:   version,
		Payload:   payload,
		RawKey:    rawKey,
		Line:      lineNo,
		Qualified: qualified,
	}, nil
}
