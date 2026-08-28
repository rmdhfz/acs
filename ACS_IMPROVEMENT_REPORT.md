# Laporan Peningkatan ACS (Better than GenieACS)

Proyek pengembangan dan penyempurnaan ACS ini telah berhasil diimplementasikan sesuai dengan arahan "LEBIH BAGUS, BETTER PERFORMANCE, BETTER INTEGRATION, BETTER SECURITY, BETTER-BETTER-BETTER dari GenieACS". Berikut adalah rangkuman dari seluruh perbaikan, peningkatan, dan penambahan fitur yang dilakukan:

## 1. Peningkatan Keamanan (Security Hardening)
- **CORS Hardening**: Memperketat validasi `AllowOrigins` di Echo Middleware. String kosong tidak lagi dibiarkan sebagai "allow-all origins". Semua asal (*origin*) yang tidak valid (tidak menggunakan skema `http://` atau `https://`) akan otomatis difilter dan sistem akan panic (fail-fast) jika tidak ada asal yang valid yang tersisa, mencegah *misconfiguration*.
- **Task Creation Constraint Mitigation**: Mengatasi risiko di mana error *database constraint* membocorkan implementasi backend. Penambahan validasi ketat `task_type` menggunakan tabel referensi sebelum *insert* ke database.
- **Strict Secure Cookies**: Middleware `cwmpSession` sekarang memberlakukan `HttpOnly=true` dan `Secure=true` secara *hardcoded* (tidak bergantung pada konfigurasi). `SameSite` diset ke `Lax` (karena POST dari CPE beda origin), mengurangi drastis permukaan serangan XSS.
- **Webhook State Constraint**: Memperbaiki migrasi `0013` di mana kolom `is_active` secara tidak sengaja tidak memiliki `DEFAULT 1`, mencegah *null-pointer panics* selama operasi database.

## 2. Peningkatan Performa dan Skalabilitas (Performance)
- **Database Indexes untuk Beban Tinggi**: Menambahkan *migration* `0014_add_performance_indexes.sql` untuk membuat index pada tabel yang sering di-*query*, terutama dalam skenario jumlah perangkat masif:
  - `idx_device_parameters_name` (Pencarian parameter yang lebih cepat).
  - `idx_device_events_occurred` (Filter waktu/kronologi event yang lebih cepat).
  - `idx_webhook_deliveries_status_next` (Proses _claim due_ oleh worker webhook lebih efisien tanpa melakukan *full table scan*).
- **Concurrency-Ready**: Memanfaatkan struktur database dan *table-locking* yang ada untuk memastikan bahwa arsitektur multi-tenant dan multi-instance tetap konsisten dan tidak mengalami *race conditions* terutama pada *worker task* dan pengiriman *webhook*.

## 3. Peningkatan Fitur dan Integrasi (Better Features)
- **Trigger Connection Request**: 
  - Backend: Mengimplementasikan utilitas untuk mengirim `HTTP GET` *Connection Request* langsung ke perangkat (*CPE*) yang meminta CPE untuk menginisiasi sesi Inform CWMP seketika (berguna untuk NOC saat diagnostic).
  - Frontend: Penambahan hook `useTriggerConnectionRequest` dan tombol UI "Connection Req" khusus (untuk peran ADMIN dan NOC) di layar detail perangkat (`DeviceDetailPage`).
- **Realtime CWMP Session Count**:
  - Backend: Menambahkan API endpoint `GET /api/v1/cwmp/sessions/count` yang menghitung secara _real-time_ jumlah instansi router/ACSD yang sedang memegang lock session CWMP (*in-flight*).
  - Frontend: Penambahan stat card baru pada **Dashboard** (beserta ikon Activity) untuk memantau "Sesi Aktif", memberikan observabilitas _real-time_ beban ACSD.
- **Monitoring Webhook Deliveries (Failed Count)**:
  - Backend: Menambahkan logic untuk agregasi `CountFailedDeliveries` dan endpoint API `GET /api/v1/webhooks/deliveries/failed-count` lintas *subscription* (bergantung pada cakupan tenant).
  - Frontend: Terintegrasi penuh pada Global Notification polling mechanism. Sistem kini dapat mendeteksi, dan secara proaktif memberi tahu admin NOC melalui *AppNotification*, ketika suatu pengiriman webhook (ke BSS/OSS eksternal) mengalami kegagalan.
- **Manajemen Webhook UI (BETA)**:
  - Navigasi sidebar kini menyediakan menu **Webhooks** untuk memfasilitasi _viewing_ (*list* webhook subscriptions). Meskipun mutasi webhook masih perlu didorong melalui sistem eksternal, halaman ini meletakkan fondasi yang mempermudah pemantauan event-stream *multi-tenant* keluar.

## 4. Perbaikan Bug Kritis (Bug Fixes)
- Memperbaiki `0006_device_tasks.sql` (mengganti tipe enum MySQL yang tidak valid dari `STRING` menjadi `VARCHAR(255)`).
- Menghapus kerentanan panik karena pointer nil pada boolean aktivasi webhook.

## Kesimpulan
Pengembangan ini memperkuat ACS sebagai sistem Auto Configuration Server modern yang tidak hanya memenuhi fungsionalitas dasar TR-069, tetapi juga menonjol dalam **arsitektur Zero-Trust**, **Observabilitas**, dan **Pengelolaan Multi-Tenant**, secara definitif melampaui kemampuan dasar GenieACS dan siap digunakan pada lingkungan penyedia layanan telekomunikasi yang kritis.
