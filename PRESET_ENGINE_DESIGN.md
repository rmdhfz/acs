# PRESET_ENGINE_DESIGN.md — Proposal Desain Engine Preset

**Status:** v1 DIIMPLEMENTASIKAN 2026-09-02 (branch `dev`) — **backend belum
diverifikasi** (`go build`/`go test`/gofmt/migrasi ke MariaDB nyata belum
dijalankan; tidak ada Go toolchain di host sesi ini — CI `.github/workflows/ci.yml`
akan memverifikasi saat push).
**Dibuat:** 2026-09-02 (sesi `/goal` "lebih baik dari GenieACS?")

### Cakupan v1 yang benar-benar diimplementasikan (2026-09-02)

- migrasi `0021_preset_engine` (`presets.enforce`/`channel` + tabel `preset_applications`) + `schema.sql`.
- `domain.Preset` (+`Enforce`/`Channel`), `PresetPrecondition`, `PresetConfigOp`, `PresetApplication` + `PresetApplicationRepository`; helper `ParsedPrecondition()`/`ParsedConfigurations()`.
- `domain.TaskEnqueuer` +`ResolveParameterPath` (task.Service sudah punya).
- `mysql.presetRepository.ListActiveEnforce` + `mysql.presetApplicationRepository`.
- `provisioning.Service.EvaluatePresets` (+`preset_eval.go`) — precondition match (reuse `matchSQLLike`), drift-check per op `set_parameter` (resolve key→path→bandingkan `device_parameters`), merge drift lintas preset → 1 `SetParameterValues`, pagar `HasPendingForDevice` + cooldown 15m + give-up setelah 3× push tanpa konvergensi (hash drift sama) → status `FAILED` + `PRESET_APPLY_FAILED` activity log.
- Wiring: `session.Service.HandleInform` memanggil `EvaluatePresets` setelah `EvaluateZeroTouch`; `cmd/acsd/main.go`.
- `preset.Service.Create/Update` validasi JSON precondition/configurations (tolak op non-`set_parameter` di v1) + salin `Enforce`/`Channel`.
- Test: `provisioning/preset_eval_test.go` (7 skenario: nil-engine no-op, drift→1 task, konvergen→0 task, pending-task skip, precondition mismatch, cooldown, give-up).
- `openapi.yaml` Preset/CreatePresetRequest/UpdatePresetRequest + `Preset` type FE.

**DITUNDA dari desain (v1.1):** op `apply_profile` & `refresh`; placeholder nilai (`{serial4}`); precondition `tag_id` (butuh `TagRepository` di `provisioning.Service`); **`channel` cuma disimpan, belum dipakai**.

**TEMUAN saat implementasi:** `PresetsPage.tsx` **TIDAK ADA** (desain keliru menyebut "halaman ada") — presets sejauh ini backend + endpoint + hook FE (`usePresets` dkk) tanpa halaman/route. UI preset (termasuk toggle `enforce` + estimasi "N device terpengaruh" per keputusan 5b) **masih perlu dibuat** — belum ada di v1 ini.

## Keputusan pemilik produk (2026-09-02)

| Pertanyaan §9 | Jawaban |
|---|---|
| §1 Arah | **Opsi C** — preset minimal = precondition gaya ZTP + drift-check tiap sesi, tanpa DSL/skrip |
| §5 FR-18 | **5a + 5b** — preset boleh enforcement berkelanjutan; FR-18 hanya untuk provisioning profile. Toggle `enforce` per preset (default OFF). Saat diaktifkan, UI tampilkan estimasi "N device akan terpengaruh" |
| §3 `op` v1 | `set_parameter` + `apply_profile`. `refresh` DITUNDA ke v1.1 (butuh method enqueuer baru) |
| Placeholder nilai | DITUNDA ke v1.1 — v1 nilai literal saja |

**Konteks:** audit 2026-09-02 menemukan tabel `presets` + endpoint `/presets` +
halaman `PresetsPage` sudah ada (migrations/0019), **tapi tidak ada engine yang
mengevaluasinya** — `preset.Service` hanya CRUD, tak pernah dipanggil dari jalur
sesi CWMP. Ini "fitur yang tampak jadi padahal kosong". Di GenieACS, preset
adalah mekanisme provisioning **inti**.

