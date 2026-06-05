param(
  [string]$AppDir = "D:\twitcasting-recorder",
  [string]$Config = "config.yaml",
  [string]$Addr = "127.0.0.1:8787"
)

$ErrorActionPreference = "Stop"
Set-Location $AppDir
.\twitcasting-recorder.exe web --config $Config --addr $Addr
