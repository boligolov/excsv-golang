# Go CLI Implementation Plan — ExCSV (plain + zip)

Implementation guide for **excsv-cli** in this repository. Normative behaviour comes from [`docs/downloaded/README-LLM.md`](downloaded/README-LLM.md) and topic files under [`docs/downloaded/llm/`](downloaded/llm/). Feature scope comes from [`docs/downloaded/plan-01-features.md`](downloaded/plan-01-features.md). Tests are driven by [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml).

**In scope:** row family (RF) — **plain** (`.excsv`, `.ecsv`) and **zip** (`.excsv.zip`, `.ecsv.zip`).

**Out of scope (v0.2, this repo):** pack family (PF) — `.excsv.pack.zip`, `#table`, `#fk`, `layout=`, `mode=`, `section-size=`. Recognise reserved names per spec (ignore on read; do not emit).

See also: [`sources_and_specifications.md`](sources_and_specifications.md).

---

## 0. Current status (this repo)

| Area | Status |
| --- | --- |
| Plain parse + serialize | Done — fixture corpus green |
| Row ZIP read/write | Done — `pkg/excsv/zip`, transparent open |
| Core CLI | Partial — see §5.2 implemented commands |
| CSV/TSV import (I1) | Done — `excsv convert` + `ImportDelimited` |
| ExCSV → plain data (I2) | Done — `excsv strip` |
| Full command tree | Not done — column/sql/agg/checksum/freeze/diff/tidy deferred |
| Streaming (A8–A10) | Not done |
| `##` round-trip on serialize | Partial — parsed by default; `SerializeCanonical` does not yet re-emit `##` |

**Module path:** `github.com/boligolov/excsv-golang` (see `go.mod`).

**Build:** `makefile.ps1` / `Makefile` — `build`, `rebuild` (flush Go cache + `-a`), `build-all`. Local Windows build always targets native `windows/amd64` or `windows/arm64` (avoids `GOOS`/`GOARCH` leak after cross-compile). `excsv version` prints link-time build timestamp.

---

## 1. Goals

| Goal | Detail |
| --- | --- |
| Reference implementation | Go is the primary track; Python parity follows the same fixture corpus |
| Spec fidelity | Strict mode fails on every MUST violation; lenient mode collects warnings and continues where spec allows |
| Pipeline-friendly CLI | stdout, `-o` output, exit codes, `--json` for machine output; FILE required on input |
| Two storage forms | Plain text and row ZIP container with transparent read and explicit wrap/unwrap |
| Fixture-green | Every `plain/*` and `zip/*` entry in `fixtures.yaml` passes before a wave is “done” |

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/excsv/main.go  CLI entry                               │
├─────────────────────────────────────────────────────────────┤
│  internal/cli       Cobra commands, I/O, exit codes         │
│    root.go          Global flags, loadDoc                   │
│    commands.go      validate, info, cat, header, meta, …    │
│    version.go       Version / BuildTime (ldflags)           │
├─────────────────────────────────────────────────────────────┤
│  pkg/excsv          Public library API                      │
│    document.go      Document, Header, MetaBlock, options    │
│    parse.go         ParseBytes, parse algorithm             │
│    open.go          ParseFile, ParsePath, zip dispatch      │
│    serialize.go     SerializeCanonical (A6)                  │
│    import.go        ImportDelimited — CSV/TSV → Document    │
│    kv.go            #!excsv and #column key=value parsing   │
│    dialect.go       Delimiter/quote resolution, defaults    │
│    csv.go           split/join CSV fields (custom dialect)  │
│    sql.go           #$ line parsing                         │
│    checksum.go      Data-section SHA-256 (M2, M3)           │
│    errors.go        ErrorKind enum (fixtures.yaml)          │
├─────────────────────────────────────────────────────────────┤
│  pkg/excsv/zip      Row ZIP container (single zip.go)       │
│    Extract, Wrap, locatePrimary, buildComment, …            │
├─────────────────────────────────────────────────────────────┤
│  internal/fixtures  YAML manifest loader + test assertions  │
└─────────────────────────────────────────────────────────────┘
```

**Not yet split out (planned):** `validate.go`, `stream.go`, dedicated zip `comment.go` / `write.go`.

**Dependency rule:** `cmd/` and `internal/cli` may import `pkg/excsv`; `pkg/excsv` must not import CLI code. ZIP layer depends on `pkg/excsv` for inner document semantics, not the reverse.

---

## 3. Document model

Central type representing one RF document (plain bytes or extracted inner file):

```go
type Document struct {
    Form   Form        // Plain | ZipInner
    Header Header      // resolved defaults applied
    Meta   MetaBlock   // ordered slices where order matters (#$, ##)
    Data   DataSection // header row + rows OR index-only rows
    Source SourceInfo  // path, zip envelope, comment snapshot
}

