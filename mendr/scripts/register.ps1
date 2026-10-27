# Idempotent Mendr CP registration for sharedpage dual-edge ingress.
# Set MENDR_SIDE=public|private (default: both when unset).
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

$Side = $env:MENDR_SIDE
if ([string]::IsNullOrEmpty($Side)) {
    Write-Host "==> MENDR_SIDE unset — registering public then private"
    $env:MENDR_SIDE = "public"
    & $PSCommandPath
    $env:MENDR_SIDE = "private"
    & $PSCommandPath
    exit 0
}
if ($Side -ne "public" -and $Side -ne "private") {
    throw "MENDR_SIDE must be public or private (got: $Side)"
}

$OpenApiDir = if ($env:MENDR_OPENAPI_DIR) { $env:MENDR_OPENAPI_DIR } else { Join-Path $Root "mendr\openapi" }

function Resolve-CpUrl {
    $url = $env:MENDR_REGISTER_CONTROL_PLANE_URL
    if (-not $url) { $url = $env:MENDR_CP_URL }
    if (-not $url) { $url = $env:MENDR_CONTROL_PLANE_URL }
    if (-not $url) { $url = "http://127.0.0.1:8095" }
    $url = $url.TrimEnd("/")
    if (-not (Test-Path "/.dockerenv") -and $url -match "host\.docker\.internal") {
        $url = $url -replace "host\.docker\.internal", "127.0.0.1"
    }
    return $url
}

$Cp = Resolve-CpUrl
$PublicHost = if ($env:PUBLIC_HOST) { $env:PUBLIC_HOST } else { "localhost" }
$PublicHostProd = if ($env:PUBLIC_HOST_PROD) { $env:PUBLIC_HOST_PROD } else { "sharedpage.app" }
$PrivateHost = if ($env:PRIVATE_HOST) { $env:PRIVATE_HOST } else { "sharedpage-internal" }
$WaitSec = if ($env:MENDR_REGISTER_WAIT_SEC) { [int]$env:MENDR_REGISTER_WAIT_SEC } else { 120 }

if ($Side -eq "public") {
    if ($env:MENDR_TENANT_ID) { $Tenant = $env:MENDR_TENANT_ID }
    elseif ($env:MENDR_TENANT_ID_PUBLIC) { $Tenant = $env:MENDR_TENANT_ID_PUBLIC }
    else { $Tenant = "00000000-0000-0000-0000-000000000001" }
    if ($env:GATEWAY_EDGE_API_KEY_PUBLIC) { $ApiKey = $env:GATEWAY_EDGE_API_KEY_PUBLIC }
    else { $ApiKey = $env:GATEWAY_INTERNAL_API_KEY }
} else {
    if ($env:MENDR_TENANT_ID) { $Tenant = $env:MENDR_TENANT_ID }
    elseif ($env:MENDR_TENANT_ID_PRIVATE) { $Tenant = $env:MENDR_TENANT_ID_PRIVATE }
    else { $Tenant = "00000000-0000-0000-0000-000000000002" }
    if ($env:GATEWAY_EDGE_API_KEY_PRIVATE) { $ApiKey = $env:GATEWAY_EDGE_API_KEY_PRIVATE }
    else { $ApiKey = $env:GATEWAY_INTERNAL_API_KEY }
}

function Get-CpHeaders {
    $h = @{ "X-Tenant-Id" = $Tenant }
    if ($ApiKey) {
        $h["Authorization"] = "Bearer $ApiKey"
        $h["X-Api-Key"] = $ApiKey
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

Write-Host "==> side: $Side"
Write-Host "==> tenant: $Tenant"
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

function Register-Service($Name, $Url) {
    Write-Host "register $Name"
    if ($Url) {
        Invoke-CpJson Post "/api/services" @{ name = $Name; baseUrl = $Url; healthEndpoint = "/healthz" } | Out-Null
        Write-Host "instance $Name -> $Url"
        Invoke-CpJson Post "/api/services/$Name/instances" @{ baseUrl = $Url; weight = 100 } | Out-Null
    } else {
        Invoke-CpJson Post "/api/services" @{ name = $Name; description = "synthetic ingress source" } | Out-Null
    }
}

function Import-OpenApi($Name) {
    $file = Join-Path $OpenApiDir $Name
    if (-not (Test-Path $file)) { throw "Missing OpenAPI file $file" }
    Write-Host "dry-run $Name"
    $dry = Invoke-CpYaml "/api/services/import-openapi/dry-run" $file
    $dryText = ($dry | ConvertTo-Json -Compress)
    Write-Host ($dryText.Substring(0, [Math]::Min(400, $dryText.Length)))
    $rawSpec = Get-Content -Raw $file
    if ($Name -match "submission|voting|canvas|archive|realtime") {
        if ($rawSpec -notmatch "x-mendr-host:\s*sharedpage-internal" -and $rawSpec -notmatch [regex]::Escape("x-mendr-host: $PrivateHost")) {
            throw "Private import $Name missing x-mendr-host: $PrivateHost"
        }
    }
    Write-Host "import $Name"
    Invoke-CpYaml "/api/services/import-openapi" $file | Out-Null
}

if ($Side -eq "public") {
    Register-Service "sharedpage-public" $null
    Register-Service "sharedpage-public-prod" $null
    Register-Service "sharedpage-gateway" "http://gateway:8080"
    foreach ($name in @("gateway-localhost.yaml", "gateway-sharedpage.yaml")) {
        Import-OpenApi $name
    }
    @(
        @{ host = $PublicHost; source = "sharedpage-public" },
        @{ host = $PublicHostProd; source = "sharedpage-public-prod" }
    ) | ForEach-Object {
        Write-Host "hostident $($_.host) -> $($_.source)"
        Invoke-CpJson Post "/api/services/ingress-host-identity" @{ host = $_.host; sourceService = $_.source } | Out-Null
    }
} else {
    Register-Service "sharedpage-gateway" "http://gateway:8080"
    Register-Service "sharedpage-submission" "http://submission:8080"
    Register-Service "sharedpage-voting" "http://voting:8080"
    Register-Service "sharedpage-canvas" "http://canvas:8080"
    Register-Service "sharedpage-archive" "http://archive:8080"
    Register-Service "sharedpage-realtime" "http://realtime:8080"
    foreach ($name in @("submission.yaml", "voting.yaml", "canvas.yaml", "archive.yaml", "realtime.yaml")) {
        Import-OpenApi $name
    }
    Write-Host "hostident $PrivateHost -> sharedpage-gateway"
    Invoke-CpJson Post "/api/services/ingress-host-identity" @{ host = $PrivateHost; sourceService = "sharedpage-gateway" } | Out-Null
}

Write-Host ""
Write-Host "==> Registration complete ($Side)."
Write-Host "Done."
