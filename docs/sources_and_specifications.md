# Sources and Specifications

Reference index for building **excsv-cli** (Go). Use the upstream [boligolov/excsv](https://github.com/boligolov/excsv) repo as the normative source; local copies are convenience snapshots, not forks.

## Upstream links

| Resource | URL | Local copy |
| --- | --- | --- |
| ExCSV specification (LLM hub) | https://github.com/boligolov/excsv/blob/master/README-LLM.md | [`docs/downloaded/README-LLM.md`](downloaded/README-LLM.md) |
| LLM spec topics | https://github.com/boligolov/excsv/tree/master/docs/llm | [`docs/downloaded/llm/`](downloaded/llm/) |
| Implementation plan (overview) | https://github.com/boligolov/excsv/blob/master/plan/README.md | [`docs/downloaded/plan-README.md`](downloaded/plan-README.md) |
| Feature catalog (step 1) | https://github.com/boligolov/excsv/blob/master/plan/01-features.md | [`docs/downloaded/plan-01-features.md`](downloaded/plan-01-features.md) |
| Test fixtures spec (step 2) | https://github.com/boligolov/excsv/blob/master/plan/02-fixtures.md | [`docs/downloaded/plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) |
| Fixture manifest | https://github.com/boligolov/excsv/blob/master/fixtures/fixtures.yaml | [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml) |

Raw download URLs (for refresh scripts):

```
https://raw.githubusercontent.com/boligolov/excsv/master/README-LLM.md
https://raw.githubusercontent.com/boligolov/excsv/master/docs/llm/<topic>.md   # see scripts/sync-upstream.*
https://raw.githubusercontent.com/boligolov/excsv/master/plan/README.md
https://raw.githubusercontent.com/boligolov/excsv/master/plan/01-features.md
https://raw.githubusercontent.com/boligolov/excsv/master/plan/02-fixtures.md
https://raw.githubusercontent.com/boligolov/excsv/master/fixtures/fixtures.yaml
```

## What each source means

### `README-LLM.md` + `docs/llm/*.md` — normative spec

**Authority:** highest. If behaviour is not defined here, do not invent it — update the spec upstream first.

Upstream split the monolithic LLM reference into a **hub** ([`README-LLM.md`](downloaded/README-LLM.md)) and **per-topic files** under [`docs/downloaded/llm/`](downloaded/llm/) (mirrors upstream `docs/llm/`). Topics include file structure, header, meta lines, aggregations, SQL, parsing/serialization algorithms, ZIP, error handling, etc.

**Use when:** implementing parse/serialize logic, error kinds, canonical output, strict vs lenient mode, container handling. Start from the hub index, then open the relevant `llm/*.md` topic.

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

Lists fixture IDs, which features they exercise, and expected parse result (ok / error kind / warnings). Actual `.excsv` (and sibling `.csv`/`.tsv`) files live in the upstream `fixtures/` tree; sync them locally with the scripts below (not committed — see `.gitignore`).

**Use when:** implementing table-driven tests; `go test` reads `test/fixtures/` after sync.

## Local layout in this repo

```
docs/
├── sources_and_specifications.md   ← this file
└── downloaded/                     ← spec + plan snapshots (gitignored; refresh from upstream)
    ├── README-LLM.md               ← hub / topic index
    ├── llm/                         ← normative topics (aggregations.md, parsing.md, …)
    ├── plan-README.md
    ├── plan-01-features.md
    └── plan-02-fixtures.md

test/
└── fixtures/                       ← gitignored; sync from upstream
    ├── fixtures.yaml               ← manifest (lists every fixture id + expect)
    ├── plain/valid|invalid/        ← .excsv (+ sidecar siblings)
    └── zip/valid|invalid/          ← .excsv.zip (wave 2)
```

`scripts/sync-upstream.ps1` and `scripts/sync-upstream.sh` automate both doc snapshots and fixture files.

## What to refresh periodically (not often)

Refresh when upstream `boligolov/excsv` changes in ways that affect implementation — typically after spec/plan releases or before starting a new wave. No fixed schedule; diff upstream when tests or behaviour feel out of date.

| Asset | Refresh trigger | Action |
| --- | --- | --- |
| **`README-LLM.md` + `docs/llm/*.md`** | Spec version bump, new meta keys, ZIP/pack rules, error semantics | Re-run sync (hub + `docs/downloaded/llm/`); re-read changed topics; update parser/CLI; extend error kinds |
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

## Refresh from upstream

### One command (recommended)

**Windows (PowerShell, repo root):**

```powershell
.\scripts\sync-upstream.ps1
```

**Git Bash / WSL / macOS / Linux:**

```bash
./scripts/sync-upstream.sh
# or: make sync-upstream
```

This downloads the LLM hub, all `docs/llm/*.md` topic files, three plan snapshots, `fixtures.yaml`, then walks the manifest and fetches every `id:` path and `data_sibling:` path under `test/fixtures/`.

Partial sync:

```powershell
.\scripts\sync-upstream.ps1 -SpecsOnly      # docs/downloaded + fixtures.yaml only
.\scripts\sync-upstream.ps1 -FixturesOnly   # re-download .excsv/.zip files from existing manifest
```

```bash
./scripts/sync-upstream.sh --specs-only
./scripts/sync-upstream.sh --fixtures-only
```

### What the fixture sync downloads

The script does **not** mirror the whole upstream `fixtures/` tree. It reads [`test/fixtures/fixtures.yaml`](../test/fixtures/fixtures.yaml) and downloads only:

| Manifest field | Example | Local path |
| --- | --- | --- |
| `- id:` | `plain/valid/001_minimal_header_only.excsv` | `test/fixtures/plain/valid/001_….excsv` |
| `data_sibling:` | `plain/valid/037_sidecar_csv_sibling.csv` | `test/fixtures/plain/valid/037_….csv` |

Raw URL pattern: `https://raw.githubusercontent.com/boligolov/excsv/master/fixtures/<id>`.

Pack fixtures (`pack/…`) are skipped until wave 3; the Go runner only exercises `plain/*` and `zip/*` today.

### Manual / CI alternatives

**Spec + manifest only (inline PowerShell):**

```powershell
New-Item -ItemType Directory -Force -Path docs/downloaded, test/fixtures | Out-Null
.\scripts\sync-upstream.ps1 -SpecsOnly
# or download hub + plan + docs/llm/*.md manually — see scripts/sync-upstream.ps1 for the topic list
```

**Full fixture tree (clone):** CI uses a shallow clone of [boligolov/excsv](https://github.com/boligolov/excsv) and copies `fixtures/plain` and `fixtures/zip` — see [`.github/workflows/ci.yml`](../.github/workflows/ci.yml). Equivalent locally:

```bash
git clone --depth 1 https://github.com/boligolov/excsv.git /tmp/excsv-spec
cp /tmp/excsv-spec/fixtures/fixtures.yaml test/fixtures/
cp -r /tmp/excsv-spec/fixtures/plain test/fixtures/
cp -r /tmp/excsv-spec/fixtures/zip test/fixtures/  # if present
```

See [`plan-02-fixtures.md`](downloaded/plan-02-fixtures.md) for directory layout and manifest schema.
