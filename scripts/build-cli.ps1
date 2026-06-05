$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path "$PSScriptRoot\.."
Set-Location "$RepoRoot"

$env:GOTELEMETRY = "off"
$env:GOCACHE = Join-Path $env:TEMP "twitcasting-recorder-go-build-cache"

go build -o apps\cli\twitcasting-recorder.exe .\apps\cli
go build -o apps\cli\recorder-worker.exe .\apps\gui\cmd\recorder-worker

Write-Host "Built apps\cli\twitcasting-recorder.exe and apps\cli\recorder-worker.exe"
