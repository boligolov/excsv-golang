# excsv CLI — command map (grouped)

**Status:** **shipped** in **excsv-cli 0.0.3** (`internal/cli`). This document is the
authoritative map of the grouped layout. Summary of what changed vs the pre-0.0.2 CLI:

1. **Grouping** — flat top-level commands replaced with whole-document verbs plus noun
   groups (`data`, `header`, `meta`, `column`, `agg`, `sql`, `comment`, `export`,
   `pack`, `zip`). Every meta line the format defines has exactly one owning group.
2. **Deduplication** — `verify`, `column check`, `freeze`, and `tidy` collapsed into
   `validate` (reporter) and `fix` (repairer); `strip` merged into `data print`; `cat`
   deleted.
3. **Import / re-encode** — `convert` describes the source in one invocation
   (`--format`, enrichment flags) and accepts ExCSV as input for re-encoding.
4. **Read-only `header`** — `header set` / `header remove` deleted; each `#!excsv`
   field is written by its owner (`convert`, `fix`, `sql dialect set`, …).
5. **Boundary exports** — CSVW removed from the ExCSV v0.4 document; survives only
   as `export csvw` (write-only). The normative JSON form ships as `export json`.
6. **Rich `info`** — multi-line document summary plus `info header` for `#column`
   schema (distinct from the read-only `header` group for `#!excsv`).

**Binary:** `excsv` (built from `cmd/excsv`)

**Invocation pattern**

```text
excsv [--global flags] FILE <group> <command> ...
excsv version
```

`FILE` is always the **first positional argument** (after global flags). It is not
repeated on subcommands.

---

## Design rules

- **Noun groups own their meta-line.** Each ExCSV meta line has exactly one group:
  `#!excsv` → `header`, `#@` → `meta`, `#column` → `column`, `#%` → `agg`,
  `#$` → `sql`, `##` → `comment`. The data section → `data`. The pack container
  (`#table`, `#fk`, multi-table zip) → `pack`. Row ZIP framing → `zip`.
- **Foreign formats live at the boundary, never inside the document.** We do not
  store CSV inside an ExCSV file; we convert it. CSVW gets the same treatment, minus
  the import half: written at export, never embedded, never read. A format that
  carries a second format's schema needs a field to say which one wins, and that
  field is the complexity — not the compatibility.
- **Export a foreign format, don't import one.** Writing to a standard is a
  serializer with one right answer. Reading one is a policy about whose declaration
  beats the data, and we would rather make the user state that policy explicitly
  (`--column-attr`) than pick a default that mislabels a column quietly.
- **Whole-document lifecycle stays flat.** `info`, `validate`, `fix`, `convert`,
  `version` act on the whole file (or create it) and are the commands you reach for
  most, so they stay top-level rather than hiding behind a `doc` group.
- **One reporter, one repairer.** `validate` never writes; `fix` never reports
  conformance. They share the same scoping flags (`--column`, `--table`) so a
  finding maps directly onto the command that repairs it.
- **One way to print the data.** `strip` and `print` differed only in flags, so they
  collapse into `data print`; with no flags it dumps the whole data section.
- **Don't reimplement the shell.** `cat` is gone: printing a plain file whole is the
  operating system's job (`cat` / `type`). The CLI should only own what the shell
  cannot reach.
- **Batch flags delegate, never duplicate.** `convert`'s enrichment flags
  (`--meta`, `--comment`, `--agg`, `--sql`, `--column-attr`) must call the same
  functions as the corresponding group commands. A convenience flag that grows its
  own parsing and validation is a duplicate with extra steps.
- **Facts by default, choices on request.** A command may emit what it can *derive*
  from the input (dialect, row count, column types, checksum) without being asked,
  but never what the author has to *decide* (aggregations, SQL, comments).
- **Declare, don't imply.** When we write a document, dialect fields are spelled out
  even when they match the spec default — `quote=none` included. Relying on a default
  saves a few bytes and costs the reader a lookup, which is the opposite of what this
  format is for.
- **A header field is written by whoever owns what it governs.** `#!excsv` is not one
  thing, it is a bag of pointers into the rest of the document, so it has no single
  owner. `delim`/`quote`/`header`/`encoding`/`null` govern the data section →
  `convert`. `rows`/`checksum` are computed from it → `fix`. `sql-dialect` governs
  `#$` → `sql`. `reference` and the container fields belong to the shape that
  requires them. The consequence is what
  matters: **`header` itself has no write verbs**, so no command can change a field
  whose blast radius it does not understand.
- **Consistent verbs inside groups:** `list`, `get`, `set`/`add`, `remove` — except
  `header`, per the rule above.

---

## Conventions

- `FILE` — plain ExCSV (`.excsv`, `.ecsv`, `.extsv`), row ZIP (`.excsv.zip`,
  `.ecsv.zip`), pack (`.excsv.pack.zip`), or data sibling (`.csv` / `.tsv` with
  sidecar discovery).
- **No stdin** — every document command requires `FILE`.
- **Sidecar discovery:** `excsv sales.csv …` discovers `sales.excsv` / `.ecsv` /
  `.extsv` in the same directory; in-place writes go to the sidecar.
- **Pack scope:** `--table NAME` selects one table inside a pack; multi-table packs
  require it for table-scoped commands.
- **`… set` commands:** one positional `KEY`/`NAME`, required `--value` (single
  shell string; quote spaces). A `set` never re-encodes the file — if changing a
  field would rewrite the data section, it belongs to `convert` instead.
- **Exit codes:** `0` ok · `1` user/usage · `2` findings or spec violation ·
  `3` I/O or other.

---

## Global flags (all commands)

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `true` | Findings (including warnings) make the command exit `2` |
| `--lenient` | `false` | Print the same findings but exit `0` |
| `--json` | `false` | Machine-readable stdout where supported |
| `--clean-human-comments` | `false` | Drop `##` on read/rewrite |
| `--expect-profile` | `""` | Fixture/testing: `stub`, `sidecar`, `sidecar_strict` |
| `--zip-password` | `""` | Password for encrypted ZIP or pack |
| `--table` | `""` | Pack table name to scope table-level commands |

Flags may appear before or after `FILE` (e.g. `excsv --json sales.excsv info`).

---

## Command tree

