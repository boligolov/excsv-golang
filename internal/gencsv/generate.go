package gencsv

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

// Format is comma-separated or tab-separated output.
type Format string

const (
	FormatCSV Format = "csv"
	FormatTSV Format = "tsv"
)

// Options controls dummy file generation.
type Options struct {
	Rows    int
	Columns []ColumnSpec
	Format  Format
	Header  bool
	Seed    *int64 // nil = deterministic from row/column indices
}

func (o Options) dialect() excsv.Dialect {
	switch o.Format {
	case FormatTSV:
		return excsv.Dialect{Delim: '\t', Quote: '"', QuoteEnabled: true}
	default:
		return excsv.Dialect{Delim: ',', Quote: '"', QuoteEnabled: true}
	}
}

// Write generates rows to w (header row first when Header is true).
func Write(w io.Writer, opts Options) error {
	if opts.Rows < 0 {
		return fmt.Errorf("rows must be >= 0")
	}
	if len(opts.Columns) == 0 {
		return fmt.Errorf("at least one --column is required")
	}
	switch opts.Format {
	case FormatCSV, FormatTSV:
	default:
		return fmt.Errorf("format must be csv or tsv")
	}

	d := opts.dialect()
	var rng *rand.Rand
	if opts.Seed != nil {
		rng = rand.New(rand.NewSource(*opts.Seed))
	}

	writeLine := func(fields []string) error {
		_, err := fmt.Fprintln(w, excsv.JoinCSVFields(fields, d))
		return err
	}

	if opts.Header {
		names := make([]string, len(opts.Columns))
		for i, c := range opts.Columns {
			names[i] = c.Name
		}
		if err := writeLine(names); err != nil {
			return err
		}
	}

	for row := 0; row < opts.Rows; row++ {
		fields := make([]string, len(opts.Columns))
		for col, spec := range opts.Columns {
			fields[col] = cellValue(row, col, spec, rng)
		}
		if err := writeLine(fields); err != nil {
			return err
		}
	}
	return nil
}

func cellValue(row, col int, spec ColumnSpec, rng *rand.Rand) string {
	if spec.Type == TypeNull {
		return ""
	}
	if spec.Nulls && isNullCell(row, col, spec.Name, rng) {
		return ""
	}
	switch spec.Type {
	case TypeInt:
		return fmt.Sprintf("%d", row*1000+col*17+len(spec.Name))
	case TypeString:
		return fmt.Sprintf("%s_%d", spec.Name, row+1)
	case TypeDate:
		t := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, row%3650)
		return t.Format("2006-01-02")
	case TypeFloat:
		return fmt.Sprintf("%.4f", float64(row)*1.37+float64(col)*0.11)
	case TypeBoolean:
		if row%2 == 0 {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// ~10% null rate when Nulls is set.
func isNullCell(row, col int, name string, rng *rand.Rand) bool {
	if rng != nil {
		return rng.Intn(10) == 0
	}
	h := row*31 + col*17
	for _, r := range name {
		h += int(r)
	}
	return h%10 == 0
}

func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "csv", "":
		return FormatCSV, nil
	case "tsv", "tab":
		return FormatTSV, nil
	default:
		return "", fmt.Errorf("unknown format %q (csv, tsv)", s)
	}
}
