# LOGIN.md — Cara Masuk ke ACS

Panduan singkat login untuk Tim Operations dan developer. Versi ringkas ada di `LOGIN_INSTRUCTION.md`; versi lengkap bergambar di `PANDUAN_LOGIN_ACS.pdf`.

> **Tidak ada akun bawaan (default account).** Akun pertama harus dibuat lewat `seed-admin` (lihat §2). Jangan menulis password di file ini, di commit, atau di chat tim.

## 1. Alamat

| Lingkungan | UI | REST API |
|---|---|---|
| Dev lokal (Vite) | http://localhost:5173/login | http://localhost:18080/api/v1 |
| Docker compose | sesuai reverse proxy / port yang dipublish | port `ACS_HTTP_ADDR` (default `:8080` di dalam container) |

Endpoint CPE (TR-069) **terpisah** dari UI: `http://<host>:7547/cwmp` (`ACS_CWMP_ADDR`). Login UI/API tidak berlaku di sana — CPE memakai kredensial CWMP per-tenant (Basic Auth).

## 2. Membuat akun SUPERADMIN pertama

```bash
docker compose exec acsd /app/seed-admin -username <USERNAME> -password <PASSWORD> -email <EMAIL>
```

- Pakai password kuat dan unik; ganti segera setelah login pertama (§5).
- Perintah ini hanya untuk bootstrap. Akun berikutnya dibuat dari menu **Administrasi → Users** oleh SUPERADMIN/ADMIN.
- Di stack dev lokal saat ini ada akun uji sementara (`e2e-super`, `demo-admin`). Itu **khusus dev**: hapus atau ganti passwordnya sebelum UAT, jangan pernah dibawa ke lingkungan bersama/produksi.

## 3. Role (RBAC)

| Role | Cakupan | Ringkasan hak |
|---|---|---|
| `SUPERADMIN` | Semua tenant | Kelola tenant, kredensial CWMP tenant, katalog vendor & item global |
| `ADMIN` | Satu tenant | Kelola user, device, firmware, preset, provisioning, webhook di tenant sendiri |
| `NOC` | Satu tenant | Operasional device: Connection Request, reboot, baca/ubah parameter, diagnostik |
| `VIEWER` | Satu tenant | Hanya baca |
| `ENDUSER` | Perangkat miliknya | Dikurung ke `/self-service` (tidak bisa mengakses API admin) |

Item global (`tenant_id` kosong) hanya bisa diubah/dihapus SUPERADMIN; ADMIN tenant akan menerima 403.

## 4. Keamanan login

- **Lockout:** 5 kali password salah → akun terkunci 15 menit.
- **SSO (OIDC):** bila dikonfigurasi, tombol SSO muncul di halaman login. Akun SSO tidak punya password lokal.
- **API token:** untuk integrasi/BSS, buat lewat menu token; punya umur (TTL) dan scope tenant. Kirim sebagai `Authorization: Bearer <token>`.
- Semua endpoint mutasi wajib terautentikasi; tidak ada jalur tanpa login.

## 5. Ganti password

- UI: ikon kunci di pojok atas.
- API: `PATCH /api/v1/auth/password`.

## 6. Troubleshooting

| Gejala | Penyebab umum | Tindakan |
|---|---|---|
| "Akun terkunci" | 5 percobaan salah | Tunggu 15 menit atau minta ADMIN/SUPERADMIN mereset |
| Login berhasil tapi menu kosong / 403 | Role tidak punya hak | Cek role di Administrasi → Users |
| UI tidak bisa menjangkau API | Proxy Vite/port salah | Pastikan stack `docker compose ps` sehat dan API di `:18080` |
| `seed-admin` gagal | Migrasi belum jalan / DB belum siap | `docker compose logs migrate mariadb`, ulangi setelah DB healthy |
| Sudah login di tab lain lalu dipaksa keluar | Token kedaluwarsa | Login ulang |
