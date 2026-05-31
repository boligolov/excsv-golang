# excsv-cli build script (Windows PowerShell)
#
# Usage:
#   .\makefile.ps1              # build local -> bin\excsv.exe
#   .\makefile.ps1 build-all    # cross-compile all platforms
#   .\makefile.ps1 test
#   .\makefile.ps1 clean
#   .\makefile.ps1 list

param(
    [Parameter(Position = 0)]
    [ValidateSet('build', 'build-all', 'test', 'clean', 'list', 'help')]
    [string]$Target = 'build'
)

$ErrorActionPreference = 'Stop'

$Binary  = 'excsv'
$Cmd     = './cmd/excsv'
$BinDir  = 'bin'
$LdFlags = '-s -w'

$Platforms = @(
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Out = "$Binary-windows-amd64.exe" },
    @{ GOOS = 'windows'; GOARCH = 'arm64'; Out = "$Binary-windows-arm64.exe" },
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Out = "$Binary-linux-amd64" },
    @{ GOOS = 'linux';   GOARCH = 'arm64'; Out = "$Binary-linux-arm64" },
    @{ GOOS = 'darwin';  GOARCH = 'amd64'; Out = "$Binary-darwin-amd64" },
    @{ GOOS = 'darwin';  GOARCH = 'arm64'; Out = "$Binary-darwin-arm64" }
)

function Ensure-BinDir {
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
}

function Build-Local {
    Ensure-BinDir
    $out = Join-Path $BinDir "$Binary.exe"
    go build -ldflags $LdFlags -o $out $Cmd
    Write-Host "-> $out"
}

function Build-Platform {
    param($Platform)
    Ensure-BinDir
    $out = Join-Path $BinDir $Platform.Out
    $env:GOOS = $Platform.GOOS
    $env:GOARCH = $Platform.GOARCH
    go build -ldflags $LdFlags -o $out $Cmd
    Write-Host "-> $out"
}

function Build-All {
    foreach ($p in $Platforms) {
        Build-Platform $p
    }
    Write-Host "All binaries in $BinDir\"
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
  build-all   Cross-compile windows/linux/darwin amd64+arm64 -> bin\
  test        go test ./...
  clean       Remove bin\
  list        Show targets
  help        This message
"@
}

switch ($Target) {
    'build'     { Build-Local }
    'build-all' { Build-All }
    'test'      { Invoke-Test }
    'clean'     { Remove-Artifacts }
    'list'      { Show-Help }
    'help'      { Show-Help }
}
