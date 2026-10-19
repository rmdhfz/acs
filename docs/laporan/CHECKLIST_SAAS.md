# CHECKLIST KEMATANGAN SaaS — ACS Console

Tanggal: 17 Oktober 2026 (dihitung ulang setelah LANJUTAN 12) · Dasar: kode di branch `dev`, e2e 22/22, `go test -race` paket yang disentuh + integrasi MariaDB/Redis, audit UI otomatis (120 tampilan), 39 tes frontend, uji beban lab, uji dua instance.

**Rubrik:** 1 = ada & teruji otomatis (unit/integrasi/e2e/uji browser) · 0,75 = ada & teruji sebagian (mis. unit/simulator tanpa skenario e2e, atau belum terhadap sistem nyata) · 0,5 = parsial · 0,25 = kerangka · 0 = belum ada. Skor kategori = rata-rata item × bobot. Rubrik ini **dibuat sendiri** dan mengukur *kelengkapan & verifikasi di lingkungan uji*, bukan bukti perilaku di perangkat/pengguna nyata.

## Ringkasan

| Kategori | Bobot | 16 Okt | Sekarang | Poin |
|---|---:|---:|---:|---:|
| A. Protokol & inti ACS (TR-069) | 25 | 92% | **98%** | 24.6 |
| B. Aplikasi web & pengalaman pengguna | 15 | 82% | **95%** | 14.2 |
| C. Multi-tenant & identitas/akses | 15 | 89% | **95%** | 14.3 |
| D. Keamanan | 10 | 79% | **85%** | 8.5 |
| E. Operasional & deployment | 15 | 87% | **87%** | 13.0 |
| F. Kualitas & verifikasi | 10 | 82% | **84%** | 8.4 |
| G. Aspek SaaS komersial | 10 | 50% | **67%** | 6.7 |
| **TOTAL** | **100** | 82.6 | | **89.7** |

> Penilaian ulang 2026-10-18 (sesi lanjutan: UDP Connection Request & RPC dari CPE teruji e2e, UI A11y/inline-error/tes UI, menu/bug/riwayat impor-ekspor tanpa IDOR): skor dihitung ulang **dari skor per item** di bawah. **Tidak ada kategori yang 100%** — A tertahan path optik TR-098 per vendor (butuh dump ONT nyata), B oleh i18n penuh, C oleh Google & SMTP signup nyata, F oleh CPE fisik, G oleh Xendit nyata/legal/halaman status. Menaikkannya tanpa bukti hanya mengubah angka, bukan kesiapan.

## Dua angka yang perlu dibedakan

- **Kelengkapan fitur & verifikasi lab: ≈ 88%** (tabel di atas).
- **Kesiapan go-live produksi berbayar: ≈ 78%** = 88% dikurangi 12 poin diskon ketidakpastian lapangan: seluruh skor A/B memakai simulator CPE; belum ada satu pun Inform dari CPE fisik, sehingga item bernilai 1 di A belum terbukti pada firmware nyata (risiko interoperabilitas per vendor). Potongan ini tidak menghitung ganda item yang sudah bernilai 0.

Untuk **pilot/UAT internal** (Rabu 21 Okt) angka relevan adalah kelengkapan fitur; untuk **menjual sebagai SaaS** yang relevan adalah kesiapan go-live.

## Kenapa bukan 100% (dan apa yang dibutuhkan)

Semua yang dapat diverifikasi di lab sudah ditutup. Sisanya **tidak bisa saya buktikan sendiri** — mengklaim 100% di sini akan menyesatkan:

| Sisa | Kategori | Kenapa tertahan | Yang dibutuhkan |
|---|---|---|---|
| Perilaku pada firmware ONT/router nyata (Inform, Set bertipe, atribut, diagnostics, optik, UDP CR) — termasuk 28 vendor yang baru ditambahkan datanya | A/F (diskon go-live 12 poin) | simulator ≠ firmware vendor; OUI & mapping standar baru **data awal**, bukan hasil uji perangkat | **UAT Rabu** + pembaruan katalog dari hasilnya |
| Path optik TR-098 per vendor (ZTE/Huawei/FiberHome/C-Data memakai ekstensi `X_…`) | A | hanya path standar TR-181 yang dipetakan; menebak path vendor berisiko merusak konfigurasi | dokumentasi vendor atau dump `GetParameterNames` dari ONT nyata |
| SSO terhadap IdP organisasi sebenarnya (+ pengikatan akun lewat `sub`, saat ini lewat email) | C | hanya IdP tiruan yang tersedia | satu IdP uji (Keycloak/Azure AD/Google) |
| Penetration test independen | D | tidak bisa dinilai oleh pembuat kode | pihak ketiga |
| Paket harga, metering, penagihan; status page; ToS/Privasi/DPA final | G | keputusan bisnis & tinjauan hukum; halaman ToS/Privasi yang ada adalah **draf bertanda belum ditinjau** | keputusan Anda + penasihat hukum |
| i18n EN penuh, navigasi keyboard/jebakan fokus modal, Safari/Firefox/perangkat fisik | B | pekerjaan luas bernilai sedang / butuh perangkat | jadwal |
| Uji beban dengan Inform realistis & multi-instance | E | lab satu laptop | lingkungan uji beban |

