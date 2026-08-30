package excsv

func sidecarDelimWarnings(sidecarPath string, h Header) []Issue {
	if sidecarExtMismatch(sidecarPath, h) {
		return []Issue{newIssue(ErrExtsvDelimMismatch, 1, ".extsv sidecar should declare delim=tab")}
	}
	return nil
}
