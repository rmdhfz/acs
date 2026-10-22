#!/usr/bin/env bash
# scripts/bench-remote.sh — benchmark ACS kita vs GenieACS dari MESIN GENERATOR TERPISAH ke server yang diuji (SUT), beberapa run, median.
#
# Beda dari scripts/bench.sh (semua di satu laptop): di sini generator beban (cmd/acsbench) berjalan di mesin lain sehingga tidak
# berebut CPU/memori/port dengan sistem yang diuji. Panduan lengkap & prasyarat: docs/pengujian/BENCHMARK_SERVER_TERPISAH.md.
#
#   OURS_CWMP=http://sut:27547/cwmp OURS_API=http://sut:28080/api/v1 \
#   GENIE_CWMP=http://sut:37547     GENIE_API=http://sut:37557 \
#   RESET_OURS='ssh sut /srv/bench/reset-ours.sh' RESET_GENIE='ssh sut /srv/bench/reset-genieacs.sh' \
#   scripts/bench-remote.sh --runs 5 --devices 1500 --concurrency 64 --rounds 4 --tasks 500 --provision
#
# RESET_*: perintah yang membuat stack bersih (database kosong) sebelum tiap run; WAJIB agar run tidak saling mencemari (perangkat
# yang sudah terdaftar mengubah fase registrasi). Tanpa RESET_*, skrip menolak jalan kecuali --no-reset diberikan (hasil registrasi
# run kedua dst. TIDAK sebanding).
# Urutan sistem BERGANTIAN tiap run (ganjil: kita dulu; genap: GenieACS dulu) agar efek urutan/pemanasan mesin tidak menguntungkan satu sisi.
set -euo pipefail
cd "$(dirname "$0")/.."
export MSYS_NO_PATHCONV=1

RUNS=5; DEVICES=1500; CONC=64; ROUNDS=4; TASKS=500; PROV=0; NORESET=0
SUPER_USER="${SUPER_USER:-e2e-super}"; SUPER_PASS="${SUPER_PASS:-E2eSuperPass!2345}"
while [ $# -gt 0 ]; do
  case "$1" in
    --runs) RUNS="$2"; shift 2 ;;
    --devices) DEVICES="$2"; shift 2 ;;
    --concurrency) CONC="$2"; shift 2 ;;
    --rounds) ROUNDS="$2"; shift 2 ;;
    --tasks) TASKS="$2"; shift 2 ;;
    --provision) PROV=1; shift ;;
    --no-reset) NORESET=1; shift ;;
    *) echo "opsi tak dikenal: $1" >&2; exit 2 ;;
  esac
done

: "${OURS_CWMP:?isi OURS_CWMP (mis. http://sut:27547/cwmp)}"; : "${OURS_API:?isi OURS_API (mis. http://sut:28080/api/v1)}"
: "${GENIE_CWMP:?isi GENIE_CWMP (mis. http://sut:37547)}";     : "${GENIE_API:?isi GENIE_API (mis. http://sut:37557)}"
if [ "$NORESET" = 0 ] && { [ -z "${RESET_OURS:-}" ] || [ -z "${RESET_GENIE:-}" ]; }; then
  echo "RESET_OURS dan RESET_GENIE wajib (perintah yang mengosongkan database stack sebelum tiap run); atau --no-reset (tidak sebanding)." >&2
  exit 2
fi

OUT="bin/remote-$(date +%Y%m%d-%H%M%S)"; mkdir -p "$OUT"
echo "==> membangun acsbench"; go build -o "$OUT/acsbench" ./cmd/acsbench
PF=(); [ "$PROV" = 1 ] && PF=(-provision)
COMMON=(-devices "$DEVICES" -concurrency "$CONC" -rounds "$ROUNDS" -tasks "$TASKS" ${PF[@]+"${PF[@]}"})

{
  echo "# Lingkungan generator ($(date -Is))"
  uname -a
  command -v nproc >/dev/null && echo "CPU generator: $(nproc)"
  command -v free >/dev/null && free -h | sed -n 2p
  echo "RUNS=$RUNS DEVICES=$DEVICES CONC=$CONC ROUNDS=$ROUNDS TASKS=$TASKS PROVISION=$PROV"
  echo "OURS=$OURS_CWMP  GENIE=$GENIE_CWMP"
} | tee "$OUT/lingkungan.txt"

run_ours() {
  local n="$1"; [ "$NORESET" = 0 ] && eval "$RESET_OURS"
  "$OUT/acsbench" -kind ours -label "ACS kita" "${COMMON[@]}" -cwmp "$OURS_CWMP" -api "$OURS_API" \
    -super-user "$SUPER_USER" -super-pass "$SUPER_PASS" -out "$OUT/ours-$n.json" | tee "$OUT/ours-$n.log"
}
run_genie() {
  local n="$1"; [ "$NORESET" = 0 ] && eval "$RESET_GENIE"
  "$OUT/acsbench" -kind genieacs -label "GenieACS 1.2.16" "${COMMON[@]}" -cwmp "$GENIE_CWMP" -api "$GENIE_API" \
    -out "$OUT/genie-$n.json" | tee "$OUT/genie-$n.log"
}

for n in $(seq 1 "$RUNS"); do
  echo "==> run $n/$RUNS ($([ $((n % 2)) = 1 ] && echo 'kita dulu' || echo 'GenieACS dulu'))"
  if [ $((n % 2)) = 1 ]; then run_ours "$n"; run_genie "$n"; else run_genie "$n"; run_ours "$n"; fi
done

echo "==> ringkasan (median antar-run)"
node scripts/bench-median.mjs "$OUT"/ours-*.json --vs "$OUT"/genie-*.json | tee "$OUT/ringkasan.md"
echo "Hasil mentah & ringkasan: $OUT/"
