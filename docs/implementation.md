# Go CLI Implementation Plan — ExCSV (plain + zip)

Implementation guide for **excsv-cli** in this repository. Normative behaviour comes from [`docs/downloaded/README-LLM.md`](downloaded/README-LLM.md). Feature scope comes from [`docs/downloaded/plan-01-features.md`](downloaded/plan-01-features.md). Tests are driven by [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml).

**In scope:** row family (RF) — **plain** (`.excsv`, `.ecsv`) and **zip** (`.excsv.zip`, `.ecsv.zip`).

**Out of scope (v0.2, this repo):** pack family (PF) — `.excsv.pack.zip`, `#table`, `#fk`, `layout=`, `mode=`, `section-size=`. Recognise reserved names per spec (ignore on read; do not emit).

See also: [`sources_and_specifications.md`](sources_and_specifications.md).

---

## 1. Goals

| Goal | Detail |
| --- | --- |
| Reference implementation | Go is the primary track; Python parity follows the same fixture corpus |
| Spec fidelity | Strict mode fails on every MUST violation; lenient mode collects warnings and continues where spec allows |
| Pipeline-friendly CLI | stdin/stdout, `-` paths, exit codes, `--json` for machine output |
| Two storage forms | Plain text and row ZIP container with transparent read and explicit wrap/unwrap |
| Fixture-green | Every `plain/*` and `zip/*` entry in `fixtures.yaml` passes before a wave is “done” |

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/excsv          CLI (cobra): flags, I/O routing, exit   │
├─────────────────────────────────────────────────────────────┤
│  internal/cli       Command handlers; thin orchestration    │
├─────────────────────────────────────────────────────────────┤
│  pkg/excsv          Public library API                      │
│    document.go      Document model + open/parse/serialize   │
│    header.go        #!excsv key=value parsing               │
│    meta.go          #@, #column, #%, #$ , #csvw, ##         │
│    csv.go           Dialect resolution + row decode/encode  │
│    sql.go           Dialect resolution, DDL/DQL helpers     │
│    checksum.go      LF-normalized data-section digests      │
│    validate.go      M1 conformance + error/warning types    │
│    canonical.go     A6 canonical byte order                 │
│    stream.go        A8/A9/A10 streaming readers/writers     │
├─────────────────────────────────────────────────────────────┤
│  pkg/excsv/zip      Row ZIP container (J1–J6)               │
│    open.go          Primary entry locate + extract          │
│    write.go         Wrap, comment builder, original-size    │
│    comment.go       Peek / refresh ZIP comment              │
├─────────────────────────────────────────────────────────────┤
│  internal/fixtures  YAML manifest loader + test runner glue │
└─────────────────────────────────────────────────────────────┘
```

**Dependency rule:** `cmd/` and `internal/cli` may import `pkg/excsv`; `pkg/excsv` must not import CLI code. ZIP layer depends on `pkg/excsv` for inner document semantics, not the reverse.

**Suggested module path:** `github.com/boligolov/excsv-golang` (adjust to actual module name in `go.mod`).

---

## 3. Document model

Central type representing one RF document (plain bytes or extracted inner file):

```go
type Document struct {
    Form       Form          // Plain | ZipInner (logical content always row-oriented)
    Header     Header        // resolved defaults applied
    Meta       MetaBlock     // ordered slices where order matters (#$, ## opt-in)
    Data       DataSection   // header row + rows OR index-only rows
    Source     SourceInfo    // path, zip envelope, comment snapshot
    Raw        *RawCapture   // optional A10 passthrough bytes for Mode A
}

type Header struct {
    Fields map[string]string // raw key=value from #!excsv
    // Resolved:
    Version, Delim, Quote, Null, Encoding, Schema, SQLDialect string
    HeaderRow bool            // header=1
    Rows      *int            // optional declared count
    Checksum  *Checksum       // algorithm + hex
    OriginalSize *int64       // required when Form is zip inner
}

type MetaBlock struct {
    FileMeta   []KV            // #@ — last-wins map materialised from slice
    Columns    []ColumnDef
    Aggregations []Aggregation // name + positional values
    SQL        []SQLStatement  // ordered; verb, dialect, version, payload
    CSVW       *string
    HumanComments []string     // ## — dropped unless PreserveHumanComments
}
```

Parsing follows the algorithm in README-LLM § PARSING ALGORITHM. Serialization follows § SERIALIZATION ALGORITHM.

### 3.1 Open dispatch (A1, J6)

```
Open(path | stdin | []byte) → Document
  1. If magic PK\x03\x04 OR extension .excsv.zip / .ecsv.zip → zip.Open
  2. Else → parse as plain ExCSV
  3. Extension is authoritative when path is known; magic sniff is optional convenience
