# Sync normative docs and test fixtures from boligolov/excsv (upstream).
#
# Usage (repo root):
#   .\scripts\sync-upstream.ps1              # specs + manifest + fixture files
#   .\scripts\sync-upstream.ps1 -SpecsOnly
#   .\scripts\sync-upstream.ps1 -FixturesOnly

param(
    [switch]$SpecsOnly,
    [switch]$FixturesOnly
)

$ErrorActionPreference = 'Stop'

try {
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $RepoRoot

$UpstreamBase = 'https://raw.githubusercontent.com/boligolov/excsv/master'
$FixtureBase = "$UpstreamBase/fixtures"

# Normative LLM spec: root hub + per-topic files under docs/llm/ (upstream split).
$LlmTopicFiles = @(
    'README.md',
    'aggregations.md',
    'canonical-example.md',
    'checksum.md',
    'columns.md',
    'csvw.md',
    'data-section.md',
    'error-handling.md',
    'file-metadata.md',
    'file-structure.md',
    'header.md',
    'identity.md',
    'license.md',
    'meta-lines.md',
    'parsing.md',
    'quick-reference.md',
    'reserved.md',
    'serialization.md',
    'sql.md',
    'zip.md'
)

$SpecDownloads = @(
    @{ url = "$UpstreamBase/README-LLM.md"; out = 'docs/downloaded/README-LLM.md' },
    @{ url = "$UpstreamBase/plan/README.md"; out = 'docs/downloaded/plan-README.md' },
    @{ url = "$UpstreamBase/plan/01-features.md"; out = 'docs/downloaded/plan-01-features.md' },
    @{ url = "$UpstreamBase/plan/02-fixtures.md"; out = 'docs/downloaded/plan-02-fixtures.md' },
    @{ url = "$FixtureBase/fixtures.yaml"; out = 'test/fixtures/fixtures.yaml' }
)
foreach ($name in $LlmTopicFiles) {
    $SpecDownloads += @{ url = "$UpstreamBase/docs/llm/$name"; out = "docs/downloaded/llm/$name" }
}

function Get-ManifestPaths {
    param([string]$ManifestPath)
    $paths = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($line in Get-Content -LiteralPath $ManifestPath) {
        if ($line -match '^\s*- id:\s*(.+)$') {
            [void]$paths.Add($Matches[1].Trim())
        }
        elseif ($line -match '^\s*data_sibling:\s*(.+)$') {
            [void]$paths.Add($Matches[1].Trim())
        }
    }
    return @($paths | Sort-Object)
}

function Save-RemoteFile {
    param([string]$Url, [string]$OutPath)
    $dir = Split-Path -Parent $OutPath
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    Write-Host "  $OutPath"
    Invoke-WebRequest -Uri $Url -OutFile $OutPath -UseBasicParsing
}

$syncSpecs = -not $FixturesOnly
$syncFixtures = -not $SpecsOnly

if ($syncSpecs) {
    Write-Host 'Downloading spec/plan snapshots...'
    New-Item -ItemType Directory -Force -Path docs/downloaded, docs/downloaded/llm, test/fixtures | Out-Null
    foreach ($item in $SpecDownloads) {
        Save-RemoteFile -Url $item.url -OutPath $item.out
    }
}

if ($syncFixtures) {
    $manifestPath = Join-Path $RepoRoot 'test/fixtures/fixtures.yaml'
    if (-not (Test-Path -LiteralPath $manifestPath)) {
        if (-not $syncSpecs) {
            New-Item -ItemType Directory -Force -Path test/fixtures | Out-Null
            Save-RemoteFile -Url "$FixtureBase/fixtures.yaml" -OutPath 'test/fixtures/fixtures.yaml'
        }
        else {
            throw "Manifest missing at $manifestPath (run without -FixturesOnly first, or fetch specs)"
        }
    }

    $paths = Get-ManifestPaths -ManifestPath $manifestPath
    Write-Host "Downloading $($paths.Count) fixture file(s) from manifest..."
    foreach ($rel in $paths) {
        if ($rel -notmatch '^(plain|zip|pack)/') {
            Write-Warning "Skipping unexpected path: $rel"
            continue
        }
        $out = Join-Path 'test/fixtures' $rel
        Save-RemoteFile -Url "$FixtureBase/$rel" -OutPath $out
    }
}

Write-Host 'Done.'
exit 0
}
catch {
    Write-Error $_
    exit 1
}