type Header struct {
    Fields map[string]string // raw key=value from #!excsv
    // Resolved:
    Version, DelimName, QuoteName, Null, Encoding, Schema, SQLDialect string
    Delim, Quote rune; QuoteEnabled, HeaderRow bool
    Rows         *int
    Checksum     *Checksum
    OriginalSize *int64       // required when Form is zip inner
    HasMagicLine bool
}

type MetaBlock struct {
    FileMeta      []KV            // #@ — last-wins via upsertKV
    Columns       []ColumnDef
    Aggregations  []Aggregation
    SQL           []SQLStatement  // ordered
    CSVW          *string
    HumanComments []string        // ## — preserved by default; dropped when ClearHumanComments
}
```

Parsing follows README-LLM § PARSING ALGORITHM. Serialization follows § SERIALIZATION ALGORITHM.

### 3.1 Open dispatch (A1, J6)

```
ParsePath(path, data, opts) → ParseResult
  1. If .excsv.zip / .ecsv.zip extension OR PK\x03\x04 magic → zip.Inspect; metadata from archive comment; inner decompress only when `ZipLoadData` (validate/strip/cat)
  2. Else → ParseBytes as plain ExCSV
  3. .pack. paths → fail gracefully (pack not supported)
```

Implemented in [`open.go`](../pkg/excsv/open.go). Stdin zip buffering (A2) not yet wired in CLI.

### 3.2 Parse modes (A4, A5, A7)

```go
type ParseOptions struct {
    Strict              bool
    ClearHumanComments  bool // default false → preserve ##
    ExpectZipInner      bool
    ZipUncompressedSize int64
}
```

CLI global flag `--clean-human-comments` maps to `ClearHumanComments` (opt-out; default preserves `##`).

- **Strict:** MUST-fail → `*ParseError` with `ErrorKind` matching fixture enum; no partial doc.
- **Lenient:** not fully wired for all warning paths; import path supports ragged-row pad/truncate with `Issue` warnings.

### 3.3 Import (I1)

```go
type ImportOptions struct {
    DelimName, QuoteName string // empty = sniff
    NoHeader, AddColumns, Checksum, Strict bool
    FileMeta   []KV
    SourcePath string // extension hint for sniff
}

func ImportDelimited(data []byte, opts ImportOptions) (*ImportResult, error)
```

Sniffs delimiter (comma/tab/pipe/semicolon) and quote style; builds `Document` with `version=0.2`, auto `rows=`, optional `#column` stubs and `checksum=sha256:…`.

### 3.4 Operating modes (feature catalog)

| Mode | Data scan | Typical commands |
| --- | --- | --- |
| **A — metadata-only** | No | `header`/`meta`/`sql` list & get, `info`, `rows` (ZIP: archive comment) |
| **B — data-aware** | Yes | `strip`, `convert`, `validate` |

Mode B auto-sync of `rows=`, `checksum=`, `#%` on write — planned (P6); partially available via `ImportOptions.Checksum` and `SetDataChecksum`.

---

## 4. Library packages — responsibilities

### 4.1 `pkg/excsv` — core

| Area | Files | Status |
| --- | --- | --- |
| Header + meta parse | `kv.go`, `parse.go`, `sql.go` | Done |
| CSV dialect engine | `csv.go`, `dialect.go` | Done (custom; not `encoding/csv`) |
| Canonical writer | `serialize.go` | Done (no `##` emit yet) |
| Checksum | `checksum.go` | Done — verify on parse; compute on import |
| CSV/TSV import | `import.go` | Done |
| Open / zip dispatch | `open.go` | Done |
| Validate helper | — | Via parse + `excsv validate` only |
| Stream API (A8–A10) | — | Not started |

**Forward compatibility (P7, P8):** unknown header keys and unknown `#` lines → ignore. Reserved pack keys and `#table` / `#fk` → ignore on read, never write.

### 4.2 `pkg/excsv/zip` — row container (J1–J6)

| Function | Status |
| --- | --- |
| `Inspect(archivePath, data)` | Done — central dir + comment, no decompress |
| `ExtractPrimary` / `Extract` | Done — decompress primary when data needed |
| `Wrap(inner, entryName, comment)` | Done — two-pass `original-size`, deflate |
| `locatePrimary`, `buildComment`, `truncateComment` | Done |
| `RefreshComment` | Not exposed on CLI |

---

