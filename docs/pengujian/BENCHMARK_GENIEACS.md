# Benchmark ACS kita vs GenieACS 1.2.16 — 21 Oktober 2026

> **Batas klaim (baca dulu).** Beban berasal dari **simulator CPE** (`internal/simcpe`), bukan firmware nyata, dijalankan dari **satu laptop**
> yang juga menjalankan kedua ACS dan generator bebannya (8 CPU, 16 GB, Docker Desktop). Angka berlaku untuk konfigurasi bawaan yang
> dipakai dan **tidak membuktikan kompatibilitas ONT**. Hasil di bawah adalah satu kali run; pengukuran ulang akan bergeser beberapa persen.

## Cara mengulang

```bash
scripts/bench.sh --devices 1500 --concurrency 64 --rounds 4 --tasks 500   # ±3 menit; hasil di bin/bench-*.json
scripts/bench.sh --down                                                    # bersihkan kedua stack
```

Dua stack terpisah dinaikkan sendiri (tidak menyentuh stack dev/UAT): ACS kita (`acs-bench`, batas laju `/cwmp` dinaikkan lewat
`docker-compose.bench.yml`) dan GenieACS (`deploy/genieacs/`, konfigurasi bawaan, MongoDB 6). Alat: `cmd/acsbench`.

## Metode (sama untuk keduanya)

- 1.500 perangkat simulasi (profil ZTE TR-098), 64 sesi paralel. Tiap pekerja memakai **satu koneksi TCP persisten**; satu sesi CWMP
  selalu di satu koneksi, seperti CPE nyata (GenieACS menolak sesi yang tersebar ke koneksi berbeda).
- Pesan SOAP ditulis ulang ke gaya **berawalan namespace** (`<soap-env:…>`, `<cwmp:…>`) seperti CPE nyata. Tanpa ini GenieACS menolak
  Inform gaya-Go simulator dengan "Unrecognized CWMP version" (dibuktikan dari kodenya); ACS kita menerima kedua gaya.
- Fase: registrasi (`0 BOOTSTRAP`) → 4 putaran Inform periodik → fase tugas (antre `GetParameterValues` lewat API, lalu CPE menjawab
  dalam sesinya). Parameter fase tugas sengaja **bukan** yang ikut dilaporkan Inform (SSID): GenieACS menganggap nilai dari Inform
  sudah segar dan melewati RPC, sedangkan ACS kita selalu bertanya ke CPE.
- Latensi = durasi satu sesi penuh (Inform → RPC → penutup) per perangkat, bukan hanya Inform→InformResponse.

## Hasil (1.500 perangkat, concurrency 64) — 0 galat pada kedua sistem

| Fase | ACS kita: sesi/dtk · p50 · p95 | GenieACS: sesi/dtk · p50 · p95 |
|---|---|---|
| Registrasi | 222 · 275 ms · 380 ms | **349** · **164 ms** · **339 ms** |
| Periodik putaran 1 | 278 · 223 ms · 304 ms | **424** · **131 ms** · **300 ms** |
| Periodik putaran 2 | 470 · 127 ms · **222 ms** | 474 · 121 ms · 258 ms |
| Periodik putaran 3 | **558** · 112 ms · **152 ms** | 532 · 104 ms · 227 ms |
| Periodik putaran 4 | 529 · 112 ms · **184 ms** | **564** · 103 ms · 206 ms |
| Tugas (antre API + sesi + RPC), 500 | 118 · 482 ms · **939 ms** | **183** · **312 ms** · **594 ms** |

| Sumber daya (rata-rata / puncak) | ACS kita | GenieACS |
|---|---|---|
| Proses ACS (CPU · memori maks) | acsd 149% / 199% · **27 MiB** | genieacs-cwmp 407% / 473% · 572 MiB |
| Basis data | MariaDB 301% / 612% · 161 MiB | MongoDB 201% / 245% · 292 MiB |
| Lainnya | Redis 14% · 11 MiB | NBI 17% · 185 MiB |

(Persentase CPU: 100% = satu inti; sampel `docker stats` tiap 2 dtk, hanya 5–7 sampel per run.)

## Pembacaan jujur

- **Tidak ada pemenang mutlak pada throughput.** GenieACS lebih cepat saat registrasi dan fase tugas; setelah pemanasan (putaran 2–4)
  keduanya setara, dengan ekor latensi (p95) ACS kita sedikit lebih rendah (152–222 ms vs 206–258 ms).
- **Titik lemah ACS kita: fase tugas** (p95 939 ms vs 594 ms) dan **beban MariaDB** (rata-rata 3 inti; sebagian besar saat tulis
  parameter/riwayat per Inform). Ini kandidat optimasi nyata (batch `UpsertBatch`, kurangi tulis event/sesi per Inform).
- **Keunggulan ACS kita: jejak memori proses** (27–112 MiB vs 572 MiB) dan fitur platform (lihat di bawah). GenieACS membagi beban ke
  banyak proses worker (CPU proses CWMP-nya 4+ inti), ACS kita satu proses Go.
- **Perbedaan perilaku yang memengaruhi angka:** ACS kita memverifikasi kredensial CWMP per tenant pada tiap Inform (Basic Auth);
  GenieACS bawaan **tanpa autentikasi** CWMP. ACS kita juga mencatat sesi, event, dan audit per Inform (tulis lebih banyak) —
  itu harga fitur, bukan semata inefisiensi.
