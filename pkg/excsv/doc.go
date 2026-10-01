/*
Package excsv is the reference Go implementation of ExCSV (Extended CSV) v0.6
— CSV that describes itself. A conforming document is still plain,
delimiter-separated data; schema, units, summary statistics, SQL DDL/DQL, and
an integrity checksum ride along as "#" comment lines that any existing CSV
reader already skips.

The package parses, validates, repairs, and serializes the text shapes the
spec defines: plain/sidecar ".excsv" (data inline, or meta-only with
reference= pointing at an untouched sibling CSV/TSV), row-oriented ZIP
(".excsv.zip"), and the multi-table columnar pack (".excsv.pack.zip"). It
also writes the JSON form (".excsv.json", a lossless bijection with the text
form); reading JSON is not implemented. See
[ParseFile] and [ParseBytes] for reading, [Document.SerializeCanonical],
[Document.ExportJSON], and [Document.ExportCSVW] for writing,
[Document.Validate] and [Document.Fix] for conformance checking and repair,
and [Pack] for the columnar container.

# Quick start

	res, err := excsv.ParseFile("data.excsv", excsv.StrictOptions())
	if err != nil {
		var pe *excsv.ParseError
		if errors.As(err, &pe) {
			// pe.Issue.Kind is one of the ErrorKind constants (error registry)
		}
		return err
	}
	doc := res.Doc

Opening "sales.csv" with a sibling "sales.excsv" auto-discovers and loads the
sidecar; set the ExpectProfile field on [ParseOptions] when the caller needs
sidecar-specific errors (e.g. a missing reference=).

# Computed columns

A #column may carry formula= to derive its value from other stored columns
instead of storing it (v0.5). [Document.MaterializeColumn] evaluates the
formula and writes the values in as an ordinary column; [Document.DematerializeColumn]
reverses that, dropping the cached data but keeping formula=.

# Charts

A #chart line suggests how already-declared columns are meant to be viewed
together (type=bar x=category y=amount, …); #chart-vega: carries a raw
Vega-Lite spec instead. Charts are advisory and parsed into
[MetaBlock].Charts; every channel is checked against the #column names, and
the FAIL codes (missing type=/name=, unknown column, missing required
channel, invalid Vega JSON) reject the file. See [Chart],
[Document.ChartByName], and [Document.AddChart].

# Notes and links

A #note pins a remark to a cell, a row, a column, or the table (v0.6); a
link= template on #column makes every value a link, and a #link line links a
single cell, overriding the template. Rows are addressed by position (row=)
or by the value of the table's id column (key=), which survives sorting.
Notes and links are advisory and never part of the data:
[Document.ResolveNotes] resolves each note's address, [Document.ResolveLinks]
substitutes and percent-encodes templates and applies the http/https/mailto
scheme check; unresolved lines only warn and are kept on round-trip.
[Document.SortRows] keeps row= anchors pointing at the same rows. See [Note],
[Link], [Document.AddNote], and [Document.SetLink].

# Error handling

Parse and validation failures are reported as *[ParseError] wrapping an
[Issue], whose Kind is one of the ErrorKind constants — the same vocabulary
as the spec's normative error-code registry (docs/implementation/error-handling.md
upstream). [Document.Validate] returns every finding in one pass rather than
stopping at the first problem.

The CLI built on this package is github.com/boligolov/excsv-golang/cmd/excsv.
The format itself is specified at https://github.com/boligolov/excsv.
*/
package excsv
