# LAPORAN KESIAPAN ACS — 8 September 2026

Laporan status jujur menjelang rencana go-live. Disusun setelah sesi kerja
otonom yang: (1) membangun harness uji end-to-end + simulator CPE multi-vendor,
(2) menjalankan 9 skenario e2e sampai 100% lulus terhadap stack hidup,
(3) audit keamanan OWASP Top 10, (4) memperbaiki temuan keamanan blocker.

---

## 1. Ringkasan Eksekutif

| Aspek | Status |
|---|---|
| Alur ACS end-to-end (onboarding → provisioning → firmware → preset → connection request) | ✅ **Terverifikasi** lewat 9 skenario e2e otomatis, 5 profil vendor, stack hidup (docker-compose + MariaDB + MinIO), stabil 3× berturut-turut |
| Migrasi `0001`→`0021` ke MariaDB nyata | ✅ jalan bersih |
| Gerbang CI (gofmt, vet, build, `go test -race`, frontend, openapi lint) | ✅ hijau |
| Audit keamanan OWASP Top 10 | ✅ selesai — 2 blocker **diperbaiki**, sisanya follow-up non-blocker |
| **Uji CPE fisik (firmware vendor sungguhan)** | ❌ **Belum** — gap #1, tidak bisa disimulasikan |
| Load test kapasitas puncak (ribuan sesi concurrent) | ⚠️ Terbatas rate limiter; fleet 40 sesi / 5 vendor OK |
| USP / TR-369 | ❌ Kerangka saja |

**Verdict:** Sistem **siap untuk pilot terbatas** (1 tenant, populasi perangkat
kecil, pemantauan ketat). **Belum siap untuk rollout penuh serentak** ke semua
anak perusahaan — penahannya adalah nol bukti dari CPE fisik, bukan cacat
software yang diketahui.

---

## 2. Apakah "lebih baik dari GenieACS"?

Penilaian per dimensi (audit kode langsung, bukan klaim dokumen):

| Dimensi | vs GenieACS |
|---|---|
| UI/UX (dashboard, dark mode, mobile, command palette, i18n) | **Unggul** |
| Multi-tenancy native + isolasi terverifikasi e2e | **Unggul** |
| Dokumentasi API (OpenAPI 103 operation + Postman) | **Unggul** — GenieACS minim |
| Model data terstruktur & auditable (vs skrip JS bebas) | **Unggul** untuk auditability; **kurang** untuk ekspresivitas |
| Keamanan (audit berlapis, lockout, RBAC, enkripsi, kini + anti-SSRF + BodyLimit + ENDUSER dikurung) | **Unggul** |
| Audit trail (7 kolom + activity_logs + timeline per-device) | **Unggul** |
| Firmware canary rollout dengan ambang failure rate | **Unggul** |
| Cakupan RPC CWMP, observability, penanganan NAT | **Setara** |
| **Kematangan lapangan lintas firmware vendor** | **Jauh tertinggal** — GenieACS bertahun-tahun di produksi |
| USP/TR-369 | **Tertinggal** |

**"Jauh lebih baik secara keseluruhan" belum bisa diklaim** sampai ada bukti
lapangan. Unggul di ~7 dari 10 dimensi secara kode.

---

## 3. Yang Diverifikasi Sesi Ini

### 3.1 Harness uji end-to-end (baru)
- `internal/simcpe/` — simulator CPE TR-069 multi-vendor: klien CWMP sungguhan,
  data model in-memory yang berubah saat di-Set, 5 profil vendor
  (ZTE/Huawei/FiberHome/Nokia/Cdata) dengan namespace CWMP `cwmp-1-0`/`1-1`/`1-2`
  dan root TR-098/TR-181 yang bervariasi.
- `cmd/cpesim/` — CLI: single device, fleet, fault injection, listener Connection Request.
- `cmd/e2e/` — 9 skenario terhadap stack hidup + assertion via REST API.
- Dokumentasi: `PENGUJIAN_E2E.md`. Runner: `scripts/e2e.ps1`.

### 3.2 Hasil 9 skenario e2e — **9/9 LULUS** (stabil 3×)

| # | Skenario | Hasil |
|---|---|---|
| S1 | Onboarding 5 vendor (BOOTSTRAP → device tercatat, tenant & vendor ter-resolve, event tersimpan) | ✅ |
| S2 | Provisioning push (profile → apply → CPE menerima SetParameterValues, data model berubah, task COMPLETED) | ✅ |
| S3 | Reboot RPC | ✅ |
| S4 | GetParameterValues ad-hoc → nilai tersimpan | ✅ |
| S5 | Fault 9005 → task FAILED permanen (1 percobaan), tidak dikirim ulang | ✅ |
| S6 | Isolasi tenant (admin B tidak lihat device tenant A; 403 pada GET by id) | ✅ |
| S7 | Connection Request (URL di-capture otomatis dari Inform → ACS trigger → Inform balik `6 CONNECTION REQUEST`) | ✅ |
| S8 | Firmware canary rollout (Download + TransferComplete → batch COMPLETED tanpa kegagalan) | ✅ |
| S9 | Preset drift-heal (enforce=1, device menyimpang → ACS auto-SetParameterValues → konvergen) | ✅ |

