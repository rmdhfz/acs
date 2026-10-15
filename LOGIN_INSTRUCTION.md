# Panduan Login ACS Console

Ringkasan cepat. **Panduan lengkap ada di [`LOGIN.md`](LOGIN.md)**; versi bergambar
(peran, pemecahan masalah) di `PANDUAN_LOGIN_ACS.pdf`.

## 1. Akses Console

- **Dev:** `http://localhost:5173/login` (frontend `npm run dev`, backend `docker compose up -d` di :18080)
- **Produksi:** sesuai domain perusahaan, mis. `https://acs.perusahaan.com/login`

## 2. Masuk dengan Email + Password

Login memakai **email** dan password. Tidak ada pendaftaran mandiri dan **tidak
ada akun default**: akun dibuat lewat *Administration → Users* oleh Admin/Superadmin;
instalasi baru membuat Superadmin pertama dengan `seed-admin` (bagian 3).

## 3. Membuat Superadmin pertama

```bash
docker compose exec acsd /app/seed-admin -username <USERNAME> -password <PASSWORD> -email <EMAIL>
```

(Di Git Bash Windows awali dengan `MSYS_NO_PATHCONV=1`.) Password disimpan sebagai
hash **Argon2id**. Gunakan password kuat — jangan memakai contoh dari dokumen.

## 4. Lupa password (lewat email)

Di halaman login klik **Lupa password?**, isi email akun. Jika terdaftar, tautan
reset (berlaku 30 menit, sekali pakai) dikirim ke email itu. Fitur ini butuh SMTP
(`ACS_SMTP_*` di `.env`, lihat `.env.example`); tanpa SMTP, hubungi Admin untuk
*Reset Password*.

## 5. SSO (jika tersedia)

Jika OIDC dikonfigurasi (`ACS_OIDC_*`), tombol **Masuk dengan SSO** aktif. Akun SSO
tidak memakai password lokal.

## 6. Ganti password sendiri

Menu **Profil → Keamanan → Ganti** (atau ikon kunci di sidebar). API:
`PATCH /api/v1/auth/password`. Setelah password berubah, semua sesi lama dicabut
(Anda tetap login di perangkat yang mengganti).

## 7. Akun terkunci

5× password salah berturut-turut → akun terkunci **15 menit** dan pemilik akun
menerima email pemberitahuan. Pesan error tetap "email atau password salah"
(disamarkan). Solusi: tunggu, **Lupa password?**, atau minta Admin mereset.
