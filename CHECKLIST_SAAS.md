# CHECKLIST KEMATANGAN SaaS — ACS Console

Tanggal: 16 Oktober 2026 · Dasar: kode di branch `dev`, hasil audit, e2e 13/13, uji browser.

**Rubrik:** 1 = ada & teruji otomatis (unit/integrasi/e2e/uji browser) · 0,5 = parsial atau belum terverifikasi penuh · 0 = belum ada. Skor kategori = rata-rata item × bobot. Ini ukuran **kelengkapan & verifikasi di lingkungan uji**, bukan bukti perilaku di perangkat nyata.

## Ringkasan

| Kategori | Bobot | Kelengkapan | Poin |
|---|---:|---:|---:|
| A. Protokol & inti ACS (TR-069) | 25 | 73% | 18.3 |
| B. Aplikasi web & pengalaman pengguna | 15 | 73% | 10.9 |
| C. Multi-tenant & identitas/akses | 15 | 70% | 10.6 |
| D. Keamanan | 10 | 63% | 6.3 |
| E. Operasional & deployment | 15 | 59% | 8.9 |
| F. Kualitas & verifikasi | 10 | 75% | 7.5 |
| G. Aspek SaaS komersial | 10 | 50% | 5.0 |
| **TOTAL** | **100** | | **67.4** |

## Dua angka yang perlu dibedakan

- **Kelengkapan fitur & verifikasi lab: 67%** (tabel di atas).
- **Kesiapan go-live produksi berbayar: ≈ 55%** = 67% dikurangi 12 poin diskon ketidakpastian lapangan:
  - −12: Seluruh skor kategori A/B memakai simulator CPE: belum ada satu pun Inform dari CPE fisik, sehingga item bernilai 1 di A belum terbukti pada firmware nyata (risiko interoperabilitas per vendor). Potongan ini TIDAK menghitung ganda item yang sudah bernilai 0 (load test, TLS, billing, dst.)

Untuk **pilot/UAT internal** (tujuan Rabu 21 Okt) angka relevan adalah kelengkapan fitur; untuk **menjual sebagai SaaS** yang relevan adalah kesiapan go-live.

## Rincian per item

### A. Protokol & inti ACS (TR-069) — 73% (bobot 25)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Sesi CWMP: Inform, RPC loop, cookie, namespace | ✅ 1 | e2e S1–S9; simulator 5 vendor |
| RPC Get/Set/GetNames/Add/Delete/Reboot/FactoryReset/Download | ✅ 1 | e2e + unit; Set bertipe sesuai laporan CPE |
| TransferComplete + TransferCompleteResponse | ✅ 1 | baru; handler + simulator + tes |
| Connection Request HTTP Digest/Basic + anti-SSRF | ✅ 1 | e2e S7, S10 |
| Connection Request UDP/STUN (CGNAT) | 🟡 0,5 | kode ada; hanya TR-098; belum teruji |
| Data model TR-098 & TR-181 via katalog/mapping | ✅ 1 | 120+ mapping, 3.176 OUI |
| Provisioning Profile + Zero-Touch Rule | ✅ 1 | e2e S2, S13 |
| Preset penegak konfigurasi (drift-heal) | ✅ 1 | e2e S9; nilai Set dicatat |
| Firmware: upload, canary rollout, auto-pause | ✅ 1 | e2e S8; kompatibilitas vendor/model |
| Antrean task: retry, backoff, fault 9005/9003, validasi | ✅ 1 | unit + e2e S5 |
| Penemuan pohon parameter / refresh cerdas | 🟡 0,5 | auto-discovery level atas saja |
| SetParameterAttributes / notifikasi / virtual parameters | ❌ 0 | tidak ada |
| RPC dari CPE selain TransferComplete (GetRPCMethods, Kicked) | 🟡 0,5 | tidak dibalas |
| Diagnostics (ping/traceroute) | ❌ 0 | API 501 |
| Redaman optik & deteksi anomali | 🟡 0,5 | heuristik nama parameter; belum teruji |

### B. Aplikasi web & pengalaman pengguna — 73% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| CRUD seluruh modul tersambung ke API (118/118) | ✅ 1 | api-parity |
| Impor/ekspor dengan pratinjau (semua modul relevan) | ✅ 1 | uji browser nyata |
| Dashboard, detail perangkat, peta, riwayat | ✅ 1 | uji browser |
| Realtime WebSocket | ✅ 1 | diverifikasi browser |
| Portal pelanggan (WiFi Saya) | 🟡 0,5 | ada; alur ENDUSER belum teruji end-to-end |
| Landing page publik SEO | ✅ 1 | baru; prerender + JSON-LD |
| Validasi form (min/max/pattern) & pesan informatif | 🟡 0,5 | diperketat; belum seragam di semua form |
| State galat/loading/empty di semua halaman | 🟡 0,5 | galat daftar masih tampil "kosong" |
| Aksesibilitas (label, fokus modal, keyboard) | 🟡 0,5 | ~120 label tanpa htmlFor |
| Mode gelap & mobile terverifikasi visual | 🟡 0,5 | belum diverifikasi |
| i18n ID/EN | 🟡 0,5 | hanya sebagian halaman |