```
excsv [--flags] FILE
│
├── info                     # document summary (default) or column schema view
│   [--no-meta]              #   omit #@ file metadata from the default summary
│   └── header               #   #column schema: names row + one attribute line per column
│
├── validate                 # read-only conformance report; never writes
│   [--with-data]            #   also scan the data section (see levels below)
│   [--schema-only]          #   explicit default; conflicts with --with-data
│   [--column NAME|IDX]...   #   narrow the data scan to these columns (implies --with-data)
│
├── fix                      # repair derived metadata; writes in place  (was: freeze + tidy)
│   [--only LIST]            #   format,columns,agg,checksum,rows,stamp (default: all)
│   [--column NAME|IDX]...   #   narrow to these columns
│   [--dry-run]              #   report what would change, write nothing
│
├── convert                  # FILE = CSV/TSV source → ExCSV. The import entry point.
│   [--format SHAPE]         #   inline | sidecar | zip | pack   (default: inline)
│   [-o OUT]                 #   output path; default depends on --format
│   [--delim D] [--quote Q]  #   override the detected output dialect
│   [--no-header]            #   every row is data → header=0
│   [--encoding ENC]         #   output encoding=
│   [--null STR]             #   null= token; REWRITES null cells
│   [--no-checksum]          #   skip checksum= (emitted by default)
│   [--reference PATH]       #   sidecar reference= target      (--format sidecar)
│   [--table NAME]           #   pack table name                (--format pack)
│   [--agg LIST]             #   add #% aggregations            (default: none)
│   [--meta KEY:VAL]...      #   add #@ lines
│   [--comment TEXT]...      #   add ## lines
│   [--sql KEY:SQL]...       #   add #$ lines
│   [--column-attr C.A=V]... #   extra #column attributes
│
├── data                     # data section only — never emits metadata
│   ├── print                # the data section as CSV/TSV     (was: strip + print)
│   │   [-o OUT]             #   write to a file instead of stdout
│   │   [--limit N]          #   max body rows (0 = all)
│   │   [--offset N]         #   skip this many body rows
│   │   [--select COLS]      #   project columns by name/index (was: --columns)
│   ├── get ROW [COLUMN]     # one row (0-based) or a single cell, raw value
│   ├── append [--row --file --skip-header]  # append rows
│   └── sort --by KEY[:asc|:desc]...         # sort body rows
│
├── header                   # #!excsv — READ-ONLY group
│   ├── list
│   ├── get [KEY]
│   └── rows                 # the row count as a bare number and nothing else
│
├── meta                     # #@ file metadata
│   ├── list
│   ├── get [KEY]
│   ├── set KEY --value VAL
│   └── remove KEY
│
├── column (alias: col)      # #column schema
│   ├── list
│   ├── get [NAME]
│   ├── set NAME --attr k=v...
│   ├── remove NAME          # (check → validate --with-data --column NAME)
│   ├── materialize NAME [-o OUT]     # write formula= output into the data, set materialized=1
│   └── dematerialize NAME [-o OUT]   # drop cached values, keep formula= (v0.5 computed columns)
│
├── agg                      # #% aggregations
│   ├── list
│   ├── get [NAME]
│   ├── add NAME             # compute + append (no-op if exists)
│   ├── update NAME          # recompute + replace (create if missing)
│   └── remove NAME
│
├── sql                      # #$ SQL companion
│   ├── list [--verb ddl|dql] [--dialect D]
│   ├── get [KEY] [--verb] [--dialect]
│   ├── set KEY --value SQL
│   ├── remove KEY
│   └── dialect              # sql-dialect= — the default for unsuffixed #$ lines
│       ├── get              #   bare token, or `ansi` when unset
│       ├── set D
│       └── remove
│
├── comment                  # ## human comments
│   ├── list
│   ├── add --value TEXT
│   └── remove INDEX
│
├── export                   # ExCSV → a foreign representation; never writes FILE
│   ├── json [-o OUT]        # JSON form (.excsv.json); see implementation/json.md
│   └── csvw [-o OUT]        # a CSVW metadata sidecar (write-only; no reader)
│       [--url PATH]         #   the CSV the metadata describes (required if inline)
│       [--enum-as-pattern]  #   encode enum= as a datatype.format regex
│
├── pack                     # pack container (.excsv.pack.zip)
│   ├── create [-o OUT] [--name N]   # single-table pack from a plain .excsv
│   ├── table
│   │   ├── list             # list tables                    (was: table list)
│   │   ├── add NAME --from FILE      # add table             (was: table add)
│   │   ├── drop NAME                 # drop table            (was: table drop)
│   │   └── extract NAME [-o OUT]     # extract to plain .excsv (was: table extract)
│   └── fk
│       └── list             # list #fk lines                 (was: fk list)
│
└── zip                      # row ZIP container (.excsv.zip / .ecsv.zip)
    ├── wrap [-o OUT] [--password P]
    ├── unwrap [-o OUT] [--password P]
    └── password
        ├── set --password NEW [--current-password CUR]
        └── remove [--password CUR]

excsv version                # no FILE; prints excsv-cli <version> (built <time>)
```

---

## `info` — document summary and column header view

Read-only. Never writes `FILE`. Reads the meta section only (no data scan, no ZIP/pack
body decompression) except where noted.

**Two surfaces, one command:**

| Invocation | Shows |
|------------|-------|
| `excsv FILE info` | Multi-line document summary (default). |
| `excsv FILE info header` | Column schema from `#column` lines only. |

This is **not** the `header` group. That group owns `#!excsv` (`header list`,
`header get`, `header rows`). `info header` owns the **logical table header** —
the `#column` declarations — and prints them in a human-oriented layout.

### Default summary (`info`)

```text
ExCSV 0.4
Rows: 3
Columns: 3
Form: plain
Profile: inline
Delimiter: comma
Quote: none (fields not quoted)
Null: (empty string)
SQL dialect: mysql              # only when sql-dialect= is set
Reference: sales.csv            # sidecar only, when present
Aggregations: sum, count        # names only, never #% values
SQL (2): ddl, dql               # count + #$ keys only, never payloads
author: author@example.com      # every #@ entry (key: value)
license: CC-BY-4.0
```

**Dialect lines** spell out what the data section uses:

- **Delimiter** — `delim=` token (`comma`, `tab`, `pipe`, …).
- **Quote** — `double` / `single`, or `none (fields not quoted)` when quoting is off.
- **Null** — `(empty string)` when `null=` is absent; otherwise the literal token
  (`NA`, `\N`, …).
- **SQL dialect** — omitted when unset; unsuffixed `#$` lines fall back to `ansi`.

**Aggregations and SQL** list *what exists*, not what it contains. For values use
`agg list` / `agg get` and `sql list` / `sql get`.

**`--no-meta`** suppresses every `#@` line from the summary. Aggregations, SQL keys,
and dialect fields are unaffected.

**Pack without `--table`:** summary line shows the manifest (`Form: pack`, table
names, foreign-key count). Manifest-level `#@` / `#$` on the pack document appear
when present; per-table aggregations and SQL require `--table`.

Global `--json` emits a structured object with the same fields (`aggregations`,
`sql.count` / `sql.keys`, `meta`, …).

### Column header view (`info header`)

Reads `#column` only — no data section.

```text
id,customer,amount
name: id, type: int, unique: 1
name: customer, type: string, required: 1, len_max: 100
name: amount, type: decimal, unit: USD, min: 0
```

- **Line 1** — comma-separated `name=` values in `#column` order.
- **Following lines** — one column each; attributes in spec display order (`name`,
  `title`, `type`, …). Values with spaces are quoted: `title: "Order ID"`.

For raw `#column` lines use `column list`. Multi-table packs require `--table`.

Global `--json` emits `{"header": ["id", …], "columns": [{…attrs…}, …]}`.

