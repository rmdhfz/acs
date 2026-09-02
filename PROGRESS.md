# PROGRESS.md — Log Sesi Kerja Otonom

---

## SESI 2026-09-02 (`/goal`: lanjutkan proses pembuatan acs kita)

### Kondisi saat sesi dimulai

Working tree bersih, di branch `dev` (`d6ae42c`). Tidak ada Go toolchain di
host (verifikasi backend butuh Docker — tidak dijalankan sesi ini). Node 24 +
`npx @redocly/cli` + `npx openapi-to-postmanv2` tersedia.

### DIKERJAKAN: sinkronisasi `openapi.yaml` + regen Postman (debt yang dicatat 3 sesi berturut-turut)

`openapi.yaml` sudah tertinggal jauh dari `router.go` — 31 operation di 22
path item tidak terdokumentasi sama sekali. Ditutup sesi ini (dokumentasi
murni, **tidak menyentuh kode Go**):

- **Path baru ditambahkan** (semua 1:1 dengan `internal/delivery/http/router.go`):
  - `GET /auth/oidc/login`, `GET /auth/oidc/callback` (alur OIDC opsional)
  - `GET /ws` (upgrade WebSocket push event)
  - `GET /cwmp/sessions/count` (tag baru **Sessions**)
  - `POST /devices/:id/connection-request`, `GET|POST /devices/:id/config-snapshots`
  - **Webhooks** (migrations/0013): `POST|GET /webhooks`,
    `GET /webhooks/deliveries/failed-count`, `GET|PATCH /webhooks/:id`,
    `POST /webhooks/:id/test`, `GET /webhooks/:id/deliveries`
  - **Files** (migrations/0018): `POST|GET /files`, `DELETE /files/:id`
  - **Tags** (migrations/0019): `POST|GET /tags`, `DELETE /tags/:id`,
    `GET|POST /devices/:id/tags`, `DELETE /devices/:id/tags/:tagId`
  - **Presets** (migrations/0019): `POST|GET /presets`, `PATCH|DELETE /presets/:id`
  - **SelfService** (migrations/0020): `GET /self-service/devices`,
    `GET /self-service/devices/:id`, `PATCH /self-service/devices/:id/wifi`,
    `POST /self-service/devices/:id/reboot`
- **Param baru**: `GET /devices?tag_id=` (filter EXISTS ke `device_tags`).
- **Schema baru**: `CountResponse`, `MessageResponse`, `DeviceConfigSnapshot`(+List),
  `WebhookSubscription`(+List), `CreateWebhookRequest`, `UpdateWebhookRequest`,
  `WebhookDelivery`(+List), `GenericFile`(+List), `UploadFileRequest`,
  `PageMeta`, `Tag`(+List), `CreateTagRequest`, `AssignDeviceTagRequest`,
  `Preset`(+List), `CreatePresetRequest`, `UpdatePresetRequest`,
  `SelfServiceDevice`(+List), `ChangeMyWiFiRequest`. 6 parameter path baru.
- **Drift lama ikut diperbaiki** (TODO-3 sesi 2026-08-27): `Task.parameters`/
  `Task.response` + `DeviceDiagnostic.result` masih dideskripsikan `[]byte`
  base64 di spec — padahal sejak 2026-08-23 sudah `domain.JSONRawMessage`
  (emit JSON verbatim). Diperbaiki jadi `type: object`. Enum role `User`/
  `CreateUserRequest`/`ReplaceUserRolesRequest` + `ENDUSER`.
- **Validasi**: `npx @redocly/cli lint openapi.yaml` → **valid**, 6 warning
  (3 pre-existing: info-license, server-url localhost, `/metrics` tanpa 4xx;
  3 baru & memang wajar: `/auth/oidc/*` balas 302, `/ws` balas 101 — semua
  bukan 2xx by design). 100 operation, semua 31 operationId baru terverifikasi
  hadir. **BUKAN validasi runtime** — tidak ada stack live sesi ini.