## Rincian per item

### A. Protokol & inti ACS (TR-069) — 98% (bobot 25)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Sesi CWMP: Inform, RPC loop, cookie, namespace | ✅ 1 | e2e S1–S9; simulator 5 profil; **sesi dibuka di instance A, selesai di B** (state di DB) |
| RPC Get/Set/GetNames/Add/Delete/Reboot/FactoryReset/Download | ✅ 1 | e2e + unit; Set bertipe sesuai laporan CPE |
| TransferComplete + TransferCompleteResponse | ✅ 1 | handler + simulator + e2e S8 |
| Connection Request HTTP Digest/Basic + anti-SSRF | ✅ 1 | e2e S7, S10; penjaga SSRF saat sambungan (DNS rebinding) |
| Connection Request UDP (Annex G, HMAC-SHA1) | ✅ 1 | e2e **S25**: CPE hanya beralamat UDP (tanpa URL TCP) dan CPE ber-URL TCP tertutup → fallback UDP; HMAC benar memicu Inform `6 CONNECTION REQUEST`, HMAC salah dibuang tanpa Inform. Menemukan & memperbaiki bug nyata (CPE hanya-UDP ditolak 400). **Belum CPE nyata** |
| Data model TR-098 & TR-181 via katalog/mapping, **semua ONT** | ✅ 1 | 33 vendor (5 baseline + 27 tambahan + `GENERIC`), 4.304 OUI IEEE, 372 mapping, `value_scale`; **fallback standar** untuk vendor tak dikenal/NULL (e2e S21, unit, integrasi MariaDB). Catatan: data awal, belum diuji perangkat nyata |
| Provisioning Profile + Zero-Touch Rule | ✅ 1 | e2e S2, S13 |
| Preset penegak konfigurasi (drift-heal) | ✅ 1 | e2e S9 |
| Firmware: upload, canary rollout, auto-pause | ✅ 1 | e2e S8 |
| Antrean task: retry, backoff, fault 9005/9003, validasi | ✅ 1 | unit + e2e S5 |
| Penemuan pohon parameter / refresh rekursif | ✅ 1 | e2e S17 (14 GetParameterNames hingga daun); menemukan & memperbaiki bug nyata (bentuk respons) |
| SetParameterAttributes / notifikasi (event 4) | ✅ 1 | e2e S17 (atribut diterima CPE, event 4 tercatat). *Virtual parameters/skrip belum ada* |
| RPC dari CPE (GetRPCMethods dijawab; Kicked/RequestDownload Fault 9000) | ✅ 1 | e2e **S25** + `cpe_initiated_test`; task menunggu tetap COMPLETED. Belum CPE nyata |
| Diagnostics Ping/Traceroute | ✅ 1 | e2e S14 (dua vendor, event 8, hasil terstruktur) |
| Redaman optik berbasis mapping + deteksi anomali | 🟡 0,75 | e2e S15 (TR-181 standar); path optik TR-098 per vendor belum terverifikasi |

