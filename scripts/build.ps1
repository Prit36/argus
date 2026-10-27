# Builds Argus.
#
#   scripts\build.ps1          production build: build\windows-amd64\Argus.exe
#                              and its installer (mygo build)
#   scripts\build.ps1 -Dev     development build with a console: build\dev\Argus.exe
#   scripts\build.ps1 -Test    vet and run the tests
#
# Go comes from PATH, or GO_EXE.
param([switch]$Dev, [switch]$Test)
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot)
$go = if ($env:GO_EXE) { $env:GO_EXE } else { (Get-Command go).Source }
$env:PATH = (Split-Path $go) + ";" + $env:PATH

if ($Test) {
    & $go vet ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    & $go test ./...
    exit $LASTEXITCODE
}
if ($Dev) {
    & $go build -trimpath -o build\dev\Argus.exe .
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    Copy-Item -LiteralPath LICENSE, THIRD_PARTY_NOTICES.txt -Destination build\dev
    exit $LASTEXITCODE
}
# Remove machine-specific source paths from distributable binaries.
$env:GOFLAGS = ($env:GOFLAGS + ' -trimpath').Trim()
& $go tool mygo build
exit $LASTEXITCODE
