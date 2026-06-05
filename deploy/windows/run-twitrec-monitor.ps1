param(
  [string]$AppDir = "D:\twitcasting-recorder",
  [string]$Config = "config.yaml"
)

$ErrorActionPreference = "Stop"
Set-Location $AppDir
.\twitcasting-recorder.exe monitor --config $Config
