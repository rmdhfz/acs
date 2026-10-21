# scripts/e2e.ps1 — pembungkus tipis atas scripts/e2e.sh (suite e2e di stack TERPISAH `acs-e2e`; lihat docker-compose.e2e.yml).
#
#   pwsh scripts/e2e.ps1                  # semua skenario
#   pwsh scripts/e2e.ps1 --run S1         # hanya skenario yang memuat "S1"
#   pwsh scripts/e2e.ps1 --fresh          # database e2e kosong
#   pwsh scripts/e2e.ps1 --down           # matikan & hapus data stack e2e setelah selesai
#
# Prasyarat: Docker Desktop nyala dan bash (Git for Windows). Stack dev/UAT tidak disentuh.
$ErrorActionPreference = "Stop"
$bash = (Get-Command bash -ErrorAction SilentlyContinue).Source
if (-not $bash) { throw "bash tidak ditemukan (pasang Git for Windows atau jalankan scripts/e2e.sh dari WSL)." }
& $bash "$PSScriptRoot/e2e.sh" @args
exit $LASTEXITCODE
