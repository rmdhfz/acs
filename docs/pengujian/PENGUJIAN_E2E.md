# PENGUJIAN_E2E.md — Uji End-to-End Otomatis (Simulator CPE)

Dokumen ini menjelaskan **harness uji end-to-end** ACS: satu program Go
(`cmd/e2e`) yang menjalankan skenario nyata terhadap stack ACS yang benar-benar
hidup (`docker-compose`: REST API + endpoint CWMP + MariaDB + MinIO + Redis),
memakai **simulator CPE TR-069 multi-vendor** (`internal/simcpe`) sebagai
"perangkat lapangan".

**Hubungan dengan `docs/pengujian/PENGUJIAN_LAPANGAN.md`:** dokumen itu untuk uji dengan CPE
**fisik** (ZTE/Huawei/FiberHome/Nokia/Cdata sungguhan). Harness di sini
**melengkapi**, bukan menggantikan — simulator tidak menjalankan firmware vendor
nyata dan tidak punya kuirk per-firmware. Gunanya:

1. Membuktikan alur ACS end-to-end (onboarding → provisioning → firmware →
   preset → connection request) benar-benar bekerja, berulang, otomatis.
2. Jaring pengaman regresi untuk jalur sesi CWMP (jalur paling rapuh & paling
   sulit di-review) sebelum tiap rilis.
3. Latihan lintas kombinasi yang jarang ada di lab: TR-098 vs TR-181,
   namespace CWMP `cwmp-1-0`/`1-1`/`1-2`, 5 profil vendor sekaligus.

---

## 1. Yang Diuji (22 skenario)

