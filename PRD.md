# PRD — Auto Configuration Server (ACS) Multi-Vendor

**Versi:** 0.1 (Draft)
**Status:** Draft untuk review
**Pemilik Produk:** IT Development
**Terakhir diperbarui:** 2026-08-18

---

## 1. Ringkasan Eksekutif

ACS (Auto Configuration Server) adalah sistem manajemen perangkat CPE (Customer Premises Equipment) — ONT/ONU, router, dan access point — berbasis protokol **TR-069 (CWMP — CPE WAN Management Protocol)**. Sistem ini memungkinkan operator jaringan (ISP) melakukan provisioning, konfigurasi, monitoring, dan troubleshooting perangkat pelanggan secara terpusat dan otomatis, lintas berbagai brand perangkat (ZTE, Huawei, FiberHome, Nokia, dan vendor lain).

Saat ini proses konfigurasi CPE dilakukan manual per-vendor (masing-masing punya web GUI, CLI, atau tools proprietary sendiri), yang tidak efisien untuk skala ribuan–puluhan ribu pelanggan dan rawan human error. ACS menggantikan proses ini dengan satu control plane terpusat.

## 2. Latar Belakang & Masalah

- Operator FTTH mengelola perangkat pelanggan dari berbagai vendor karena alasan harga, ketersediaan stok, dan kontrak proyek berbeda-beda per periode.
- Setiap vendor punya tools manajemen sendiri (mis. ZTE ZXONE/U31, Huawei U2000, FiberHome ANM, Nokia NFM-P/ONT Manager) — operasional tim NOC harus berpindah-pindah tools.
- Tidak ada satu sumber kebenaran (single source of truth) untuk status, konfigurasi, dan histori perubahan tiap CPE.
- Provisioning pelanggan baru (activation) masih melibatkan langkah manual di sisi CPE, memperlambat time-to-activate.
- Troubleshooting gangguan pelanggan (redaman optik, WiFi, WAN) butuh akses ke CPE yang sering di belakang NAT dan tidak reachable langsung dari NOC.
- Tidak ada audit trail terpusat siapa mengubah parameter apa pada CPE mana.

## 3. Tujuan

1. Satu platform ACS yang mendukung banyak vendor CPE melalui protokol standar TR-069/CWMP (TR-098 & TR-181 data model).
2. Zero-touch provisioning: CPE baru yang online otomatis dikenali dan dikonfigurasi sesuai profil tanpa intervensi manual.
3. Kemampuan mengirim perintah manajemen (get/set parameter, reboot, factory reset, firmware upgrade) ke CPE kapan saja, termasuk saat CPE idle (via Connection Request).
4. Visibilitas status real-time seluruh perangkat: online/offline, versi firmware, level redaman optik, parameter WiFi/WAN.
5. Audit trail lengkap: siapa melakukan perubahan apa, kapan, ke perangkat mana.
6. Arsitektur yang mudah diperluas untuk menambah vendor/model baru tanpa mengubah core system (vendor parameter mapping layer).

## 4. Ruang Lingkup

### 4.1 In-Scope (Fase 1)

- **CWMP Server (Northbound dari CPE):** endpoint HTTP/HTTPS yang menerima `Inform` dari CPE, memproses session CWMP penuh (Inform → InformResponse → RPC exchange → session close).
- **Task Queue / RPC ke CPE:** `GetParameterValues`, `SetParameterValues`, `GetParameterNames`, `AddObject`, `DeleteObject`, `Reboot`, `FactoryReset`, `Download` (firmware/config), `ScheduleInform`.
- **Connection Request:** ACS dapat membangunkan CPE yang sedang idle untuk membuka sesi baru (via HTTP Connection Request ke CPE, dengan fallback STUN untuk CPE di belakang NAT — fase 2 jika dibutuhkan).
- **Multi-vendor abstraction:** layer pemetaan parameter logis (mis. `wifi.2g.ssid`) ke path TR-069 spesifik per vendor/model/data-model-version (TR-098 vs TR-181).
- **Zero-touch provisioning (ZTP):** aturan auto-assign profil provisioning berdasarkan OUI, product class, atau pola serial number saat perangkat pertama kali `BOOTSTRAP`.
- **Provisioning profile/template:** kumpulan parameter default per vendor/model yang diterapkan otomatis ke CPE baru atau saat reset.
- **Firmware management:** upload firmware per vendor/model, penjadwalan upgrade massal atau per-device.
- **Inventory & monitoring device:** daftar perangkat, status online/offline, histori event (`BOOT`, `PERIODIC`, `VALUE CHANGE`, dll), metrik optik (RX/TX power) untuk ONT GPON/EPON.
- **Diagnostics:** trigger diagnostic test bawaan TR-069 (ping, traceroute, WiFi scan) dan simpan hasilnya.
- **Multi-tenant:** mendukung beberapa entitas/brand ISP dalam satu platform (mengikuti pola arsitektur multi-tenant yang sudah digunakan pada sistem BSS/OSS lain di organisasi).
- **REST API internal:** dikonsumsi oleh sistem lain (BSS/OSS, portal NOC, aplikasi pelanggan) untuk memicu task, membaca status device, dsb.
- **Audit log:** semua aksi admin/API (siapa mengubah apa) dan histori perubahan parameter per device.
- **Role-based access control:** minimal role Superadmin, Admin (per tenant), NOC/Operator, Viewer.

