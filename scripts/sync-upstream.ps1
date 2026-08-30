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

$GuideFiles = @(
    'aggregations.md',
    'checksum.md',
    'columns.md',
    'data-section.md',
    'file-metadata.md',
    'file-structure.md',
    'full-example.md',
    'header.md',
    'introduction.md',
    'json.md',
    'license.md',
    'meta-lines.md',
    'pack.md',
    'prior-art.md',
    'sql.md',
    'zip.md'
)

$ImplementationFiles = @(
    'README.md',
    'aggregations.md',
    'checksum.md',
    'columns.md',
    'data-section.md',
    'error-handling.md',
    'file-metadata.md',
    'file-structure.md',
    'full-example.md',
    'header.md',
    'introduction.md',
    'json.md',
    'license.md',
    'meta-lines.md',
    'pack.md',
    'prior-art.md',
    'sql.md',
    'zip.md'
)

$SpecDownloads = @(
    @{ url = "$UpstreamBase/README.md"; out = 'docs/downloaded/README.md' },
    @{ url = "$UpstreamBase/docs/README.md"; out = 'docs/downloaded/guide/README.md' },
    @{ url = "$UpstreamBase/plan/README.md"; out = 'docs/downloaded/plan-README.md' },
    @{ url = "$UpstreamBase/plan/01-features.md"; out = 'docs/downloaded/plan-01-features.md' },
    @{ url = "$UpstreamBase/plan/02-fixtures.md"; out = 'docs/downloaded/plan-02-fixtures.md' },
    @{ url = "$FixtureBase/fixtures.yaml"; out = 'test/fixtures/fixtures.yaml' },
    @{ url = "$UpstreamBase/schema/excsv.schema.json"; out = 'docs/downloaded/schema/excsv.schema.json' },
    @{ url = "$UpstreamBase/schema/example.excsv.json"; out = 'docs/downloaded/schema/example.excsv.json' }
)
foreach ($name in $GuideFiles) {
    $SpecDownloads += @{ url = "$UpstreamBase/docs/$name"; out = "docs/downloaded/guide/$name" }
}
foreach ($name in $ImplementationFiles) {
    $SpecDownloads += @{ url = "$UpstreamBase/docs/implementation/$name"; out = "docs/downloaded/implementation/$name" }
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
    New-Item -ItemType Directory -Force -Path docs/downloaded, docs/downloaded/guide, docs/downloaded/implementation, docs/downloaded/schema, test/fixtures | Out-Null
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
        if ($rel -like 'pack/*') {
            continue
        }
        if ($rel -like 'zip/*') {
            continue
        }
        $out = Join-Path 'test/fixtures' $rel
        try {
            Save-RemoteFile -Url "$FixtureBase/$rel" -OutPath $out
        }
        catch {
            Write-Warning "Skip $rel : $_"
        }
    }

    Write-Host 'Generating zip fixtures from upstream generator...'
    $specDir = Join-Path $env:TEMP 'excsv-spec-sync'
    if (-not (Test-Path (Join-Path $specDir '.git'))) {
        if (Test-Path $specDir) { Remove-Item -Recurse -Force $specDir }
        git clone --depth 1 https://github.com/boligolov/excsv.git $specDir
    }
    python (Join-Path $specDir 'fixtures\generate\make_zip_fixtures.py')
    python (Join-Path $specDir 'fixtures\generate\make_pack_fixtures.py')
    $zipSrc = Join-Path $specDir 'fixtures\zip'
    $zipDst = Join-Path $RepoRoot 'test\fixtures\zip'
    if (Test-Path $zipDst) { Remove-Item -Recurse -Force $zipDst }
    Copy-Item -Recurse $zipSrc $zipDst
    $packSrc = Join-Path $specDir 'fixtures\pack'
    $packDst = Join-Path $RepoRoot 'test\fixtures\pack'
    if (Test-Path $packDst) { Remove-Item -Recurse -Force $packDst }
    Copy-Item -Recurse $packSrc $packDst
}

Write-Host 'Done.'
exit 0
}
catch {
    Write-Error $_
    exit 1
}
