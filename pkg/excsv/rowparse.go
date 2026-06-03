package excsv

import (
	"strconv"
	"strings"
)

func firstFieldHashError(line string, fields []string, d Dialect, lineNo int) error {
	if len(fields) == 0 {
		return nil
	}
	if !d.QuoteEnabled && strings.HasPrefix(fields[0], "#") && !isFirstFieldQuoted(line, d) {
		return fail(ErrFirstFieldHashUnquoted, lineNo, "first field starts with # unquoted")
	}
	return nil
}

func rowArityError(fields []string, lineNo int, d Dialect, expectedCols int, quoteName string) error {
	if expectedCols <= 0 || len(fields) == expectedCols {
		return nil
	}
	if !d.QuoteEnabled && len(fields) > expectedCols && quoteName == "none" {
		return fail(ErrQuoteNoneDelimiterInValue, lineNo, "delimiter in unquoted value")
	}
	return fail(ErrDataRowArityMismatch, lineNo, fmtRowArity(len(fields), expectedCols))
}

// parseCSVRow parses one data line; when strict is false, pads/truncates to expectedCols with warnings.
func parseCSVRow(line string, lineNo int, d Dialect, expectedCols int, quoteName string, strict bool) ([]string, []Issue, error) {
	fields, err := splitCSVFields(line, d)
	if err != nil {
		return nil, nil, wrapLineErr(err, lineNo)
	}
	if err := firstFieldHashError(line, fields, d, lineNo); err != nil {
		return nil, nil, err
	}
	if expectedCols == 0 {
		return fields, nil, nil
	}
	if len(fields) == expectedCols {
		return fields, nil, nil
	}
	if strict {
		return nil, nil, rowArityError(fields, lineNo, d, expectedCols, quoteName)
	}
	var warnings []Issue
	if len(fields) < expectedCols {
		warnings = append(warnings, newIssue(ErrDataRowArityMismatch, lineNo,
			"padded row from "+strconv.Itoa(len(fields))+" to "+strconv.Itoa(expectedCols)+" fields"))
		for len(fields) < expectedCols {
			fields = append(fields, "")
		}
	} else {
		warnings = append(warnings, newIssue(ErrDataRowArityMismatch, lineNo,
			"truncated row from "+strconv.Itoa(len(fields))+" to "+strconv.Itoa(expectedCols)+" fields"))
		fields = fields[:expectedCols]
	}
	return fields, warnings, nil
}