---

## `validate` — the single reporter

Replaces `validate` + `verify` + `column check`.

**Contract:** runs every check to completion, prints the **full** list of findings,
then exits. It never stops at the first problem and it never writes to the file.

### Levels

| Level | Reads | Checks |
|-------|-------|--------|
| `--schema-only` (default) | header + meta lines only; no data scan, no ZIP/pack body decompression | the *declarations* themselves: `#!excsv`, `#column`, `#@`, `#$` |
| `--with-data` | full data section | everything above **plus** every cell, `rows=`, `checksum=`, recomputed `#%` |
| `--column NAME\|IDX` | full data section, one column | narrows the data scan; repeatable; **implies `--with-data`** |

These map onto the upstream plan's *Mode A (metadata-only)* / *Mode B (data-aware)*
split in `docs/downloaded/plan-01-features.md`.

**Flag conflict:** passing `--schema-only` together with `--with-data` prints a
warning on stderr and proceeds with `--with-data` (the wider level wins).

**Cost warning:** `--with-data` prints a one-line stderr notice before scanning when
the document is large (row count over ~100k) or requires ZIP/pack decompression —
**only when stderr is a terminal**, so CI logs stay clean.

### Exit codes

| Situation | `--strict` (default) | `--lenient` |
|-----------|----------------------|-------------|
| No findings | `0` | `0` |
| Any finding (error or warning) | `2` | `0` |
| Unparseable file | `2` | `2` |
| Bad usage | `1` | `1` |

This is what subsumes the old `verify`: `rows_mismatch` now fails by default, and
so does `checksum_mismatch` — which the old `verify` could not express at all.

### Check coverage

Legend: **have** = implemented (since excsv-cli 0.0.2).

| Check | Status | Level |
|-------|--------|-------|
| Structure, delimiters, quoting, ragged rows, encoding, ZIP/sidecar resolution | have (parse; mandatory — the file is unreadable otherwise) | always |
| Unknown `version=`, `.extsv` without `delim=tab`, unknown `#column` attributes | have (parse warnings) | schema |
| Malformed `pattern=` (regexp does not compile) | have (`checkColumnDeclaration`) | schema |
| Unknown `type=` | have (`ErrColumnUnknownType`) | schema |
| Attribute sanity: `min > max`, `len_min > len_max`, duplicate `name=`, colliding `index=`, `default`/`enum` unparseable under the declared `type` | have (`checkColumnDeclaration`) | schema |
| `reference=` present on an inline document (spec: **MUST NOT**) | have (`ErrReferenceOnInline`) | schema |
| Unrecognized `#` meta lines carried through rewrite | have (`Meta.Unknown` + `ErrUnknownMetaLine` finding) | schema |
| Cell types, `enum`, `pattern`, `min`/`max`, `len_min`/`len_max`, `required` | have (`checkColumnValues`) | data |
| `rows=` vs actual count | have | data |
| `checksum=` | have | data |
| Stored `#%` vs recomputation | have (`checkAggregations` / `ErrAggStale`) | data |
| `unique=1` duplicate values | have (`checkUnique` / `ErrColumnNotUnique`) | data |

Four checks about `csvw=` / `schema=` / `#csvw` coherence were in this table and are
gone: those fields were **removed in v0.4**, so there is nothing to be coherent about.
That is four checks deleted rather than written, which is the best kind of scope
reduction.

### Unrecognized meta lines

Implemented in v0.0.2. Unrecognized `#` lines are collected in `Meta.Unknown`,
re-emitted verbatim by `Serialize`, reported by `validate` as informational findings
(`ErrUnknownMetaLine`), and never modified by `fix`. This matters for forward
compatibility: v0.3 documents may carry `#csvw` lines that this version does not
interpret but must not strip on rewrite.

---

## `fix` — the single repairer

Replaces `freeze` + `tidy`. (`freeze` was never a spec term — it only appears as
feature M6 in the upstream plan. The `#@exported` key it stamps *is* spec.)

Renamed away from `freeze` because "freeze" describes export-prep, not repair, and
away from `recalculate` because the command does more than recompute: it also
rewrites cell content (NFC normalization, control-character stripping, width
padding) and stamps provenance.

### `--only` targets

| Target | Effect | Was |
|--------|--------|-----|
| `format` | Normalize line endings, padding, quoting, cell text, meta order | `tidy` |
| `columns` | Infer `#column` stubs when none exist (`InferColumns`) | part of `freeze` |
| `agg` | Recompute the `#%` lines that already exist. **Never adds new ones** — see note below | part of `freeze` |
| `checksum` | Recompute `checksum=sha256:…` | part of `freeze` |
| `rows` | Resync `rows=` to the actual count | part of `freeze` |
| `stamp` | Set `#@exported` and `#@tool` | part of `freeze` |

Default is all six. `excsv FILE fix --only format` == the old `tidy`.

**Behavior change vs `freeze`:** `freeze` injects the default aggregation set when a
document has none. `fix` must not — `convert` deliberately emits no `#%`, so a
`freeze`-style `fix` would immediately write back aggregations the author chose not
to have. A command called "fix" repairs what is there; it does not invent metadata
nobody asked for. Adding an aggregation stays `agg add NAME`'s job.

`--only columns` is the one remaining place `fix` creates something from nothing
(`InferColumns` runs only when there are no `#column` lines at all). That is
defensible — columns are structural, and after `convert` they always exist so it is
a no-op — but it is an asymmetry worth revisiting if it ever surprises anyone.

Note that `SyncDerived()` already recomputes `rows`, `checksum` (when present), and
every existing `#%` on each write, so these do not drift between `fix` runs.

### What `fix` can and cannot repair

**Can:** anything *derived* from the data — `rows=`, `checksum=`, `#%` values,
inferred `#column` stubs, formatting.

**Cannot:** anything where the *data itself* is wrong. `type=int` with a cell of
`abc`, a violated `required`, a value outside `enum`, a `pattern` mismatch — these
are reported by `validate --with-data` and must be fixed at the source. The docs
must say this explicitly so nobody expects magic.

### Reporter → repairer symmetry

```sh
excsv sales.excsv validate --with-data --column amount   # finds: stale #%sum, checksum
excsv sales.excsv fix --only agg,checksum --column amount
```

`validate` ends its report with the exact `fix` invocation that addresses the
repairable findings.

---

## `convert` — the import entry point

The richest command in the CLI, and deliberately so: importing is the one moment
where you have the whole source in hand and can describe it in full, so everything
you might want to declare should be expressible in a single invocation.

`FILE` is normally the **delimited source**. It may also be an existing ExCSV
document, in which case `convert` re-encodes it — see below.

**Two input modes.**

| Input | Behavior |
|-------|----------|
| CSV / TSV | Import: generate the full metadata set described below. |
| ExCSV (plain, zip, pack) | Re-encode: **preserve every existing metadata line, regenerate nothing.** Apply only the requested dialect, `--format`, and enrichment flags. |

