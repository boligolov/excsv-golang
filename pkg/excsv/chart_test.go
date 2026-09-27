package excsv_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

const chartSchema = "#!excsv version=0.5 header=1 rows=2\n" +
	"#column name=category type=string role=dimension\n" +
	"#column name=amount type=decimal role=measure agg=sum\n"

const chartData = "category,amount\nwidgets,1\ngadgets,2\n"

func TestChartRoundTrip(t *testing.T) {
	src := chartSchema +
		"#chart type=bar name=top x=category y=amount sort=desc limit=10 title=\"Top categories\"\n" +
		"#chart type=bar name=hist bin=20 x=amount y=count() tooltip=category,amount\n" +
		"#chart-vega: {\"mark\":\"arc\"}\n" +
		chartData
	doc := parseDoc(t, src)
	if len(doc.Meta.Charts) != 3 {
		t.Fatalf("charts=%d", len(doc.Meta.Charts))
	}
	top, ok := doc.ChartByName("top")
	if !ok || top.Type() != "bar" {
		t.Fatalf("ChartByName(top) = %+v, %v", top, ok)
	}
	if title, _ := top.Attr("title"); title != "Top categories" {
		t.Fatalf("title=%q", title)
	}
	hist, _ := doc.ChartByName("hist")
	if got := strings.Join(hist.ColumnRefs(), ","); got != "amount,category,amount" {
		t.Fatalf("ColumnRefs=%s (count() must be left out)", got)
	}
	if !doc.Meta.Charts[2].IsEscape() || doc.Meta.Charts[2].Engine != excsv.ChartEngineVega {
		t.Fatalf("escape chart = %+v", doc.Meta.Charts[2])
	}

	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"#chart type=bar name=top x=category y=amount sort=desc limit=10 title=\"Top categories\"\n",
		"#chart type=bar name=hist bin=20 x=amount y=count() tooltip=category,amount\n",
		"#chart-vega: {\"mark\":\"arc\"}\n",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("serialized output lacks %q:\n%s", want, out)
		}
	}
	// #chart sits between #column and the data, per the recommended order.
	if strings.Index(string(out), "#chart") < strings.LastIndex(string(out), "#column") {
		t.Fatalf("#chart written before #column:\n%s", out)
	}
	again := parseDoc(t, string(out))
	if len(again.Meta.Charts) != 3 {
		t.Fatalf("re-parse charts=%d", len(again.Meta.Charts))
	}
}