Dokumen ini bukan spesifikasi final — ini menaruh keputusan di tangan pemilik
produk sebelum kode ditulis, sesuai CLAUDE.md ("bila mengerjakan area yang
keputusannya belum final, tanyakan dulu").

---

## 1. Keputusan #0 (paling penting): apakah kita butuh preset sama sekali?

`zero_touch_rules` (ZTP) sudah cukup kaya sejak batch 2026-08-26: precondition
`vendor`/`model`/`oui`/`serial_pattern`/`software_version_pattern`/
`match_parameter_*`, aksi gabungan (profile + reboot + firmware), dan **trigger
per-rule** (`BOOTSTRAP_ONLY` / `BOOTSTRAP_OR_BOOT` / `EVERY_INFORM`).

**Perbedaan konseptual preset vs ZTP rule:**

| | Zero-Touch Rule | Preset (gaya GenieACS) |
|---|---|---|
| Tujuan | Provisioning **awal** device baru | **Desired-state berkelanjutan** — device selalu dijaga sesuai konfigurasi |
| Kapan | Saat trigger cocok (bootstrap/boot/inform) | Setiap sesi, **jika device menyimpang** dari nilai yang diinginkan |
| Idempotensi | Tidak dijaga (rule bisa apply berkali-kali; ada pagar anti-loop) | Wajib: apply HANYA saat ada drift |
| FR-18 | Sejalan (apply eksplisit sekali) | **Bertentangan** — lihat §5 |

**Opsi:**

- **Opsi A — Hapus `presets`.** Buang tabel + endpoint + `PresetsPage` +
  `preset.Service`. Perkaya `zero_touch_rules` bila butuh trigger/aksi baru.
  Paling murah, tidak ada utang desain. **Kekurangan:** tidak ada "desired-state
  enforcement" — kalau pelanggan/teknisi mengubah SSID lewat GUI CPE, ACS tidak
  mengembalikannya. GenieACS bisa.
- **Opsi B — Wire engine preset penuh** sebagai layer "desired-state" terpisah
  dari ZTP. Lebih dekat ke GenieACS, tapi menambah kompleksitas dan menyentuh
  FR-18. Detail di §2–§7.
- **Opsi C — Wire versi minimal:** preset = "ZTP rule yang dievaluasi tiap sesi
  DENGAN drift-check", tanpa channel/schedule/virtual-param. Reuse mesin
  precondition ZTP. ~70% nilai GenieACS dengan ~30% kompleksitasnya.

**Rekomendasi: Opsi C.** Alasan: menutup gap paling terasa (enforcement +
drift-heal) tanpa membangun DSL/scheduler. Bisa naik ke Opsi B kalau terbukti
perlu. Sisa dokumen mengasumsikan Opsi C.

---

## 2. Precondition — bentuk data

**JANGAN buat bahasa ekspresi/DSL baru** (dan JANGAN skrip JS seperti GenieACS
provisions — itu keluar dari filosofi "data terstruktur, auditable" di
`ROADMAP.md` §1 dan bertentangan dengan CLAUDE.md soal tidak menaruh logika di
tempat yang sulit di-review).

`presets.precondition` (kolom JSON yang sudah ada) diisi objek terstruktur yang
**kosakatanya identik** dengan precondition ZTP — supaya bisa reuse
`matchPrecondition()` + `matchSQLLike()` yang sudah ada & sudah ada test-nya:

```json
{
  "vendor_id": 3,
  "device_model_id": null,
  "oui": null,
  "serial_pattern": "ZTEG%",
  "software_version_pattern": "V9.0.10%",
  "match_parameter_name": null,
  "match_parameter_value_pattern": null,
  "tag_id": 12
}
```

Semua field opsional, digabung dengan **AND**. Tambahan dari ZTP: `tag_id`
(preset per-segmen device — sejalan dengan fitur tag yang baru dilengkapi).
Field kosong/`null` = tidak dibatasi. Precondition `{}` = berlaku ke semua
device tenant (valid, tapi UI harus warning).

---

## 3. Configurations — bentuk data

`presets.configurations` (kolom JSON) diisi **array operasi bertipe**, diterapkan
berurutan:

```json
[
  { "op": "set_parameter", "key": "wifi.ssid", "value": "MyISP-{serial4}" },
  { "op": "set_parameter", "key": "device.periodic_inform_interval", "value": "300" },
  { "op": "apply_profile", "profile_id": 7 }
]
```

**`op` yang didukung di v1:**

| `op` | Arti | Implementasi |
|---|---|---|
| `set_parameter` | Jaga satu parameter pada nilai tertentu | `key` di-resolve lewat `vendor_parameter_mappings` (logical) atau dipakai apa adanya (raw TR-069 path), sama seperti provisioning profile |
| `apply_profile` | Terapkan seluruh parameter sebuah `provisioning_profile` | reuse `provisioning.Service.ApplyProfile` |
| `refresh` | `GetParameterValues` path tertentu tiap sesi (jaga `device_parameters` tetap segar untuk drift-check & dashboard) | perlu method BARU `TaskEnqueuer.EnqueueGetParameterValues` — saat ini enqueuer hanya punya `EnqueueSetParameterValues`/`EnqueueGetParameterNames`/`EnqueueReboot` |

**Placeholder nilai** (opsional, v1.1): `{serial}`, `{serial4}` (4 digit
terakhir), `{mac}` — supaya SSID/hostname bisa per-device. Kalau menambah
kompleksitas parser terlalu jauh, tunda.

**TIDAK di v1:** `op: reboot` (rawan loop bila digabung enforcement tiap sesi),
`add_object`/`delete_object` (butuh logika instance-matching yang rumit),
virtual/computed parameters, extension call-out.

---

## 4. Kapan & bagaimana dievaluasi (jantung desainnya)

Dievaluasi di `session.Service`, **setelah Inform diproses & `device_parameters`
ter-update**, sebelum sesi ditutup — titik yang sama dengan `EvaluateZeroTouch`
(lihat `session/service.go` sekitar baris 287).

```
Inform diproses
      │
      ▼
EvaluateZeroTouch (sudah ada)  ── trigger bootstrap/boot/inform
      │
      ▼
EvaluatePresets (BARU)
  1. Ambil presets aktif milik tenant device, urut weight ASC
  2. Untuk tiap preset yang precondition-nya cocok:
       untuk tiap op set_parameter:
         desired = resolve(key) -> (tr069_path, value)
         current = device_parameters[tr069_path]
         if current != value:  kumpulkan ke drift-set
  3. Bila drift-set tidak kosong DAN device tidak punya task
     pending/queued/sent:
       enqueue SATU SetParameterValues berisi seluruh drift-set
       (gabungan semua preset, preset weight lebih besar menang saat
        key bentrok)
  4. op apply_profile / refresh: enqueue sesuai aturannya sendiri
       (apply_profile hanya bila ada indikasi profile belum pernah
        diterapkan — butuh tracking, lihat §6)
```

**Kunci: drift-check.** Tanpa ini, tiap sesi akan mengantre SetParameterValues
identik → task spam + kemungkinan loop. Dengan drift-check, setelah device
konvergen, preset jadi no-op sampai ada yang mengubah nilainya di luar ACS
(teknisi via GUI CPE, factory reset sebagian, dsb) — persis perilaku
"self-healing" GenieACS.

**Pagar keamanan (reuse dari ZTP EVERY_INFORM):**
- Device dengan task `PENDING`/`QUEUED`/`SENT` → lewati evaluasi preset sesi ini.
- Hormati kuota task queue tenant (`ErrQuotaExceeded`).
- Cooldown per (device, preset): jangan re-apply preset yang sama ke device yang
  sama dalam < N menit (default 15) — jaga-jaga bila CPE menolak nilai dan
  melaporkan nilai lama terus (drift "abadi"). Setelah cooldown, coba lagi;
  setelah M kegagalan berturut-turut, tandai preset "gagal" pada device itu &
  catat `activity_logs` (`PRESET_APPLY_FAILED`) supaya NOC tahu — sejalan FR-15.

---

## 5. Konflik dengan FR-18 — keputusan produk yang WAJIB dikonfirmasi

`PRD.md` FR-18 + `CLAUDE.md`:

> Perubahan pada `provisioning_profile_parameters` tidak otomatis mendorong ulang
> konfigurasi ke device yang sudah terprovisioning — ini keputusan produk yang
> disengaja, jangan "diperbaiki" jadi otomatis tanpa konfirmasi.

Engine preset (enforcement tiap sesi) **secara desain melanggar semangat ini**:
ubah `configurations` sebuah preset → semua device yang cocok akan drift →
otomatis di-push ulang pada sesi berikutnya.

**Ini bukan bug, ini definisi preset.** Tapi harus eksplisit disetujui:

- **Keputusan 5a:** Preset = enforcement berkelanjutan (device selalu dijaga),
  BERBEDA dari provisioning profile (apply eksplisit sekali). Keduanya hidup
  berdampingan. → Perlu tanda tangan pemilik produk bahwa FR-18 hanya berlaku
  untuk provisioning profile, bukan preset.
- **Keputusan 5b (mitigasi):** Ubah `configurations`/`precondition` preset
  TIDAK langsung aktif — butuh toggle "aktifkan enforcement" per preset, dan
  saat pertama diaktifkan tampilkan estimasi "N device akan terpengaruh".
- **Keputusan 5c (alternatif konservatif):** Preset hanya apply **sekali** saat
  precondition BARU cocok untuk sebuah device (butuh tabel tracking §6), bukan
  enforcement terus-menerus. Ini lebih dekat ke "ZTP rule dengan trigger kaya"
  daripada "preset GenieACS", tapi tidak menyentuh FR-18.

**Rekomendasi: 5a + 5b.** 5c mengurangi nilai fitur sampai nyaris tidak beda
dari ZTP rule.

---

## 6. Perubahan skema yang dibutuhkan

Kolom `presets` yang ada cukup untuk precondition + configurations. Tambahan:

```sql
-- migrations/00XX_preset_engine

ALTER TABLE presets
  ADD COLUMN enforce TINYINT(1) NOT NULL DEFAULT 0
    COMMENT 'Bila 1: enforcement berkelanjutan tiap sesi (drift-heal). Bila 0: preset tersimpan tapi engine mengabaikannya (keputusan 5b).',
  ADD COLUMN channel VARCHAR(64) NULL
    COMMENT 'Grouping opsional (gaya GenieACS channel) — informasi UI saja di v1, tidak mengubah evaluasi';

-- Tracking apply per (device, preset): idempotensi apply_profile,
-- cooldown, dan hitung kegagalan. Tabel log volume sedang -> audit minimal.
CREATE TABLE preset_applications (
    preset_id       BIGINT UNSIGNED NOT NULL,
    device_id       BIGINT UNSIGNED NOT NULL,
    last_applied_at DATETIME        NULL,
    last_drift_hash CHAR(64)        NULL COMMENT 'sha256 dari drift-set terakhir yang di-push, untuk short-circuit',
    consecutive_failures INT        NOT NULL DEFAULT 0,
    status          VARCHAR(16)     NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING|CONVERGED|FAILED',
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (preset_id, device_id),
    KEY idx_preset_apps_device (device_id),
    CONSTRAINT fk_preset_apps_preset FOREIGN KEY (preset_id) REFERENCES presets (id) ON DELETE CASCADE,
    CONSTRAINT fk_preset_apps_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

Ikuti CLAUDE.md: tambah ke `schema.sql`, migrasi terurut di `migrations/`,
validasi up/down ke MariaDB nyata.

---

## 7. Struktur kode (Clean Architecture)

- `internal/domain/preset.go` — perluas `Preset` (`Enforce`, `Channel`), tambah
  tipe `PresetPrecondition`, `PresetConfigOp`, `PresetApplication` +
  interface `PresetApplicationRepository`.
- `internal/usecase/provisioning/preset_eval.go` (BARU) — `EvaluatePresets(ctx,
  actor, dev)`. Ditaruh di package `provisioning` (bukan `preset`) karena butuh
  akses ke `vendor_parameter_mappings` resolver + `enqueuer` yang sama dengan
  `EvaluateZeroTouch`; `preset.Service` tetap CRUD murni. Reuse
  `matchPrecondition`, `matchSQLLike`, resolver logical-key.
- `internal/usecase/session/service.go` — panggil `EvaluatePresets` tepat setelah
  `EvaluateZeroTouch`. Guard nil, jangan gagalkan sesi bila preset error (log &
  lanjut, pola sama dengan ZTP).
- `internal/repository/mysql/preset_repository.go` — tambah query
  `ListActiveByTenant`, `preset_applications` upsert/get.
- **Test wajib** (CLAUDE.md — area task queue): drift terdeteksi → 1 task;
  device sudah konvergen → 0 task; device punya task in-flight → skip; cooldown
  dihormati; M kegagalan → status FAILED + activity log; kuota tenant → tidak
  enqueue. Mutation-test seperti `TestCreateTaskTenantQuota`.

---

## 8. Yang TIDAK dikerjakan (batas eksplisit v1)

- Virtual/computed parameters (GenieACS `virtualParameters`) — butuh evaluator
  ekspresi; keluar dari filosofi data-terstruktur. Tunda / mungkin tidak pernah.
- Extensions (call-out ke API eksternal saat provisioning) — idem.
- Schedule/cron per preset — `channel` disiapkan sebagai kolom tapi tidak
  mengubah evaluasi di v1.
- `op: reboot` di dalam preset.
- Preset lintas-tenant global (`tenant_id NULL`) — v1 preset selalu per-tenant
  (beda dari ZTP rule yang boleh global). Bisa ditambah kalau perlu.

---

## 9. Keputusan yang diminta dari pemilik produk

1. **§1:** Opsi A (hapus preset), B (engine penuh), atau **C (minimal + drift)**?
2. **§5:** Setuju preset = enforcement berkelanjutan dan FR-18 hanya untuk
   provisioning profile (5a)? Dengan toggle `enforce` + estimasi dampak (5b)?
   Atau versi konservatif apply-sekali (5c)?
3. **§3:** Cukup 3 `op` (`set_parameter`, `apply_profile`, `refresh`) untuk v1?
4. Butuh placeholder nilai (`{serial4}` dll) di v1 atau tunda?

Setelah 4 jawaban ini, implementasi bisa dimulai (perkiraan: 1 migrasi + ~400
baris usecase + ~200 baris test + wiring session + UI PresetsPage yang sudah ada
tinggal disesuaikan field baru).
