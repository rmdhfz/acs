# PROGRESS.md — Log Sesi Kerja Otonom

Dibuat atas permintaan `/goal` (sesi kerja otonom semalam). Isinya: apa yang
ditemukan saat sesi dimulai, keputusan interpretasi, apa yang dikerjakan &
diverifikasi, dan **daftar TODO/temuan yang jujur** untuk ditinjau saat bangun.
Baca bersama `ROADMAP.md` — dokumen ini melengkapinya, bukan menggantikannya.

**Sesi:** 2026-08-27 (dini hari)
**Agent:** Claude Sonnet 5 (Claude Code)

---

## 0-A. Status Milestone `/goal` (Milestone 1–4) — SUDAH TERPENUHI, dibuktikan live

Milestone di prompt `/goal` menggambarkan proyek dari nol. Proyek ini sudah jauh
melewati semuanya (ROADMAP Fase 0/1/2 selesai). Alih-alih membangun ulang &
menghancurkan kode tim yang sudah teruji, sesi ini **membuktikan tiap milestone
lewat menjalankan stack lengkap end-to-end** (`docker compose up` — MariaDB +
MinIO + acsd + Prometheus, semua sehat):

| Milestone `/goal` | Bukti live sesi ini (2026-08-27) |
|---|---|
| **M1** — struktur proyek, lint, dockerisasi | 23 paket Go (`cmd/*`, `internal/{domain,usecase/*,repository,delivery/*,metrics}`, `pkg/*`) + `frontend/` React 19. `go vet ./...` ✅ bersih, `npx oxlint src` ✅ bersih, `tsc -b && vite build` ✅ (2447 modul). `Dockerfile` (multi-stage) + `docker-compose.yml` (5 service) + `.github/workflows/ci.yml`. **Stack di-`docker compose up` dan berjalan sehat saat sesi ini.** |
| **M2** — TR-069 listener/parser + connection request | `pkg/cwmpxml` (parser SOAP/XML manual) + `internal/delivery/cwmp` (listener :7547) + `internal/usecase/session` (state machine). **Live: `cmd/loadtest` mengirim 40 sesi CWMP nyata (Inform→InformResponse→POST kosong) ke acsd → 40/40 sukses, p95 76 ms (target NFR <300 ms).** 40 device baru ter-upsert dari Inform, metrik `acs_cwmp_inform_response_latency_seconds_count`=40. Connection Request handler: `internal/usecase/device/service.go` (HTTP GET ke `connection_request_url` device). |
| **M3** — API device management + integrasi + OpenAPI | 59 endpoint REST (`internal/delivery/http/router.go`), `openapi.yaml` 3.0.3 (lint ✅ valid). **Live (JWT superadmin): `GET /devices` (total 314), `/devices/stats`, `/tasks/stats`, `/refs/*`, `GET /firmware/rollout-batches` (3 batch nyata terlihat: COMPLETED / PAUSED / IN_PROGRESS), `POST` validasi 400 benar, `PATCH /tenants/:id/cwmp-credentials` 204, `GET /api/v1/metrics` Prometheus exposition.** |
| **M4** — React dashboard + state mgmt + telemetry graph | 10 halaman (`frontend/src/pages/`), state via `@tanstack/react-query` (`useQuery`/`useMutation`, polling 5 dtk), grafik telemetri via `recharts` (Dashboard + tren redaman optik di Device Detail). **Live: `vite preview` menyajikan build → `HTTP 200`, `<title>ACS Console</title>`, bundle JS 855 KB ter-serve.** Verifikasi visual manual tetap perlu (tidak ada tool browser di environment ini). |

