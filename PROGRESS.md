# PROGRESS.md — Log Sesi Kerja Otonom

---

## SESI 2026-10-15 (persiapan uji lapangan: OUI seed, self-heal vendor, gladi jaringan)

Pertanyaan pembuka: apa yang bisa dikerjakan menuju uji lapangan? Cek status: `dev` lebih maju 32 commit dari `main` (fast-forward bersih); sesi dimulai di `dev` `332f5a3`, working tree bersih. Host kini punya Go 1.27 (tanpa cgo → `-race` tidak bisa lokal) dan Docker Desktop (dinyalakan sesi ini).

### DIKERJAKAN

1. **Migrasi `0022_seed_vendor_ouis`** — `vendor_ouis` diisi 3.176 blok MA-L: ZTE 502, Huawei 2.088, FiberHome 220, Nokia 358, Cdata 8. Sumber: Wireshark `manuf` (turunan registry IEEE; situs IEEE menolak unduhan otomatis, HTTP 418), **disilangkan dengan nmap-mac-prefixes**: 0 nama registrant tak cocok dari 3.005 OUI yang ada di kedua sumber (171 sisanya alokasi baru). Pemetaan per NAMA REGISTRANT PERSIS (bukan substring); pengecualian & alasannya ada di header migrasi. `INSERT ... ON DUPLICATE KEY UPDATE id = id` (bukan `IGNORE`, supaya vendor hilang = gagal keras). `schema.sql` disinkronkan.
2. **Self-heal vendor/model saat Inform** — `FindOrCreateFromInform` sebelumnya hanya me-resolve vendor saat device DIBUAT; device yang terlanjur dibuat tanpa vendor tidak pernah sembuh meski OUI didaftarkan belakangan. Kini helper `resolveVendorModel` dipakai jalur create & jalur existing: hanya mengisi yang masih NULL (assign manual tak ditimpa), nol query tambahan untuk device yang sudah lengkap. 6 unit test baru (`resolve_vendor_test.go`).
3. **`PENGUJIAN_LAPANGAN.md` diperbarui** — status validasi, OUI seeded, batasan baru (rate limiter 5 req/s per IP, CWMP tanpa TLS, 401/204), prasyarat baru (HTTP vs HTTPS, urutan nyala CPE), **§3a Gladi Jaringan**, template lembar hasil (§6), **§8 tabel gejala→penyebab**. Nomor bagian lama dipertahankan (skill `/diagnose` merujuk §2/§6). Dokumen lain yang menyebut OUI kosong (ROADMAP, LAPORAN_KESIAPAN, PENGUJIAN_E2E) diberi keterangan.

### DIVERIFIKASI (hasil nyata, bukan baca sintaks)

| Gerbang | Hasil |
|---|---|
| `go build ./...` · `go vet ./...` · `go test ./...` | ✅ hijau (**tanpa `-race`**: host tanpa cgo; `-race` tetap di CI) |
| Migrasi `0→22` ke MariaDB 10.11 nyata (container sekali-pakai) | ✅ `version=22 dirty=false` |
| Seed: jumlah per vendor, semua 6-hex, lookup case-insensitive | ✅ |
| Idempoten (up dua kali), OUI manual tidak ditimpa, `down` hanya hapus baris ber-marker, up ulang | ✅ |
| Vendor hilang → migrasi gagal keras (`ERROR 1048`) | ✅ |
| `schema.sql` dimuat di DB kosong | ✅ 42 tabel = hasil migrasi, seed identik |
| e2e penuh pada stack compose nyata (MariaDB+MinIO+Redis+acsd) | ✅ **10/10 PASS** |
| Gladi pilot dari host via port 17547 + **self-heal di DB nyata** | ✅ device `vendor_id NULL` → `ZTE` di Inform berikutnya |

### TEMUAN (belum diperbaiki — butuh keputusan Anda)

- **Image `minio/minio:latest` di `docker-compose.yml` TIDAK BISA di-pull** (Docker Hub: `pull access denied`; quay.io juga 401, termasuk tag rilis). **Deploy ke server baru akan gagal** di langkah ini. Validasi lokal memakai override sementara ke `cgr.dev/chainguard/minio` (di luar repo; e2e S8 lulus dengannya). `docker-compose.yml` TIDAK diubah — memilih pengganti storage firmware = keputusan Anda (opsi: mirror Chainguard, build MinIO dari source/pin image internal, atau storage S3-compatible lain). Healthcheck `mc ready local` juga tidak ada di image pengganti.
- **OUI yang di-soft-delete memblokir pendaftaran ulang** (unique key `uq_vendor_ouis_oui` tidak melihat `is_deleted`) — dari skema; belum dicoba lewat UI.

### LANJUTAN: kelengkapan CRUD API (permintaan: "CRUD harus lengkap dulu")

Audit awal: dari ~25 resource, hanya provisioning-profiles dan webhooks yang CRUD-nya penuh. Ditutup **19 endpoint** (kontrak 103 → **122 operation**):

| Area | Ditambah |
|---|---|
| Katalog | `GET·PATCH·DELETE /vendors/{id}`, `DELETE /vendors/{id}/ouis/{ouiId}`, `GET·PATCH·DELETE /device-models/{id}`, `GET·DELETE /vendor-parameter-mappings/{id}` |
| Device | `DELETE /devices/{id}` |
| IAM | `GET·DELETE /tenants/{id}`, `GET /users/{id}` |
| Firmware | `GET·PATCH·DELETE /firmware/{id}` |
| Lain | `PATCH /tags/{id}`, `GET /presets/{id}`, `GET /zero-touch-rules/{id}` |

Desain (keputusan yang saya ambil, perlu Anda tahu):
- **Delete = soft delete + guard 409** bila masih dipakai (`domain.UsageRepository`, query COUNT; pesan menyebut jumlah per jenis dan menunjuk `is_active=false`). Vendor/model/firmware/tenant. Usecase baru `usecase/catalog` (sebelumnya handler katalog memanggil repository langsung).
- **Device delete**: task PENDING/QUEUED/SENT dibatalkan dulu (`domain.OpenTaskCanceller`), lalu soft delete. **Revive saat Inform**: unique key `(oui, serial)` mencakup baris terhapus, jadi tanpa ini CPE yang pernah dihapus lalu Inform lagi kena duplicate key dan terputus dari ACS. Baris lama dihidupkan (id/uuid/riwayat tetap; kredensial per-device, profil, catatan direset).
- **Revive OUI** (jebakan yang saya temukan tadi): OUI yang dihapus kini bisa didaftarkan ulang.
- PATCH vendor/model/firmware: partial; `code` vendor, `vendor_id` model, serta isi file/checksum/targeting firmware **tidak bisa diubah**. Objek MinIO tidak dihapus saat firmware di-soft-delete (kebijakan retensi terpisah).
- Tenant delete ditolak bila masih punya user/device aktif, atau tenant milik akun sendiri.
- Mutasi katalog/firmware superadmin saja; device delete ADMIN+ scope tenant.

