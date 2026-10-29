# Dokumentasi ACS

Indeks seluruh dokumen. Path di sini dan di dokumen lain dihitung dari **root repositori**.

| Saya ingin… | Buka |
|---|---|
| Menjalankan / mengoperasikan sistem | [`operasional/RUNBOOK.md`](operasional/RUNBOOK.md) |
| Memahami cara login, peran, SSO Google, MFA | [`operasional/LOGIN.md`](operasional/LOGIN.md) |
| Mendaftarkan vendor / model ONT baru | [`operasional/VENDOR_ONBOARDING.md`](operasional/VENDOR_ONBOARDING.md) |
| Belajar memakai Console (untuk tim NOC/operasional) | [`panduan/BUKU_PANDUAN_ACS.html`](panduan/BUKU_PANDUAN_ACS.html) (PDF di sebelahnya) |
| Menyiapkan & menjalankan UAT | [`pengujian/PANDUAN_UAT_ACS.html`](pengujian/PANDUAN_UAT_ACS.html) |
| Menguji dengan ONT fisik | [`pengujian/PENGUJIAN_LAPANGAN.md`](pengujian/PENGUJIAN_LAPANGAN.md), [`pengujian/PANDUAN_PENGUJIAN_LAPANGAN.html`](pengujian/PANDUAN_PENGUJIAN_LAPANGAN.html) |
| Menjalankan uji otomatis (e2e + simulator) | [`pengujian/PENGUJIAN_E2E.md`](pengujian/PENGUJIAN_E2E.md) |
| Menguji fitur baru (faults, query bebas, virtual parameter, otomasi) di ONT fisik | [`pengujian/PANDUAN_UJI_ONT_FITUR_BARU.md`](pengujian/PANDUAN_UJI_ONT_FITUR_BARU.md) |
| Benchmark vs GenieACS di server terpisah (beberapa run, median) | [`pengujian/BENCHMARK_SERVER_TERPISAH.md`](pengujian/BENCHMARK_SERVER_TERPISAH.md), hasil terakhir: [`pengujian/BENCHMARK_GENIEACS.md`](pengujian/BENCHMARK_GENIEACS.md) |
| Tahu status kesiapan & skor | [`laporan/CHECKLIST_SAAS.md`](laporan/CHECKLIST_SAAS.md) |
| Melihat riwayat pekerjaan | [`laporan/PROGRESS.md`](laporan/PROGRESS.md) |
| Memahami kebutuhan produk | [`produk/PRD.md`](produk/PRD.md), [`produk/ROADMAP.md`](produk/ROADMAP.md) |
| Memahami arsitektur & keputusan teknis | [`arsitektur/TECH.md`](arsitektur/TECH.md) (+ desain Preset, USP/TR-369, [OLT Controller](arsitektur/OLT_CONTROLLER_DESIGN.md), [Connection Request di balik NAT/CGNAT](arsitektur/CONNECTION_REQUEST_NAT_DESIGN.md) (usulan), [Peta, status realtime & diagnosis offline](arsitektur/PETA_STATUS_DIAGNOSIS.md), panduan arsitektur PDF) |
| Membaca kontrak API | [`../openapi.yaml`](../openapi.yaml) · koleksi Postman [`../ACS-API.postman_collection.json`](../ACS-API.postman_collection.json) |

## Struktur

```
docs/
  produk/        PRD, ROADMAP                     — apa & mengapa (keputusan produk)
  arsitektur/    TECH, desain Preset, USP/TR-369, OLT Controller  — bagaimana (keputusan teknis)
  operasional/   RUNBOOK, LOGIN, VENDOR_ONBOARDING — menjalankan & merawat
  panduan/       Buku Panduan pengguna (HTML + PDF)
  pengujian/     UAT, uji lapangan, e2e
  laporan/       CHECKLIST_SAAS (status berkala), PROGRESS (log kronologis)
  arsip/         dokumen lama yang sudah digantikan (jangan dipakai sebagai acuan)
```

Di root hanya tersisa yang dipakai alat: `README.md`, `CLAUDE.md` (aturan kerja), `openapi.yaml`, koleksi Postman, `schema.sql`, `docker-compose*.yml`, `Dockerfile`.

## Aturan agar tidak berantakan lagi

1. **Satu topik, satu dokumen.** Tambahkan ke dokumen yang ada, bukan berkas baru. Status berkala masuk `laporan/CHECKLIST_SAAS.md`; catatan sesi kerja masuk `laporan/PROGRESS.md`.
2. **PDF dibuat dari HTML**, jangan disunting tangan: `node scripts/build-docs-pdf.mjs` (butuh Chrome/Chromium). HTML adalah sumbernya.
3. **Dokumen yang sudah digantikan dipindah ke `arsip/`** (bukan dihapus, riwayatnya tetap ada), dengan satu baris di `arsip/README.md` tentang penggantinya.
4. `produk/PRD.md` dan `arsitektur/TECH.md` adalah keputusan — ubah hanya atas persetujuan pemilik produk.
5. Tulis tanggal absolut (mis. 2026-10-17), dan nyatakan jujur apa yang **belum** diuji.

## Catatan riwayat

`laporan/PROGRESS.md` dan `produk/ROADMAP.md` adalah catatan kronologis; nama berkas yang disebut di dalamnya (mis. `RUNBOOK.md`, `TECH.md`) merujuk ke lokasi **sebelum** penataan ini — cari di tabel di atas.