The second mode is what lets the whole of `header set` go away. Without it there is
no path to re-encode an existing document without laundering it through a temp CSV
and losing all of its `#@`, `#%`, `#$` and `##` lines.

```sh
excsv sales.excsv convert --delim tab -o sales.extsv     # re-encode the dialect
excsv sales.excsv convert --null NA -o sales.excsv       # materialize nulls as NA
```

**No `-o` always means stdout, in both modes.** It would be convenient for the
re-encode mode to default to in-place, but a command that silently overwrites its
input when you forget a flag is not worth the keystrokes. In-place is `-o` with the
input path — safe, because the source is fully read into memory before anything is
written. (`--in-place` sugar can come later.) This differs from `fix`, which does
default to in-place, and the difference is intentional: `fix` repairs *this* file,
`convert` produces *a* file.

### Always emitted, no flags needed

| Output | Detail |
|--------|--------|
| `#!excsv` | `version`, **`delim`**, **`quote`**, `rows`, `header=0` when applicable, `checksum=sha256:…` |
| `#column` | one per column, with an inferred `type=` (`int` / `double` / `string`) |
| `#@` | `created` (import timestamp), `source` (source filename), `tool` |

**`delim=` and `quote=` are both always written, including `quote=none`.** Today
`delim` is unconditional but `quote` is emitted only when it is not `none`:

```go
if outputQuoteName != "" && outputQuoteName != "none" {
    fields["quote"] = outputQuoteName
}
```

That is not a correctness bug — the spec's default for `quote` is `none` and
`applyHeaderDefaults` agrees — but the spec marks `quote` as **SHOULD** be present,
and a format whose entire pitch is "the dialect is declared inside the file" should
not make the reader recall a default. Write both, always.

Column typing reuses `InferColumns` / `sniffColumnType` rather than the import
path's own hardcoded `type=string`. Today those are two separate implementations
and the worse one sits on the import path; after this there is one.

**Column naming.** `name=` gets a sanitized identifier, `title=` preserves the raw
header text. This is required, not cosmetic: today `--columns` hard-fails on any
header that does not match `^[A-Za-z_][A-Za-z0-9_-]*$`, so making columns the
default without sanitizing would turn `Total Sales` into a conversion error on files
that convert fine today. `title` is already in `knownColumnAttrs`, and CSVW and
Frictionless split name/title the same way.

```
#column name=total_sales title=Total Sales type=double
```

### Never emitted unless asked

`#%` aggregations, `#$` SQL, `##` comments. These are authorial choices, not facts
about the source, so they stay opt-in.

### Flags

**Output shape**

| Flag | Default | Notes |
|------|---------|-------|
| `--format` | `inline` | `inline` \| `sidecar` \| `zip` \| `pack`. Replaces the old mutually-exclusive `--sidecar` and `--zip` booleans. |
| `-o`, `--output` | per format | `inline` → stdout · `sidecar` → `<stem>.excsv` / `.extsv` · `zip` → `<stem>.excsv.zip` · `pack` → `<stem>.excsv.pack.zip`. Binary shapes never default to stdout. |
| `--reference` | basename of source | `--format sidecar` only |
| `--table` | source stem | `--format pack` only — one CSV is one table |
| `--zip-password` | — | `--format zip` / `pack` only |

**Dialect**

| Flag | Default | Notes |
|------|---------|-------|
| `--delim` | detected | output delimiter; the input delimiter is always sniffed separately |
| `--quote` | detected | output quoting; `none`, `double`, `single`, or a literal character |
| `--no-header` | `false` | every row is data; columns get `index=` |
| `--encoding` | `UTF-8` | output `encoding=` |
| `--null STR` | none | `null=` — **rewrites cells**, see below |
| `--no-checksum` | `false` | opt out of `checksum=` |

**`--null` is a dialect flag, not a declaration.** Worth spelling out because it
looks like one. Spec: *"Additional non-empty string representing null. Empty fields
are always null by default."* So `null=NA` does not mean "NA is a synonym for
empty" — it means the writer *materializes* nulls as the token `NA`. The
implementation already works that way:

```go
func (doc *Document) convertNull(value string) error {
	old := doc.Header.Null
	rewriteNullCells(doc, old, value)
```

`rewriteNullCells` walks every cell: anything that was null under the old token
(empty, or equal to the old token) is written out as the new one. Setting it
rewrites the data section; clearing it rewrites it back to empty. It also shifts
`#%` values — `agg.go` skips cells equal to `Header.Null` — and therefore the
checksum. That is three derived things moving, which is exactly the profile of
`--delim` and `--quote`, so it belongs next to them. (`null=""` is redundant per
spec; `--null ""` clears the field.)

**`convert` sets no declarations.** It briefly was the home for `sql-dialect=`,
`schema=` and `csvw=`, purely because `header set` had died and they had nowhere else
to go. `sql-dialect=` now lives with the lines it governs (`sql dialect set`), and
`schema=`/`csvw=` are leaving the format altogether. So `convert`'s header surface is
exactly the fields that describe how the bytes are encoded, plus `reference=` for the
sidecar shape — a much better story than "convert is where the leftover header fields
live".

`reference=` stays in the output-shape table above, because it is meaningless outside
`--format sidecar` — the spec says it **MUST NOT** be set on an inline document, so
`--reference` without `--format sidecar` is a usage error, not a header edit.

**Quoting guard.** `Tidy()` already promotes `quote=none` to `quote=double` when any
cell contains the delimiter. `convert` has no such guard today, and it can change
the delimiter on output — so converting `a;b` (`delim=semicolon quote=none`) with
`--delim comma` produces a corrupt file whenever a value contains a comma. Running
the `format` step from `fix` as part of the import pipeline fixes this for free; the
promotion should also emit a warning so the change is not silent.

**Enrichment (opt-in)**

| Flag | Repeatable | Effect |
|------|------------|--------|
| `--agg LIST` | no | `#%` for each name; also accepts `default` (`count_nonnull,count_null,sum,min,max`) and `all` |
| `--meta KEY:VAL` | yes | `#@KEY: VAL` |
| `--comment TEXT` | yes | `##` line |
| `--sql KEY:SQL` | yes | `#$KEY: SQL` |
| `--column-attr COL.ATTR=VAL` | yes | extra `#column` attributes, e.g. `amount.unit=USD`. `.` is a safe separator because sanitized names cannot contain one. |

**These flags are a batch form, not a second implementation.** Each one must call
the same function as its group command (`meta set`, `comment add`, `agg add`,
`sql set`, `column set`). Otherwise we have just reintroduced the duplication this
document exists to remove.

**`convert` does not read CSVW.** There is no `--csvw` input flag: CSVW is
write-only in this tool, see `export csvw`. The reasoning is in *Alternatives
considered* — briefly, an importer that reads CSVW has to decide what to do when the
foreign schema and the sniffed reality disagree, and that decision has no good
default. Every column attribute CSVW could have supplied is reachable with
`--column-attr`, which is explicit about who declared what.

### `--format pack` vs `pack create`

