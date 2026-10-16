# CHECKLIST KEMATANGAN SaaS — ACS Console

Tanggal: 16 Oktober 2026 (dihitung ulang setelah LANJUTAN 9) · Dasar: kode di branch `dev`, e2e 16/16, `go test -race` seluruh paket + integrasi MariaDB/Redis, audit UI otomatis (68 tampilan), uji beban lab, uji dua instance.

**Rubrik:** 1 = ada & teruji otomatis (unit/integrasi/e2e/uji browser) · 0,75 = ada & teruji sebagian (mis. unit/simulator tanpa skenario e2e, atau belum terhadap sistem nyata) · 0,5 = parsial · 0,25 = kerangka · 0 = belum ada. Skor kategori = rata-rata item × bobot. Rubrik ini **dibuat sendiri** dan mengukur *kelengkapan & verifikasi di lingkungan uji*, bukan bukti perilaku di perangkat/pengguna nyata.

## Ringkasan

| Kategori | Bobot | Sebelum | Sekarang | Poin |
|---|---:|---:|---:|---:|
| A. Protokol & inti ACS (TR-069) | 25 | 73% | **92%** | 22.9 |
| B. Aplikasi web & pengalaman pengguna | 15 | 73% | **82%** | 12.3 |
| C. Multi-tenant & identitas/akses | 15 | 70% | **89%** | 13.3 |
| D. Keamanan | 10 | 63% | **79%** | 7.9 |
| E. Operasional & deployment | 15 | 59% | **87%** | 13.0 |
| F. Kualitas & verifikasi | 10 | 75% | **82%** | 8.2 |
| G. Aspek SaaS komersial | 10 | 50% | **50%** | 5.0 |
| **TOTAL** | **100** | 67.4 | | **82.6** |

## Dua angka yang perlu dibedakan

- **Kelengkapan fitur & verifikasi lab: ≈ 83%** (tabel di atas).
- **Kesiapan go-live produksi berbayar: ≈ 70%** = 83% dikurangi 12 poin diskon ketidakpastian lapangan: seluruh skor A/B memakai simulator CPE; belum ada satu pun Inform dari CPE fisik, sehingga item bernilai 1 di A belum terbukti pada firmware nyata (risiko interoperabilitas per vendor). Potongan ini tidak menghitung ganda item yang sudah bernilai 0.

Untuk **pilot/UAT internal** (Rabu 21 Okt) angka relevan adalah kelengkapan fitur; untuk **menjual sebagai SaaS** yang relevan adalah kesiapan go-live.

## Kenapa bukan 100% (dan apa yang dibutuhkan)

Permintaan "100%" untuk Protokol, Multi-tenant, dan Keamanan: semua yang dapat diverifikasi di lab sudah ditutup. Sisanya **tidak bisa saya buktikan sendiri**:

| Sisa | Kategori | Kenapa tertahan | Yang dibutuhkan |
|---|---|---|---|
| Perilaku pada firmware ONT nyata (Inform, Set bertipe, atribut, diagnostics, optik, UDP CR) | A/B (diskon go-live 12 poin; item A tersisa 0,75) | simulator ≠ firmware vendor; hanya UAT yang membuktikan | **UAT Rabu** + pembaruan katalog dari hasilnya |
| SSO terhadap IdP organisasi sebenarnya | C | hanya IdP tiruan yang tersedia | satu IdP uji (Keycloak/Azure AD/Google) |
| Pendaftaran tenant mandiri (self-signup) | C | keputusan model bisnis (siapa boleh mendaftar, verifikasi, anti-penyalahgunaan) | keputusan Anda; teknisnya ≈ 1–2 hari |
| Penetration test independen | D | tidak bisa dinilai oleh pembuat kode | pihak ketiga |
| Validasi form seragam, i18n penuh | B | pekerjaan luas, bernilai sedang | jadwal |

## Rincian per item

