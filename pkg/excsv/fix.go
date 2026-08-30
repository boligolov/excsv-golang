package excsv

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Fix targets. Each names one class of derived metadata the repairer owns.
const (
	FixFormat   = "format"
	FixColumns  = "columns"
	FixAgg      = "agg"
	FixChecksum = "checksum"
	FixRows     = "rows"
	FixStamp    = "stamp"
)

// AllFixTargets is the default set, in execution order.
var AllFixTargets = []string{FixFormat, FixColumns, FixAgg, FixChecksum, FixRows, FixStamp}

// FixOptions configures the single repairer.
//
// Only selects which targets run (empty means all of AllFixTargets). Columns
// narrows the targets that are per-column, which today is agg. DryRun reports
// what would change and writes nothing. Now overrides the stamp timestamp.
type FixOptions struct {
	Only    []string
	Columns []string
	DryRun  bool
	Now     time.Time
}

// FixReport names the targets that actually changed something.
type FixReport struct {
	Changed []string `json:"changed"`
	DryRun  bool     `json:"dry_run"`
}

func (r FixReport) OK() bool { return len(r.Changed) == 0 }

func (r *FixReport) mark(target string) {
	for _, t := range r.Changed {
		if t == target {
			return
		}
	}
	r.Changed = append(r.Changed, target)
}

func (r *FixReport) merge(other FixReport) {
	for _, t := range other.Changed {
		r.mark(t)
	}
}

// ParseFixTargets validates a comma-separated --only list.
func ParseFixTargets(list string) ([]string, error) {
	list = strings.TrimSpace(list)
	if list == "" {
		return nil, nil
	}
	valid := map[string]bool{}
	for _, t := range AllFixTargets {
		valid[t] = true
	}
	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !valid[part] {
			return nil, fmt.Errorf("unknown fix target %q (want %s)", part, strings.Join(AllFixTargets, ", "))
		}
		out = append(out, part)
	}
	return out, nil
}

// Fix repairs derived metadata: formatting, inferred #column stubs, stored #%
// values, checksum=, rows= and the #@exported / #@tool stamp.
//
// It repairs only what is derived from the data. A cell that contradicts its
// declared type, a violated required=, a value outside enum= — those are
// reported by Validate and must be corrected at the source. Fix also never
// invents metadata: unlike the freeze command it replaces, it recomputes the
// aggregations a document already has and adds none.
func (doc *Document) Fix(opts FixOptions) (FixReport, error) {
	report := FixReport{DryRun: opts.DryRun}
	if doc == nil {
		return report, fmt.Errorf("nil document")
	}
	targets := opts.Only
	if len(targets) == 0 {
		targets = AllFixTargets
	}
	selected := map[string]bool{}
	for _, t := range targets {
		selected[t] = true
	}

	scope, err := doc.resolveColumnScope(opts.Columns)
	if err != nil {
		return report, err
	}

	work := doc
	if opts.DryRun {
		work, err = doc.clone()
		if err != nil {
			return report, err
		}
	}

	requested := len(opts.Only) > 0
	for _, target := range AllFixTargets {
		if !selected[target] {
			continue
		}
		changed, err := work.applyFixTarget(target, scope, opts.Now, requested)
		if err != nil {
			return report, err
		}
		if changed {
			report.mark(target)
		}
	}
	if err := work.SyncDerived(); err != nil {
		return report, err
	}
	return report, nil
}

