package excsv

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ValidateOptions selects how deep the single reporter reads.
//
// The default level is schema-only: header and meta lines, no data scan.
// WithData adds every cell plus the derived fields (rows=, checksum=, #%).
// Columns narrows the data scan and implies WithData.
type ValidateOptions struct {
	WithData bool
	Columns  []string
}

// Finding is one conformance problem plus the fix target that repairs it, when
// one exists. Repair is empty for findings fix cannot address.
type Finding struct {
	Issue
	Repair string `json:"repair,omitempty"`
}

// ValidateReport is the full result of a validate run. Every check runs to
// completion, so Findings is never truncated at the first problem.
type ValidateReport struct {
	Findings []Finding `json:"findings"`
	Table    string    `json:"table,omitempty"`
	WithData bool      `json:"with_data"`
}

func (r ValidateReport) OK() bool { return len(r.Findings) == 0 }

// RepairCommand returns the `fix --only …` invocation covering the repairable
// findings, or "" when nothing in the report is repairable.
func (r ValidateReport) RepairCommand() string {
	seen := map[string]bool{}
	var targets []string
	for _, f := range r.Findings {
		if f.Repair == "" || seen[f.Repair] {
			continue
		}
		seen[f.Repair] = true
		targets = append(targets, f.Repair)
	}
	if len(targets) == 0 {
		return ""
	}
	sort.Strings(targets)
	return "fix --only " + strings.Join(targets, ",")
}

var knownColumnTypes = map[string]bool{
	"string": true, "int": true, "long": true, "float": true, "double": true,
	"decimal": true, "boolean": true, "date": true, "time": true,
	"datetime": true, "uuid": true, "binary": true,
}

// IsDataLevelWarning reports whether a parse-time warning belongs on a
// --with-data validate pass rather than the default schema-only pass.
func IsDataLevelWarning(kind ErrorKind) bool {
	switch kind {
	case ErrRowsMismatch, ErrChecksumMismatch, ErrSidecarChecksumMismatch, ErrDefaultWithNulls:
		return true
	default:
		return false
	}
}

// Validate runs every conformance check the requested level covers and returns
// the complete list of findings. It never writes to the document.
func (doc *Document) Validate(opts ValidateOptions) ValidateReport {
	report := ValidateReport{WithData: opts.WithData || len(opts.Columns) > 0}
	if doc == nil {
		return report
	}
	add := func(iss Issue, repair string) {
		report.Findings = append(report.Findings, Finding{Issue: iss, Repair: repair})
	}

	for _, iss := range doc.checkDeclarations() {
		add(iss, "")
	}
	for _, u := range doc.Meta.Unknown {
		add(newIssue(ErrUnknownMetaLine, u.Line,
			"unrecognized meta line carried through verbatim: "+u.Text), "")
	}
	if !report.WithData {
		return report
	}

	scope, err := doc.resolveColumnScope(opts.Columns)
	if err != nil {
		add(newIssue(ErrColumnValueInvalid, 0, err.Error()), "")
		return report
	}
	for _, iss := range doc.checkSchemaScoped(scope) {
		add(iss, "")
	}
	for _, iss := range doc.checkUnique(scope) {
		add(iss, "")
	}
	if doc.Header.Rows != nil && doc.RowCount() != *doc.Header.Rows {
		add(newIssue(ErrRowsMismatch, 1,
			fmt.Sprintf("rows=%d but the data section has %d rows", *doc.Header.Rows, doc.RowCount())), "rows")
	}
	if doc.Header.Checksum != nil {
		if err := verifyChecksum(doc.SerializeDataSection(), doc.Header.Checksum); err != nil {
			kind := ErrChecksumMismatch
			if pe, ok := err.(*ParseError); ok && pe.Issue.Kind == ErrChecksumUnknownAlgorithm {
				kind = ErrChecksumUnknownAlgorithm
			}
			add(newIssue(kind, 1, err.Error()), "checksum")
			if kind == ErrChecksumMismatch {
				// The data changed underneath a checksum that hasn't been
				// refreshed; any materialized computed column's cached
				// values may equally be stale.
				for _, col := range doc.Meta.Columns {
					if col.Attrs["formula"] != "" && col.Attrs["materialized"] == "1" {
						add(newIssue(ErrComputedStale, col.Line,
							"column "+col.Attrs["name"]+": materialized value may not reflect current formula output"), "")
					}
				}
			}
		}
	}
	for _, iss := range doc.checkAggregations() {
		add(iss, "agg")
	}
	for _, iss := range doc.checkComputedMaterialization() {
		add(iss, "")
	}
	return report
}

