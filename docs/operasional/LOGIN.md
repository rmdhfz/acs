# Cara Masuk ke ACS

Panduan login untuk Tim Operations dan developer (satu-satunya panduan login; versi lama `LOGIN_INSTRUCTION.md` dan `PANDUAN_LOGIN_ACS` sudah digabung/diarsipkan — lihat `docs/arsip/`). Penjelasan peran dan MFA yang lebih bergambar ada di `docs/panduan/BUKU_PANDUAN_ACS.html` bab 4.

> **Tidak ada akun bawaan (default account).** Akun pertama dibuat lewat `seed-admin` (§2). Jangan menulis password di file ini, di commit, atau di chat tim.

## 1. Alamat

| Lingkungan | UI | REST API |
|---|---|---|
| Dev lokal (Vite) | http://localhost:5173/login | http://localhost:18080/api/v1 |
| Docker compose | sesuai reverse proxy / port yang dipublish | port `ACS_HTTP_ADDR` (default `:8080` di container) |

Endpoint CPE (TR-069) **terpisah** dari UI: `http://<host>:7547/cwmp`. Login UI/API tidak berlaku di sana — CPE memakai kredensial CWMP per-tenant (Basic Auth).

## 2. Membuat akun SUPERADMIN pertama

```bash
docker compose exec acsd /app/seed-admin -username <USERNAME> -password <PASSWORD> -email <EMAIL> -fullname "<NAMA>"
```

- Di Git Bash Windows awali dengan `MSYS_NO_PATHCONV=1` (kalau tidak, path `/app/...` diterjemahkan keliru).
- Login selanjutnya memakai **email** + password. Password disimpan sebagai hash **Argon2id** (64 MiB, t=3, p=2); hash bcrypt lama otomatis di-upgrade saat login berhasil.
- Gunakan password kuat dan unik (≥ 12 karakter, campuran). Batas server: 8–128 karakter.
- Akun berikutnya dibuat dari **Administrasi → Users** oleh SUPERADMIN/ADMIN. Email wajib valid & unik; akun baru menerima email selamat datang (tanpa password).
- Akun uji lokal (`e2e-super`, `demo-admin`) hanya untuk dev — hapus/ganti sebelum UAT.

## 3. Masuk (email + password)

Halaman login meminta **email** dan password (email tak peka huruf besar/kecil). Tambahan UX: tampilkan/sembunyikan password, peringatan Caps Lock, opsi "Ingat email di perangkat ini" (hanya email yang disimpan, tidak pernah password).
Untuk skrip lama, API masih menerima field `username` sebagai pengganti `email`.

### 3b. Masuk dengan Google / SSO
Bila admin mengisi `ACS_OIDC_*` (panduan: `docs/operasional/RUNBOOK.md` §R), halaman login menampilkan tombol **Masuk dengan Google** (atau **Masuk dengan SSO** untuk IdP lain). Syarat: email akun Google sama dengan email akun ACS dan terverifikasi di Google; akun tidak dibuat otomatis (admin mendaftarkannya dulu). Login pertama mengikat akun ke ID Google; mengubah email akun mengosongkan ikatan itu. Login SSO tidak meminta kode MFA lokal.

### 3c. Verifikasi dua langkah (MFA TOTP) & pendaftaran mandiri
Pengguna mengaktifkan MFA sendiri di Profil → Keamanan; login berikutnya meminta kode 6 digit setelah password (kode pemulihan tersedia). Halaman **Daftar** (`/signup`) membuat tenant baru lewat email verifikasi; **bawaan nonaktif** (`ACS_SELF_SIGNUP=true` + SMTP) — lihat RUNBOOK §N.

## 4. Lupa password (lewat email)

1. Di halaman login klik **Lupa password?**, isi email akun.
2. Jika email terdaftar, sistem mengirim tautan `…/reset-password#token=…` (berlaku **30 menit**, **sekali pakai**). Respons layar selalu sama — baik email terdaftar atau tidak (anti enumerasi akun).
3. Buka tautan, buat password baru → sistem mengirim email konfirmasi dan lockout akun dibersihkan.

Batas: 3 permintaan per akun per jam, dan rate limit per-IP. Token hanya disimpan sebagai hash (SHA-256); tautan memakai *fragment* sehingga token tidak masuk log server maupun header Referer.

