---
description: Onboarding vendor/model CPE baru lewat jalur data (bukan percabangan kode)
argument-hint: "<vendor + model + OUI/MAC yang diketahui, mis. 'Cdata FD511GW OUI 00E0FC'>"
---

Onboarding vendor/model berikut: **$ARGUMENTS**

Gunakan agent `vendor-mapping-specialist`. Prinsipnya: menambah vendor adalah **operasi data**, bukan perubahan kode — default-nya INSERT, bukan `if vendor == "..."`.

Yang saya harapkan:
1. Baca `VENDOR_ONBOARDING.md` dan struktur kolom persis dari `schema.sql` — jangan menebak nama kolom.
2. Hasilkan INSERT untuk `ref_vendors`, `vendor_ouis`, `device_models`, lalu `vendor_parameter_mappings` untuk logical key yang relevan (mapping per `vendor_id` + `data_model_version_id`, lebih spesifik ke `device_model_id` hanya bila memang ada pengecualian di model itu).
3. Perhatikan root data model device: `InternetGatewayDevice.*` (TR-098) vs `Device.*` (TR-181) — jangan asumsikan satu.
4. Kalau butuh tabel/kolom yang belum ada, koordinasikan dengan `db-schema-guardian`; kalau vendor ini punya kuirk protokol nonstandar, adapter masuk ke `internal/vendor_adapter/<vendor>/`, bukan percabangan tersebar.
5. Tandai eksplisit mana path parameter yang **diverifikasi dari device nyata** dan mana yang masih **asumsi konvensi** (instance `.1`, dst).