// checkDeclarations covers everything readable without touching the data
// section: the header line and the #column / #$ declarations themselves.
func (doc *Document) checkDeclarations() []Issue {
	var issues []Issue
	if doc.Header.HasMagicLine && doc.Header.Version != "" && !implementedVersions[doc.Header.Version] {
		issues = append(issues, newIssue(ErrUnknownVersion, 1, "unknown version="+doc.Header.Version))
	}
	if ref := headerReference(doc.Header); ref != "" && doc.Source.Profile == ProfileInline {
		issues = append(issues, newIssue(ErrReferenceOnInline, 1, "inline document must not set reference="))
	}
	if cs, ok := doc.Header.Fields["checksum"]; ok && cs != "" {
		if _, kind := classifyChecksumField(cs); kind != "" {
			issues = append(issues, newIssue(kind, 1, "checksum="+cs))
		}
	}

	seenName := map[string]int{}
	seenIndex := map[string]int{}
	for _, col := range doc.Meta.Columns {
		issues = append(issues, checkColumnDeclaration(col)...)
		if name := col.Attrs["name"]; name != "" {
			if prev, ok := seenName[name]; ok {
				issues = append(issues, newIssue(ErrDuplicateColumn, col.Line,
					fmt.Sprintf("duplicate #column name=%s (also at line %d)", name, prev)))
			} else {
				seenName[name] = col.Line
			}
		}
		if idx := col.Attrs["index"]; idx != "" {
			if prev, ok := seenIndex[idx]; ok {
				issues = append(issues, newIssue(ErrDuplicateColumn, col.Line,
					fmt.Sprintf("colliding #column index=%s (also at line %d)", idx, prev)))
			} else {
				seenIndex[idx] = col.Line
			}
		}
	}
	for _, stmt := range doc.Meta.SQL {
		if !knownSQLVerbs[stmt.Verb] {
			issues = append(issues, newIssue(ErrSQLUnknownVerb, stmt.Line, "unknown #$ verb "+stmt.Verb))
		}
		if stmt.Qualified && stmt.Dialect != "" && !isKnownDialect(stmt.Dialect) {
			issues = append(issues, newIssue(ErrSQLUnknownDialect, stmt.Line, "unknown SQL dialect "+stmt.Dialect))
		}
	}
	issues = append(issues, doc.checkComputedColumns()...)
	return issues
}

// checkComputedColumns validates #column formula=/materialized= against the
// rest of the schema: unknown/chained references, unparseable formulas, and
// a materialized= flag that disagrees with whether the column actually has
// physical data. index=/header=0 combinations are already rejected at parse
// time (validateColumns), since those are structural, not merely advisory.
func (doc *Document) checkComputedColumns() []Issue {
	var issues []Issue
	byName := map[string]ColumnDef{}
	for _, col := range doc.Meta.Columns {
		if name := col.Attrs["name"]; name != "" {
			byName[name] = col
		}
	}
	for _, col := range doc.Meta.Columns {
		expr := col.Attrs["formula"]
		if expr == "" {
			continue
		}
		name := col.Attrs["name"]
		label := "column " + name + ": "

		if col.Attrs["default"] != "" || col.Attrs["required"] != "" {
			issues = append(issues, newIssue(ErrComputedDefaultIgnored, col.Line,
				label+"default=/required= is ignored on a computed column"))
		}

		node, err := parseFormula(expr)
		if err != nil {
			issues = append(issues, newIssue(ErrFormulaParseError, col.Line, label+err.Error()))
			continue
		}
		for _, ref := range formulaReferencedNames(node) {
			target, ok := byName[ref]
			if !ok {
				issues = append(issues, newIssue(ErrFormulaUnknownReference, col.Line,
					label+"formula references unknown column "+ref))
				continue
			}
			if target.Attrs["formula"] != "" {
				issues = append(issues, newIssue(ErrFormulaReferencesComputed, col.Line,
					label+"formula references computed column "+ref+" (chaining is not supported)"))
			}
		}

	}
	return issues
}

// checkComputedMaterialization compares materialized= against whether the
// column actually has physical data. Unlike checkComputedColumns, this needs
// the data section to have actually been read — a metadata-only read (a
// zip/pack "peek" that only parses the comment/manifest, the default
// schema-only validate pass on a zip) sees an empty Data section regardless
// of what the real data looks like, so this only runs under --with-data,
// where the data section is guaranteed loaded.
func (doc *Document) checkComputedMaterialization() []Issue {
	var issues []Issue
	for _, col := range doc.Meta.Columns {
		if col.Attrs["formula"] == "" {
			continue
		}
		name := col.Attrs["name"]
		materialized := col.Attrs["materialized"] == "1"
		hasData := columnHasPhysicalData(doc, name)
		if materialized != hasData {
			issues = append(issues, newIssue(ErrComputedMaterializedMismatch, col.Line,
				"column "+name+": materialized= disagrees with whether physical data is present"))
		}
	}
	return issues
}

// columnHasPhysicalData reports whether a computed column currently has a
// header cell (and therefore a field in every row) — i.e. whether it has
// actually been materialized, independent of what materialized= claims.
func columnHasPhysicalData(doc *Document, name string) bool {
	if name == "" || !doc.Data.HasHeaderRow {
		return false
	}
	for _, cell := range doc.Data.HeaderRow {
		if cell == name {
			return true
		}
	}
	return false
}

