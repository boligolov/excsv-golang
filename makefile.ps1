# excsv-cli build script (Windows PowerShell)
#
# Usage:
#   .\makefile.ps1              # build local -> bin\excsv.exe
#   .\makefile.ps1 rebuild      # flush Go cache + force full rebuild
#   .\makefile.ps1 build-all    # cross-compile all platforms
#   .\makefile.ps1 test
#   .\makefile.ps1 clean

param(
    [Parameter(Position = 0)]
    [ValidateSet('build', 'rebuild', 'build-all', 'test', 'clean', 'list', 'help')]
    [string]$Target = 'build'
)

$ErrorActionPreference = 'Stop'

$Binary  = 'excsv'
$Cmd     = './cmd/excsv'
$BinDir  = 'bin'
$Module  = 'github.com/boligolov/excsv-golang/internal/cli'

$Platforms = @(
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Out = "$Binary-windows-amd64.exe" },
    @{ GOOS = 'windows'; GOARCH = 'arm64'; Out = "$Binary-windows-arm64.exe" },
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Out = "$Binary-linux-amd64" },
    @{ GOOS = 'linux';   GOARCH = 'arm64'; Out = "$Binary-linux-arm64" },
    @{ GOOS = 'darwin';  GOARCH = 'amd64'; Out = "$Binary-darwin-amd64" },
    @{ GOOS = 'darwin';  GOARCH = 'arm64'; Out = "$Binary-darwin-arm64" }
)

function Get-LdFlags {
    $buildTime = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    return "-s -w -X ${Module}.Version=0.2.0 -X ${Module}.BuildTime=$buildTime"
}

function Get-NativePlatform {
    if ($Env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
        return @{ GOOS = 'windows'; GOARCH = 'arm64' }
    }
    return @{ GOOS = 'windows'; GOARCH = 'amd64' }
}

function Save-GoPlatform {
    return @{
        GOOS   = $env:GOOS
        GOARCH = $env:GOARCH
    }
}

function Restore-GoPlatform {
    param($Saved)
    foreach ($key in @('GOOS', 'GOARCH')) {
        if ($null -eq $Saved[$key] -or $Saved[$key] -eq '') {
            Remove-Item "Env:$key" -ErrorAction SilentlyContinue
        }
        else {
            Set-Item "Env:$key" $Saved[$key]
        }
    }
}

function Use-GoPlatform {
    param([string]$GOOS, [string]$GOARCH)
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
}

function Clear-GoPlatform {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}

function Ensure-BinDir {
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
}

function Remove-StaleBinary {
    param([string]$Path)
    if (-not (Test-Path $Path)) {
        return
    }
    try {
        Remove-Item -Force $Path
    }
    catch {
        throw "Cannot remove stale binary '$Path'. Close any running excsv process and retry."
    }
}

function Invoke-GoBuild {
    param(
        [string]$Out,
        [switch]$ForceAll
    )
    $ldFlags = Get-LdFlags
    $args = @('build', '-trimpath', '-ldflags', $ldFlags, '-o', $Out)
    if ($ForceAll) {
        $args += '-a'
    }
    $args += $Cmd
    & go @args
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}

function Build-Local {
    param([switch]$ForceAll)
    Ensure-BinDir
    $out = Join-Path $BinDir "$Binary.exe"
    Remove-StaleBinary $out

    $saved = Save-GoPlatform
    try {
        $native = Get-NativePlatform
        Use-GoPlatform $native.GOOS $native.GOARCH
        Invoke-GoBuild -Out $out -ForceAll:$ForceAll
        Write-Host "-> $out ($($native.GOOS)/$($native.GOARCH))"
    }
    finally {
        Restore-GoPlatform $saved
    }
}

function Build-Platform {
    param($Platform, [switch]$ForceAll)
    Ensure-BinDir
    $out = Join-Path $BinDir $Platform.Out
    Remove-StaleBinary $out

    $saved = Save-GoPlatform
    try {
        Use-GoPlatform $Platform.GOOS $Platform.GOARCH
        Invoke-GoBuild -Out $out -ForceAll:$ForceAll
        Write-Host "-> $out ($($Platform.GOOS)/$($Platform.GOARCH))"
    }
    finally {
        Restore-GoPlatform $saved
    }
}

function Build-All {
    param([switch]$ForceAll)
    foreach ($p in $Platforms) {
        Build-Platform $p -ForceAll:$ForceAll
    }
    Clear-GoPlatform
    Write-Host "All binaries in $BinDir\"
}

function Invoke-Rebuild {
    Write-Host 'Flushing Go build cache...'
    go clean -cache -testcache
    if ($LASTEXITCODE -ne 0) {
        throw 'go clean failed'
    }
    Build-Local -ForceAll
}

function Invoke-Test {
    go test ./...
}

function Remove-Artifacts {
    if (Test-Path $BinDir) {
        Remove-Item -Recurse -Force $BinDir
    }
    Write-Host "Removed $BinDir\"
}

function Show-Help {
    @"
excsv-cli build (PowerShell)

  .\makefile.ps1 [target]

Targets:
  build       Build for current Windows -> bin\excsv.exe (default)
  rebuild     Flush Go cache + force full rebuild (-a)
  build-all   Cross-compile windows/linux/darwin amd64+arm64 -> bin\
  test        go test ./...
  clean       Remove bin\
  list        Show targets
  help        This message

Local build always targets native Windows (amd64 or arm64).
Use bin\excsv-windows-amd64.exe from build-all on x64 Windows if unsure.

Tip: run `excsv version` after rebuild — the built timestamp confirms you picked up changes.
"@
}

switch ($Target) {
    'build'     { Build-Local }
    'rebuild'   { Invoke-Rebuild }
    'build-all' { Build-All }
    'test'      { Invoke-Test }
    'clean'     { Remove-Artifacts }
    'list'      { Show-Help }
    'help'      { Show-Help }
}