## 5. CLI design

Framework: **cobra**. Binary name: `excsv`. Output: `bin/excsv.exe` (Windows) via `makefile.ps1`.

### 5.1 Global flags

| Flag | Purpose |
| --- | --- |
| `--strict` / `--lenient` | Parse mode (default: strict) |
| `--json` | Machine-readable output where supported |
| `--clean-human-comments` | Drop `##` on read (default: preserve) |

All read commands require a **FILE** argument; stdin input is not supported. Write commands may print to stdout when `-o` is omitted.

Planned: `--in-place` (P1).

Exit codes: `0` ok; `1` user error; `2` parse/validation failure; `3` I/O failure.

### 5.2 Command tree

**Implemented today:**

```
excsv
├── validate [file]         # M1 — parse check
├── info [file]             # N1 summary (--json)
├── cat [file]              # canonical inner document (unwraps zip)
├── header
│   ├── list FILE
│   └── get [KEY] FILE
├── meta
│   ├── list FILE
│   └── get [KEY] FILE
├── rows FILE              # alias: header get rows FILE
├── sql
│   ├── list FILE
│   └── get [KEY] FILE
├── strip [file]            # I2 — remove ExCSV metadata, print data rows as CSV/TSV
├── convert [file]          # I1 — CSV/TSV → ExCSV
│   # flags: -o, --delim, --quote, --no-header, --columns,
│   #        --checksum, --meta KEY:VAL, --zip
├── zip
│   ├── wrap INPUT -o OUT
│   └── unwrap INPUT.zip -o OUT
└── version                 # prints version + build timestamp
```

**Planned (not implemented):**

```
excsv
├── header set/unset, meta set/import, column …, agg …
├── rows head/tail/slice, data print/get
├── convert normalize, to-tsv          # I3 — or separate commands
├── zip refresh-comment
├── checksum compute/verify, freeze, diff, tidy
└── open (alias of cat)
```

**Naming note:** I2 (ExCSV → delimited) is **`strip`**, not `convert to-csv`. I1 (delimited → ExCSV) is top-level **`convert`**.

**Output family rule:** `convert --zip` wraps output; otherwise plain `.excsv`. `strip` always prints delimited text to stdout.

### 5.3 Command → feature mapping

