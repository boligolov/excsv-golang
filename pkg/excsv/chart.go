package excsv

import (
	"encoding/json"
	"strings"
)

// Chart is one #chart suggestion (implementation/charts.md): an advisory,
// declarative hint that already-declared columns are meant to be viewed
// together as a specific presentation.
//
// The compact form (#chart type=bar name=… x=… y=…) keeps its attributes in
// Attrs, in file order. The escape-hatch form (#chart-<engine>: <payload>)
// sets Engine and keeps the raw payload in Payload.
type Chart struct {
	Engine  string
	Attrs   []KV
	Payload string
	Line    int
}

// ChartCountLiteral is the reserved row-count value usable wherever a channel
// expects a column name (e.g. y=count() for a histogram).
const ChartCountLiteral = "count()"

// ChartEngineVega is the only #chart-<engine>: suffix the spec defines.
const ChartEngineVega = "vega"

// chartChannels are the encoding channels; each takes a column name.
var chartChannels = []string{
	"x", "y", "x2", "y2", "color", "size", "theta", "radius", "shape",
	"opacity", "column", "row", "detail", "order", "tooltip", "text",
}

// chartModifiers are the non-channel attributes of a compact-form line.
var chartModifiers = []string{
	"type", "name", "title", "aggregate", "bin", "stack", "sort", "limit", "hole",
}

var (
	knownChartChannels  = stringSet(chartChannels)
	knownChartModifiers = stringSet(chartModifiers)
)

// chartRequiredChannels lists, per recognized mark, alternatives of channel
// sets: at least one alternative must be fully present.
var chartRequiredChannels = map[string][][]string{
	"bar":       {{"x", "y"}},
	"line":      {{"x", "y"}},
	"area":      {{"x", "y"}},
	"point":     {{"x"}, {"y"}},
	"circle":    {{"x", "y"}},
	"arc":       {{"theta"}},
	"rect":      {{"x", "y", "color"}},
	"tick":      {{"x"}, {"y"}},
	"boxplot":   {{"x"}, {"y"}},
	"text":      {{"x", "y", "text"}},
	"sparkline": {{"y"}},
}

// ChartMarks returns the recognized compact-form type= values.
func ChartMarks() []string {
	return []string{"bar", "line", "area", "point", "circle", "arc", "rect", "tick", "boxplot", "text", "sparkline"}
}

