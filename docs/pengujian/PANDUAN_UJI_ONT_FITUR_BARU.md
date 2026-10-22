# Panduan Uji ONT Fisik — Fitur Baru (Faults, Query Bebas, Virtual Parameter, Aturan Otomasi)

Tambahan untuk `PENGUJIAN_LAPANGAN.md` dan `PANDUAN_UAT_ACS.html` (jangan diulang di sini: prasyarat jaringan, gladi, backup, dan daftar batasan ada di sana).
Dokumen ini hanya untuk fitur yang ditambahkan 22 Okt 2026, yang **baru lulus di simulator (e2e S31–S34) dan belum pernah menyentuh firmware nyata**.

**Mengapa ini penting:** simulator sudah pernah "lulus" sementara ONT nyata menolak (bug `xsi:type` → Fault 9002, UAT 21 Okt 2026). Setiap fitur di bawah bergantung pada
perilaku CPE yang tidak bisa ditiru simulator: kode fault yang dikirim firmware, nilai/satuan parameter vendor, dan apakah nilai yang ditulis benar-benar diterima.

## 0. Batas aman (baca dulu)

Lab UAT 21 Okt 2026 punya state yang **harus dikembalikan** (lihat §6). Agar uji ini tidak menambah beban pemulihan:

1. **Hanya tenant uji** (`DEPOK UAT`) dan hanya ONT lab. Jangan pernah menjalankan aturan otomasi pada tenant/ONT pelanggan.
2. **Jangan ubah setelan TR-069 ONT** (ACS URL, interval Inform, kredensial) dari ACS — jangan pakai aturan `set_parameter` ke `ManagementServer.*`.
3. **Jangan memakai aksi `reboot` di aturan** pada ONT fisik. Reboot untuk menguji pemicu BOOT dilakukan **manual** (tombol Reboot di detail perangkat) dan hanya pada ONT yang sedang tidak dipakai.
4. **Aksi tulis (`set_parameter`) hanya pada satu ONT yang ditunjuk sebagai "kelinci"** (disarankan ZTE F660 — satu-satunya yang tulis SSID/password-nya sudah terbukti), pada **SSID uji**, dan catat SSID asli **sebelum** mulai.
5. Setiap aturan diuji **dry-run dulu** (tombol *Uji* di halaman Otomasi), lalu diaktifkan, dan **dinonaktifkan/dihapus di akhir sesi**.
6. Nyalakan **Capture sesi** (tab Capture, maks 60 menit) pada ONT yang sedang diuji — itu bukti terbaik bila ada yang tak sesuai harapan. Isinya sudah menyamarkan rahasia.

## 1. Prasyarat

- [ ] ACS memakai kode terbaru (`git log` memuat `feat(automation)` dan `perf(register)`); migrasi ≥ `0041` terpasang (`docker compose run --rm migrate version`).
- [ ] 2–3 ONT lab sudah Inform ke tenant uji (ZTE F660, F609, Optictimes sesuai UAT); catat serial + MAC masing-masing.
- [ ] Login sebagai ADMIN tenant uji (peran NOC/VIEWER tidak bisa mengubah aturan — bagus untuk uji RBAC di §7).
- [ ] Backup database (lihat `PENGUJIAN_LAPANGAN.md` §7).
- [ ] Siapkan akses GUI ONT (halaman admin lokal) untuk **membandingkan** nilai yang ditampilkan ACS dengan yang ditampilkan perangkat.

## 2. Faults — `GET /faults`, halaman *Fault*

Tujuan: fault nyata dari firmware tercatat, tidak berlipat ganda, dan tertutup sendiri bila RPC yang sama kemudian berhasil. **Hasil sampingan paling berharga: kode fault
yang sebenarnya dikirim tiap vendor/firmware** (spesifikasi menyebut 9005, praktiknya sering 9002/9003/9007/9008).