Both produce a single-table pack, from different inputs: `convert --format pack`
starts from CSV/TSV, `pack create` starts from an existing plain `.excsv`. Same
relationship as `--format zip` and `zip wrap`. Both stay.

### Order of operations

Aggregations and columns must be computed **before** the sidecar branch zeroes
`doc.Data`. Today `--sidecar` clears the data section after the metadata is built,
so any data-derived output added later would silently come out empty.

### Speculative, not decided

`--ddl DIALECT` to generate `#$ddl` from the inferred column types. The spec has
`#$ddl` with dialect tagging and the types are already known, so this is nearly
free. The default-dialect half of the problem is now solved — `sql dialect set`
writes `sql-dialect=`, and the spec's resolution order (line suffix → header field →
`ansi`) gives the answer for free, so `--ddl` needs no dialect argument at all.
What is still missing is a table-name source: `--table` exists but only means
something under `--format pack`. Flagged as my suggestion, not a settled decision.

---

## `sql dialect` — the `sql-dialect=` header field

`sql-dialect=` is the default dialect for `#$` lines that carry no suffix. It
governs `#$`, so it belongs to `sql`, not to `convert`.

| Command | Behavior |
|---------|----------|
| `sql dialect get` | bare token on stdout — `postgres-18`, or `ansi` when the field is unset, since `ansi` is what the resolver actually uses. Never JSON. |
| `sql dialect set D` | `D` is a well-known token, optionally `-VERSION` (`mysql`, `postgres-17`, `duckdb`). Unknown prefixes are preserved with a warning, per spec. |
| `sql dialect remove` | drop the field; `#$` lines fall back to `ansi` |

The resolution order is spec, not ours (`implementation/sql.md`), and the
implementation already matches it:

```go
func effectiveDialect(stmt SQLStatement, headerSQLDialect string) string {
	if stmt.Qualified && stmt.Dialect != "" { /* … */ }
	if headerSQLDialect != "" {
		return headerSQLDialect
	}
	return "ansi"
}
```

So this is the field that lets you write `#$ddl:` once instead of
`#$ddl-postgres-18:` on every line. It is consumed today by `sql list` / `sql get`
via `EffectiveDialect`, so it is the one optional header declaration that actually
changes behavior — which is why it survives while `csvw=` and `schema=` do not.

**Name collision, deliberate.** `sql list --dialect postgres` *filters* lines;
`sql dialect set postgres` *sets the default*. Same word, two jobs. I considered
`sql default-dialect` to remove the ambiguity and rejected it on length: the shapes
are different enough (a subcommand taking a positional vs. a flag on a sibling) that
confusing them requires effort. Flagging it because if anyone does trip on this,
renaming is the fix.

---

## `export` — foreign representations at the boundary

**CSVW was removed in v0.4.** `#csvw`, `csvw=` and `schema=` no longer exist in the
format. The reasoning is in *Alternatives considered*; the short version is that
CSVW's natural deployment — `sales.csv` plus `sales.csv-metadata.json` — is
*structurally our sidecar*, JSON instead of `#` lines. Two sidecar formats do not
belong nested inside one another. Embedding it forced a `schema=` field to
arbitrate which schema wins, and that arbitration was the entire cost of the feature.

**CSVW is write-only.** One direction, deliberately: we can *produce* a CSVW sidecar
for consumers that want one, and we do not consume CSVW at all. That asymmetry is the
whole feature — publishing to a standard costs us nothing but a serializer, while
reading it would put a foreign schema in a position to contradict the document.

`export` never modifies `FILE`. Both subcommands default to stdout.

| Command | Output | Fidelity |
|---------|--------|----------|
| `export json` | the v0.5 JSON form | lossless except `##` (see below) |
| `export csvw` | a CSVW metadata sidecar | lossy, and says exactly what it dropped |

**The two have different contracts and must report differently.** `export json` is a
bijection by specification: if it ever warns about losing something, that is a bug.
`export csvw` loses by nature, so it names every attribute it could not carry, on
stderr, every time.

**Neither has an inverse, and for different reasons.** `export json` does not need
one yet only because nothing consumes `.excsv.json` in this tool — the spec makes it
a full peer of the text form, so an `import json` is a legitimate future command.
`export csvw` will never get one on purpose.

### `export json` — the JSON form that had no command