```

For **stdin zip** (A2, J6): buffer to a seekable temp file (or memory if small), emit warning on stderr in lenient mode. Plain stdin streams line-at-a-time.

### 3.2 Parse modes (A4, A5)

```go
type ParseOptions struct {
    Strict bool
    PreserveHumanComments bool // A7 opt-in for ##
}
```

- **Strict:** MUST-fail conditions → return `*ParseError` with `ErrorKind` matching fixture enum.
- **Lenient:** same errors become warnings where spec says SHOULD warn; continue if document is still structurally usable.

Shared **error kinds** (must match `fixtures.yaml` header): `header_missing_version`, `zip_original_size_mismatch`, etc.

### 3.3 Operating modes (feature catalog)

| Mode | Data scan | Typical commands |
| --- | --- | --- |
| **A — metadata-only** | No | `header get`, `meta set`, `column add`, `sql list`, `zip peek` |
| **B — data-aware** | Yes | `rows count`, `agg compute`, `checksum verify`, `convert`, transforms |

Mode B writes auto-sync derived fields when configured (P6): `rows=`, `checksum=`, `#%`, `#@exported`.

---

## 4. Library packages — responsibilities

### 4.1 `pkg/excsv` — core

| Area | Spec section | Features |
| --- | --- | --- |
| Header parser | HEADER LINE | B1–B6 |
| Meta parsers | META LINES, SQL SECTION, COLUMN SCHEMA, AGGREGATIONS | C*, D*, E*, F* |
| CSV engine | DATA SECTION, DELIMITERS, QUOTING | G6, dialect for #% |
| Checksum | CHECKSUM | M2, M3 |
| Validate | ERROR HANDLING | M1 |
| Canonical writer | SERIALIZATION ALGORITHM | A6, A7, O1–O5 |
| Stream API | — | A8, A9, A10 |

**CSV engine:** use Go 1.x `encoding/csv` only when dialect matches its model; for `quote=none`, custom `#` delimiter, and `quote=#`, implement a small dialect-aware splitter/encoder aligned with spec (quoted fields single-line, no raw newlines).

**Forward compatibility (P7, P8):** unknown header keys and unknown `#` lines → ignore. Reserved pack keys (`layout`, `mode`, `section-size`, `table-count`) and meta prefixes (`#table`, `#fk`) → ignore on read, never write.

### 4.2 `pkg/excsv/zip` — row container (J1–J6)

| Function | Feature |
| --- | --- |
| `OpenArchive(path)` | J6 transparent open |
| `LocatePrimary(cd)` | First `.excsv`/`.ecsv` entry; name = archive base or `data.excsv` |
| `ExtractPrimary()` | Full read path |
| `PeekComment()` | J3 — parse EOCD comment as ExCSV prefix |
| `Wrap(inner []byte, opts)` | J1 — two-pass `original-size`, Deflate default |
| `Unwrap(archive)` | J2 |
| `RefreshComment(doc)` | J4 |
| `VerifyOriginalSize()` | J5 |

**ZIP writer requirements:** deterministic output for fixture generation (fixed `#@created`, stable ordering, no extra fields). Support Deflate (8), Store (0), BZIP2 (12) read; write Deflate by default.

**Comment builder:** priority list from spec; truncate at line boundary; append `#@comment-truncated: 1` when truncated. Max 65535 bytes UTF-8.

**Reject:** encrypted entries (`zip_encrypted`), unsupported compression (`zip_unsupported_compression`), missing/bad primary (`zip_primary_*`).

---

## 5. CLI design

Framework: **cobra** + **pflag**. Binary name: `excsv`.

### 5.1 Global flags

| Flag | Purpose |
| --- | --- |
| `--strict` / `--lenient` | Parse mode (default: strict) |
| `--json` | Machine-readable output (P3) |
| `--in-place` | Atomic rewrite via temp + rename (P1) |
| `-` | stdin / stdout |
| `--preserve-human-comments` | A7 round-trip for `##` |