Detail keputusan "kenapa tidak greenfield" ada di §1. Progres sesi ini:
verifikasi + sinkronisasi + demonstrasi, plus (jika sempat) modul baru
**webhook** (gap nyata vs. `/goal` directive #3 — lihat §7).

---

## 0. TL;DR untuk dibaca saat bangun

1. **Prompt `/goal` adalah template greenfield** ("Initialize project
   structures", "Build React frontend", "GraphQL", "split time-series DB",
   "TR-369/USP", "scale to 2,000,000") — sebagian besar **tidak cocok** dengan
   repo ini yang sudah matang (Fase 0/1/2 `ROADMAP.md` selesai, Fase 3
   sebagian), dan beberapa poin **bertabrakan langsung dengan `CLAUDE.md`/`TECH.md`**
   (REST bukan GraphQL, MariaDB InnoDB, tidak ada perubahan skema auto-apply,
   konfirmasi dulu sebelum keputusan arsitektur besar). Detail §1.
2. **Interpretasi yang diambil:** kerjakan *maksud*-nya ("ACS lebih baik dari
   GenieACS") dengan **memajukan codebase yang ADA** sesuai konvensinya, bukan
   membangun ulang atau menabrak arsitektur mapan selagi Anda tidur. Tidak ada
   subagent di-spawn. **Tidak ada commit dibuat** (§4).
3. **Batch besar 2026-08-26 (~2.900 baris, migrasi 0009–0012) belum di-commit.**
   Sesi ini: **direview lewat inspeksi + diverifikasi `go vet`/`go build`/`go
   test` SEMUA LULUS + migrasi 0009–0012 up & down LULUS ke MariaDB nyata.**
   Layak di-commit (§2, §3a).
4. **Ditemukan & diperbaiki regresi: frontend Zero-Touch Rule tidak sinkron
   dengan kontrak API baru** — buat ZT rule dari UI akan gagal `400` karena
   `trigger_event_id` sekarang wajib. Frontend disinkronkan + ditambah UI
   Firmware Rollout Batch. Build+lint frontend lulus. **Belum diuji di browser**
   (§3b, §3c).
5. **Ditemukan bug pre-existing: `migrations/0006_task_metrics_indexes.down.sql`
   rusak** — `migrate down` (revert penuh) gagal di 0006 (`Cannot drop index
   'idx_tasks_status': needed in a foreign key constraint`). Bukan dari batch
   ini, tidak memengaruhi `migrate up`. TODO-2 (§5).
6. **Docker Desktop wedged berat di awal sesi** (EOF, `500` engine API, proxy
   modul korup) — dipulihkan dengan force-kill + `wsl --shutdown` + relaunch.
   Setelah pulih, semua verifikasi di atas berhasil dijalankan.

---

## 1. Kondisi saat sesi dimulai vs. isi prompt `/goal`

| Prompt `/goal` minta | Realita repo |
|---|---|
| "Milestone 1: Initialize project structures (Backend & Frontend)" | Backend Go lengkap (6 usecase + IAM + CWMP), frontend React 19 + Vite + Tailwind v4 (~10 halaman), `docker-compose` + CI GitHub Actions sudah ada sejak Agustus |
| "Build the frontend using React.js" | Sudah ada |
| "RESTful **or GraphQL** API ... Swagger/OpenAPI" | REST (Echo v5) + `openapi.yaml` 3.0.3 + Postman collection sudah ada. `TECH.md`: REST, bukan GraphQL |
| "separating time-series data from relational data" | `TECH.md` §9/§12: MariaDB InnoDB; strategi partisi tabel log **sengaja belum diputuskan** — "jangan diasumsikan sepihak" |
| "prepare the foundation for TR-369 (WebSockets/MQTT)" | Out-of-scope Fase 1 (`PRD.md` §4.2); Fase 3 belum mulai. Keputusan desain besar → butuh konfirmasi |
| "scale up to 2,000,000 active CPE" | `PRD.md` NFR: "puluhan ribu device, ribuan sesi concurrent". Desain stateless sudah ada |
| "append `-y`/`--force`" / "Automatically create ... database schemas" | `CLAUDE.md`: validasi migrasi end-to-end ke MariaDB nyata **wajib**; jangan hard-delete; konfirmasi sebelum perubahan besar |

**Kesimpulan:** menuruti milestone secara harfiah = menimpa codebase matang
tanpa Anda mengoreksi. Jadi maksudnya dikerjakan lewat jalur aman & sesuai
konvensi.

---

## 2. Batch belum-commit 2026-08-26 — status VERIFIKASI

`git status` (awal sesi): ~2.900 baris berubah belum di-commit + migrasi baru
`0009`–`0012` + 4 file test/metrik baru. Ini batch **"Robustness Protokol CWMP +
rules engine ZTP + firmware canary rollout"** yang di `ROADMAP.md` sudah
ditandai `[x]` tapi **belum masuk git** (commit terakhir `68c5c08`, 2026-08-24).

### 2a. Review inspeksi (diff seluruh file)

Diperiksa: `pkg/cwmpxml/{envelope,rpc}.go`, `internal/delivery/cwmp/{handler,
builder}.go`, `internal/usecase/{session,task,provisioning,firmware}/service.go`
+ test-nya, `internal/domain/*`, `internal/repository/mysql/*`,
`cmd/acsd/main.go`, `internal/config/config.go`, `schema.sql`, migrasi
`0009`–`0012`, `internal/metrics/inform_latency.go`, handler REST terkait.

**Tidak ada red flag arsitektural.** Clean Architecture dipatuhi (logika di
usecase, handler tipis, repo hanya query), konvensi `ref_*`/audit-7-kolom/
soft-delete/UUID diikuti, tidak ada `if vendor == "..."`, `schema.sql` sinkron
dengan migrasi. Komentar sadar edge-case (anti-reboot-loop 2 lapis + cooldown,
advisory-lock lintas-instance untuk wave rollout, bug `matchSQLLike` QuoteMeta,
bug namespace XML hardcode yang diperbaiki di batch ini).

### 2b. Verifikasi eksekusi — **SEMUA LULUS** (via image `golang:latest`, Docker)

```
go vet ./...     -> exit 0, tanpa output   ✅
go build ./...    -> exit 0, tanpa output   ✅
go test ./...     -> semua paket ok, exit 0 ✅
```
Paket yang lulus test termasuk yang disentuh batch ini: `delivery/cwmp`,
`metrics`, `usecase/{auth,firmware,provisioning,session,task}`, `pkg/cwmpxml`.

### 2c. Verifikasi migrasi 0009–0012 — **LULUS ke MariaDB 10.11 nyata**

Dijalankan pada container MariaDB throwaway terpisah (BUKAN volume dev
persisten `acs_acs-mariadb-data` — pelajaran dari insiden `down -v`
2026-08-23):
```
migrate up   (0 -> 12)  -> "selesai", version=12 dirty=false          ✅
migrate down (12 -> ...)  -> 0012,0011,0010,0009,0008,0007 REVERT OK,
                             lalu GAGAL di 0006 (bug pre-existing, §5)
```
Jadi **up & down migrasi 0009–0012 keduanya terbukti bersih.** Kegagalan
`down` terjadi jauh di bawahnya, di migrasi lama `0006`.

**Kesimpulan §2:** batch 2026-08-26 layak di-commit. Yang tersisa sebelum
commit: (a) update `openapi.yaml`/Postman (TODO-3), (b) sinkronisasi frontend
(sudah dikerjakan, §3c — perlu uji browser), (c) keputusan Anda untuk
meng-commit.

---

## 3. Yang dikerjakan sesi ini

### 3a. Verifikasi backend + frontend baseline

- Backend: lihat §2b/§2c (semua lulus).
- Frontend baseline (sebelum perubahan): `npm run build` exit 0, `npx oxlint
  src` exit 0 (3 warning `only-export-components` pre-existing).

### 3b. TEMUAN: regresi kontrak frontend ↔ backend Zero-Touch Rule

Batch 2026-08-26 mengubah `POST/PUT /zero-touch-rules`:
- `trigger_event_id` **sekarang WAJIB** (`400` bila 0/kosong).
- `provisioning_profile_id` **sekarang opsional** (rule boleh hanya
  reboot/firmware push).
- Field baru: `software_version_pattern`, `match_parameter_name` +
  `match_parameter_value_pattern` (berpasangan), `post_apply_reboot`,
  `firmware_file_id`.

Frontend (`hooks.ts` `ZTRuleFormInput`, `types.ts` `ZeroTouchRule`,
`ProvisioningPage.tsx` `ZTRuleModal`) masih pakai bentuk lama → **buat/edit ZT
rule dari UI GAGAL 400** begitu batch backend dideploy. Entri `ROADMAP.md`
2026-08-26 adalah backend-only dan tidak menyebut frontend.

### 3c. Perbaikan frontend (murni FE, ikut pola komponen yang ada)

**Belum diuji di browser** — tidak ada tool browser di environment ini
(konsisten dengan seluruh pekerjaan FE sesi sebelumnya). `npm run build` (tsc +
vite) **exit 0**, `npx oxlint src` **exit 0**.

- `src/lib/types.ts` — `ZeroTouchRule` diperluas (field baru;
  `provisioning_profile_id` jadi nullable); `VendorParameterMapping` +
  `software_version_pattern`; tipe baru `FirmwareRolloutBatch`.
- `src/lib/hooks.ts` — `ZTRuleFormInput` & `UpsertMappingInput` diperluas;
  hook baru `useFirmwareRolloutBatches` / `useCreateRolloutBatch` /
  `useAdvanceRolloutBatch` / `useCancelRolloutBatch`.
- `src/pages/ProvisioningPage.tsx` `ZTRuleModal` — dropdown **Trigger Event**
  (wajib, default `BOOTSTRAP_ONLY` untuk rule baru = perilaku lama), profil jadi
  opsional, checkbox "Reboot setelah apply", dropdown "Push Firmware" (scoped ke
  vendor rule), input `software_version_pattern` + pasangan match-parameter.
  Validasi klien: minimal 1 aksi, match-param harus berpasangan. Tabel ZT rule
  dapat kolom "Trigger" & "Aksi".
- `src/pages/CatalogPage.tsx` `CreateMappingModal` — field opsional
  `software_version_pattern` + ditampilkan di kolom Scope.
- `src/pages/FirmwarePage.tsx` — **tab baru "Rollout Batch"**: daftar batch
  (status via `ref_firmware_rollout_status`, progres wave, ambang gagal),
  tombol Advance/Batalkan per baris (ADMIN), modal "Rollout Baru" (filter
  vendor/model, firmware target, wave %, max failure %). Halaman lama jadi tab
  "Katalog".

### 3d. Dokumen

- `PROGRESS.md` (file ini).
- `ROADMAP.md` — entri 2026-08-27 ditambahkan di §6 log perubahan + catatan
  jujur pada entri 2026-08-26 bahwa frontend rules-engine menyusul di sesi ini.

---

## 4. Kenapa TIDAK ada commit dibuat

- Aturan harness: commit/push hanya kalau user memintanya eksplisit. Prompt
  `/goal` tidak memintanya.
- Working tree dibiarkan **siap commit**: backend batch 2026-08-26 sudah
  terverifikasi (§2), perbaikan frontend §3c build+lint lulus. Saran urutan
  saat Anda bangun & setuju:
  1. `openapi.yaml` + Postman regen (TODO-3), lalu commit backend batch
     2026-08-26 (kemungkinan pesan: `feat(fase3): robustness CWMP + rules
     engine ZTP + firmware canary rollout`).
  2. Uji frontend §3c di browser (`npm run dev`), lalu commit terpisah
     (`feat(fase3): UI sync rules-engine ZTP + halaman firmware rollout`).
- File baru non-kode yang belum di-track: `PROGRESS.md` (ini),
  `ACS_Arsitektur_dan_Konfigurasi_Guide_Lengkap.pdf` (dokumen referensi yang
  Anda taruh — bukan buatan sesi ini; pertimbangkan `.gitignore` atau `docs/`).

---

## 5. TODO & temuan

### TODO-1 — (SELESAI sesi ini) Verifikasi backend batch 2026-08-26
`go vet`/`build`/`test` + migrasi 0009–0012 up/down semua lulus. Lihat §2b/§2c.

### TODO-2 — BUG pre-existing: `migrations/0006_task_metrics_indexes.down.sql`

`migrate down` (revert penuh) gagal:
```
ALTER TABLE tasks DROP KEY idx_tasks_status, DROP KEY idx_tasks_completed_at;
-> Error 1553: Cannot drop index 'idx_tasks_status': needed in a foreign key constraint
```
Penyebab: saat `0006` up menambah `idx_tasks_status` pada `tasks.task_status_id`,
MariaDB menghapus index implisit yang tadinya dibuat untuk FK
`fk_tasks_status` (dianggap redundan). Kini `idx_tasks_status` satu-satunya
index yang melayani FK itu, jadi `DROP KEY` di down-migration ditolak.
Kemungkinan `idx_device_sessions_status` (FK `device_sessions.device_id`? —
perlu dicek persisnya) kena pola yang sama.

- **Dampak nyata: rendah.** `migrate up` tidak terpengaruh sama sekali;
  `migrate down` penuh bukan operasi produksi (menghapus semua tabel).
- **Bukan dari batch 2026-08-26** (migrasi 0006 di-commit 2026-08-23,
  `92a7ca8`).
- **Fix yang disarankan** (butuh siklus validasi sendiri): di
  `0006_*.down.sql`, `DROP FOREIGN KEY` dulu → `DROP KEY` → `ADD FOREIGN KEY`
  lagi (yang otomatis membuat ulang index implisit); ATAU biarkan
  `idx_tasks_status`/`idx_device_sessions_status` tidak di-drop di down (index
  ekstra yang tidak berbahaya). Konfirmasi dulu pendekatan mana.

### TODO-3 — Postman collection regen (openapi.yaml SUDAH diupdate)

- **`openapi.yaml` (root): SUDAH disinkronkan sesi ini** — schema `ZeroTouchRule`
  + `ZeroTouchRuleRequest` + `VendorParameterMapping` +
  `UpsertVendorParameterMappingRequest` diperluas; ditambah 5 endpoint
  `/firmware/rollout-batches*` + schema `FirmwareRolloutBatch` /
  `CreateRolloutBatchRequest` / `FirmwareRolloutBatchListResponse` + param
  `rolloutBatchIdPath`. **`npx @redocly/cli lint openapi.yaml` → valid** (3
  warning pre-existing: info-license, server url localhost, /metrics tanpa 4xx —
  bukan dari perubahan ini).
- **`ACS-API.postman_collection.json`: BELUM diregen** — di-generate dari
  `openapi.yaml` via `npx openapi-to-postman`, TAPI koleksi existing punya
  kustomisasi (auth level-collection, script auto-capture `access_token`,
  variabel `loginUsername`/`loginPassword`) yang regen mentah akan hilangkan.
  Regen + re-apply kustomisasi itu (atau pakai opsi generator yang
  mempertahankannya) sebaiknya dilakukan bareng keputusan commit, bukan
  otomatis semalam.
- **Drift pre-existing yang ikut terlihat** (bukan dari sesi ini): schema
  `DeviceDiagnostic.result` / `Task.parameters` / `Task.response` di
  `openapi.yaml` masih dideskripsikan sbg base64 `[]byte`, padahal batch
  2026-08-23 sudah mengubahnya jadi `domain.JSONRawMessage` (emit JSON
  verbatim). Perlu dikoreksi saat regen.

### TODO-4 — Uji visual frontend §3c

`npm run dev`, lalu:
- Provisioning > Zero-Touch Rules: buat rule tiap kombinasi trigger; rule
  reboot-only tanpa profil; rule dengan match-parameter.
- Firmware > tab Rollout Batch: buat batch, cek progres wave, Advance, Batalkan.
- Catalog: parameter mapping dengan `software_version_pattern`.

### TODO-5 — (dari ROADMAP, TIDAK disentuh sesi ini — sengaja)

Item Fase 3 yang keputusan desainnya belum final per `CLAUDE.md` (STUN/CGNAT,
strategi partisi tabel log, tuning retry/backoff berbasis data produksi) dan
yang butuh hardware (uji kompatibilitas CPE fisik). TR-369/USP juga tidak
disentuh (keputusan arsitektur besar — butuh konfirmasi: WebSocket vs MQTT,
model sesi USP).

---

## 6. Catatan lingkungan (Docker)

Docker Desktop tidak stabil sepanjang paruh pertama sesi: `error waiting for
container: unexpected EOF`, `500 Internal Server Error` di
`//./pipe/dockerDesktopLinuxEngine`, dan `go mod download` mengembalikan
checksum korup (proxy mengembalikan body error identik untuk beberapa modul).
Dicoba berturut-turut: recreate volume cache Go, legacy builder,
`docker desktop restart` (gagal — proses tak mau berhenti). **Yang berhasil:**
`Stop-Process -Force` semua proses Docker Desktop + `wsl --shutdown` +
relaunch `Docker Desktop.exe`. Setelah itu engine sehat (`Server 29.5.3`) dan
semua verifikasi di §2 berhasil dijalankan.

Volume Go cache yang dibuat sesi ini: `acs-go-mod-cache`, `acs-go-build-cache`
(aman ditinggal; ada juga `acs-gocache`/`acs-gomod` dari sesi lama). Semua
container/network throwaway migrasi sudah dihapus; volume dev persisten
(`acs_acs-mariadb-data`, `acs_acs-minio-data`) TIDAK disentuh.