### 3.3 Fleet test
20 device (5 vendor), 2 siklus (BOOTSTRAP + PERIODIC) = **40/40 sesi OK**.

---

## 4. Perbaikan Keamanan Sesi Ini

### 4.1 BLOCKER — diperbaiki

**B1. SSRF via `connection_request_url` yang dikontrol CPE** (diperkenalkan oleh
fitur baru "capture ConnectionRequestURL dari Inform"). CPE terkompromi bisa
melaporkan `ManagementServer.ConnectionRequestURL = http://169.254.169.254/...`
lalu ACS meng-GET-nya (dengan kredensial) saat operator memicu Connection
Request → oracle port-scan internal / akses metadata cloud.
**Fix:** (a) saat capture, URL hanya disimpan bila host-nya == IP sumber Inform
(`safeInformConnectionRequestURL`); (b) saat trigger, guard `pkg/netguard` tolak
loopback/link-local/metadata + `CheckRedirect` tolak redirect; (c) validasi sama
untuk `UDPConnectionRequestAddress` (jalur STUN); (d) status/host upstream tidak
lagi bocor ke response API.

**B2. Role ENDUSER tidak dikurung ke `/self-service`** (pre-existing). Token
portal pelanggan bisa memanggil `GET /devices`, `/tasks`, dll. → membaca data
seluruh tenant.
**Fix:** middleware `staffOnly` pada grup `authed` — ENDUSER kini **403** di
seluruh API staf, hanya `/self-service/*` + ganti password sendiri yang boleh.
Diverifikasi: 6 endpoint staf → 403, `/self-service/devices` → 200.

### 4.2 NON-BLOCKER — diperbaiki

- **BodyLimit** ditambahkan: REST 2 MiB (upload firmware dikecualikan), CWMP 4 MiB
  — menutup memory-exhaustion via satu POST raksasa ke endpoint pra-auth.
- **`CreateUser`** kini memvalidasi panjang password minimum (disamakan dengan
  reset/change password).
- **SSRF webhook `target_url`** — `pkg/netguard` juga diterapkan di webhook +
  `CheckRedirect`, re-cek saat dispatch.

### 4.3 NON-BLOCKER — dicatat, belum dikerjakan

| Temuan | Rekomendasi |
|---|---|
| OIDC `state`/`nonce` tidak divalidasi (`auth_handler.go`) | Generate state acak + cookie; atau disable route `/auth/oidc/*` bila OIDC tidak dipakai go-live |
| Rate limiter in-memory per-instance | Ganti store berbasis Redis sebelum horizontal scale (single-instance go-live: OK) |
| API token tanpa `expires_at` boleh dibuat permanen | Wajibkan expiry / default maksimum 1 tahun |
| `GET /metrics` tanpa auth (by design) | Pastikan difirewall di ingress produksi — jangan ter-expose bareng REST |
| `govulncheck` belum dijalankan sesi ini | Tambahkan ke pipeline CI |
| Security headers (HSTS/CSP/X-Content-Type-Options) | Konfirmasi ditangani reverse proxy, atau tambah `middleware.Secure()` |

### 4.4 Dikonfirmasi AMAN
A02 kripto (AES-256-GCM, bcrypt, JWT HS256 + alg-check), A03 injection (sqlx
parameterized, whitelist refs, `encoding/xml` stdlib tidak rentan XXE),
isolasi tenant lintas endpoint, auth CWMP Inform (shared secret per tenant +
anti-spoofing).

---

## 5. Gap Menuju Go-Live Penuh (diurut dampak)

1. **Uji CPE fisik** — `PENGUJIAN_LAPANGAN.md` siap. Minimal 1–2 unit per vendor.
   Satu Inform nyata > sebulan simulasi. **Penahan #1.**
2. **`vendor_ouis` kosong** — isi dari IEEE OUI registry sebelum uji lapangan.
3. **Load test kapasitas** — butuh rig multi-IP (rate limiter `/cwmp` 5 req/s
   per IP membatasi pengukuran dari satu sumber).
4. **Follow-up keamanan non-blocker** di §4.3.
5. **USP/TR-369** — Fase 3, tidak menghalangi go-live CWMP.

---

## 6. Rekomendasi

**Untuk tanggal go-live:** lakukan **soft-launch** — satu anak perusahaan,
1 tenant pilot, ≤ beberapa ratus perangkat, wave provisioning kecil, pemantauan
dashboard + alert Prometheus aktif. Jalankan `PENGUJIAN_LAPANGAN.md` di hari yang
sama dengan perangkat fisik yang tersedia. Tahan rollout ke tenant lain sampai:
(a) minimal 1 vendor terverifikasi di lapangan tanpa kejutan besar, (b) OUI
terisi, (c) follow-up keamanan §4.3 (minimal OIDC state bila SSO dipakai)
selesai.

**Dokumen pendamping:** `BUKU_PANDUAN_ACS.pdf` (panduan operasional),
`PANDUAN_LOGIN_ACS.pdf`, `PENGUJIAN_E2E.md`, `PENGUJIAN_LAPANGAN.md`,
`RUNBOOK.md`, `openapi.yaml`.