### A. Protokol & inti ACS (TR-069) — 92% (bobot 25)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Sesi CWMP: Inform, RPC loop, cookie, namespace | ✅ 1 | e2e S1–S9; simulator 5 vendor; **sesi dibuka di instance A, selesai di B** (state di DB) |
| RPC Get/Set/GetNames/Add/Delete/Reboot/FactoryReset/Download | ✅ 1 | e2e + unit; Set bertipe sesuai laporan CPE |
| TransferComplete + TransferCompleteResponse | ✅ 1 | handler + simulator + e2e S8 |
| Connection Request HTTP Digest/Basic + anti-SSRF | ✅ 1 | e2e S7, S10; penjaga SSRF saat sambungan (DNS rebinding) |
| Connection Request UDP (Annex G, HMAC-SHA1) | 🟡 0,75 | TR-098 & TR-181; teruji terhadap penerima uji; **belum CPE nyata** |
| Data model TR-098 & TR-181 via katalog/mapping | ✅ 1 | 180+ mapping, 3.176 OUI, `value_scale` |
| Provisioning Profile + Zero-Touch Rule | ✅ 1 | e2e S2, S13 |
| Preset penegak konfigurasi (drift-heal) | ✅ 1 | e2e S9 |
| Firmware: upload, canary rollout, auto-pause | ✅ 1 | e2e S8 |
| Antrean task: retry, backoff, fault 9005/9003, validasi | ✅ 1 | unit + e2e S5 |
| Penemuan pohon parameter / refresh rekursif | 🟡 0,75 | unit + verifikasi API; tanpa skenario e2e |
| SetParameterAttributes / notifikasi (event 4) | 🟡 0,75 | unit + simulator + UI; tanpa skenario e2e; virtual parameters tidak ada |
| RPC dari CPE (GetRPCMethods dijawab; lainnya Fault 8000 sesuai spek) | 🟡 0,75 | `cpe_initiated_test`; belum CPE nyata |
| Diagnostics Ping/Traceroute | ✅ 1 | e2e S14 (dua vendor, event 8, hasil terstruktur) |
| Redaman optik berbasis mapping + deteksi anomali | 🟡 0,75 | e2e S15 (TR-181); path optik TR-098 per vendor belum terverifikasi |

### B. Aplikasi web & pengalaman pengguna — 82% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| CRUD seluruh modul tersambung ke API | ✅ 1 | api-parity: 1 dari 121 endpoint (get-parameter-attributes) tanpa UI |
| Impor/ekspor dengan pratinjau (semua modul relevan) | ✅ 1 | uji browser nyata |
| Dashboard, detail perangkat, peta, riwayat | ✅ 1 | uji browser |
| Realtime WebSocket | ✅ 1 | diverifikasi browser; lintas instance (HA) |
| Portal pelanggan (WiFi Saya) | 🟡 0,5 | ada; alur ENDUSER belum teruji end-to-end |
| Landing page publik SEO | ✅ 1 | prerender + JSON-LD |
| Validasi form (min/max/pattern) & pesan informatif | 🟡 0,5 | belum seragam di semua form |
| State galat/loading/empty | 🟡 0,75 | galat pemuatan kini toast global (diuji browser); belum ada keadaan galat inline per halaman |
| Aksesibilitas (label, nama tombol, kontras) | 🟡 0,75 | `npm run audit:ui`: **0 pelanggaran WCAG A/AA** pada 68 tampilan; jebakan fokus modal & navigasi keyboard penuh belum diuji |
| Mode gelap & mobile terverifikasi visual | ✅ 1 | 12 halaman × terang/gelap × desktop/mobile + modal; bug nyata ditemukan & diperbaiki; Chrome saja (Safari/Firefox/perangkat fisik belum) |
| i18n ID/EN | 🟡 0,5 | sebagian halaman; string baru (MFA, galat) sebagian hanya Indonesia |

### C. Multi-tenant & identitas/akses — 89% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Isolasi data antar tenant | ✅ 1 | e2e S6 + audit IDOR; berlaku lintas instance untuk WebSocket |
| RBAC 5 peran | ✅ 1 | diaudit; e2e S10, S16 |
| Login email, Argon2id, lockout | ✅ 1 | unit + integrasi MariaDB |
| Lupa/reset password lewat email, notifikasi email | ✅ 1 | S12 + SMTP nyata |
| Pencabutan sesi (ganti password/role, nonaktif, reset MFA) | ✅ 1 | diverifikasi API + e2e |
| API token per tenant | ✅ 1 | tak bisa beranak; mati bila user nonaktif; tak bisa kelola MFA |
| Audit trail lengkap | ✅ 1 | LOGIN_FAILED, ACCOUNT_LOCKED, token, MFA_*, IP klien; ADMIN tenant tak melihat baris NULL |
| White-label/branding & kuota per tenant | ✅ 1 | ada |
| SSO OIDC | 🟡 0,75 | alur penuh teruji dengan IdP tiruan RS256 (nonce, aud, exp, email_verified, pemetaan lewat email); **belum IdP nyata**; tanpa auto-provisioning (disengaja) |
| MFA TOTP | ✅ 1 | unit (RFC 6238), integrasi MariaDB (CAS paralel), e2e S16, tes komponen; anti-replay, kode pemulihan, reset admin |
| Pendaftaran tenant mandiri (self-signup) | ❌ 0 | keputusan bisnis; tenant dibuat superadmin |