- **`ACS-API.postman_collection.json` diregen** dari `openapi.yaml` via
  `openapi-to-postmanv2` (folderStrategy=Paths, requestNameSource=Fallback) —
  auto-sinkron, `_postman_id` lama dipertahankan supaya diff minimal. 100
  request (dari 64). Style sama dgn koleksi lama (auth bearer `{{bearerToken}}`
  level-collection, var `baseUrl`). Diff besar (~25k baris) murni karena versi
  generator berbeda dari yang membuat koleksi lama (urutan field), bukan
  perubahan semantik.

### TEMUAN (tidak diperbaiki — dicatat)

- **`deleteWebhook` = dead code**: handler `internal/delivery/http/webhook_handler.go`
  `deleteWebhook` ada tapi **tidak pernah di-route** di `router.go` (tidak ada
  `DELETE /webhooks/:id`). Sengaja TIDAK didokumentasikan di `openapi.yaml`
  (spec ikut route, bukan handler). Follow-up: hapus handler-nya atau tambahkan
  route-nya — keputusan produk.
- **CI workflow masih hilang** (temuan sesi 2026-08-31 belum ditindaklanjuti) —
  `.github/` tidak ada. Konten CI = ranah user.

### BELUM (lanjutan `/goal`)

- Verifikasi backend (`go build/vet/test`) + live smoke belum dijalankan sesi
  ini (butuh Docker). Perubahan sesi ini dokumentasi murni, risiko regresi nol.

---

## SESI 2026-09-02 (2) (`/goal`: apakah sistem kita sudah lebih baik/lengkap dari GenieACS?)

### Audit jujur — verdict: BELUM secara keseluruhan

Audit kode langsung (bukan baca ROADMAP). 12 dimensi: **6 unggul** (UI/UX,
multi-tenancy, dok API, skema, keamanan, rollout firmware), **3 setara**
(cakupan RPC CWMP, observability, NAT), **3 tertinggal** (ekspresivitas
provisioning, USP, **bukti lapangan**). Penahan utama klaim "lebih baik":
sistem **belum pernah menerima Inform dari CPE fisik** (hanya simulasi 1
BOOTSTRAP), belum ada CI, load test terbatas rate limiter.

### Koreksi status dokumen (kode ≠ klaim)

- **STUN/TR-111 SUDAH ADA** (`device/stun_client.go` + fallback di
  `device/service.go`) — ROADMAP menandainya "belum dikerjakan". Untested ke
  CGNAT nyata. → ROADMAP dikoreksi jadi `[~]`.
- **Parameter-tree browser SUDAH ADA** (`ParameterTree.tsx` — expand/edit/
  AddObject/DeleteObject/filter) — PROGRESS lama mencatatnya gap terbuka.
- Sebaliknya, **preset engine = STUB**: tabel `presets` + `/presets` +
  `PresetsPage` ada, tapi `preset.Service` cuma CRUD — tak ada evaluator di
  `session/service.go`. Fitur tampak jadi padahal kosong.
- **USP `HandleMessage` = no-op** (`usp_session/service.go` — `// TODO`),
  state in-memory. Efektif belum ada.
- `TagsPage.tsx` ada tapi tak ter-route; nav "My WiFi" (`/self-service`)
  menunjuk route yang tak ada (portal ENDUSER tak ada halaman).

### DIKERJAKAN sesi ini (gap terjangkau)