**Celah keamanan yang ditemukan dan ditutup di tengah jalan:**
1. **Tag global** (tenant_id NULL) bisa diubah/dihapus ADMIN tenant mana pun — kini superadmin saja.
2. **Preset global** (menegakkan konfigurasi ke device SEMUA tenant) bisa diubah/dihapus ADMIN satu tenant — kini superadmin saja.
3. **Nil-deref** (`*actor.TenantID` tanpa cek) di `Tag.Delete` dan `Preset.Get` untuk akun non-superadmin tanpa tenant — kini ErrForbidden.
4. `POST /vendor-parameter-mappings` mengembalikan **`id: 0`** (Upsert tak mengisi ID) — mapping tak bisa direferensikan per id; diperbaiki (`LAST_INSERT_ID`).

Verifikasi: 7 file test baru (catalog, iam, device, tag, preset, firmware, provisioning; semua jalur RBAC/scope/guard) — `go build/vet/test` hijau (tanpa `-race`, host tanpa cgo); **e2e S11 baru, 11/11 PASS stabil 3×** di stack nyata — ini yang membuktikan SQL guard, revive, dan pembatalan task (tidak bisa dibuktikan test berfake). `openapi.yaml`: diff dua arah 0, redocly valid (6 warning pre-existing/by-design), Postman diregen 103 → 122 request. Parity UI: 19 endpoint baru **belum dipakai frontend**, 0 panggilan yatim.

**Belum:** UI untuk 19 endpoint baru (frontend tidak disentuh); `-race` lokal; delete tenant/vendor yang recreate dengan `code` sama masih 409 (unique key code mencakup baris terhapus — tidak ada revive, hanya OUI/device/mapping).

### LANJUTAN 2: gap UAT (setelah keputusan "UAT dulu, OLT Controller sesudahnya")

Konteks: user meminta spesifikasi besar "ACS + OLT Controller multi-vendor" (SNMP/SSH/NETCONF, driver ZTE/Huawei/Fiberhome/Nokia/C-Data/VSOL, router MikroTik/Huawei/Nokia/Junos, topology UI, coverage >=85%). Dijawab jujur: repo nol kode OLT, coverage 10%, CLI vendor tak bisa diverifikasi tanpa perangkat. Keputusan user: **UAT dulu; OLT Controller sesudahnya (irisan ZTE read-only, Go native)** — tersimpan di memori proyek.

**Keamanan (dari audit sebelumnya):**
- Dependensi: `x/net` v0.60.0, `go-jose` v4.1.4, `go-redis` v9.7.3. Dockerfile dipin `golang:1.26.9` (dulu `:latest`) + `toolchain go1.26.9` di go.mod; **govulncheck: 0 kerentanan terpanggil**. Runtime image kini non-root.
- OIDC: state+nonce acak (256 bit) diikat ke browser lewat cookie HttpOnly/SameSite=Lax/sekali-pakai, nonce diverifikasi, `email_verified=false` ditolak (dulu `state="random-state"`).
- Header keamanan REST (nosniff, DENY, CSP default-src 'none', no-store, HSTS bila https). API token: default & maksimum 365 hari (dulu boleh abadi).

**Bug interoperabilitas nyata yang ditemukan lewat test parser/handler (dua di antaranya akan membuat uji lapangan gagal diam-diam):**
1. **Cookie sesi CWMP selalu `Secure`** padahal ACS melayani HTTP: CPE libcurl tidak mengirim cookie Secure lewat http → POST kosong tanpa cookie → sesi ditutup → **tak ada RPC yang pernah terkirim**. Simulator e2e tidak menangkapnya (mengabaikan atribut Secure). Kini Secure hanya bila https/X-Forwarded-Proto. Terbukti dengan mutasi: test gagal bila `Secure: true` dikembalikan. Simulator CPE (`internal/simcpe`) kini juga setia: cookie Secure TIDAK dikirim lewat http, sehingga e2e akan gagal bila bug ini kembali (dulu simulator mengirim cookie apa pun dan menyembunyikannya).
2. **Deklarasi `encoding="ISO-8859-1"`/windows-1252** membuat seluruh Inform ditolak (Go hanya paham UTF-8) → CPE lama tak bisa terhubung. Kini `CharsetReader` (label tak dikenal tetap error).
3. **Spasi/newline di `DeviceId`** ikut jadi identitas device (kunci unik OUI+serial) → kini di-trim (nilai parameter tidak disentuh).
4. List SOAP keluar tanpa `soap-enc:arrayType` (TR-069 mewajibkan; firmware ketat menolak) dan urutan SetParameterValues acak → kini arrayType ada dan urutan deterministik.

**Test:** `pkg/cwmpxml` 22% → **98,9%** (fixture bergaya ZTE/Huawei/FiberHome, kuirk encoding, round-trip semua RPC outbound, fuzz seed); `delivery/cwmp` (handler HTTP) 3,8% → **97,7%** dengan mock CPE ber-cookie-jar ala libcurl (handler kini menerima interface `SessionService`); `usecase/session` 12% → 42,5% (NextRequest, HandleRPCResponse, fault 9005 permanen, retry → FAILED di max_retries, TransferComplete); auth/OIDC/token/header. **Total proyek 10,0% → 11,8%** — jujur: target >=85% TIDAK tercapai; repository MySQL/usecase besar dicakup e2e, bukan unit.

**Belum / menunggu keputusan:** image MinIO (tak bisa di-pull), HTTP vs HTTPS untuk CWMP (kini cookie aman untuk keduanya, tapi tanpa TLS kredensial Inform tetap polos); tipe parameter per-nilai di SetParameterValues (semua `xsd:string`); UI untuk 19 endpoint CRUD baru; `-race` lokal; coverage sisa.

### LANJUTAN 3: pipeline /siap-commit (review independen) — temuan & perbaikan

Dua reviewer independen (arsitektur + keamanan) membaca diff 68 file. Tidak ada BLOCKER, tetapi **1 HIGH dan beberapa MAJOR yang lolos dari build/test/e2e** — semuanya akibat fitur delete/revive buatan sesi ini:

| Temuan | Tingkat | Tindakan |
|---|---|---|
| **Revive device lintas tenant**: tenant B (secret CWMP sendiri) bisa me-revive baris device yang dihapus tenant A dgn OUI+serial sama → id/uuid/riwayat parameter-event-task pindah ke B | **HIGH** | **Diperbaiki**: `UPDATE revive` kini ber-klausa tenant (`tenant_id IS NULL OR tenant_id <=> :tenant_id`); lintas tenant → duplicate key → `ErrConflict` → Inform **409**. Dibuktikan e2e S11 (Inform tenant lain ditolak 409, device tetap terhapus, tenant lain tak melihatnya) |
| Pemetaan ENDUSER (`user_devices`) dan `device_tags` tidak dibersihkan saat soft delete (FK CASCADE tidak jalan) → pelanggan lama tetap bisa mengganti SSID/PSK CPE yang di-revive untuk orang lain | MEDIUM | **Diperbaiki**: `SoftDelete` kini transaksi (hapus relasi + tandai terhapus); `requireOwnership` juga mewajibkan tenant device = tenant akun (test baru, paket sebelumnya 0%) |
| OUI/model/mapping bisa dibuat untuk vendor yang sudah di-soft-delete (handler lama memanggil repository langsung; FK tak melihat is_deleted); vendor tak ada → 500 | MAJOR | **Diperbaiki**: ketiganya dipindah ke `usecase/catalog` dgn cek vendor aktif (404), model harus milik vendor yang sama, `device_type_id/data_model_version_id` wajib. Spec +404/409 |
| Ubah `data_model_version_id` model yang sudah dipakai device merusak `Resolve()` | MAJOR | **Diperbaiki**: 409 bila masih ada device |
| `err.Error()` OIDC (respons IdP, enumerasi email) ke klien | LOW | Diperbaiki: pesan generik, detail ke log |
| `session_token` utuh di log (satu-satunya bukti auth POST lanjutan) | LOW | Diperbaiki: `session.RedactToken` (8 karakter) |
| Migrasi 0022 gagal-dirty bila vendor baseline sudah dihapus; OUI ada dilewati | MINOR | Didokumentasikan di header migrasi |

**Ditemukan sampingan (bukan dari reviewer):** boot pertama volume kosong → `migrate` gagal `connection refused` (healthcheck MariaDB `ping` lewat socket lolos terhadap server SEMENTARA init yang tanpa TCP). Terjadi 2× di sesi ini; akan terjadi di deploy pertama server pilot. **Diperbaiki** di `docker-compose.yml` (ping `-h 127.0.0.1`), terbukti: volume kosong → migrate lolos percobaan pertama.

**Belum diputuskan / dicatat (butuh keputusan user, tidak diubah):**
1. **Unique key "kuburan"** (`uq_ref_vendors_code`, `uq_tenants_code`, `uq_tenants_cwmp_inform_username`, `uq_device_models_vendor_model` tak melihat `is_deleted`): setelah DELETE, membuat ulang dgn code/model_name yang sama = 409 selamanya; utk tenant, `cwmp_inform_username` ikut terkunci. Revive hanya ada utk device/OUI/mapping. Opsi: revive per-repo, atau ubah unique key.
2. **Rate limiter per `RemoteAddr`** tanpa `IPExtractor` trusted-proxy: di belakang reverse proxy semua CPE berbagi satu bucket 5 rps (CWMP) dan semua klien REST 30 rps; `RemoteIP` yang tersimpan = IP proxy. Perlu daftar proxy tepercaya.
3. **OIDC `email_verified` absen diterima** (Azure AD tak mengirimnya) + akun dipetakan lewat email → risiko nOAuth bila SSO dipakai. OIDC mati secara default.
4. Guard "masih dipakai → 409" = cek lalu hapus tanpa transaksi (TOCTOU); `DeleteVendor` menghapus OUI satu per satu tanpa transaksi. Hanya superadmin; dampak konsistensi data.
5. `devices.Update` full-row tiap Inform (sudah ada sebelumnya, melawan optimasi dfbdbfe) dan `resolveVendorModel` query tiap Inform bila model tak ada di katalog — belum diukur.
6. 403 vs 404 pada ID lintas tenant membocorkan keberadaan resource (ID berurutan; dampak kecil).

### LANJUTAN 4: UI untuk seluruh API (frontend)

API parity backend↔frontend: **95/114 → 114/114 (100%)**, 0 panggilan yatim. Dikerjakan: hook baru (`useVendor/useUpdateVendor/useDeleteVendor`, OUI, model, mapping, firmware, tenant, user, tag, preset, ZT rule, `useDeleteDevice`), komponen bersama `RecordActions` (EditButton/DeleteButton/DetailButton/DetailModal), dan:

| Halaman | Ditambah |
|---|---|
| Katalog Vendor | Edit & Hapus vendor (code read-only); hapus OUI; Edit & Hapus model; **Detail** & Hapus mapping |
| Firmware | Edit metadata (versi, catatan rilis, aktif) & Hapus; kolom status |
| Tag | Edit (nama/warna); tag global hanya superadmin |
| Administrasi | Detail user; Detail & Hapus tenant |
| Detail Device | Hapus Device (konfirmasi menjelaskan revive/task/pemetaan) |
| Preset / Provisioning | Detail preset (JSON terformat), Detail zero-touch rule (nama terjemahan); preset global hanya superadmin |

Semua modal edit memuat data segar lewat `GET by id`. Pesan 409 guard hapus tampil apa adanya di toast (backend: `usageConflictError` — pesannya kini "vendor X masih dipakai oleh 2 device, …", bukan "data sudah ada / konflik unique"; frontend membuang awalan teknis `domain:`).
Mapping sengaja TANPA tombol Edit: mapping generik (model & pattern NULL) tidak memicu ON DUPLICATE sehingga "edit lewat upsert" membuat baris ganda.

**Diuji sungguhan di browser** (agent-browser, stack nyata, superadmin DAN ADMIN tenant): siklus vendor buat→edit→hapus, guard 409 vendor ZTE & tenant pilot, modal OUI + pencarian, edit tag, edit firmware, hapus device (+redirect), detail user/tenant/preset/ZT rule, RBAC (ADMIN tidak melihat aksi superadmin-only). Temuan yang HANYA terlihat di browser dan sudah diperbaiki:
1. **Seed OUI merusak tabel Vendor**: sel "OUI Terdaftar" merender semua OUI sebagai chip (ZTE 502, Huawei 2.088). Kini ringkas (jumlah + 4 pertama) + modal "Kelola" dengan pencarian dan batas 200 chip.
2. **Bug logika `tenant_id === null`**: backend menghilangkan `tenant_id` untuk item global (`omitempty`) sehingga nilainya `undefined`; badge "Global" tak muncul dan ADMIN tenant akan melihat Edit/Hapus yang pasti 403. Kini `== null` (dibuktikan sebagai ADMIN tenant).
3. **Spinner tanpa akhir** di detail device yang tidak ada/terhapus (mis. tombol Back setelah hapus). Kini state "Device tidak ditemukan" + tombol kembali; query berhenti retry untuk 4xx.
4. Toolbar detail device (5 tombol) dan baris aksi tabel Tenants/Users terpotong/terbelah → membungkus.

