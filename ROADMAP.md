# ROADMAP.md — Peta Jalan: ACS Group-Grade, Melampaui GenieACS

Dokumen tracking hidup (living document) untuk membawa ACS ini dari "kerangka arsitektur solid tapi belum teruji" menjadi platform yang benar-benar lebih baik dari GenieACS dari sisi UI/UX, dan layak dipakai oleh **Group** (10+ perusahaan, berbagai vendor CPE) sebagai satu platform bersama.

Update dokumen ini setiap kali sebuah item selesai atau prioritas berubah — jangan biarkan basi. Lihat `PRD.md` untuk requirement produk dan `TECH.md` untuk keputusan arsitektur; dokumen ini adalah **rencana eksekusi**, bukan pengganti keduanya.

**Terakhir diperbarui:** 2026-08-21

---

## 1. Definisi "Lebih Bagus dari GenieACS" (Success Criteria)

Klaim "lebih bagus" harus bisa diukur. Ini baseline pembanding jujur:

| Dimensi | GenieACS (baseline) | Target ACS ini |
|---|---|---|
| UI/UX | Tabel admin utilitarian, tanpa dashboard analitik, navigasi parameter tree mentah | Dashboard visual (tren redaman optik, status device), navigasi berbasis logical key, bulk actions, responsive |
| Multi-tenancy | Tidak native — perlu workaround manual per instance/namespace | Native (`tenants` table, RBAC scope tenant) — target: 10+ perusahaan dalam 1 platform tanpa saling bocor data |
| Provisioning | Script JS bebas per device (powerful tapi rawan, sulit di-review) | Data terstruktur (`vendor_parameter_mappings`, `provisioning_profiles`) — lebih aman & auditable, dengan raw-path fallback |
| Observability | Minim bawaan | Metrics Prometheus + dashboard + alerting backlog task (TECH.md §10) |
| Auditability | Log dasar | Audit trail 7-kolom + activity log + timeline visual per device |
| Kematangan protokol | Bertahun-tahun teruji di lapangan lintas vendor | **Belum teruji sama sekali ke CPE nyata** — ini gap terbesar, lihat Fase 0 |
| Skala terbukti | Production, ratusan ribu device | **Belum ada bukti** — desain stateless tapi belum load-test |

**Kesimpulan jujur:** kita bisa unggul di UI/UX dan model data dalam waktu dekat. Kita **tidak bisa** mengklaim lebih baik secara keseluruhan sampai Fase 0 (validasi dasar) selesai — jangan promosikan ke Group sebelum itu.

---

## 2. Snapshot Kondisi Saat Ini (2026-08-21)

**Sudah ada:**
- Backend Go lengkap untuk 6 usecase inti (session, task, provisioning, device, firmware, diagnostics) + IAM, seluruh REST API FR-1 s/d FR-28 di PRD.md, CWMP handler.
- Frontend React/Tailwind: Devices, Device Detail (parameter/event/task/diagnostics/firmware), Tasks, Provisioning Profiles & Zero-Touch Rules, Firmware catalog, Catalog Vendor (vendor/model/mapping), Administration (user/tenant).
- `docker-compose.yml` untuk stack dev (MariaDB + migrate + acsd).
- 7 subagent proyek di `.claude/agents/` untuk kerja backend (lihat §4).

**Belum tervalidasi / gap kritis — jangan diabaikan:**
- Belum pernah `go build`/`go vet`/`go test` berhasil dijalankan di environment manapun (Go toolchain tidak ada di mesin dev saat dokumen ini ditulis).
- `docker-compose` belum pernah benar-benar dijalankan — migrasi belum pernah dieksekusi ke MariaDB nyata.
- Frontend belum pernah dibuka di browser sungguhan — hanya lulus `tsc -b` + `vite build`.
- Belum ada CPE sungguhan (atau simulator) yang pernah mengirim Inform ke endpoint CWMP ini.
- Belum ada commit git sama sekali — tidak ada baseline yang bisa di-rollback.
- Tidak ada CI/CD — tidak ada gerbang otomatis sebelum kode masuk.
- `GET /vendors/:id/ouis` tidak ada — Catalog UI tidak bisa menampilkan daftar OUI yang sudah didaftarkan.
- Upload firmware hanya mencatat metadata — belum terhubung ke object storage sungguhan (TECH.md §12, belum diputuskan).
- STUN/CGNAT belum ada (memang Fase 2 di PRD).
- Tidak ada subagent yang meng-cover kerja frontend/UX — gap ini ditutup di §4 (agent baru `frontend-ux-builder`).
- Observability (Prometheus/metrics) disebut di TECH.md §10 tapi belum diimplementasikan sama sekali.

