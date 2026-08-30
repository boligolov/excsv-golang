#!/usr/bin/env bash
# Print the CLI version string for -ldflags.
# Prefer the nearest annotated tag (v*); fall back to internal/cli/version.go.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

if git -C "$root" describe --tags --match 'v*' >/dev/null 2>&1; then
  git -C "$root" describe --tags --match 'v*' --always --dirty | sed 's/^v//'
else
  sed -n 's/.*Version   = "\([^"]*\)".*/\1/p' "$root/internal/cli/version.go" | head -n1
fi
