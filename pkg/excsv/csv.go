package excsv

import (
	"strings"
)

func splitCSVFields(line string, d Dialect) ([]string, error) {
	if !d.QuoteEnabled {
		return splitUnquoted(line, d.Delim)
	}
	return splitQuoted(line, d.Delim, d.Quote)
}

func splitUnquoted(line string, delim rune) ([]string, error) {
	if line == "" {
		return []string{""}, nil
	}
	d := string(delim)
	parts := strings.Split(line, d)
	for _, p := range parts {
		if strings.ContainsAny(p, "\r\n") {
			return nil, errQuotedNewline
		}
		if strings.Count(p, d) > 0 {
			return nil, errDelimiterInValue
		}
	}
	// strings.Split behavior: "a||b" with | gives ["a","","b"]
	return parts, nil
}

var errQuotedNewline = fail(ErrQuotedValueRawNewline, 0, "raw newline in quoted value")
var errDelimiterInValue = fail(ErrQuoteNoneDelimiterInValue, 0, "delimiter in unquoted value")

func splitQuoted(line string, delim, quote rune) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inQuote := false
	d := string(delim)
	q := string(quote)
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		rs := string(r)
		if inQuote {
			if rs == q {
				if i+1 < len(runes) && string(runes[i+1]) == q {
					cur.WriteRune(r)
					i++
					continue
				}
				inQuote = false
				continue
			}
			if r == '\n' || r == '\r' {
				return nil, errQuotedNewline
			}
			cur.WriteRune(r)
			continue
		}
		if rs == q {
			inQuote = true
			continue
		}
		if rs == d {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		if r == '\n' || r == '\r' {
			return nil, errQuotedNewline
		}
		cur.WriteRune(r)
	}
	if inQuote {
		if quote == '#' {
			fields = append(fields, cur.String())
			return fields, nil
		}
		return nil, errQuotedNewline
	}
	fields = append(fields, cur.String())
	return fields, nil
}

func joinCSVFields(fields []string, d Dialect) string {
	if !d.QuoteEnabled {
		return strings.Join(fields, string(d.Delim))
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = quoteField(f, d.Delim, d.Quote)
	}
	return strings.Join(parts, string(d.Delim))
}

func JoinCSVFields(fields []string, d Dialect) string {
	return joinCSVFields(fields, d)
}

func SplitCSVFields(line string, d Dialect) ([]string, error) {
	return splitCSVFields(line, d)
}

func quoteField(s string, delim, quote rune) string {
	q := string(quote)
	d := string(delim)
	needs := strings.ContainsAny(s, q+d+"\r\n") || strings.Contains(s, " ")
	if !needs {
		return s
	}
	return q + strings.ReplaceAll(s, q, q+q) + q
}

func parseAggPayload(payload string, d Dialect) ([]string, error) {
	return splitCSVFields(payload, d)
}