**Konfigurasi email** (di `.env`, lihat `.env.example`): `ACS_SMTP_HOST`, `ACS_SMTP_PORT` (587 = STARTTLS, 465 = TLS implisit), `ACS_SMTP_USERNAME`, `ACS_SMTP_PASSWORD`, `ACS_SMTP_FROM`, `ACS_PUBLIC_URL` (origin frontend untuk tautan). Untuk Gmail pakai **App Password** (butuh 2-Step Verification), bukan password akun Google. STARTTLS wajib — kredensial tidak pernah dikirim lewat koneksi polos. Tanpa SMTP, "lupa password" menjawab 503 dan reset dilakukan Admin lewat *Administrasi → Users → Reset password*.

## 5. Email notifikasi yang dikirim

| Peristiwa | Penerima |
|---|---|
| Permintaan reset password | pemilik akun (tautan) |
| Password berhasil diganti (reset email / ganti sendiri) | pemilik akun |
| Password direset oleh admin | pemilik akun |
| Akun terkunci (5× salah password) | pemilik akun |
| Akun baru dibuat admin | pemilik akun baru |

Email **tidak pernah memuat password**. Alamat berdomain khusus-uji (`*.local`, `*.test`, `example.*`, …) dilewati agar akun uji/e2e tidak menghasilkan bounce.

## 6. Role (RBAC)

| Role | Cakupan | Ringkasan hak |
|---|---|---|
| `SUPERADMIN` | Semua tenant | Kelola tenant, kredensial CWMP tenant, katalog vendor & item global |
| `ADMIN` | Satu tenant | Kelola user, device, firmware, preset, provisioning, webhook di tenant sendiri |
| `NOC` | Satu tenant | Operasional device: Connection Request, reboot, baca/ubah parameter, diagnostik |
| `VIEWER` | Satu tenant | Hanya baca |
| `ENDUSER` | Perangkat miliknya | Dikurung ke `/self-service` (tidak bisa mengakses API admin) |

Item global (`tenant_id` kosong) hanya bisa diubah/dihapus SUPERADMIN; ADMIN tenant akan menerima 403.

## 7. Keamanan sesi & login

- **Lockout:** 5 kali password salah → akun terkunci 15 menit (pemilik menerima email). Pesan error disamarkan seragam: "email atau password salah".
- **Pencabutan sesi:** JWT yang terbit sebelum password diganti (reset email, ganti sendiri, reset admin) ditolak; akun yang dinonaktifkan/dihapus juga langsung kehilangan akses. Ganti password dari Profil menerbitkan token baru, jadi Anda tidak terlempar.
- **SSO (OIDC/Google):** bila dikonfigurasi, tombol SSO/Google aktif (lihat §3b). Pengguna SSO tetap boleh punya password lokal; batasi domain dengan `ACS_OIDC_ALLOWED_DOMAINS`.
- **API token:** untuk integrasi/BSS, buat di Administrasi; punya umur (TTL) dan scope tenant. Kirim sebagai `Authorization: Bearer <token>`.
- Endpoint mutasi wajib terautentikasi, kecuali yang setara login (`/auth/login`, `/auth/forgot-password`, `/auth/reset-password`) yang dijaga rate limit, token sekali pakai, dan respons seragam.

## 8. Ganti password sendiri

Profil → Keamanan → **Ganti**, atau `PATCH /api/v1/auth/password` (wajib `current_password`).

## 9. Troubleshooting

| Gejala | Penyebab umum | Tindakan |
|---|---|---|
| "Email atau password salah" berulang | Salah ketik / akun terkunci | Tunggu 15 menit atau **Lupa password?** |
| Email reset tidak datang | Folder spam, SMTP belum dikonfigurasi, atau domain email reserved | Cek spam; pastikan `ACS_SMTP_*` terisi; lihat log `acsd` (`mailer:`) |
| "Tautan tidak valid" saat reset | Sudah dipakai / lewat 30 menit / password diganti sejak itu | Minta tautan baru |
| Login berhasil tapi menu kosong / 403 | Role tidak punya hak | Cek role di Administrasi → Users |
| "Server sedang sibuk" saat login | Antrean hashing Argon2 penuh (beban/serangan) | Coba lagi beberapa detik |
| UI tidak bisa menjangkau API | Proxy Vite/port salah | `docker compose ps`, pastikan API di `:18080` |
| `seed-admin` gagal | Migrasi belum jalan / DB belum siap / email sudah dipakai | `docker compose logs migrate mariadb`, ulangi setelah DB healthy |