func (doc *Document) applyFixTarget(target string, scope map[int]bool, now time.Time, requested bool) (bool, error) {
	switch target {
	case FixFormat:
		before, err := doc.SerializeCanonical()
		if err != nil {
			return false, err
		}
		if err := doc.Tidy(); err != nil {
			return false, err
		}
		after, err := doc.SerializeCanonical()
		if err != nil {
			return false, err
		}
		return !bytes.Equal(before, after), nil

	case FixColumns:
		if len(doc.Meta.Columns) > 0 {
			return false, nil
		}
		doc.InferColumns()
		return len(doc.Meta.Columns) > 0, nil

	case FixAgg:
		changed := false
		for i, a := range doc.Meta.Aggregations {
			want, err := ComputeAggregationValues(doc, a.Name)
			if err != nil {
				return changed, err
			}
			merged := mergeAggValues(a.Values, want, scope)
			if !equalStrings(a.Values, merged) {
				doc.Meta.Aggregations[i].Values = merged
				changed = true
			}
		}
		return changed, nil

	case FixChecksum:
		before, present := doc.Header.Fields["checksum"]
		// A default run repairs the checksum a document has; only an explicit
		// --only checksum creates one, so `convert --no-checksum` output is not
		// silently re-checksummed by the next fix.
		if !present && !requested {
			return false, nil
		}
		if err := doc.SetDataChecksum("sha256"); err != nil {
			return false, err
		}
		return before != doc.Header.Fields["checksum"], nil

	case FixRows:
		before := doc.Header.Fields["rows"]
		n := doc.RowCount()
		doc.Header.Fields["rows"] = strconv.Itoa(n)
		doc.Header.Rows = &n
		return before != doc.Header.Fields["rows"], nil

	case FixStamp:
		if now.IsZero() {
			now = time.Now().UTC()
		}
		meta := doc.MetaMap()
		stamp := now.UTC().Format(time.RFC3339)
		changed := meta["exported"] != stamp
		doc.SetFileMeta("exported", stamp)
		if _, ok := meta["tool"]; !ok {
			doc.SetFileMeta("tool", "excsv-cli")
			changed = true
		}
		return changed, nil
	}
	return false, nil
}

// mergeAggValues keeps out-of-scope positions untouched so `fix --column amount`
// only rewrites the column the user asked about.
func mergeAggValues(current, recomputed []string, scope map[int]bool) []string {
	if scope == nil {
		return recomputed
	}
	out := make([]string, len(recomputed))
	for i := range recomputed {
		if scope[i] {
			out[i] = recomputed[i]
			continue
		}
		if i < len(current) {
			out[i] = current[i]
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clone deep-copies everything Fix can mutate, so --dry-run cannot leak edits.
func (doc *Document) clone() (*Document, error) {
	out := *doc
	out.Header.Fields = map[string]string{}
	for k, v := range doc.Header.Fields {
		out.Header.Fields[k] = v
	}
	if doc.Header.Rows != nil {
		n := *doc.Header.Rows
		out.Header.Rows = &n
	}
	if doc.Header.Checksum != nil {
		cs := *doc.Header.Checksum
		out.Header.Checksum = &cs
	}
	out.Meta.FileMeta = append([]KV(nil), doc.Meta.FileMeta...)
	out.Meta.HumanComments = append([]string(nil), doc.Meta.HumanComments...)
	out.Meta.SQL = append([]SQLStatement(nil), doc.Meta.SQL...)
	out.Meta.Unknown = append([]UnknownMetaLine(nil), doc.Meta.Unknown...)
	out.Meta.Columns = make([]ColumnDef, len(doc.Meta.Columns))
	for i, col := range doc.Meta.Columns {
		attrs := map[string]string{}
		for k, v := range col.Attrs {
			attrs[k] = v
		}
		out.Meta.Columns[i] = ColumnDef{Attrs: attrs, Line: col.Line}
	}
	out.Meta.Aggregations = make([]Aggregation, len(doc.Meta.Aggregations))
	for i, a := range doc.Meta.Aggregations {
		out.Meta.Aggregations[i] = Aggregation{Name: a.Name, Values: append([]string(nil), a.Values...), Line: a.Line}
	}
	out.Data.HeaderRow = append([]string(nil), doc.Data.HeaderRow...)
	out.Data.Rows = make([][]string, len(doc.Data.Rows))
	for i, row := range doc.Data.Rows {
		out.Data.Rows[i] = append([]string(nil), row...)
	}
	return &out, nil
}