### D. Keamanan — 79% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Otorisasi & isolasi (OWASP A01) | ✅ 1 | audit + perbaikan |
| Validasi input & SQL aman (A03) | 🟡 0,75 | parameter terikat; validasi mayoritas |
| Kredensial terenkripsi, rahasia disamarkan | 🟡 0,9 | tenant/CR/TOTP/nilai Preset & Profil AES-GCM; nilai yang *dilaporkan CPE* (`device_parameters`) masih plaintext di DB (disamarkan di API) |
| Rate limit & anti brute-force | ✅ 1 | per-IP + lockout akun (password & MFA) + **terdistribusi** (Redis) + trusted proxy |
| TLS (REST & CWMP) | 🟡 0,75 | overlay Caddy diuji fungsional (sertifikat internal); penerbitan Let's Encrypt & CPE nyata belum |
| SSRF guard (CR, webhook) | 🟡 0,9 | penjaga **saat sambungan** (DNS rebinding/redirect) + `ACS_WEBHOOK_BLOCK_PRIVATE` untuk SaaS publik; bawaan tetap mengizinkan LAN |
| Header keamanan, CORS, batas body | ✅ 1 | ada |
| Penetration test independen | ❌ 0 | belum (tidak dapat dilakukan oleh pembuat kode) |

### E. Operasional & deployment — 87% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Docker Compose + migrasi otomatis (27) | ✅ 1 | diverifikasi (fresh DB + up/down/up) |
| Backup & restore terdokumentasi/teruji | ✅ 1 | drill restore |
| Metrik Prometheus, alert, dashboard | ✅ 1 | ada |
| Retensi log otomatis | ✅ 1 | uji MariaDB |
| CI (gofmt, vet, build, test -race, lint, build FE, npm test) | ✅ 1 | ci.yml |
| Health/readiness endpoint | ✅ 1 | `/healthz`, `/readyz` (DB mati → 503, pulih sendiri); healthcheck compose |
| Multi-instance/HA | 🟡 0,9 | `ACS_HA`: fan-out WS, rate limit, kunci leader sweeper; diuji 2 `acsd` nyata + sesi CWMP lintas instance; belum uji beban multi-instance/chaos |
| Reverse proxy TLS siap pakai di repo | 🟡 0,9 | `docker-compose.tls.yml` + Caddyfile, diuji fungsional |
| Skala: load test kapasitas | 🟡 0,75 | ~550–620 sesi/dtk, p95 99 ms @50; minimal Inform, satu instance, laptop; belum Inform realistis/CPE fisik |
| Orkestrasi (K8s/helm), autoscaling | ❌ 0 | belum; probe `/healthz` `/readyz` sudah siap dipakai |
| Runbook insiden & panduan operasional | ✅ 1 | RUNBOOK §F–L + buku panduan |

### F. Kualitas & verifikasi — 82% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Unit test backend + race | ✅ 1 | seluruh paket |
| Integrasi MariaDB & Redis nyata | ✅ 1 | `ACS_TEST_DSN`, `ACS_TEST_REDIS` |
| E2E 16 skenario (simulator) | ✅ 1 | 16/16 |
| Uji CPE fisik (≥1 per vendor) | ❌ 0 | BELUM — UAT Rabu |
| Uji frontend otomatis | 🟡 0,75 | Vitest 14 tes + audit axe 68 tampilan; belum alur UI end-to-end terhadap backend di CI |
| Dokumentasi API OpenAPI + Postman | ✅ 1 | 137 request, redocly lint valid |
| Audit keamanan/kualitas | ✅ 1 | beberapa audit review + perbaikan (bukan pentest manusia independen) |

### G. Aspek SaaS komersial — 50% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Onboarding tenant (wizard) & panduan pengguna | ✅ 1 | ada + buku panduan |
| Landing page & materi pemasaran dasar | ✅ 1 | ada |
| Paket/plan, metering penggunaan, penagihan | ❌ 0 | hanya kuota task; butuh keputusan harga/paket |
| Halaman status / SLA / dukungan (helpdesk) | ❌ 0 | belum (`/readyz` bisa dipakai uptime checker eksternal) |
| Syarat layanan, kebijakan privasi, DPA | ❌ 0 | butuh badan hukum & tinjauan penasihat hukum; tidak saya buatkan draf yang tampak final |
| Notifikasi email transaksional | ✅ 1 | ada |

## Prioritas berikutnya (nilai ÷ usaha)

1. **UAT Rabu dengan CPE fisik** — membuka −12 poin; hasilnya menentukan perbaikan katalog/mapping/path optik.
2. **SSO dengan IdP nyata + keputusan self-signup** (C).
3. **Pentest independen** sebelum membuka ke tenant yang tidak dikenal (D).
4. **Load test dengan profil Inform realistis** dan multi-instance (E).
5. Keputusan bisnis: plan/metering/penagihan, ToS/privasi/DPA, status page (G) — hanya bila akan dijual sebagai layanan berbayar.
6. Manifest Kubernetes/helm bila target deployment-nya K8s (probe sudah siap).