### 4.2 Out-of-Scope (Fase 1)

- Protokol **TR-369 / USP** (User Services Platform, penerus TR-069) — dipertimbangkan di roadmap fase berikutnya.
- Manajemen perangkat non-TR-069 (mis. perangkat yang hanya bisa dikonfigurasi via SNMP/Telnet proprietary) — bisa jadi adapter terpisah di masa depan.
- Portal self-service pelanggan (end-user mengubah SSID/password sendiri) — dianggap sistem terpisah yang memanggil REST API ACS.
- Voice/VoIP (TR-104) dan STB/IPTV data model spesifik — hanya data umum WAN/LAN/WiFi/Device Info di fase 1.
- Billing/mediation — tetap di sistem BSS terpisah, ACS hanya menyediakan status teknis perangkat.

## 5. Vendor & Perangkat Target

| Vendor | Contoh tipe perangkat | Data model |
|---|---|---|
| ZTE | ONT/ONU GPON (F6xx/F660/F6600 series) | TR-098 & TR-181 (tergantung firmware) |
| Huawei | ONT/ONU GPON (HG8xxx series) | TR-098 & TR-181 |
| FiberHome | ONT/ONU GPON (HG6xxx/AN5xxx series) | TR-098 |
| Nokia (eks Alcatel-Lucent) | ONT GPON (G-xxx series) | TR-181 |
| Vendor lain (VSOL, BDCOM, Dasan Zhone, dll) | ONT/ONU generik | TR-098/TR-181 |

Daftar vendor bersifat terbuka — arsitektur harus memungkinkan penambahan vendor baru cukup dengan menambah data (vendor, OUI, parameter mapping), bukan perubahan kode inti. Lihat `TECH.md` bagian "Vendor Extensibility".

## 6. Pengguna & Peran

| Peran | Kebutuhan Utama |
|---|---|
| **NOC / Operator** | Melihat status device, trigger diagnostic, restart/reboot device, lihat histori event, buka tiket berdasarkan data ACS |
| **Admin (per tenant)** | Kelola profil provisioning, aturan zero-touch, kelola firmware, kelola user tenant |
| **Superadmin** | Kelola tenant, vendor & parameter mapping global, kelola akses lintas tenant |
| **Sistem eksternal (BSS/OSS, portal aktivasi)** | Memicu provisioning via REST API saat pelanggan baru aktif, membaca status device untuk ditampilkan di portal |

## 7. Functional Requirements

### 7.1 CWMP Session Handling
- FR-1: Sistem menerima `Inform` RPC dari CPE via HTTP/HTTPS, memvalidasi kredensial (Basic/Digest Auth per device atau per profil), dan membalas `InformResponse`.
- FR-2: Sistem mempertahankan session CWMP hingga CPE mengirim `http empty POST` (tanda sesi selesai) atau timeout tercapai.
- FR-3: Sistem mencatat setiap event code yang dikirim CPE (`0 BOOTSTRAP`, `1 BOOT`, `2 PERIODIC`, `4 VALUE CHANGE`, `6 CONNECTION REQUEST`, `7 TRANSFER COMPLETE`, dll).
- FR-4: Saat event `0 BOOTSTRAP` diterima (indikasi factory-reset/first-contact), sistem otomatis mengevaluasi aturan zero-touch provisioning.

