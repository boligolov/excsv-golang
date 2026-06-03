# excsv-golang

Go reference implementation of **excsv-cli** — a command-line tool and library for [ExCSV](https://github.com/boligolov/excsv) v0.2 (Extended CSV).

Supports **plain** (`.excsv`, `.ecsv`, `.extsv` sidecars) and **row ZIP** (`.excsv.zip`, `.ecsv.zip`) storage forms. Pack format (`.excsv.pack.zip`) is not implemented yet.

## Requirements

- Go 1.22+
- Python 3 (optional — regenerate zip test fixtures upstream)

## Build

```powershell
# Fast local build -> bin\excsv.exe
.\makefile.ps1

# Flush Go cache + force full rebuild (use when changes don't show up)
.\makefile.ps1 rebuild

# Or manually:
go build -trimpath -o bin/excsv.exe ./cmd/excsv
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

## Test fixtures

The fixture corpus lives under `test/fixtures/` and is **not committed** to this repo (see `.gitignore`). CI and local tests expect it to be present.

**Authority:** [boligolov/excsv](https://github.com/boligolov/excsv) — manifest at `fixtures/fixtures.yaml`, files under `fixtures/plain/` and `fixtures/zip/`.

### Download (recommended)

Sync spec snapshots, the manifest, and every file referenced by the manifest (`id` + `data_sibling`):

```powershell
.\scripts\sync-upstream.ps1
# or:
.\makefile.ps1 sync-upstream
```

```bash
./scripts/sync-upstream.sh
# or:
make sync-upstream
```

Partial sync:

| Command | Gets |
| --- | --- |
| `.\makefile.ps1 sync-specs` | `docs/downloaded/*.md` + `test/fixtures/fixtures.yaml` |
| `.\makefile.ps1 sync-fixtures` | Fixture bytes only (needs `fixtures.yaml` already) |
| `.\scripts\sync-upstream.ps1 -SpecsOnly` | Same as sync-specs |
| `.\scripts\sync-upstream.ps1 -FixturesOnly` | Same as sync-fixtures |

After sync you should have (among others):

```
test/fixtures/fixtures.yaml
test/fixtures/plain/valid/*.excsv
test/fixtures/plain/invalid/*.excsv
test/fixtures/zip/valid/*.excsv.zip
test/fixtures/zip/invalid/*.excsv.zip
```

Sidecar pairs also pull sibling `.csv` / `.tsv` files listed as `data_sibling` in the manifest.

### Full upstream tree (optional)

CI clones the entire `fixtures/plain` and `fixtures/zip` trees from upstream (not just manifest-listed paths). Equivalent locally:

```bash
git clone --depth 1 https://github.com/boligolov/excsv.git /tmp/excsv-spec
cp /tmp/excsv-spec/fixtures/fixtures.yaml test/fixtures/
cp -r /tmp/excsv-spec/fixtures/plain test/fixtures/
cp -r /tmp/excsv-spec/fixtures/zip test/fixtures/
```

More detail: [`docs/sources_and_specifications.md`](docs/sources_and_specifications.md).

## Testing

**Prerequisite:** fixtures synced (see above).

### Library + manifest (default)

Runs `pkg/excsv` tests against `fixtures.yaml` — same expectations as upstream (parse ok/fail, error kinds, sidecar profiles):

```powershell
.\makefile.ps1 test
# or:
go test ./...
```

```bash
make sync-upstream && make test
```

### Compiled CLI against fixtures

Builds `bin/excsv` if needed, then runs `excsv validate` on every `plain/*` and `zip/*` manifest entry:

```powershell
.\makefile.ps1 build
go test ./internal/cli -run TestCLIValidateFixtures -count=1
```

### Manual spot-check

```powershell
.\bin\excsv.exe validate test\fixtures\plain\valid\020_canonical_full_small.excsv
.\bin\excsv.exe validate test\fixtures\plain\invalid\017_checksum_mismatch.excsv   # exit 2
.\bin\excsv.exe info test\fixtures\plain\valid\037_sidecar_csv_sibling.excsv
.\bin\excsv.exe info test\fixtures\plain\valid\037_sidecar_csv_sibling.csv      # discovers .excsv sidecar
```

When upstream adds or changes fixtures, re-sync then re-run both test paths:

```powershell
.\makefile.ps1 sync-fixtures
.\makefile.ps1 test
go test ./internal/cli -run TestCLIValidateFixtures -count=1
```

## Continuous integration

GitHub Actions on **push to `main`** and **PRs to `main`**:

1. **test** — shallow-clone [boligolov/excsv](https://github.com/boligolov/excsv) fixtures, `go test ./...`
2. **build** — cross-compile Windows, Linux, macOS (amd64 + arm64)
3. **bundle** — on push to `main`, artifact `excsv-binaries`

View runs: repo **Actions** tab. Download binaries: green run on `main` → **Artifacts**.

No secrets required for build/test.

## Usage

Use `.\bin\excsv.exe` locally, or `excsv` if on your `PATH`.

```powershell
# Validate
excsv validate data.excsv
excsv validate archive.excsv.zip

# Summary (sidecar: profile + reference= when applicable)
excsv info data.excsv
excsv info data.excsv --json

# Print canonical inner document (unwraps zip; sidecar emits meta only)
excsv cat archive.excsv.zip

# Header and metadata
excsv header list data.excsv
excsv header get version data.excsv
excsv meta list data.excsv

# SQL companions (#$ddl / #$dql)
excsv sql list data.excsv
excsv sql list data.excsv --verb ddl --dialect postgres

# Data
excsv rows count data.excsv
excsv clean data.excsv
excsv convert data.csv -o data.excsv
excsv convert data.tsv -o data.excsv --columns
excsv convert data.csv --sidecar -o data.excsv          # metadata only; reference=data.csv
excsv convert data.csv -o out.excsv --delim pipe --quote double   # re-encode inline data

# ZIP container
excsv zip wrap data.excsv -o data.excsv.zip
excsv zip unwrap data.excsv.zip -o data.excsv
excsv zip peek data.excsv.zip
```

**Sidecar:** open `sales.excsv` (meta + `reference=sales.csv`) or `sales.csv` (auto-discovers `sales.excsv` / `.extsv` in the same directory). Data commands use the merged logical document.

**Flags:** `--strict` (default), `--lenient`, `--json`, `--clean-human-comments`. `--expect-profile` (`stub` | `sidecar` | `sidecar_strict`) is for fixture-style validation. All read commands require a FILE path (no stdin).

## Library

```go
import "github.com/boligolov/excsv-golang/pkg/excsv"

res, err := excsv.ParseFile("data.excsv", excsv.StrictOptions())
if err != nil {
    // handle *excsv.ParseError
}
doc := res.Doc
```

Opening `sales.csv` with a sibling `sales.excsv` loads the sidecar automatically. Set `ParseOptions.ExpectProfile` when you need sidecar-specific errors (e.g. missing `reference=`).

Package layout:

```
cmd/excsv/           CLI entry point
internal/cli/        Cobra commands + CLI fixture tests
internal/fixtures/   Manifest loader (shared by library + CLI tests)
pkg/excsv/           Core parser and serializer
pkg/excsv/zip/       Row ZIP container
test/fixtures/       Fixture corpus (gitignored; sync from upstream)
docs/downloaded/     Spec snapshots (gitignored; sync from upstream)
```

## Status

| Wave | Scope | Status |
| --- | --- | --- |
| 1 | Plain `.excsv` parse, validate, basic CLI, sidecar | Done |
| 2 | Row `.excsv.zip` read/write, zip CLI | Done |
| 3+ | Pack, transforms, full command tree | Not yet |

## License

See [LICENSE](LICENSE).

## Credits

Initial implementation assisted by **Claude Opus 4.8** (Cursor).