| # | Langkah | Hasil yang diharapkan | Bukti |
|---|---|---|---|
| F-01 | Pada ONT X buat task **Set Parameter Values** (halaman Task → *Buat Task*) dengan nama **tidak ada**, mis. `InternetGatewayDevice.X_ACS_UJI.Trigger` = `1`; tunggu Inform berikutnya (atau picu Connection Request). | Task `FAILED`; satu baris di halaman **Fault** untuk X: kode, teks fault dari ONT, RPC `SET_PARAMETER_VALUES`, `kejadian 1×`. **Catat kode yang muncul.** | Tangkapan layar + Capture |
| F-02 | Ulangi tugas yang sama sekali lagi. | Tetap **satu** baris; `kejadian 2×`, `terakhir` berubah. | Layar |
| F-03 | Buat task **Set Parameter Values** yang **sah** (mis. SSID uji) pada ONT X. | Setelah task `COMPLETED`, baris fault `SET_PARAMETER_VALUES` untuk X **hilang otomatis**. | Layar |
| F-04 | Picu fault lagi, lalu tekan ✕ (Tutup fault) sebagai ADMIN. | Hilang dari daftar; muncul lagi bila ONT mengulang fault yang sama. | Layar |
| F-05 | Ulangi F-01 pada **tiap model** ONT lab. | Catat kode fault per model → masukkan ke §8 (kuirk vendor). | Tabel §8 |
| F-06 | Login sebagai NOC lalu VIEWER. | NOC boleh menutup fault; VIEWER hanya melihat (tanpa tombol ✕). | Layar |

> **Catatan data:** filter, virtual parameter, dan kondisi aturan hanya melihat parameter yang **sudah tersinkron ke ACS** (dari Inform atau GetParameterValues). Banyak ONT
> tidak menyertakan `UpTime`/SSID di Inform — tekan **Refresh Data** (tab Parameter) atau jalankan task Get Parameter Values dulu. Nilai kosong/"—" berarti *belum dilaporkan*, bukan bug.

## 3. Query bebas — `GET /devices?param=…&sort=…&fields=…` dan filter *Filter parameter*

Tujuan: filter atas **nilai yang benar-benar dilaporkan ONT** mengembalikan perangkat yang tepat; format nilai nyata (spasi, huruf besar, satuan) tidak mematahkan operator.

| # | Langkah | Hasil yang diharapkan |
|---|---|---|
| Q-01 | Di halaman Perangkat → *Filter parameter*: `InternetGatewayDevice.DeviceInfo.SoftwareVersion~prefix~<2–3 karakter awal versi ONT X>` | ONT X (dan hanya yang versinya berawalan itu) muncul. Bandingkan dengan versi di GUI ONT. |
| Q-02 | `…DeviceInfo.UpTime~gt~60` lalu `~lt~86400000` | Hanya ONT dengan uptime sesuai; tidak ada yang "nol" karena nilai non-angka. |
| Q-03 | `…WLANConfiguration.1.SSID~like~<sebagian SSID lab>` | ONT lab dengan SSID itu. (Huruf besar/kecil tidak dibedakan.) |
| Q-04 | `GET /devices?serial_number=<serial persis ONT X>` (Postman/curl dengan token) | Tepat 1 baris; serial sebagian saja → 0 baris. |
| Q-05 | Urutan *Inform terakhir (terbaru)* dan *Serial A–Z* | Urutan sesuai kolom yang tampil. |
| Q-06 | `GET /devices?params=<path SSID>&fields=serial_number,parameters` | Baris hanya berisi `id`, `serial_number`, `parameters` dengan SSID yang sama seperti di GUI ONT. |
| Q-07 | Coba filter pada parameter **password WiFi** (`…KeyPassphrase~prefix~a`) sebagai **VIEWER/NOC**, lalu sebagai ADMIN. | VIEWER/NOC: ditolak (400, "parameter rahasia"); ADMIN diizinkan. *Catat bila ONT tertentu menamai password dengan nama yang tidak terdeteksi sebagai rahasia → itu temuan keamanan.* |

