#!/usr/bin/env bash
# Sync normative docs and test fixtures from boligolov/excsv (upstream).
#
# Usage (repo root):
#   ./scripts/sync-upstream.sh              # specs + manifest + fixture files
#   ./scripts/sync-upstream.sh --specs-only
#   ./scripts/sync-upstream.sh --fixtures-only

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

UPSTREAM_BASE="https://raw.githubusercontent.com/boligolov/excsv/master"
FIXTURE_BASE="${UPSTREAM_BASE}/fixtures"

# Human-readable guide: docs/*.md. Normative parser spec: docs/implementation/*.md.
GUIDE_FILES=(
  aggregations.md
  checksum.md
  columns.md
  data-section.md
  file-metadata.md
  file-structure.md
  full-example.md
  header.md
  introduction.md
  json.md
  license.md
  meta-lines.md
  pack.md
  prior-art.md
  sql.md
  zip.md
)

IMPLEMENTATION_FILES=(
  README.md
  aggregations.md
  checksum.md
  columns.md
  data-section.md
  error-handling.md
  file-metadata.md
  file-structure.md
  full-example.md
  header.md
  introduction.md
  json.md
  license.md
  meta-lines.md
  pack.md
  prior-art.md
  sql.md
  zip.md
)

SPECS_ONLY=0
FIXTURES_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --specs-only) SPECS_ONLY=1 ;;
    --fixtures-only) FIXTURES_ONLY=1 ;;
    -h|--help)
      echo "Usage: $0 [--specs-only | --fixtures-only]"
      exit 0
      ;;
    *) echo "Unknown option: $arg" >&2; exit 1 ;;
  esac
done

download() {
  local url="$1" out="$2"
  mkdir -p "$(dirname "$out")"
  echo "  $out"
  curl -fsSL "$url" -o "$out"
}

if [[ "$FIXTURES_ONLY" -eq 0 ]]; then
  echo "Downloading spec/plan snapshots..."
  mkdir -p docs/downloaded/guide docs/downloaded/implementation docs/downloaded/schema test/fixtures
  download "${UPSTREAM_BASE}/README.md" docs/downloaded/README.md
  download "${UPSTREAM_BASE}/docs/README.md" docs/downloaded/guide/README.md
  download "${UPSTREAM_BASE}/plan/README.md" docs/downloaded/plan-README.md
  download "${UPSTREAM_BASE}/plan/01-features.md" docs/downloaded/plan-01-features.md
  download "${UPSTREAM_BASE}/plan/02-fixtures.md" docs/downloaded/plan-02-fixtures.md
  download "${FIXTURE_BASE}/fixtures.yaml" test/fixtures/fixtures.yaml
  download "${UPSTREAM_BASE}/schema/excsv.schema.json" docs/downloaded/schema/excsv.schema.json
  download "${UPSTREAM_BASE}/schema/example.excsv.json" docs/downloaded/schema/example.excsv.json
  for name in "${GUIDE_FILES[@]}"; do
    download "${UPSTREAM_BASE}/docs/${name}" "docs/downloaded/guide/${name}"
  done
  for name in "${IMPLEMENTATION_FILES[@]}"; do
    download "${UPSTREAM_BASE}/docs/implementation/${name}" "docs/downloaded/implementation/${name}"
  done
fi

if [[ "$SPECS_ONLY" -eq 0 ]]; then
  MANIFEST="test/fixtures/fixtures.yaml"
  if [[ ! -f "$MANIFEST" ]]; then
    mkdir -p test/fixtures
    download "${FIXTURE_BASE}/fixtures.yaml" "$MANIFEST"
  fi

  mapfile -t PATHS < <(
    grep -E '^\s*- id:|^\s*data_sibling:' "$MANIFEST" \
      | sed -E 's/^\s*- id:\s*//; s/^\s*data_sibling:\s*//' \
      | sort -u
  )

  echo "Downloading ${#PATHS[@]} fixture file(s) from manifest..."
  for rel in "${PATHS[@]}"; do
    case "$rel" in
      pack/*)
        continue
        ;;
      zip/*)
        continue
        ;;
      plain/*) ;;
      *)
        echo "warning: skipping unexpected path: $rel" >&2
        continue
        ;;
    esac
    download "${FIXTURE_BASE}/${rel}" "test/fixtures/${rel}" || echo "warning: skip ${rel}" >&2
  done

  echo "Generating zip fixtures from upstream generator..."
  SPEC_DIR="${TMPDIR:-/tmp}/excsv-spec-sync"
  if [[ ! -d "$SPEC_DIR/.git" ]]; then
    rm -rf "$SPEC_DIR"
    git clone --depth 1 https://github.com/boligolov/excsv.git "$SPEC_DIR"
  fi
  python3 "$SPEC_DIR/fixtures/generate/make_zip_fixtures.py"
  python3 "$SPEC_DIR/fixtures/generate/make_pack_fixtures.py"
  rm -rf test/fixtures/zip test/fixtures/pack
  cp -r "$SPEC_DIR/fixtures/zip" test/fixtures/zip
  cp -r "$SPEC_DIR/fixtures/pack" test/fixtures/pack
fi

echo "Done."