- **Tidak diukur:** jutaan perangkat, retensi data berhari-hari, beban REST/UI bersamaan, kegagalan jaringan, dan perilaku firmware
  nyata (yang sudah menemukan bug `xsi:type` hari ini dan tidak tertangkap simulator).

## Optimasi lanjutan — 22 Oktober 2026

Titik lemah di atas (beban MariaDB, fase tugas) ditelusuri lewat `general_log` MariaDB dan selisih `Com_*` per sesi. Temuan: tiap query
dikirim sebagai Prepare+Execute+Close (3 round trip), dan hot path Inform/tugas melakukan SELECT/UPDATE yang berulang. Perubahan:

| Perubahan | Efek |
|---|---|
| `interpolateParams=true` otomatis pada koneksi DB (`repository/mysql/db.go`) | satu query = satu round trip |
| Namespace CWMP ikut `INSERT device_sessions` (tanpa `UPDATE` + invalidasi cache Redis + `SELECT` ulang) | −1 UPDATE, −1 SELECT, −Redis ops per sesi baru |
| `UpsertBatch` satu chunk tanpa `BEGIN/COMMIT` | −2 round trip, −1 commit per Inform |
| Cache TTL 5 dtk aturan ZTP & preset enforce (`repository/hotcache`; mutasi di instance sama langsung membuang cache) | −2 SELECT per Inform |
| `MarkSent`/`Complete` memakai task yang sudah dipegang; event status tak mengambil ulang task | −3 SELECT per tugas |

Query per sesi (500 perangkat, 4 sesi/perangkat, MariaDB `Com_*`): SELECT 13,3→10,3 · UPDATE 3,5→2,5 · commit 1,5→0,2.

**A/B beban penuh** (1.500 perangkat, concurrency 64, 4 putaran, 500 tugas; stack baru & DB kosong tiap run; urutan selang-seling; 0 galat
di semua run). Setiap sel = dua run, sesi/dtk · p95:

| Fase | Sebelum | Sesudah |
|---|---|---|
| Registrasi | 144 · 704 ms / 160 · 750 ms | 224 · 442 ms / 329 · 274 ms |
| Periodik putaran 3 | 285 · 363 ms / 341 · 288 ms | 602 · 152 ms / 709 · 121 ms |
| Periodik putaran 4 | 310 · 342 ms / 403 · 247 ms | 636 · 144 ms / 701 · 124 ms |
| Tugas (500) | 66 · 1400 ms / 89 · 1125 ms | 79 · 1483 ms / 108 · 813 ms |
| MariaDB CPU rata-rata | 265% / 294% | 246% / 249% |
| Memori `acsd` maks | 140 MiB | 27–30 MiB |

- **Jalur Inform ±1,7–2× lebih cepat** dan p95 turun 2–3×; selisih antar-run jauh lebih kecil daripada selisih sebelum/sesudah.
- **Fase tugas hanya membaik tipis dan masih berisik** (p95 813–1483 ms): belum bisa diklaim. Sisa waktunya kemungkinan di jalur antre via API
  (cek kuota, `activity_logs`, `INSERT tasks`) dan fsync MariaDB — belum diprofil. Pada hari yang sama GenieACS belum diukur ulang.
- **Mesin hari ini lebih lambat dari 21 Okt** (baseline fase tugas 66–89 sesi/dtk vs 118 kemarin), jadi angka absolut kedua hari TIDAK
  boleh dibandingkan; hanya pasangan sebelum/sesudah di hari yang sama yang valid. Tabel GenieACS di atas belum diperbarui.
- Kompromi konsistensi: aturan ZTP/preset yang diubah lewat instance lain baru terlihat setelah ≤5 dtk (bukan konsistensi instan).
- Verifikasi: `go vet`/`go test ./...` hijau, e2e 30/30 lulus pada kode baru.

## Perbandingan fitur (diverifikasi pada GenieACS 1.2.16 yang berjalan)

NBI GenieACS menjawab 200 pada `/devices`, `/tasks`, `/presets`, `/provisions`, `/virtual_parameters`, `/files`, `/faults`, `/objects`;
query bebas dengan `projection`/`query` pada `/devices` bekerja; UI di port 3000.

| Kemampuan | ACS kita | GenieACS 1.2.16 |
|---|---|---|
| Multi-tenant, RBAC bertingkat, MFA TOTP, SSO OIDC | ✅ | ❌ (peran dasar; tanpa tenant) |
| Audit trail, notifikasi in-app/email, webhook, billing SaaS | ✅ | ❌ |
| Tab WiFi, dashboard optik/suhu/uptime, retry task, "Refresh ONT", capture sesi + redaksi rahasia | ✅ | ❌ (UI generik) |
| Katalog vendor/path sebagai data, enkripsi rahasia task | ✅ | ⚠️ lewat skrip/provision |
| Provision berskrip (JavaScript), **virtual parameter** | ❌ | ✅ |
| Query perangkat bebas + proyeksi + sort | ⚠️ filter tetap | ✅ |
| Resource `faults` tersendiri | ✅ (22 Okt 2026: `GET/DELETE /faults`, UI Fault) | ✅ |
| Kematangan lintas firmware, skala produksi | ❌ baru 3 ONT fisik | ✅ bertahun-tahun |

**Kesimpulan:** lebih lengkap sebagai produk operasional multi-tenant, setara pada throughput simulator, tetapi **belum lebih matang**
dan belum menyamai fleksibilitas provisioning GenieACS. Klaim "lebih bagus" hanya layak untuk fitur platform, bukan untuk keandalan.
