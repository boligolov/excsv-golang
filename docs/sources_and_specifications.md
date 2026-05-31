# Sources and Specifications

Reference index for building **excsv-cli** (Go). Use the upstream [boligolov/excsv](https://github.com/boligolov/excsv) repo as the normative source; local copies are convenience snapshots, not forks.

## Upstream links

| Resource | URL | Local copy |
| --- | --- | --- |
| ExCSV specification (LLM reference) | https://github.com/boligolov/excsv/blob/master/README-LLM.md | [`docs/downloaded/README-LLM.md`](downloaded/README-LLM.md) |
| Implementation plan (overview) | https://github.com/boligolov/excsv/blob/master/plan/README.md | [`docs/downloaded/plan-README.md`](downloaded/plan-README.md) |
| Feature catalog (step 1) | https://github.com/boligolov/excsv/blob/master/plan/01-features.md | [`docs/downloaded/plan-01-features.md`](downloaded/plan-01-features.md) |
| Test fixtures spec (step 2) | https://github.com/boligolov/excsv/blob/master/plan/02-fixtures.md | [`docs/downloaded/plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) |
| Fixture manifest | https://github.com/boligolov/excsv/blob/master/fixtures/fixtures.yaml | [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml) |

Raw download URLs (for refresh scripts):

```
https://raw.githubusercontent.com/boligolov/excsv/master/README-LLM.md
https://raw.githubusercontent.com/boligolov/excsv/master/plan/README.md
https://raw.githubusercontent.com/boligolov/excsv/master/plan/01-features.md
https://raw.githubusercontent.com/boligolov/excsv/master/plan/02-fixtures.md
https://raw.githubusercontent.com/boligolov/excsv/master/fixtures/fixtures.yaml
```

## What each source means

### `README-LLM.md` — normative spec

**Authority:** highest. If behaviour is not defined here, do not invent it — update the spec upstream first.

Covers ExCSV v0.2 (Draft): file layout (header → `#` meta → data), header line (`#!excsv`), delimiters/quoting, meta keys (`#%`, `#@`, `#$`, etc.), CSVW embedding, checksums, ZIP container (`.excsv.zip`), encoding, SQL meta keys. This is what parsers, serializers, and the CLI must implement.

**Use when:** implementing parse/serialize logic, error kinds, canonical output, strict vs lenient mode, container handling.

### `plan/README.md` — implementation strategy

**Authority:** process and sequencing, not format semantics.

Defines three tracks (Go primary, Python parity, CLI cookbook), wave gating (plain → zip → pack), and rules: spec-first, no partial waves, reserved pack names stay reserved in v0.2 row readers.

**Use when:** deciding build order, scope for a wave, and what *not* to implement yet.

### `plan/01-features.md` — feature catalog

**Authority:** source of truth for *what* to build (capability map), derived from the spec.

Abstract features (A–…) mapped to storage forms: **plain** (`.excsv`), **zip** (`.excsv.zip`), **pack** (`.excsv.pack.zip`, post-v0.3). Each row notes Mode A (metadata-only) vs Mode B (data-aware).

**Use when:** designing CLI commands, package API surface, and test coverage checklist. Cross-reference feature IDs (e.g. `A4`, `B1`) in tests and commit messages.

### `plan/02-fixtures.md` — fixture corpus rules

**Authority:** how tests are structured and named; complements the manifest.

Defines directory layout (`fixtures/plain|zip|pack/valid|invalid/`), naming (`NNN_<slug>.excsv`), manifest schema, generation scripts, and parity rules (Go and Python must agree).

**Use when:** wiring Go tests, adding fixtures, interpreting manifest fields (`exercises`, `expect.parse`, `expect.errors`).

### `fixtures/fixtures.yaml` — test manifest

**Authority:** expected outcomes per fixture; test runners walk this file, not the directory tree alone.

Lists fixture IDs, which features they exercise, and expected parse result (ok / error kind / warnings). Actual `.excsv` files live in the upstream `fixtures/` tree (not yet vendored here beyond the manifest).

**Use when:** implementing table-driven tests; pointing tests at upstream fixture files or a synced copy.

## Local layout in this repo

```
docs/
├── sources_and_specifications.md   ← this file
└── downloaded/                     ← spec + plan snapshots (refresh from upstream)
    ├── README-LLM.md
    ├── plan-README.md
    ├── plan-01-features.md
    └── plan-02-fixtures.md

test/
└── fixtures/
    └── fixtures.yaml               ← manifest only; .excsv files TBD from upstream tree
```

## What to refresh periodically (not often)

Refresh when upstream `boligolov/excsv` changes in ways that affect implementation — typically after spec/plan releases or before starting a new wave. No fixed schedule; diff upstream when tests or behaviour feel out of date.

| Asset | Refresh trigger | Action |
| --- | --- | --- |
| **`README-LLM.md`** | Spec version bump, new meta keys, ZIP/pack rules, error semantics | Re-download to `docs/downloaded/`; re-read changed sections; update parser/CLI; extend error kinds |
| **`plan/01-features.md`** | New features, changed RF/PF matrix, wave scope | Re-download; reconcile CLI command tree and backlog |
| **`plan/02-fixtures.md`** | New fixture categories, naming/manifest schema changes | Re-download; adjust test harness |
| **`fixtures/fixtures.yaml`** | New fixture IDs, changed `expect` blocks, new `error_kinds` | Re-download to `test/fixtures/`; sync `.excsv` files from upstream `fixtures/` if manifest references new entries |
| **`plan/README.md`** | Wave sequencing or track rules change | Re-download; update milestone docs only |
| **Fixture binary files** (`fixtures/plain/…`, etc.) | Manifest adds or changes fixture paths | Clone or sparse-checkout upstream `fixtures/`; do not hand-edit |

**Do not periodically refresh** unless upstream changed: this index file, Go module deps, or local implementation notes.

## Agent / developer checklist

1. **Spec first** — behaviour comes from `README-LLM.md`; features catalog is the build checklist.
2. **Wave 1 scope** — row plain (`.excsv`) only; ignore pack semantics on row readers; zip is wave 2.
3. **Tests** — drive from `fixtures.yaml`; feature IDs in `exercises` map to `01-features.md`.
4. **Parity** — same manifest drives Go and Python; divergence means spec ambiguity or a bug.
5. **Before implementing** — read local snapshots; if stale, refresh downloads and skim upstream diff.

## PowerShell refresh (Windows)

```powershell
New-Item -ItemType Directory -Force -Path docs/downloaded, test/fixtures | Out-Null
@(
  @{ url = "https://raw.githubusercontent.com/boligolov/excsv/master/README-LLM.md"; out = "docs/downloaded/README-LLM.md" },
  @{ url = "https://raw.githubusercontent.com/boligolov/excsv/master/plan/README.md"; out = "docs/downloaded/plan-README.md" },
  @{ url = "https://raw.githubusercontent.com/boligolov/excsv/master/plan/01-features.md"; out = "docs/downloaded/plan-01-features.md" },
  @{ url = "https://raw.githubusercontent.com/boligolov/excsv/master/plan/02-fixtures.md"; out = "docs/downloaded/plan-02-fixtures.md" },
  @{ url = "https://raw.githubusercontent.com/boligolov/excsv/master/fixtures/fixtures.yaml"; out = "test/fixtures/fixtures.yaml" }
) | ForEach-Object { Invoke-WebRequest -Uri $_.url -OutFile $_.out -UseBasicParsing }
```

To pull fixture files, clone or sparse-checkout the upstream repo’s `fixtures/` directory (see `02-fixtures.md` for layout).
