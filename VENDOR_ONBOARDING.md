# VENDOR_ONBOARDING.md — Playbook Menambah Vendor/Model CPE Baru

Panduan operasional untuk superadmin: menambah dukungan vendor atau model CPE baru di ACS. Baca `TECH.md` §5 dan §5.1 untuk latar belakang arsitektur sebelum mengikuti langkah di bawah — dokumen ini adalah versi eksekusi praktisnya, bukan pengganti keduanya.

**Prinsip inti (jangan dilanggar):** menambah vendor/model baru adalah **operasi data**, bukan perubahan kode. Kalau kamu (atau siapa pun) menemukan diri sendiri sedang menulis kode Go untuk menangani vendor tertentu di luar `internal/vendor_adapter/`, berhenti — itu artinya ada yang salah pendekatan. Lihat §5 di bawah untuk pengecualian yang sah.

---

## 1. Prasyarat

- Akses superadmin ke ACS Console (menu **Catalog Vendor** hanya muncul untuk role `SUPERADMIN`).
- Informasi vendor: nama resmi, OUI (Organizationally Unique Identifier, 6 digit hex) dari MAC address perangkatnya — bisa dicek dari label perangkat atau [IEEE OUI registry](https://standards-oui.ieee.org/) kalau belum tahu.
- Sample data dari minimal satu unit CPE nyata (atau dokumentasi TR-069 resmi vendor tsb): daftar path TR-069 untuk parameter yang mau dipetakan (WiFi SSID, PPPoE username, dst), dan apakah device pakai `InternetGatewayDevice.*` (TR-098) atau `Device.*` (TR-181).

## 2. Langkah 1 — Daftarkan Vendor & OUI

Buka **Catalog Vendor > Vendors**.

1. Klik **Vendor Baru** — isi `Code` (singkat, huruf besar, mis. `ZTE`, `HUAWEI`) dan `Nama` (nama resmi lengkap).
2. Setelah vendor dibuat, klik **+ OUI** di baris vendor tsb untuk tiap OUI yang dipakai vendor ini (satu vendor bisa punya banyak OUI dari model/lini produk berbeda). Format 6 digit hex tanpa pemisah (mis. `3C6A9D`).
3. OUI yang sudah terdaftar tampil sebagai badge di kolom "OUI Terdaftar" pada tabel vendor.

**Kenapa ini penting:** saat CPE baru pertama kali mengirim `Inform`, ACS mencocokkan `OUI` di `DeviceId` terhadap tabel ini (`internal/usecase/device/service.go#FindOrCreateFromInform`) untuk otomatis me-resolve `vendor_id` device tsb. Tanpa OUI terdaftar, device tetap tercatat (FR-15) tapi `vendor_id` akan `NULL` selamanya sampai di-assign manual.

## 3. Langkah 2 — Daftarkan Device Model

Buka **Catalog Vendor > Device Models**, pilih vendor yang baru dibuat.

1. Klik **Model Baru**.
2. Isi `Nama Model` (mis. `F670L`) dan `Product Class` (nilai persis dari field `ProductClass` yang dikirim CPE saat Inform — cek dari data sample atau log Inform pertama device tsb kalau belum tahu).
3. Pilih `Device Type` dan `Data Model Version` (TR-098/`InternetGatewayDevice.*` atau TR-181/`Device.*`) dari dropdown — ini referensi `ref_device_types`/`ref_data_model_versions` yang sudah ada, bukan input bebas.

Device model bersifat **opsional** untuk resolusi dasar (device tetap ter-assign `vendor_id` dari OUI saja), tapi dibutuhkan kalau kamu mau override parameter mapping yang lebih spesifik dari sekadar level vendor (lihat langkah berikut), atau untuk asosiasi firmware per-model.

## 4. Langkah 3 — Petakan Parameter (Vendor Parameter Mapping)

Buka **Catalog Vendor > Parameter Mappings**, pilih vendor.

Untuk tiap **logical key** yang tim operasi butuhkan (cek daftar konvensi logical key yang sudah dipakai tim di profile provisioning existing — buka **Provisioning > Provisioning Profiles** dan lihat parameter yang sudah ada di profil vendor lain sebagai referensi penamaan, supaya konsisten, mis. `wifi.5g.ssid`, `wan.pppoe.username`):

1. Klik **Mapping Baru**.
2. Isi `Logical Key` (snake/dot-case, deskriptif, konsisten dengan mapping vendor lain).
3. Isi `TR-069 Path` — path **persis** sesuai data model vendor ini (mis. `InternetGatewayDevice.LANDevice.1.WLANConfiguration.5.SSID` untuk TR-098, atau `Device.WiFi.SSID.5.SSID` untuk TR-181). Salah satu huruf pun akan membuat `SetParameterValues`/`GetParameterValues` gagal di CPE nyata.
4. Pilih `Data Model Version` yang sesuai.
5. Isi `Device Model` **hanya** kalau path ini berbeda dari default vendor untuk model tsb spesifik (override) — kosongkan untuk berlaku ke semua model vendor ini.

**Tidak perlu memetakan semua parameter yang mungkin ada** — cukup logical key yang benar-benar dipakai tim operasi (biasanya: WiFi SSID/password 2.4G & 5G, PPPoE username/password, dan parameter diagnostic optik untuk ONT GPON). Operator tetap bisa mengirim raw TR-069 path langsung untuk kasus yang belum dipetakan (lihat TECH.md §5 — mapping adalah *convenience layer*, bukan satu-satunya jalur).

## 5. Kapan Butuh Kode, Bukan Data

Operasi data di atas cukup untuk **>95% kasus**. Kamu baru butuh menyentuh kode (`internal/vendor_adapter/<vendor>/`) kalau vendor tsb punya kuirk protokol yang **benar-benar menyimpang dari spec CWMP standar** — bukan sekadar path parameter yang beda (itu sudah ditangani mapping di atas). Contoh yang SAH butuh adapter kode:

- Urutan RPC yang nonstandar (mis. CPE menolak `GetParameterValues` sebelum `SetParameterValues` pertama, di luar spec).
- Encoding response XML yang menyimpang dari spec (mis. tag tambahan nonstandar yang membuat parser gagal).
- Kuirk autentikasi/session yang spesifik vendor.

Contoh yang **BUKAN** alasan sah untuk kode (harus lewat data/mapping di atas):
- Path parameter yang berbeda dari vendor lain — itu tugas Parameter Mapping.
- Root data model TR-098 vs TR-181 — sudah ditangani `data_model_version_id` per device model.
- Vendor extension object (`X_<OUI>_...`) untuk fitur nonstandar seperti optical diagnostics tertentu — tetap petakan sebagai logical key + raw path seperti biasa, path-nya saja yang mengandung prefix `X_<OUI>_`.

Kalau ragu apakah suatu kuirk butuh adapter kode, tanyakan ke tim engineering sebelum menulis kode — jangan langsung asumsikan butuh percabangan `if vendor == "..."` di tempat lain selain `vendor_adapter/`.

## 6. Verifikasi Setelah Selesai

Jangan anggap selesai hanya karena data sudah diinput — validasi end-to-end:

1. Kalau ada unit CPE fisik vendor ini tersedia: sambungkan ke ACS (arahkan URL Inform CPE ke endpoint CWMP ACS), tunggu Inform pertama, lalu cek di **Devices** apakah device baru muncul dengan `vendor_id`/vendor name yang benar terisi (bukan "Vendor belum diketahui").
2. Kalau belum ada unit fisik: minimal cek data ter-input benar via **Catalog Vendor** (OUI badge muncul, device model tersimpan, parameter mapping tersimpan dengan path yang benar — baca ulang, salah ketik path adalah kesalahan paling umum).
3. Coba buat **Provisioning Profile** untuk vendor ini di halaman **Provisioning**, isi minimal satu parameter pakai autocomplete logical key (kalau mapping sudah benar, logical key yang baru dibuat akan muncul di datalist saat vendor profil dipilih) — ini validasi tidak langsung bahwa mapping tersimpan dan bisa di-resolve.
4. Untuk parameter kritis (WiFi, PPPoE): kalau memungkinkan, uji kirim task `SetParameterValues` pakai logical key ke satu device sungguhan dan konfirmasi CPE benar-benar berubah konfigurasinya — jangan asumsikan path yang "kelihatannya benar" dari dokumentasi vendor pasti cocok dengan firmware aktual yang dipakai pelanggan (variasi antar-firmware adalah risiko yang eksplisit disebut PRD.md §12).

## 7. Kalau Salah Input

- Vendor/device model/mapping yang salah bisa diedit ulang dengan submit form yang sama (`Mapping Baru` bersifat upsert berdasarkan kombinasi vendor+data_model_version+device_model+logical_key — submit ulang dengan logical_key sama akan menimpa path lama).
- Untuk vendor/device model yang salah total dan perlu dihapus: belum ada tombol hapus di UI Catalog Vendor saat ini (baru create/list) — hubungi tim engineering untuk soft-delete manual via database kalau memang diperlukan (ingat: soft-delete via `is_deleted`, bukan `DELETE` fisik, sesuai konvensi skema di `CLAUDE.md`).