- **`.github/workflows/ci.yml` DIPERKUAT** — **KOREKSI: file ini TIDAK hilang.**
  Ada di `main` + `dev` + `origin/dev` sejak baseline `6adf25a`
  (`git ls-tree -r main --name-only` mengonfirmasi). Klaim "CI workflow hilang"
  di PROGRESS sesi 2026-08-31 + commit `d6ae42c` **SALAH** — entah `git ls-tree`
  saat itu keliru dijalankan atau salah baca. Yang dikerjakan sesi ini:
  memperkuat file yang sudah ada — tambah cek `gofmt`, `go test -race`, job
  `openapi` (`@redocly/cli lint`), trigger branch `dev` (tadinya cuma `main`),
  `concurrency` cancel-in-progress, cache npm via `package-lock.json`. Belum
  diverifikasi jalan di GitHub Actions — struktur standar.
- **`TagsPage` di-route** (`/tags`) + item nav "Tags" (ADMIN). Halaman sudah
  lengkap sejak dulu, cuma tak pernah disambungkan.
- **`ParameterTree` root TR-098/TR-181** — `handleGlobalRefresh` tadinya
  hardcode `InternetGatewayDevice.` → device TR-181 (Nokia dll) tak bisa
  "Refresh Root". Sekarang `rootPrefix` diturunkan dari parameter device yang
  sudah ter-sync (fallback TR-098). Sesuai CLAUDE.md "resolve data model milik
  device, jangan diasumsikan".
- **`DELETE /webhooks/:id` di-route** — handler `deleteWebhook` +
  `WebhookSubscriptionRepository.SoftDelete` + hook FE `useDeleteWebhook` +
  tombol "Hapus" di `WebhooksPage` SEMUA sudah ada, tapi route-nya tidak
  pernah didaftarkan → tombol Hapus webhook di UI selama ini gagal. +entri
  `openapi.yaml` + Postman diregen (100→101 op).
- Verifikasi: FE `npm run build` + `oxlint src` **hijau** (hanya warning
  pre-existing). Backend (`router.go`, `openapi.yaml`) **belum diverifikasi
  compile** — tanpa Go toolchain di host; perubahannya kecil & mekanis
  (1 baris route memakai handler yang sudah ada).

### DIKERJAKAN (lanjutan) — Portal self-service ENDUSER (frontend)

Backend `/self-service/*` sudah lengkap sejak batch 2026-08-29 tapi TIDAK ada
halaman/route/hook — nav "My WiFi" menunjuk ke ketiadaan. Dibuat sesi ini
(tanpa perubahan backend):
- `SelfServicePage.tsx` — kartu per device (serial/model/firmware/online),
  form "Ubah WiFi" (SSID/passphrase/band, validasi klien 8–63 char WPA-PSK),
  tombol "Restart" dgn konfirmasi. Theme-aware (`dark:` variants).
- Hook `useMyDevices`/`useChangeMyWiFi`/`useRebootMyDevice` + tipe
  `SelfServiceDevice` di `types.ts`.
- Route `/self-service` + `HomeRedirect`: role ENDUSER murni diarahkan ke
  `/self-service` (bukan `/dashboard` yg akan 403); role lain tetap ke
  dashboard. `path="*"` sekarang ke `/` (biar ikut logika redirect).
- Verifikasi: `tsc -b --force` bersih, `npm run build` exit 0, `oxlint src`
  hanya warning pre-existing. **Belum diuji browser** (tidak ada tool browser,
  konsisten seluruh kerja FE sesi lain).

GenieACS tidak punya portal end-user sama sekali — ini diferensiator, bukan
sekadar parity.

### BELUM — butuh keputusan/akses user (lihat ROADMAP §"Gap jujur pasca-audit")

- Uji lapangan CPE fisik (butuh hardware).
- Engine preset: butuh keputusan desain (bahasa precondition, bentuk
  configurations, kapan dievaluasi) — jangan implementasi sepihak.
- Load test multi-IP sungguhan (butuh infra).

---

## SESI 2026-08-31 (`/goal`: lanjutkan pembuatan ACS sampai lebih bagus dari GenieACS)

### Kondisi saat sesi dimulai

