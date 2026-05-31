# excsv-golang

Go reference implementation of **excsv-cli** — a command-line tool and library for [ExCSV](https://github.com/boligolov/excsv) v0.2 (Extended CSV).

Supports **plain** (`.excsv`, `.ecsv`) and **row ZIP** (`.excsv.zip`, `.ecsv.zip`) storage forms. Pack format (`.excsv.pack.zip`) is not implemented yet.

## Specification

Normative behaviour comes from the upstream spec and plan:

| Document | Link |
| --- | --- |
| ExCSV specification (LLM reference) | [README-LLM.md](https://github.com/boligolov/excsv/blob/master/README-LLM.md) |
| Implementation plan | [plan/README.md](https://github.com/boligolov/excsv/blob/master/plan/README.md) |
| Feature catalog | [plan/01-features.md](https://github.com/boligolov/excsv/blob/master/plan/01-features.md) |
| Test fixtures spec | [plan/02-fixtures.md](https://github.com/boligolov/excsv/blob/master/plan/02-fixtures.md) |
| Fixture manifest | [fixtures/fixtures.yaml](https://github.com/boligolov/excsv/blob/master/fixtures/fixtures.yaml) |

Local snapshots and refresh notes: [`docs/sources_and_specifications.md`](docs/sources_and_specifications.md). Implementation details: [`docs/implementation.md`](docs/implementation.md).

## Requirements

- Go 1.22+
- Python 3 (optional — regenerate zip test fixtures)

## Build

```powershell
go build -o excsv.exe ./cmd/excsv
```

## Usage

```powershell
# Validate
excsv validate data.excsv
excsv validate archive.excsv.zip

# Summary
excsv info data.excsv
excsv info data.excsv --json

# Print inner plain document (unwraps zip transparently)
excsv cat archive.excsv.zip

# Header and metadata
excsv header list data.excsv
excsv header get version data.excsv
excsv meta list data.excsv

# Data
excsv rows count data.excsv
excsv convert to-csv data.excsv

# ZIP container
excsv zip wrap data.excsv -o data.excsv.zip
excsv zip unwrap data.excsv.zip -o data.excsv
excsv zip peek data.excsv.zip
```

Use `-` for stdin where supported. Global flags: `--strict` (default), `--lenient`, `--json`.

## Library

```go
import "github.com/boligolov/excsv-golang/pkg/excsv"

res, err := excsv.ParseFile("data.excsv", excsv.StrictOptions())
if err != nil {
    // handle *excsv.ParseError
}
doc := res.Doc
```

Package layout:

```
cmd/excsv/           CLI entry point
internal/cli/        Cobra commands
internal/fixtures/   Test manifest loader
pkg/excsv/           Core parser and serializer
pkg/excsv/zip/       Row ZIP container
test/fixtures/       Fixture corpus (plain + zip)
```

## Tests

```powershell
go test ./...

# Regenerate zip fixtures (after changing plain sources)
python test/fixtures/generate/make_zip_fixtures.py
```

Tests walk `test/fixtures/fixtures.yaml` and cover all `plain/*` and `zip/*` entries.

## Status

| Wave | Scope | Status |
| --- | --- | --- |
| 1 | Plain `.excsv` parse, validate, basic CLI | Done |
| 2 | Row `.excsv.zip` read/write, zip CLI | Done |
| 3+ | Pack, transforms, full command tree | Not yet |

## License

See [LICENSE](LICENSE).

## Credits

Initial implementation assisted by **Claude Opus 4.8** (Cursor).
