# PENGUJIAN_E2E.md — Uji End-to-End Otomatis (Simulator CPE)

Dokumen ini menjelaskan **harness uji end-to-end** ACS: satu program Go
(`cmd/e2e`) yang menjalankan skenario nyata terhadap stack ACS yang benar-benar
hidup (`docker-compose`: REST API + endpoint CWMP + MariaDB + MinIO + Redis),
memakai **simulator CPE TR-069 multi-vendor** (`internal/simcpe`) sebagai
"perangkat lapangan".

**Hubungan dengan `PENGUJIAN_LAPANGAN.md`:** dokumen itu untuk uji dengan CPE
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

## 1. Yang Diuji (9 skenario)

| # | Skenario | Yang dibuktikan |
|---|---|---|
| S1 | Onboarding multi-vendor | 5 profil vendor kirim Inform `0 BOOTSTRAP` → device tercatat, `tenant_id` ter-assign dari shared secret CWMP, `vendor_id` ter-resolve dari OUI, event tersimpan |
| S2 | Provisioning push | Buat provisioning profile → `apply-profile` → sesi berikutnya CPE menerima `SetParameterValues`, data model CPE berubah, task `COMPLETED` |
| S3 | Reboot RPC | `POST /devices/:id/reboot` → CPE menerima `Reboot`, task `COMPLETED` |
| S4 | GetParameterValues ad-hoc | NOC minta baca parameter → CPE menjawab → nilai tersimpan di `device_parameters` |
| S5 | Fault 9005 tidak di-retry | CPE membalas `cwmp:Fault` 9005 (Invalid Parameter Name) → task `FAILED` permanen dalam 1 percobaan, TIDAK dikirim ulang di sesi berikutnya |
| S6 | Isolasi tenant | Admin tenant B tidak melihat device tenant A (list kosong + `GET /devices/:id` → 403/404); admin tenant A tetap melihat device-nya |
| S7 | Connection Request | ACS men-trigger CR ke `ConnectionRequestURL` (di-capture otomatis dari Inform) → CPE membuka sesi baru dengan event `6 CONNECTION REQUEST` |
| S8 | Firmware canary rollout | Upload file → buat rollout batch wave 100% → CPE menerima `Download` + mengirim `TransferComplete` → batch `COMPLETED` tanpa kegagalan |
| S9 | Preset drift-heal | Preset `enforce=1`, device menyimpang → ACS otomatis mengantre `SetParameterValues` → CPE konvergen |

Status terakhir: **9/9 PASS**, stabil pada 3 run berturut-turut.

---

## 2. Prasyarat

- Docker + Docker Compose.
- Stack ACS hidup: `docker compose up -d` (menjalankan migrasi `0001`→`0021`).
- Satu user `SUPERADMIN` sudah di-seed.
- Node 20+ opsional (hanya untuk gerbang frontend, tidak dipakai harness ini).

Host **tidak** perlu Go toolchain — semua build lewat image `golang:1.26`.

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
  golang:1.26 sh -c 'go build -o /src/bin/e2e ./cmd/e2e && go build -o /src/bin/cpesim ./cmd/cpesim'

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
konsisten dengan `migrations/0004` yang sengaja mengosongkan `vendor_ouis`.
`cmd/e2e` menyuntik OUI ini ke katalog saat setup supaya `vendor_id`
ter-resolve; di luar itu device muncul "vendor belum diketahui" (memang
ekspektasi).

---

## 5. Batasan yang Jujur

- **Simulator ≠ firmware vendor nyata.** Tidak ada kuirk per-firmware, tidak ada
  variasi index WLAN/WAN antar model, tidak ada perilaku aneh saat parameter
  besar. Uji fisik (`PENGUJIAN_LAPANGAN.md`) tetap wajib sebelum klaim
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
