# excsv CLI — command map

**Status:** snapshot of **implemented** behavior (`internal/cli`, v0.2). Use this doc to review, add, or drop commands before changing code.

**Binary:** `excsv` (built from `cmd/excsv`)

**Invocation pattern**

```text
excsv [--global flags] FILE <command> ...
excsv version
```

`FILE` is always the **first positional argument** (after global flags). It is not repeated on subcommands.

---

## Conventions

- `FILE` — plain ExCSV (`.excsv`, `.ecsv`, `.extsv`), row ZIP (`.excsv.zip`, `.ecsv.zip`), or data sibling (`.csv` / `.tsv` with sidecar discovery).
- **No stdin** — every document command requires `FILE`.
- **Sidecar discovery:** `excsv sales.csv …` discovers `sales.excsv` / `.ecsv` / `.extsv` in the same directory; writes from `meta set` / `sql set` go to the sidecar.
- **`meta set` / `sql set`:** one positional `KEY`, required `--value` (single shell string; use `'…'` or `"…"` for spaces).
- **No `header set`** — `#!excsv` fields affect encoding/checksum/rows; change via `convert` / re-import, not in-place set.
- **Exit codes:** `0` ok · `1` user/usage · `2` parse/spec (`*excsv.ParseError`) · `3` I/O or other

---

## Global flags (all commands)

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `true` | Fail on spec violations |
| `--lenient` | `false` | Collect warnings and continue |
| `--json` | `false` | Machine-readable stdout where supported |
| `--clean-human-comments` | `false` | Drop `##` on read/rewrite |
| `--expect-profile` | `""` | Fixture/testing: `stub`, `sidecar`, `sidecar_strict` |

Flags may appear before or after `FILE` (e.g. `excsv --json sales.excsv info`).

---

## Command tree

```
excsv [--flags] FILE
├── validate
├── info
├── cat
├── strip
├── convert          # FILE = CSV/TSV input
├── wrap [-o OUT]    # FILE = plain .excsv / .ecsv
├── unwrap [-o OUT]  # FILE = .excsv.zip / .ecsv.zip
├── rows             # alias: header get rows
├── header
│   ├── list
│   └── get [KEY]
├── meta
│   ├── list
│   ├── get [KEY]
│   └── set KEY --value VAL
└── sql
    ├── list [--verb] [--dialect]
    ├── get [KEY] [--verb] [--dialect]
    └── set KEY --value VAL

excsv version
```

---

## Document commands

### `excsv FILE validate`

| | |
|---|---|
| **Stdout** | `ok` or JSON `{"ok":true,"path":"..."}` |
| **Notes** | Full parse + sidecar + `reference=`; ZIP decompresses inner |

### `excsv FILE info`

| | |
|---|---|
| **Stdout (text)** | `ExCSV <version> rows=N columns=M form=plain\|zip profile=…` |
| **Stdout (JSON)** | `version`, `delim`, `quote`, `rows`, `columns`, `form`, `profile`, optional `reference`, `reference_path` |

### `excsv FILE cat`

| | |
|---|---|
| **Stdout** | canonical inner ExCSV bytes |
| **Notes** | ZIP → decompress inner; sidecar profile omits data in output |

### `excsv FILE strip`

| | |
|---|---|
| **Stdout** | plain CSV/TSV data rows |
| **Stderr** | sidecar-only notice when opening sidecar path directly (no stdout, exit `0`) |

### `excsv FILE convert`

Import CSV/TSV → ExCSV. **`FILE` is the delimited source**, not an existing ExCSV document.

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | output path (stdout if omitted; sidecar default `<stem>.excsv` / `.extsv`) |
| `--delim` | | output delimiter |
| `--quote` | | output quoting |
| `--sidecar` | | metadata-only sidecar; `reference=` → `FILE` |
| `--reference` | | override sidecar `reference=` |
| `--no-header` | | `header=0` |
| `--columns` | | emit `#column` from header row |
| `--checksum` | | `checksum=sha256:…` |
| `--meta` | | repeatable `#@` as `KEY:VAL` |
| `--zip` | | wrap as `.excsv.zip` (not with `--sidecar`) |