### 7.2 Task & RPC Management
- FR-5: Operator/sistem eksternal dapat mengantre task RPC (`SetParameterValues`, `GetParameterValues`, `Reboot`, dst.) untuk device tertentu melalui REST API.
- FR-6: Task dieksekusi pada session CWMP berikutnya (periodic inform berikutnya) atau memicu **Connection Request** agar dieksekusi segera.
- FR-7: Sistem mencatat status tiap task: `PENDING → SENT → COMPLETED/FAILED/TIMEOUT`, termasuk response/error dari CPE.
- FR-8: Task mendukung retry dengan batas maksimum percobaan yang dapat dikonfigurasi.
- FR-9: Task dapat diprioritaskan (mis. reboot darurat lebih prioritas dari sync parameter rutin).

### 7.3 Multi-Vendor Abstraction
- FR-10: Sistem menyimpan pemetaan "parameter logis" ke path TR-069 aktual per kombinasi vendor + model + data-model-version.
- FR-11: Saat operator mengirim perintah menggunakan parameter logis (mis. `wifi.5g.ssid`), sistem menerjemahkan otomatis ke path sesuai vendor tujuan sebelum dikirim ke CPE.
- FR-12: Sistem tetap mendukung pengiriman raw TR-069 parameter path untuk kasus vendor-specific yang belum dipetakan.

### 7.4 Zero-Touch Provisioning
- FR-13: Sistem dapat mencocokkan CPE baru berdasarkan OUI, product class, dan/atau pola serial number ke sebuah provisioning profile.
- FR-14: Saat kecocokan ditemukan, sistem otomatis mengantre task `SetParameterValues` sesuai isi profil ke device tersebut.
- FR-15: Jika tidak ada aturan yang cocok, device tetap tercatat di inventory dengan status "unprovisioned" menunggu tindakan manual.

### 7.5 Provisioning Profile
- FR-16: Admin dapat membuat, mengubah, menonaktifkan profil provisioning berisi kumpulan parameter logis/raw + nilai default.
- FR-17: Profil dapat berlaku umum (semua vendor) atau spesifik vendor/model.
- FR-18: Perubahan pada profil tidak otomatis mendorong ulang ke device yang sudah terprovisioning (harus eksplisit re-apply) — mencegah perubahan massal tidak sengaja.

### 7.6 Firmware Management
- FR-19: Admin dapat mengunggah file firmware, terasosiasi ke vendor/model tertentu, dengan checksum untuk validasi integritas.
- FR-20: Admin dapat menjadwalkan upgrade firmware untuk satu device, grup device, atau seluruh device suatu model.
- FR-21: Sistem mencatat histori upgrade (versi lama → baru, waktu, hasil) per device.

### 7.7 Monitoring & Diagnostics
- FR-22: Sistem menampilkan status device: online/offline (berdasarkan `last_inform_at` vs periodic interval yang diharapkan), versi firmware, uptime.
- FR-23: Untuk ONT GPON/EPON, sistem menyimpan metrik optik (RX power, TX power, temperature, voltage, bias current) setiap kali tersedia dari parameter TR-069.
- FR-24: Operator dapat memicu diagnostic test standar TR-069 (ping, traceroute, WiFi scan) dan melihat hasilnya.
- FR-25: Sistem menyediakan histori event per device (kapan boot, kapan value change, dsb) untuk keperluan troubleshooting.

### 7.8 Multi-Tenant & Akses
- FR-26: Data device, profil, dan firmware terpisah secara logis per tenant.
- FR-27: User memiliki role yang membatasi aksi dan cakupan tenant yang bisa diakses.
- FR-28: Superadmin dapat mengelola data referensi global (vendor, parameter mapping) yang dipakai lintas tenant.

### 7.9 Audit & Log
- FR-29: Setiap perubahan data master (profil, aturan ZTP, firmware, user) tercatat di activity log: siapa, kapan, aksi apa.
- FR-30: Setiap perubahan nilai parameter device (baik dari operator maupun hasil sync dari CPE) memiliki jejak waktu.

## 8. Non-Functional Requirements

