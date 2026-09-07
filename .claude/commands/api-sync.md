---
description: Sinkronkan openapi.yaml dengan router.go, lint spec, dan regenerasi koleksi Postman
argument-hint: "[nama tag/area, mis. Devices atau Webhooks] (opsional, default seluruh spec)"
---

Sinkronkan kontrak API untuk area: **$ARGUMENTS** (kosong = seluruh spec).

Gunakan agent `api-contract-sync`. Sumber kebenaran adalah `internal/delivery/http/router.go`, bukan `openapi.yaml`.

Urutan yang diharapkan:
1. Tampilkan **diff dua arah** (route ada di kode tapi tidak di spec, path hantu di spec, dan beda method/param) **sebelum** menulis perubahan.
2. Perbaiki spec, ambil bentuk payload dari `internal/delivery/http/dto.go` dan `internal/domain/` — jangan mengarang field.
3. `npx --yes @redocly/cli@2 lint openapi.yaml` harus bersih.
4. Regen `ACS-API.postman_collection.json` dari spec (jangan edit manual).
5. Laporkan angka: berapa operation ditambah/diubah/dihapus.

Kalau ditemukan route mutasi tanpa auth di `router.go`, hentikan dan laporkan itu sebagai temuan keamanan lebih dulu.