---

## 3. Workstream per Fase

Checklist ini dipecah per fase. **Jangan lompat ke Fase 1/2 sebelum Fase 0 selesai** — UI bagus di atas backend yang belum teruji ke CPE nyata adalah rumah bagus di atas fondasi yang belum dicek.

### Fase 0 — Fondasi & Validasi (prasyarat sebelum klaim apa pun ke Group)

- [x] `go build ./...`, `go vet ./...`, `go test ./...` berhasil lulus — divalidasi 2026-08-21 via image `golang:latest` (build ✅, vet ✅ bersih, test ✅ termasuk skenario retry/max_retries di `internal/usecase/task`)
- [x] `docker-compose up` berhasil — divalidasi 2026-08-21: MariaDB healthy, migrasi `0001_init_schema` jalan bersih ("migrate up: selesai"), `acsd` listening di :8080 (REST) dan :7547 (CWMP)
- [ ] Smoke test manual frontend di browser: login, lihat semua halaman baru (Tasks, Provisioning, Firmware, Catalog, Administration), coba satu alur create/edit di masing-masing — **belum bisa dilakukan Claude** (tidak ada tool browser di environment ini), REST API sudah dicek lewat curl/PowerShell tapi interaksi UI sungguhan butuh dicoba manual oleh user
- [x] Commit awal ke git sebagai baseline — commit `6adf25a`, 2026-08-21 (lokal, belum di-push ke `origin` — konfirmasi dulu ke user sebelum push)
- [x] Setup CI dasar (GitHub Actions) — `.github/workflows/ci.yml` dibuat (job backend: vet+build+test; job frontend: lint+build); **belum divalidasi jalan sungguhan di GitHub** karena belum di-push
- [x] Uji sesi CWMP end-to-end — divalidasi 2026-08-21 dengan simulasi Inform (event `0 BOOTSTRAP`) manual: InformResponse benar, device ter-upsert, event tercatat, sesi ditutup bersih (POST kosong -> 204, `device_sessions.status=CLOSED`)
- [ ] Jalankan `/security-review` menyeluruh sebelum promosi ke fase berikutnya — temuan kritis di bawah **sudah diperbaiki**, review lain (RBAC endpoint lain, isolasi tenant di luar CWMP) belum menyeluruh

**[SELESAI 2026-08-22] Endpoint CWMP TIDAK memvalidasi kredensial sama sekali — diperbaiki.**
Root cause: `internal/delivery/cwmp/handler.go` tidak punya pengecekan Basic/Digest Auth apa pun, padahal `TECH.md` §3/§8 dan `PRD.md` FR-1 mensyaratkannya. Ditemukan dengan mengirim Inform simulasi tanpa header `Authorization` — diterima begitu saja.

**Pendekatan yang dipilih & diimplementasikan: shared secret Inform per tenant** (`tenants.cwmp_inform_username`/`cwmp_inform_password_enc`, migrasi `0002_tenant_cwmp_inform_credentials`), dengan kredensial per-device (`devices.inform_username`) sebagai override opsional setelah device dikenal. Anti-spoofing lintas-tenant: bila device sudah py `tenant_id`, secret HARUS milik tenant yang sama. Superadmin kelola shared secret via `POST /tenants` (saat create) atau `PATCH /tenants/:id/cwmp-credentials` (rotate) — sudah ada UI-nya di Administration > Tenants.