### B. Aplikasi web & pengalaman pengguna — 95% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| CRUD seluruh modul tersambung ke API | ✅ 1 | api-parity: **156/156 endpoint relevan dipakai UI** (5 sengaja bukan lewat `api.*`: WebSocket, metrik, redirect OIDC, webhook Xendit) |
| Impor/ekspor dengan pratinjau (semua modul relevan) | ✅ 1 | uji browser nyata |
| Dashboard, detail perangkat, riwayat | ✅ 1 | uji browser |
| Realtime WebSocket | ✅ 1 | diverifikasi browser; lintas instance (HA); **kredensial dicek ulang tiap siklus ping** (tes unit) |
| Portal pelanggan (WiFi Saya) + penautan akun↔perangkat | ✅ 1 | e2e S18 (isolasi, validasi, SSID sampai ke CPE) + **tes UI** halaman portal (validasi WiFi, konfirmasi restart, rincian) + label form terhubung ke input; belum pengguna nyata |
| Konsol Super Admin: laporan SaaS (KPI, tren 30 hari, CSV), impersonate, kuota & paket | ✅ 1 | e2e S19, S20; audit UI; token impersonate 30 menit, aksi sensitif diblokir, jejak `impersonated_by` |
| Landing page publik SEO | ✅ 1 | prerender + JSON-LD (WebSite, SoftwareApplication, HowTo, FAQ), manifest, OG image, robots/sitemap, 404 sungguhan; desain Google/Material, teks ringkas |
| Notifikasi in-app + email + preferensi | ✅ 1 | e2e S22, tes unit/integrasi/komponen; WebSocket realtime; **tampilan email belum dicek di klien email nyata** |
| Validasi form (min/max/pattern) & pesan informatif | 🟢 0,95 | gerbang vitest: teks ber-`maxLength`, angka ber-`min`/`max`, password baru ber-`minLength`, **field wajib ber-`minLength`/`pattern`**; server memvalidasi ulang semuanya (QueryGuard, validator bersama Create=Update). Sisa: aturan semantik tiap form lama belum seragam penuh |
| State galat/loading/empty | ✅ 1 | **banner galat inline global** (tetap tampil sampai data dimuat, tombol "Coba lagi") + toast + keadaan kosong; dites komponen |
| Aksesibilitas (label, nama tombol, kontras, fokus) | 🟢 0,9 | `audit:ui`: **0 pelanggaran WCAG A/AA pada 136 tampilan** termasuk dialog; **jebakan fokus** semua dialog (Modal + konfirmasi) dites unit & di browser; navigasi keyboard penuh seluruh halaman belum diuji |
| Mode gelap & mobile terverifikasi visual | ✅ 1 | halaman × terang/gelap × desktop/mobile + modal; Chrome saja |
| i18n ID/EN | 🟡 0,5 | sebagian halaman; string baru sebagian hanya Indonesia |
| Langganan & tagihan (UI `/billing`) | 🟢 0,85 | tes UI (checkout hanya `plan_code`, tautan bayar wajib https, sync, RBAC, form paket) + audit UI; **belum dicoba dengan Xendit nyata** |
| Pengelola menu (sub menu + drag & drop), laporan bug (DB + email), riwayat impor/ekspor, tabel header tetap + paginasi | ✅ 1 | e2e **S26** (tanpa IDOR lintas tenant) + unit/integrasi MariaDB + tes UI + audit UI; email bug/riwayat teruji dengan pengirim palsu dan satu kali kirim nyata (pengantaran ke inbox tidak bisa saya verifikasi) |

### C. Multi-tenant & identitas/akses — 95% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Isolasi data antar tenant | ✅ 1 | e2e S6 + audit IDOR; jumlah sesi terbuka kini per tenant; berlaku lintas instance untuk WebSocket |
| RBAC 5 peran | ✅ 1 | diaudit; e2e S10, S16 |
| Login email, Argon2id, lockout | ✅ 1 | unit + integrasi MariaDB |
| Lupa/reset password lewat email, notifikasi email | ✅ 1 | S12 + SMTP nyata |
| Pencabutan sesi (ganti password/role, nonaktif, reset MFA, WS) | ✅ 1 | diverifikasi API + e2e + tes WS |
| API token per tenant | ✅ 1 | tak bisa beranak; mati bila user nonaktif; tak bisa kelola MFA. *Tidak otomatis dicabut saat password diganti (disengaja untuk integrasi)* |
| Audit trail lengkap | ✅ 1 | LOGIN_FAILED, ACCOUNT_LOCKED, token, MFA_*, IMPERSONATE_*, IP klien; ADMIN tenant tak melihat baris NULL |
| White-label/branding, kuota task & batas perangkat per tenant | ✅ 1 | e2e S20 (503 untuk Inform baru, 429 pra-registrasi) |
| SSO OIDC / Masuk dengan Google | 🟡 0,75 | alur penuh teruji dengan IdP tiruan RS256; tombol resmi Google, pembatasan domain, **ikatan ke `sub` IdP** (migrasi 0031); **belum diuji dengan Google nyata**; tanpa auto-provisioning (disengaja) |
| MFA TOTP | ✅ 1 | unit (RFC 6238), integrasi MariaDB, e2e S16, tes komponen |
| Pendaftaran tenant mandiri (self-signup, verifikasi email) | 🟡 0,75 | **bawaan MATI** (`ACS_SELF_SIGNUP`, butuh SMTP); unit + integrasi MariaDB (tenant dibuat atomik, token sekali pakai), e2e S20 hanya jalur "mati → 403"; jalur aktif end-to-end dengan email nyata belum diuji |