**Belum / dicatat:** nama jenis di pesan 409 masih snake_case internal (`parameter_mapping`, `rollout_batch`); `created_at` pada respons create tag = 0001-01-01 (Create tidak mengisi timestamp, sudah ada sebelumnya); `frontend/devmock` (mock API) belum mengimplementasikan endpoint baru; tidak ada test otomatis frontend (belum ada test runner) — verifikasi lewat browser manual + parity script.

### LANJUTAN 5: Connection Request mendukung HTTP Digest

Temuan saat memetakan protokol: ACS mengirim Connection Request dengan **Basic preemptive** saja, padahal TR-069 §3.2.2 mewajibkan Digest di endpoint CPE; simulator menerima request apa pun sehingga e2e S7 tidak bisa mendeteksinya. Risiko lapangan: tombol Connection Request gagal (401) pada CPE yang hanya menerima Digest.

- `pkg/digestauth` (baru): Digest klien + verifier (MD5, MD5-sess, SHA-256, SHA-256-sess; qop auth atau tanpa qop; auth-int sengaja tidak didukung). Terbukti dengan **vektor resmi RFC 2617** + round-trip semua algoritma.
- `usecase/device/connreq_http.go`: request pertama TANPA kredensial; jawab tantangan Digest (prioritas) atau Basic (hanya bila CPE menawarkan), retry sekali untuk `stale=true`, skema tak dikenal dikembalikan sebagai 401. Efek samping baik: password tidak lagi terkirim sebelum CPE memilih skema.
- Simulator: `ConnReqListener.RequireDigest` dan flag `cpesim -cr-user/-cr-pass` (CPE sungguhan menantang Digest; tanpa jawaban benar tidak ada Inform).
- e2e S7 diperluas: kredensial diset via `PATCH /devices/:id`, ACS menjawab tantangan, **password salah → 502 dan tidak ada Inform**. 11/11 lulus pada stack hidup; `go test -race` lulus di Docker.

Catatan: password Connection Request tidak dilaporkan CPE lewat Inform — operator mengisinya per device (PATCH/UI). Belum diuji dengan CPE fisik; Digest dengan `algorithm=SHA-256` hanya teruji lewat verifier sendiri (vektor RFC 7616 belum dipakai).

### LANJUTAN 6: Login email, Argon2id, lupa password lewat email, notifikasi email, UI/UX baru

**UI/UX** — halaman Login (layout dua panel, Caps Lock, ingat email, SSO), Lupa/Reset password, Profil (identitas, sesi & hitung mundur, hak akses per role, preferensi) dan Dashboard (sapaan, peringatan, KPI drill-down, kesehatan armada, device perlu perhatian, task terbaru, rollout firmware, aktivitas, aksi cepat) dirombak; deep-link `/devices?status=OFFLINE` dan `/tasks?status=FAILED`. Footer kredit pembuat (nama → LinkedIn, tab baru, noopener) di app dan halaman auth. Diuji di browser nyata (light/dark, desktop/mobile, role SUPERADMIN & VIEWER). Dua bug lama yang hanya tampak di browser: 401 pada login/ganti-password diperlakukan "sesi berakhir" (pesan salah-password tak pernah tampil, dan salah password-lama di "Ganti password" **mengeluarkan user**); endpoint `/ws` mewajibkan header Bearer padahal browser hanya bisa kirim query → **WebSocket realtime tidak pernah tersambung** (BELUM diperbaiki, lihat bawah).

**Login email + Argon2id** — `POST /auth/login` menerima `email` (tak peka huruf besar) atau `username` (kompat.). `pkg/passwordhash`: Argon2id (64 MiB/t=3/p=2, PHC string), verifikasi bcrypt lama + **upgrade otomatis** saat login (compare-and-swap, tidak menimpa reset paralel), `DummyVerify` meratakan timing user tak dikenal, semaphore 4 slot dengan timeout → `ErrBusy` → 503 (BUKAN dihitung password salah).

**Lupa password lewat email** — migrasi 0023 `password_reset_tokens` (hash SHA-256 saja, sekali pakai, 30 mnt; pengecualian audit 7-kolom didokumentasikan di SQL), `POST /auth/forgot-password` (publik, respons seragam 202, 3/jam/akun, rate limit per-IP, diproses di goroutine terbatas+recover) dan `POST /auth/reset-password` (Peek murah sebelum Argon2, cek akun aktif sebelum Consume atomik, validasi password sebelum token dihanguskan). Tautan memakai fragment (`#token=`). `pkg/mailer`: SMTP **STARTTLS wajib** (kredensial tak pernah lewat plaintext), anti header-injection, MIME base64, antrean asinkron + retry, domain reserved (`.local .test example.*`) dibuang (hindari bounce dari akun uji/e2e).

**Notifikasi email** — reset password, password diganti, password direset admin, akun terkunci (sekali per penguncian), akun baru. Tidak pernah memuat password.

**Pencabutan sesi** — migrasi 0024 `users.password_changed_at`; `ResolveActor` menolak JWT yang terbit sebelum password diganti, atau milik akun nonaktif/terhapus (1 query PK per request). `PATCH /auth/password` kini 200 + JWT baru (sebelumnya 204).

**Temuan test integrasi MariaDB**: SQL penguncian akun salah urutan assignment (MariaDB mengevaluasi SET kiri→kanan) → akun terkunci pada percobaan ke-4, bukan ke-5. Diperbaiki.

**Review independen (agent) → diperbaiki**: DoS Argon2 sebelum cek token, race rehash vs reset, JWT hidup setelah reset, token hangus sebelum password tersimpan, banjir email kunci, 401 membocorkan error DB di login, goroutine tanpa recover & Close vs Notify, DeleteExpired tiap request, token reset tak dicabut saat email diganti/akun nonaktif, validasi format email user. **Tidak diperbaiki / dicatat**: (a) rate limit per-IP memakai alamat socket — di belakang reverse proxy semua klien berbagi satu bucket (butuh keputusan IPExtractor/trusted proxy); (b) `UpdatePassword` + Consume tidak satu transaksi (kegagalan DB sesaat setelah Consume = minta tautan baru); (c) antrean email bersama untuk reset & notifikasi (4 worker, buffer 256, drop saat penuh); (d) timing login user bcrypt-lama vs Argon2 masih berbeda tipis sampai di-upgrade.