| Kategori | Requirement |
|---|---|
| **Skalabilitas** | Mendukung minimal puluhan ribu device terdaftar dan ribuan sesi CWMP concurrent tanpa degradasi signifikan; desain harus horizontal-scalable (stateless app server, state di database/queue). |
| **Performa** | Pemrosesan satu siklus Inform → InformResponse rata-rata < 300ms (tanpa RPC tambahan); task queue polling per session tidak menambah beban N+1 berlebihan. |
| **Ketersediaan** | Target uptime 99.5% untuk endpoint CWMP (CPE terus mencoba reconnect bila gagal, namun downtime lama mengganggu monitoring real-time). |
| **Keamanan** | Autentikasi CPE↔ACS (Basic/Digest Auth minimal, TLS wajib untuk endpoint publik); kredensial connection request per device disimpan terenkripsi; RBAC untuk seluruh operasi admin; rate limiting pada REST API publik. |
| **Observability** | Structured logging per session/task; metrik jumlah device online, task queue depth, error rate per vendor; alerting saat backlog task menumpuk. |
| **Extensibility** | Menambah vendor/model baru tidak memerlukan deploy ulang kode — cukup entri data (vendor, OUI, parameter mapping). |
| **Auditability** | Seluruh tabel data master menggunakan jejak audit standar (created/updated/deleted by & at) dan soft-delete, bukan hard delete. |
| **Data Retention** | Log event & histori parameter mentah (high-volume) memiliki kebijakan retensi/rotasi terpisah dari data master. |

## 9. Asumsi

- Perangkat CPE yang didukung mengimplementasikan TR-069 (CWMP) sesuai standar Broadband Forum, meski dengan variasi minor antar-vendor/firmware.
- ACS akan diakses CPE melalui endpoint HTTPS publik (perlu domain/IP publik dan sertifikat TLS valid).
- Sistem ini adalah komponen dalam ekosistem yang lebih besar (BSS/OSS ISP) dan akan berintegrasi dengan sistem provisioning pelanggan yang sudah ada — bukan pengganti sistem billing/AAA.
- Multi-tenant diasumsikan dibutuhkan mengikuti pola arsitektur SaaS yang sudah dipakai pada sistem BSS/OSS lain di organisasi; bila ACS ini hanya untuk satu ISP internal, tabel `tenants` tetap ada namun cukup diisi satu baris.
- Connection Request mengasumsikan CPE reachable langsung (port terbuka) di fase awal; dukungan STUN/NAT traversal penuh masuk fase berikutnya bila banyak CPE di belakang CGNAT tanpa port forwarding.

## 10. Kriteria Sukses

- Waktu provisioning pelanggan baru (dari CPE menyala pertama kali sampai layanan aktif) turun signifikan dibanding proses manual saat ini.
- Tim NOC dapat melakukan troubleshooting dasar (cek status, reboot, cek redaman optik) dari satu dashboard tanpa login ke tools masing-masing vendor.
- Semua device baru dari vendor yang didukung dapat ter-zero-touch-provision tanpa sentuhan manual.
- Tidak ada perubahan skema/kode inti yang diperlukan saat menambah vendor baru ke-5, ke-6, dst.

## 11. Roadmap Bertahap (Indikatif)

| Fase | Cakupan |
|---|---|
| **Fase 1** | Core CWMP server, task queue, ZTP dasar, provisioning profile, inventory & monitoring dasar, RBAC, REST API internal |
| **Fase 2** | Firmware management penuh, diagnostics lanjutan, metrik optik & dashboard historis, Connection Request dengan STUN untuk CPE di belakang NAT |
| **Fase 3** | Dukungan TR-369/USP, self-service integration, analytics/alerting lanjutan (anomaly detection redaman optik, dsb) |

## 12. Risiko & Mitigasi

| Risiko | Mitigasi |
|---|---|
| Variasi implementasi TR-069 antar-vendor/firmware tidak selalu sesuai spec | Layer parameter mapping + kemampuan override/raw path per device; testing lab per vendor sebelum rollout massal |
| CPE di belakang CGNAT tidak reachable untuk Connection Request | Fallback: task dieksekusi saat periodic inform berikutnya; STUN di fase 2 |
| Volume data tinggi (parameter/event log) membengkakkan database | Kebijakan retensi & partisi tabel log terpisah dari tabel master; lihat `TECH.md` |
| Kredensial connection request bocor | Enkripsi at-rest, akses terbatas RBAC, rotasi berkala |
