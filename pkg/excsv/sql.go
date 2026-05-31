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

func parseSQLKey(raw string) (verb, dialect, version string, qualified bool) {
	parts := strings.Split(raw, "-")
	verb = parts[0]
	if len(parts) == 1 {
		return verb, "", "", false
	}
	if len(parts) == 2 {
		return verb, normalizeDialect(parts[1]), "", true
	}
	return verb, normalizeDialect(parts[0]), strings.Join(parts[1:], "-"), true
}

func normalizeDialect(d string) string {
	d = strings.ToLower(d)
	if a, ok := dialectAliases[d]; ok {
		return a
	}
	return d
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
	if !knownSQLVerbs[verb] {
		return nil, fail(ErrSQLUnknownVerb, lineNo, "unknown SQL verb: "+verb)
	}
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