**Pengujian**: unit (passwordhash, mailer + SMTP palsu, notify, auth), integrasi MariaDB nyata (`ACS_TEST_DSN` — termasuk 8 Consume paralel = 1 pemenang), e2e **S12** (12/12 lulus), `go test -race ./...` (Docker 1.26.9), spec 124 operation (redocly valid), parity 116/116. **Dibuktikan manual**: login email di browser, alur reset di UI dengan token tersisip DB (password baru berlaku, lama ditolak, token tak bisa dipakai ulang), ganti password → JWT lama 401 / baru 200, email reset nyata terkirim lewat Gmail SMTP.

### LANJUTAN 7: tambah/edit/hapus perangkat, impor/ekspor dengan pratinjau, Buku Panduan v2.0

**Perangkat** — normalnya self-register saat Inform pertama; kini juga manual: `POST /devices` (pra-registrasi, status `UNREGISTERED`; ADMIN terkunci ke tenant sendiri, superadmin wajib `tenant_id`, serial+OUI milik tenant lain → 409 tanpa bocor pemilik, baris terhapus milik tenant yang sama di-*revive*), `PATCH /devices/:id` diperluas (product_class, MAC, vendor, model, lokasi; string kosong = hapus), `POST /devices/import` (`?dry_run=true`, mode `skip|update`, ≤ 1000 baris, hasil per baris create/update/unchanged/skip/error, satu audit `IMPORT_DEVICES`). Inform pertama CPE pra-registrasi memakai baris yang sama (id tetap, catatan terjaga); tenant LAIN ditolak. UI: tombol Tambah/Edit/Hapus (+hapus massal) sesuai RBAC (NOC edit; ADMIN tambah/hapus).

**Kerangka impor/ekspor frontend** — `lib/csv.ts` (RFC 4180, deteksi pemisah `, ; tab`, BOM, netralisasi CSV-injection), `lib/importer.ts`, `lib/clientAdapter.ts` (plan: parse → duplikat-dalam-file → muat data eksisting → diff → klasifikasi; execute: pool 4 paralel + retry 429/503), `ImportWizard` (pilih berkas → **pratinjau vs data eksisting** dengan label Baru/Diperbarui/Tidak berubah/Dilewati/Error + alasan per baris → mode skip/update → eksekusi dengan progres & batal → ringkasan + CSV baris gagal), `ExportMenu` (CSV/JSON), `DataTools`. Header ekspor = kunci impor, jadi ekspor → impor ulang = semua "Tidak berubah". Rahasia tidak pernah diekspor; nilai rahasia kosong pada baris yang sudah ada = pertahankan nilai tersimpan, pada baris baru = Error **di pratinjau** (bukan baru gagal saat eksekusi).

Modul impor+ekspor: Perangkat, User, Tenant, Katalog (Vendor/OUI/Model/Mapping), Tag, Preset, Provisioning Profile, Zero-Touch Rule. Ekspor saja: Task, Firmware & File (metadata), Audit Trail, Webhook (URL disamarkan).

**Bug lama yang baru terlihat lewat uji sungguhan** (semua diperbaiki): (1) `latitude/longitude` tidak pernah tersimpan oleh repository (Create/Update/revive); (2) membuat webhook selalu gagal — `ref_webhook_event_types` tidak ada di whitelist ref; (3) `/refs/ref_parameter_types` 500 (`code AS name AS name`); (4) `device-models` mengembalikan `null` untuk vendor tanpa model → crash form/ekspor; (5) validasi OUI hex-only menolak OUI simulator → dilonggarkan jadi 6 karakter alfanumerik (backend, frontend, spec, tes).

**Pengujian** — go vet + `go test -race ./...` (Docker 1.26.9), integrasi MariaDB (lokasi, ref), e2e **S13** (13/13), `tsc -b`, `oxlint` (hanya warning only-export-components lama), `npm test` (8), `vite build`, redocly lint, parity 118/118. **Browser nyata** (superadmin, ADMIN tenant, NOC, VIEWER): Perangkat, User, Tenant, Katalog (round trip 3.181 OUI ≈ 5,6 dtk), Audit, Webhook, **Tag, Preset, Provisioning Profile, Zero-Touch Rule** (round trip semua "Tidak berubah", baris tak valid ditolak dengan alasan, ADMIN tenant tak bisa membuat item global, redaksi rahasia), **ekspor Task (40 baris), Firmware (5), File**. Data uji dibersihkan.

**Catatan keamanan (belum diubah, perlu keputusan)**: nilai `set_parameter` berkunci rahasia pada Preset & Provisioning Profile **disimpan apa adanya di DB dan dikembalikan oleh API ke ADMIN** (hanya ekspor CSV yang mengosongkannya). Pertimbangkan enkripsi at-rest seperti kredensial CR (`pkg/cryptoutil`) bila secret WiFi/PPPoE dianggap sensitif. Juga: `POST /files` menerima `file_type` bebas (tidak dicocokkan dengan konstanta domain).

**Buku Panduan v2.0** (`BUKU_PANDUAN_ACS.html` → `.pdf`, 21 hlm; dibuat dengan Chrome headless `--print-to-pdf`, diagram Mermaid dari CDN + SVG inline): bab baru **"Cara kerja ACS — di OLT atau di ONT?"** (ACS mengonfigurasi ONT/CPE lewat TR-069; OLT dikelola terpisah lewat SNMP/CLI/NETCONF; peta dua bidang manajemen, tabel "dikonfigurasi di mana", perjalanan ONT baru, cara ONT tahu URL ACS, Connection Request, kasus bridge/SFU/OMCI, FAQ), serta pembaruan login email/Argon2id/reset, tambah-edit-hapus perangkat, impor/ekspor, matriks protokol & batasan, troubleshooting.

**Panduan UAT (Rabu 21 Okt 2026)** — `PANDUAN_UAT_ACS.html/.pdf` (16 hlm): linimasa & serah-terima antar tiga tim, langkah demi langkah per tim (IT: server, .env, firewall, Console, tenant/akun, gladi, pemantauan; Network: jalur dua arah, ONT uji, isian TR-069; Ops: akun, data uji, peran), agenda ±4 jam, 10 kasus uji Network (N1–N10), 20 kasus Operasional (O1–O20), 2 skenario gabungan (G1–G2), daftar batasan yang sudah diketahui, kriteria GO/NO-GO + tingkat keparahan, lembar lingkungan/ONT/bug/sign-off, backup & tabel gejala. Melengkapi `PENGUJIAN_LAPANGAN.md` (gladi jaringan §3a).

