# OLT Controller — Rancangan Lapisan 1 (auto-provisioning ONU di sisi OLT)

Status: **irisan 1 (kerangka + simulator), read-only, tanpa data vendor nyata**. Dokumen ini adalah rencana dan batas kerja; ubah bila keputusan berubah.

## 1. Mengapa ada dokumen ini

Zero-touch FTTH punya dua lapisan yang sering tertukar:

| | Lapisan 1 — OLT | Lapisan 2 — ACS (TR-069/CWMP) |
|---|---|---|
| Istilah umum | auto-find / auto-provisioning ONU | zero-touch / auto-config CPE |
| Kanal | OMCI, dikendalikan lewat CLI/SNMP OLT atau EMS | HTTP/SOAP, ONT menghubungi ACS |
| Mengatur | pengenalan serial number (SN), pengesahan ONU, T-CONT/GEM, profil bandwidth, VLAN, service port | SSID/password WiFi, PPPoE, mode WAN, firmware, parameter perangkat |
| Syarat | ONU terpasang di PON | ONU sudah punya jalur IP ke ACS |

ACS yang ada (`internal/delivery/cwmp`, Zero-Touch Rules, Provisioning Profile) mengerjakan lapisan 2 dan sudah teruji di ONT fisik (UAT 21 Okt 2026). Lapisan 1 belum ada di repo. Lapisan 1 harus berhasil lebih dulu: tanpa jalur data dari OLT, ONT tidak akan pernah mencapai ACS.

## 2. Keputusan yang sudah diambil

Dari catatan keputusan 15 Okt 2026 dan jawaban 26 Okt 2026:

1. **Urutan:** UAT lapangan ACS dulu (sudah berjalan), lalu OLT Controller.
2. **Vendor pertama:** ZTE C300/C320/C600. OLT lab ZTE tersedia untuk pengembangan.
3. **Read-only dulu.** Tidak ada penulisan ke OLT sampai terbukti di perangkat nyata. Penulisan (provisioning + rollback) menyusul, awalnya lab-only.
4. **Go native** (satu binary, Clean Architecture tetap). Bukan layanan Python.
5. **Irisan 1 (dokumen ini):** rancangan, inventaris OLT, antarmuka driver, simulator, dan antrean ONU belum terdaftar di UI. Tanpa menulis ke OLT, tanpa klaim kompatibilitas vendor.

**Aturan keras:** sintaks CLI, OID SNMP, dan format keluaran vendor **tidak boleh ditulis dari ingatan**. Sumbernya hanya dokumentasi resmi atau keluaran nyata dari OLT lab. Alasannya sama dengan `migrations/0004`: data vendor yang salah terlihat benar sampai bertemu perangkat.

## 3. Cakupan irisan 1

Dikerjakan:
- Inventaris OLT per tenant (host, port, kredensial terenkripsi, driver).
- Antarmuka driver `domain.OLTDriver` dan registry driver.
- Driver simulator `sim` (deterministik, dengan injeksi kegagalan) untuk pengembangan, e2e, dan demo. **Nonaktif secara bawaan** (`ACS_OLT_SIM_ENABLED`).
- Tes koneksi (`Probe`) dan polling ONU belum terdaftar, manual lewat API.
- Antrean "ONU belum terdaftar" per OLT: terdeteksi → diabaikan / selesai (sudah tidak muncul di OLT).
- API REST, halaman UI, OpenAPI, tes unit/integrasi/e2e.

**Tidak** dikerjakan (sengaja):
- Menulis apa pun ke OLT (authorize ONU, service port, profil). Tombol "setujui" tidak ada di irisan ini karena akan menjanjikan sesuatu yang belum terjadi.
- Driver ZTE nyata. Menunggu keluaran OLT lab (lihat §8).
- Polling periodik otomatis. Irisan 1 hanya polling manual; periodik menyusul bersama driver nyata, lewat kunci leader sweeper yang sudah ada.
- Menghubungkan SN ONU ke serial CPE yang Inform. Hubungan keduanya spesifik vendor dan tidak boleh diasumsikan.
- Pembacaan optik dan status port dari sisi OLT.

## 4. Model data

Mengikuti konvensi CLAUDE.md: tabel referensi `ref_*` (bukan ENUM), audit 7 kolom, soft delete, PK internal tidak diekspos (UUID publik).

- `ref_olt_poll_status`: `UNKNOWN`, `OK`, `UNREACHABLE`, `AUTH_FAILED`, `ERROR`.
- `ref_onu_discovery_status`: `PENDING` (terdeteksi), `IGNORED` (operator abaikan), `RESOLVED` (tidak lagi belum-terdaftar di OLT).
- `olts`: `olt_uuid`, `tenant_id` (wajib), `name`, `driver_code`, `host`, `snmp_port`, `ssh_port`, `snmp_community_enc`, `ssh_username`, `ssh_password_enc` (AES-GCM, kunci `ACS_CREDENTIAL_ENC_KEY`), `notes`, `is_active`, `poll_status_id`, `last_poll_at`, `last_poll_error`, `reported_name`, `reported_version`.
- `olt_unregistered_onus`: `onu_uuid`, `olt_id`, `pon_port`, `serial_number`, `onu_type`, `status_id`, `first_seen_at`, `last_seen_at`, `resolved_at`. Unik per (`olt_id`, `serial_number`); kemunculan ulang menghidupkan baris lama (revive), bukan menambah baris.

Kredensial **tidak pernah** dikembalikan API (`json:"-"`), hanya penanda `has_snmp_community` / `has_ssh_password`. Tidak pernah dicatat di log.

## 5. Antarmuka driver