| Group | Feature IDs | Wave | Repo status |
| --- | --- | --- | --- |
| Core parse/serialize | A1–A7 | 1–2 | Done (A7 partial: ## parse yes, serialize no) |
| Import | I1 | 1 | Done |
| Export data | I2 | 1 | Done (`strip`) |
| Zip container | J1–J6 | 2 | Done (J4 CLI pending) |
| Header / meta read | B*, C* partial | 1 | Partial |
| Data read | G1 partial | 1 | use `header get rows` or `rows` alias |
| Integrity CLI | M2–M3 | 1–2 | Library only |
| Convert I3, H*, rest | — | 3+ | Not started |

---

## 6. Implementation phases

Work in two waves per upstream plan. **Do not start wave 2 until wave 1 fixture subset is green.**

### Phase 0 — scaffold

- [x] `go.mod`, `cmd/excsv/main.go`, cobra root
- [x] `pkg/excsv` types, error kinds enum mirroring YAML
- [x] `internal/fixtures` — load manifest, resolve paths
- [x] CI: `.github/workflows/ci.yml` — `go test ./...`, cross-build
- [x] Fixture sync in CI (clone upstream `boligolov/excsv`)

### Phase 1 — plain row (wave 1)

**Milestone 1.1 — parse only** — [x]

- Header, meta, data, strict errors, fixture runner

**Milestone 1.2 — serialize + round-trip** — [x]

- `SerializeCanonical`, BOM/CRLF handling

**Milestone 1.3 — validation & integrity** — [x] parse-time; [ ] dedicated checksum CLI

- Checksum verify on parse; aggregation/column validation in parser
- `excsv validate`

**Milestone 1.4 — Mode A CLI** — [~] partial

- [x] `header list/get`, `meta list`, `cat`
- [ ] mutators, column/sql subcommands, in-place writes

**Milestone 1.5 — Mode B read + convert** — [~] partial

- [x] `rows` (alias), `strip`, `convert` (from-csv)
- [ ] `data print`, checksum/freeze CLI, agg compute

Target: all **36** plain valid + **26** plain invalid fixtures green — **done**.

### Phase 2 — row zip (wave 2)

**Milestone 2.1 — zip read** — [x]

**Milestone 2.2 — zip write** — [x]

**Milestone 2.3 — zip CLI** — [~] partial

- [x] `zip wrap`, `unwrap` (no `peek`; metadata via comment + `ZipLoadData=false`)
- [ ] `refresh-comment`

**Milestone 2.4 — zip + Mode B** — [x] basic

- [x] `convert --zip`, transparent open on read paths

Target: all **10** zip valid + **9** zip invalid fixtures green — **done**.

### Phase 3 — polish (still RF only)

- [ ] Streaming (A8, A9), `##` serialize round-trip
- [ ] `diff`, `tidy`, full meta/column/sql/agg CLI
- [ ] H1–H4 transforms

---

## 7. Testing strategy

### 7.1 Fixture-driven tests

Implemented as `TestManifestFixtures` in [`pkg/excsv/fixtures_test.go`](../pkg/excsv/fixtures_test.go) — walks `plain/*` and `zip/*` in `fixtures.yaml`.

### 7.2 Unit tests

| Package | File | Focus |
| --- | --- | --- |
| `pkg/excsv` | `import_test.go` | Sniff, headers, columns, checksum, round-trip |
| `pkg/excsv` | (inline) | CSV dialect, parse errors via fixtures |
| `pkg/excsv/zip` | — | Covered indirectly by fixtures |

### 7.3 Golden / canonical bytes

Optional `*.canonical` siblings — not yet used.

### 7.4 Fixture file sync

Run `.\scripts\sync-upstream.ps1` (Windows) or `make sync-upstream` / `./scripts/sync-upstream.sh` — downloads spec snapshots, `fixtures.yaml`, and every manifest `id` / `data_sibling` into `test/fixtures/`. CI clones the full upstream `fixtures/plain` and `fixtures/zip` trees instead; both approaches must yield the same bytes. Local `test/fixtures/` and `docs/downloaded/` are gitignored — see README and [`sources_and_specifications.md`](sources_and_specifications.md).

---

## 8. Error and warning model

```go
type ErrorKind string // mirrors fixtures.yaml — see errors.go

type Issue struct {
    Kind    ErrorKind
    Message string
    Line    int // 1-based; 0 if unknown
}

type ParseError struct { Issue Issue }

type ParseResult struct {
    Doc      *Document
    Warnings []Issue // lenient/import paths only today
}
```

Strict parse: return `(nil, *ParseError)` — fail fast, no doc.

Import lenient: return `ImportResult{Doc, Warnings}` with padded/truncated rows.

---

## 9. I/O and concurrency

| Concern | Status |
| --- | --- |
| Stdin (plain or zip) | Not supported — FILE required on all read commands |
| Atomic in-place (P1) | Not done |
| `--json` | Partial (`validate`, `info`, `convert`) |
| UTF-8 | Required on import; encoding header on ExCSV output |

---

## 10. Dependencies

| Module | Use |
| --- | --- |
| `github.com/spf13/cobra` | CLI |
| `gopkg.in/yaml.v3` | Fixture manifest |
| `archive/zip` | Row container |
| `crypto/sha256` | Checksum |

Stdlib-first; no `golang.org/x/text` yet (UTF-8 only in practice).

---

## 11. Explicit non-goals

Unchanged — see prior plan. Pack format, `#table`/`#fk`, SQL execution, encryption, plugins.

---

## 12. Success criteria

| Wave | Done when | Status |
| --- | --- | --- |
| **1 — plain** | Fixtures green; validate, convert, basic read CLI | **Done** |
| **2 — zip** | Zip fixtures green; wrap/unwrap; comment metadata; transparent open | **Done** |
| **RF complete** | Waves 1–2 + README + importable `pkg/excsv` | **Core done**; full CLI tree remains |

---

## 13. Suggested next PR sequence

1. Emit `HumanComments` in `SerializeCanonical` (A7 round-trip)
2. `checksum compute/verify` CLI
3. `header set`, `meta set`, column/sql read subcommands
4. `data print`, streaming reader
5. `zip refresh-comment`, `freeze`, `tidy`, `diff`

---

## 14. References

| Document | Role |
| --- | --- |
| [`README-LLM.md`](downloaded/README-LLM.md) + [`llm/`](downloaded/llm/) | Normative format (hub + topics) |
| [`plan-README.md`](downloaded/plan-README.md) | Wave gating, spec-first rule |
| [`plan-01-features.md`](downloaded/plan-01-features.md) | Feature IDs A–P |
| [`plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) | Fixture layout, manifest schema |
| [`fixtures.yaml`](../test/fixtures/fixtures.yaml) | Expected outcomes (plain + zip entries) |
| [`sources_and_specifications.md`](sources_and_specifications.md) | Upstream links + refresh policy |