Working tree = batch besar sesi 2026-08-29 masih **belum di-commit** (48 file
berubah, +732/−313) plus file untracked (USP, selfservice, stun_client,
migrasi 0020, RUNBOOK/LOGIN_INSTRUCTION, deploy/grafana|alertmanager).
Tidak ada Go toolchain di host — verifikasi lewat Docker (`golang:1.26` mount).

**Verifikasi baseline seluruh tree (termasuk untracked) — SEMUA HIJAU:**
- `go build ./...` ✅ · `go vet ./...` ✅ · `go test ./...` ✅ · `gofmt -l` bersih
- `frontend/ npm run build` ✅ (hanya warning chunk-size)
- **Live smoke `docker compose up` ✅** (item "BELUM" sesi lalu — sekarang
  ditutup): MariaDB 10.11 + migrate 0001→0020 bersih ("migrate up: selesai"),
  MinIO + Redis healthy, `acsd` boot bersih (REST :8080, CWMP :7547, redis
  connected). Login superadmin (`seed-admin`), smoke `GET /devices/stats`,
  `/tasks/stats`, `/tenants`, `/refs/*`, `/api/v1/metrics` — semua 200 & data
  wajar.

### TEMUAN + FIX: sesi CWMP "OPEN" menumpuk selamanya (robustness gap vs GenieACS)

Saat smoke, `GET /api/v1/cwmp/sessions/count` mengembalikan **348** — padahal
tidak ada sesi in-flight. Sebabnya: 348 baris `device_sessions.status='OPEN'`
sisa loadtest 2026-08-26 yang **tidak pernah ditutup** (CPE/loadtest berhenti
tanpa mengirim POST-kosong penutup). Tidak ada mekanisme apa pun yang
membersihkannya → metrik `acs_cwmp_sessions_open` (dashboard "Sesi Aktif")
naik permanen dan jadi tak berguna. GenieACS punya session timeout; ACS ini
belum.

**Fix — session reaper periodik (backend-only, tanpa migrasi):**
- `domain.SessionStatusTimeout = "TIMEOUT"` (status baru, dibedakan dari
  `ERROR` = fault protokol eksplisit). Kolom `device_sessions.status` sudah
  `VARCHAR(16)` bebas (bukan ENUM/ref_* — konsisten dgn OPEN/CLOSED/ERROR yang
  sudah ada di kolom yang sama), jadi **tidak perlu migrasi** — hanya komentar
  kolom di `schema.sql` diperbarui.
- `domain.DeviceSessionRepository.TimeoutStaleOpen(ctx, olderThan)` +
  implementasi MySQL (`UPDATE ... SET status='TIMEOUT', ended_at=NOW() WHERE
  status='OPEN' AND started_at < ?`) + passthrough di redisrepo (dgn komentar
  jujur soal entri cache Redis sesi yg di-reap: tidak diinvalidasi per-key,
  aman krn TTL 1 jam + guard `Status==OPEN` di resolveSession/NextRequest).
- `session.Service.TimeoutStaleSessions(ctx, threshold)` — orkestrasi tipis
  (hitung cutoff, log bila >0).
- `cmd/acsd` `runSweepers`: panggil tiap tick 1 menit, **ambang 15 menit**
  (jauh di atas durasi sesi CWMP normal detik–menit, tapi cukup cepat supaya
  metrik akurat). Mengikuti pola `taskSvc.TimeoutStaleSent` yang sudah ada.
- Test unit `TestTimeoutStaleSessions` (`session/service_test.go`): reap yang
  basi, jangan sentuh sesi <15 menit / yang sudah CLOSED, dan cutoff yang
  diteruskan ke repo ~15 menit lalu (bukan "sekarang").

**Validasi:**
- `go build`/`vet`/`test ./...` ✅, `gofmt` bersih.
- SQL reaper dijalankan langsung ke MariaDB live: 348 baris OPEN → TIMEOUT,
  `GET /cwmp/sessions/count` → `{"count":0}`, metrik `acs_cwmp_sessions_open 0`.
