/*
Command excsv is the reference command-line tool for ExCSV (Extended CSV) —
a plain-CSV-compatible format that carries its own schema, types, units,
summary statistics, SQL DDL/DQL, and an integrity checksum as "#" comment
lines that ordinary CSV readers already ignore.

It reads and writes four container shapes: plain/sidecar ".excsv" (and
".extsv" for TSV), the JSON form ".excsv.json", row-oriented ".excsv.zip",
and the multi-table columnar ".excsv.pack.zip".

# Usage

The document comes first, then a command:

	excsv [flags] FILE <group> <command> [args]

Examples:

	excsv data.csv convert -o data.excsv          # CSV/TSV -> ExCSV
	excsv data.excsv validate --with-data         # conformance report
	excsv data.excsv fix                          # repair derived metadata
	excsv data.excsv info                         # document summary
	excsv data.excsv data print -o data.csv       # strip back to plain CSV
	excsv data.excsv column list                  # #column schema
	excsv data.excsv column materialize total     # write a formula= column's values in
	excsv data.excsv agg update sum               # recompute #% aggregations
	excsv data.excsv sql ddl postgres             # emit #$ddl for one dialect
	excsv data.excsv export json -o data.excsv.json
	excsv data.excsv pack create -o data.excsv.pack.zip
	excsv data.excsv zip wrap -o data.excsv.zip
	excsv version

Run "excsv <group> --help" for a command's own flags, or see the full
command map at https://github.com/boligolov/excsv-golang/blob/main/docs/cli_commands_map.md.

Global flags (accepted before or after FILE): --strict (default), --lenient,
--json, --clean-human-comments, --zip-password, --expect-profile, --table
(pack). All parsing and mutation logic lives in the library package,
github.com/boligolov/excsv-golang/pkg/excsv; this command is a thin Cobra
wrapper around it (see package cli in internal/cli).

The ExCSV format itself is specified at https://github.com/boligolov/excsv.
*/
package main