**Bonus fix sekalian:** device yang dibuat dari Inform sekarang otomatis dapat `tenant_id` dari tenant yang match kredensialnya — sebelumnya `FindOrCreateFromInform` TIDAK PERNAH meng-assign tenant_id sama sekali (device baru selalu `tenant_id=NULL`, artinya admin/NOC ber-scope-tenant tidak akan pernah melihat device barunya sendiri — bug fondasional untuk use-case Group).

**Divalidasi end-to-end manual** (docker-compose + MariaDB nyata, bukan cuma unit test) untuk 5 skenario: (1) tanpa auth → 401, (2) shared secret benar + device baru → 200 & tenant_id ter-assign otomatis, (3) password salah → 401, (4) device tenant A coba pakai secret tenant B → 401 (anti-spoofing), (5) device dgn override per-device wajib pakai kredensial override-nya, bukan shared secret tenant → 401 kalau salah. `go build`/`vet`/`test` dan `npm run build` lulus semua setelah perubahan.

**Independent review (`acs-code-reviewer` + `acs-security-reviewer`) atas fix di atas — semua temuan signifikan sudah ditindaklanjuti (2026-08-22):**
- **[KRITIS, DIPERBAIKI]** Device lama/orphan (`tenant_id NULL`) tidak ter-lindungi anti-spoofing DAN tidak pernah "sembuh" — celah re-terbuka tiap Inform. Fix: `FindOrCreateFromInform` sekarang meng-assign `tenant_id` begitu device orphan berhasil Inform dgn kredensial tenant manapun, lalu klaim **terkunci** (tenant lain langsung ditolak sesudahnya) — divalidasi live: device orphan `SIMTEST-0001` sembuh ke `tenant_id=1` pada Inform pertama, lalu klaim dari tenant lain ditolak 401 di percobaan berikutnya.
- **[TINGGI, DIPERBAIKI]** Tidak ada rate limiting di `/cwmp` padahal sekarang menjaga secret sungguhan. Fix: `middleware.RateLimiter` 5 req/s per identifier di `cwmpEcho` — divalidasi live (burst 15 request memicu beberapa `429`).
- **[TINGGI, DIPERBAIKI]** Tidak ada validasi panjang minimum `cwmp_inform_password`. Fix: minimal 16 karakter di `iam.Service` — divalidasi live (password 8 karakter ditolak `400`).
- **[SEDANG, DIPERBAIKI]** Kredensial override per-device tidak ikut tercabut saat tenant pemiliknya dinonaktifkan. Fix: `authenticateInform` sekarang cross-check `tenant.IsActive` juga di jalur override (unit test).
- **[SEDANG, DIPERBAIKI — code review]** Query `devices` duplikat di setiap Inform (hot path). Fix: `authenticateInform` meneruskan device yang sudah di-fetch ke `FindOrCreateFromInform` lewat `ExistingDevice`, bukan query ulang.
- **[SEDANG, DIPERBAIKI — code review]** Logic keamanan baru (`authenticateInform`/`passwordMatches`) tanpa unit test. Fix: `internal/usecase/session/service_test.go` ditambahkan, 11 skenario (anti-spoofing, override wajib, tenant nonaktif, orphan healing, password kosong, dst) — semua lulus.
- **[RENDAH, DITERIMA APA ADANYA]** Timing side-channel minor (durasi respons beda antara username tak dikenal vs password salah) dan perbandingan username tidak constant-time — severity rendah menurut reviewer sendiri, mitigasi penuh butuh dummy-crypto-ops yang menambah kompleksitas tidak sepadan untuk saat ini. Dicatat, tidak diperbaiki.
- **[FOLLOW-UP, bukan blocker]** Tidak ada endpoint REST untuk SET kredensial per-device (`devices.inform_username/inform_password_enc`) — kolom & logic override-nya sudah ada dan tervalidasi jalan lewat unit test + DB manual, tapi baru bisa diisi manual di DB, belum ada jalur API/UI. Tambahkan ke Fase 2 kalau override per-device memang dibutuhkan operasional.
- **[FOLLOW-UP, bukan blocker]** Rate limiting baru ada di `/cwmp`, REST API internal (`restEcho`) masih belum ada — TECH.md §8 mensyaratkan keduanya. Tambahkan ke Fase 0/2.
- **[FOLLOW-UP, bukan blocker]** Tidak ada endpoint untuk menonaktifkan tenant (hanya create/list) — jadi skenario "device override milik tenant nonaktif ditolak" baru tervalidasi lewat unit test, belum lewat REST end-to-end (butuh endpoint `PATCH /tenants/:id` untuk toggle `is_active` dulu).