| # | Skenario | Yang dibuktikan |
|---|---|---|
| S1 | Onboarding multi-vendor | 5 profil vendor kirim Inform `0 BOOTSTRAP` → device tercatat, `tenant_id` ter-assign dari shared secret CWMP, `vendor_id` ter-resolve dari OUI, event tersimpan |
| S2 | Provisioning push | Buat provisioning profile → `apply-profile` → sesi berikutnya CPE menerima `SetParameterValues`, data model CPE berubah, task `COMPLETED`. Profil memuat parameter **sensitif** (`KeyPassphrase`): tersegel AES-GCM di DB, tetapi CPE menerima **plaintext** |
| S3 | Reboot RPC | `POST /devices/:id/reboot` → CPE menerima `Reboot`, task `COMPLETED` |
| S4 | GetParameterValues ad-hoc | NOC minta baca parameter → CPE menjawab → nilai tersimpan di `device_parameters` |
| S5 | Fault 9005 tidak di-retry | CPE membalas `cwmp:Fault` 9005 (Invalid Parameter Name) → task `FAILED` permanen dalam 1 percobaan, TIDAK dikirim ulang di sesi berikutnya. Lalu `POST /tasks/:id/retry` (Coba lagi): task SAMA kembali `PENDING` (retry_count 0, tanpa task baru), dikirim ulang dan `FAILED` lagi; tenant lain 403, task tak ada 404, task `COMPLETED` 409 |
| S6 | Isolasi tenant | Admin tenant B tidak melihat device tenant A (list kosong + `GET /devices/:id` → 403/404); admin tenant A tetap melihat device-nya |
| S7 | Connection Request | ACS men-trigger CR ke `ConnectionRequestURL` (di-capture otomatis dari Inform) → CPE membuka sesi baru dengan event `6 CONNECTION REQUEST`. CPE simulasi **mewajibkan HTTP Digest**: ACS harus menjawab tantangan 401; password salah ditolak tanpa Inform |
| S8 | Firmware canary rollout | Upload file → buat rollout batch wave 100% → CPE menerima `Download` + mengirim `TransferComplete` → batch `COMPLETED` tanpa kegagalan |
| S9 | Preset drift-heal | Preset `enforce=1`, device menyimpang → ACS otomatis mengantre `SetParameterValues` → CPE konvergen |
| S10 | Regresi keamanan | Role ENDUSER dikurung ke `/self-service` (403 di API staf); Connection Request ke alamat metadata-cloud ditolak (SSRF guard) |
| S11 | CRUD lengkap | Katalog (vendor/OUI/model/mapping) get·patch·delete + **guard 409** bila masih dipakai (vendor ZTE nyata), **revive OUI** yang pernah dihapus, device delete → task terbuka **CANCELLED** → Inform berikutnya **revive** (id sama, task lama tidak jalan), tenant get/delete (guard user/device) + scope, GET user by id + scope, firmware get/patch/delete (identitas isi file tak berubah), tag patch (+ tag global hanya superadmin), preset get (scope), ZTP get 404 |
| S12 | Auth email & reset | Login dengan **email** (tak peka huruf besar, spasi dipangkas) + kompatibilitas username; pesan 401 **seragam** (salah password vs email tak dikenal); `forgot-password` 202 generik untuk email tak terdaftar + **429** rate limit; `reset-password` token ngawur → 400; **admin reset** → password baru berlaku, lama ditolak, dan **JWT lama dicabut (401)**. Tidak mengirim email ke alamat nyata (domain `@e2e.local` dibuang mailer). Alur token email→tautan→password baru dibuktikan di test unit + integrasi MariaDB (`ACS_TEST_DSN`) + uji manual |
| S13 | Pra-registrasi & impor perangkat | `POST /devices` (ADMIN dikunci ke tenant sendiri; duplikat/tenant lain 409; OUI/MAC/koordinat/vendor salah 400; superadmin wajib tenant), **PATCH menyimpan notes/MAC/lokasi/vendor** (regresi: repository dulu tidak menulis latitude/longitude), `POST /devices/import` — **dry-run mengklasifikasi tanpa menulis**, eksekusi = pratinjau, impor ulang idempoten, mode skip, tenant lain ditolak tanpa membocorkan pemilik; **Inform pertama CPE pra-registrasi memakai baris yang sama** (id sama, catatan terjaga, status berubah, vendor ter-resolve) sementara tenant LAIN ditolak |
| S14 | Diagnostics Ping & Traceroute | `POST /devices/:id/diagnostics` (host/tipe divalidasi: host terlarang & tipe salah → 400) → task terstruktur via logical key `diag.*` (mapping, bukan percabangan vendor) → CPE menjalankan dan mengirim Inform berevent **`8 DIAGNOSTICS COMPLETE`** → ACS membaca hasil → status `COMPLETED` + hasil terstruktur; diuji pada dua vendor/data model berbeda (ZTE Ping, Nokia Traceroute) |
| S15 | Redaman optik berbasis mapping | Device TR-181 (Nokia, model belum terdaftar): ACS mendeteksi data model dari parameter, me-resolve `optical.*` (standar `Device.Optical.Interface`, `value_scale` 0,001), mengantre GetParameterValues; hasil disimpan sebagai metrik dBm (−23500 → −23,5; 2100 → +2,1) |
| S16 | MFA TOTP | Enrol (setup→enable, kode salah 400, 10 kode pemulihan) → login dua tahap (`mfa_required` tanpa `access_token`) → token tahap-2 **bukan sesi** (401 di endpoint lain) → kode salah 401 → kode benar → sesi sah → **replay kode ditolak** → kode pemulihan sekali pakai → ADMIN tenant lain mereset MFA 403, ADMIN tenant sama berhasil, sesi lama dicabut, login kembali satu tahap |
| S17 | Pohon parameter & atribut notifikasi | `POST /devices/:id/refresh` menelusuri pohon secara rekursif (14 `GetParameterNames` hingga daun, kedalaman 4) lalu membaca nilai; `SetParameterAttributes` diterima CPE (tenant lain ditolak) dan perubahan nilai memunculkan event `4 VALUE CHANGE`. Menemukan & memperbaiki bug nyata: respons `GetParameterNames` berbentuk terbungkus tidak pernah terbaca |
| S18 | Portal pelanggan | ENDUSER tanpa tautan tidak melihat apa pun (404); tenant lain 403; ADMIN menautkan akun↔perangkat; pelanggan melihat 1 perangkat, validasi WiFi (SSID/passphrase) 400, perubahan SSID **sampai ke CPE**; lepas tautan → 404 |
| S19 | Operator SaaS | Laporan SaaS (hanya SUPERADMIN, 30 hari, CSV; ADMIN tenant 403). **Impersonate**: token ADMIN berumur pendek + jejak impersonator, isolasi tenant utuh, 6 aksi platform/sensitif 403 (laporan, tenant, impersonate ulang, ganti password, MFA, token API), ADMIN/diri sendiri ditolak, audit `IMPERSONATE_START/END` + `impersonated_by` pada aksi |
| S20 | Paket & self-signup | Batas perangkat per tenant: Inform perangkat baru **503** tanpa membuat baris, pra-registrasi **429**, perangkat yang sudah ada tetap diterima, menaikkan batas memulihkan, ADMIN tenant 403. Self-signup **mati secara bawaan**: signup & verify → 403 (jalur aktif diuji di unit + integrasi MariaDB, bukan e2e, karena butuh SMTP) |
| S21 | Vendor generik (semua ONT) | Migrasi 0029: CPE TR-098 ber-OUI TP-Link (OUI seed IEEE) → `vendor_id` ter-resolve; logical key `wifi.ssid` & `wan.pppoe.username` jatuh ke mapping standar vendor `GENERIC` dan nilainya berubah di CPE. CPE TR-181 ber-OUI **tak dikenal** (vendor NULL) → data model terdeteksi dari parameter Inform, logical key tetap ter-resolve ke path standar TR-181, task `COMPLETED` |
| S22 | Notifikasi | Pengumuman SUPERADMIN ke tenant pilot → ADMIN pilot melihatnya (unread bertambah, id UUID, isi sebagai teks), tenant pilot2 **tidak**; ENDUSER 403; ADMIN tak bisa mengumumkan (403); link/judul tak sah & tenant tak ada → 400; mark-read menurunkan unread; jenis wajib tak bisa dimatikan |
| S23 | Billing (mati) | Billing bawaan MATI: 3 paket contoh, subscription `enabled=false`, checkout/sync 409, webhook 404, RBAC (ENDUSER/NOC/ADMIN), CRUD paket tervalidasi. **Pembayaran Xendit nyata TIDAK diuji.** |
| S24 | Validasi masukan | Query rusak (page/page_size/*_id/search) → 400; user/tenant cacat → 400 di POST **dan** PATCH; parameter task melewati batas → 400; login/reset dengan payload raksasa → 400; Inform dengan OUI/serial cacat → 400 (bukan 500) |
| S25 | UDP CR & RPC dari CPE | CPE hanya beralamat UDP (tanpa URL TCP) dan CPE dengan URL TCP tertutup → ACS fallback ke UDP Connection Request (Annex G, HMAC-SHA1): tanda tangan benar memicu Inform '6 CONNECTION REQUEST', HMAC salah dibuang tanpa Inform; CPE mengirim GetRPCMethods (dijawab daftar method), Kicked/RequestDownload (Fault 9000) dan task yang menunggu tetap COMPLETED |
| S26 | Fitur Console & IDOR | Layout menu per pengguna (tenant lain/superadmin tak melihat; bentuk jahat 400); laporan bug (dua kolom terpisah, tenant/pelapor dari token, body/field multipart bertenant_id 400, tangkapan layar: PNG sah diterima & diambil pelapor/SUPERADMIN, teks/SVG/PNG rusak 400, gambar tenant lain 404, lintas tenant 404, hanya SUPERADMIN ubah status, pelapor menerima notifikasi "sudah ditangani"); riwayat impor/ekspor (daftar dikunci ke tenant, tenant_id klien diabaikan, lintas tenant 404, SUPERADMIN lihat semua, angka tak wajar 400) |
| S27 | ONT nyata HG6145F (identitas) | Identitas ONT FiberHome HG6145F milik pengguna (OUI **58AEF1** dari seed IEEE tanpa OUI palsu, firmware RP2951): vendor ter-resolve ke FIBERHOME, versi tersimpan, logical key `wifi.ssid` → path TR-098 via mapping vendor diterima CPE tanpa fault. **Bukan bukti perangkat fisik** — lihat `CATATAN_ONT_HG6145F.md` |
| S28 | Ringkasan daftar perangkat | `POST /devices/refresh-summary`: `device_ids` kosong/>100 → 400, device tenant lain dilewati (tidak diantre), pemilik diantre; CPE menjawab `GetParameterValues` cabang PPP (path parsial) → `GET /devices` membawa `pppoe_username`; pencarian lewat username menemukan device (tenant lain tidak). S15 juga memastikan daftar membawa `rx_power_dbm`/`tx_power_dbm` terakhir |

Status terakhir: **24/24 PASS** (2026-10-18), termasuk run ulang penuh setelah penambahan vendor generik/OUI, revalidasi WebSocket, hitungan sesi per tenant, dan batas laju REST yang dapat diatur. Catatan jujur: seluruh skenario memakai **simulator** — bukan bukti perilaku pada firmware nyata (lihat `docs/pengujian/PENGUJIAN_LAPANGAN.md`).

---

## 2. Prasyarat

- Docker + Docker Compose.
- Stack ACS hidup: `docker compose up -d` (menjalankan migrasi `0001`→`0029`).
- Satu user `SUPERADMIN` sudah di-seed.
- Node 20+ opsional (hanya untuk gerbang frontend, tidak dipakai harness ini).

Host **tidak** perlu Go toolchain — semua build lewat image `golang:1.26.9`.

---

## 3. Cara Menjalankan

### Windows (PowerShell) — satu perintah

```powershell
pwsh scripts/e2e.ps1
```

Skrip ini: menyalakan stack bila belum, men-seed superadmin bila belum,
membangun binary `e2e` lewat Docker, menjalankannya di jaringan compose, lalu
menulis ringkasan JSON ke `bin/e2e-result.json`.

### Manual (POSIX / detail)

```bash
# 1. Stack
docker compose up -d
# tunggu sampai: curl -s localhost:18080/api/v1/metrics -o /dev/null -w '%{http_code}'  → 200

# 2. Superadmin (sekali saja per volume DB)
MSYS_NO_PATHCONV=1 docker compose exec -T acsd \
  /app/seed-admin -username e2e-super -password 'E2eSuperPass!2345' -email e2e-super@acs.local

# 3. Build harness (linux static, via Docker)
MSYS_NO_PATHCONV=1 docker run --rm -v "/d/Project/acs":/src -w /src -e CGO_ENABLED=0 \
  golang:1.26.9 sh -c 'go build -o /src/bin/e2e ./cmd/e2e && go build -o /src/bin/cpesim ./cmd/cpesim'

# 4. Jalankan di jaringan compose (nama container = -cr-host, dipakai ACS untuk Connection Request balik)
MSYS_NO_PATHCONV=1 docker run --rm --network acs_default --name e2e-runner \
  -v "/d/Project/acs/bin":/e2ebin:ro alpine:3 \
  /e2ebin/e2e -rest http://acsd:8080 -cwmp http://acsd:7547/cwmp \
  -super-user e2e-super -super-pass 'E2eSuperPass!2345' -json /tmp/e2e.json
```

Flag berguna:

| Flag | Arti |
|---|---|
| `-run S8` | jalankan hanya skenario yang namanya memuat substring `S8` |
| `-cr-host <host>` | host yang dilaporkan device sebagai `ConnectionRequestURL` (default: hostname container) |
| `-json <path>` | tulis ringkasan hasil ke file JSON |

Exit code `1` bila ada skenario `FAIL`.

---

## 4. Simulator CPE Manual (`cmd/cpesim`)

Untuk uji satu device secara manual / eksplorasi:

```bash
# Satu sesi BOOTSTRAP, profil ZTE, verbose
cpesim -acs http://localhost:17547/cwmp -user <cwmp_user> -pass <cwmp_pass> \
       -vendor zte -serial ZTE-DEMO-001 -events "0 BOOTSTRAP" -v

# Fleet: 20 device (5 vendor merata), 2 siklus (BOOTSTRAP lalu PERIODIC)
cpesim -acs ... -user ... -pass ... -fleet -devices 20 -cycles 2

# Uji jalur fault: balas Fault 9005 bila ACS mengirim SetParameterValues
# yang memuat substring nama parameter tertentu
cpesim -acs ... -user ... -pass ... -vendor huawei -fault-param X_BADPARAM

# Listener Connection Request (device menunggu ACS memicunya)
cpesim -acs ... -user ... -pass ... -vendor nokia -conn-req -cr-host host.docker.internal
```

Profil vendor (`internal/simcpe/vendor.go`):

| key | Data model | Namespace CWMP | Catatan |
|---|---|---|---|
| `zte` | TR-098 | `cwmp-1-0` | |
| `huawei` | TR-098 | `cwmp-1-2` | |
| `fiberhome` | TR-098 | `cwmp-1-1` | |
| `nokia` | TR-181 (`Device.`) | `cwmp-1-2` | tidak melaporkan `ConnectionRequestURL` |
| `cdata` | TR-098 | `cwmp-1-0` | |

OUI di profil adalah **placeholder** (mengandung huruf, bukan format IEEE) —
dan sengaja non-hex supaya tidak pernah bentrok dengan seed OUI IEEE asli di `migrations/0022` (sebelumnya `0004` mengosongkan `vendor_ouis`).
`cmd/e2e` menyuntik OUI ini ke katalog saat setup supaya `vendor_id`
ter-resolve; di luar itu device muncul "vendor belum diketahui" (memang
ekspektasi).

---

## 5. Batasan yang Jujur

- **Simulator ≠ firmware vendor nyata.** Tidak ada kuirk per-firmware, tidak ada
  variasi index WLAN/WAN antar model, tidak ada perilaku aneh saat parameter
  besar. Uji fisik (`docs/pengujian/PENGUJIAN_LAPANGAN.md`) tetap wajib sebelum klaim
  kompatibilitas vendor.
- **Load test terbatas rate limiter `/cwmp`** (5 req/s per IP). Dari satu
  mesin/IP, "ribuan sesi concurrent" tidak bisa diukur murni — butuh rig
  multi-IP. Harness ini memvalidasi *kebenaran alur*, bukan *kapasitas puncak*.
- **Data uji menumpuk di DB dev.** Tiap run membuat tenant + device + task baru
  (di-prefix run-id acak). Tidak ada cleanup otomatis (tidak ada endpoint hard
  delete). Bersihkan manual bila perlu, atau `docker compose down -v` untuk
  reset total (HATI-HATI: menghapus semua data dev).
- **`InsecureSkipVerify: true`** di simulator & e2e client — untuk dev/e2e yang
  memakai HTTP polos atau self-signed. Jangan pakai pola ini di kode produksi.
