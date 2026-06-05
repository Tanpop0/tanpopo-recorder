param(
  [string]$Config = "config.yaml"
)

$ErrorActionPreference = "Stop"
Set-Location "$PSScriptRoot\..\apps\cli"
go run . --config $Config
