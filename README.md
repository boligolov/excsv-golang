# excsv-golang

Go reference implementation of **excsv-cli** — a command-line tool and library for [ExCSV](https://github.com/boligolov/excsv) v0.2 (Extended CSV).

Supports **plain** (`.excsv`, `.ecsv`) and **row ZIP** (`.excsv.zip`, `.ecsv.zip`) storage forms. Pack format (`.excsv.pack.zip`) is not implemented yet.

## Requirements

- Go 1.22+
- Python 3 (optional — regenerate zip test fixtures)

## Build

```powershell
# Fast local build -> bin\excsv.exe
.\makefile.ps1

# Flush Go cache + force full rebuild (use when changes don't show up)
.\makefile.ps1 rebuild

# Or manually:
go build -trimpath -a -o bin/excsv.exe ./cmd/excsv
```

Verify you picked up the new binary:

```powershell
.\bin\excsv.exe version
# excsv-cli 0.2.0 (built 2026-06-01T12:34:56Z)
```

> If rebuild fails with "Cannot remove stale binary", close any running `excsv.exe` first — Windows locks the file while it's in use.
>
> **Wrong platform / exe won't start:** If you ran `build-all` in the same PowerShell session before an older `makefile.ps1`, `GOOS`/`GOARCH` could leak and produce a non-Windows `bin\excsv.exe`. Run `.\makefile.ps1 rebuild` (fixed in current script) or use `bin\excsv-windows-amd64.exe` directly.

Cross-platform builds:

```powershell
# Windows (PowerShell)
.\makefile.ps1 build-all

# Git Bash / WSL / macOS / Linux
make build-all
```

Binaries land in `bin/` (see `Makefile` / `makefile.ps1`).

## Continuous integration

GitHub Actions runs on every **push to `main`** and on **pull requests targeting `main`**:

1. **test** — fetch fixture corpus from [boligolov/excsv](https://github.com/boligolov/excsv), run `go test ./...`
2. **build** — cross-compile for Windows, Linux, and macOS (amd64 + arm64)
3. **bundle** — on push to `main`, combine all binaries into one artifact (`excsv-binaries`)

### Setup (one-time)

1. Push this repo to GitHub (Actions are enabled by default for public repos).
2. Merge the workflow file (`.github/workflows/ci.yml`) into `main` — CI starts automatically.
3. View runs: **GitHub repo → Actions** tab.
4. Download built binaries: open a green workflow run on `main` → **Artifacts** → `excsv-binaries`.

No secrets required for build/test. For GitHub Releases on tag push, add a release workflow later.

> **Note:** `test/fixtures/` is gitignored locally; CI clones fixtures from upstream `boligolov/excsv`. To run tests fully offline, remove that line from `.gitignore` and commit your fixture tree.

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
excsv clean data.excsv
excsv convert data.csv -o data.excsv
excsv convert data.tsv -o data.excsv --columns

# ZIP container
excsv zip wrap data.excsv -o data.excsv.zip
excsv zip unwrap data.excsv.zip -o data.excsv
excsv zip peek data.excsv.zip
```

Use `--strict` (default), `--lenient`, `--json`. All read commands require a FILE path (no stdin).

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
