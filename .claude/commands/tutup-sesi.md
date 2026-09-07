---
description: Tutup sesi kerja — catat progres jujur ke PROGRESS.md dan perbarui status ROADMAP.md
argument-hint: "<goal sesi ini> (opsional, kalau kosong akan disimpulkan dari percakapan)"
---

Tutup sesi kerja ini. Goal sesi: **$ARGUMENTS**

Gunakan agent `docs-keeper`.

Langkah:
1. `git log --oneline -10` dan `git status --short` untuk menetapkan apa yang benar-benar berubah dan sudah masuk commit mana.
2. Tambahkan entri sesi **di atas** entri terbaru `PROGRESS.md`, memakai struktur yang sudah dipakai di sana (Kondisi saat sesi dimulai / DIKERJAKAN / Diverifikasi / Belum dikerjakan).
3. Perbarui `ROADMAP.md` bila ada item yang statusnya berubah, termasuk baris **Terakhir diperbarui** di header.
4. Pakai tanggal absolut dan rujuk commit hash.

Aturan kejujuran yang tidak boleh dilanggar: bedakan **diimplementasikan** (kode ada), **terverifikasi** (build/test/lint lulus — sebutkan perintahnya), dan **tervalidasi di dunia nyata** (jalan dengan CPE/DB/browser sungguhan). Kalau sesuatu tidak diverifikasi, tulis "tidak diverifikasi" beserta alasannya. Catat juga utang yang sengaja ditinggalkan.

Jangan sentuh `PRD.md`, `TECH.md`, atau `CLAUDE.md`.