### LANJUTAN 8: audit pra-deployment (API, frontend, perbandingan GenieACS) — 2026-10-16

Tiga audit independen (agent baca-saja) + gerbang otomatis; tiap temuan diverifikasi di kode/API hidup sebelum diperbaiki. **Diperbaiki:** NOC bisa membuat task apa pun (RBAC per tipe + validasi parameters, diagnostics → 501); WebSocket realtime akhirnya tersambung (token via subprotocol, Origin dibatasi, race hub); role/sesi (ganti role mencabut JWT, API token mati bila user nonaktif & tak bisa beranak token); ADMIN tak bisa memutasi SUPERADMIN; file global/nil-deref; TransferComplete lintas device; firmware (cocok vendor/model, rollout dibatasi, cancel membatalkan task); rahasia disamarkan untuk NOC/VIEWER; JWT di log akses; FK/data-terlalu-panjang → 400; profil setengah jadi; mapping duplikat (NULL di UNIQUE); protokol: TransferCompleteResponse, URL presigned dibuat saat kirim, tipe XSD sesuai CPE, nilai Set dicatat (drift preset), backoff retry, task tak-terbangun FAILED (bukan 500+SENT); retensi log (ACS_LOG_RETENTION_DAYS); trusted proxy/Redis password/USP default mati/port lokal; **B1 frontend**: ADMIN tenant sebelumnya 403 saat membuat user/profil/ZTP/webhook; cache cleared saat logout; Enter di dialog; SSO disembunyikan; validasi form & data referensi (OUI, path TR-069, tag, tenant).

**Belum diperbaiki / keputusan:** state galat daftar (M2), mode gelap & tabel mobile (M8/M9, perlu verifikasi visual — agent-browser macet), label tanpa htmlFor & fokus modal (m1/m2), SSRF jaringan internal via webhook/CR (jaringan privat sengaja diizinkan), TLS CWMP, enkripsi nilai rahasia Preset/Profil, diagnostics, OIDC, rate limit terdistribusi, SetParameterAttributes/virtual parameters/refresh pohon parameter (kesenjangan vs GenieACS), UDP CR TR-111 (diragukan). **Belum teruji di CPE fisik.**

### BELUM

- Gladi jaringan ANTAR-MESIN sungguhan (§3a) — hanya bisa dari segmen jaringan CPE.
- Putusan HTTP vs HTTPS untuk ACS URL; unit CPE per vendor; keputusan MinIO di atas.
- `-race` lokal (butuh cgo/Docker build); gerbang frontend tidak disentuh sesi ini (tidak ada perubahan frontend).
- Semua perubahan **belum di-commit** (menunggu perintah).

---

## SESI 2026-09-08 (2) (`/goal`: harness uji e2e + audit keamanan + kesiapan go-live)

Lanjutan sesi yang sama. Setelah commit batch validasi input (`ac7209a`), user
minta: bangun pengujian sampai "benar-benar bisa dipakai", audit keamanan
menyeluruh (OWASP), buat laporan + `BUKU_PANDUAN.pdf`. Docker dinyalakan user.

### DIKERJAKAN: harness uji end-to-end + simulator CPE multi-vendor

- **`internal/simcpe/`** (baru) — simulator CPE TR-069/CWMP: klien CWMP sungguhan
  (Inform semua event code, terima & jawab RPC, `cwmp:Fault` konfigurabel,
  listener Connection Request), data model in-memory yang **benar-benar berubah**
  saat di-`Set`, 5 profil vendor (ZTE/Huawei/FiberHome/Nokia/Cdata) dengan
  namespace CWMP `cwmp-1-0`/`1-1`/`1-2` & root TR-098/TR-181 yang bervariasi.
- **`cmd/cpesim/`** (baru) — CLI: single/fleet/fault-injection/conn-req.
- **`cmd/e2e/`** (baru) — 9 skenario terhadap stack `docker-compose` hidup
  (REST + CWMP + MariaDB + MinIO), assertion via REST API.
- `PENGUJIAN_E2E.md` + `scripts/e2e.ps1`.

**Hasil: 9/9 LULUS, stabil 3× berturut-turut** — S1 onboarding 5 vendor, S2
provisioning push, S3 reboot, S4 get-param, S5 fault-9005-no-retry, S6 isolasi
tenant, S7 connection-request, S8 firmware canary rollout, S9 preset drift-heal.
Fleet: 20 device × 2 siklus = 40/40 sesi OK. Migrasi `0→21` ke MariaDB nyata: ✅.

### Bug/gap ACS ditemukan & diperbaiki lewat e2e

- **ACS tidak meng-capture `ConnectionRequestURL` dari Inform** — operator harus
  isi manual per device sebelum bisa Connection Request. Diperbaiki:
  `FindOrCreateFromInform` meng-capture dari parameter Inform standar (penting
  untuk skala auto-provisioning ribuan CPE).
