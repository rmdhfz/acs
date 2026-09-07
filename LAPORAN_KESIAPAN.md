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

## 5. Audit Performa Skala (`perf-scale-auditor`) — "tahan ribuan device"

**Semua temuan dari pembacaan kode — belum ada load test sungguhan.**
Hitungan: **≈17–20 round-trip DB per periodic Inform** (device dikenal, ~20
param, tanpa ZTP/preset). Pada 100.000 device @ interval 300s = ~333 Inform/detik
→ ~6.000 query/detik untuk inform "kosong"; jauh lebih tinggi bila preset enforce
aktif.

### 5.1 Diperbaiki sesi ini

| # | Temuan | Fix |
|---|---|---|
| B2 | `ref_*` (event code, status, trigger ZTP) di-query dari DB **tiap Inform** padahal isinya enum konstan — ~6 query/Inform sia-sia | **`mysql.NewCachedRefRepository`** — cache in-memory untuk tabel imutabel (`ref_event_codes`, `ref_task_status`, `ref_task_types`, `ref_device_status`, `ref_ztp_trigger_event`, `ref_firmware_rollout_status`, `ref_roles`, `ref_parameter_types`), TTL 10 menit. `GET /refs/:table` (UI) tetap segar. |
| B5 (parsial) | `MarkOnline` menulis `UPDATE devices` **setiap Inform** walau status sudah ONLINE; `SetCWMPNamespace` `UPDATE device_sessions` tiap Inform walau namespace tak berubah | `MarkOnline` menerima state device & **skip UPDATE bila sudah ONLINE**; `SetCWMPNamespace` **hanya menulis bila namespace berubah** |

### 5.2 BLOCKER untuk klaim "ribuan device" — belum dikerjakan (butuh desain / load-test data)

| # | Temuan | Rekomendasi | Mulai sakit di ~N |
|---|---|---|---|
| **B1** | Tabel jalur-panas tumbuh **tanpa retensi/pruning/partisi**: `device_sessions` (tak pernah dihapus, hanya OPEN→TIMEOUT), `device_events` (per event per Inform), `device_optical_metrics` (tiap Inform ONT, bukan saat berubah), `activity_logs`. 100k device ≈ 28 juta baris/hari `device_sessions`. | **Kebijakan retensi + job pruning WAJIB sebelum go-live penuh** (mis. hapus `device_sessions` non-OPEN > 7 hari, `device_events`/`optical` > 30–90 hari). Partisi RANGE by-month menyusul setelah ada data volume (keputusan sengaja ditunda di CLAUDE.md). | beberapa hari produksi @ puluhan ribu device |
| **B3** | N+1 di `EvaluatePresets`/ZTP: resolusi path per-op per-preset tiap Inform. `ResolveParameterPath` = 3 query (devices+deviceModels+paramMappings) per op, tak dicache. 5 preset × 10 op = 200 query/Inform/device. | Resolve `VendorID`/`DeviceModelID`/`DataModelVersionID` sekali per Inform; cache `vendor_parameter_mappings` per `(vendor,dmv,model)`; ambil `device_parameters` relevan dalam 1 query `IN (...)`. | 1–5k device/tenant dengan preset enforce |
| **B4** | Rate limiter `/cwmp` (5 req/s) & REST (30) **in-memory per-instance** + di-key **per-IP**. Multi-instance → limit efektif N×; **CGNAT → ribuan CPE satu ISP berbagi jatah 5 req/s → Inform sah ditolak 429 massal**. | Store terdistribusi (Redis, sudah ada). Untuk `/cwmp`: rate-limit per-device (OUI+serial) atau per-tenant, bukan per-IP. **Risiko tinggi**: bikin auto-provisioning gagal untuk fleet di belakang CGNAT. | begitu >1 instance ATAU fleet di CGNAT |
| **B5** (penuh) | `FindOrCreateFromInform` → `devices.Update` **rewrite ~20 kolom termasuk `connection_request_password_enc`, `inform_password_enc`** tiap Inform. | `UPDATE devices SET last_inform_at=?, ip_address=?, software_version=?, ... WHERE id=?` — jangan pernah rewrite kolom kredensial di jalur Inform. (Butuh method repo baru + update 4 fake test.) | 20–50k device |

### 5.3 Akan jadi masalah pada N tertentu — dicatat

- **C1** `HasPendingForDevice` dipanggil 2× per Inform (ZTP + preset) — gabungkan.
- **C3** Listing device: `serial LIKE '%x%'` (leading wildcard → full scan) + filter tag `EXISTS` — tambah `KEY device_tags (tag_id, device_id)`, ubah ke JOIN; batasi search ke prefix / FULLTEXT.
- **C4** `SweepRolloutBatches` tarik SEMUA batch (termasuk COMPLETED historis) tiap 60s — tambah filter `status_id IN (...)` di SQL.
- **C5** WS Hub **in-memory** — dengan >1 instance, event `DEVICE_ONLINE`/`TASK_STATUS_CHANGED` hanya sampai ke klien di instance yang memproses. Dashboard NOC parsial. Fan-out via Redis pub/sub. (Pelanggaran stateless TECH.md §9.)
- **C6** Auto-discovery: `go handleAutoDiscoveryResponse(context.Background(), ...)` fire-and-forget tanpa pool/timeout, dipicu tiap BOOTSTRAP device vendor tak dikenal — worker pool + timeout.
- **C7** Connection pool `MaxOpenConns(50)`/`MaxIdleConns(10)` — evaluasi 100–200 & naikkan idle setelah profiling.

### 5.4 Dikonfirmasi OK
`tasks.NextForDevice` (index `device_id,status,priority`, no global polling),
`deviceParams.UpsertBatch` (multi-row per chunk 500), `GetByOUISerial` (unique
index), session state di `device_sessions`+Redis (bukan in-memory),
`webhook_deliveries.ClaimDue` (guarded UPDATE aman multi-instance),
`TriggerConnectionRequest` (timeout + CheckRedirect + SSRF guard).

---

## 6. Gap Menuju Go-Live Penuh (diurut dampak)

1. **Uji CPE fisik** — `PENGUJIAN_LAPANGAN.md` siap. Minimal 1–2 unit per vendor.
   Satu Inform nyata > sebulan simulasi. **Penahan #1.**
2. **`vendor_ouis` kosong** — isi dari IEEE OUI registry sebelum uji lapangan.
3. **Retensi tabel log (B1)** — kebijakan pruning sebelum rollout lebar.
4. **Rate limiter terdistribusi (B4)** — bila deploy multi-instance atau ada CPE di CGNAT.
5. **Load test kapasitas** — butuh rig multi-IP; validasi B3/B5/C7 dengan angka nyata.
6. **Follow-up keamanan non-blocker** di §4.3.
7. **USP/TR-369** — Fase 3, tidak menghalangi go-live CWMP.

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
