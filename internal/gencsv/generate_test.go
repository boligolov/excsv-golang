package gencsv

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Options{
		Rows: 3,
		Columns: []ColumnSpec{
			{Name: "a", Type: TypeInt},
			{Name: "b", Type: TypeString},
			{Name: "c", Type: TypeDate, Nulls: true},
		},
		Format: FormatCSV,
		Header: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4 (header + 3 rows):\n%s", len(lines), buf.String())
	}
	if lines[0] != "a,b,c" {
		t.Fatalf("header = %q", lines[0])
	}
	if lines[1] != "1,b_1,2020-01-01" {
		t.Fatalf("row1 = %q", lines[1])
	}
}

func TestWriteTSV(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Options{
		Rows: 1,
		Columns: []ColumnSpec{
			{Name: "id", Type: TypeInt},
			{Name: "flag", Type: TypeBoolean},
		},
		Format: FormatTSV,
		Header: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\t") {
		t.Fatalf("expected tab delimiter: %q", buf.String())
	}
}

func TestWriteNullColumn(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Options{
		Rows: 2,
		Columns: []ColumnSpec{
			{Name: "n", Type: TypeNull},
			{Name: "v", Type: TypeInt},
		},
		Format: FormatCSV,
		Header: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.HasPrefix(line, ",") {
			t.Fatalf("null column should be empty first field: %q", line)
		}
	}
}