func checkColumnDeclaration(col ColumnDef) []Issue {
	var issues []Issue
	name := col.Attrs["name"]
	if name == "" {
		name = col.Attrs["index"]
	}
	label := "column " + name + ": "

	for k := range col.Attrs {
		if !strings.HasPrefix(k, "x-") && !knownColumnAttrs[k] {
			issues = append(issues, newIssue(ErrColumnUnknownAttribute, col.Line, label+"unknown attribute "+k))
		}
	}
	ct := strings.ToLower(strings.TrimSpace(col.Attrs["type"]))
	if ct != "" && !knownColumnTypes[ct] {
		issues = append(issues, newIssue(ErrColumnUnknownType, col.Line, label+"unknown type="+ct))
	}
	if pat := col.Attrs["pattern"]; pat != "" {
		if _, err := regexp.Compile(pat); err != nil {
			issues = append(issues, newIssue(ErrColumnMalformedAttribute, col.Line, label+"pattern does not compile: "+err.Error()))
		}
	}
	minV, hasMin := col.Attrs["min"]
	maxV, hasMax := col.Attrs["max"]
	if hasMin && hasMax && compareBoundValues(ct, minV, maxV) > 0 {
		issues = append(issues, newIssue(ErrColumnMalformedAttribute, col.Line, label+"min= is greater than max="))
	}
	lenMin, okMin := parseAttrInt(col.Attrs["len_min"])
	lenMax, okMax := parseAttrInt(col.Attrs["len_max"])
	if okMin && okMax && lenMin > lenMax {
		issues = append(issues, newIssue(ErrColumnMalformedAttribute, col.Line, label+"len_min= is greater than len_max="))
	}
	if v, ok := col.Attrs["default"]; ok && v != "" {
		if msg := checkTypedValue(col, v); msg != "" {
			issues = append(issues, newIssue(ErrColumnMalformedAttribute, col.Line, label+"default= is "+msg))
		}
	}
	if v, ok := col.Attrs["enum"]; ok && v != "" {
		for _, part := range strings.Split(v, "|") {
			if msg := checkTypedValue(col, part); msg != "" {
				issues = append(issues, newIssue(ErrColumnMalformedAttribute, col.Line,
					label+"enum value "+part+" is "+msg))
			}
		}
	}
	return issues
}

func parseAttrInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}

func compareBoundValues(columnType, a, b string) int {
	switch columnType {
	case "int", "long", "float", "double", "decimal", "number":
		return compareNumeric(a, b)
	default:
		return strings.Compare(a, b)
	}
}

// resolveColumnScope turns column references into physical indexes. A nil
// result means "every column".
func (doc *Document) resolveColumnScope(refs []string) (map[int]bool, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	scope := map[int]bool{}
	for _, ref := range refs {
		idx, err := doc.ColumnIndex(ref)
		if err != nil {
			return nil, err
		}
		scope[idx] = true
	}
	return scope, nil
}

func (doc *Document) checkSchemaScoped(scope map[int]bool) []Issue {
	if len(doc.Meta.Columns) == 0 {
		return nil
	}
	var issues []Issue
	width := doc.columnWidth()
	for col := 0; col < width; col++ {
		if scope != nil && !scope[col] {
			continue
		}
		def, ok := doc.columnDefAt(col)
		if !ok {
			continue
		}
		issues = append(issues, checkColumnValues(doc, col, def)...)
	}
	return issues
}

// checkUnique enforces unique=1, which the schema pass treats as a hint.
func (doc *Document) checkUnique(scope map[int]bool) []Issue {
	var issues []Issue
	width := doc.columnWidth()
	for col := 0; col < width; col++ {
		if scope != nil && !scope[col] {
			continue
		}
		def, ok := doc.columnDefAt(col)
		if !ok || def.Attrs["unique"] != "1" {
			continue
		}
		name := def.Attrs["name"]
		if name == "" {
			name = strconv.Itoa(col)
		}
		seen := map[string]int{}
		for r, row := range dataRows(doc) {
			v := cellAt(doc, row, col)
			if isNullCell(doc, v) {
				continue
			}
			line := r + 1
			if doc.Data.HasHeaderRow {
				line++
			}
			if prev, dup := seen[v]; dup {
				issues = append(issues, newIssue(ErrColumnNotUnique, line,
					fmt.Sprintf("column %s: unique=1 but value %q repeats line %d", name, v, prev)))
				continue
			}
			seen[v] = line
		}
	}
	return issues
}

// checkAggregations compares every stored #% line against a recomputation.
func (doc *Document) checkAggregations() []Issue {
	var issues []Issue
	for _, agg := range doc.Meta.Aggregations {
		want, err := ComputeAggregationValues(doc, agg.Name)
		if err != nil {
			continue
		}
		if len(want) != len(agg.Values) {
			issues = append(issues, newIssue(ErrAggStale, agg.Line,
				fmt.Sprintf("#%%%s has %d values, recomputation yields %d", agg.Name, len(agg.Values), len(want))))
			continue
		}
		for i := range want {
			if want[i] != agg.Values[i] {
				issues = append(issues, newIssue(ErrAggStale, agg.Line,
					fmt.Sprintf("#%%%s column %d is %q, recomputation yields %q", agg.Name, i, agg.Values[i], want[i])))
			}
		}
	}
	return issues
}
