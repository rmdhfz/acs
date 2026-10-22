#!/usr/bin/env bash
# scripts/bench.sh — benchmark ACS kita vs GenieACS dengan beban simulator CPE yang IDENTIK (cmd/acsbench).
# Dua stack terpisah dinaikkan sendiri (tidak menyentuh stack dev/UAT):
#   - ACS kita : proyek compose `acs-bench`  (docker-compose.yml + e2e + bench; batas laju dinaikkan)
#   - GenieACS : proyek compose `genieacs-bench` (deploy/genieacs/docker-compose.yml, konfigurasi bawaan)
#
#   scripts/bench.sh                       # 300 perangkat, concurrency 32
#   scripts/bench.sh --devices 1000 --concurrency 64 --rounds 3 --tasks 300
#   scripts/bench.sh --provision           # pekerjaan provisioning setara di kedua sistem (lihat di bawah)
#   scripts/bench.sh --down                # matikan kedua stack setelah selesai
#
# --provision: ACS kita diberi aturan otomasi (paksa PeriodicInformInterval=600 di tiap Inform + 3 pengecoh); GenieACS diberi provision
# JavaScript + preset yang setara (+ 3 preset pengecoh). Hasil: bin/bench-*-prov.json; keberhasilan diverifikasi di sisi CPE.
#
# Hasil: bin/bench-ours.json, bin/bench-genieacs.json (+ tabel di layar). Lihat docs/pengujian/BENCHMARK_GENIEACS.md.
set -euo pipefail
cd "$(dirname "$0")/.."
export MSYS_NO_PATHCONV=1

DEVICES=300; CONC=32; ROUNDS=3; TASKS=150; DOWN=0; PROV=0
while [ $# -gt 0 ]; do
  case "$1" in
    --devices) DEVICES="$2"; shift 2 ;;
    --concurrency) CONC="$2"; shift 2 ;;
    --rounds) ROUNDS="$2"; shift 2 ;;
    --tasks) TASKS="$2"; shift 2 ;;
    --down) DOWN=1; shift ;;
    --provision) PROV=1; shift ;;
    *) echo "opsi tak dikenal: $1" >&2; exit 2 ;;
  esac
done

if ! docker image inspect minio/minio:latest >/dev/null 2>&1 && docker image inspect cgr.dev/chainguard/minio:latest >/dev/null 2>&1; then
  export ACS_E2E_MINIO_IMAGE=cgr.dev/chainguard/minio:latest
fi
OURS=(docker compose -p acs-bench -f docker-compose.yml -f docker-compose.e2e.yml -f docker-compose.bench.yml)
GENIE=(docker compose -p genieacs-bench -f deploy/genieacs/docker-compose.yml)

echo "==> lingkungan"
docker info --format 'Docker {{.ServerVersion}} | CPU {{.NCPU}} | RAM {{.MemTotal}} B | {{.OperatingSystem}}'

echo "==> stack ACS kita (acs-bench) — database kosong"
"${OURS[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${OURS[@]}" up -d --build --pull never mariadb minio redis acsd
for _ in $(seq 1 60); do curl -fsS http://127.0.0.1:28080/readyz >/dev/null 2>&1 && break; sleep 2; done
"${OURS[@]}" exec -T acsd /app/seed-admin -username e2e-super -password 'E2eSuperPass!2345' -email e2e-super@acs.local 2>&1 | tail -1 || true

echo "==> stack GenieACS (genieacs-bench) — database kosong"
"${GENIE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${GENIE[@]}" up -d --build
for _ in $(seq 1 60); do curl -fsS http://127.0.0.1:37557/devices >/dev/null 2>&1 && break; sleep 2; done

mkdir -p bin
COMMON=(-devices "$DEVICES" -concurrency "$CONC" -rounds "$ROUNDS" -tasks "$TASKS")
SUFFIX=""
if [ "$PROV" = 1 ]; then COMMON+=(-provision); SUFFIX="-prov"; fi

echo "==> benchmark: ACS kita"
go run ./cmd/acsbench -kind ours -label "ACS kita" "${COMMON[@]}" \
  -cwmp http://127.0.0.1:27547/cwmp -api http://127.0.0.1:28080/api/v1 \
  -containers acs-bench-acsd-1,acs-bench-mariadb-1,acs-bench-redis-1 -out "bin/bench-ours${SUFFIX}.json"

echo "==> benchmark: GenieACS"
go run ./cmd/acsbench -kind genieacs -label "GenieACS 1.2.16" "${COMMON[@]}" \
  -cwmp http://127.0.0.1:37547 -api http://127.0.0.1:37557 \
  -containers genieacs-bench-genieacs-cwmp-1,genieacs-bench-genieacs-nbi-1,genieacs-bench-mongo-1 -out "bin/bench-genieacs${SUFFIX}.json"

if [ "$DOWN" = 1 ]; then
  "${OURS[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  "${GENIE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  echo "==> kedua stack dimatikan dan datanya dihapus"
fi