*Agent terkait: `cwmp-session-engineer` (uji sesi & auth), `db-schema-guardian` (validasi migrasi), `acs-security-reviewer` + `acs-code-reviewer` (review independen), kerja CI/commit dilakukan langsung bersama user.*

### Fase 1 — Diferensiasi UI/UX (bagian yang bikin "lebih bagus dari GenieACS" terasa nyata)

- [ ] Dashboard analitik: tren status online/offline dari waktu ke waktu, grafik redaman optik (RX/TX power) per device dan agregat per tenant — ini yang paling dirasakan tim NOC dibanding GenieACS yang tanpa visualisasi
- [ ] Bulk actions: pilih banyak device sekaligus untuk apply profile / jadwalkan firmware upgrade / reboot massal
- [ ] Provisioning profile builder dengan parameter picker dari `vendor_parameter_mappings` (autocomplete logical key), bukan ketik manual raw path
- [ ] Global search / command palette (serial number, MAC, device UUID) — lintas tenant untuk superadmin, dalam tenant untuk role lain
- [ ] Notifikasi in-app real-time (task gagal, device offline mendadak, backlog task menumpuk) — polling atau websocket
- [ ] Timeline audit trail visual per device (siapa ubah parameter apa, kapan) — dari `activity_logs`
- [ ] Dark mode + aksesibilitas dasar (kontras warna, navigasi keyboard)
- [ ] Tampilan ringkas mobile-responsive untuk teknisi lapangan (device detail minimal: status, redaman optik, tombol reboot)
- [ ] Wizard onboarding tenant baru (buat tenant → admin pertama → pilih vendor default) — memudahkan onboarding tiap perusahaan baru di Group

*Agent terkait: `frontend-ux-builder` (baru, lihat §4), `rest-api-builder` (endpoint agregasi/statistik bila backend belum expose data yang dibutuhkan chart).*

**Belum diputuskan — konfirmasi dulu sebelum dikerjakan:** pilihan library charting (mis. Recharts vs visx vs D3 manual) belum ditentukan; jangan pilih sepihak, tanyakan preferensi (ukuran bundle vs fleksibilitas) sebelum epic pertama di fase ini dimulai.

### Fase 2 — Enterprise / Multi-Tenant untuk Skala Group (10+ perusahaan)

- [ ] White-labeling per tenant (logo, nama produk, warna aksen) di `Layout.tsx`
- [ ] Tenant admin self-service penuh: kelola user & role tenant sendiri tanpa perlu superadmin turun tangan
- [ ] Uji isolasi tenant end-to-end: pastikan tidak ada satu pun query yang lolos tanpa filter `tenant_id` (audit sistematis, bukan spot-check)
- [ ] Kuota/rate limit per tenant — mencegah satu perusahaan menghabiskan resource bersama (task queue, koneksi CWMP)
- [ ] Tambahkan `GET /vendors/:id/ouis` + tampilkan daftar OUI di Catalog UI (gap yang sudah diketahui dari §2)
- [ ] Playbook tertulis "cara menambah vendor baru" (operasi data, bukan kode — TECH.md §5.1) + form approval request vendor baru untuk non-superadmin
- [ ] Putuskan & implementasikan strategi object storage firmware (S3-compatible vs filesystem) — TECH.md §12, perlu keputusan eksplisit dari user dulu
- [ ] Observability: metrics Prometheus + dashboard (device online/offline, task queue depth per status, error rate per vendor) + alert saat backlog menumpuk (TECH.md §10)