- End-to-end jalur Go **✅**: insert 1 baris sintetis `OPEN` (started_at
  −40 mnt), rebuild image `acsd` + restart, tick sweeper pertama (02:28:29,
  ~1 mnt setelah boot) mengubah baris jadi `TIMEOUT` — log `{"msg":"reap sesi
  CWMP basi","count":1,"threshold":"15m0s"}` lalu `sweeper: 1 sesi CWMP basi
  di-reap`. Boot bersih, tanpa panic.

### TEMUAN + FIX (2): cache Redis sesi CWMP tidak pernah diinvalidasi → namespace/status basi

Saat validasi live ketahuan: `internal/repository/redisrepo/device_session_repository.go`
meng-cache objek sesi saat `Create` (TTL 1 jam) **tapi tidak pernah
memperbarui/menghapusnya** saat `UpdateStatus`/`SetCWMPID`/`SetCWMPNamespace`.
Akibatnya (saat Redis aktif — default di compose):
- **`SetCWMPNamespace` (migrations/0012) efektif tak berguna**: `NextRequest`
  membaca salinan cache pra-set → `sess.CWMPNamespace == nil` → RPC proaktif
  dikirim dgn namespace default, justru bug yang migrations/0012 perbaiki.
- Sesi yang sudah `CLOSED`/`TIMEOUT` di DB tetap tampak `OPEN` dari cache s/d
  1 jam → `resolveSession` bisa "menyambung" ke sesi yang secara logis mati
  bila CPE mengirim ulang cookie lama.

**Fix** (`redisrepo`, tanpa dependensi baru):
- Indeks balik `acs:cwmp:session:byid:<id> → token` ditulis bersama entri
  token, supaya mutator yang cuma punya `id` bisa menemukan & `DEL` entri
  cache-nya. Semua mutator (`UpdateStatus`/`SetCWMPID`/`SetCWMPNamespace`)
  kini invalidasi setelah tulis MySQL.
- `GetByToken` mengisi ulang cache pada fallback MySQL (baca berikutnya dalam
  sesi yang sama kembali kena hot path).
- TTL cache 1 jam → **15 menit** (= ambang reaper): membatasi jendela basi
  untuk jalur bulk `TimeoutStaleOpen` yang tidak lewat invalidasi per-key.
- **Validasi live** (real Redis di compose): loadtest 3 sesi CWMP →
  `cwmp_namespace` = `urn:dslforum-org:cwmp-1-2` tersimpan di DB, sesi
  `CLOSED`, dan **0 key `acs:cwmp:session:*` tersisa di Redis** (semua
  terinvalidasi).

### TEMUAN + FIX (3): `cmd/loadtest` tidak pernah menutup sesi (POST penutup tanpa cookie)

`runSession` membuat `closeReq` (POST kosong penutup) **tanpa membawa cookie
sesi** dari InformResponse (`http.Client` tanpa cookie jar). ACS tak bisa
meresolve sesi → tiap sesi loadtest tertinggal `status=OPEN` selamanya. **Ini
akar 348 baris OPEN basi** dari run loadtest 2026-08-26 (klaim ROADMAP "sesi
ditutup bersih via loadtest" tidak pernah benar untuk jalur ini — verifikasi
close dulu pakai curl manual yang memang bawa cookie). Fix: teruskan
`resp.Cookies()` ke `closeReq` secara manual (bukan cookie jar bersama, supaya
sesi antar-goroutine tidak tercampur). **Validasi**: loadtest 3 sesi → semua
`CLOSED` (sebelum fix: `OPEN`).

### FITUR: device ↔ tag (segmentasi gaya GenieACS) — dilengkapi + celah authz ditutup