## 4. Virtual parameter — halaman *Virtual Parameter*, kartu *Nilai turunan* di tab Parameter

Tujuan: ekspresi menghitung nilai benar dari data **nyata**, termasuk redaman optik yang path-nya belum dipetakan untuk ZTE.

| # | Langkah | Hasil yang diharapkan |
|---|---|---|
| V-01 | Buat `uptime_hari` = `round({InternetGatewayDevice.DeviceInfo.UpTime} / 86400, 2)`, satuan `hari`. Klik *Uji* pada ONT X. | Nilai cocok dengan uptime di GUI ONT (±1 menit). Kartu *Nilai turunan* di detail ONT X menampilkannya. |
| V-02 | Buat `uptime_terbaca` = `duration({…UpTime})`. | Format `1d 2h 3m 4s` benar. |
| V-03 | **Redaman optik ZTE:** gunakan *asisten sumber redaman optik* (tab Redaman Optik) untuk menemukan path RX mentah ONT; lalu buat `rx_dbm` = `round({<path mentah>} / 1000, 1)` (atau sesuai satuan yang ditemukan). | Nilai cocok dengan redaman di GUI ONT (toleransi ±0,5 dB). **Catat path + satuan** → tambahkan sebagai mapping model (bukan tebakan; lihat catatan UAT: path ZTE belum dipetakan). |
| V-04 | Buat `uptime_lintas_model` = `first({InternetGatewayDevice.DeviceInfo.UpTime}, {Device.DeviceInfo.UpTime})`. | Terhitung pada ONT TR-098 maupun (jika ada) TR-181. |
| V-05 | Coba ekspresi merujuk password WiFi. | Ditolak saat simpan (400). |
| V-06 | Parameter sumber belum pernah dilaporkan ONT. | Nilai "—" (bukan 0, bukan kosong). |

## 5. Aturan otomasi — halaman *Otomasi*

Tujuan: aturan memicu pada Inform **nyata**, aksinya benar-benar sampai dan diterima ONT, dan pagar anti-badai bekerja pada firmware nyata.
**Selalu mulai dari *Uji* (dry-run); aturan tulis hanya pada ONT kelinci (§0.4).**

| # | Aturan | Langkah & hasil yang diharapkan |
|---|---|---|
| A-01 | **Hanya-baca (aman):** pemicu *Setiap Inform*, kondisi `{@serial_number} == '<serial ONT X>'`, aksi `add_tag` (buat tag `uji-otomasi` dulu). | *Uji* → "AKAN dijalankan". Aktifkan. Pada Inform berikutnya: tag terpasang di ONT X, **satu** catatan di *Riwayat*, tanpa pemicuan ulang berikutnya (tag sudah ada = konvergen). |
| A-02 | **Kondisi dari nilai nyata:** `{…DeviceInfo.UpTime} > 600 and startswith({@oui}, '<OUI ONT>')`, aksi `get_parameter_values` (SSID). | Memicu hanya bila kondisi benar (bandingkan dengan uptime); task *GetParameterValues* muncul di halaman Task dan `COMPLETED`. Cooldown: tidak memicu lagi dalam jeda. |
| A-03 | **Event:** pemicu *Saat boot / bootstrap*, kondisi `contains({@events}, '1 BOOT')`, aksi `add_tag`. Reboot ONT **manual** (tombol Reboot di detail perangkat). | Setelah boot, ONT mengirim `1 BOOT`; aturan memicu satu kali. (Tanpa reboot, tidak memicu.) |
| A-04 | **Tulis (hanya ONT kelinci):** pemicu *Setiap Inform*, kondisi kosong, aksi `set_parameter` `InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID` = `'UJI-OTOMASI'`. | *Uji* menunjukkan "saat ini X, target UJI-OTOMASI". Setelah Inform: SSID di **GUI ONT** berubah. Inform berikutnya: tidak memicu lagi (konvergen). Lalu **ubah SSID kembali manual di GUI ONT** → Inform berikutnya aturan menegakkannya lagi **setelah jeda** (self-heal). Nonaktifkan aturan, **pulihkan SSID asli**. |
| A-05 | **Pagar suspend (opsional):** `set_parameter` ke parameter **read-only** (mis. `…DeviceInfo.SerialNumber` = `'x'`), jeda 5 menit, "berhenti setelah" 1. | ONT menolak (Fault 9008 atau serupa — tercatat di *Fault*); aturan memicu, lalu setelah jeda **di-suspend** (catatan *di-suspend* di Riwayat) dan tidak memicu lagi. *Buka kembali* di Riwayat → bisa memicu lagi. Catat kode fault. |
| A-06 | **RBAC:** login NOC. | Melihat aturan & Riwayat, tanpa tombol buat/edit/hapus/aktif. |
| A-07 | **Pagar task tertunda:** buat tugas Reboot/GPV lalu matikan ONT sebelum Inform. | Selama task tertunda, aturan tidak memicu perangkat itu. |