### D. Keamanan — 85% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Otorisasi & isolasi (OWASP A01) | ✅ 1 | audit + perbaikan; impersonate dibatasi SUPERADMIN, aksi sensitif diblokir |
| Autentikasi & manajemen sesi (A07) | ✅ 1 | JWT ber-`purpose`, MFA, lockout bersama, WS dicek ulang, pesan seragam |
| Validasi input & SQL aman (A03) | 🟢 0,95 | parameter terikat; **audit validasi read-only + perbaikan**: query-string rusak → 400, batas jumlah/ukuran parameter task/preset/Inform, isi Inform CPE divalidasi, unggahan divalidasi sebelum streaming, kebijakan password satu pintu; e2e S24. **2026-10-19: strict-decode JSON** (`StrictJSONSerializer`, `restEcho` — field asing & sampah di belakang nilai JSON ditolak 400 alih-alih diam-diam diabaikan; CWMP/USP tidak tersentuh, pakai XML/SOAP), diverifikasi e2e 27/27 + `-race`. Sisa: kuota storage, transfer kepemilikan pra-registrasi (T4) |
| Kredensial terenkripsi, rahasia disamarkan | 🟡 0,9 | tenant/CR/TOTP/nilai Preset & Profil AES-GCM; respons task disamarkan; nilai yang *dilaporkan CPE* (`device_parameters`) masih plaintext di DB (disamarkan di API) |
| Rate limit & anti brute-force | ✅ 1 | per-IP (REST `ACS_REST_RATE_LIMIT` bawaan 100/dtk, CWMP) + pembatas ketat login/MFA/reset/signup + lockout + terdistribusi (Redis) |
| TLS (REST & CWMP) | 🟡 0,75 | overlay Caddy diuji fungsional (sertifikat internal); Let's Encrypt & CPE nyata belum |
| SSRF guard (CR, webhook) | 🟡 0,9 | penjaga **saat sambungan** + `ACS_WEBHOOK_BLOCK_PRIVATE` untuk SaaS publik; bawaan tetap mengizinkan LAN |
| Header keamanan, CSP, CORS, batas body | ✅ 1 | CSP `script-src 'self'` diuji browser; metrik tak terbuka publik |
| Komponen rentan & rantai pasok (A06) | ✅ 1 | `govulncheck` 0 reachable, `npm audit` 0 (per 16–17 Okt) |
| Penetration test independen | ❌ 0 | belum (tidak dapat dilakukan oleh pembuat kode) |

### E. Operasional & deployment — 87% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Docker Compose + migrasi otomatis (29) | ✅ 1 | diverifikasi (fresh DB + up/down/up; 0029 juga diuji `down` terpisah & `up` berulang) |
| Backup & restore terdokumentasi/teruji | ✅ 1 | drill restore |
| Metrik Prometheus, alert, dashboard | ✅ 1 | ada |
| Retensi log otomatis | ✅ 1 | uji MariaDB |
| CI (gofmt, vet, build, test -race, lint, build FE, npm test) | ✅ 1 | ci.yml |
| Health/readiness endpoint | ✅ 1 | `/healthz`, `/readyz` (DB mati → 503, pulih sendiri); healthcheck compose |
| Multi-instance/HA | 🟡 0,9 | `ACS_HA`: fan-out WS, rate limit, kunci leader sweeper; diuji 2 `acsd` nyata; belum uji beban multi-instance/chaos |
| Reverse proxy TLS siap pakai di repo | 🟡 0,9 | `docker-compose.tls.yml` + Caddyfile, diuji fungsional |
| Skala: load test kapasitas | 🟡 0,8 | Diukur ulang 2026-10-18 (`cmd/loadtest`, satu instance, laptop): p95 **memenuhi target <300ms** sampai concurrency ≈100 (p95 231ms @100); di atas itu p95 melebihi target karena antrean, **bukan error** (0 gagal sampai concurrency 1000). Throughput **plateau ~450–480 sesi/dtk** berapa pun concurrency-nya — akar penyebab: `db.SetMaxOpenConns(50)`, bukan CPU (acsd 36%, MariaDB 54% saat idle pasca-tes). **2026-10-19: dijadikan configurable** (`ACS_DB_MAX_OPEN_CONNS`/`_MAX_IDLE_CONNS`/`_CONN_MAX_LIFETIME_MINUTES`, bawaan tetap 50/10/30 — lihat RUNBOOK §J); belum diukur ulang dengan nilai dinaikkan untuk konfirmasi throughput naik. Minimal Inform saja, satu instance; belum multi-instance/HA, belum profil RPC penuh |
| Orkestrasi (K8s/helm), autoscaling | ❌ 0 | belum; probe `/healthz` `/readyz` sudah siap dipakai |
| Runbook insiden & panduan operasional | ✅ 1 | RUNBOOK §F–L (+ variabel lingkungan baru) + buku panduan |