Fitur tag setengah jadi: tabel `tags`/`device_tags` (migrations/0019) + endpoint
`POST/GET/DELETE /tags` ada, **tapi tidak ada cara menempelkan tag ke device**
(`tag.Service.AssignToDevice/RemoveFromDevice/ListByDevice` ada tapi **tak
pernah di-route** — dead code) dan **tidak bisa memfilter device by tag**. Tag
praktis tidak berguna di produk. Selain itu `AssignToDevice` punya komentar
`// Pengecekan otorisasi tenant diabaikan sementara` — operator tenant A bisa
menempelkan tag tenant B.

**Yang dikerjakan (backend-only, tanpa migrasi):**
- 3 route baru: `GET /devices/:id/tags`, `POST /devices/:id/tags` (body
  `{tag_id}`), `DELETE /devices/:id/tags/:tagId` — assign/remove `adminOrNOC`,
  GET semua role terautentikasi (pola sama `GET /devices`).
- **Celah authz ditutup**: handler cek kepemilikan device lewat
  `Devices.Get(actor, id)` (sudah enforce `RequireTenantScope`); `tag.Service`
  cek sisi tag lewat `requireTagInTenantScope` (tag global `tenant_id=NULL`
  boleh; tag tenant lain → `ErrForbidden`). Komentar "diabaikan sementara"
  dihapus.
- Filter `GET /devices?tag_id=N` — `EXISTS (SELECT 1 FROM device_tags …)` di
  `deviceRepository.List` (tetap tenant-scoped: filter tag ∧ tenant).
- Test unit `internal/usecase/tag/service_test.go` (baru — package ini tadinya
  tanpa test): assign/remove tag tenant sendiri OK, tag global OK, tag tenant
  lain → `ErrForbidden` & repo tidak disentuh, superadmin bebas.
- **Validasi live**: create tag → assign ke device 360 (204) →
  `GET /devices/360/tags` tampil → `GET /devices?tag_id=1` → device 360
  (total 1) → `?tag_id=999` → total 0 → remove (204) → `?tag_id=1` → total 0.
- **Frontend (build+lint hijau, belum diuji browser — konsisten sesi FE lain)**:
  `hooks.ts` `useDeviceTags`/`useAssignDeviceTag`/`useRemoveDeviceTag` +
  `tag_id` di `DeviceFilters`; `DevicesPage` dropdown filter "Semua tag"
  (muncul hanya bila ada tag); `DeviceDetailPage` `DeviceTagsBar` di bawah
  kartu ringkasan — chip tag berwarna, tombol × (ADMIN/NOC), dropdown
  "+ Tambah tag…". `npm run build` (tsc+vite) & `oxlint` bersih (0 warning
  dari file yang disentuh).

### BELUM (lanjutan `/goal`)
- Batch besar belum di-commit (menunggu review user). Kandidat urutan commit
  di §PROGRESS sesi 2026-08-29 masih berlaku; fix sesi 2026-08-31 relatif
  kecil & berdiri sendiri (session reaper + cache Redis + loadtest cookie +
  device↔tag).
- **`openapi.yaml` makin tertinggal**: sudah tidak punya `/tags`, `/presets`,
  `/files`, `/self-service` (batch 2026-08-29), sekarang + 3 route device↔tag
  & param `?tag_id=`. Regen bareng keputusan commit batch besar.
- Item robustness lain vs GenieACS masih terbuka: USP/TR-369 masih mock,
  parameter-tree browser UI, device search expression language (baru `tag_id`
  + filter dasar).
- **Push ke `origin/dev`** (permintaan user 2026-08-31: "push semuanya ke branch
  dev, fokus di dev sebelum masuk main"). `main` TIDAK disentuh (tetap di
  `9339f3c`, ahead 4 dari `origin/main`). `dev` = `origin/main` + 4 commit
  main yang belum ter-push + `wip(dev)` (batch 2026-08-29 + fix backend
  2026-08-31) + `feat(dev)` (UI tag).

### TEMUAN: tidak ada CI workflow di repo