Exit codes: `0` ok; `1` user error; `2` parse/validation failure; `3` I/O failure.

### 5.2 Command tree (RF plain + zip)

Commands grouped by user goal. Pack-only branches omitted.

```
excsv
├── open / cat              # debug: dump inner plain (unwrap zip transparently)
├── validate                # M1 full conformance
├── info / summary          # N1 compact summary
├── header                  # B*
│   ├── list
│   ├── get KEY
│   ├── set KEY=VAL
│   └── unset KEY
├── meta (@)                # C*
│   ├── list
│   ├── get KEY
│   ├── set KEY: VAL
│   ├── unset KEY
│   └── import FILE
├── column                  # D*
│   ├── list
│   ├── show NAME|INDEX
│   ├── add ...
│   ├── remove ...
│   ├── rename ...
│   └── reorder ...
├── agg (%)                 # E*
│   ├── list
│   ├── get NAME
│   ├── set ...
│   ├── compute             # Mode B
│   ├── verify              # Mode B
│   └── clear
├── sql ($)                 # F*
│   ├── list [--verb ddl|dql] [--dialect D]
│   ├── get N
│   ├── append ...
│   ├── ddl generate [--dialect D]
│   └── ddl apply --dialect D   # emit ordered SQL to stdout (no DB execution)
├── rows                    # G*
│   ├── count
│   ├── head N
│   ├── tail N
│   └── slice FROM:TO
├── data
│   ├── print               # G6 iterate rows
│   └── get ROW COL         # G5 cell read
├── convert                 # I*
│   ├── from-csv
│   ├── to-csv
│   ├── to-tsv
│   └── normalize           # I3 dialect/encoding/null
├── zip                     # J*
│   ├── wrap IN -o OUT.excsv.zip
│   ├── unwrap IN.zip -o OUT.excsv
│   ├── peek                  # comment-only
│   └── refresh-comment
├── checksum                # M2, M3
│   ├── compute
│   └── verify
├── freeze                  # M6 one-shot finalize
├── diff                    # M7, M8
├── tidy                    # O8 repair + canonical sort
└── version
```

**Output family rule:** transforms default to same form as input (plain → plain, zip → zip) unless `--output` / `-o` specifies otherwise. Wrapping plain → zip is explicit (`excsv zip wrap` or `convert --zip`).

**SQL execution:** out of scope. `sql ddl apply` prints statements; user pipes to `psql`, `mysql`, etc.

### 5.3 Command → feature mapping (in-scope only)

| Group | Feature IDs | Wave |
| --- | --- | --- |
| Core parse/serialize | A1–A7, A10 | 1 plain; zip adds A1≈, A10≈ |
| Streaming | A8, A9 | 1 (plain); 2 (zip inner stream) |
| Header | B1–B6 | 1 |
| Meta `#@` | C1–C7 | 1 |
| Columns | D1–D6 | 1 |
| Aggregations | E1–E7 | 1–2 |
| SQL `#$` | F1–F8 | 1 |
| Data read | G1, G4, G6 | 1–2 |
| Data transform | H1–H13 | later milestones (Mode B) |
| Convert | I1–I3, I10 | 1–2 |
| Zip container | J1–J6 | 2 |
| Integrity | M1–M6, M8 | 1–2 |
| Inspect | N1–N5 | 1–2 |
| Cleanup | O1–O8 | 1–2 |
| Cross-cutting | P1–P8 | throughout |

Deferred within RF: G2–G3, G5, G7 (optimised column/row access — functional via full scan first). H* transforms ship after core parse is green.

---

## 6. Implementation phases

Work in two waves per upstream plan. **Do not start wave 2 until wave 1 fixture subset is green.**

### Phase 0 — scaffold

- [ ] `go.mod`, `cmd/excsv/main.go`, cobra root
- [ ] `pkg/excsv` types, error kinds enum mirroring YAML
- [ ] `internal/fixtures` — load manifest, resolve paths to upstream `fixtures/` tree
- [ ] CI: `go test ./...`, fixture runner skeleton
- [ ] Sync fixture binary files from upstream (junction/submodule/copy into `test/fixtures/files/` or read from cloned sibling)

### Phase 1 — plain row (wave 1)

**Milestone 1.1 — parse only**

Implement parsing algorithm steps 1–6 for plain files:

- Header line + defaults (B1, B5, B6)
- All meta kinds (#@, #column, #%, #$, #csvw, ## ignore)
- Data section with dialect resolution
- Strict/lenient + error kinds for plain invalid fixtures

Deliverable: `TestFixtures_Plain_Parse` walks manifest `plain/valid/*` and `plain/invalid/*`.

**Milestone 1.2 — serialize + round-trip**

- Canonical serialization (A6)
- Round-trip tests on valid plain fixtures (A7)
- BOM strip, CRLF acceptance, LF output (O1)

**Milestone 1.3 — validation & integrity**

- Checksum compute/verify (M2, M3)
- Aggregation arity validation (E1)
- Column/header consistency (D6)
- `excsv validate` command

**Milestone 1.4 — Mode A CLI**

- `header`, `meta`, `column`, `sql list/get/append`
- Stream-passthrough for metadata edits (A10)
- Atomic in-place writes (P1)

**Milestone 1.5 — Mode B read + convert**

- `rows count`, `data print`, `convert to-csv` / `from-csv`
- Aggregation compute/verify (E3, E4)
- `checksum compute`, `freeze`

Target: all **36** plain valid + **26** plain invalid fixtures green.

### Phase 2 — row zip (wave 2)

**Milestone 2.1 — zip read**

- Primary entry location rules
- Extract + parse inner document
- `original-size` vs central directory (J5)
- Comment peek (J3) — advisory parse
- Invalid zip fixtures green

**Milestone 2.2 — zip write**

- Wrap plain → zip (J1): two-pass header patch
- Comment builder with truncation
- Deterministic generator matching upstream `make_zip.ps1`

**Milestone 2.3 — zip CLI**

- `zip wrap`, `zip unwrap`, `zip peek`, `zip refresh-comment`
- Transparent open on `.excsv.zip` paths (J6)
- Stdin zip via temp buffer (A2)

**Milestone 2.4 — zip + Mode B**

- Checksum survives re-wrap
- `convert` and `freeze` preserve or target zip per flags

Target: all **10** zip valid + **9** zip invalid fixtures green.

### Phase 3 — polish (still RF only)

- Streaming row reader/writer (A8, A9) for large plain files
- `diff`, `tidy`, `info --json`
- H1–H4 basic transforms (filter, sort, select) — plain first, then zip
- Performance: avoid full materialisation for Mode A zip peek

---

## 7. Testing strategy

### 7.1 Fixture-driven tests

Primary integration test:

```go
func TestManifestFixtures(t *testing.T) {
    manifest := fixtures.Load("test/fixtures/fixtures.yaml")
    root := fixtures.ResolveRoot() // upstream fixtures/ directory
    for _, fx := range manifest.Fixtures {
        if !strings.HasPrefix(fx.ID, "plain/") && !strings.HasPrefix(fx.ID, "zip/") {
            continue // skip pack
        }
        t.Run(fx.ID, func(t *testing.T) {
            doc, result := excsv.ParseFile(filepath.Join(root, fx.ID), excsv.Strict)
            fixtures.AssertExpectation(t, fx, result, doc)
        })
    }
}
```

- Filter manifest: **only** `plain/*` and `zip/*` IDs.
- Compare `expect.parse`, `expect.error_kind`, `expect.warnings`, spot-check fields (`header`, `meta`, `sql`, `comment`).
- Negative tests: assert error kind matches exactly — not merely “an error occurred”.

### 7.2 Unit tests

| Package | Focus |
| --- | --- |
| `header` | key=value splitting, quoting, defaults |
| `meta` | #% CSV split uses file delimiter; #$ key parsing |
| `sql` | dialect resolution (F7), family match warnings |
| `csv` | quote=none, quote=#, doubled quotes |
| `checksum` | trailing newline sensitivity (fixtures 035, 036) |
| `zip` | primary naming, comment truncation, compression methods |

### 7.3 Golden / canonical bytes

For selected valid fixtures (e.g. `001_minimal_header_only`), optional `*.canonical` sibling files — byte-exact output of canonical writer. Add when writer stabilises.

### 7.4 Fixture file sync

Manifest is local; binary fixtures live upstream. Options:

1. **Git submodule** — `test/fixtures/upstream` → `boligolov/excsv/fixtures`
2. **Junction** — Windows `mklink /J test\fixtures\files <path-to-upstream/fixtures>`
3. **CI checkout** — sparse clone fixtures path in pipeline

Document chosen approach in repo README when wired.

---

## 8. Error and warning model

```go
type ErrorKind string

const (
    ErrHeaderMissingVersion ErrorKind = "header_missing_version"
    // ... mirror fixtures.yaml error_kinds exactly
)

type Issue struct {
    Kind    ErrorKind // or WarningKind
    Message string
    Line    int       // 1-based, 0 if unknown
}

type ParseResult struct {
    Doc      *Document
    Errors   []Issue
    Warnings []Issue
}
```

Strict mode: first MUST-class issue → return error, no partial doc (or return doc + error — pick one convention and keep consistent; lean: fail fast, no doc).

Lenient mode: populate `Warnings`, return doc if parse completed.

---

## 9. I/O and concurrency

| Concern | Approach |
| --- | --- |
| Atomic in-place (P1) | Write `path.excsv.tmp`, fsync, `rename` over original |
| Zip in-place | Rewrite archive to temp; rename — never partial central directory |
| Stdin plain | Buffered line reader; stream rows for `data print` |
| Stdin zip | Temp file; warn once |
| `--json` | Structured schema: `{ "ok": true, "header": {...}, "warnings": [] }` |
| Encoding (P5) | Read declared encoding; UTF-8 default; re-emit UTF-8 |

No server mode, no background daemon.

---

## 10. Dependencies (initial)

| Module | Use |
| --- | --- |
| `github.com/spf13/cobra` | CLI |
| `gopkg.in/yaml.v3` | Fixture manifest |
| `archive/zip` | stdlib — row container; evaluate if ZIP64 edge cases need `github.com/klauspost/compress/zip` |
| `crypto/sha256` | Checksum |
| `golang.org/x/text` | Encoding transforms (non-UTF-8 fixtures) |

Avoid heavy DB or query engines. Keep stdlib-first.

---

## 11. Explicit non-goals

| Item | Reason |
| --- | --- |
| `.excsv.pack.zip` | Wave 3+; reserved names only (P8) |
| `#table`, `#fk` meta | Pack manifest |
| Embedded SQL execution | Catalog § “NOT in this catalog” |
| Encryption | Not in v0.2 |
| Plugin protocol | Future document |
| Automatic pack detection | CLI surface for packs will be explicit later (`excsv pack ...`) |

If a user opens a `.excsv.pack.zip`: treat as ZIP, fail gracefully (“pack format not supported”) rather than mis-parsing as row zip.

---

## 12. Success criteria

| Wave | Done when |
| --- | --- |
| **1 — plain** | All `plain/valid/*` + `plain/invalid/*` fixtures pass; `excsv validate`, `convert`, Mode A meta/column/sql commands work on plain files |
| **2 — zip** | All `zip/valid/*` + `zip/invalid/*` fixtures pass; wrap/unwrap/peek/refresh; transparent open; stdin zip with temp fallback |
| **RF complete** | Waves 1–2 green; README with install + cookbook pointers; library importable as `pkg/excsv` |

---

## 13. Suggested first PR sequence

1. Module scaffold + error kinds + header/meta parsers + plain valid 001–009
2. Complete plain parse (all valid/invalid fixtures)
3. Canonical writer + round-trip
4. `validate`, `header`, `meta`, `column`, `sql list`
5. Checksum + aggregation validation
6. `convert`, `rows`, `data print`
7. `pkg/excsv/zip` read + zip invalid fixtures
8. Zip write + generator + zip valid fixtures
9. Zip CLI commands + transparent open
10. `freeze`, `tidy`, `diff`, streaming optimisations

Each PR should cite feature IDs from the manifest fixtures it enables.

---

## 14. References

| Document | Role |
| --- | --- |
| [`README-LLM.md`](downloaded/README-LLM.md) | Normative format |
| [`plan-README.md`](downloaded/plan-README.md) | Wave gating, spec-first rule |
| [`plan-01-features.md`](downloaded/plan-01-features.md) | Feature IDs A–P |
| [`plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) | Fixture layout, manifest schema |
| [`fixtures.yaml`](../test/fixtures/fixtures.yaml) | Expected outcomes (plain + zip entries) |
| [`sources_and_specifications.md`](sources_and_specifications.md) | Upstream links + refresh policy |
