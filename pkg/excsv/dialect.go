package excsv

import "strings"

var wellKnownDelims = map[string]rune{
	"comma":     ',',
	"tab":       '\t',
	"pipe":      '|',
	"semicolon": ';',
}

var wellKnownQuotes = map[string]rune{
	"double": '"',
	"single": '\'',
	"none":   0,
}

func resolveDelim(name string) (rune, error) {
	if name == "" {
		return ',', nil
	}
	if r, ok := wellKnownDelims[name]; ok {
		return r, nil
	}
	runes := []rune(name)
	if len(runes) != 1 {
		return 0, fail(ErrHeaderInvalidValue, 1, "delimiter must be one character")
	}
	return runes[0], nil
}

func resolveQuote(name string) (rune, bool, error) {
	if name == "" {
		return '"', true, nil
	}
	if name == "none" {
		return 0, false, nil
	}
	if r, ok := wellKnownQuotes[name]; ok {
		if r == 0 {
			return 0, false, nil
		}
		return r, true, nil
	}
	runes := []rune(name)
	if len(runes) != 1 {
		return 0, false, fail(ErrHeaderInvalidValue, 1, "quote must be one character or well-known name")
	}
	return runes[0], true, nil
}

func applyHeaderDefaults(h *Header) error {
	if h.Fields == nil {
		h.Fields = map[string]string{}
	}
	if !h.HasMagicLine {
		h.DelimName = "comma"
		h.QuoteName = "none"
		h.Encoding = "UTF-8"
		h.Schema = "excsv"
		h.HeaderRow = true
	} else {
		if v, ok := h.Fields["delim"]; ok {
			if v == "" {
				return fail(ErrHeaderInvalidValue, 1, "empty delimiter")
			}
			h.DelimName = v
		} else {
			h.DelimName = "comma"
		}
		if v, ok := h.Fields["quote"]; ok {
			h.QuoteName = v
		} else {
			h.QuoteName = "none"
		}
		if v, ok := h.Fields["encoding"]; ok {
			h.Encoding = v
		} else {
			h.Encoding = "UTF-8"
		}
		if v, ok := h.Fields["schema"]; ok {
			h.Schema = v
		} else {
			h.Schema = "excsv"
		}
		if v, ok := h.Fields["header"]; ok {
			if v != "0" && v != "1" {
				return fail(ErrHeaderInvalidValue, 1, "invalid header="+v)
			}
			h.HeaderRow = v != "0"
		} else {
			h.HeaderRow = true
		}
		if v, ok := h.Fields["null"]; ok {
			h.Null = v
		}
		if v, ok := h.Fields["sql-dialect"]; ok {
			h.SQLDialect = v
		}
		if v, ok := h.Fields["version"]; ok {
			h.Version = v
		}
	}

	d, err := resolveDelim(h.DelimName)
	if err != nil {
		return err
	}
	h.Delim = d

	q, enabled, err := resolveQuote(h.QuoteName)
	if err != nil {
		return err
	}
	h.Quote = q
	h.QuoteEnabled = enabled

	if h.DelimName == "" {
		return fail(ErrHeaderInvalidValue, 1, "empty delimiter")
	}

	if h.HasMagicLine {
		if h.Version == "" {
			return fail(ErrHeaderMissingVersion, 1, "missing version=")
		}
		if rows, ok := h.Fields["rows"]; ok {
			n, err := parseIntField(rows)
			if err != nil {
				return fail(ErrHeaderInvalidValue, 1, "invalid rows="+rows)
			}
			h.Rows = &n
		}
		if cs, ok := h.Fields["checksum"]; ok {
			alg, hex, err := parseChecksumField(cs)
			if err != nil {
				return fail(ErrHeaderInvalidValue, 1, err.Error())
			}
			h.Checksum = &Checksum{Algorithm: alg, Hex: hex}
		}
		if os, ok := h.Fields["original-size"]; ok {
			n, err := parseInt64Field(os)
			if err != nil {
				return fail(ErrHeaderInvalidValue, 1, "invalid original-size="+os)
			}
			h.OriginalSize = &n
		}
	}

	return nil
}

func parseIntField(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalid
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func parseInt64Field(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalid
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var errInvalid = fail(ErrHeaderInvalidValue, 0, "invalid")

func parseChecksumField(s string) (string, string, error) {
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", "", errInvalid
	}
	return s[:i], s[i+1:], nil
}

type Dialect struct {
	Delim        rune
	Quote        rune
	QuoteEnabled bool
}

func (h Header) Dialect() Dialect {
	return Dialect{Delim: h.Delim, Quote: h.Quote, QuoteEnabled: h.QuoteEnabled}
}
