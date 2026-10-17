# CHECKLIST KEMATANGAN SaaS — ACS Console

Tanggal: 17 Oktober 2026 (dihitung ulang setelah LANJUTAN 10) · Dasar: kode di branch `dev`, e2e 21/21, `go test -race` paket yang disentuh + integrasi MariaDB/Redis, audit UI otomatis (88 tampilan), 25 tes frontend, uji beban lab, uji dua instance.

**Rubrik:** 1 = ada & teruji otomatis (unit/integrasi/e2e/uji browser) · 0,75 = ada & teruji sebagian (mis. unit/simulator tanpa skenario e2e, atau belum terhadap sistem nyata) · 0,5 = parsial · 0,25 = kerangka · 0 = belum ada. Skor kategori = rata-rata item × bobot. Rubrik ini **dibuat sendiri** dan mengukur *kelengkapan & verifikasi di lingkungan uji*, bukan bukti perilaku di perangkat/pengguna nyata.

## Ringkasan

| Kategori | Bobot | 16 Okt | Sekarang | Poin |
|---|---:|---:|---:|---:|
| A. Protokol & inti ACS (TR-069) | 25 | 92% | **95%** | 23.8 |
| B. Aplikasi web & pengalaman pengguna | 15 | 82% | **88%** | 13.1 |
| C. Multi-tenant & identitas/akses | 15 | 89% | **95%** | 14.3 |
| D. Keamanan | 10 | 79% | **83%** | 8.3 |
| E. Operasional & deployment | 15 | 87% | **87%** | 13.0 |
| F. Kualitas & verifikasi | 10 | 82% | **82%** | 8.2 |
| G. Aspek SaaS komersial | 10 | 50% | **63%** | 6.3 |
| **TOTAL** | **100** | 82.6 | | **87.0** |

## Dua angka yang perlu dibedakan

- **Kelengkapan fitur & verifikasi lab: ≈ 87%** (tabel di atas).
- **Kesiapan go-live produksi berbayar: ≈ 75%** = 87% dikurangi 12 poin diskon ketidakpastian lapangan: seluruh skor A/B memakai simulator CPE; belum ada satu pun Inform dari CPE fisik, sehingga item bernilai 1 di A belum terbukti pada firmware nyata (risiko interoperabilitas per vendor). Potongan ini tidak menghitung ganda item yang sudah bernilai 0.

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

### A. Protokol & inti ACS (TR-069) — 95% (bobot 25)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Sesi CWMP: Inform, RPC loop, cookie, namespace | ✅ 1 | e2e S1–S9; simulator 5 profil; **sesi dibuka di instance A, selesai di B** (state di DB) |
| RPC Get/Set/GetNames/Add/Delete/Reboot/FactoryReset/Download | ✅ 1 | e2e + unit; Set bertipe sesuai laporan CPE |
| TransferComplete + TransferCompleteResponse | ✅ 1 | handler + simulator + e2e S8 |
| Connection Request HTTP Digest/Basic + anti-SSRF | ✅ 1 | e2e S7, S10; penjaga SSRF saat sambungan (DNS rebinding) |
| Connection Request UDP (Annex G, HMAC-SHA1) | 🟡 0,75 | TR-098 & TR-181; teruji terhadap penerima uji; **belum CPE nyata** |
| Data model TR-098 & TR-181 via katalog/mapping, **semua ONT** | ✅ 1 | 33 vendor (5 baseline + 27 tambahan + `GENERIC`), 4.304 OUI IEEE, 372 mapping, `value_scale`; **fallback standar** untuk vendor tak dikenal/NULL (e2e S21, unit, integrasi MariaDB). Catatan: data awal, belum diuji perangkat nyata |
| Provisioning Profile + Zero-Touch Rule | ✅ 1 | e2e S2, S13 |
| Preset penegak konfigurasi (drift-heal) | ✅ 1 | e2e S9 |
| Firmware: upload, canary rollout, auto-pause | ✅ 1 | e2e S8 |
| Antrean task: retry, backoff, fault 9005/9003, validasi | ✅ 1 | unit + e2e S5 |
| Penemuan pohon parameter / refresh rekursif | ✅ 1 | e2e S17 (14 GetParameterNames hingga daun); menemukan & memperbaiki bug nyata (bentuk respons) |
| SetParameterAttributes / notifikasi (event 4) | ✅ 1 | e2e S17 (atribut diterima CPE, event 4 tercatat). *Virtual parameters/skrip belum ada* |
| RPC dari CPE (GetRPCMethods dijawab; lainnya Fault 8000 sesuai spek) | 🟡 0,75 | `cpe_initiated_test`; belum CPE nyata |
| Diagnostics Ping/Traceroute | ✅ 1 | e2e S14 (dua vendor, event 8, hasil terstruktur) |
| Redaman optik berbasis mapping + deteksi anomali | 🟡 0,75 | e2e S15 (TR-181 standar); path optik TR-098 per vendor belum terverifikasi |