> `reboot` dan penulisan ke `ManagementServer.*` **tidak diuji pada ONT fisik** (§0). Pagar terhadap loop reboot sudah diuji di unit test dengan jam palsu.

## 6. Pembersihan & pemulihan state lab (WAJIB di akhir sesi)

Dari catatan UAT 21 Okt 2026 — pastikan semuanya, bukan hanya yang terkait uji ini:

- [ ] Semua aturan otomasi uji **dinonaktifkan lalu dihapus**; semua virtual parameter uji dihapus; tag `uji-otomasi` dihapus; fault uji ditutup.
- [ ] SSID/password ONT kelinci **dikembalikan ke nilai asli** (catat di §0.4 sebelum mulai).
- [ ] Capture sesi dimatikan (kedaluwarsa otomatis ≤ 60 menit, tetapi matikan eksplisit).
- [ ] Windows Firewall dinyalakan kembali + rule TCP 17547 (sempat `netsh advfirewall … off`).
- [ ] Setelan TR-069 ketiga ONT dikembalikan ke ACS Telkom (`acs.telkom.net:9090`, interval 43200; password asli harus diketahui).
- [ ] Password `e2e-super` diganti.
- [ ] Temuan terbuka dari UAT yang belum selesai: password WiFi tersimpan plaintext di `tasks.parameters` (hanya diredaksi di API) — sudah dicatat; jangan dianggap hilang.

## 7. Lembar hasil

Salin tabel ini ke laporan; isi per ONT. Status: ✅ lulus · ⚠️ lulus dengan catatan · ❌ gagal · ⏳ tidak diuji. **Setiap ❌/⚠️ wajib menyertakan Capture/tangkapan layar.**

| ID | ONT / model / firmware | Status | Catatan & bukti |
|---|---|---|---|
| F-01…F-06 | | | |
| Q-01…Q-07 | | | |
| V-01…V-06 | | | |
| A-01…A-07 | | | |

## 8. Kuirk yang ditemukan (isi saat uji → jadi data, bukan kode)

| Model / firmware | Kode fault pada parameter tak ada | Kode fault pada parameter read-only | Path redaman RX (mentah) | Satuan/skala | Catatan |
|---|---|---|---|---|---|
| | | | | | |

Path & skala optik masuk lewat `vendor_parameter_mappings` (`value_scale`), **bukan** percabangan kode (CLAUDE.md). Fault code vendor-spesifik yang perlu perlakuan khusus
→ pertimbangkan adapter di `internal/vendor_adapter/` hanya bila perilakunya nonstandar, bukan sekadar kode berbeda.

## 9. Kriteria lulus

- Semua F-, Q-, V-, A-01…A-04 ✅ atau ⚠️ dengan catatan dan **tanpa** kehilangan data/konfigurasi ONT; state lab pulih (§6).
- Tidak ada aturan yang memicu di luar tenant uji atau ONT non-kelinci.
- Setiap ⚠️/❌ punya tindak lanjut (isu, mapping data, atau perbaikan) sebelum fitur dipakai pada tenant sungguhan.