`ROADMAP.md` Fase 0 mengklaim `.github/workflows/ci.yml` dibuat & "jalan hijau
di GitHub 2026-08-22" (dengan link run). **Realita: `.github/` tidak ada di
disk maupun di tree ref manapun** (`git ls-tree main`, `HEAD` — kosong). Entah
tak pernah di-commit atau terhapus. Tidak diblokir apa pun sekarang, tapi
artinya tidak ada gerbang otomatis di `origin/dev`/`origin/main`. Belum
dibuat ulang sesi ini (keputusan konten CI = ranah user).

> **[KOREKSI 2026-09-02]** Temuan di atas **SALAH**. `.github/workflows/ci.yml`
> ADA dan tracked di `main`, `dev`, dan `origin/dev` sejak commit baseline
> `6adf25a` — diverifikasi ulang dengan `git ls-tree -r main --name-only`
> dan `git show HEAD:.github/workflows/ci.yml`. Kemungkinan `git ls-tree` saat
> sesi 2026-08-31 dijalankan dari worktree/ref yang salah. Sesi 2026-09-02
> memperkuat file yang sudah ada (gofmt, `-race`, job openapi, trigger `dev`).

---

## SESI 2026-08-29 (`/goal`: lengkapi API tandingi GenieACS + UI/UX + audit + API log + integrasi FTTH)

### TEMUAN KRITIS: `main` (HEAD `9339f3c`) TIDAK BISA BUILD & migrasi TIDAK BISA JALAN

Bertentangan dengan klaim "divalidasi live" di seluruh `ROADMAP.md`. Kondisi
nyata working tree + HEAD saat sesi ini mulai:

1. **`go build ./...` GAGAL** — commit `6b114c9` ("complete phase 5 and 6")
   men-*commit* kode yang tidak pernah dikompilasi:
   - `go.sum` tidak punya entry `github.com/coreos/go-oidc/v3` (OIDC di-`require`
     tanpa `go mod tidy`).
   - `internal/usecase/session/service.go`: `detectAnomalyBoot`/`detectAnomalyDNS`
     pakai `s.activity` (field tidak ada), `dev.LastBootTime` (nama salah,
     harusnya `LastBootEventAt`), `strings` tidak diimpor, var `bootCount`/
     `cutoff` tidak dipakai, `s.taskSvc.EnqueueGetParameterNames` tidak ada.
     Logika `detectAnomalyBoot` juga **salah** (baca `LastBootEventAt` SETELAH
     `TouchLastBootEvent` menimpanya → setiap BOOT dianggap "reboot loop").
   - `internal/delivery/http/`: `file_handler.go`/`tag_preset_handler.go`/
     `ws_handler.go`/`selfservice_handler.go` pakai `parsePagination`,
     `c.PathParam`, `ResolveActor`, `echo.Context` (non-pointer) — semua nama/
     tipe yang tidak ada di codebase ini (echo v5 pakai `*echo.Context`,
     helper-nya `paginationFromQuery`/`parseUint64Param`/`ActorFrom`).
   - `router.go`: `session` tidak diimpor padahal field `Sessions *session.Service`.
   - `EventPublisher.BroadcastToTenant(int64)` vs pemanggil kirim `uint64`.
   - `domain.Task` tak punya field type-code, `t.TaskType` dipakai di task svc.
   - `domain.ActivityLog.Details` dipakai di device svc, field tidak ada.
   - `webhook.Service.CountFailedDeliveries` pakai `actor.Role` (harusnya `Roles`).
   - `main.go`: `task.NewService`/`session.NewService` argumen kurang;
     `tag.Service`/`preset.Service` **tidak pernah dikonstruksi/di-wire** ke
     Router (handler-nya nil-panic saat runtime).
   - Test `auth`/`webhook`/`task` **tidak kompilasi** (signature drift + 1 file
     korup: `gotالسig` — nama variabel tercampur aksara Arab di
     `webhook/service_test.go`).
