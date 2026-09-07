# scripts/e2e.ps1 — jalankan suite uji end-to-end ACS (lihat PENGUJIAN_E2E.md)
#
#   pwsh scripts/e2e.ps1                    # semua skenario
#   pwsh scripts/e2e.ps1 -Run S8            # hanya skenario yang memuat "S8"
#   pwsh scripts/e2e.ps1 -SuperUser x -SuperPass y
#
# Prasyarat: Docker Desktop nyala. Host tidak perlu Go toolchain.

param(
  [string]$Run = "",
  [string]$SuperUser = "e2e-super",
  [string]$SuperPass = "E2eSuperPass!2345",
  [string]$Rest = "http://acsd:8080",
  [string]$Cwmp = "http://acsd:7547/cwmp"
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path "$PSScriptRoot/..").Path
Set-Location $root
$env:MSYS_NO_PATHCONV = "1"
$srcMount = ($root -replace '^([A-Za-z]):', { "/$($_.Groups[1].Value.ToLower())" }) -replace '\\', '/'

Write-Host "==> stack docker-compose" -ForegroundColor Cyan
docker compose up -d | Out-Null

Write-Host "==> menunggu acsd sehat" -ForegroundColor Cyan
$ok = $false
for ($i = 0; $i -lt 40; $i++) {
  try {
    $code = (Invoke-WebRequest -UseBasicParsing "http://localhost:18080/api/v1/metrics" -TimeoutSec 3).StatusCode
    if ($code -eq 200) { $ok = $true; break }
  } catch {}
  Start-Sleep 2
}
if (-not $ok) { throw "acsd tidak sehat setelah 80 detik" }

Write-Host "==> seed superadmin (abaikan kalau sudah ada)" -ForegroundColor Cyan
try {
  docker compose exec -T acsd /app/seed-admin -username $SuperUser -password $SuperPass -email "$SuperUser@acs.local" 2>&1 | Out-Host
} catch { Write-Host "   (superadmin kemungkinan sudah ada)" -ForegroundColor DarkGray }

Write-Host "==> build binary e2e + cpesim (via golang:1.26)" -ForegroundColor Cyan
docker run --rm -v "${srcMount}:/src" -w /src -e CGO_ENABLED=0 golang:1.26 `
  sh -c 'go build -o /src/bin/e2e ./cmd/e2e && go build -o /src/bin/cpesim ./cmd/cpesim && echo built'

Write-Host "==> jalankan e2e" -ForegroundColor Cyan
$net = (docker compose ps --format json | ConvertFrom-Json | Select-Object -First 1).Networks
if (-not $net) { $net = "acs_default" }

$args = @(
  "run", "--rm", "--network", $net, "--name", "acs-e2e-runner",
  "-v", "${srcMount}/bin:/e2ebin:ro", "alpine:3",
  "/e2ebin/e2e", "-rest", $Rest, "-cwmp", $Cwmp,
  "-super-user", $SuperUser, "-super-pass", $SuperPass,
  "-json", "/tmp/e2e.json"
)
if ($Run) { $args += @("-run", $Run) }

docker @args
$exit = $LASTEXITCODE
Write-Host ""
if ($exit -eq 0) { Write-Host "E2E: SEMUA LULUS" -ForegroundColor Green }
else { Write-Host "E2E: ADA KEGAGALAN (exit $exit)" -ForegroundColor Red }
exit $exit