- **`TriggerConnectionRequest` ke CPE offline → HTTP 500** ("internal server
  error"). Diperbaiki: `domain.ErrUpstreamUnavailable` → **502**.
- Assertion e2e menyingkap: task list JSON tidak mengisi `task_type_code`/
  `task_status_code` (harus resolve via `ref_*`) — bukan bug, catatan.
- `jitterPeriodicInformInterval` (anti thundering-herd) membuat nilai akhir
  PeriodicInformInterval beda dari literal preset — **fitur**, bukan bug.

### DIKERJAKAN: audit keamanan OWASP Top 10 (`acs-security-reviewer`)

**2 BLOCKER — diperbaiki:**
1. **SSRF via `connection_request_url`** (diperkenalkan fitur capture di atas).
   CPE terkompromi → `http://169.254.169.254/...` → ACS meng-GET saat operator
   trigger CR. Fix: (a) capture hanya bila host URL == IP sumber Inform
   (`safeInformConnectionRequestURL`); (b) `pkg/netguard` (baru) tolak
   loopback/link-local/metadata + `CheckRedirect` tolak redirect di
   `TriggerConnectionRequest`; (c) guard sama untuk `UDPConnectionRequestAddress`
   (jalur STUN); (d) status/host upstream tidak lagi bocor ke response.
   Diverifikasi e2e S7 masih lulus (host container == RemoteIP).
2. **Role ENDUSER tidak dikurung** — token portal pelanggan bisa `GET /devices`
   dll. → baca data seluruh tenant. Fix: middleware `staffOnly` di grup `authed`
   (`router.go`), ENDUSER → 403 di semua API staf, hanya `/self-service/*` +
   `PATCH /auth/password` (grup `authedSelf`). Diverifikasi live: 6 endpoint staf
   → 403, `/self-service/devices` → 200.

**NON-BLOCKER — diperbaiki:**
- `middleware.BodyLimit` — REST 2 MiB (upload firmware dikecualikan via Skipper),
  CWMP 4 MiB. Menutup memory-exhaustion via POST raksasa pra-auth.
- `iam.CreateUser` kini validasi panjang password minimum (disamakan dgn
  reset/change).
- SSRF webhook `target_url` — `pkg/netguard` + `CheckRedirect` + re-cek saat
  dispatch. `validateTargetURL` jadi method (guard injectable untuk test).

**NON-BLOCKER — dicatat, belum dikerjakan:** OIDC `state`/`nonce` tidak
divalidasi; rate limiter in-memory per-instance; API token tanpa expiry;
`govulncheck` belum di CI; security headers. Detail: `LAPORAN_KESIAPAN.md` §4.3.

### Review kode (`acs-code-reviewer`) — temuan ditindaklanjuti

B1 binary `cpesim`/`e2e` nyangkut di root → `.gitignore` + dihapus. B2 data race
slice `got` di S7 → mutex. M1 pesan `ErrUpstreamUnavailable` bocor detail infra →
statis. M2 `paramSuffix` suffix penuh `.ManagementServer.ConnectionRequestURL`.
M3 S8 advance hanya saat macet. Dead code (`qv`, `failuresOnly`, `booted`)
dihapus.

### DIKERJAKAN: audit performa skala (`perf-scale-auditor`) + perbaikan

Temuan: ~17–20 round-trip DB per periodic Inform; 100k device @ 300s ≈ 6.000
query/detik untuk inform "kosong".

**Diperbaiki (commit `dfbdbfe`):**
- **B2** — `mysql.NewCachedRefRepository` (baru): cache tabel `ref_*` imutabel
  (event code, status, trigger ZTP, role, tipe param) — hilangkan ~6 query DB
  per Inform. `List()` (UI) tetap segar.
- **B5 parsial** — `MarkOnline` skip `UPDATE devices` bila sudah ONLINE;
  `SetCWMPNamespace` hanya `UPDATE device_sessions` bila namespace berubah.

**BLOCKER skala belum dikerjakan** (butuh desain/load-test — `LAPORAN_KESIAPAN.md`
§5.2): B1 retensi tabel log tanpa batas, B3 N+1 resolusi path preset, B4 rate
limiter in-memory per-IP (gagal untuk CGNAT/multi-instance), B5 penuh (targeted
UPDATE devices tanpa rewrite kolom kredensial). C1/C3/C4/C5/C6 dicatat §5.3.

### Gerbang & deliverable

- `ci-gatekeeper` + gate lokal: gofmt/vet/build/`go test -race`/frontend/redocly
  — **HIJAU** setiap commit (`pkg/netguard` + 6 fake test webhook + ref cache
  diverifikasi).
- e2e: **10/10 LULUS stabil 3×** setelah SEMUA perbaikan (keamanan + perf).
- `LAPORAN_KESIAPAN.md` (baru) — status jujur go-live + audit perf §5.
- `BUKU_PANDUAN_ACS.html` + `.pdf` (baru) — panduan operasional 15 bagian.

### Verdict go-live

**Siap pilot terbatas** (1 tenant, populasi kecil, monitoring ketat). **Belum
siap rollout penuh** — penahan: nol bukti CPE fisik (tak bisa disimulasikan),
`vendor_ouis` kosong, load test kapasitas puncak butuh rig multi-IP.

---

## SESI 2026-09-08 (`/siap-commit`: verifikasi + commit batch validasi input yang menggantung)

### Kondisi saat sesi dimulai

Branch `dev` (`1e467b0`). Working tree **penuh perubahan tak-tersimpan** dari sesi
~5–7 Sep yang belum pernah di-commit: batch validasi panjang input
(`validate.go`), endpoint `GET /vendor-parameter-mappings`, `limits.ts` + 45
`maxLength` frontend, sinkron `openapi.yaml` + Postman, 7 agent + 8 slash command
baru, `.mcp.json` (agent-browser), `scripts/`. Docker Desktop **mati**, host tanpa
Go toolchain → gerbang Go tidak bisa dijalankan lokal.

### Temuan penting: `go test ./...` di `dev` HEAD SEDANG MERAH

Commit `1e467b0` menambah method `ListByTenant` ke interface
`domain.ActivityLogRepository` + implementasinya, tapi **tidak** mengupdate 6 fake
di `internal/usecase/{auth,firmware,provisioning,tag,task,webhook}/service_test.go`
→ paket-paket itu gagal compile saat test. Batch yang menggantung ini yang
memperbaikinya (6 fake dapat `ListByTenant` no-op). Jadi commit ini juga
memulihkan CI.

### Gerbang yang dijalankan sesi ini

| Gerbang | Hasil |
|---|---|
| `npm run lint` (oxlint) | ✅ 0 error (4 warning pre-existing) |
| `npm run build` (`tsc -b && vite build`) | ✅ bersih |
| `redocly lint openapi.yaml` | ✅ valid (6 warning pre-existing/by-design) |
| `gofmt`/`vet`/`build`/`go test -race` | ⚠️ TIDAK dijalankan (Docker mati) — diserahkan ke CI GitHub atas keputusan user |
| `acs-code-reviewer` (arsitektur) | ✅ tidak ada blocker |
| `acs-security-reviewer` (keamanan) | ✅ tidak ada blocker baru dari diff ini |
| `api-contract-sync` | ✅ 103 route ↔ 103 operationId, 0 drift; Postman diregen (101→103 request) |

### Follow-up dari review (dicatat, TIDAK diperbaiki sesi ini — bukan blocker)

1. **Validasi ganda branding + `target_url`.** `updateTenantBranding` dan
   `create/updateWebhook` memanggil `checkMaxLen` padahal `validateBranding`
   (`iam/service.go:178`) dan `validateTargetURL` (`webhook/service.go:412`) sudah
   memvalidasi field yang sama. Lebih buruk: `validateBranding` pakai `len()`
   **byte**, `checkMaxLen` pakai **rune** — `brand_name` 128 karakter beraksen
   bisa lolos delivery lalu ditolak usecase dengan pesan beda. Fix: samakan
   `validateBranding`/`validateTargetURL` ke rune, atau hapus cek delivery yang
   dobel.
2. **Cakupan `checkMaxLen` belum lengkap.** `updateDevice` (`notes`,
   `connection_request_url/username`), `uploadFirmware` (`version`, `file_name`),
   firmware upgrade/batch (`from_version`/`to_version`, `notes`) masih bisa jadi
   500 untuk input kepanjangan — kelas bug yang sama yang batch ini tutup
   sebagian.
3. **`createWebhook`/`updateWebhook`:** `checkMaxLen` dipanggil sebelum cek
   "wajib diisi" — beda urutan dari 5 handler lain, pesan error kurang menuntun.
4. **Tidak ada `middleware.BodyLimit`** di `restEcho`/`cwmpEcho` — `c.Bind()`
   unbounded, termasuk `POST /auth/login` anonim. Tambah `BodyLimit("1M")` REST.
5. **Password CWMP tanpa cap panjang** (`setTenantCWMPCredentials`,
   `createTenant`) → plaintext >~227 char → ciphertext AES-GCM > `VARBINARY(255)`
   → MariaDB 1406 → 500. Beri cap ~256.
6. **Role ENDUSER tidak dikurung ke `/self-service`** — token ENDUSER diterima di
   semua route grup `authed` tanpa `RequireRoles` (`GET /devices` dll. bocor
   daftar device se-tenant ke principal portal pelanggan). Sudah tercatat samar
   di ROADMAP "Gap jujur"; reviewer menguatkan. Item Fase 0/2, konfirmasi user.
7. `TestLenRulesMatchSchema` hanya menjaga 10 dari ~30 konstanta `validate.go`.

### BELUM

- Gerbang Go lokal (butuh Docker) + `migrate up 0→21` ke MariaDB nyata.
- Perbaikan 7 follow-up di atas.

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

### DIKERJAKAN (lanjutan) — Engine preset v1 (backend)

User memilih **Opsi C + 5a/5b** (via AskUserQuestion). Diimplementasikan:
- Migrasi `0021_preset_engine`: `presets.enforce`(default 0)/`channel` + tabel
  `preset_applications` (tracking per preset×device). `schema.sql` disinkron.
- `domain`: `Preset`+`Enforce`/`Channel`, `PresetPrecondition`, `PresetConfigOp`,
  `PresetApplication`+repo; `Preset.ParsedPrecondition()/ParsedConfigurations()`.
  `TaskEnqueuer`+`ResolveParameterPath`.
- `mysql`: `presetRepository.ListActiveEnforce` + `presetApplicationRepository`.
- `provisioning.Service.EvaluatePresets` (`preset_eval.go`): dipanggil di
  `session.HandleInform` SETELAH `EvaluateZeroTouch`. Precondition match
  (reuse `matchSQLLike`), drift-check op `set_parameter` (resolve key→path,
  bandingkan `device_parameters`), merge drift lintas preset → 1
  `SetParameterValues`. Pagar: `HasPendingForDevice`, cooldown 15m per
  (preset,device), give-up setelah 3× push tanpa konvergensi (hash drift sama)
  → status FAILED + `PRESET_APPLY_FAILED` activity log (FR-15).
- `preset.Service.Create/Update`: validasi JSON + tolak op non-`set_parameter`
  (v1), salin `Enforce`/`Channel`.
- 7 unit test `provisioning/preset_eval_test.go` (mutation-safe: tiap skenario
  menegaskan enqueue count).
- `openapi.yaml` (Preset/CreatePresetRequest/UpdatePresetRequest + skema `Preset`)
  + `Preset` type FE (+enforce/channel). **Postman regen DITUNDA** — regen
  openapi2postmanv2 hanya meng-churn ratusan `id` UUID acak per item (bukan
  perubahan semantik); jalankan `npx openapi-to-postmanv2 -s openapi.yaml ...`
  saat mau merge ke main.

**Verifikasi backend via CI (GitHub Actions):**
- Commit `7c59a76`/`c80ee54` awalnya MERAH — hanya `gofmt` di `preset.go`
  (kolom tipe struct field 10→8). Diperbaiki manual (tanpa Go toolchain di
  host — hand-align mengikuti pola `domain/tenancy.go`).
- **Commit `59f9014`: CI HIJAU penuh** — `gofmt` bersih, `go vet ./...`,
  `go build ./...`, `go test -race ./...` (termasuk 7 test preset_eval baru +
  test provisioning/session yang disentuh) semua lulus. Frontend + openapi lint
  juga hijau.
- **MASIH belum diverifikasi:** `migrate up 0→21` ke MariaDB nyata (CI tidak
  punya service DB). Migrasi `0021` = ALTER + CREATE TABLE standar, konsisten
  sintaks dgn 20 migrasi lain — tapi belum dijalankan ke instance nyata.

**TEMUAN + DITINDAKLANJUTI:** `PresetsPage.tsx` TIDAK ADA (PRESET_ENGINE_DESIGN.md
keliru menyebutnya). **Dibuat sesi ini:** `PresetsPage.tsx` (list, create/edit
modal dgn textarea JSON precondition/configurations + validasi klien, toggle
`enforce` dgn penjelasan drift-heal, delete) + route `/presets` + nav "Presets"
(ADMIN) + banner peringatan FR-18. FE `tsc`+`build`+`oxlint` hijau. **Belum:**
estimasi "N device terpengaruh" saat aktifkan enforce (butuh endpoint count
backend — v1.1).

### (arsip) Proposal desain engine preset

`PRESET_ENGINE_DESIGN.md` dibuat — proposal keputusan (lalu diimplementasi di atas).
Isi: preset vs ZTP rule (beda: enforcement berkelanjutan vs provisioning awal);
3 opsi (hapus / engine penuh / minimal+drift → **rekomendasi minimal**);
precondition = objek terstruktur identik ZTP (reuse `matchPrecondition`, TANPA
DSL/skrip JS — sejalan filosofi "data auditable"); configurations = array op
bertipe (`set_parameter`/`apply_profile`/`refresh`); evaluasi tiap sesi setelah
`EvaluateZeroTouch` DENGAN drift-check (enqueue hanya bila device menyimpang) +
pagar anti-loop; **konflik FR-18** diangkat eksplisit sebagai keputusan produk;
skema (`presets.enforce`/`channel` + tabel `preset_applications`); struktur kode
Clean Arch + test wajib. §9: 4 keputusan menunggu user.

### BELUM — butuh keputusan/akses user (lihat ROADMAP §"Gap jujur pasca-audit")

- Uji lapangan CPE fisik (butuh hardware).
- Engine preset: 4 keputusan produk di `PRESET_ENGINE_DESIGN.md` §9 — jangan
  implementasi sebelum dijawab.
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
