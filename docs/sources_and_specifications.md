# Sources and Specifications

Reference index for building **excsv-cli** (Go). Use the upstream [boligolov/excsv](https://github.com/boligolov/excsv) repo as the normative source; local copies are convenience snapshots, not forks.

Upstream spec is **v0.5**. This repo implements the **row family** (plain + zip) and **pack family** (`.excsv.pack.zip`).

## v0.5 changes (vs v0.4)

| Area | Change |
| --- | --- |
| **Version** | `version=0.5` |
| **Computed columns** | `#column formula=` declares a value derived from other stored columns instead of stored data; `materialized=1` caches it as an ordinary column. Reversible via `column materialize` / `column dematerialize`. Virtual by default: no header cell, no field in any row, no pack `.col` file. Normative spec: [`implementation/columns.md`](downloaded/implementation/columns.md#computed-columns-formula). New error codes in [`error-handling.md`](downloaded/implementation/error-handling.md#computed-columns). |

## v0.4 changes (vs v0.3)

| Area | Change |
| --- | --- |
| **Version** | `version=0.4` |
| **JSON form** | First-class shape: `.excsv.json`, media type `application/excsv+json`. Normative spec: [`implementation/json.md`](downloaded/implementation/json.md). Schema: [`schema/excsv.schema.json`](downloaded/schema/excsv.schema.json) (`$id` `https://excsv.org/schema/excsv-0.4.schema.json`). |
| **CSVW removed** | `csvw=`, `schema=`, and `#csvw:` are gone from the format. Parsers treat them as unknown header keys / unrecognized meta lines. CSVW interop is a **tool concern**, and write-only: `export csvw` produces a CSVW metadata sidecar, nothing reads CSVW — see [`cli_commands_map.md`](cli_commands_map.md). |
| **Meta lines** | Five structured kinds: `#@`, `#column`, `#$`, `#%`, `##`. No `#csvw`. |

## Upstream links

| Resource | URL | Local copy |
| --- | --- | --- |
| Spec hub (README) | https://github.com/boligolov/excsv/blob/master/README.md | [`docs/downloaded/README.md`](downloaded/README.md) |
| Guide topics | https://github.com/boligolov/excsv/tree/master/docs | [`docs/downloaded/guide/`](downloaded/guide/) |
| **Normative implementation spec** | https://github.com/boligolov/excsv/tree/master/docs/implementation | [`docs/downloaded/implementation/`](downloaded/implementation/) |
| JSON form spec | https://github.com/boligolov/excsv/blob/master/docs/implementation/json.md | [`docs/downloaded/implementation/json.md`](downloaded/implementation/json.md) |
| JSON Schema | https://github.com/boligolov/excsv/blob/master/schema/excsv.schema.json | [`docs/downloaded/schema/excsv.schema.json`](downloaded/schema/excsv.schema.json) |
| JSON example | https://github.com/boligolov/excsv/blob/master/schema/example.excsv.json | [`docs/downloaded/schema/example.excsv.json`](downloaded/schema/example.excsv.json) |
| Feature catalog | https://github.com/boligolov/excsv/blob/master/plan/01-features.md | [`docs/downloaded/plan-01-features.md`](downloaded/plan-01-features.md) |
| Implementation plan | https://github.com/boligolov/excsv/blob/master/plan/README.md | [`docs/downloaded/plan-README.md`](downloaded/plan-README.md) |
| Fixture corpus spec | https://github.com/boligolov/excsv/blob/master/plan/02-fixtures.md | [`docs/downloaded/plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) |
| Fixture manifest | https://github.com/boligolov/excsv/blob/master/fixtures/fixtures.yaml | [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml) |

Raw download URLs:

```
https://raw.githubusercontent.com/boligolov/excsv/master/README.md
https://raw.githubusercontent.com/boligolov/excsv/master/docs/<topic>.md
https://raw.githubusercontent.com/boligolov/excsv/master/docs/implementation/<topic>.md
https://raw.githubusercontent.com/boligolov/excsv/master/schema/excsv.schema.json
https://raw.githubusercontent.com/boligolov/excsv/master/plan/01-features.md
https://raw.githubusercontent.com/boligolov/excsv/master/fixtures/fixtures.yaml
```

## What each source means

### `docs/implementation/*.md` — normative spec (highest authority)

RFC 2119 parser/writer rules, error-code registry, ZIP/pack invariants. **If behaviour is not defined here, do not invent it.** Start at [`implementation/README.md`](downloaded/implementation/README.md). Error codes live in `error-handling.md`. Column types/constraints: `columns.md`. Aggregations: `aggregations.md`. JSON bijection: `json.md`.

When the human guide (`docs/*.md`) and the implementation spec disagree, **implementation wins**.

### `README.md` + `docs/*.md` — human guide

Benefits-first tour of the format (inline / sidecar / pack / JSON). Use for CLI wording and examples, not for fail-vs-warn decisions.

### `plan/01-features.md` — feature catalog

Capability map (A–P) across RF plain, RF zip, PF pack. Feature IDs (`I1` convert, `H2` sort, `H6` append, `M1` validate, `E3` compute aggregations, …) are the build checklist.

### `plan/02-fixtures.md` + `fixtures.yaml`

How tests are structured. The Go runner walks `fixtures.yaml` (`plain/*` and `zip/*` only).

## Local layout in this repo

```
docs/
├── sources_and_specifications.md   ← this file
├── cli_commands_map.md             ← proposed CLI surface
└── downloaded/                     ← gitignored snapshots
    ├── README.md
    ├── guide/                      ← docs/*.md (human guide)
    ├── implementation/             ← normative topics (incl. json.md)
    ├── schema/                     ← excsv.schema.json, example.excsv.json
    ├── plan-README.md
    ├── plan-01-features.md
    └── plan-02-fixtures.md

test/fixtures/                      ← gitignored; sync from upstream
```

## What to refresh

Refresh when upstream `boligolov/excsv` changes in ways that affect parse/CLI behaviour.

| Asset | Refresh trigger |
| --- | --- |
| **`docs/implementation/`** | Error codes, MUST/SHOULD changes, ZIP rules, column types, JSON profile |
| **`docs/downloaded/schema/`** | JSON Schema or example changes |
| **`docs/` guide** | New examples, wording |
| **`plan/01-features.md`** | New features / wave scope |
| **`fixtures.yaml` + fixture files** | New IDs or `expect` blocks |

## Agent / developer checklist

1. **Spec first** — behaviour comes from `docs/implementation/`; the feature catalog is the build checklist.
2. **RF + PF** — plain / row zip, and `.excsv.pack.zip`. Pack keys on row files stay WARN+ignore.
3. **Tests** — drive from `fixtures.yaml`; add unit tests for CLI transforms (append/sort/schema) that fixtures do not cover.
4. **Before implementing** — refresh snapshots if the local copies 404 or look stale.
5. **CSVW is not in the format** — the only surface is `export csvw`, which writes a sidecar. There is no CSVW reader, by design.

## Refresh from upstream

**Windows (PowerShell, repo root):**

```powershell
.\scripts\sync-upstream.ps1
# or: .\makefile.ps1 sync-upstream
```

**Git Bash / WSL / macOS / Linux:**

```bash
./scripts/sync-upstream.sh
# or: make sync-upstream
```

Partial:

```powershell
.\scripts\sync-upstream.ps1 -SpecsOnly
.\scripts\sync-upstream.ps1 -FixturesOnly
```

Pack fixtures (`pack/…`) are generated by `fixtures/generate/make_pack_fixtures.py` (same as zip).
