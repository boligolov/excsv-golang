# excsv-golang

Go reference implementation of **excsv-cli** — a command-line tool and library for [ExCSV](https://github.com/boligolov/excsv) v0.4 (Extended CSV).

Supports **plain** (`.excsv`, `.ecsv`, `.extsv` sidecars), **row ZIP** (`.excsv.zip`, `.ecsv.zip`), and **pack** (`.excsv.pack.zip`).

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
# excsv-cli 0.0.2 (built …)
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

**Authority:** [boligolov/excsv](https://github.com/boligolov/excsv) — manifest at `fixtures/fixtures.yaml`, files under `fixtures/plain/`, generated `fixtures/zip/` and `fixtures/pack/`.

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
| `.\makefile.ps1 sync-specs` | `docs/downloaded/README.md`, `guide/`, `implementation/`, plan snapshots + `test/fixtures/fixtures.yaml` |
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
test/fixtures/pack/valid/*.excsv.pack.zip
test/fixtures/pack/invalid/*.excsv.pack.zip
```

Sidecar pairs also pull sibling `.csv` / `.tsv` files listed as `data_sibling` in the manifest.

### Full upstream tree (optional)

CI clones the entire `fixtures/plain` tree from upstream and generates `zip` + `pack`. Equivalent locally:

```bash
git clone --depth 1 https://github.com/boligolov/excsv.git /tmp/excsv-spec
cp /tmp/excsv-spec/fixtures/fixtures.yaml test/fixtures/
cp -r /tmp/excsv-spec/fixtures/plain test/fixtures/
python3 /tmp/excsv-spec/fixtures/generate/make_zip_fixtures.py
python3 /tmp/excsv-spec/fixtures/generate/make_pack_fixtures.py
cp -r /tmp/excsv-spec/fixtures/zip test/fixtures/
cp -r /tmp/excsv-spec/fixtures/pack test/fixtures/
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

Builds `bin/excsv` if needed, then runs `excsv validate` on every `plain/*`, `zip/*`, and `pack/*` manifest entry:

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
# Validate (schema-only by default; --with-data for cells, rows=, checksum=, #%)
excsv data.excsv validate
excsv data.excsv validate --with-data
excsv data.excsv validate --with-data --column amount
excsv archive.excsv.zip validate

# Repair derived metadata in place
excsv data.excsv fix
excsv data.excsv fix --only format
excsv data.excsv fix --only agg,checksum --dry-run

# Summary
excsv data.excsv info
excsv data.excsv info --json

# Convert CSV/TSV → ExCSV (or re-encode an existing document)
excsv data.csv convert -o data.excsv
excsv data.tsv convert -o data.extsv --format sidecar
excsv data.excsv convert --delim tab -o data.extsv

# Data section
excsv data.excsv data print -o data.csv
excsv data.excsv data print --limit 20 --select id,amount
excsv data.excsv data get 0 amount
excsv data.excsv data append --row "3,9.50"
excsv data.excsv data sort --by amount:desc

# Schema / metadata
excsv data.excsv column list
excsv data.excsv column set amount --attr type=decimal --attr unit=USD
excsv data.excsv agg update sum
excsv data.excsv header list          # read-only
excsv data.excsv header rows
excsv data.excsv meta set author --value "author@example.com"
excsv data.excsv sql dialect set postgres

# Export (never modifies FILE)
excsv data.excsv export json -o data.excsv.json
excsv data.excsv export csvw --url data.csv -o data.csv-metadata.json

# Pack / ZIP
excsv data.excsv pack create -o data.excsv.pack.zip
excsv data.excsv.pack.zip pack table list
excsv data.excsv zip wrap -o data.excsv.zip
excsv data.excsv.zip zip unwrap -o data.excsv
```

**Pattern:** `excsv [flags] FILE <command> …`. Full map: [`docs/cli_commands_map.md`](docs/cli_commands_map.md).

**Sidecar:** `excsv sales.csv …` auto-discovers `sales.excsv` / `.extsv`; data writes also update the referenced CSV.

**Row ZIP:** `info` / `header` / `meta` / `sql` / `header rows` / `column list` read the archive comment; `validate --with-data` / `data print` / `fix` / `export` decompress the inner `.excsv`.

**Flags:** `--strict` (default), `--lenient`, `--json`, `--clean-human-comments`, `--zip-password`, `--expect-profile` (`stub` | `sidecar` | `sidecar_strict`).

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
docs/downloaded/     Spec hub + guide/ + implementation/ (gitignored; sync from upstream)
```

## Status

| Wave | Scope | Status |
| --- | --- | --- |
| 1 | Plain `.excsv` parse, convert, data, schema, sidecar | Done |
| 2 | Row `.excsv.zip` read/write, zip CLI, password | Done |
| 3 | Pack, grouped CLI (v0.0.2), validate/fix, export json/csvw | Done |
| 4+ | Streaming, diff | Not yet |

## License

See [LICENSE](LICENSE).

## Credits

Initial implementation assisted by **Claude Opus 4.8** (Cursor).