This is the gap that matters more than CSVW. `implementation/json.md` (v0.5) is
normative: it defines a **bijection** — *"any conforming ExCSV text document maps to
exactly one JSON document and back, with no loss"* — ships
[`schema/excsv.schema.json`](https://excsv.org/schema/excsv-0.5.schema.json)
(draft 2020-12, `$id` `https://excsv.org/schema/excsv-0.5.schema.json`) and an
example, and calls out LLM structured output as a target.

Output defaults to stdout; `-o` writes a `.excsv.json` file. The JSON form is not a
sidecar — `sales.excsv.json` and `sales.excsv` are two encodings of one document.

**One documented loss, and it is the spec's choice, not ours.** The profile drops
`##` human comments: *"Free-text `##` lines carry no structured meaning and are not
represented in JSON. This is the one intentional non-round-trip."* `export json`
must therefore warn when the document has `##` lines and the output is being used as
a round-trip vehicle. Everything else — including `#%` arity, `enum` splitting, and
`decimal`/`long` precision as strings — is specified and must be exact.

Two things the JSON Schema explicitly cannot check, so `export json` has to get them
right itself: `aggregates[*]` array lengths equal the physical column count, and
`decimal`/`long` cells are encoded as JSON strings rather than numbers.

Packs are already covered by the profile (`layout: "pack"` with a `tables` array and
root-level `fk`), so `export json` works on every shape without a `--table` flag.

### `export csvw`

CSVW's required property on a table description is exactly one: `url`, *"the single
URL of the CSV file that the table is held in"*. That single fact decides the
interface:

| Document shape | `url` |
|----------------|-------|
| sidecar | `reference=` — already correct, no flag needed |
| inline / zip / pack | there is no CSV file; `--url PATH` is **required** |

An inline ExCSV document is not a CSV file — a CSVW processor following a URL
pointing at it would read `#!excsv version=0.4 …` as the first data row. So for
inline input the honest workflow is to produce the pair:

```sh
excsv sales.excsv data print -o sales.csv
excsv sales.excsv export csvw --url sales.csv -o sales.csv-metadata.json
```

Default filename when `-o` is a directory, or a convenience later: CSVW's convention
is `<data>.csv-metadata.json`.

A multi-table pack maps onto a CSVW **TableGroup** with one `tables[]` entry per
table, which is a genuinely clean fit — `--table` narrows it to one table if you
want a single table description instead.

### What `export csvw` carries

Most of the ExCSV column vocabulary maps onto CSVW cleanly, because CSVW's derived
datatypes have the constraint facets we need:

| ExCSV | CSVW |
|-------|------|
| `name` | `name` |
| `title` | `titles` |
| `description` | `dc:description` |
| `type` | `datatype.base` (see the type table) |
| `min` / `max` | `datatype.minimum` / `maximum` |
| `len_min` / `len_max` | `datatype.minLength` / `maxLength` |
| `pattern` | `datatype.format` |
| `required` | `required` |
| `default` | `default` |
| `null` | `null` |
| `separator` | `separator` |

Omitting what CSVW leaves optional is not unfaithful — CSVW is designed to be
partially specified, and a small metadata document is a valid one.

`datatype.base` takes an XSD datatype name. The ExCSV type list from
`implementation/columns.md` maps onto it one-way:

| ExCSV `type` | CSVW `datatype.base` |
|--------------|----------------------|
| `string` | `string` |
| `int` | `int` |
| `long` | `long` |
| `float` | `float` |
| `double` | `double` |
| `decimal` | `decimal` |
| `boolean` | `boolean` |
| `date` | `date` |
| `time` | `time` |
| `datetime` | `dateTime` |
| `uuid` | `string` with `format` `[0-9a-fA-F-]{36}` |
| `binary` | `base64Binary` |

Every ExCSV type has a target, so `type=` never gets dropped. `uuid` and `binary` are
the two that widen: XSD has no UUID type, and `base64Binary` is a near-fit for our
`binary` (*"Base64-encoded binary"*), so both are noted on stderr as approximations
rather than silent successes.

### What it refuses to carry, and why

**`unique=` is dropped, not translated.** This one deserves the space because the
tempting answer is wrong. CSVW's only uniqueness construct is schema-level
`primaryKey`, and the spec is explicit that validators *"MUST check that each row has
a unique combination of values of cells in the indicated columns"*. ExCSV `unique=1`
on two columns asserts two **independent** constraints. Emitting
`"primaryKey": ["a", "b"]` asserts only that the **pair** is unique — strictly
weaker, and a validator would happily pass data that violates the original. Silently
weakening a constraint while appearing to preserve it is worse than dropping it, so
`export csvw` drops it and says so.

**No counterpart at all:** `unit`, `role`, `agg`, `order`, `regexp_dialect`. Each is
named on stderr. `enum` is the interesting near-miss — CSVW has no enumeration facet,
so the only encoding is a `datatype.format` regex alternation, which turns a value
list into a pattern. Dropped by default; `--enum-as-pattern` opts in, because the
result validates the same values but no longer reads as a list.

**No `#%`, no `#$`.** Aggregation values and SQL companions have no CSVW analogue
worth inventing. `#@` file metadata maps to the Dublin Core properties CSVW already
uses (`dc:title`, `dc:description`, `dc:creator`) where the key matches, and is
dropped otherwise.

### Why there is no CSVW reader

Producing CSVW is a serializer with a fixed contract: walk `#column`, emit what maps,
name what does not. Consuming it is a policy problem, and there is no defensible
default policy.

An importer handed `sales.csv` plus `sales.csv-metadata.json` has to answer questions
the data cannot settle. CSVW says `datatype.base=int`, the column sniffs as `string`
because row 40 000 holds `N/A` — who wins? CSVW's `tableSchema` describes nine
columns and the CSV has ten — is that a fatal mismatch or a partial description?
CSVW in the wild is frequently a bare `{"tableSchema": {"columns": […]}}` fragment
with no `url` and no `@context`, so we would also be committing to accepting
non-conforming input, which means our "CSVW support" would be a guess about which
subset people actually ship.

Every one of those questions has a *reasonable* answer and no *right* one, and the
wrong answer mislabels data silently. Meanwhile the capability it buys is already
present: `--column-attr amount.type=double` declares the same thing, in the invocation
that created the document, attributable to the person who ran it. So the reader is not
a missing feature; it is a rejected one.

---

## Group summaries

### Whole-document verbs (flat)

| Command | Purpose |
|---------|---------|
| `info` | Multi-line read-only summary: version, rows, columns, form, profile, delimiter, quote, null, optional `sql-dialect`, sidecar `reference`, aggregation *names*, SQL *keys* (with count), and all `#@` metadata. `--no-meta` hides `#@`. Subcommand `info header` prints the `#column` schema (names row + attribute lines). Pack without `--table` summarizes the manifest. Global `--json` available. |
| `validate` | Read-only conformance report. See section above. |
| `fix` | In-place repair of derived metadata. See section above. |
| `convert` | Import CSV/TSV → ExCSV, in any of four output shapes, with optional enrichment. See section above. |

### `data` — data section

| Command | Purpose |
|---------|---------|
| `data print` | The data section as CSV/TSV in the document's own dialect. **With no flags: header row plus every body row, verbatim** — that is the old `strip`. `--limit`/`--offset`/`--select` narrow it, `-o` writes to a file, global `--json` switches the shape. |
| `data get` | One row (0-based) or a single cell by row + column name/index. Kept separate from `print` because it emits a raw scalar for scripting, not CSV. |
| `data append` | Append rows from `--row` and/or a delimited `--file` (`--skip-header`). |
| `data sort` | Sort body rows by `--by NAME\|idx[:asc\|:desc]` (repeatable), `--desc` default. |

**Merging `strip` into `print` — details that must survive:**

- `strip`'s sidecar guard: when `FILE` is a metadata-only sidecar, it re-parses with
  `ResolveReference = false`, prints an explanatory notice on stderr, and exits `0`
  without writing anything. `print` must keep this.
- `strip` had `-o`, `print` did not → merged command gets `-o`.
- `print` had `--json` and projection, `strip` did not → merged command keeps both.
- No output conflict: both already go through `loadTableDoc` and both emit the
  header row when `Data.HasHeaderRow`, so flagless `print` is byte-identical to the
  old `strip`.
- `--columns` is renamed to `--select`, which also settles the name clash with
  `convert --columns` (a bool that emits `#column` lines).

### `header` — `#!excsv`, read-only

**Not the same as `info header`.** This group owns the magic line (`#!excsv …`).
`info header` owns `#column` and lives under `info` — see the `info` section above.

**`header set` and `header remove` are deleted.** The header line is the one place
where a piecemeal edit can quietly break the document: change `delim` and the data
section no longer parses; change `quote` and values containing the delimiter become
ambiguous; change `rows` or `checksum` and the file now lies about itself. A verb
that can corrupt the format from the outside should not exist.

Want a different header? Convert. Want the header to match reality? `fix`.

The old `header set` was six unrelated jobs wearing one name. Where each field goes:

| Field | Nature | Owner |
|-------|--------|-------|
| `delim`, `quote`, `header`, `encoding`, `null` | **Re-encode** — changing these rewrites the data bytes | `convert --delim` / `--quote` / `--no-header` / `--encoding` / `--null` |
| `sql-dialect` | **Declaration** governing `#$` | `sql dialect set` / `remove` |
| `csvw`, `schema` | *(removed in v0.4)* | nobody — the only CSVW surface left is `export csvw` |
| `reference` | **Container wiring** — spec: MUST NOT appear on an inline document | `convert --format sidecar --reference` |
| `rows`, `checksum` | **Derived** from the data | `fix --only rows` / `--only checksum` |
| `version` | Tool-managed; `convert` emits the current one | nobody |
| `original-size` | Container-managed | `zip` / `pack` |
| `layout`, `single-table`, `table-count`, `section-size` | Pack structure | `pack` group (deferred to the pack pass) |

Note `null` sitting in the **first** row, not the declaration rows. It reads like a
declaration and is not one: setting it rewrites every null cell in the data section.
The `convert` section explains why.

The Owner column has five different answers, which is the argument against a single
`header set` in one line. Each owner already understands the consequences of the
field it writes; a generic setter understands none of them. All of them call
`SetHeaderField`, which stays in `pkg/excsv` as the shared internal helper even
though its CLI surface is gone.

This makes `header` the only group with no write verbs, breaking the otherwise
uniform `list`/`get`/`set`/`remove` shape. That is the point: the exception is the
message.

**`header rows` prints a bare number.** No `rows=` prefix, no JSON envelope even
under `--json` — its only reason to exist is `$(excsv f.excsv header rows)`. It
already prints just the digits today; what changes is that it must always succeed:
`rows=` is `MAY` in the spec, so when the field is absent it falls back to the
counted row count (`DeclaredOrCountedRows`) instead of exiting `1`. Note this makes
it read the data section when `rows=` is missing. Nothing is lost by the fallback —
reporting a declared-vs-actual disagreement is `validate --with-data`'s job.

### Other meta-line groups

| Group | Line | Verbs |
|-------|------|-------|
| `meta` | `#@` | `list`, `get`, `set`, `remove` |
| `column` | `#column` | `list`, `get`, `set`, `remove`, `materialize`, `dematerialize` |
| `agg` | `#%` | `list`, `get`, `add`, `update`, `remove` |
| `sql` | `#$` + `sql-dialect=` | `list`, `get`, `set`, `remove`, `dialect get/set/remove` |
| `comment` | `##` | `list`, `add`, `remove` |

`sql` is the only meta-line group that also owns a header field, because
`sql-dialect=` is a pointer at the lines it manages.

**`column materialize`/`dematerialize`** are the two verbs `column` has beyond
the uniform `list`/`get`/`set`/`remove` shape — the v0.5 computed-column pair.
A `#column formula=` is virtual by default (no header cell, no field in any
row); `materialize NAME` evaluates it and writes the values in as an ordinary
trailing column, setting `materialized=1`; `dematerialize NAME` removes that
cached data and clears `materialized`, without ever touching `formula=`. Both
rewrite `FILE` in place (or `-o`) for plain, row ZIP, and pack exactly like
any other Mode A write — `MaterializeColumn`/`DematerializeColumn` only touch
`Document.Data`/`Document.Meta`, so the existing zip re-wrap and pack
`.col`-file resync pick up the change for free. A **sidecar is the one
exception**: its reference is never rewritten, so both verbs instead write a
brand-new inline file (`-o`, or `<name>.materialized.excsv` by default) and
leave the sidecar and its referenced CSV untouched.

### `export` — foreign representations

| Command | Purpose |
|---------|---------|
| `export json` | The v0.5 JSON form (`.excsv.json`). Lossless except `##`, per the spec. |
| `export csvw` | A CSVW metadata sidecar. Lossy; names every dropped attribute. Write-only. |

Not a meta-line group — it owns no lines and writes no ExCSV. It exists because both
targets are *representations of the whole document*, and because the JSON form had no
command at all.

### `pack` — multi-table container

| Command | Purpose |
|---------|---------|
| `pack create` | Build a single-table pack from a plain `.excsv`. |
| `pack table list/add/drop/extract` | Manage tables inside a pack. |
| `pack fk list` | List `#fk` declarations. |

### `zip` — row ZIP framing

| Command | Purpose |
|---------|---------|
| `zip wrap` / `zip unwrap` | Wrap plain ExCSV as `.excsv.zip` / extract inner. |
| `zip password set/remove` | Add, change, or remove the entry password. |

---

## Migration: old → new

### Removed / merged

| Old | New | Note |
|-----|-----|------|
| `verify` | `validate` | `rows_mismatch` now fails by default; so does `checksum_mismatch` |
| `validate --schema` | `validate --with-data` | |
| `column check` | `validate --with-data [--column NAME]` | was a duplicate of `validate --schema` |
| `freeze` | `fix` | same behavior at default `--only` |
| `tidy` | `fix --only format` | `freeze` already called `Tidy()` internally |
| `strip` | `data print` | flagless `print` already produces identical bytes |
| `data print --columns` | `data print --select` | frees `--columns` from its double meaning |
| `cat` | *(deleted)* | plain files: use the shell's `cat` / `type`. Canonical form: `fix --only format --dry-run`. Reading inside a ZIP/pack is an open gap — see open questions. |
| `convert --sidecar` / `--zip` | `convert --format sidecar` / `zip` | two mutually-exclusive booleans become one enum, which also gains `pack` |
| `convert --columns` | *(default)* | `#column` is always emitted now, with a real inferred type instead of `type=string` |
| `convert --checksum` | *(default)* | opt out with `--no-checksum` |
| `header set` / `header remove` | *(deleted — `header` is read-only)* | a piecemeal edit to `#!excsv` can make the data section unparseable; see the field-ownership table above |
| `header set delim\|quote\|header\|encoding\|null` | `convert --delim` / `--quote` / `--no-header` / `--encoding` / `--null`, now accepting an ExCSV input | all five rewrite the data section; that is conversion, not a metadata edit |
| `header set sql-dialect` | `sql dialect set D` | the field governs `#$`, so `sql` owns it |
| `header set csvw\|schema` | *(removed — fields gone in v0.4)* | the only CSVW surface is `export csvw`; nothing is read, nothing is stored |
| `header set reference` | `convert --format sidecar --reference` | spec forbids `reference=` on an inline document, so it cannot be a standalone edit |
| `header set rows\|checksum` | `fix --only rows` / `--only checksum` | derived fields were never meant to be hand-set |
| `header set version\|original-size` | *(no command)* | tool- and container-managed |
| `header set layout\|single-table\|table-count\|section-size` | `pack` group | deferred to the pack pass |

### Regrouped

| Old (flat) | New (grouped) |
|------------|---------------|
| `rows` | `header rows` |
| `table list/add/drop/extract` | `pack table list/add/drop/extract` |
| `fk list` | `pack fk list` |

### New — no old spelling

| Command | Why it did not exist |
|---------|----------------------|
| `sql dialect get/set/remove` | `sql-dialect=` was reachable only through the generic `header set`, which had no idea what it was setting |
| `export json` | the JSON form is normative and ships a JSON Schema, and no command has ever emitted it |
| `export csvw` | CSVW could only be embedded, never produced — so it was metadata you carried, not metadata you could hand to anyone |

**Back-compat:** keep every old spelling as a hidden (deprecated) alias so existing
scripts and tests keep working — `Hidden: true` commands that delegate to the new
paths, dropped after a release.

---

## Open questions (not yet decided)

Carried over from the dedup pass; listed so they are not lost.

1. **`get [KEY]` with no argument duplicates `list`** in every group. Drop the
   fallback and require the key?
2. **Mutation input flags are inconsistent** — `meta/sql set KEY --value`,
   `column set NAME --attr`, `comment add --value`, `data append --row`. Pick one
   canon.
3. **Password has two channels** — global `--zip-password` and local `--password` /
   `--current-password` under `zip`.
4. **JSON shapes differ per command** — array here, `{ok:true}` there, bare object
   elsewhere. Adopt one envelope (`{ok, path, findings?, data?}`).
5. **`convert --ddl` (no dialect argument)** — generate `#$ddl` from the inferred
   column types. The dialect half is settled: `sql-dialect=`, else `ansi`. Still
   needs a table-name source.
6. **The CSVW mapping is ours, not anyone's spec.** The table under `export csvw` is
   a proposal. CSVW says nothing about ExCSV and, after the spec change, ExCSV says
   nothing about CSVW — so there is no normative reference to appeal to and no one to
   defer to. It needs its own fixtures, and since the mapping is one-way there is no
   round-trip test to lean on: correctness has to be asserted against a real CSVW
   validator consuming the output.
7. **Unrecognized `#` meta lines are dropped on rewrite** — see the section under
   `validate`. Blocking for the CSVW spec removal, because `#csvw` in existing v0.3
   documents becomes an unrecognized line and would be destroyed by `fix`.
8. **Does `export` need `--table` for `export json`?** A pack maps to
   `layout: "pack"` with a `tables` array, so the whole-pack answer is well defined
   and `--table` is only a convenience for extracting one table's JSON. Cheap either
   way; not deciding now.
9. **Reading inside a ZIP/pack without writing to disk** — deleting `cat` removes the
   only way to do this. `zip unwrap` and `pack table extract` write with a direct
   `os.WriteFile` and have no stdout path. Deferred to the ZIP/pack pass; the
   obvious candidate is a `-o -` convention, but that is a decision for then, not
   now.

---

## Alternatives considered

- **`doc` group for lifecycle verbs** (`doc info`, `doc validate`, …): rejected —
  these are the most-typed commands; extra nesting hurts more than it helps.
- **`--fail-on rows,checksum,schema` on `validate`:** rejected — the existing
  `--strict` / `--lenient` pair already expresses the same dial with no new flags.
- **`--fix` on `validate`:** rejected — a validator that writes is a footgun, it
  muddies the exit-code contract, and `fix` already owns that job.
- **`recalculate` instead of `fix`:** rejected — the command also rewrites cell text
  and stamps provenance, so "recalculate" understates what it touches.
- **Folding `cat` into `data print --full`:** moot — `cat` is deleted outright. It
  would also have been the wrong shape: a flag should change how much output you
  get, not what kind, and it would have made a `data` command emit metadata.
- **Keeping `cat` for containers:** rejected as a *command* — one edge case does not
  earn a top-level verb. How that case gets covered instead is deferred to the ZIP
  pass; see open questions.
- **Folding `data get` into `data print`:** rejected — `get` returns a raw scalar for
  shell use; expressing it as `print --limit 1 --offset N --select COL` is both
  clumsy and the wrong output shape.
- **`convert` under `data`/`import`:** it creates a new file rather than editing one,
  so it stays a top-level entry point.
- **Keeping `header set` for the declaration keys** (`null`, `csvw`, `schema`,
  `sql-dialect`, `reference`): rejected. It leaves a command whose safety depends
  entirely on a keyword allowlist — every future header field has to be classified
  correctly or the guard leaks, and the user still has to learn which keys it accepts
  and which it refuses. The premise was wrong too: `null` rewrites the data section,
  and `reference` is illegal on an inline document, so only three of the five were
  ever safe. Distributing them to the groups that own what they govern costs a few
  flags once and removes the class of mistake.
- **Parking `sql-dialect`, `csvw` and `schema` on `convert`:** rejected, and this was
  my own earlier proposal. It only ever made sense as "somewhere to put the orphans
  after `header set` died", which is not a design. `convert`'s header surface should
  be exactly the encoding of the bytes it writes; a `--schema csvw` flag on an import
  command is a field with no relationship to importing.
- **A `csvw` group owning `#csvw` + `csvw=` + `schema=`:** superseded. It was the
  right answer to the wrong question. Giving the orphaned meta line an owner made the
  three pieces of state consistent, but it left the underlying problem intact: a
  format carrying a second format's schema, plus a field to arbitrate between them.
  Removing CSVW from the spec deletes the group, the arbitration, four `validate`
  checks, and the `expand` verb in one move.
- **`csvw` folded into `meta`** (`meta get csvw`): rejected while the payload was
  still stored — `#csvw` is not a `#@` line, and the group would have had to
  special-case a payload plus two header fields. Moot now.
- **Keeping CSVW embedded for the migration story** ("bring your CSVW along"):
  rejected, and this is the decision the rest follows from. A carried payload nothing
  reads is not interoperability, it is storage.
- **`convert --csvw`: reading a CSVW sidecar and lowering it to `#column`:**
  proposed, then rejected, and the rejection is worth its own entry because it cuts
  the one argument in favor of it. A CSVW reader would be the migration on-ramp —
  point `convert` at `sales.csv` + `sales.csv-metadata.json` and get real column
  declarations instead of sniffed ones. Genuinely useful, and dropped anyway, because
  the implementation has to resolve conflicts it cannot resolve: declared type vs.
  sniffed type, column-count mismatch, and the very common non-conforming fragment
  with no `url` or `@context`. Each has a plausible default and no correct one, and
  the failure mode is a silently mislabeled column — the worst class of bug for a
  format whose entire pitch is self-description. The capability is not lost, only made
  explicit: `--column-attr amount.type=double` says the same thing with a human behind
  it. Direction of travel matters here: we publish to standards, we do not import from
  them.
- **Symmetry as an argument for the reader** ("we export CSVW, so we should import
  it"): rejected. Export and import are not the same problem wearing two hats.
  Serializing is total and deterministic — every `#column` attribute either has a
  target or is named as dropped. Deserializing is a merge with an adversary, and the
  asymmetry in the difficulty is the reason for the asymmetry in the CLI.
- **`#column` → CSVW generation, considered impossible:** wrong, and worth recording
  because the correction changed the design. My argument was that CSVW's vocabulary
  is too wide to fill faithfully; that is false — CSVW is designed to be partially
  specified, and most of the ExCSV attribute set maps onto derived-datatype facets
  directly. Of the three concrete objections that replaced it, two only applied to
  generating CSVW *inside* the document: the required `url` has a perfectly honest
  value when the output sits next to a CSV file, and "two sources of truth that
  drift" does not apply to an export artifact any more than it applies to a build
  output. Only `unique=` survives as a real mismatch, and for an export the answer is
  simply to drop it loudly. That is what turned "do not generate CSVW" into
  `export csvw`.
- **`convert` defaulting to in-place for ExCSV input:** rejected — see the `convert`
  section. Overwriting the input because a flag was forgotten is a worse failure
  than typing `-o`.
