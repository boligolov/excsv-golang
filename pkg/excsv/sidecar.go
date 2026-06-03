package excsv

import (
	"os"
	"path/filepath"
	"strings"
)

func headerReference(h Header) string {
	return strings.TrimSpace(h.Fields["reference"])
}

// IsSidecarMetaOnly reports a metadata-only sidecar (reference= set, no inline data section).
func IsSidecarMetaOnly(doc *Document) bool {
	if doc == nil || headerReference(doc.Header) == "" {
		return false
	}
	return len(doc.Data.Rows) == 0 && !doc.Data.HasHeaderRow
}

func isLikelySidecarReference(sidecarPath, ref string) bool {
	if sidecarPath == "" {
		return true
	}
	sideBase := filepath.Base(sidecarPath)
	sideStem := strings.TrimSuffix(sideBase, filepath.Ext(sideBase))
	refStem := strings.TrimSuffix(filepath.Base(ref), filepath.Ext(ref))
	return sideStem == refStem
}

func sidecarExtMismatch(path string, h Header) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".extsv" {
		return false
	}
	return h.DelimName != "tab"
}

func resolveReferencePath(sidecarPath, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fail(ErrSidecarMissingReference, 1, "missing reference=")
	}
	if filepath.IsAbs(ref) {
		return "", fail(ErrHeaderInvalidValue, 1, "reference= must be relative")
	}
	dir := filepath.Dir(sidecarPath)
	if dir == "." {
		dir = ""
	}
	return filepath.Join(dir, ref), nil
}

func parseDelimitedData(data []byte, h Header) (*DataSection, string, error) {
	data = stripUTF8BOM(data)
	records := splitRecords(data)
	d := h.Dialect()
	ds, section, _, _, err := buildDataSection(records, string(data), d, h.HeaderRow, h.Fields["quote"], 0, 0, true)
	if err != nil {
		return nil, "", err
	}
	return &ds, section, nil
}

func attachReferencedData(doc *Document, dataPath string, data []byte, opts ParseOptions) (*ParseResult, error) {
	refHeader := headerForDataPath(doc.Header, dataPath)
	ds, dataSection, err := parseDelimitedData(data, refHeader)
	if err != nil {
		return nil, err
	}
	doc.Data = *ds
	doc.Source.ReferencePath = dataPath
	doc.Source.Profile = ProfileSidecar

	colCount := len(doc.Data.HeaderRow)
	if !doc.Data.HasHeaderRow {
		colCount = columnCountFromSchema(doc.Meta.Columns)
	}
	if err := validateSidecarSchema(doc, colCount); err != nil {
		return nil, err
	}

	warnings := sidecarDelimWarnings(doc.Source.SidecarPath, doc.Header)

	if doc.Header.Rows != nil && doc.RowCount() != *doc.Header.Rows {
		return nil, fail(ErrHeaderInvalidValue, 1, "rows= does not match data row count")
	}

	if doc.Header.Checksum != nil {
		if err := verifyChecksum(dataSection, doc.Header.Checksum); err != nil {
			if pe, ok := err.(*ParseError); ok {
				pe.Issue.Kind = ErrSidecarChecksumMismatch
			}
			return nil, err
		}
	}

	return &ParseResult{Doc: doc, Warnings: warnings}, nil
}

func finishSidecarMeta(doc *Document, opts ParseOptions) (*ParseResult, error) {
	ref := headerReference(doc.Header)
	doc.Source.Reference = ref
	doc.Source.SidecarPath = opts.SourcePath
	doc.Source.Profile = ProfileSidecar

	colCount := columnCountFromSchema(doc.Meta.Columns)
	if err := validateSidecarSchema(doc, colCount); err != nil {
		return nil, err
	}

	warnings := sidecarDelimWarnings(opts.SourcePath, doc.Header)

	if opts.ExpectProfile == "sidecar" || opts.ExpectProfile == "sidecar_strict" {
		if ref == "" {
			return nil, fail(ErrSidecarMissingReference, 1, "sidecar missing reference=")
		}
	}

	if !opts.ResolveReference {
		if ref == "" && doc.Header.Rows != nil && *doc.Header.Rows != 0 {
			return nil, fail(ErrHeaderInvalidValue, 1, "rows= does not match data row count")
		}
		return &ParseResult{Doc: doc, Warnings: warnings}, nil
	}

	dataPath, err := resolveReferencePath(opts.SourcePath, ref)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			if opts.ExpectProfile == "sidecar_strict" || opts.Strict {
				return nil, fail(ErrSidecarReferenceNotFound, 1, "referenced file not found: "+ref)
			}
			warnings = append(warnings, newIssue(ErrSidecarReferenceNotFound, 1, "referenced file not found: "+ref))
			return &ParseResult{Doc: doc, Warnings: warnings}, nil
		}
		return nil, err
	}
	return attachReferencedData(doc, dataPath, data, opts)
}

func discoverSidecarForData(dataPath string) (string, []byte, bool, error) {
	ext := strings.ToLower(filepath.Ext(dataPath))
	if ext != ".csv" && ext != ".tsv" {
		return "", nil, false, nil
	}
	dir := filepath.Dir(dataPath)
	base := strings.TrimSuffix(filepath.Base(dataPath), ext)
	for _, sideExt := range []string{".excsv", ".ecsv", ".extsv"} {
		candidate := filepath.Join(dir, base+sideExt)
		data, err := os.ReadFile(candidate)
		if err == nil {
			return candidate, data, true, nil
		}
	}
	return "", nil, false, nil
}

func validateExpectProfile(doc *Document, profile string) error {
	if profile == "" {
		return nil
	}
	ref := headerReference(doc.Header)
	hasData := doc.RowCount() > 0 || doc.Data.HasHeaderRow
	switch profile {
	case "stub":
		if ref != "" || hasData {
			return fail(ErrHeaderInvalidValue, 1, "expected header-only stub profile")
		}
	case "sidecar", "sidecar_strict":
		if ref == "" {
			return fail(ErrSidecarMissingReference, 1, "sidecar missing reference=")
		}
	}
	return nil
}
