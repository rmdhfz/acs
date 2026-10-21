#!/usr/bin/env bash
# scripts/e2e.sh — jalankan suite e2e ACS di stack TERPISAH (proyek compose `acs-e2e`, lihat docker-compose.e2e.yml).
# Stack dev/UAT (proyek `acs`) tidak disentuh: tidak ada data uji yang masuk ke sana.
#
#   scripts/e2e.sh                 # semua skenario
#   scripts/e2e.sh --run S15       # hanya skenario yang namanya memuat "S15" (S15 butuh S1: pakai --run S1 untuk keduanya)
#   scripts/e2e.sh --fresh         # database kosong (down -v dulu)
#   scripts/e2e.sh --down          # matikan & hapus data stack e2e setelah selesai
#
# Prasyarat: Docker, berkas .env di akar repo. Host tidak perlu Go toolchain.
set -euo pipefail
cd "$(dirname "$0")/.."
export MSYS_NO_PATHCONV=1   # Git Bash: jangan ubah path /app/... menjadi path Windows

RUN=""; FRESH=0; DOWN=0
SUPER_USER="${E2E_SUPER_USER:-e2e-super}"; SUPER_PASS="${E2E_SUPER_PASS:-E2eSuperPass!2345}"
while [ $# -gt 0 ]; do
  case "$1" in
    --run) RUN="$2"; shift 2 ;;
    --fresh) FRESH=1; shift ;;
    --down) DOWN=1; shift ;;
    *) echo "opsi tak dikenal: $1" >&2; exit 2 ;;
  esac
done

# Image MinIO: pakai minio/minio bila ada; bila tidak, image lokal alternatif (chainguard) yang dipakai stack dev.
if ! docker image inspect minio/minio:latest >/dev/null 2>&1 && docker image inspect cgr.dev/chainguard/minio:latest >/dev/null 2>&1; then
  export ACS_E2E_MINIO_IMAGE=cgr.dev/chainguard/minio:latest
fi

COMPOSE=(docker compose -p acs-e2e -f docker-compose.yml -f docker-compose.e2e.yml)
if [ "$FRESH" = 1 ]; then "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; fi

echo "==> stack e2e terpisah (proyek acs-e2e; REST 127.0.0.1:28080, CWMP 127.0.0.1:27547)"
"${COMPOSE[@]}" up -d --build --pull never mariadb minio redis acsd

echo "==> menunggu acsd sehat"
ok=0
for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:28080/readyz >/dev/null 2>&1; then ok=1; break; fi
  sleep 2
done
[ "$ok" = 1 ] || { echo "acsd tidak sehat setelah 120 detik" >&2; "${COMPOSE[@]}" logs --tail 40 acsd >&2; exit 1; }

echo "==> seed superadmin (abaikan kalau sudah ada)"
"${COMPOSE[@]}" exec -T acsd /app/seed-admin -username "$SUPER_USER" -password "$SUPER_PASS" -email "$SUPER_USER@acs.local" 2>&1 | tail -1 || true

echo "==> build binary e2e (golang:1.26.9)"
SRC="$(pwd)"
docker run --rm -v "$SRC:/src" -w /src -e CGO_ENABLED=0 golang:1.26.9 sh -c 'go build -o /src/bin/e2e ./cmd/e2e && go build -o /src/bin/cpesim ./cmd/cpesim && echo built'

echo "==> jalankan e2e"
set +e
ARGS=(-rest http://acsd:8080 -cwmp http://acsd:7547/cwmp -super-user "$SUPER_USER" -super-pass "$SUPER_PASS" -json /tmp/e2e.json)
[ -n "$RUN" ] && ARGS+=(-run "$RUN")
docker run --rm --network acs-e2e_default --name acs-e2e-runner -v "$SRC/bin:/e2ebin:ro" alpine:3 /e2ebin/e2e "${ARGS[@]}"
CODE=$?
set -e

if [ "$DOWN" = 1 ]; then "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; echo "==> stack e2e dimatikan dan datanya dihapus"; fi
[ "$CODE" = 0 ] && echo "E2E: SEMUA LULUS" || echo "E2E: ADA KEGAGALAN (exit $CODE)"
exit $CODE