func TestChartParseFailures(t *testing.T) {
	cases := []struct {
		line string
		want excsv.ErrorKind
	}{
		{"#chart name=c x=category y=amount", excsv.ErrChartMissingType},
		{"#chart type=bar x=category y=amount", excsv.ErrChartMissingName},
		{"#chart type=bar name=c x=category y=missing", excsv.ErrChartUnknownColumn},
		{"#chart type=bar name=c x=category", excsv.ErrChartMissingRequiredChannel},
		{"#chart type=arc name=c color=category", excsv.ErrChartMissingRequiredChannel},
		{"#chart type=rect name=c x=category y=amount", excsv.ErrChartMissingRequiredChannel},
		{"#chart type=bar name=c x=category y=amount tooltip=category,nope", excsv.ErrChartUnknownColumn},
		{"#chart-vega: {\"mark\":", excsv.ErrChartVegaInvalidJSON},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			_, err := excsv.ParseBytes([]byte(chartSchema+tc.line+"\n"+chartData), excsv.StrictOptions())
			var pe *excsv.ParseError
			if !errors.As(err, &pe) || pe.Issue.Kind != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestChartWarnings(t *testing.T) {
	cases := []struct {
		lines string
		want  excsv.ErrorKind
	}{
		{"#chart type=radar name=c x=category y=amount", excsv.ErrChartUnknownType},
		{"#chart-plotly: {}", excsv.ErrChartUnknownType},
		{"#chart type=bar name=c x=category y=amount glow=1", excsv.ErrChartUnknownChannel},
		{"#chart type=bar name=c x=category y=amount\n#chart type=arc name=c theta=amount", excsv.ErrChartDuplicateName},
		// One-channel marks: point/tick/boxplot accept x or y alone.
		{"#chart type=point name=c x=amount\n#chart type=tick name=d y=amount\n#chart type=boxplot name=e y=amount", ""},
	}
	for _, tc := range cases {
		t.Run(tc.lines, func(t *testing.T) {
			res, err := excsv.ParseBytes([]byte(chartSchema+tc.lines+"\n"+chartData), excsv.StrictOptions())
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(res.Warnings) != 0 {
					t.Fatalf("unexpected warnings %v", res.Warnings)
				}
				return
			}
			if len(res.Warnings) != 1 || res.Warnings[0].Kind != tc.want {
				t.Fatalf("warnings = %v, want one %s", res.Warnings, tc.want)
			}
		})
	}
}

func TestChartDuplicateNameLastWins(t *testing.T) {
	doc := parseDoc(t, chartSchema+
		"#chart type=bar name=c x=category y=amount\n#chart type=arc name=c theta=amount\n"+chartData)
	c, _ := doc.ChartByName("c")
	if c.Type() != "arc" {
		t.Fatalf("ChartByName picked %s, want the last line (arc)", c.Type())
	}
}

func TestChartOnPackManifestIgnored(t *testing.T) {
	src := "#!excsv version=0.5 layout=pack original-size=0\n#chart type=bar name=c x=a y=b\n"
	opts := excsv.StrictOptions()
	opts.PackRole = "manifest"
	res, err := excsv.ParseBytes([]byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Meta.Charts) != 0 {
		t.Fatalf("manifest kept %d charts", len(res.Doc.Meta.Charts))
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Kind != excsv.ErrChartOnManifest {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestAddAndRemoveChart(t *testing.T) {
	doc := parseDoc(t, chartSchema+chartData)
	err := doc.AddChart([]excsv.KV{{Key: "type", Value: "bar"}, {Key: "name", Value: "c"}, {Key: "x", Value: "category"}})
	var pe *excsv.ParseError
	if !errors.As(err, &pe) || pe.Issue.Kind != excsv.ErrChartMissingRequiredChannel {
		t.Fatalf("AddChart without y: err = %v", err)
	}
	ok := []excsv.KV{{Key: "type", Value: "bar"}, {Key: "name", Value: "c"}, {Key: "x", Value: "category"}, {Key: "y", Value: "amount"}}
	if err := doc.AddChart(ok); err != nil {
		t.Fatal(err)
	}
	replaced := []excsv.KV{{Key: "type", Value: "arc"}, {Key: "name", Value: "c"}, {Key: "theta", Value: "amount"}}
	if err := doc.AddChart(replaced); err != nil {
		t.Fatal(err)
	}
	if len(doc.Meta.Charts) != 1 || doc.Meta.Charts[0].Type() != "arc" {
		t.Fatalf("AddChart with an existing name must replace it: %+v", doc.Meta.Charts)
	}
	if !doc.RemoveChart("c") || len(doc.Meta.Charts) != 0 {
		t.Fatalf("RemoveChart: %+v", doc.Meta.Charts)
	}
}

func TestChartJSONMirror(t *testing.T) {
	doc := parseDoc(t, chartSchema+
		"#chart type=arc name=donut theta=amount color=category hole=0.5 stack=1 tooltip=category,amount\n"+
		"#chart type=bar name=hist bin=1 x=amount y=count() limit=5\n"+
		"#chart-vega: {\"mark\":\"arc\"}\n"+chartData)
	res, err := doc.ExportJSON(excsv.JSONExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Charts []map[string]any `json:"charts"`
	}
	if err := json.Unmarshal(res.Data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Charts) != 3 {
		t.Fatalf("charts=%v", got.Charts)
	}
	donut, hist, vega := got.Charts[0], got.Charts[1], got.Charts[2]
	if donut["hole"] != 0.5 || donut["stack"] != true {
		t.Fatalf("donut = %v", donut)
	}
	if tt, ok := donut["tooltip"].([]any); !ok || len(tt) != 2 {
		t.Fatalf("tooltip = %#v, want a two-name array", donut["tooltip"])
	}
	if hist["bin"] != true || hist["limit"] != float64(5) || hist["y"] != "count()" {
		t.Fatalf("hist = %v", hist)
	}
	if m, ok := vega["vega"].(map[string]any); !ok || m["mark"] != "arc" {
		t.Fatalf("vega = %v", vega)
	}
}