### `excsv FILE wrap` / `unwrap`

| Command | `FILE` | `-o` default |
|---------|--------|----------------|
| `wrap` | plain `.excsv` / `.ecsv` | `<stem>.excsv.zip` |
| `unwrap` | `.excsv.zip` / `.ecsv.zip` | basename without `.zip` |

---

## `header` — `#!excsv` (read-only)

### `excsv FILE header list`

All `key=value` fields (text or JSON).

### `excsv FILE header get [KEY]`

Single value, or list if `KEY` omitted.

**Exit 1** if `KEY` unknown.

---

## `meta` — `#@`

### `excsv FILE meta list` / `get [KEY]`

Same pattern as `header` (`key: value` text; JSON object on list).

### `excsv FILE meta set KEY --value VALUE`

| | |
|---|---|
| **Writes** | upsert `#@KEY: VALUE` in place |
| **Stdout** | `ok` or JSON `{"ok":true,"path":"…","key":"…"}` |

---

## `rows`

### `excsv FILE rows`

Prints `rows=` from `#!excsv` (same as `header get rows`).

---

## `sql` — `#$`

### `excsv FILE sql list`

Flags: `--verb` (`ddl` \| `dql`), `--dialect`.

### `excsv FILE sql get [KEY]`

SQL payload; list if `KEY` omitted. **Exit 1** if unknown/ambiguous.

### `excsv FILE sql set KEY --value VALUE`

| | |
|---|---|
| **KEY** | raw `#$` key (`ddl`, `ddl-mysql`, …) |
| **VALUE** | single-line SQL (shell-quoted) |
| **Writes** | update or append `#$KEY: VALUE` |

---

## `version`

```text
excsv version
```

No `FILE`. Prints `excsv-cli <version> (built <time>)`.

---

## Input forms

| Input | Behavior |
|-------|----------|
| `.excsv` / `.ecsv` / `.extsv` | Parse as ExCSV |
| `.csv` / `.tsv` + sidecar | Discover sidecar; merge for data commands |
| `.excsv.zip` / `.ecsv.zip` | See class below |
| `.excsv.pack.zip` | Not supported |

### Command classes vs row ZIP

| Class | Commands | ZIP |
|-------|----------|-----|
| **Metadata read** | `info`, `header`, `meta`, `sql`, `rows` | comment only |
| **Metadata write** | `meta set`, `sql set` | decompress → edit → re-wrap |
| **Data read** | `validate`, `strip`, `cat` | decompress inner |

---

## Not implemented

| Idea | Notes |
|------|--------|
| `header set` | Use `convert` / rewrite; header fields are structural |
| stdin / `-` | file paths only |
| `zip` subcommand group | use `FILE wrap` / `FILE unwrap` |
| `--value-file` | optional future; use `--value` with quotes today |

---

## Quick reference

```text
excsv [--strict|--lenient] [--json] [--clean-human-comments] [--expect-profile=...] FILE <command>

excsv FILE validate
excsv FILE info
excsv FILE cat
excsv FILE strip
excsv data.csv convert [-o PATH] [--delim] [--quote] [--sidecar] [--reference] [--no-header] [--columns] [--checksum] [--meta KEY:VAL]... [--zip]
excsv FILE wrap [-o OUT.zip]
excsv FILE.zip unwrap [-o OUT.excsv]
excsv version

excsv FILE header list
excsv FILE header get [KEY]

excsv FILE meta list
excsv FILE meta get [KEY]
excsv FILE meta set KEY --value "VALUE"

excsv FILE rows

excsv FILE sql list [--verb ddl|dql] [--dialect DIALECT]
excsv FILE sql get [KEY] [--verb] [--dialect]
excsv FILE sql set KEY --value "SQL PAYLOAD"
```
