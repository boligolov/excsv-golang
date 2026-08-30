package excsv_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestComputeAggregationValues(t *testing.T) {
	root := filepath.Join(filepath.Dir(filePath(t)), "..", "..", "test", "fixtures", "plain", "valid")
	path := filepath.Join(root, "011_aggregations_standard.excsv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("fixtures not synced")
	}
	res, err := excsv.ParseBytes(data, excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	doc := res.Doc

	cases := map[string]string{
		"count_nonnull":  "3,3,3",
		"count_null":     "0,0,0",
		"count_distinct": "3,3,3",
		"sum":            ",,60.00",
		"avg":            ",,20.00",
		"min":            ",,10.00",
		"max":            ",,30.00",
		"len_min":        ",3,",
		"len_max":        ",5,",
	}
	for name, want := range cases {
		got, err := excsv.ComputeAggregationValues(doc, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d := doc.Header.Dialect()
		have := excsv.JoinCSVFields(got, d)
		if have != want {
			t.Fatalf("%s: got %q want %q", name, have, want)
		}
	}
}

func TestAddAggregationNoOpWhenExists(t *testing.T) {
	doc := &excsv.Document{
		Header: excsv.Header{HeaderRow: true, Fields: map[string]string{"version": "0.2"}},
		Meta: excsv.MetaBlock{
			Aggregations: []excsv.Aggregation{{Name: "sum", Values: []string{"", "", "1"}}},
		},
		Data: excsv.DataSection{
			HasHeaderRow: true,
			HeaderRow:    []string{"a"},
			Rows:         [][]string{{"1"}},
		},
	}
	added, err := doc.AddAggregation("sum")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("expected no-op add")
	}
	if doc.Meta.Aggregations[0].Values[2] != "1" {
		t.Fatalf("values changed: %v", doc.Meta.Aggregations[0].Values)
	}
}

func filePath(t *testing.T) string {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	return f
}
