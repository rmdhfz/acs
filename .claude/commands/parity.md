---
description: Audit kesesuaian API backend ↔ frontend — cakupan endpoint + kesetiaan kontrak per-field
argument-hint: "[domain, mis. Devices / Tenant / SelfService] (kosong = cakupan penuh + kontrak bertahap)"
---

Audit parity backend ↔ frontend untuk area: **$ARGUMENTS** (kosong = jalankan cakupan penuh dulu, lalu kontrak per domain secara bertahap).

Gunakan agent `api-frontend-parity`.

**Tahap 1 — cakupan (mekanis):** jalankan `node scripts/api-parity.mjs`. Laporkan panggilan yatim (bug, prioritas tertinggi) terpisah dari endpoint yang belum dipakai UI (gap fitur).

**Tahap 2 — kontrak per-field:** untuk endpoint mutasi di area tsb, bandingkan handler Go + `dto.go`, `schema.sql` (`VARCHAR(n)`, `NOT NULL`), `openapi.yaml`, dan form frontend (`required`/`maxLength`/`pattern`/tipe).

Yang paling saya ingin tahu: field yang **wajib/dibatasi di backend tapi tidak dijaga di form**, sehingga user dapat 500 atau 400 yang membingungkan.

Setiap temuan harus menyebut **kedua sisi** (file:baris backend/skema dan file:baris frontend) plus skenario konkretnya. Kalau tidak sempat mengaudit semua domain, katakan sampai mana — jangan mengklaim menyeluruh.
