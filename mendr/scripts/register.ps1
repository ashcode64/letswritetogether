# Idempotent Mendr CP registration for colorbook dual-host ingress.
# Loads .env.mendr / .env from the repo root when present.
$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")

function Import-DotEnv([string]$Path) {
    if (-not (Test-Path $Path)) { return }
    Write-Host "==> loading $Path"
    Get-Content $Path | ForEach-Object {
        $line = $_.Trim()
        if (-not $line -or $line.StartsWith("#")) { return }
        $eq = $line.IndexOf("=")
        if ($eq -lt 1) { return }
        $name = $line.Substring(0, $eq).Trim()
        $value = $line.Substring($eq + 1).Trim()
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        # Do not clobber vars already set in the shell / CI
        $existing = [Environment]::GetEnvironmentVariable($name, "Process")
        if ([string]::IsNullOrEmpty($existing)) {
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

if (-not $env:MENDR_ENV_LOADED) {
    Import-DotEnv (Join-Path $Root ".env.mendr")
    Import-DotEnv (Join-Path $Root ".env")
    $env:MENDR_ENV_LOADED = "1"
}

if ($env:MENDR_AUTO_REGISTER -eq "false" -or $env:MENDR_AUTO_REGISTER -eq "0") {
    Write-Host "MENDR_AUTO_REGISTER disabled — skipping registration"
    exit 0
}

$OpenApiDir = if ($env:MENDR_OPENAPI_DIR) { $env:MENDR_OPENAPI_DIR } else { Join-Path $Root "mendr\openapi" }

function Resolve-CpUrl {
    $url = $env:MENDR_REGISTER_CONTROL_PLANE_URL
    if (-not $url) { $url = $env:MENDR_CP_URL }
    if (-not $url) { $url = $env:MENDR_CONTROL_PLANE_URL }
    if (-not $url) { $url = "http://127.0.0.1:8095" }
    $url = $url.TrimEnd("/")
    # Host-side register: rewrite Docker DNS to loopback
    if (-not (Test-Path "/.dockerenv") -and $url -match "host\.docker\.internal") {
        $url = $url -replace "host\.docker\.internal", "127.0.0.1"
    }
    return $url
}

$Cp = Resolve-CpUrl
$Tenant = if ($env:MENDR_TENANT_ID) { $env:MENDR_TENANT_ID } else { "00000000-0000-0000-0000-000000000001" }
$PublicHost = if ($env:PUBLIC_HOST) { $env:PUBLIC_HOST } else { "localhost" }
$PublicHostProd = if ($env:PUBLIC_HOST_PROD) { $env:PUBLIC_HOST_PROD } else { "sharedpage.app" }
$WaitSec = if ($env:MENDR_REGISTER_WAIT_SEC) { [int]$env:MENDR_REGISTER_WAIT_SEC } else { 120 }

function Get-CpHeaders {
    $h = @{ "X-Tenant-Id" = $Tenant }
    if ($env:GATEWAY_INTERNAL_API_KEY) {
        $h["Authorization"] = "Bearer $($env:GATEWAY_INTERNAL_API_KEY)"
        $h["X-Api-Key"] = $env:GATEWAY_INTERNAL_API_KEY
    }
    return $h
}

function Invoke-CpJson($Method, $Path, $BodyObj) {
    $headers = Get-CpHeaders
    $headers["Content-Type"] = "application/json"
    $json = $BodyObj | ConvertTo-Json -Compress
    try {
        Invoke-RestMethod -Method $Method -Uri "$Cp$Path" -Headers $headers -Body $json -TimeoutSec 60
    } catch {
        Write-Warning "$Method $Path : $($_.Exception.Message)"
    }
}

function Invoke-CpYaml($Path, $File) {
    $headers = Get-CpHeaders
    $headers["Content-Type"] = "application/x-yaml"
    $bytes = [System.IO.File]::ReadAllBytes($File)
    Invoke-RestMethod -Method Post -Uri "$Cp$Path" -Headers $headers -Body $bytes -TimeoutSec 60
}

function Test-CpReachable {
    foreach ($path in @("/health", "/actuator/health", "/api/services")) {
        try {
            $headers = Get-CpHeaders
            Invoke-WebRequest -Uri "$Cp$path" -Headers $headers -Method GET -TimeoutSec 5 -UseBasicParsing | Out-Null
            return $true
        } catch { }
    }
    return $false
}

Write-Host "==> CP: $Cp"
Write-Host "==> OpenAPI: $OpenApiDir"
Write-Host "==> waiting for CP (up to ${WaitSec}s)"
$elapsed = 0
while ($elapsed -lt $WaitSec) {
    if (Test-CpReachable) {
        Write-Host "==> CP reachable"
        break
    }
    Start-Sleep -Seconds 2
    $elapsed += 2
}
if ($elapsed -ge $WaitSec -and -not (Test-CpReachable)) {
    throw "Control plane not reachable at $Cp"
}

$services = @(
    @{ name = "colorbook-public"; url = $null },
    @{ name = "colorbook-public-prod"; url = $null },
    @{ name = "colorbook-gateway"; url = "http://gateway:8080" },
    @{ name = "colorbook-submission"; url = "http://submission:8080" },
    @{ name = "colorbook-voting"; url = "http://voting:8080" },
    @{ name = "colorbook-canvas"; url = "http://canvas:8080" },
    @{ name = "colorbook-archive"; url = "http://archive:8080" },
    @{ name = "colorbook-realtime"; url = "http://realtime:8080" }
)

foreach ($s in $services) {
    Write-Host "register $($s.name)"
    if ($s.url) {
        Invoke-CpJson Post "/api/services" @{ name = $s.name; baseUrl = $s.url; healthEndpoint = "/healthz" } | Out-Null
        Write-Host "instance $($s.name) -> $($s.url)"
        Invoke-CpJson Post "/api/services/$($s.name)/instances" @{ baseUrl = $s.url; weight = 100 } | Out-Null
    } else {
        Invoke-CpJson Post "/api/services" @{ name = $s.name; description = "synthetic ingress source" } | Out-Null
    }
}

$specs = @(
    "gateway-localhost.yaml",
    "gateway-sharedpage.yaml",
    "submission.yaml",
    "voting.yaml",
    "canvas.yaml",
    "archive.yaml",
    "realtime.yaml"
)

foreach ($name in $specs) {
    $file = Join-Path $OpenApiDir $name
    if (-not (Test-Path $file)) { throw "Missing OpenAPI file $file" }
    Write-Host "dry-run $name"
    $dry = Invoke-CpYaml "/api/services/import-openapi/dry-run" $file
    $dryText = ($dry | ConvertTo-Json -Compress)
    Write-Host ($dryText.Substring(0, [Math]::Min(400, $dryText.Length)))
    $rawSpec = Get-Content -Raw $file
    if ($name -match "submission|voting|canvas|archive|realtime") {
        if ($rawSpec -notmatch "x-mendr-host:\s*colorbook-internal") {
            throw "Private import $name missing x-mendr-host: colorbook-internal"
        }
    }
    Write-Host "import $name"
    Invoke-CpYaml "/api/services/import-openapi" $file | Out-Null
}

@(
    @{ host = $PublicHost; source = "colorbook-public" },
    @{ host = $PublicHostProd; source = "colorbook-public-prod" },
    @{ host = "colorbook-internal"; source = "colorbook-gateway" }
) | ForEach-Object {
    Write-Host "hostident $($_.host) -> $($_.source)"
    Invoke-CpJson Post "/api/services/ingress-host-identity" @{ host = $_.host; sourceService = $_.source } | Out-Null
}

Write-Host ""
Write-Host "==> Registration complete. After edge sync:"
Write-Host "curl -sS -H 'Host: $PublicHost' http://127.0.0.1/ | head"
Write-Host "Done."
