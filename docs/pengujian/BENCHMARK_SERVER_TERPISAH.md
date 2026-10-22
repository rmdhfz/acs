# Benchmark ACS vs GenieACS di Server Terpisah — Runbook

Tujuan: menggantikan benchmark satu-laptop (`scripts/bench.sh`) dengan pengukuran yang layak dipakai **di luar tim**. Tiga sumber kebisingan di
laptop dihilangkan: (1) generator beban dan sistem uji berebut CPU/memori/port, (2) Docker Desktop (VM, disk virtual) mendistorsi I/O, (3) satu run
tidak cukup — antar-run bergeser sampai ±30%. Hasil laptop tetap benar sebagai **perbandingan relatif**; dokumen ini untuk angka absolut yang bisa
dipertanggungjawabkan.

> Baca batas klaim di `BENCHMARK_GENIEACS.md` dulu. Benchmark ini tetap memakai **simulator CPE** (`internal/simcpe`) — ia mengukur kapasitas ACS,
> bukan kompatibilitas firmware. Uji ONT fisik: `PANDUAN_UJI_ONT_FITUR_BARU.md`.

## 1. Topologi

```
 [Mesin G: generator]  --- LAN >= 1 Gbps, satu segmen, RTT < 1 ms ---  [Mesin S: SUT (sistem yang diuji)]
   cmd/acsbench                                                          docker: ACS kita  (acsd+MariaDB+Redis)
   scripts/bench-remote.sh                                               docker: GenieACS  (cwmp/nbi/mongo)
```

- **S dan G harus mesin berbeda.** Jangan taruh G di VM yang sama dengan S.
- Kedua sistem berjalan di **S** pada waktu berbeda (bergantian, bukan bersamaan): stack yang tidak sedang diuji **dimatikan**, bukan dibiarkan idle
  (di laptop dibiarkan idle; di S dimatikan agar tidak ada gangguan sama sekali).
- Spesifikasi S: catat persis (CPU model/inti, RAM, jenis disk — **SSD/NVMe**, bukan HDD). Rekomendasi minimum: 8 inti, 16 GB, SSD.
- Spesifikasi G: setidaknya 4 inti; generator sendiri tidak boleh jenuh (periksa CPU G < 70% selama run; kalau lebih, tambah mesin G atau turunkan `--concurrency`).

## 2. Menyiapkan S

1. Salin repo (atau hanya `docker-compose*.yml`, `deploy/genieacs/`, `.env`) ke S, mis. `/srv/bench`.
2. **ACS kita**: gunakan overlay bench agar batas laju per-IP tidak memotong beban dari satu IP generator:
   `docker compose -p acs-bench -f docker-compose.yml -f docker-compose.e2e.yml -f docker-compose.bench.yml up -d --build mariadb minio redis acsd`
   (overlay menaikkan `ACS_CWMP_RATE_LIMIT`/`ACS_REST_RATE_LIMIT`; **jangan** dipakai pada stack yang menjaga kredensial sungguhan).
   Port CWMP 27547 dan API 28080 dibuka ke G (di compose terikat 127.0.0.1 — ubah ke `0.0.0.0` atau pakai tunnel/ufw yang membatasi ke IP G).
3. **GenieACS**: `deploy/genieacs/docker-compose.yml` (konfigurasi bawaan GenieACS 1.2.16 apa adanya; CWMP 37547, NBI 37557). Bila Anda men-tuning GenieACS
   (jumlah worker, indeks MongoDB), **catat** dan jalankan pula versi bawaan — yang dibandingkan di laporan harus jelas.
4. Samakan hal yang bisa disamakan: tidak ada beban lain di S; `vm.swappiness` rendah; **frekuensi CPU tetap** (`cpupower frequency-set -g performance`);
   `ulimit -n` ≥ 65536 untuk Docker daemon; catat versi kernel/Docker (`docker info`).
5. **Skrip reset** (dipanggil sebelum tiap run; database HARUS kosong — perangkat yang sudah terdaftar mengubah fase registrasi). Contoh `/srv/bench/reset-ours.sh`:

   ```bash
   #!/usr/bin/env bash
   set -euo pipefail
   cd /srv/bench
   docker compose -p genieacs-bench -f deploy/genieacs/docker-compose.yml down -v --remove-orphans || true   # matikan lawan
   O=(docker compose -p acs-bench -f docker-compose.yml -f docker-compose.e2e.yml -f docker-compose.bench.yml)
   "${O[@]}" down -v --remove-orphans || true
   "${O[@]}" up -d --build mariadb minio redis acsd
   for _ in $(seq 1 60); do curl -fsS http://127.0.0.1:28080/readyz >/dev/null 2>&1 && break; sleep 2; done
   "${O[@]}" exec -T acsd /app/seed-admin -username e2e-super -password 'E2eSuperPass!2345' -email e2e-super@acs.local
   sleep 10   # biarkan JIT/pool/cache stabil sebelum beban
   ```

   `/srv/bench/reset-genieacs.sh` kembarannya: matikan stack kita, `down -v` + `up -d` GenieACS, tunggu `http://127.0.0.1:37557/devices` menjawab, `sleep 10`.
