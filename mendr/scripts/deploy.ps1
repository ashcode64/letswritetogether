# Bring up sharedpage + dual Mendr edges, then register both tenants.
$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

function Import-DotEnv([string]$Path) {
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
        Set-Item -Path "Env:$name" -Value $value
    }
}

$EnvFile = $null
foreach ($cand in @(".env.mendr", ".env")) {
    $p = Join-Path $Root $cand
    if (Test-Path $p) { $EnvFile = $cand; break }
}
if (-not $EnvFile) {
    Write-Error "No .env.mendr or .env found. Copy .env.mendr.example first."
}

Write-Host "==> using env file: $EnvFile"
Import-DotEnv (Join-Path $Root $EnvFile)
$env:MENDR_ENV_LOADED = "1"

$composeArgs = @("compose", "--env-file", $EnvFile)
if ($env:DEPLOY_PROD -eq "1" -and (Test-Path (Join-Path $Root "docker-compose.prod.yml"))) {
    $composeArgs += @("-f", "docker-compose.yml", "-f", "docker-compose.prod.yml")
}

Write-Host "==> docker compose up"
& docker @composeArgs up --build -d
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

function Wait-Healthy([string]$Service) {
    $tries = 0
    while ($tries -lt 60) {
        $ps = & docker @composeArgs ps $Service 2>$null | Out-String
        if ($ps -match "healthy") { return }
        Start-Sleep -Seconds 2
        $tries++
    }
    Write-Warning "$Service not healthy yet; continuing"
}

Write-Host "==> waiting for Mendr edges"
Wait-Healthy "mendr-gateway-public"
Wait-Healthy "mendr-gateway-private"

Write-Host "==> registering public + private tenants"
$env:MENDR_SIDE = "public"
& (Join-Path $PSScriptRoot "register.ps1")
$env:MENDR_SIDE = "private"
& (Join-Path $PSScriptRoot "register.ps1")

Write-Host "==> deploy complete"
& docker @composeArgs ps