### F. Kualitas & verifikasi — 82% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Unit test backend + race | ✅ 1 | seluruh paket lulus `go test -race` (Docker, termasuk integrasi MariaDB/Redis) |
| Integrasi MariaDB & Redis nyata | ✅ 1 | `ACS_TEST_DSN`, `ACS_TEST_REDIS` (+ seed 0029) |
| E2E 26 skenario (simulator) | ✅ 1 | 26/26 (S25 UDP CR + RPC CPE, S26 fitur Console & IDOR) |
| Uji CPE fisik (≥1 per vendor) | ❌ 0 | BELUM — UAT Rabu |
| Uji frontend otomatis | 🟢 0,85 | Vitest 76 tes (menu, dialog, banner galat, portal, Billing, pengelola menu, gerbang batas input) + audit axe 136 tampilan; belum alur UI end-to-end terhadap backend di CI |
| Dokumentasi API OpenAPI + Postman | ✅ 1 | 172 operasi, redocly lint valid (8 peringatan lama, by design); spec sudah 1:1 dgn `router.go` (diaudit dua arah 2026-10-19, 0 drift path/method/payload). **Koleksi Postman diregenerasi** 2026-10-19 (sempat stale di 147 request sejak sebelum endpoint menu-layout/bug-reports/data-transfers ditambahkan) → kini 172 request, `_postman_id` dipertahankan |
| Audit keamanan/kualitas | ✅ 1 | beberapa audit review + perbaikan (bukan pentest manusia independen) |

### G. Aspek SaaS komersial — 67% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Onboarding tenant (wizard) & panduan pengguna | ✅ 1 | ada + buku panduan |
| Landing page & materi pemasaran dasar | ✅ 1 | ada |
| Paket/plan, penagihan (Xendit), batas pemakaian | 🟡 0,75 | batas perangkat & kuota task per tenant ditegakkan; **checkout Invoice Xendit, webhook idempoten (token + verifikasi ulang), perpanjangan masa aktif, notifikasi/email, tanpa pemutusan otomatis** — bawaan MATI, diuji dengan server tiruan + MariaDB nyata, **bukan Xendit nyata**; harga paket contoh placeholder; tanpa metering berbasis pemakaian/pajak |
| Halaman status / SLA / dukungan (helpdesk) | ❌ 0 | belum (`/readyz` bisa dipakai uptime checker eksternal) |
| Syarat layanan, kebijakan privasi, DPA | 🔴 0,25 | halaman draf ada, **bertanda belum ditinjau hukum**; DPA tidak ada |
| Notifikasi email transaksional | ✅ 1 | ada |

## Prioritas berikutnya (nilai ÷ usaha)

1. **UAT Rabu dengan CPE fisik** — membuka −12 poin; hasilnya menentukan perbaikan katalog/mapping/path optik (termasuk vendor baru).
1b. **Uji billing di mode test Xendit** (buat invoice, bayar lewat simulator, pastikan callback + "Periksa status" memperpanjang masa aktif) sebelum `ACS_BILLING_ENABLED=true` di produksi.
2. **SSO dengan IdP nyata** (+ ikat akun lewat `sub`) dan uji jalur aktif self-signup dengan SMTP nyata (C).
3. **Pentest independen** sebelum membuka ke tenant yang tidak dikenal (D).
4. **Load test dengan profil Inform realistis** dan multi-instance (E).
5. Keputusan bisnis: harga/metering/penagihan, ToS/privasi/DPA final, status page (G) — hanya bila akan dijual sebagai layanan berbayar.
6. Manifest Kubernetes/helm bila target deployment-nya K8s (probe sudah siap).
