# Panduan Login ACS Console

Dokumen ini berisi panduan untuk masuk (login) ke dalam ACS Console.

## 1. Akses Console

Buka browser Anda dan akses URL berikut:
**[http://localhost:5173/login](http://localhost:5173/login)**

## 2. Kredensial Default

Secara bawaan, sudah terdapat beberapa akun percobaan di dalam database (misalnya `admin` atau `superadmin`). Namun, jika Anda tidak mengetahui password dari akun tersebut, sangat disarankan untuk membuat akun Superadmin baru.

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
Jika integrasi SSO (Single Sign-On) telah dikonfigurasi pada environment, Anda juga bisa menekan tombol **Login with SSO** pada halaman login.