2. **`migrate up` GAGAL** — 4 pasang migrasi bertabrakan nomor
   (`0013`,`0014`,`0015`,`0016` masing-masing dua file) → golang-migrate
   menolak "duplicate migration version".
3. **`schema.sql` basi** — 6 objek dari migrasi 0014–0020 tidak ada di `schema.sql`.
4. **`migrations/0006_*.down.sql` rusak** (pre-existing, TODO-2 lama) —
   `DROP KEY idx_tasks_status` ditolak errno 1553 (dibutuhkan FK).
5. **`migrations/0020_user_devices_mapping` (untracked) rusak** —
   `INSERT INTO ref_roles (id, name, description)` padahal `ref_roles` tak punya
   kolom `description` & `code` NOT NULL.

### YANG DIPERBAIKI SESI INI — tree sekarang HIJAU (divalidasi via Docker)

- **Renumber migrasi**: `0014_performance_indexes`→`0017`,
  `0015_generic_files`→`0018`, `0016_tags_and_presets`→`0019`,
  `0013_user_devices_mapping` (untracked)→`0020`. Rantai kini linear 0001–0020.
- **`schema.sql` disinkronkan** — `device_config_snapshots`, kolom
  `devices.latitude/longitude`, `files`, `tags`/`device_tags`/`presets`,
  `user_devices`, role `ENDUSER`, index performa 0017. Diverifikasi:
  `migrate up` (chain) dan `schema.sql` menghasilkan set tabel + kolom
  **identik** (diff information_schema, MariaDB 10.11 nyata).
- **`migrate` divalidasi via `cmd/migrate` (golang-migrate) nyata**: up 0→20
  bersih, down 20→0 bersih (setelah fix 0006 down: `DROP FK` → `DROP KEY` →
  `ADD FK`), re-up 0→20 bersih.
- **Semua error kompilasi di atas diperbaiki** — `go build ./...`, `go vet ./...`,
  `go test ./...` semua LULUS (image `golang:1.26`); `gofmt` bersih; frontend
  `npm run build` LULUS.
- **`detectAnomalyBoot` logika diperbaiki** — bandingkan boot-time SEBELUM vs
  SESUDAH, ambang <15 menit, guard nil `activity`.
- **Modul self-service pelanggan diselesaikan** (sebelumnya stub kosong):
  `internal/domain/selfservice.go` (`UserDeviceRepository`, `SelfServiceWiFiChange`),
  `internal/repository/mysql/user_device_repository.go`,
  `internal/usecase/selfservice/service.go` (ownership-check, bukan RBAC tenant;
  `ListMyDevices`/`GetMyDevice`/`ChangeMyWiFi`/`RebootMyDevice`), handler
  `/api/v1/self-service/*` di-wire penuh ke `main.go`.
- **`tag.Service`/`preset.Service` di-wire** ke Router (sebelumnya nil).
- **`domain.Task.TaskTypeCode`** ditambah (JOIN di `GetByID`), enrich response API.
- **`ActivityLog.Details`** (map konteks) dilipat jadi JSON di kolom `description`
  oleh repo bila description kosong.

### BELUM (lanjutan `/goal` sesi ini):
- API request log persisten (tabel + middleware + endpoint + UI).
- Analisa gap API vs GenieACS + isian.
- Frontend: halaman self-service, halaman tag/preset, wiring fitur baru.
- USP/TR-369 masih level mock (`internal/delivery/usp`, `pkg/usp`) — kompilasi OK,
  di-mount di `:7547/usp`, tapi `HandleMessage` belum route apa-apa. Perlu
  keputusan arsitektur (lihat TODO-5 lama).
- Live smoke `docker compose up` (boot acsd) belum dijalankan sesi ini.
- Belum di-commit (menunggu review; kandidat: branch `fix/build-and-migration-repair`).

---


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
