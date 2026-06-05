param(
  [Parameter(Mandatory = $true)]
  [string]$ScreenId,

  [int]$Seconds = 90,
  [int]$Port = 9222,
  [string]$BrowserPath = "",
  [string]$UserDataDir = "",
  [string]$OutFile = "",
  [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"

function Find-Browser {
  param([string]$ExplicitPath)
  if ($ExplicitPath -and (Test-Path -LiteralPath $ExplicitPath)) {
    return $ExplicitPath
  }

  $candidates = @(
    "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
    "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe",
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
  )
  foreach ($candidate in $candidates) {
    if ($candidate -and (Test-Path -LiteralPath $candidate)) {
      return $candidate
    }
  }
  throw "Chrome or Edge was not found. Pass -BrowserPath explicitly."
}

function Get-HeaderNames {
  param($Headers)
  if ($null -eq $Headers) {
    return @()
  }
  return @($Headers.PSObject.Properties.Name | Sort-Object)
}

function Has-Header {
  param($Headers, [string]$Name)
  if ($null -eq $Headers) {
    return $false
  }
  foreach ($prop in $Headers.PSObject.Properties) {
    if ($prop.Name -ieq $Name) {
      return $true
    }
  }
  return $false
}

function Test-HlsUrl {
  param([string]$Url)
  if ([string]::IsNullOrWhiteSpace($Url)) {
    return $false
  }
  $lower = $Url.ToLowerInvariant()
  return ($lower -match "\.m3u8(\?|$)") -or $lower.Contains("/livehls/") -or $lower.Contains("tc.livehls")
}

function Wait-DevTools {
  param([int]$Port, [int]$TimeoutSeconds)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    try {
      return Invoke-RestMethod -Uri "http://127.0.0.1:$Port/json/version" -TimeoutSec 2
    } catch {
      Start-Sleep -Milliseconds 300
    }
  } while ((Get-Date) -lt $deadline)
  throw "DevTools endpoint did not become ready on port $Port."
}

function Send-Cdp {
  param($Socket, [string]$Method, $Params)
  $script:NextCdpId++
  $payload = [ordered]@{
    id = $script:NextCdpId
    method = $Method
  }
  if ($null -ne $Params) {
    $payload.params = $Params
  }
  $json = $payload | ConvertTo-Json -Depth 20 -Compress
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
  $segment = New-Object System.ArraySegment[byte] -ArgumentList @(,$bytes)
  $null = $Socket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
}

function Receive-Cdp {
  param($Socket, [int]$TimeoutMilliseconds)
  $buffer = New-Object byte[] 65536
  $stream = New-Object System.IO.MemoryStream
  do {
    $cts = New-Object System.Threading.CancellationTokenSource
    $cts.CancelAfter($TimeoutMilliseconds)
    try {
      $segment = New-Object System.ArraySegment[byte] -ArgumentList @(,$buffer)
      $result = $Socket.ReceiveAsync($segment, $cts.Token).GetAwaiter().GetResult()
    } catch [System.OperationCanceledException] {
      return $null
    } catch [System.Net.WebSockets.WebSocketException] {
      return $null
    } finally {
      $cts.Dispose()
    }

    if ($result.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) {
      return $null
    }
    $stream.Write($buffer, 0, $result.Count)
  } while (-not $result.EndOfMessage)

  $text = [System.Text.Encoding]::UTF8.GetString($stream.ToArray())
  if ([string]::IsNullOrWhiteSpace($text)) {
    return $null
  }
  return $text | ConvertFrom-Json
}

function New-CaptureRecord {
  param([string]$RequestId)
  return [ordered]@{
    request_id = $RequestId
    url = ""
    method = ""
    resource_type = ""
    status = $null
    status_text = ""
    mime_type = ""
    request_header_names = @()
    response_header_names = @()
    request_has_cookie_header = $false
    associated_cookie_names = @()
    blocked_cookie_names = @()
    failed_error = ""
    first_seen = (Get-Date).ToString("o")
  }
}

$ScreenId = $ScreenId.Trim().TrimStart("@")
if (-not $ScreenId) {
  throw "ScreenId is empty."
}

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
if (-not $UserDataDir) {
  $UserDataDir = Join-Path $repoRoot "apps\gui\build\browser-profile"
}
if (-not $OutFile) {
  $outDir = Join-Path $repoRoot "apps\gui\build\cdp-capture"
  New-Item -ItemType Directory -Force -Path $outDir | Out-Null
  $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
  $safeId = $ScreenId -replace "[^\w.-]", "_"
  $OutFile = Join-Path $outDir "$safeId-$stamp.json"
}

$url = "https://twitcasting.tv/$ScreenId"
$browser = Find-Browser $BrowserPath

if (-not $NoLaunch) {
  New-Item -ItemType Directory -Force -Path $UserDataDir | Out-Null
  $args = @(
    "--remote-debugging-port=$Port",
    "--user-data-dir=$UserDataDir",
    "--no-first-run",
    "--new-window",
    $url
  )
  Start-Process -FilePath $browser -ArgumentList $args | Out-Null
}

Wait-DevTools -Port $Port -TimeoutSeconds 20 | Out-Null

$tabs = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/json" -TimeoutSec 5
$page = @($tabs | Where-Object { $_.type -eq "page" -and $_.url -like "*twitcasting.tv/$ScreenId*" } | Select-Object -First 1)[0]
if ($null -eq $page) {
  $page = @($tabs | Where-Object { $_.type -eq "page" } | Select-Object -First 1)[0]
}
if ($null -eq $page -or -not $page.webSocketDebuggerUrl) {
  throw "No debuggable browser page was found."
}

$socket = New-Object System.Net.WebSockets.ClientWebSocket
$null = $socket.ConnectAsync([Uri]$page.webSocketDebuggerUrl, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()

$script:NextCdpId = 0
$captures = @{}
$startedAt = Get-Date

try {
  Send-Cdp $socket "Network.enable" $null
  Send-Cdp $socket "Page.enable" $null
  Send-Cdp $socket "Page.navigate" ([ordered]@{ url = $url })

  Write-Host "Capturing HLS requests for $ScreenId during $Seconds seconds..."
  Write-Host "If the dedicated browser profile is not logged in, log in there first and run this script again."

  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    $msg = Receive-Cdp $socket 1000
    if ($null -eq $msg -or -not $msg.method) {
      continue
    }

    $method = [string]$msg.method
    $p = $msg.params
    if ($null -eq $p) {
      continue
    }

    if ($method -eq "Network.requestWillBeSent") {
      $requestUrl = [string]$p.request.url
      if (-not (Test-HlsUrl $requestUrl)) {
        continue
      }
      $rid = [string]$p.requestId
      if (-not $captures.ContainsKey($rid)) {
        $captures[$rid] = New-CaptureRecord $rid
      }
      $record = $captures[$rid]
      $record.url = $requestUrl
      $record.method = [string]$p.request.method
      $record.resource_type = [string]$p.type
      $record.request_header_names = Get-HeaderNames $p.request.headers
      $record.request_has_cookie_header = Has-Header $p.request.headers "Cookie"
      continue
    }

    if ($method -eq "Network.requestWillBeSentExtraInfo") {
      $rid = [string]$p.requestId
      if (-not $captures.ContainsKey($rid)) {
        continue
      }
      $record = $captures[$rid]
      $record.request_header_names = Get-HeaderNames $p.headers
      $record.request_has_cookie_header = Has-Header $p.headers "Cookie"
      $names = @()
      foreach ($item in @($p.associatedCookies)) {
        if ($item.cookie -and $item.cookie.name) {
          $names += [string]$item.cookie.name
        }
      }
      $record.associated_cookie_names = @($names | Sort-Object -Unique)
      continue
    }

    if ($method -eq "Network.responseReceived") {
      $rid = [string]$p.requestId
      if (-not $captures.ContainsKey($rid)) {
        continue
      }
      $record = $captures[$rid]
      $record.status = $p.response.status
      $record.status_text = [string]$p.response.statusText
      $record.mime_type = [string]$p.response.mimeType
      $record.response_header_names = Get-HeaderNames $p.response.headers
      continue
    }

    if ($method -eq "Network.responseReceivedExtraInfo") {
      $rid = [string]$p.requestId
      if (-not $captures.ContainsKey($rid)) {
        continue
      }
      $record = $captures[$rid]
      if ($null -ne $p.statusCode) {
        $record.status = $p.statusCode
      }
      if ($record.response_header_names.Count -eq 0) {
        $record.response_header_names = Get-HeaderNames $p.headers
      }
      $blocked = @()
      foreach ($item in @($p.blockedCookies)) {
        if ($item.cookie -and $item.cookie.name) {
          $blocked += [string]$item.cookie.name
        }
      }
      $record.blocked_cookie_names = @($blocked | Sort-Object -Unique)
      continue
    }

    if ($method -eq "Network.loadingFailed") {
      $rid = [string]$p.requestId
      if (-not $captures.ContainsKey($rid)) {
        continue
      }
      $captures[$rid].failed_error = [string]$p.errorText
      continue
    }
  }
} finally {
  try {
    if ($socket.State -eq [System.Net.WebSockets.WebSocketState]::Open) {
      $null = $socket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "done", [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
    }
  } catch {
    # Browser pages can abort the CDP socket during reload; the capture report is still useful.
  }
  $socket.Dispose()
}

$records = @($captures.Values | Sort-Object url, request_id)
$report = [ordered]@{
  screen_id = $ScreenId
  page_url = $url
  started_at = $startedAt.ToString("o")
  finished_at = (Get-Date).ToString("o")
  seconds = $Seconds
  browser = $browser
  user_data_dir = $UserDataDir
  note = "Cookie and Authorization values are intentionally not saved."
  requests = $records
}

$json = $report | ConvertTo-Json -Depth 20
$parent = Split-Path -Parent $OutFile
if ($parent) {
  New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
Set-Content -LiteralPath $OutFile -Value $json -Encoding UTF8

Write-Host "Captured $($records.Count) HLS-related requests."
foreach ($record in $records) {
  $status = if ($null -ne $record.status) { $record.status } else { "-" }
  $cookie = if ($record.request_has_cookie_header -or $record.associated_cookie_names.Count -gt 0) { "cookie" } else { "no-cookie" }
  Write-Host "[$status][$cookie] $($record.url)"
}
Write-Host "Report: $OutFile"
