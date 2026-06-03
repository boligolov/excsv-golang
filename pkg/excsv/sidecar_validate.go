package excsv

func validateDocAggregationArity(doc *Document, colCount int) error {
	expectedCols := effectiveColumnCount(doc, colCount)
	for _, agg := range doc.Meta.Aggregations {
		if expectedCols > 0 && len(agg.Values) != expectedCols {
			return fail(ErrAggArityMismatch, agg.Line, fmtAggArity(len(agg.Values), expectedCols))
		}
	}
	return nil
}

func validateSidecarSchema(doc *Document, colCount int) error {
	if err := validateColumns(doc, colCount); err != nil {
		return err
	}
	return validateDocAggregationArity(doc, colCount)
}

func sidecarDelimWarnings(sidecarPath string, h Header) []Issue {
	if sidecarExtMismatch(sidecarPath, h) {
		return []Issue{newIssue(ErrSidecarDelimExtMismatch, 1, ".extsv sidecar should declare delim=tab")}
	}
	return nil
}