### B. Aplikasi web & pengalaman pengguna — 88% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| CRUD seluruh modul tersambung ke API | ✅ 1 | api-parity: 139 endpoint relevan, 4 sengaja tanpa UI |
| Impor/ekspor dengan pratinjau (semua modul relevan) | ✅ 1 | uji browser nyata |
| Dashboard, detail perangkat, peta, riwayat | ✅ 1 | uji browser |
| Realtime WebSocket | ✅ 1 | diverifikasi browser; lintas instance (HA); **kredensial dicek ulang tiap siklus ping** (tes unit) |
| Portal pelanggan (WiFi Saya) + penautan akun↔perangkat | 🟡 0,75 | e2e S18 (isolasi, validasi, SSID sampai ke CPE), tes komponen penautan; belum pengguna nyata, halaman portal tanpa tes UI sendiri |
| Konsol Super Admin: laporan SaaS (KPI, tren 30 hari, CSV), impersonate, kuota & paket | ✅ 1 | e2e S19, S20; audit UI; token impersonate 30 menit, aksi sensitif diblokir, jejak `impersonated_by` |
| Landing page publik SEO | ✅ 1 | prerender + JSON-LD |
| Validasi form (min/max/pattern) & pesan informatif | 🟡 0,75 | **gerbang vitest: 147 input semuanya ber-`maxLength`/`max`**; backend memetakan galat kolom ke 400; min/pattern belum seragam di semua form |
| State galat/loading/empty | 🟡 0,75 | galat pemuatan = toast global; bug toast palsu superadmin diperbaiki + audit UI kini gagal bila ada toast galat; belum ada keadaan galat inline per halaman |
| Aksesibilitas (label, nama tombol, kontras) | 🟡 0,75 | `npm run audit:ui`: **0 pelanggaran WCAG A/AA** pada 88 tampilan; jebakan fokus modal & navigasi keyboard penuh belum diuji |
| Mode gelap & mobile terverifikasi visual | ✅ 1 | halaman × terang/gelap × desktop/mobile + modal; Chrome saja |
| i18n ID/EN | 🟡 0,5 | sebagian halaman; string baru sebagian hanya Indonesia |

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
| SSO OIDC | 🟡 0,75 | alur penuh teruji dengan IdP tiruan RS256; **belum IdP nyata**; pemetaan lewat email, belum `sub`; tanpa auto-provisioning (disengaja) |
| MFA TOTP | ✅ 1 | unit (RFC 6238), integrasi MariaDB, e2e S16, tes komponen |
| Pendaftaran tenant mandiri (self-signup, verifikasi email) | 🟡 0,75 | **bawaan MATI** (`ACS_SELF_SIGNUP`, butuh SMTP); unit + integrasi MariaDB (tenant dibuat atomik, token sekali pakai), e2e S20 hanya jalur "mati → 403"; jalur aktif end-to-end dengan email nyata belum diuji |

### D. Keamanan — 83% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Otorisasi & isolasi (OWASP A01) | ✅ 1 | audit + perbaikan; impersonate dibatasi SUPERADMIN, aksi sensitif diblokir |
| Autentikasi & manajemen sesi (A07) | ✅ 1 | JWT ber-`purpose`, MFA, lockout bersama, WS dicek ulang, pesan seragam |
| Validasi input & SQL aman (A03) | 🟡 0,75 | parameter terikat; validasi mayoritas; penetralan formula CSV; batas input FE terpusat |
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
| Skala: load test kapasitas | 🟡 0,75 | ~550–620 sesi/dtk, p95 99 ms @50; minimal Inform, satu instance, laptop |
| Orkestrasi (K8s/helm), autoscaling | ❌ 0 | belum; probe `/healthz` `/readyz` sudah siap dipakai |
| Runbook insiden & panduan operasional | ✅ 1 | RUNBOOK §F–L (+ variabel lingkungan baru) + buku panduan |

### F. Kualitas & verifikasi — 82% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Unit test backend + race | ✅ 1 | 32 paket lulus; race via Docker pada paket yang disentuh |
| Integrasi MariaDB & Redis nyata | ✅ 1 | `ACS_TEST_DSN`, `ACS_TEST_REDIS` (+ seed 0029) |
| E2E 21 skenario (simulator) | ✅ 1 | 21/21 |
| Uji CPE fisik (≥1 per vendor) | ❌ 0 | BELUM — UAT Rabu |
| Uji frontend otomatis | 🟡 0,75 | Vitest 25 tes (+ gerbang batas input) + audit axe 88 tampilan; belum alur UI end-to-end terhadap backend di CI |
| Dokumentasi API OpenAPI + Postman | ✅ 1 | 147 operasi, redocly lint valid (8 peringatan lama) |
| Audit keamanan/kualitas | ✅ 1 | beberapa audit review + perbaikan (bukan pentest manusia independen) |

### G. Aspek SaaS komersial — 63% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Onboarding tenant (wizard) & panduan pengguna | ✅ 1 | ada + buku panduan |
| Landing page & materi pemasaran dasar | ✅ 1 | ada |
| Paket/plan, metering penggunaan, penagihan | 🟡 0,5 | batas perangkat & kuota task per tenant ditegakkan + laporan penggunaan per tenant; **tanpa harga, metering berbayar, atau penagihan** |
| Halaman status / SLA / dukungan (helpdesk) | ❌ 0 | belum (`/readyz` bisa dipakai uptime checker eksternal) |
| Syarat layanan, kebijakan privasi, DPA | 🔴 0,25 | halaman draf ada, **bertanda belum ditinjau hukum**; DPA tidak ada |
| Notifikasi email transaksional | ✅ 1 | ada |

## Prioritas berikutnya (nilai ÷ usaha)

1. **UAT Rabu dengan CPE fisik** — membuka −12 poin; hasilnya menentukan perbaikan katalog/mapping/path optik (termasuk vendor baru).
2. **SSO dengan IdP nyata** (+ ikat akun lewat `sub`) dan uji jalur aktif self-signup dengan SMTP nyata (C).
3. **Pentest independen** sebelum membuka ke tenant yang tidak dikenal (D).
4. **Load test dengan profil Inform realistis** dan multi-instance (E).
5. Keputusan bisnis: harga/metering/penagihan, ToS/privasi/DPA final, status page (G) — hanya bila akan dijual sebagai layanan berbayar.
6. Manifest Kubernetes/helm bila target deployment-nya K8s (probe sudah siap).
