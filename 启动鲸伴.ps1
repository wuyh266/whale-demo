param([switch]$NoBrowser)
& (Join-Path $PSScriptRoot 'start.ps1') -NoBrowser:$NoBrowser
exit $LASTEXITCODE