// Attr returns the value of a compact-form attribute.
func (c Chart) Attr(key string) (string, bool) {
	for _, kv := range c.Attrs {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return "", false
}

// Type returns type= (the mark) of a compact-form chart.
func (c Chart) Type() string {
	v, _ := c.Attr("type")
	return v
}

// Name returns name= of a compact-form chart.
func (c Chart) Name() string {
	v, _ := c.Attr("name")
	return v
}

// IsEscape reports whether the chart uses the #chart-<engine>: form.
func (c Chart) IsEscape() bool { return c.Engine != "" }

// ColumnRefs returns every column name the chart's channels reference, in
// attribute order. tooltip= may list several names separated by commas; the
// count() literal is not a column and is left out.
func (c Chart) ColumnRefs() []string {
	var out []string
	for _, kv := range c.Attrs {
		if !knownChartChannels[kv.Key] {
			continue
		}
		for _, ref := range chartChannelValues(kv) {
			if ref != ChartCountLiteral {
				out = append(out, ref)
			}
		}
	}
	return out
}

// Text renders the chart back to its meta-line form.
func (c Chart) Text() string {
	if c.IsEscape() {
		return "#chart-" + c.Engine + ": " + c.Payload
	}
	var b strings.Builder
	b.WriteString("#chart")
	for _, kv := range c.Attrs {
		b.WriteByte(' ')
		b.WriteString(formatColumnAttr(kv.Key, kv.Value))
	}
	return b.String()
}

// ChartByName returns the chart addressed by name=. When several lines share
// a name, the last one wins (chart_duplicate_name).
func (doc *Document) ChartByName(name string) (Chart, bool) {
	for i := len(doc.Meta.Charts) - 1; i >= 0; i-- {
		if c := doc.Meta.Charts[i]; !c.IsEscape() && c.Name() == name {
			return c, true
		}
	}
	return Chart{}, false
}

// AddChart appends a compact-form #chart built from attrs (in the given
// order). type= and name= are required; a chart whose name= is already used
// replaces the earlier one in place.
func (doc *Document) AddChart(attrs []KV) error {
	c := Chart{Attrs: append([]KV(nil), attrs...)}
	candidate := *doc
	candidate.Meta.Charts = []Chart{c}
	for _, iss := range candidate.checkCharts() {
		if isFailKind(iss.Kind) {
			return &ParseError{Issue: iss}
		}
	}
	for i, existing := range doc.Meta.Charts {
		if !existing.IsEscape() && existing.Name() == c.Name() {
			doc.Meta.Charts[i] = c
			return nil
		}
	}
	doc.Meta.Charts = append(doc.Meta.Charts, c)
	return nil
}

// RemoveChart removes every compact-form #chart with the given name=.
// Returns false if none matched.
func (doc *Document) RemoveChart(name string) bool {
	var out []Chart
	found := false
	for _, c := range doc.Meta.Charts {
		if !c.IsEscape() && c.Name() == name {
			found = true
			continue
		}
		out = append(out, c)
	}
	doc.Meta.Charts = out
	return found
}

// parseChartLine parses a line starting with "#chart". ok is false when the
// line only looks like a chart (e.g. "#chartx", or "#chart-vega" without a
// colon) and must be carried through as an unrecognized meta line.
func parseChartLine(line string, lineNo int) (c Chart, dropped []string, ok bool, err error) {
	rest := strings.TrimPrefix(line, "#chart")
	switch {
	case strings.HasPrefix(rest, "-"):
		colon := strings.IndexByte(rest, ':')
		if colon < 2 {
			return Chart{}, nil, false, nil
		}
		payload := rest[colon+1:]
		if len(payload) > 0 && payload[0] == ' ' {
			payload = payload[1:]
		}
		return Chart{Engine: rest[1:colon], Payload: payload, Line: lineNo}, nil, true, nil
	case rest == "" || rest[0] == ' ' || rest[0] == '\t':
		pairs, err := splitHeaderPairs(strings.TrimSpace(rest))
		if err != nil {
			return Chart{}, nil, false, wrapLineErr(err, lineNo)
		}
		c := Chart{Line: lineNo}
		for _, p := range pairs {
			eq := strings.IndexByte(p, '=')
			if eq <= 0 {
				dropped = append(dropped, p)
				continue
			}
			c.Attrs = upsertKV(c.Attrs, p[:eq], p[eq+1:])
		}
		return c, dropped, true, nil
	}
	return Chart{}, nil, false, nil
}

// checkCharts runs every #chart rule from implementation/charts.md against
// the document's current #column declarations. FAIL and WARN kinds are both
// returned; callers split them with isFailKind.
func (doc *Document) checkCharts() []Issue {
	if len(doc.Meta.Charts) == 0 {
		return nil
	}
	declared := map[string]bool{}
	for _, col := range doc.Meta.Columns {
		if name := col.Attrs["name"]; name != "" {
			declared[name] = true
		}
	}
	var issues []Issue
	seenName := map[string]bool{}
	for _, c := range doc.Meta.Charts {
		if c.IsEscape() {
			if c.Engine != ChartEngineVega {
				issues = append(issues, newIssue(ErrChartUnknownType, c.Line,
					"unrecognized #chart-"+c.Engine+": engine; ignored"))
				continue
			}
			if !json.Valid([]byte(c.Payload)) {
				issues = append(issues, newIssue(ErrChartVegaInvalidJSON, c.Line,
					"#chart-vega: payload is not valid JSON"))
			}
			continue
		}

		mark, hasType := c.Attr("type")
		if !hasType || mark == "" {
			issues = append(issues, newIssue(ErrChartMissingType, c.Line, "#chart lacks type="))
			continue
		}
		name, hasName := c.Attr("name")
		if !hasName || name == "" {
			issues = append(issues, newIssue(ErrChartMissingName, c.Line, "#chart lacks name="))
			continue
		}
		label := "chart " + name + ": "
		if seenName[name] {
			issues = append(issues, newIssue(ErrChartDuplicateName, c.Line,
				label+"duplicate name=; last-wins"))
		}
		seenName[name] = true

		for _, kv := range c.Attrs {
			if !knownChartChannels[kv.Key] && !knownChartModifiers[kv.Key] {
				issues = append(issues, newIssue(ErrChartUnknownChannel, c.Line,
					label+"unknown attribute "+kv.Key+"; ignored"))
			}
		}
		for _, ref := range c.ColumnRefs() {
			if !declared[ref] {
				issues = append(issues, newIssue(ErrChartUnknownColumn, c.Line,
					label+"channel references undeclared column "+ref))
			}
		}

		required, known := chartRequiredChannels[mark]
		if !known {
			issues = append(issues, newIssue(ErrChartUnknownType, c.Line,
				label+"unrecognized type="+mark+"; preserved"))
			continue
		}
		if !c.hasAnyChannelSet(required) {
			issues = append(issues, newIssue(ErrChartMissingRequiredChannel, c.Line,
				label+"type="+mark+" requires "+describeChannelSets(required)))
		}
	}
	return issues
}

func (c Chart) hasAnyChannelSet(sets [][]string) bool {
	for _, set := range sets {
		all := true
		for _, ch := range set {
			if v, ok := c.Attr(ch); !ok || v == "" {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func describeChannelSets(sets [][]string) string {
	alts := make([]string, len(sets))
	for i, set := range sets {
		alts[i] = strings.Join(set, "+")
	}
	return strings.Join(alts, " or ")
}

// chartChannelValues splits a channel value into column names: tooltip= takes
// a comma-separated list, every other channel exactly one name.
func chartChannelValues(kv KV) []string {
	if kv.Key != "tooltip" {
		return []string{kv.Value}
	}
	var out []string
	for _, part := range strings.Split(kv.Value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}