*Agent terkait: `vendor-mapping-specialist` (playbook & operasi data vendor), `rest-api-builder` (endpoint baru), `acs-security-reviewer` (audit isolasi tenant), `db-schema-guardian` (bila kuota/rate limit butuh tabel baru).*

### Fase 3 — Robustness Protokol & Skala (menandingi kematangan lapangan GenieACS)

- [ ] Uji kompatibilitas riil terhadap firmware CPE ZTE/Huawei/FiberHome/Nokia (lab test dengan perangkat fisik atau emulator resmi vendor)
- [ ] Connection Request via STUN untuk CPE di belakang CGNAT (Fase 2 PRD)
- [ ] Load test: ribuan sesi CWMP concurrent, ukur p95 latency Inform→InformResponse (target NFR: <300ms)
- [ ] Tuning retry/backoff task berbasis data produksi nyata (bukan asumsi)
- [ ] Strategi partisi tabel log (`device_events`, `device_optical_metrics`) setelah ada data volume produksi (TECH.md §12)
- [ ] Backup/disaster recovery MariaDB terdokumentasi dan **dites lewat restore drill sungguhan**, bukan cuma didokumentasikan

*Agent terkait: `cwmp-session-engineer`, `db-schema-guardian`, `task-queue-test-writer`.*

---

## 4. Cara Pakai Dokumen Ini Bersama Subagent

Subagent proyek ada di `.claude/agents/` (lihat masing-masing file untuk detail aturan):

| Agent | Fase yang paling relevan |
|---|---|
| `db-schema-guardian` | 0, 2, 3 — perubahan skema & migrasi |
| `cwmp-session-engineer` | 0, 3 — protokol & sesi CWMP |
| `vendor-mapping-specialist` | 2, 3 — operasi data vendor/parameter mapping |
| `rest-api-builder` | 1, 2 — endpoint REST baru (auth+RBAC wajib sejak awal) |
| `task-queue-test-writer` | 0, 3 — test retry/max_retries task queue |
| `acs-code-reviewer` | semua fase — review sebelum lapor selesai |
| `acs-security-reviewer` | 0, 2 — audit auth/RBAC/isolasi tenant/kredensial |
| `frontend-ux-builder` **(baru)** | 1, 2 — kerja React/Tailwind, konvensi UI |

**Gap yang baru ditutup:** sebelum dokumen ini dibuat, belum ada subagent yang meng-cover kerja frontend — padahal Fase 1 (prioritas utama user saat ini) adalah kerja frontend. Agent `frontend-ux-builder` dibuat bersamaan dengan dokumen ini untuk menutup gap tersebut.

**Alur kerja per item checklist:**
1. Pilih satu item, delegasikan ke agent yang sesuai (atau kerjakan langsung bila item lintas-domain).
2. Setelah implementasi, jalankan `acs-code-reviewer` (dan `acs-security-reviewer` bila menyentuh auth/kredensial/RBAC).
3. Tandai `[x]` di dokumen ini + catat tanggal selesai di §6.
4. Untuk perubahan skema: validasi end-to-end ke MariaDB nyata sebelum ditandai selesai (bukan opsional, lihat CLAUDE.md).

---

## 5. Definition of Done

Item baru boleh ditandai selesai hanya jika:
- Kode lulus review (`acs-code-reviewer`, ditambah `acs-security-reviewer` untuk area sensitif).
- Test relevan ada dan lulus (khusus task queue: skenario retry/`max_retries` wajib, bukan hanya happy path).
- Untuk perubahan skema: migrasi sudah dijalankan ke instance MariaDB nyata, bukan hanya dibaca sintaksnya.
- Untuk perubahan frontend: sudah dicoba manual di browser (bukan hanya lulus type-check/build).
- Tidak melanggar keputusan produk yang disengaja (mis. FR-18 — profil tidak auto re-push ke device terprovisioning).

---

## 6. Log Perubahan Dokumen

| Tanggal | Perubahan |
|---|---|
| 2026-08-21 | Dokumen dibuat. Snapshot kondisi awal dicatat, 3 fase + Fase 0 didefinisikan, agent `frontend-ux-builder` ditambahkan untuk menutup gap kerja frontend. |