### C. Multi-tenant & identitas/akses — 70% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Isolasi data antar tenant | ✅ 1 | e2e S6 + audit IDOR |
| RBAC 5 peran (Superadmin/Admin/NOC/Viewer/EndUser) | ✅ 1 | diaudit; celah B1 ditutup |
| Login email, hash Argon2id, lockout | ✅ 1 | unit + integrasi MariaDB |
| Lupa/reset password lewat email, notifikasi email | ✅ 1 | S12 + SMTP nyata |
| Pencabutan sesi (ganti password/role, nonaktif) | ✅ 1 | diverifikasi API |
| API token per tenant | ✅ 1 | tak bisa beranak; mati bila user nonaktif |
| Audit trail (siapa/kapan/apa) lengkap | 🟡 0,5 | celah: login gagal, token, IP tak tercatat |
| White-label/branding & kuota per tenant | ✅ 1 | ada |
| SSO OIDC | 🟡 0,25 | callback masih POC |
| MFA / 2FA | ❌ 0 | tidak ada |
| Pendaftaran tenant mandiri (self-signup) | ❌ 0 | tenant dibuat superadmin |

### D. Keamanan — 63% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Otorisasi & isolasi (OWASP A01) | ✅ 1 | audit + perbaikan |
| Validasi input & SQL aman (A03) | 🟡 0,75 | parameter terikat; validasi mayoritas |
| Kredensial terenkripsi, rahasia disamarkan | 🟡 0,75 | nilai Preset/Profil masih polos di DB |
| Rate limit & anti brute-force | 🟡 0,75 | per-IP in-memory; trusted proxy kini ada |
| TLS (REST & CWMP) | 🟡 0,25 | harus reverse proxy; belum disediakan |
| SSRF guard (CR, webhook) | 🟡 0,5 | jaringan privat diizinkan |
| Header keamanan, CORS, batas body | ✅ 1 | ada |
| Penetration test independen | ❌ 0 | belum |

### E. Operasional & deployment — 59% (bobot 15)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Docker Compose + migrasi otomatis (24) | ✅ 1 | diverifikasi |
| Backup & restore terdokumentasi/teruji | ✅ 1 | drill restore |
| Metrik Prometheus, alert, dashboard | ✅ 1 | ada |
| Retensi log otomatis | ✅ 1 | baru; uji MariaDB |
| CI (gofmt, vet, build, test -race, lint, build FE) | ✅ 1 | ci.yml |
| Health/readiness endpoint untuk orkestrator | ❌ 0 | tidak ada |
| Multi-instance/HA (rate limit, hub WS in-memory) | 🟡 0,5 | stateless sebagian |
| Reverse proxy TLS siap pakai di repo | ❌ 0 | belum |
| Skala: load test kapasitas | ❌ 0 | belum pernah |
| Orkestrasi (K8s/helm), autoscaling | ❌ 0 | belum |
| Runbook insiden & panduan operasional | ✅ 1 | RUNBOOK + buku panduan |

### F. Kualitas & verifikasi — 75% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Unit test backend + race | ✅ 1 | seluruh paket |
| Integrasi MariaDB nyata | ✅ 1 | ACS_TEST_DSN |
| E2E 13 skenario (simulator) | ✅ 1 | 13/13 |
| Uji CPE fisik (≥1 per vendor) | ❌ 0 | BELUM — UAT Rabu |
| Uji frontend otomatis (komponen/E2E UI) | 🟡 0,25 | hanya tes CSV |
| Dokumentasi API OpenAPI + Postman | ✅ 1 | 126 operasi |
| Audit keamanan/kualitas independen | ✅ 1 | 3 audit + perbaikan |

### G. Aspek SaaS komersial — 50% (bobot 10)

| Item | Skor | Bukti / catatan |
|---|:--:|---|
| Onboarding tenant (wizard) & panduan pengguna | ✅ 1 | ada + buku panduan |
| Landing page & materi pemasaran dasar | ✅ 1 | baru |
| Paket/plan, metering penggunaan, penagihan | ❌ 0 | tidak ada (hanya kuota task) |
| Halaman status / SLA / dukungan (helpdesk) | ❌ 0 | tidak ada |
| Syarat layanan, kebijakan privasi, DPA | ❌ 0 | tidak ada |
| Notifikasi email transaksional | ✅ 1 | ada |

## Prioritas menaikkan skor (nilai ÷ usaha)

1. **Uji CPE fisik** (UAT Rabu) — membuka −12 poin risiko terbesar. Hasilnya menentukan perbaikan katalog/mapping.
2. **Reverse proxy TLS + `/healthz`** di repo (kecil) — menutup D/E sekaligus.
3. **Verifikasi visual** mode gelap/mobile + perbaikan label/fokus/state galat (sedang).
4. **Diagnostics, SetParameterAttributes, refresh pohon parameter** (besar) — kesenjangan terbesar vs GenieACS.
5. **Enkripsi nilai rahasia Preset/Profil, audit trail lengkap, MFA** (sedang).
6. **Plan/metering/penagihan, ToS & privasi, halaman status** (besar) — hanya bila akan dijual sebagai layanan berbayar.
7. **Load test multi-IP** dan rate limit terdistribusi (sedang) sebelum onboarding armada besar.
