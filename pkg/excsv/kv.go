package excsv

import (
	"strings"
	"unicode/utf8"
)

func parseKVLine(line string, lineNo int) (map[string]string, error) {
	s := strings.TrimSpace(line)
	if s == "" {
		return map[string]string{}, nil
	}
	pairs, err := splitHeaderPairs(s)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			return nil, fail(ErrColumnMalformedAttribute, lineNo, "missing = in attribute")
		}
		key := p[:eq]
		val := p[eq+1:]
		if key == "" {
			return nil, fail(ErrColumnMalformedAttribute, lineNo, "empty attribute key")
		}
		if val == "" && key != "null" {
			// required= without value is malformed for #column
			if key == "required" || key == "unique" {
				return nil, fail(ErrColumnMalformedAttribute, lineNo, "attribute "+key+" requires a value")
			}
		}
		out[key] = val
	}
	return out, nil
}

func parseHeaderLine(line string) (map[string]string, error) {
	if !strings.HasPrefix(line, "#!excsv") {
		return nil, fail(ErrHeaderMalformedMagic, 1, "expected #!excsv")
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "#!excsv"))
	if rest == "" {
		return map[string]string{}, nil
	}
	pairs, err := splitHeaderPairs(rest)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			return nil, fail(ErrHeaderMalformedKV, 1, "malformed key=value")
		}
		key := p[:eq]
		val := p[eq+1:]
		if key == "" {
			return nil, fail(ErrHeaderMalformedKV, 1, "empty key")
		}
		out[key] = val
	}
	return out, nil
}

func splitHeaderPairs(s string) ([]string, error) {
	var pairs []string
	i := 0
	for i < len(s) {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		if s[i] == '"' {
			i++
			var b strings.Builder
			closed := false
			for i < len(s) {
				if s[i] == '"' {
					if i+1 < len(s) && s[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				b.WriteByte(s[i])
				i++
			}
			if !closed {
				return nil, fail(ErrHeaderUnclosedQuote, 1, "unclosed quote in header")
			}
			segment := b.String()
			eq := strings.IndexByte(segment, '=')
			if eq < 0 {
				return nil, fail(ErrHeaderMalformedKV, 1, "malformed quoted token")
			}
			pairs = append(pairs, segment)
			continue
		}
		for i < len(s) && s[i] != ' ' {
			i++
		}
		token := s[start:i]
		if strings.Contains(token, " ") {
			return nil, fail(ErrHeaderMalformedKV, 1, "unquoted value contains spaces")
		}
		if eq := strings.IndexByte(token, '='); eq >= 0 {
			val := token[eq+1:]
			if strings.HasPrefix(val, `"`) && !strings.HasSuffix(val, `"`) {
				return nil, fail(ErrHeaderUnclosedQuote, 1, "unclosed quote in header")
			}
			if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) && len(val) >= 2 {
				// strip quotes for storage
				token = token[:eq+1] + val[1:len(val)-1]
			}
		}
		pairs = append(pairs, token)
	}
	return pairs, nil
}

func skipColonValue(line, prefix string) (key, value string, ok bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := line[len(prefix):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return "", "", false
	}
	key = rest[:colon]
	value = rest[colon+1:]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return key, value, true
}

func isReservedHeaderKey(key string) bool {
	switch key {
	case "layout", "mode", "section-size", "table-count":
		return true
	}
	return false
}

func isReservedMetaPrefix(line string) bool {
	return strings.HasPrefix(line, "#table") || strings.HasPrefix(line, "#fk")
}

func validUTF8(data []byte) bool {
	return utf8.Valid(data)
}

func encodingMismatch(data []byte, encoding string) bool {
	enc := strings.ToUpper(strings.TrimSpace(encoding))
	if enc == "" || enc == "UTF-8" || enc == "UTF8" {
		return false
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b > 127 {
			return true
		}
	}
	return false
}
