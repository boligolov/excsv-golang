package excsv

func linesFromRecords(records []record) ([]string, []int) {
	records = trimTrailingEmptyRecords(records)
	lines := make([]string, len(records))
	nums := make([]int, len(records))
	for i, r := range records {
		lines[i] = r.text
		nums[i] = r.num
	}
	return lines, nums
}

// trimTrailingEmptyRecords drops a final blank line used only for row logic (checksum may still use full records).
func trimTrailingEmptyRecords(records []record) []record {
	for len(records) > 0 && records[len(records)-1].text == "" && len(records) > 1 {
		records = records[:len(records)-1]
	}
	return records
}

// buildDataSection parses a delimited data block from pre-split records.
// sectionStart is the index in records for checksum extraction (usually 0 or data section start).
func buildDataSection(records []record, full string, d Dialect, hasHeader bool, quoteName string, schemaColCount int, sectionStart int, strict bool) (DataSection, string, int, []Issue, error) {
	records = trimTrailingEmptyRecords(records)
	ds := DataSection{}
	var warnings []Issue
	rowStart := 0
	colCount := schemaColCount

	if hasHeader && len(records) > 0 && records[0].text != "" {
		fields, err := splitCSVFields(records[0].text, d)
		if err != nil {
			return ds, "", 0, nil, wrapLineErr(err, records[0].num)
		}
		if err := firstFieldHashError(records[0].text, fields, d, records[0].num); err != nil {
			return ds, "", 0, nil, err
		}
		ds.HasHeaderRow = true
		ds.HeaderRow = fields
		colCount = len(fields)
		rowStart = 1
	}

	expectedCols := colCount
	for i := rowStart; i < len(records); i++ {
		if records[i].text == "" {
			continue
		}
		var row []string
		var w []Issue
		var err error
		if strict {
			fields, err := splitCSVFields(records[i].text, d)
			if err != nil {
				return ds, "", 0, nil, wrapLineErr(err, records[i].num)
			}
			if err := firstFieldHashError(records[i].text, fields, d, records[i].num); err != nil {
				return ds, "", 0, nil, err
			}
			if expectedCols > 0 {
				if err := rowArityError(fields, records[i].num, d, expectedCols, quoteName); err != nil {
					return ds, "", 0, nil, err
				}
			} else if len(fields) > 0 {
				expectedCols = len(fields)
			}
			row = fields
		} else {
			row, w, err = parseCSVRow(records[i].text, records[i].num, d, expectedCols, quoteName, false)
			if err != nil {
				return ds, "", 0, nil, err
			}
			warnings = append(warnings, w...)
			if expectedCols == 0 && len(row) > 0 {
				expectedCols = len(row)
			}
		}
		ds.Rows = append(ds.Rows, row)
	}

	if !hasHeader && colCount == 0 && expectedCols > 0 {
		colCount = expectedCols
	}
	section := extractDataSection(full, records, sectionStart)
	return ds, section, colCount, warnings, nil
}
