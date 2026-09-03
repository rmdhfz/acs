# Panduan Login ACS Console

Ringkasan cepat. **Versi lengkap (peran, kunci akun, pemecahan masalah) ada di
`PANDUAN_LOGIN_ACS.pdf`** — bagikan PDF itu ke pengguna baru.

## 1. Akses Console

Buka browser ke URL halaman login:
- **Dev:** `http://localhost:5173/login` (frontend `npm run dev`, backend `docker compose up -d` di :18080)
- **Produksi:** sesuai domain perusahaan, mis. `https://acs.perusahaan.com/login`

## 2. Kredensial

Tidak ada pendaftaran mandiri dan **tidak ada akun default yang bisa diandalkan**.
Akun dibuat lewat *Administration → Users* oleh Admin/Superadmin. Untuk instalasi
baru, buat Superadmin pertama dengan `seed-admin` (bagian 3).

## 3. Cara Membuat Akun Superadmin Baru

Jika Anda membutuhkan akses langsung atau lupa password, Anda dapat membuat akun Superadmin baru menggunakan script `seed-admin` yang sudah disertakan di dalam *container* backend (`acsd`).

Jalankan perintah berikut di terminal (pastikan berada di dalam folder project `acs` dan Docker Compose sedang berjalan):

```bash
docker compose exec acsd /app/seed-admin -username <USERNAME_BARU> -password <PASSWORD_BARU> -email <EMAIL_BARU>
```

**Contoh Penggunaan:**
```bash
docker compose exec acsd /app/seed-admin -username admin_baru -password rahasia123 -email admin_baru@example.com
```

Setelah perintah tersebut berhasil dieksekusi, Anda dapat langsung login di halaman ACS Console menggunakan `admin_baru` dan password `rahasia123`.

## 4. SSO Login (Jika Tersedia)
Jika integrasi SSO (OIDC) telah dikonfigurasi (`ACS_OIDC_*`), tekan tombol
**Masuk dengan SSO** di halaman login. Bila belum dikonfigurasi, tombol tetap
muncul tapi mengarah ke halaman "tidak ditemukan" — pakai username/password.

## 5. Ganti password sendiri

Setelah login, klik ikon **kunci** di pojok kiri bawah (samping nama + tombol
Logout). Semua peran bisa. Alternatif API: `PATCH /api/v1/auth/password`
(butuh `current_password`).

## 6. Akun terkunci

5× password salah berturut-turut → akun terkunci **15 menit**. Pesannya tetap
"username atau password salah" (disamarkan). Solusi: tunggu 15 menit, atau minta
Admin melakukan *Reset Password* (langsung membuka kunci). Detail di PDF bagian 6.