6. Pemantauan sumber daya di S selama run (karena `acsbench` di G tidak bisa membaca `docker stats` S). Jalankan di S, catat ke berkas:

   ```bash
   while sleep 2; do docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' >> /srv/bench/stats-$(date +%F).csv; done
   ```

   Ringkas CPU rata-rata dan memori maks per kontainer per run; laporkan di samping tabel throughput. (Mode `--containers` di `acsbench` hanya untuk satu mesin.)

## 3. Menjalankan dari G

```bash
export OURS_CWMP=http://S:27547/cwmp OURS_API=http://S:28080/api/v1
export GENIE_CWMP=http://S:37547      GENIE_API=http://S:37557
export RESET_OURS='ssh S /srv/bench/reset-ours.sh' RESET_GENIE='ssh S /srv/bench/reset-genieacs.sh'
scripts/bench-remote.sh --runs 5 --devices 1500 --concurrency 64 --rounds 4 --tasks 500 --provision
```

- `--provision`: pekerjaan provisioning setara di kedua sistem (lihat `BENCHMARK_GENIEACS.md`); **hapus** untuk mengukur Inform polos.
- Urutan sistem **bergantian** tiap run (ganjil: kita dulu; genap: GenieACS dulu).
- Minimal **5 run**. Untuk klaim keras: 7–10 run dan beberapa ukuran (`--devices 1500`, `5000`, `10000`; naikkan `--concurrency` bertahap 32/64/128 dan cari titik jenuh,
  bukan satu titik).
- Keluaran di `bin/remote-<stempel>/`: `ours-N.json`, `genie-N.json`, log, `lingkungan.txt` (spesifikasi G), dan **`ringkasan.md`** (median, min–maks, "searah").

## 4. Cara membaca ringkasan (aturan yang harus dipatuhi)

`scripts/bench-median.mjs` menandai satu fase sebagai "menang" hanya bila **rentang semua run kedua sistem tidak tumpang-tindih**. Selainnya = "setara".
Jangan menulis "lebih cepat X%" dari median bila kolom *searah* bertuliskan "tumpang-tindih". Laporkan juga:

1. Spesifikasi S dan G, versi Docker/kernel, dan commit ACS kita (`git rev-parse --short HEAD`) + versi GenieACS.
2. Bila GenieACS di-tuning atau tidak, dan bagaimana.
3. CPU G selama run (bukti generator tidak jenuh).
4. Kegagalan (kolom galat harus 0; bila tidak, jelaskan dan jangan dibandingkan).
5. Verifikasi provisioning di sisi CPE (`provisioning diterapkan di CPE: N/N` di log) — bila bukan N/N, run itu tidak setara.

## 5. Yang sengaja BELUM diukur (tambahkan bila perlu klaim terkait)

- Skala ≥ 100 ribu perangkat dan retensi berhari-hari (pertumbuhan tabel, kinerja query lama). Rencana: isi database dengan perangkat sintetis lalu ukur
  Inform + daftar perangkat + query bebas (`param=`) pada ukuran itu.
- Beban REST/UI bersamaan dengan Inform (dashboard, daftar perangkat) dan kegagalan jaringan/restart komponen.
- Biaya aturan otomasi dengan banyak aturan per tenant (di benchmark ini 1 aturan aktif + 3 pengecoh).
- Firmware nyata. Simulator tidak menangkap kuirk seperti bug `xsi:type` yang ditemukan di UAT 21 Okt 2026.

## 6. Keselamatan

- Overlay bench menaikkan batas laju dan memakai kredensial uji (`e2e-super`/`E2eSuperPass!2345`): jalankan hanya di jaringan uji tertutup, **bukan** di server yang
  menyimpan data sungguhan, dan matikan setelah selesai.
- Jangan arahkan `acsbench` ke ACS produksi: ia mendaftarkan ribuan perangkat sintetis ke tenant baru dan mengantre ribuan task.
