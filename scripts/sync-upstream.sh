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
  mkdir -p docs/downloaded test/fixtures
  download "${UPSTREAM_BASE}/README-LLM.md" docs/downloaded/README-LLM.md
  download "${UPSTREAM_BASE}/plan/README.md" docs/downloaded/plan-README.md
  download "${UPSTREAM_BASE}/plan/01-features.md" docs/downloaded/plan-01-features.md
  download "${UPSTREAM_BASE}/plan/02-fixtures.md" docs/downloaded/plan-02-fixtures.md
  download "${FIXTURE_BASE}/fixtures.yaml" test/fixtures/fixtures.yaml
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
      plain/*|zip/*|pack/*) ;;
      *)
        echo "warning: skipping unexpected path: $rel" >&2
        continue
        ;;
    esac
    download "${FIXTURE_BASE}/${rel}" "test/fixtures/${rel}"
  done
fi

echo "Done."