```
OLTDriver
  Code() string
  Probe(ctx, OLTConnection) (OLTInfo, error)
  ListUnregisteredONUs(ctx, OLTConnection) ([]UnregisteredONU, error)
```

- `OLTConnection` memuat host, port, dan kredensial **plaintext hanya di memori**, dibuka dari kolom terenkripsi tepat sebelum dipakai.
- Galat driver dipetakan ke `ErrOLTUnreachable` / `ErrOLTAuthFailed` / galat umum, supaya status poll akurat tanpa membocorkan teks dari perangkat ke klien.
- Driver dipilih lewat registry berdasarkan `driver_code`. Ini pola adapter (CLAUDE.md: kuirk vendor di adapter, bukan percabangan tersebar). `GET /olt-drivers` mengembalikan driver yang terdaftar; `sim` hanya muncul bila diaktifkan.
- Penambahan driver vendor baru = paket baru + `Register`, tanpa mengubah usecase.

Antarmuka sengaja kecil. Metode untuk optik, status port, dan penulisan ditambahkan hanya setelah ada keluaran nyata yang menentukan bentuknya.

## 6. Keamanan

- **Tenancy:** OLT milik satu tenant. Non-superadmin hanya melihat/mengubah OLT tenantnya (`auth.ScopedTenantFilter`, `auth.RequireTenantScope`); superadmin wajib menyebut `tenant_id` saat membuat.
- **RBAC:** baca untuk semua peran staf; tambah/ubah/hapus/tes koneksi untuk ADMIN; polling dan abaikan ONU untuk ADMIN/NOC. Semua di balik middleware auth.
- **SSRF:** `host` adalah masukan admin tenant yang menjadi tujuan koneksi keluar. Divalidasi saat simpan dengan `netguard.CheckHostPort` (loopback, link-local/metadata, multicast ditolak; RFC1918 diizinkan karena OLT hampir selalu di jaringan privat). Driver nyata wajib memakai dialer yang sama saat menyambung, agar DNS rebinding tertutup. Driver `sim` tidak membuka koneksi jaringan sama sekali.
- **Teks dari perangkat tidak tepercaya:** semua string dari OLT dipangkas ke batas kolom dan dibersihkan; jumlah ONU per OLT dibatasi.
- **Audit:** pembuatan, perubahan, penghapusan, tes koneksi, dan polling tercatat di `activity_logs`.

### Catatan implementasi (ditemukan saat e2e)
`GET /olt-onus` memfilter OLT lewat parameter **`olt`** (UUID), bukan `olt_id`: `QueryGuard` global mewajibkan setiap parameter query berakhiran `_id` berupa bilangan bulat, sehingga `olt_id=<uuid>` selalu ditolak 400. Aturan itu sengaja tidak dilonggarkan; identifier publik berupa UUID memakai nama tanpa akhiran `_id`. Di JSON respons, `olt_id` tetap menjadi UUID OLT (bukan parameter query).

## 7. Alur

1. Admin menambah OLT (driver, host, kredensial). Kredensial dienkripsi sebelum disimpan.
2. Admin menekan **Tes koneksi**: `Probe` → `reported_name/version` dan status poll terisi.
3. NOC menekan **Poll**: `ListUnregisteredONUs` → rekonsiliasi: SN baru → `PENDING`; SN yang sudah ada → `last_seen_at` diperbarui (IGNORED tetap IGNORED); SN `PENDING/IGNORED` yang tidak lagi muncul → `RESOLVED`; SN `RESOLVED` yang muncul lagi → `PENDING`.
4. Operator melihat antrean di halaman OLT dan dapat mengabaikan/memulihkan sebuah entri.

## 8. Yang dibutuhkan dari OLT lab ZTE (untuk irisan 2)

Tidak ada yang ditulis dari ingatan, jadi irisan 2 menunggu bahan ini dari OLT lab. Cukup tempel keluarannya, atau beri akses SNMP + SSH read-only:

1. Model dan versi firmware OLT (C300/C320/C600, versi perangkat lunak).
2. Keluaran perintah yang menampilkan ONU belum terdaftar (belum diautorisasi), satu sampel berisi minimal satu ONU, dan satu sampel kosong.
3. Keluaran perintah informasi sistem/versi, untuk `Probe`.
4. Bila SNMP dipakai: community read-only dan hasil `snmpwalk` pada cabang yang memuat ONU belum terdaftar.
5. Perilaku sesi SSH/Telnet: prompt, paginasi (`--More--`), batas waktu, dan jumlah sesi bersamaan yang diizinkan.

## 9. Peta jalan

| Irisan | Isi | Syarat |
|---|---|---|
| **1** | Kerangka, simulator, inventaris, antrean, UI | — (dokumen ini) |
| 2 | Driver ZTE read-only: ONU belum terdaftar + `Probe`, polling periodik | Bahan §8 dari OLT lab |
| 3 | Optik (RX/TX) dan status port dari sisi OLT | Irisan 2 + keluaran nyata |
| 4 | Authorize ONU + profil, **lab-only**, dengan dry-run, persetujuan eksplisit, dan rollback | Irisan 2–3 terbukti; keputusan produk soal persetujuan |
| 5 | Menghubungkan ONU di OLT ↔ CPE di ACS (SN ↔ serial) agar zero-touch tersambung ujung ke ujung | Data nyata tentang format SN di perangkat kita |

Risiko utama sepanjang jalan: penulisan ke OLT produksi yang salah dapat memutus pelanggan. Karena itu irisan 4 dibatasi lab sampai ada bukti, dan tidak boleh diaktifkan bawaan.
